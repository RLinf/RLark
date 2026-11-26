package utils

import (
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type unsupportedReadDeadlineConn struct {
	net.Conn
	closes atomic.Int64
}

func (*unsupportedReadDeadlineConn) SetReadDeadline(time.Time) error {
	return errors.New("read deadline unsupported")
}

func (c *unsupportedReadDeadlineConn) Close() error {
	c.closes.Add(1)
	return c.Conn.Close()
}

func TestRelayUnsupportedDeadlineDrains(t *testing.T) {
	for _, bothUnsupported := range []bool{false, true} {
		t.Run(map[bool]string{false: "one", true: "both"}[bothUnsupported], func(t *testing.T) {
			origin, originPeer := net.Pipe()
			remote, remotePeer := net.Pipe()
			defer func() { _ = origin.Close(); _ = remote.Close(); _ = originPeer.Close(); _ = remotePeer.Close() }()
			one := &unsupportedReadDeadlineConn{Conn: origin}
			two := remote
			if bothUnsupported {
				two = &unsupportedReadDeadlineConn{Conn: remote}
			}
			done := make(chan struct{})
			go func() { _, _ = relayConnectionsWithDrainTimeout(one, two, 50*time.Millisecond); close(done) }()
			_ = originPeer.Close()
			select {
			case <-done:
				t.Fatal("relay skipped the opposite-direction drain budget")
			case <-time.After(10 * time.Millisecond):
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("unsupported deadline retained an ended stream")
			}
			if bothUnsupported {
				require.Equal(t, int64(1), one.closes.Load())
				require.Equal(t, int64(1), two.(*unsupportedReadDeadlineConn).closes.Load())
			}
		})
	}
}

func TestRelayCompletedDrainCancelsFallback(t *testing.T) {
	origin, originPeer := net.Pipe()
	remote, remotePeer := net.Pipe()
	defer func() { _ = origin.Close(); _ = remote.Close() }()
	one := &unsupportedReadDeadlineConn{Conn: origin}
	two := &unsupportedReadDeadlineConn{Conn: remote}
	done := make(chan struct{})
	go func() { _, _ = relayConnectionsWithDrainTimeout(one, two, 50*time.Millisecond); close(done) }()
	_ = originPeer.Close()
	_ = remotePeer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completed drain did not return")
	}
	time.Sleep(75 * time.Millisecond)
	require.Zero(t, one.closes.Load(), "canceled fallback must not close returned connections later")
	require.Zero(t, two.closes.Load())
}

func TestRelayActiveStreamsHaveNoDrainTimer(t *testing.T) {
	origin, originPeer := net.Pipe()
	remote, remotePeer := net.Pipe()
	defer func() { _ = origin.Close(); _ = remote.Close(); _ = originPeer.Close(); _ = remotePeer.Close() }()
	one := &unsupportedReadDeadlineConn{Conn: origin}
	two := &unsupportedReadDeadlineConn{Conn: remote}
	done := make(chan struct{})
	go func() { _, _ = relayConnectionsWithDrainTimeout(one, two, 20*time.Millisecond); close(done) }()
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, one.closes.Load())
	require.Zero(t, two.closes.Load())
	_ = originPeer.SetDeadline(time.Now().Add(time.Second))
	_ = remotePeer.SetDeadline(time.Now().Add(time.Second))
	for _, pair := range [][2]net.Conn{{originPeer, remotePeer}, {remotePeer, originPeer}} {
		written := make(chan error, 1)
		go func() { _, err := pair[0].Write([]byte("active")); written <- err }()
		data := make([]byte, 6)
		_, err := io.ReadFull(pair[1], data)
		require.NoError(t, err)
		require.Equal(t, "active", string(data))
		require.NoError(t, <-written)
	}
	_ = originPeer.Close()
	_ = remotePeer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active stream did not drain after peers exited")
	}
}
