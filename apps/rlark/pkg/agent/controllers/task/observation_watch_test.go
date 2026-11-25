package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

// This test runs actual controller-runtime cache/list/watch sources against a
// minimal HTTP API, not a fake informer. It does not replace cluster acceptance.
func TestScopedCRDWatchSourceReplaysDeletesAndShutsDown(t *testing.T) {
	c, reconciler, task, root := observationFixture(t, nil)
	deletion := make(chan bool, 1)
	watchStarted := make(chan struct{})
	var once sync.Once
	var pathsMu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("sendInitialEvents") == "true" {
			// This mock implements classic LIST+WATCH, not watch-list bookmarks.
			http.Error(w, "watch-list unsupported", http.StatusBadRequest)
			return
		}
		pathsMu.Lock()
		paths = append(paths, req.URL.Path)
		pathsMu.Unlock()
		if req.URL.Path != "/apis/example.io/v1/namespaces/runtime/trainingjobs" {
			http.Error(w, "unexpected unscoped API path", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Query().Get("watch") != "true" {
			// Reflector replay establishes a root hint after restart/watch loss.
			if err := json.NewEncoder(w).Encode(map[string]any{"apiVersion": "example.io/v1", "kind": "TrainingJobList", "metadata": map[string]any{"resourceVersion": "1"}, "items": []any{root.Object}}); err != nil {
				t.Errorf("encode list: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		once.Do(func() { close(watchStarted) })
		select {
		case <-deletion:
			if err := json.NewEncoder(w).Encode(map[string]any{"type": "DELETED", "object": root.Object}); err != nil {
				t.Errorf("encode deletion: %v", err)
			}
			w.(http.Flusher).Flush()
		case <-req.Context().Done():
			return
		}
		<-req.Context().Done()
	}))
	defer server.Close()
	scope := reconciler.push.scope
	mapper := observationDependencyMapper()
	mapper.(*meta.DefaultRESTMapper).Add(trainingGVK, meta.RESTScopeNamespace)
	scopedCache, err := cache.New(&rest.Config{Host: server.URL}, observationCacheOptions(c, mapper, scope))
	if err != nil {
		t.Fatal(err)
	}
	// Root-only is deliberate here; owner-chain/Pod behavior has focused tests.
	runner, err := c.newObservationRunner(scopedCache, scope, []observationWatchPlan{{root: c.observations[0]}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- runner.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("observer did not shut down")
		}
	})
	select {
	case <-watchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("CRD watch did not start")
	}
	waitHint := func() trigger.Set {
		t.Helper()
		select {
		case <-c.triggers.events:
		case <-time.After(5 * time.Second):
			t.Fatal("watch did not produce a Task trigger")
		}
		key := trigger.Key{Namespace: task.Namespace, Name: task.Name}
		set, ok := c.triggers.accumulator.Take(key)
		if !ok {
			t.Fatal("no accumulated hint")
		}
		c.triggers.finish(key)
		return set
	}
	if set := waitHint(); len(set.Roots) != 1 || set.Roots[0].ID != trainingGVK.String() {
		t.Fatalf("replay trigger=%+v", set)
	}
	if err := c.LocalKubeClient.Delete(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	deletion <- true
	if set := waitHint(); !set.FullReconcileRequired || len(set.Roots) != 0 {
		t.Fatalf("delete trigger=%+v", set)
	}
	pathsMu.Lock()
	defer pathsMu.Unlock()
	if len(paths) < 2 {
		t.Fatal("list and watch did not both run")
	}
	for _, path := range paths {
		if path != fmt.Sprintf("/apis/example.io/v1/namespaces/%s/trainingjobs", scope.Namespace) {
			t.Fatalf("out-of-scope API request: %s", path)
		}
	}
}
