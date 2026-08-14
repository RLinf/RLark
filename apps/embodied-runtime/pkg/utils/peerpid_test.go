package utils

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc/peer"
)

// TestPIDFromContext verifies the handler can recover the peer PID that
// PeerPIDListener attached to the connection's RemoteAddr, and rejects
// contexts that lack it (no peer, wrong addr type, or non-positive pid).
func TestPIDFromContext(t *testing.T) {
	// Happy path: a PeerPIDAddr carries the PID.
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: PeerPIDAddr{PID: 4321, Addr: &net.UnixAddr{Name: "test", Net: "unix"}},
	})
	if got, err := PIDFromContext(ctx); err != nil || got != 4321 {
		t.Errorf("PIDFromContext = %d, err=%v, want 4321", got, err)
	}

	// No peer at all.
	if _, err := PIDFromContext(context.Background()); err == nil {
		t.Error("PIDFromContext with no peer: expected error, got nil")
	}

	// Peer addr is not a PeerPIDAddr (e.g. a raw unix addr from a normal
	// listener that was not wrapped).
	ctx2 := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.UnixAddr{Name: "x", Net: "unix"},
	})
	if _, err := PIDFromContext(ctx2); err == nil {
		t.Error("PIDFromContext with plain unix addr: expected error, got nil")
	}

	// Non-positive PID is rejected.
	ctx3 := peer.NewContext(context.Background(), &peer.Peer{
		Addr: PeerPIDAddr{PID: 0, Addr: &net.UnixAddr{Name: "test", Net: "unix"}},
	})
	if _, err := PIDFromContext(ctx3); err == nil {
		t.Error("PIDFromContext with pid=0: expected error, got nil")
	}
}
