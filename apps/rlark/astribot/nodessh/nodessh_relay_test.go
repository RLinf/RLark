package nodessh

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func relaySSHServer(t *testing.T, response string, expected string) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan string, 1)
	go func() {
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		reader := bufio.NewReaderSize(conn, 1)
		command, _ := reader.ReadString('\n')
		commands <- command
		if expected != "" && command != expected {
			return
		}
		if response != "ok\n" {
			_, _ = io.WriteString(conn, response)
			return
		}
		_, _ = io.WriteString(conn, response)
		config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == "operator" && string(password) == "login-secret" {
				return nil, nil
			}
			return nil, errors.New("unauthorized")
		}}
		config.AddHostKey(signer)
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for channel := range channels {
			ch, reqs, err := channel.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(reqs)
			go ch.Close()
		}
	}()
	return listener.Addr().String(), commands
}

func TestDialRelayAuthenticatesThenPerformsSSHWithoutForwardingCredentials(t *testing.T) {
	endpoint, commands := relaySSHServer(t, "ok\n", "gateway relay-secret 192.0.2.8:22\n")
	client, shown, failure := DialRelay(context.Background(), endpoint, "192.0.2.8", 22, "operator", "", "", "relay-secret", []byte("login-secret"))
	if failure != nil {
		t.Fatal(failure)
	}
	defer client.Close()
	if shown == "" {
		t.Fatal("SSH server host key was not recorded")
	}
	if command := <-commands; strings.Contains(command, "login-secret") || strings.Contains(command, "operator") {
		t.Fatalf("credentials were sent to relay: %q", command)
	}
}

func TestDialRelaySurfacesWrongTokenOrRefusedTarget(t *testing.T) {
	for _, response := range []string{"refused\n"} {
		t.Run(strings.TrimSpace(response), func(t *testing.T) {
			endpoint, commands := relaySSHServer(t, response, "gateway wrong-token 192.0.2.8:22\n")
			_, _, failure := DialRelay(context.Background(), endpoint, "192.0.2.8", 22, "operator", "", "", "wrong-token", []byte("login-secret"))
			if failure == nil || failure.Reason != RelayRejected {
				t.Fatalf("expected relay rejection, got %#v", failure)
			}
			if strings.Contains(failure.Error(), "wrong-token") {
				t.Fatalf("relay token leaked in error: %v", failure)
			}
			<-commands
		})
	}
}

func TestDialRelayCancellationClosesThePendingHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan *Error, 1)
	go func() {
		_, _, failure := DialRelay(ctx, listener.Addr().String(), "192.0.2.8", 22, "operator", "", "", "relay-secret", []byte("login-secret"))
		result <- failure
	}()
	conn := <-accepted
	defer conn.Close()
	cancel()
	select {
	case failure := <-result:
		if failure == nil || failure.Reason != Unreachable {
			t.Fatalf("expected canceled relay handshake to fail as unreachable, got %#v", failure)
		}
		if strings.Contains(failure.Error(), "relay-secret") {
			t.Fatal("relay token appeared in cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled relay handshake did not close promptly")
	}
}
