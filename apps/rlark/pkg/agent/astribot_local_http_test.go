package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// The agent's probes and its metrics are served apart, each on its own address.
func TestProbeAndMetricsEndpoints(t *testing.T) {
	a := NewAgent(DefaultConfig())
	probes := a.probeRouter()

	assertStatus(t, probes, http.MethodGet, "/livez", http.StatusOK)
	assertStatus(t, probes, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	a.SetReady(true)
	assertStatus(t, probes, http.MethodGet, "/readyz", http.StatusOK)
	assertStatus(t, probes, http.MethodGet, "/metrics", http.StatusNotFound)

	// One registry holds the process's, the controllers' and the node server's metrics.
	metrics := a.metricsRouter()
	recorder := httptest.NewRecorder()
	metrics.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, name := range []string{"go_goroutines", "rlark_nodeserver_connections_active"} {
		if !strings.Contains(recorder.Body.String(), name) {
			t.Errorf("/metrics has no %s", name)
		}
	}
	assertStatus(t, metrics, http.MethodGet, "/readyz", http.StatusNotFound)
	assertStatus(t, metrics, http.MethodGet, "/debug/pprof/", http.StatusOK)
	assertStatus(t, metrics, http.MethodGet, "/debug/pprof/goroutine", http.StatusOK)
}

func TestProbeDrainMarksAgentNotReadyAndCancels(t *testing.T) {
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

func TestTaskDaemonSetWorkersFlag(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{"--task-daemonset-push-controller-workers=7"}); err != nil {
		t.Fatal(err)
	}
	if config.TaskDaemonSetWorkers != 7 {
		t.Fatalf("task DaemonSet workers = %d, want 7", config.TaskDaemonSetWorkers)
	}
}

// An agent is ready when every part it runs has said so, and not before any has.
func TestReadyWhenEveryPartIs(t *testing.T) {
	a := NewAgent(DefaultConfig())
	a.ready.set(partNodeWatches, false)
	a.ready.set(partNodeServer, false)
	_, changed := a.ready.state()

	a.SetReady(true)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-changed:
		t.Fatal("readiness changed while a part was not ready")
	default:
	}

	a.ready.set(partNodeWatches, true)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusOK)
	select {
	case <-changed:
	default:
		t.Fatal("becoming ready was not announced, so the tunnel would go on saying not ready")
	}

	_, changed = a.ready.state()
	a.SetReady(false)
	assertStatus(t, a.probeRouter(), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-changed:
	default:
		t.Fatal("no longer being ready was not announced")
	}
}
