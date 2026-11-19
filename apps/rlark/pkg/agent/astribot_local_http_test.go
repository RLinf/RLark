package agent

import (
	"net/http"
	"testing"
)

// An agent is ready when every part it runs has said so, and not before any has.
func TestReadyWhenEveryPartIs(t *testing.T) {
	a := NewAgent(DefaultConfig())
	a.readiness.set(partNodeWatches, false)
	a.readiness.set(partNodeServer, false)
	_, changed := a.readiness.state()

	a.SetReady(true)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-changed:
		t.Fatal("readiness changed while a part was not ready")
	default:
	}

	a.readiness.set(partNodeWatches, true)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusOK)
	select {
	case <-changed:
	default:
		t.Fatal("becoming ready was not announced, so the tunnel would go on saying not ready")
	}

	_, changed = a.readiness.state()
	a.SetReady(false)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-changed:
	default:
		t.Fatal("no longer being ready was not announced")
	}
}
