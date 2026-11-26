package task

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestIndexTaskByObservedNodesDeduplicatesAndSkipsEmpty(t *testing.T) {
	task := mkPullTask([]string{"node-b", "", "node-a", "node-b"}, "img:a")

	keys := indexTaskByObservedNodes(task)
	if len(keys) != 2 {
		t.Fatalf("expected 2 distinct index keys, got %v", keys)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" {
			t.Errorf("empty node name must not be indexed")
		}
		if seen[key] {
			t.Errorf("duplicate index key %q", key)
		}
		seen[key] = true
	}
	if !seen["node-a"] || !seen["node-b"] {
		t.Errorf("expected node-a and node-b to be indexed, got %v", keys)
	}
}

func TestIndexTaskByObservedNodesReturnsNilWhenNothingObserved(t *testing.T) {
	if got := indexTaskByObservedNodes(mkPullTask(nil, "img:a")); got != nil {
		t.Errorf("expected nil for a Task observing no node, got %v", got)
	}
	if got := indexTaskByObservedNodes(mkPullTask([]string{"", ""}, "img:a")); got != nil {
		t.Errorf("expected nil when every observed name is empty, got %v", got)
	}
}

// The indexer is registered for Tasks only, but a wrong type must not panic:
// returning no key simply leaves the object out of the index.
func TestIndexTaskByObservedNodesIgnoresOtherTypes(t *testing.T) {
	if got := indexTaskByObservedNodes(mkNode("node-a", "rlark-system", nil)); got != nil {
		t.Errorf("expected nil for a non-Task object, got %v", got)
	}
}

func TestNodeReportChangedPredicateIgnoresUnrelatedUpdates(t *testing.T) {
	progress := []rlarkv1alpha1.PullProgress{{Image: "img:a", Status: "Pulling"}}
	oldNode := mkPullNode("node-a", "rlark-system", progress)
	newNode := oldNode.DeepCopy()
	// A heartbeat-style update that touches neither aggregated field.
	heartbeat := metav1.Now()
	newNode.Status.LastHeartbeatTime = &heartbeat
	newNode.ResourceVersion = "2"

	if nodeReportChangedPredicate().Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}) {
		t.Errorf("expected an update that changes neither pullProgress nor events to be filtered out")
	}
}

func TestNodeReportChangedPredicateAcceptsAggregatedFieldChanges(t *testing.T) {
	base := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling"},
	})

	progressChanged := base.DeepCopy()
	progressChanged.Status.PullProgress[0].Status = "Pulled"
	if !nodeReportChangedPredicate().Update(event.UpdateEvent{ObjectOld: base, ObjectNew: progressChanged}) {
		t.Errorf("expected a pullProgress change to be accepted")
	}

	eventsChanged := base.DeepCopy()
	eventsChanged.Status.Events = []rlarkv1alpha1.NodeEvent{
		mkEvent("Warning", "Failed", "boom", "kubelet", "Pod", "p1", metav1.Now().Time, 1),
	}
	if !nodeReportChangedPredicate().Update(event.UpdateEvent{ObjectOld: base, ObjectNew: eventsChanged}) {
		t.Errorf("expected a node-events change to be accepted")
	}
}

// An unexpected type carries no information about what changed, so the
// predicate must fall back to enqueueing and let the reconciler decide from
// current state rather than silently dropping a wakeup.
func TestNodeReportChangedPredicateFallsBackOnUnexpectedTypes(t *testing.T) {
	p := nodeReportChangedPredicate()
	notANode := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "rlark-system"}}

	if !p.Update(event.UpdateEvent{ObjectOld: notANode, ObjectNew: notANode}) {
		t.Errorf("expected a non-Node update to be accepted conservatively")
	}
	if !p.Create(event.CreateEvent{Object: notANode}) {
		t.Errorf("expected create events to be accepted")
	}
	if !p.Delete(event.DeleteEvent{Object: notANode}) {
		t.Errorf("expected delete events to be accepted")
	}
	if !p.Generic(event.GenericEvent{Object: notANode}) {
		t.Errorf("expected generic events to be accepted")
	}
}

func TestMapNodeToTasksEnqueuesOnlyPendingObservers(t *testing.T) {
	pending := mkPullTask([]string{"node-a"}, "img:a")

	running := mkPullTask([]string{"node-a"}, "img:a")
	running.Name = "t-running"
	running.Status.Phase = rlarkv1alpha1.TaskPhaseRunning

	otherNode := mkPullTask([]string{"node-b"}, "img:a")
	otherNode.Name = "t-other-node"

	otherNamespace := mkPullTask([]string{"node-a"}, "img:a")
	otherNamespace.Name = "t-other-ns"
	otherNamespace.Namespace = "other"

	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(pending, running, otherNode, otherNamespace).
		WithIndex(&rlarkv1alpha1.Task{}, taskObservedNodeField, indexTaskByObservedNodes).
		Build()
	r := &Reconciler{Client: c}

	requests := r.mapNodeToTasks(context.Background(), mkNode("node-a", "rlark-system", nil))
	if len(requests) != 1 {
		t.Fatalf("expected exactly the Pending observer in the same namespace, got %v", requests)
	}
	if requests[0].Name != pending.Name || requests[0].Namespace != pending.Namespace {
		t.Errorf("expected request for %s/%s, got %v", pending.Namespace, pending.Name, requests[0])
	}
}

func TestMapNodeToTasksIgnoresOtherTypes(t *testing.T) {
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithIndex(&rlarkv1alpha1.Task{}, taskObservedNodeField, indexTaskByObservedNodes).
		Build()
	r := &Reconciler{Client: c}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "rlark-system"}}
	if got := r.mapNodeToTasks(context.Background(), pod); got != nil {
		t.Errorf("expected nil for a non-Node object, got %v", got)
	}
}
