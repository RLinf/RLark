package controller

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDialNodeSSHPinsHostKey(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) { return nil, nil }}
	config.AddHostKey(signer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			server, channels, requests, err := ssh.NewServerConn(conn, config)
			if err != nil {
				_ = conn.Close()
				continue
			}
			go ssh.DiscardRequests(requests)
			go func() {
				for ch := range channels {
					_ = ch.Reject(ssh.UnknownChannelType, "unsupported")
				}
			}()
			_ = server.Close()
		}
	}()
	cfg := NodeSSHConfig{Address: listener.Addr().String(), User: "user", Password: []byte("test"), HostKeyFingerprint: "SHA256:wrong", Timeout: time.Second}
	if client, err := DialNodeSSH(context.Background(), cfg); err == nil {
		_ = client.Close()
		t.Fatal("accepted wrong host key")
	}
	cfg.HostKeyFingerprint = ssh.FingerprintSHA256(signer.PublicKey())
	client, err := DialNodeSSH(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	<-done
}

func TestBoundedNodeOutput(t *testing.T) {
	var output boundedNodeOutput
	input := bytes.Repeat([]byte("x"), maxNodeCommandOutput+10)
	if n, err := output.Write(input); err != nil || n != len(input) {
		t.Fatalf("write: %d %v", n, err)
	}
	if !output.truncated || output.data.Len() != maxNodeCommandOutput {
		t.Fatal("output was not bounded")
	}
	if _, err := output.Write([]byte("more")); err != nil {
		t.Fatal(err)
	}
}

func TestDialNodeSSHRejectsMissingCredentialsAndCancellation(t *testing.T) {
	if _, err := DialNodeSSH(context.Background(), NodeSSHConfig{}); err == nil {
		t.Fatal("accepted empty configuration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DialNodeSSH(ctx, NodeSSHConfig{Address: "127.0.0.1:1", User: "user", Password: []byte("test"), HostKeyFingerprint: "SHA256:test"}); err == nil {
		t.Fatal("accepted cancelled context")
	}
}
