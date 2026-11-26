package nodeserver

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func TestLocalServiceConcurrentAdmissionStops(t *testing.T) {
	s := newTestNodeServer(time.Second)
	stop := s.startLocalService()
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := make(chan struct{})
	admitted := make(chan struct{}, 1)
	errors := make(chan error, 64)
	var joined sync.WaitGroup
	for range 64 {
		joined.Add(1)
		go func() {
			defer joined.Done()
			<-start
			conn, err := s.localServiceDialer(ctx)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			select {
			case admitted <- struct{}{}:
			default:
			}
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			_, err = conn.Write([]byte("GET /missing HTTP/1.1\r\nHost: local\r\n\r\n"))
			if err == nil {
				_, err = io.Copy(io.Discard, conn)
			}
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				errors <- err
			}
		}()
	}
	close(start)
	select {
	case <-admitted:
	case <-ctx.Done():
		t.Fatal("private HTTP admission did not start")
	}
	var stoppers sync.WaitGroup
	for range 4 {
		stoppers.Add(1)
		go func() { defer stoppers.Done(); stop() }()
	}
	stoppers.Wait()
	joined.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("admitted pipe survived HTTP shutdown: %v", err)
	}
}

func TestLocalWatchStopsBeforeWorkloadDrain(t *testing.T) {
	s := newTestNodeServer(2 * time.Second)
	s.unmarshalCred = func(string) (testPodCred, error) { return testPodCred{}, nil }
	s.config.UnixSocketAddress = filepath.Join(t.TempDir(), "node.sock")
	watching := make(chan struct{}, 1)
	s.getHosts = func(context.Context, testPodCred) (map[string]string, error) {
		select {
		case watching <- struct{}{}:
		default:
		}
		return map[string]string{}, nil
	}
	// Keep a workload stream alive so delayed local-service cleanup is visible.
	workload, peer := net.Pipe()
	wrapped := utils.NewWrapConn(workload)
	s.trackConnection(wrapped)
	releaseWorkload := sync.OnceFunc(func() { s.untrackConnection(wrapped); _ = workload.Close(); _ = peer.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	lifecycle := &testLifecycle{ready: make(chan bool, 4)}
	go func() { done <- s.Run(ctx, lifecycle) }()
	t.Cleanup(func() {
		cancel()
		releaseWorkload()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("server did not join after workload drain")
		}
	})
	waitForReady(t, lifecycle)
	conn, err := s.localServiceDialer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	request := "GET /watch_hosts?version=" + url.QueryEscape(hostsVersion(map[string]string{})) + " HTTP/1.1\r\nHost: local\r\n\r\n"
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watching:
	case <-time.After(time.Second):
		t.Fatal("watch did not start")
	}
	cancel()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := bufio.NewReader(conn).ReadByte(); err == nil {
		t.Fatal("retired watch produced a response")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("watch did not close before workload drain")
	}
	select {
	case err := <-done:
		t.Fatalf("server exited before workload drain: %v", err)
	default:
	}
}
