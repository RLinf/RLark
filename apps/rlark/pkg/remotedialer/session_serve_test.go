package remotedialer

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"
)

// TestSession_writeControl verifies that peering commands are serialized to the
// control stream in the expected wire format.
func TestSession_writeControl(t *testing.T) {
	t.Parallel()

	local, remote := net.Pipe()
	defer func() { _ = local.Close() }()
	defer func() { _ = remote.Close() }()

	s := NewServerSession(rand.Int63(), "", nil)
	s.control = local

	lines := make(chan string, 2)
	go func() {
		scanner := bufio.NewScanner(remote)
		defer func() {
			_ = scanner.Err() // ignore errors on close
		}()
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	go func() {
		s.sessionAdded("clientA", 42)
		s.sessionRemoved("clientA", 42)
	}()

	want := []string{"ADD clientA/42", "REMOVE clientA/42"}
	for _, w := range want {
		select {
		case got := <-lines:
			if got != w {
				t.Errorf("control line mismatch, got %q want %q", got, w)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for control line %q", w)
		}
	}
}

// TestSession_serveControl verifies the receiving side updates its remote client
// mapping when control commands arrive.
func TestSession_serveControl(t *testing.T) {
	t.Parallel()

	local, remote := net.Pipe()
	defer func() { _ = local.Close() }()
	defer func() { _ = remote.Close() }()

	s := NewServerSession(rand.Int63(), "", nil)
	go s.serveControl(local)

	clientKey, sessionKey := "peerclient", rand.Int()
	_, _ = fmt.Fprintf(remote, "ADD %s/%d\n", clientKey, sessionKey)

	waitFor(t, func() bool {
		return len(s.getSessionKeys(clientKey)) == 1
	}, "remote client not added")

	_, _ = fmt.Fprintf(remote, "REMOVE %s/%d\n", clientKey, sessionKey)
	waitFor(t, func() bool {
		return len(s.getSessionKeys(clientKey)) == 0
	}, "remote client not removed")
}

func TestSession_writeControlNoPeer(t *testing.T) {
	t.Parallel()

	// A session with no control stream (plain client) must not error out.
	s := &Session{remoteClientKeys: map[string]map[int]bool{}}
	if err := s.writeControl(AddClient, "clientA/1"); err != nil {
		t.Errorf("unexpected error writing control with no peer: %v", err)
	}
}

func TestReadLineLimit(t *testing.T) {
	t.Parallel()

	_, _, err := readConnectHeader(strings.NewReader(strings.Repeat("x", 1024)))
	if err == nil {
		t.Fatal("expected error for overly long connect header")
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}
