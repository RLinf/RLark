package remotedialer

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtaci/smux"
)

func TestClientDialCancellationClosesIdleTarget(t *testing.T) {
	stream, peer := net.Pipe()
	target, targetPeer := net.Pipe()
	defer func() { _ = peer.Close(); _ = targetPeer.Close() }()
	done := make(chan struct{})
	admitted := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		clientDial(ctx, func(context.Context, string, string) (net.Conn, error) { return target, nil }, stream, "tcp", "test", func() { close(admitted) })
	}()
	<-admitted
	_ = peer.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream end retained idle target")
	}
	_ = targetPeer.SetReadDeadline(time.Now().Add(time.Second))
	_, err := targetPeer.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func TestSessionPendingLimitRecoveryAndSiblingTraffic(t *testing.T) {
	one, two := net.Pipe()
	serverMux, err := smux.Server(one, smuxConfig())
	require.NoError(t, err)
	clientMux, err := smux.Client(two, smuxConfig())
	require.NoError(t, err)
	defer func() { _ = serverMux.Close(); _ = clientMux.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{mux: serverMux, auth: func(string, string) bool { return true }, dialer: func(context.Context, string, string) (net.Conn, error) {
		c, peer := net.Pipe()
		go func() { defer func() { _ = peer.Close() }(); _, _ = io.Copy(peer, peer) }()
		return c, nil
	}}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("session failed to join")
		}
	})
	open := func() *smux.Stream {
		stream, err := clientMux.OpenStream()
		require.NoError(t, err)
		return stream
	}
	active := open()
	defer func() { _ = active.Close() }()
	require.NoError(t, writeConnectHeader(active, "tcp", "echo"))
	// Observe actual relay traffic before checking the released admission slot;
	// zero alone could also mean the stream has not reached Accept yet.
	_ = active.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = active.Write([]byte("ready"))
	require.NoError(t, err)
	ready := make([]byte, 5)
	_, err = io.ReadFull(active, ready)
	require.NoError(t, err)
	require.Equal(t, "ready", string(ready))
	waitFor(t, func() bool { s.RLock(); defer s.RUnlock(); return s.pendingStreams == 0 }, "established stream retained admission slot")
	var pending []*smux.Stream
	for range maxPendingStreams {
		pending = append(pending, open())
	}
	defer func() {
		for _, stream := range pending {
			_ = stream.Close()
		}
	}()
	waitFor(t, func() bool { s.RLock(); defer s.RUnlock(); return s.pendingStreams == maxPendingStreams }, "pending limit not filled")
	overflow := open()
	defer func() { _ = overflow.Close() }()
	require.NoError(t, writeConnectHeader(overflow, "tcp", "echo"))
	_ = overflow.SetReadDeadline(time.Now().Add(time.Second))
	_, err = overflow.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	exchange := func(stream *smux.Stream) {
		_ = stream.SetDeadline(time.Now().Add(time.Second))
		_, err := stream.Write([]byte("live"))
		require.NoError(t, err)
		data := make([]byte, 4)
		_, err = io.ReadFull(stream, data)
		require.NoError(t, err)
		require.Equal(t, "live", string(data))
	}
	exchange(active)
	for _, stream := range pending {
		_ = stream.Close()
	}
	waitFor(t, func() bool { s.RLock(); defer s.RUnlock(); return s.pendingStreams == 0 }, "closed pending streams did not release budget")
	fresh := open()
	defer func() { _ = fresh.Close() }()
	require.NoError(t, writeConnectHeader(fresh, "tcp", "echo"))
	exchange(fresh)
}

func TestClientDialCancellationAndAdmissionRelease(t *testing.T) {
	stream, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan struct{})
	var releases atomic.Int64
	go func() {
		defer close(done)
		clientDial(ctx, func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}, stream, "tcp", "test", func() { releases.Add(1) })
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending target dial ignored cancellation")
	}
	require.Equal(t, int64(1), releases.Load())
}

func TestSuccessfulDialTransfersOwnershipBeforeContextCancellation(t *testing.T) {
	one, two := net.Pipe()
	serverMux, err := smux.Server(one, smuxConfig())
	require.NoError(t, err)
	clientMux, err := smux.Client(two, smuxConfig())
	require.NoError(t, err)
	defer func() { _ = serverMux.Close(); _ = clientMux.Close() }()
	s := &Session{mux: serverMux}
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		stream, err := clientMux.AcceptStream()
		if err != nil {
			return
		}
		defer func() { _ = stream.Close() }()
		if _, _, err := readConnectHeader(stream); err != nil {
			return
		}
		_, _ = io.Copy(stream, stream)
	}()
	t.Cleanup(func() { _ = clientMux.Close(); <-peerDone })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan net.Conn, 1)
	errors := make(chan error, 1)
	go func() { c, err := s.Dial(ctx, "tcp", "echo"); result <- c; errors <- err }()
	var conn net.Conn
	select {
	case conn = <-result:
		require.NoError(t, <-errors)
	case <-time.After(time.Second):
		t.Fatal("successful dial failed to return")
	}
	defer func() { _ = conn.Close() }()
	cancel()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, err = conn.Write([]byte("live"))
	require.NoError(t, err)
	data := make([]byte, 4)
	_, err = io.ReadFull(conn, data)
	require.NoError(t, err)
	require.Equal(t, "live", string(data))
}

func TestClientDialPreservesResponseAfterPeerCloseWrite(t *testing.T) {
	one, two := net.Pipe()
	serverMux, err := smux.Server(one, smuxConfig())
	require.NoError(t, err)
	clientMux, err := smux.Client(two, smuxConfig())
	require.NoError(t, err)
	defer func() { _ = serverMux.Close(); _ = clientMux.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream, err := serverMux.AcceptStream()
		if err != nil {
			return
		}
		clientDial(ctx, func(context.Context, string, string) (net.Conn, error) {
			c, peer := net.Pipe()
			go func() {
				defer func() { _ = peer.Close() }()
				request := make([]byte, 4)
				if _, err := io.ReadFull(peer, request); err != nil {
					return
				}
				time.Sleep(20 * time.Millisecond)
				_, _ = peer.Write([]byte("response"))
			}()
			return c, nil
		}, stream, "tcp", "test", func() {})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("half-close relay failed to join")
		}
	})
	stream, err := clientMux.OpenStream()
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()
	_ = stream.SetDeadline(time.Now().Add(time.Second))
	_, err = stream.Write([]byte("body"))
	require.NoError(t, err)
	require.NoError(t, stream.CloseWrite())
	response := make([]byte, 8)
	_, err = io.ReadFull(stream, response)
	require.NoError(t, err)
	require.Equal(t, "response", string(response))
}
