// Package utils provides small, reusable helpers shared across the
// embodied-runtime packages.
//
// Currently it hosts the Unix-socket peer-PID machinery: a listener/conn
// wrapper that captures the caller's PID via SO_PEERCRED and exposes it to
// gRPC handlers, plus the host-network detection used to skip pods that
// already run in the host netns.
package utils

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc/peer"
)

// ---------------------------------------------------------------------------
// Peer-PID listener / conn wrapper
// ---------------------------------------------------------------------------

// PeerPIDListener wraps a Unix socket listener. On Accept it reads the peer
// PID (SO_PEERCRED via GetPeerPID) and returns a PeerPIDConn whose
// RemoteAddr carries that PID. gRPC copies conn.RemoteAddr() into
// peer.Peer.Addr, so the handler recovers the PID via PIDFromContext.
//
// Construct with the underlying listener as the embedded field, e.g.
//
//	l := &utils.PeerPIDListener{Listener: raw}
type PeerPIDListener struct {
	net.Listener
}

// Accept waits for and returns the next connection, wrapping it in a
// PeerPIDConn that carries the peer PID.
func (l *PeerPIDListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	pid, err := GetPeerPID(c)
	if err != nil {
		// Without a peer PID the caller's netns cannot be targeted — reject
		// the connection so the client fails loudly rather than silently.
		_ = c.Close()
		return nil, fmt.Errorf("read peer pid: %w", err)
	}
	return &PeerPIDConn{Conn: c, PID: pid}, nil
}

// PeerPIDConn is a net.Conn that reports a PeerPIDAddr as its RemoteAddr so
// the PID is visible to gRPC handlers via peer.FromContext. All I/O is
// delegated to the wrapped conn.
type PeerPIDConn struct {
	net.Conn
	// PID is the peer process PID captured at Accept time.
	PID int
}

// RemoteAddr returns a PeerPIDAddr carrying the peer PID and the wrapped
// connection's remote address.
func (c *PeerPIDConn) RemoteAddr() net.Addr {
	return PeerPIDAddr{PID: c.PID, Addr: c.Conn.RemoteAddr()}
}

// PeerPIDAddr carries the peer PID alongside the original address. It
// satisfies net.Addr; String encodes the PID for log readability.
type PeerPIDAddr struct {
	// PID is the peer process PID, captured from the socket credentials.
	PID int
	net.Addr
}

func (a PeerPIDAddr) String() string {
	return fmt.Sprintf("peer-pid=%d", a.PID)
}

// PIDFromContext extracts the peer PID that PeerPIDListener attached to the
// connection's RemoteAddr. Returns an error if the peer is absent (e.g. the
// RPC did not arrive over a PeerPIDListener-wrapped listener) or the PID is
// non-positive.
func PIDFromContext(ctx context.Context) (int, error) {
	pr, ok := peer.FromContext(ctx)
	if !ok || pr.Addr == nil {
		return 0, fmt.Errorf("no peer in context")
	}
	a, ok := pr.Addr.(PeerPIDAddr)
	if !ok {
		return 0, fmt.Errorf("peer addr %T does not carry a pid", pr.Addr)
	}
	if a.PID <= 0 {
		return 0, fmt.Errorf("invalid peer pid %d", a.PID)
	}
	return a.PID, nil
}
