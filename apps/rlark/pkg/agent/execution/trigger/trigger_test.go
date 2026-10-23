package trigger

import (
	"sync"
	"testing"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestAccumulatorMergeTakeDonePreservesNextBatch(t *testing.T) {
	accumulator := NewAccumulator(Config{})
	key := Key{Namespace: "management", Name: "task"}
	if got := accumulator.Merge(Trigger{Key: key, Kind: RuntimeState, ObjectUID: "uid", Generation: 1, RuntimeID: "kubernetes", Root: root("one")}); got.Status != MergeEnqueue {
		t.Fatalf("first merge=%v", got)
	}
	first, ok := accumulator.Take(key)
	if !ok || len(first.Roots) != 1 {
		t.Fatalf("first=%#v ok=%v", first, ok)
	}
	if got := accumulator.Merge(Trigger{Key: key, Kind: RuntimeState, ObjectUID: "uid", Generation: 1, RuntimeID: "kubernetes", Root: root("two")}); got.Status != MergeCoalesced {
		t.Fatalf("processing merge=%v", got)
	}
	if !accumulator.Done(key) {
		t.Fatal("Done did not request next enqueue")
	}
	second, ok := accumulator.Take(key)
	if !ok || len(second.Roots) != 1 || second.Roots[0].Name != "two" {
		t.Fatalf("second=%#v ok=%v", second, ok)
	}
	if accumulator.Done(key) {
		t.Fatal("idle Done requested enqueue")
	}
}

func TestAccumulatorConservativeMergeAndBounds(t *testing.T) {
	accumulator := NewAccumulator(Config{MaxKeys: 1, MaxRootsPerKey: 1})
	key := Key{Namespace: "management", Name: "task"}
	if accumulator.Merge(Trigger{Key: key, Kind: RuntimeState, ObjectUID: "uid", Generation: 1, RuntimeID: "one", Root: root("one")}).Status != MergeEnqueue {
		t.Fatal("first merge was not enqueued")
	}
	accumulator.Merge(Trigger{Key: key, Kind: RuntimeState, ObjectUID: "other", Generation: 2, RuntimeID: "two", Root: root("two")})
	set, ok := accumulator.Take(key)
	if !ok || !set.FullReconcileRequired || len(set.Roots) != 0 {
		t.Fatalf("set=%#v ok=%v", set, ok)
	}
	if got := accumulator.Merge(Trigger{Key: Key{Namespace: "management", Name: "other"}, Kind: DesiredState}); got.Status != MergeSaturated || got.Fallback == nil || !got.Fallback.FullReconcileRequired || got.Fallback.Key.Name != "other" {
		t.Fatalf("saturation=%v", got)
	}
}

func TestAccumulatorMergesConcurrentDuplicateHints(t *testing.T) {
	accumulator := NewAccumulator(Config{})
	key := Key{Namespace: "management", Name: "task"}
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			accumulator.Merge(Trigger{Key: key, Kind: RuntimeState, ObjectUID: "uid", Generation: 3, RuntimeID: "kubernetes", Root: root("root")})
		}()
	}
	wait.Wait()
	set, ok := accumulator.Take(key)
	if !ok || len(set.Roots) != 1 || set.Generation != 3 || set.FullReconcileRequired {
		t.Fatalf("set=%#v ok=%v", set, ok)
	}
}

func TestDesiredAndPeriodicRequireFullReconcile(t *testing.T) {
	for _, kind := range []Kind{DesiredState, Periodic} {
		accumulator := NewAccumulator(Config{})
		key := Key{Namespace: "management", Name: string(kind)}
		accumulator.Merge(Trigger{Key: key, Kind: kind, ObjectUID: "uid", Generation: 1})
		set, _ := accumulator.Take(key)
		if !set.FullReconcileRequired || !set.Has(kind) {
			t.Fatalf("kind=%s set=%#v", kind, set)
		}
	}
}

func TestAccumulatorWakeupFailureRetainsPendingHints(t *testing.T) {
	accumulator := NewAccumulator(Config{})
	key := Key{Namespace: "management", Name: "task"}
	value := Trigger{Key: key, Kind: RuntimeState, ObjectUID: "uid", Generation: 1, RuntimeID: "kubernetes", Root: root("one")}
	if accumulator.Merge(value).Status != MergeEnqueue || !accumulator.WakeupFailed(key) {
		t.Fatal("failed to release undelivered wakeup")
	}
	if accumulator.Merge(value).Status != MergeEnqueue {
		t.Fatal("retried source did not request a new wakeup")
	}
	set, ok := accumulator.Take(key)
	if !ok || len(set.Roots) != 1 || set.Roots[0].Name != "one" {
		t.Fatalf("set=%#v ok=%v", set, ok)
	}
}

func root(name string) *agentruntime.ObjectRef {
	return &agentruntime.ObjectRef{RuntimeID: "kubernetes", ID: "apps/v1, Kind=Deployment", Namespace: "runtime", Name: name, UID: name + "-uid"}
}
