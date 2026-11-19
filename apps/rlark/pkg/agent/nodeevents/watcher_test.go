package nodeevents

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func newTestWatcher(t *testing.T, nodeName string) *Watcher {
	t.Helper()
	return &Watcher{
		log:      testr.New(t),
		nodeName: nodeName,
		events:   make(map[eventKey]*eventEntry),
	}
}

func mkEvent(involvedKind, involvedName, eventType, reason, message, source string, lastTS time.Time, count int32) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:              involvedName + "." + reason,
			CreationTimestamp: metav1.NewTime(lastTS.Add(-1 * time.Minute)),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: involvedKind,
			Name: involvedName,
		},
		Type:          eventType,
		Reason:        reason,
		Message:       message,
		LastTimestamp: metav1.NewTime(lastTS),
		Count:         count,
		Source:        corev1.EventSource{Component: source},
	}
}

func TestIsRelevantEvent(t *testing.T) {
	w := newTestWatcher(t, "node-1")

	cases := []struct {
		name string
		ev   *corev1.Event
		want bool
	}{
		// Only Warning-type events whose involvedObject is the local Node are
		// surfaced. Everything else is filtered to keep the signal clean.
		{"warning on local node", mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "kubelet disk full", "kubelet", time.Now(), 3), true},
		{"warning MemoryPressure on local node", mkEvent("Node", "node-1", corev1.EventTypeWarning, "MemoryPressure", "mem", "kubelet", time.Now(), 1), true},
		{"warning on a pod is filtered (Pod kind)", mkEvent("Pod", "pod-abc", corev1.EventTypeWarning, "FailedScheduling", "no nodes", "kube-scheduler", time.Now(), 1), false},
		{"warning BackOff on a pod is filtered (Pod kind)", mkEvent("Pod", "pod-abc", corev1.EventTypeWarning, "BackOff", "image pull back-off", "kubelet", time.Now(), 5), false},
		{"normal Pulling is filtered (Normal type)", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Pulling", "pulling image", "kubelet", time.Now(), 1), false},
		{"normal Scheduled is filtered (Normal type)", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Scheduled", "assigned node-1", "kube-scheduler", time.Now(), 1), false},
		{"normal Started is filtered (Normal type)", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Started", "started container", "kubelet", time.Now(), 1), false},
		{"normal Pulled is filtered (Normal type)", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Pulled", "image pulled", "kubelet", time.Now(), 1), false},
		{"normal Killing is filtered (Normal type)", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Killing", "deleting pod", "kubelet", time.Now(), 1), false},
		{"normal unrelated is filtered", mkEvent("Node", "node-1", corev1.EventTypeNormal, "Created", "container created", "kubelet", time.Now(), 1), false},
		{"warning on a different node is filtered (name mismatch)", mkEvent("Node", "node-2", corev1.EventTypeWarning, "DiskPressure", "other node disk", "kubelet", time.Now(), 1), false},
		{"unknown type is filtered", mkEvent("Node", "node-1", "Weird", "DiskPressure", "?", "kubelet", time.Now(), 1), false},
		{"empty type is filtered", mkEvent("Node", "node-1", "", "DiskPressure", "?", "kubelet", time.Now(), 1), false},
		{"nil event", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := w.isRelevantEvent(c.ev); got != c.want {
				t.Errorf("isRelevantEvent() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestUpsertEventDeduplicatesAndKeepsLatest(t *testing.T) {
	w := newTestWatcher(t, "node-1")

	t1 := time.Now()
	t2 := t1.Add(2 * time.Minute)

	// First observation: DiskPressure warning, count=1.
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "kubelet disk full", "kubelet", t1, 1))
	// Second observation: same key, later timestamp, higher count.
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "kubelet disk still full", "kubelet", t2, 3))

	if got := len(w.events); got != 1 {
		t.Fatalf("expected 1 entry, got %d", got)
	}
	entry := w.events[eventKey{kind: "Node", name: "node-1", reason: "DiskPressure"}]
	if entry == nil {
		t.Fatal("entry is nil")
	}
	if entry.ev.Count != 3 {
		t.Errorf("Count = %d, want 3 (latest wins)", entry.ev.Count)
	}
	if got := entry.ev.LastTime.Time; !got.Equal(t2) {
		t.Errorf("LastTime = %v, want %v", got, t2)
	}
	if entry.ev.Message != "kubelet disk still full" {
		t.Errorf("Message = %q, want latest message", entry.ev.Message)
	}
}

func TestUpsertEventFiltersNonNodeAndNormal(t *testing.T) {
	w := newTestWatcher(t, "node-1")

	now := time.Now()
	// Only the first (Node, node-1, Warning) survives. The rest are filtered
	// by either kind (Pod) or type (Normal).
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "disk", "kubelet", now, 1))
	w.upsertEvent(mkEvent("Pod", "pod-abc", corev1.EventTypeWarning, "FailedScheduling", "no nodes", "kube-scheduler", now, 2))
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeNormal, "Pulling", "pulling", "kubelet", now, 1))
	w.upsertEvent(mkEvent("Pod", "pod-abc", corev1.EventTypeNormal, "Created", "created", "kubelet", now, 1))

	if got := len(w.events); got != 1 {
		t.Fatalf("expected 1 entry (only DiskPressure on local Node), got %d", got)
	}
	if _, ok := w.events[eventKey{kind: "Node", name: "node-1", reason: "DiskPressure"}]; !ok {
		t.Error("expected DiskPressure entry to be stored")
	}
}

func TestUpsertEventDropsExpiredAtInsertion(t *testing.T) {
	w := newTestWatcher(t, "node-1")

	now := time.Now()
	recent := now.Add(-30 * time.Second)
	stale := now.Add(-2 * eventRetention) // 2h ago, beyond the 1h retention

	// Recent Node warning: stored.
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "recent", "kubelet", recent, 1))
	// Stale Node warning: dropped at insertion, never enters the cache.
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "MemoryPressure", "stale", "kubelet", stale, 1))

	if got := len(w.events); got != 1 {
		t.Fatalf("expected 1 entry after dropping stale event at insertion, got %d", got)
	}
	if _, ok := w.events[eventKey{kind: "Node", name: "node-1", reason: "DiskPressure"}]; !ok {
		t.Error("expected recent DiskPressure entry to be stored")
	}
	if _, ok := w.events[eventKey{kind: "Node", name: "node-1", reason: "MemoryPressure"}]; ok {
		t.Error("stale MemoryPressure event should not have been stored")
	}
}

func TestReportPrunesExpiredAndOrdersNewestFirst(t *testing.T) {
	w := newTestWatcher(t, "node-1")

	now := time.Now()
	recent := now.Add(-30 * time.Second)
	stale := now.Add(-2 * eventRetention)

	// MemoryPressure is inserted via the cache directly (bypass upsert's
	// insertion-time age check) so we can exercise the report-time pruning
	// path: report() must drop it even though it made it into the map.
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "DiskPressure", "recent disk", "kubelet", recent, 1))
	w.injectRaw(eventKey{kind: "Node", name: "node-1", reason: "MemoryPressure"},
		rlarkv1alpha1.NodeEvent{Type: "Warning", Reason: "MemoryPressure", Message: "stale memory", LastTime: metav1.NewTime(stale), ObjectKind: "Node", ObjectName: "node-1", Source: "kubelet"})
	w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "PIDPressure", "pid recent", "kubelet", recent.Add(-1*time.Second), 1))

	got := captureReport(t, w)

	if len(got) != 2 {
		t.Fatalf("expected 2 events after pruning stale MemoryPressure, got %d: %+v", len(got), got)
	}
	// Newest first: DiskPressure (30s ago) before PIDPressure (31s ago).
	if got[0].Reason != "DiskPressure" {
		t.Errorf("first event = %q, want DiskPressure", got[0].Reason)
	}
	if got[1].Reason != "PIDPressure" {
		t.Errorf("second event = %q, want PIDPressure", got[1].Reason)
	}
}

func TestReportCapsMaxEvents(t *testing.T) {
	w := newTestWatcher(t, "node-1")
	now := time.Now()
	// Insert maxEvents+5 distinct Node Warning events.
	for i := 0; i < maxEvents+5; i++ {
		w.upsertEvent(mkEvent("Node", "node-1", corev1.EventTypeWarning, "Reason"+string(rune('A'+i)), "msg", "kubelet", now.Add(-time.Duration(i)*time.Second), 1))
	}
	got := captureReport(t, w)
	if len(got) != maxEvents {
		t.Fatalf("expected report capped to %d events, got %d", maxEvents, len(got))
	}
}

// injectRaw bypasses the upsert filters so tests can exercise the report-time
// pruning path for entries that would otherwise be rejected at insertion.
func (w *Watcher) injectRaw(key eventKey, ev rlarkv1alpha1.NodeEvent) {
	w.mu.Lock()
	w.events[key] = &eventEntry{ev: ev}
	w.mu.Unlock()
}

// captureAdapter implements Patcher and stashes the last patch payload.
type captureAdapter struct {
	calls int
	last  []rlarkv1alpha1.NodeEvent
}

func (c *captureAdapter) PatchNodeEvents(ctx context.Context, nodeName, namespace string, events []rlarkv1alpha1.NodeEvent) error {
	c.calls++
	c.last = events
	return nil
}

// captureReport invokes the unexported report method with a fake patcher and
// returns the payload that would have been sent to Node.status.events.
func captureReport(t *testing.T, w *Watcher) []rlarkv1alpha1.NodeEvent {
	t.Helper()
	adapter := &captureAdapter{}
	w.patcher = adapter
	w.report(context.Background())
	if adapter.calls != 1 {
		t.Fatalf("expected 1 patch call, got %d", adapter.calls)
	}
	return adapter.last
}
