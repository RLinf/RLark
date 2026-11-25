package utils

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Exercise the fallback against real multiplexed SSH channels, not just a
// wrapper reporting unsupported deadlines.
func TestRelaySSHDeadlineFallbackPreservesSession(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		_ = conn.SetDeadline(time.Time{})
		defer func() { _ = server.Close() }()
		var joined sync.WaitGroup
		joined.Add(1)
		go func() { defer joined.Done(); ssh.DiscardRequests(requests) }()
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				continue
			}
			joined.Add(2)
			go func() { defer joined.Done(); ssh.DiscardRequests(requests) }()
			go func() {
				defer joined.Done()
				defer func() { _ = channel.Close() }()
				_, _ = io.Copy(channel, channel)
			}()
		}
		joined.Wait()
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-serverDone:
		case <-time.After(6 * time.Second):
			t.Error("SSH fixture did not join")
		}
	})
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	ended, err := client.Dial("tcp", "example.test:80")
	require.NoError(t, err)
	defer func() { _ = ended.Close() }()
	require.Error(t, ended.SetReadDeadline(time.Now()))
	sibling, err := client.Dial("tcp", "example.test:80")
	require.NoError(t, err)
	defer func() { _ = sibling.Close() }()
	origin, peer := net.Pipe()
	defer func() { _ = origin.Close(); _ = peer.Close() }()
	done := make(chan struct{})
	go func() { _, _ = relayConnectionsWithDrainTimeout(origin, ended, 50*time.Millisecond); close(done) }()
	_ = peer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSH opposite-direction read ignored its drain budget")
	}
	exchange := func(conn net.Conn) {
		t.Helper()
		exchanged := make(chan error, 1)
		go func() {
			if _, err := conn.Write([]byte("live")); err != nil {
				exchanged <- err
				return
			}
			data := make([]byte, 4)
			_, err := io.ReadFull(conn, data)
			if err == nil && string(data) != "live" {
				err = io.ErrUnexpectedEOF
			}
			exchanged <- err
		}()
		select {
		case err := <-exchanged:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("drain fallback disrupted another channel")
		}
	}
	exchange(sibling)
	fresh, err := client.Dial("tcp", "example.test:80")
	require.NoError(t, err, "same SSH session must accept fresh channels")
	defer func() { _ = fresh.Close() }()
	exchange(fresh)
}
