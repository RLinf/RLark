package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	a := NewAgent(DefaultConfig())
	probes := a.probeRouter()

	assertStatus(t, probes, http.MethodGet, "/livez", http.StatusOK)
	assertStatus(t, probes, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	a.SetReady(true)
	assertStatus(t, probes, http.MethodGet, "/readyz", http.StatusOK)
	assertStatus(t, probes, http.MethodGet, "/metrics", http.StatusNotFound)

	// One registry holds the process's, the controllers' and the node server's metrics.
	metrics := a.metricsRouter()
	body := assertStatus(t, metrics, http.MethodGet, "/metrics", http.StatusOK)
	for _, name := range []string{"go_goroutines", "rlark_nodeserver_connections_active"} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics has no %s", name)
		}
	}
	assertStatus(t, metrics, http.MethodGet, "/readyz", http.StatusNotFound)
	assertStatus(t, metrics, http.MethodGet, "/debug/pprof/", http.StatusOK)
	assertStatus(t, metrics, http.MethodGet, "/debug/pprof/goroutine", http.StatusOK)
}

func TestDrainMarksAgentNotReadyAndCancels(t *testing.T) {
	a := NewAgent(DefaultConfig())
	drainCtx, cancel := context.WithCancel(context.Background())
	a.drainMu.Lock()
	a.drain = cancel
	a.drainMu.Unlock()
	a.SetReady(true)

	assertStatus(t, a.probeRouter(), http.MethodPost, "/drain", http.StatusOK)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-drainCtx.Done():
	default:
		t.Fatal("drain did not cancel agent context")
	}
}

func assertStatus(t *testing.T, handler http.Handler, method, path string, want int) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != want {
		t.Fatalf("%s %s status = %d, want %d", method, path, recorder.Code, want)
	}
	return recorder.Body.String()
}
