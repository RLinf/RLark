package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	a := NewAgent(DefaultConfig())
	router := a.healthRouter()

	assertStatus(t, router, http.MethodGet, "/livez", http.StatusOK)
	assertStatus(t, router, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	a.SetReady(true)
	assertStatus(t, router, http.MethodGet, "/readyz", http.StatusOK)
	assertStatus(t, router, http.MethodGet, "/metrics", http.StatusOK)
	assertStatus(t, router, http.MethodGet, "/debug/pprof/", http.StatusOK)
	assertStatus(t, router, http.MethodGet, "/debug/pprof/goroutine", http.StatusOK)
}

func TestDrainMarksAgentNotReadyAndCancels(t *testing.T) {
	a := NewAgent(DefaultConfig())
	drainCtx, cancel := context.WithCancel(context.Background())
	a.drainMu.Lock()
	a.drain = cancel
	a.drainMu.Unlock()
	a.SetReady(true)

	assertStatus(t, a.healthRouter(), http.MethodPost, "/drain", http.StatusOK)
	assertStatus(t, a.healthRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-drainCtx.Done():
	default:
		t.Fatal("drain did not cancel agent context")
	}
}

func assertStatus(t *testing.T, handler http.Handler, method, path string, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != want {
		t.Fatalf("%s %s status = %d, want %d", method, path, recorder.Code, want)
	}
}
