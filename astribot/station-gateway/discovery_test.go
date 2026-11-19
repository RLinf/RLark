package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// sshHost is an SSH server that counts every attempt to authenticate to it.
func sshHost(t *testing.T) (port int, fingerprint string, attempts *atomic.Int32) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	attempts = &atomic.Int32{}
	config := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		AuthLogCallback:  func(ssh.ConnMetadata, string, error) { attempts.Add(1) },
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				if server, _, _, err := ssh.NewServerConn(conn, config); err == nil {
					_ = server.Close()
				}
				_ = conn.Close()
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, ssh.FingerprintSHA256(signer.PublicKey()), attempts
}

func TestScanReportsTheHostsWithAFactoryKeyAndAsksNoHostToAuthenticate(t *testing.T) {
	robotPort, robotKey, robotAttempts := sshHost(t)
	otherPort, _, otherAttempts := sshHost(t)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	_ = closed.Close()
	d := &discovery{Ranges: []string{"127.0.0.1/32"}, Ports: []int{robotPort, otherPort, closedPort}, HostKeys: []string{robotKey}}
	found, err := d.scan(context.Background(), func(string) string { return "48:b0:2d:5c:9a:11" })
	if err != nil {
		t.Fatal(err)
	}
	if want := []discoveredHost{{Address: "127.0.0.1", Port: robotPort, MAC: "48:b0:2d:5c:9a:11", HostKey: robotKey}}; !reflect.DeepEqual(found, want) {
		t.Fatalf("found %+v", found)
	}
	if robotAttempts.Load() != 0 || otherAttempts.Load() != 0 {
		t.Fatalf("attempts to authenticate: %d at the robot, %d at the other host", robotAttempts.Load(), otherAttempts.Load())
	}
	// A host that is not on the hub's own network has no hardware address to be known by.
	if found, _ := d.scan(context.Background(), func(string) string { return "" }); len(found) != 0 {
		t.Fatalf("a host without a hardware address: %+v", found)
	}

	for text, count := range map[string]int{"192.168.31.0/24": 254, "192.168.31.4/30": 2, "192.168.31.7/32": 1} {
		if addresses, err := (&discovery{Ranges: []string{text}}).addresses(); err != nil || len(addresses) != count {
			t.Fatalf("%s: %d addresses, %v", text, len(addresses), err)
		}
	}
	if _, err := (&discovery{Ranges: []string{"10.0.0.0/8"}}).addresses(); err == nil {
		t.Fatal("a range of sixteen million addresses is looked at")
	}
}

func TestRelayConnectsOnePeerOnlyToTheRangesAndPortsOfTheDocument(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	var dialed atomic.Int32
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			dialed.Add(1)
			go func() { _, _ = io.Copy(conn, conn); _ = conn.Close() }()
		}
	}()
	port := echo.Addr().(*net.TCPAddr).Port
	d := &discovery{Ranges: []string{"127.0.0.0/30"}, Ports: []int{port}, RelayFrom: "127.0.0.1"}
	relay, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	go func() {
		for {
			conn, err := relay.Accept()
			if err != nil {
				return
			}
			go d.relay(conn)
		}
	}()
	// ask names a host to the relay from a peer's address and returns the relay's answer.
	ask := func(from, target string) (string, net.Conn) {
		t.Helper()
		conn, err := (&net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(from)}}).Dial("tcp", relay.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(conn, target+"\n")
		answer, _ := bufio.NewReader(conn).ReadString('\n')
		return answer, conn
	}
	answer, conn := ask("127.0.0.1", "127.0.0.1:"+strconv.Itoa(port))
	if answer != "ok\n" {
		t.Fatalf("a host of the ranges, asked for by the node the management plane dials from: %q", answer)
	}
	_, _ = io.WriteString(conn, "through\n")
	if line, _ := bufio.NewReader(conn).ReadString('\n'); line != "through\n" {
		t.Fatalf("what came back through the relay: %q", line)
	}
	_ = conn.Close()
	for _, target := range []string{"127.0.0.9:" + strconv.Itoa(port), "127.0.0.1:" + strconv.Itoa(port+1), "example.com:22", "10.254.240.1:18443"} {
		if answer, conn := ask("127.0.0.1", target); answer != "refused\n" {
			t.Fatalf("%s: %q", target, answer)
		} else {
			_ = conn.Close()
		}
	}

	// Any other peer of the tunnel is closed without an answer, and nothing is dialed for it.
	before := dialed.Load()
	answer, conn = ask("127.0.0.2", "127.0.0.1:"+strconv.Itoa(port))
	_ = conn.Close()
	if answer != "" || dialed.Load() != before {
		t.Fatalf("another peer: answered %q, %d hosts dialed for it", answer, dialed.Load()-before)
	}
}
