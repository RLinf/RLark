package nodeserver

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

type testLifecycle struct {
	ready chan bool
}

func (l *testLifecycle) SetReady(ready bool) {
	select {
	case l.ready <- ready:
	default:
	}
}

func TestDrainConnectionsWaitsForActiveConnection(t *testing.T) {
	s := newTestNodeServer(500 * time.Millisecond)
	serverConn, peerConn := net.Pipe()
	wrappedConn := utils.NewWrapConn(serverConn)
	s.trackConnection(wrappedConn)

	done := make(chan error, 1)
	go func() {
		done <- s.drainConnections(log.GetLogger())
	}()

	select {
	case <-done:
		t.Fatal("drain returned while connection was active")
	case <-time.After(20 * time.Millisecond):
	}

	s.untrackConnection(wrappedConn)
	_ = serverConn.Close()
	_ = peerConn.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain did not return after connection closed")
	}
}

func TestDrainConnectionsClosesConnectionAfterTimeout(t *testing.T) {
	s := newTestNodeServer(20 * time.Millisecond)
	serverConn, peerConn := net.Pipe()
	wrappedConn := utils.NewWrapConn(serverConn)
	s.trackConnection(wrappedConn)

	connectionDone := make(chan struct{})
	go func() {
		defer close(connectionDone)
		defer s.untrackConnection(wrappedConn)
		buffer := make([]byte, 1)
		_, _ = serverConn.Read(buffer)
	}()

	if err := s.drainConnections(log.GetLogger()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-connectionDone:
	case <-time.After(time.Second):
		t.Fatal("timed-out drain did not close active connection")
	}
	_ = peerConn.Close()
}

func TestRunStopsListeningAndDrainsConnection(t *testing.T) {
	config := DefaultConfig()
	config.UnixSocketAddress = filepath.Join(t.TempDir(), "nodeserver.sock")
	config.DrainTimeout = time.Second
	s := NewNodeServer(
		config,
		func(context.Context, int32) (testPodCred, error) { return testPodCred{}, nil },
		func(context.Context, testPodCred, string, url.Values) (utils.Dial, error) {
			return func(context.Context) (net.Conn, error) {
				server, peer := net.Pipe()
				go func() {
					defer func() { _ = peer.Close() }()
					buffer := make([]byte, 1024)
					for {
						n, err := peer.Read(buffer)
						if err != nil {
							return
						}
						if _, err := peer.Write(buffer[:n]); err != nil {
							return
						}
					}
				}()
				return server, nil
			}, nil
		},
		nil,
		func(testPodCred) (string, error) { return "pod", nil },
		func(string) (testPodCred, error) { return testPodCred{}, nil },
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	lifecycle := &testLifecycle{ready: make(chan bool, 4)}
	go func() { done <- s.Run(ctx, lifecycle) }()
	waitForReady(t, lifecycle)

	conn := dialUnixEventually(t, config.UnixSocketAddress)
	if _, err := conn.Write([]byte("tcp://10.0.0.2:80\n")); err != nil {
		t.Fatal(err)
	}
	waitForActiveConnections(t, s, 1)
	cancel()

	select {
	case <-done:
		t.Fatal("server returned before active connection drained")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := net.DialTimeout("unix", config.UnixSocketAddress, 20*time.Millisecond); err == nil {
		t.Fatal("server accepted a new connection while draining")
	}

	_ = conn.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not finish draining")
	}
}

func TestListenAtomicallySwitchesStableSocket(t *testing.T) {
	config := DefaultConfig()
	config.UnixSocketAddress = filepath.Join(t.TempDir(), "nodeserver.sock")

	oldListener, oldAddress, err := config.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = oldListener.Close()
		config.CleanupSocket(oldAddress)
	}()
	if _, err := os.Lstat(config.UnixSocketAddress); !os.IsNotExist(err) {
		t.Fatalf("stable socket published before readiness: %v", err)
	}
	if err := config.Publish(oldAddress); err != nil {
		t.Fatal(err)
	}
	if target := readSocketLink(t, config.UnixSocketAddress); target != filepath.Base(oldAddress) {
		t.Fatalf("stable socket target = %q, want %q", target, filepath.Base(oldAddress))
	}

	newListener, newAddress, err := config.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = newListener.Close()
		config.CleanupSocket(newAddress)
	}()
	if target := readSocketLink(t, config.UnixSocketAddress); target != filepath.Base(oldAddress) {
		t.Fatalf("listen changed stable socket target to %q", target)
	}
	if err := config.Publish(newAddress); err != nil {
		t.Fatal(err)
	}
	if target := readSocketLink(t, config.UnixSocketAddress); target != filepath.Base(newAddress) {
		t.Fatalf("stable socket target = %q, want %q", target, filepath.Base(newAddress))
	}

	oldAccepted := make(chan net.Conn, 1)
	go func() {
		conn, _ := oldListener.Accept()
		oldAccepted <- conn
	}()
	oldConn, err := net.Dial("unix", oldAddress)
	if err != nil {
		t.Fatalf("dial old instance after switch: %v", err)
	}
	defer func() { _ = oldConn.Close() }()
	if conn := <-oldAccepted; conn != nil {
		defer func() { _ = conn.Close() }()
	}

	newAccepted := make(chan net.Conn, 1)
	go func() {
		conn, _ := newListener.Accept()
		newAccepted <- conn
	}()
	newConn, err := net.Dial("unix", config.UnixSocketAddress)
	if err != nil {
		t.Fatalf("dial stable socket after switch: %v", err)
	}
	defer func() { _ = newConn.Close() }()
	if conn := <-newAccepted; conn != nil {
		defer func() { _ = conn.Close() }()
	}

	config.CleanupSocket(oldAddress)
	if target := readSocketLink(t, config.UnixSocketAddress); target != filepath.Base(newAddress) {
		t.Fatalf("old cleanup changed stable socket target to %q", target)
	}
}

func TestCleanupSocketLeavesStableSocketForAtomicReplacement(t *testing.T) {
	config := DefaultConfig()
	config.UnixSocketAddress = filepath.Join(t.TempDir(), "nodeserver.sock")
	listener, instanceAddress, err := config.Listen()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Publish(instanceAddress); err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()

	config.CleanupSocket(instanceAddress)
	if target := readSocketLink(t, config.UnixSocketAddress); target != filepath.Base(instanceAddress) {
		t.Fatalf("stable socket target = %q, want %q", target, filepath.Base(instanceAddress))
	}
	if _, err := os.Lstat(instanceAddress); !os.IsNotExist(err) {
		t.Fatalf("instance socket still exists after cleanup: %v", err)
	}
}

func newTestNodeServer(drainTimeout time.Duration) *NodeServer[testPodCred] {
	config := DefaultConfig()
	config.DrainTimeout = drainTimeout
	return NewNodeServer[testPodCred](config, nil, nil, nil, nil, nil)
}

func dialUnixEventually(t *testing.T, address string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", address, 20*time.Millisecond)
		if err == nil {
			return conn
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("node server did not listen on %s", address)
	return nil
}

func waitForActiveConnections(t *testing.T, s *NodeServer[testPodCred], want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.connectionsMu.Lock()
		got := len(s.connections)
		s.connectionsMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("active connections did not reach %d", want)
}

func readSocketLink(t *testing.T, address string) string {
	t.Helper()
	target, err := os.Readlink(address)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func waitForReady(t *testing.T, lifecycle *testLifecycle) {
	t.Helper()
	select {
	case ready := <-lifecycle.ready:
		if !ready {
			t.Fatal("node server became not ready before readiness")
		}
	case <-time.After(time.Second):
		t.Fatal("node server did not become ready")
	}
}
