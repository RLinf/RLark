package task

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add rlark scheme: %v", err)
	}
	return scheme
}

func mkNode(name, namespace string, events []rlarkv1alpha1.NodeEvent) *rlarkv1alpha1.Node {
	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     rlarkv1alpha1.NodeStatus{Events: events},
	}
}

func mkEvent(typ, reason, message, source, kind, name string, last time.Time, count int32) rlarkv1alpha1.NodeEvent {
	return rlarkv1alpha1.NodeEvent{
		Type: typ, Reason: reason, Message: message, Source: source,
		ObjectKind: kind, ObjectName: name,
		LastTime: metav1.NewTime(last), Count: count,
	}
}

func mkPullNode(name, namespace string, progress []rlarkv1alpha1.PullProgress) *rlarkv1alpha1.Node {
	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     rlarkv1alpha1.NodeStatus{PullProgress: progress},
	}
}

// mkPullTask builds a Pending Task whose common workload template references
// the given images, so aggregatePullProgress has something to filter against.
func mkPullTask(observedNodes []string, images ...string) *rlarkv1alpha1.Task {
	containers := make([]corev1.Container, 0, len(images))
	for i, img := range images {
		containers = append(containers, corev1.Container{Name: "c" + string(rune('0'+i)), Image: img})
	}
	return &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"},
		Spec: rlarkv1alpha1.TaskSpec{
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: containers},
				},
			},
		},
		Status: rlarkv1alpha1.TaskStatus{
			Phase:         rlarkv1alpha1.TaskPhasePending,
			ObservedNodes: observedNodes,
		},
	}
}

func TestAggregatePullProgressEmptyObservedReturnsNil(t *testing.T) {
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()}

	if got := r.aggregatePullProgress(context.Background(), mkPullTask(nil, "img:a")); got != nil {
		t.Errorf("expected nil for empty observedNodes, got %v", got)
	}
}

func TestAggregatePullProgressNoSpecImagesReturnsNil(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling"},
	})
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()}

	// A Task with no container images cannot attribute any pull to itself.
	task := mkPullTask([]string{"node-a"})
	if got := r.aggregatePullProgress(context.Background(), task); got != nil {
		t.Errorf("expected nil when the spec references no images, got %v", got)
	}
}

// Node names may repeat in ObservedNodes and the map iteration order is
// random, so the aggregation must deduplicate nodes and produce a stable
// (image, node) order regardless of how ObservedNodes is written.
func TestAggregatePullProgressDeduplicatesNodesAndSortsStably(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:b", Status: "Pulling", Message: "a/b"},
		{Image: "img:a", Status: "Pulling", Message: "a/a"},
	})
	nodeB := mkPullNode("node-b", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling", Message: "b/a"},
	})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA, nodeB).Build()
	r := &Reconciler{Client: c}

	// node-b listed first and node-a listed twice: the result must still be
	// ordered by image, then by node.
	task := mkPullTask([]string{"node-b", "node-a", "node-b"}, "img:a", "img:b")

	got := r.aggregatePullProgress(context.Background(), task)
	want := []string{"a/a", "b/a", "a/b"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Message != w {
			t.Errorf("entry %d message = %q, want %q", i, got[i].Message, w)
		}
	}

	// Re-aggregating the same observed state must be byte-identical,
	// otherwise the pullProgressEqual guard never holds and the controller
	// rewrites status forever.
	again := r.aggregatePullProgress(context.Background(), task)
	if !pullProgressEqual(got, again) {
		t.Errorf("aggregation is not stable across calls: %+v vs %+v", got, again)
	}
}

func TestAggregatePullProgressFiltersUnreferencedImages(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling"},
		{Image: "other:1", Status: "Pulling"},
	})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	got := r.aggregatePullProgress(context.Background(), mkPullTask([]string{"node-a"}, "img:a"))
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after filtering, got %d: %+v", len(got), got)
	}
	if got[0].Image != "img:a" {
		t.Errorf("image = %q, want img:a", got[0].Image)
	}
}

func TestAggregatePullProgressMissingNodeIsSkipped(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling"},
	})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	// node-b has not reported yet; the aggregation must skip it.
	got := r.aggregatePullProgress(context.Background(), mkPullTask([]string{"node-a", "node-b"}, "img:a"))
	if len(got) != 1 {
		t.Fatalf("expected 1 entry (node-b skipped), got %d", len(got))
	}
}

func TestAggregatePullProgressCapsMaxAggregated(t *testing.T) {
	scheme := newTestScheme(t)

	progress := make([]rlarkv1alpha1.PullProgress, 0, maxAggregatedPullProgress+5)
	for i := 0; i < maxAggregatedPullProgress+5; i++ {
		progress = append(progress, rlarkv1alpha1.PullProgress{Image: "img:a", Status: "Pulling", Downloaded: int64(i)})
	}
	nodeA := mkPullNode("node-a", "rlark-system", progress)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	got := r.aggregatePullProgress(context.Background(), mkPullTask([]string{"node-a"}, "img:a"))
	if len(got) != maxAggregatedPullProgress {
		t.Fatalf("expected %d entries (capped), got %d", maxAggregatedPullProgress, len(got))
	}
}

func TestAggregateNodeEventsEmptyObservedReturnsNil(t *testing.T) {
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"}}

	if got := r.aggregateNodeEvents(context.Background(), task); got != nil {
		t.Errorf("expected nil for empty observedNodes, got %v", got)
	}
}

func TestAggregateNodeEventsDeduplicatesAcrossNodes(t *testing.T) {
	scheme := newTestScheme(t)
	now := time.Now()

	// node-a and node-b both report a Node-kind DiskPressure event; the
	// entry with the latest LastTime must win.
	nodeA := mkNode("node-a", "rlark-system", []rlarkv1alpha1.NodeEvent{
		mkEvent("Warning", "DiskPressure", "disk full", "kubelet", "Node", "node-a", now.Add(-2*time.Minute), 1),
		mkEvent("Warning", "FailedScheduling", "no nodes", "kube-scheduler", "Pod", "pod-a", now.Add(-1*time.Minute), 2),
	})
	nodeB := mkNode("node-b", "rlark-system", []rlarkv1alpha1.NodeEvent{
		mkEvent("Warning", "DiskPressure", "disk still full", "kubelet", "Node", "node-a", now.Add(-30*time.Second), 4),
	})

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA, nodeB).Build()
	r := &Reconciler{Client: c}

	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"},
		Status:     rlarkv1alpha1.TaskStatus{ObservedNodes: []string{"node-a", "node-b", "node-a"}},
	}

	got := r.aggregateNodeEvents(context.Background(), task)
	if len(got) != 2 {
		t.Fatalf("expected 2 events after dedup, got %d: %+v", len(got), got)
	}

	// Newest first: DiskPressure (30s ago) before FailedScheduling (1m ago).
	if got[0].Reason != "DiskPressure" {
		t.Errorf("first event reason = %q, want DiskPressure", got[0].Reason)
	}
	if got[0].Count != 4 {
		t.Errorf("deduped DiskPressure Count = %d, want 4 (latest wins)", got[0].Count)
	}
	if got[0].Message != "disk still full" {
		t.Errorf("deduped DiskPressure Message = %q, want latest message", got[0].Message)
	}
	if got[1].Reason != "FailedScheduling" {
		t.Errorf("second event reason = %q, want FailedScheduling", got[1].Reason)
	}
}

func TestAggregateNodeEventsMissingNodeIsSkipped(t *testing.T) {
	scheme := newTestScheme(t)
	now := time.Now()

	nodeA := mkNode("node-a", "rlark-system", []rlarkv1alpha1.NodeEvent{
		mkEvent("Warning", "DiskPressure", "disk", "kubelet", "Node", "node-a", now, 1),
	})

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	// node-b is in ObservedNodes but not in the client (e.g. the node-agent
	// has not yet reported); aggregator must skip it rather than fail.
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"},
		Status:     rlarkv1alpha1.TaskStatus{ObservedNodes: []string{"node-a", "node-b"}},
	}

	got := r.aggregateNodeEvents(context.Background(), task)
	if len(got) != 1 {
		t.Fatalf("expected 1 event (node-b skipped), got %d", len(got))
	}
	if got[0].Reason != "DiskPressure" {
		t.Errorf("reason = %q, want DiskPressure", got[0].Reason)
	}
}

func TestAggregateNodeEventsReturnsNilWhenNoEvents(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := mkNode("node-a", "rlark-system", nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"},
		Status:     rlarkv1alpha1.TaskStatus{ObservedNodes: []string{"node-a"}},
	}

	if got := r.aggregateNodeEvents(context.Background(), task); got != nil {
		t.Errorf("expected nil when no events on observed nodes, got %v", got)
	}
}

func TestAggregateNodeEventsCapsMaxAggregated(t *testing.T) {
	scheme := newTestScheme(t)
	now := time.Now()

	events := make([]rlarkv1alpha1.NodeEvent, 0, maxAggregatedEvents+5)
	for i := 0; i < maxAggregatedEvents+5; i++ {
		events = append(events, mkEvent("Warning", "Reason"+string(rune('A'+i)), "m", "kubelet", "Node", "node-a", now.Add(-time.Duration(i)*time.Second), 1))
	}
	nodeA := mkNode("node-a", "rlark-system", events)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeA).Build()
	r := &Reconciler{Client: c}

	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "rlark-system"},
		Status:     rlarkv1alpha1.TaskStatus{ObservedNodes: []string{"node-a"}},
	}

	got := r.aggregateNodeEvents(context.Background(), task)
	if len(got) != maxAggregatedEvents {
		t.Fatalf("expected %d events (capped), got %d", maxAggregatedEvents, len(got))
	}
}

func TestNodeEventsEqual(t *testing.T) {
	now := time.Now()
	a := []rlarkv1alpha1.NodeEvent{
		mkEvent("Warning", "DiskPressure", "m1", "kubelet", "Node", "n1", now, 1),
		mkEvent("Warning", "FailedScheduling", "m2", "kube-scheduler", "Pod", "p1", now.Add(-1*time.Second), 2),
	}

	cases := []struct {
		name string
		b    []rlarkv1alpha1.NodeEvent
		want bool
	}{
		{"identical", append([]rlarkv1alpha1.NodeEvent(nil), a...), true},
		{"different length", []rlarkv1alpha1.NodeEvent{a[0]}, false},
		{"different reason", []rlarkv1alpha1.NodeEvent{
			mkEvent("Warning", "MemoryPressure", "m1", "kubelet", "Node", "n1", now, 1),
			a[1],
		}, false},
		{"different count", []rlarkv1alpha1.NodeEvent{
			mkEvent("Warning", "DiskPressure", "m1", "kubelet", "Node", "n1", now, 9),
			a[1],
		}, false},
		{"different LastTime", []rlarkv1alpha1.NodeEvent{
			mkEvent("Warning", "DiskPressure", "m1", "kubelet", "Node", "n1", now.Add(1*time.Second), 1),
			a[1],
		}, false},
		{"same LastTime, different order", []rlarkv1alpha1.NodeEvent{a[1], a[0]}, false},
		{"nil b vs non-nil a", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nodeEventsEqual(a, c.b); got != c.want {
				t.Errorf("nodeEventsEqual() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNodeEventsEqualEmptySlicesAreEqual(t *testing.T) {
	if !nodeEventsEqual(nil, nil) {
		t.Error("nil vs nil should be equal")
	}
	if !nodeEventsEqual([]rlarkv1alpha1.NodeEvent{}, []rlarkv1alpha1.NodeEvent{}) {
		t.Error("empty vs empty should be equal")
	}
	if nodeEventsEqual(nil, []rlarkv1alpha1.NodeEvent{{Reason: "x"}}) {
		t.Error("nil vs one-entry should not be equal")
	}
}
