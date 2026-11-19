package nodessh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// A machine that shows another key than the one expected is refused before
// the password is sent; with no key expected, the key shown is answered.
func TestDialRefusesAChangedHostKeyBeforeThePassword(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(private)
	var tried atomic.Int32
	config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
		tried.Add(1)
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				if _, channels, requests, err := ssh.NewServerConn(conn, config); err == nil {
					go ssh.DiscardRequests(requests)
					for range channels {
					}
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	shown := ssh.FingerprintSHA256(signer.PublicKey())

	_, _, failure := Dial(context.Background(), host, number, "ops", "SHA256:"+strings.Repeat("A", 43), []byte("pw"))
	if failure == nil || failure.Reason != HostKeyChanged || tried.Load() != 0 {
		t.Fatalf("a changed key: %v, password tried %d times", failure, tried.Load())
	}
	client, key, failure := Dial(context.Background(), host, number, "ops", "", []byte("pw"))
	if failure != nil || key != shown {
		t.Fatalf("first login: %q %v", key, failure)
	}
	client.Close()
	client, _, failure = Dial(context.Background(), host, number, "ops", shown, []byte("pw"))
	if failure != nil {
		t.Fatal(failure)
	}
	client.Close()
	if tried.Load() != 2 {
		t.Fatalf("password tried %d times", tried.Load())
	}
}
