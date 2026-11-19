// Package nodeevents implements node-level Kubernetes Event monitoring for the
// node-agent.
//
// The Watcher subscribes to local Kubernetes Events involving the local Node
// (Node-kind, name == local node) and filters them down to Warning entries
// (DiskPressure, MemoryPressure, PIDPressure, FailedMount, …). Normal-type
// events and Pod-scope events are intentionally NOT surfaced, to keep the
// signal clean for operators diagnosing why a Task is stuck Pending. A
// sliding 1h window is written to Node.status.events on the management
// cluster via a Patcher.
//
// Events are best-effort and never block node-agent startup or the more
// critical pull-progress reporting path.
package nodeevents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	// reportInterval is how often the watcher recomputes the relevant event
	// set and patches Node.status.events. It bounds the staleness of the
	// events list seen by the control-plane Task reconciler.
	reportInterval = 5 * time.Second
	// eventRetention caps how long an event remains in Node.status.events
	// after its LastTimestamp. Kubernetes Events already have a kubelet-managed
	// TTL (default 1h); this is the upper bound we advertise to the management
	// plane. Entries older than this are dropped both at insertion time (so a
	// stale informer backlog never pollutes the cache) and at report time
	// (so the field self-cleans even between reconciler runs).
	eventRetention = 1 * time.Hour
	// maxEvents caps the events slice to avoid unbounded growth on a noisy
	// node. When the limit is exceeded, oldest entries are dropped first.
	maxEvents = 50
)

// Patcher patches the management Node status with the observed events list.
// Implementations MUST target the status subresource: nodes.rlinf.io enables
// subresources.status, so a main-resource Patch on {"status":...} would have
// its status field silently stripped by the API server.
type Patcher interface {
	PatchNodeEvents(ctx context.Context, nodeName, namespace string, events []rlarkv1alpha1.NodeEvent) error
}

// Watcher watches local Kubernetes Events involving the local Node and
// periodically reports a sliding window of Warning entries to
// Node.status.events on the management cluster.
type Watcher struct {
	log       logr.Logger
	nodeName  string
	mgmtNS    string
	patcher   Patcher
	localKube kubernetes.Interface

	mu     sync.Mutex
	events map[eventKey]*eventEntry
}

type eventKey struct {
	kind, name, reason string
}

type eventEntry struct {
	ev rlarkv1alpha1.NodeEvent
}

// Config configures a Watcher.
type Config struct {
	// NodeName is the local Kubernetes Node name. Only Events whose
	// involvedObject is this Node (kind=Node, name=NodeName) are surfaced.
	NodeName string
	// Namespace is the management namespace where the Node CR lives.
	Namespace string
	// LocalKube is the local Kubernetes client used to list/watch Events.
	LocalKube kubernetes.Interface
	// Patcher patches Node.status.events on the management cluster.
	// If nil, reporting is disabled (events are only logged locally).
	Patcher Patcher
}

// NewWatcher constructs a Watcher. The returned Watcher is not started; callers
// must call Run.
func NewWatcher(cfg Config, log logr.Logger) *Watcher {
	return &Watcher{
		log:       log.WithName("nodeEvents"),
		nodeName:  cfg.NodeName,
		mgmtNS:    cfg.Namespace,
		patcher:   cfg.Patcher,
		localKube: cfg.LocalKube,
		events:    make(map[eventKey]*eventEntry),
	}
}

// Run starts the watcher. It blocks until ctx is cancelled.
//
// The watcher uses a field-selector-scoped Events informer
// (involvedObject.kind=Node, involvedObject.name=<nodeName>) so the agent only
// ever sees Events for its own node, even on a busy control plane. An
// additional in-process filter (isRelevantEvent) re-checks the kind/name pair
// and the event type, so a misconfigured field selector or a future
// multi-node path cannot leak unrelated events into Node.status.events.
func (w *Watcher) Run(ctx context.Context) error {
	if w.nodeName == "" || w.localKube == nil {
		w.log.Info("node events watcher disabled: node name or local kube client missing")
		<-ctx.Done()
		return ctx.Err()
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		w.localKube,
		30*time.Minute,
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			// Limit to events involving the local Node directly. Pod-scope
			// events (FailedScheduling, ImagePullBackOff, etc.) are out of
			// scope by design: they would mix unrelated pods' warnings into
			// the node's event list.
			opts.FieldSelector = fmt.Sprintf("involvedObject.kind=Node,involvedObject.name=%s", w.nodeName)
		}),
	)
	eventInformer := factory.Core().V1().Events().Informer()

	_, _ = eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onEvent,
		UpdateFunc: w.onEventUpdate,
		DeleteFunc: w.onEventDelete,
	})

	factory.Start(ctx.Done())

	if err := waitForCacheSync(ctx, []cache.SharedIndexInformer{eventInformer}); err != nil {
		return err
	}
	w.log.Info("node events watcher started", "nodeName", w.nodeName)

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("node events watcher stopped")
			return ctx.Err()
		case <-ticker.C:
			w.report(ctx)
		}
	}
}

// onEvent handles an informer Add for an Event.
func (w *Watcher) onEvent(obj interface{}) {
	ev, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	w.upsertEvent(ev)
}

// onEventUpdate handles an informer Update. Count/LastTimestamp may change
// without the event being re-added; refresh the cached entry.
func (w *Watcher) onEventUpdate(_, newObj interface{}) {
	ev, ok := newObj.(*corev1.Event)
	if !ok {
		return
	}
	w.upsertEvent(ev)
}

// onEventDelete handles an informer Delete. We do NOT remove the entry on
// delete: kubelet can re-create an Event with the same name+reason after a
// short pause, and the in-memory map is bounded by eventRetention anyway.
func (w *Watcher) onEventDelete(obj interface{}) {}

// upsertEvent records an Event if it passes all filters:
//   - non-nil,
//   - Type == Warning,
//   - involvedObject.Kind == "Node" and involvedObject.Name == local node,
//   - LastTimestamp within eventRetention (older events are dropped, since a
//     stale informer backlog could otherwise repopulate long-resolved
//     warnings).
func (w *Watcher) upsertEvent(ev *corev1.Event) {
	if !w.isRelevantEvent(ev) {
		return
	}
	if !ev.LastTimestamp.IsZero() && time.Since(ev.LastTimestamp.Time) > eventRetention {
		return
	}
	entry := eventEntry{
		ev: rlarkv1alpha1.NodeEvent{
			Type:       ev.Type,
			Reason:     ev.Reason,
			Message:    ev.Message,
			LastTime:   ev.LastTimestamp,
			Count:      ev.Count,
			Source:     eventSource(ev),
			ObjectKind: ev.InvolvedObject.Kind,
			ObjectName: ev.InvolvedObject.Name,
		},
	}
	key := eventKey{kind: ev.InvolvedObject.Kind, name: ev.InvolvedObject.Name, reason: ev.Reason}

	w.mu.Lock()
	w.events[key] = &entry
	w.mu.Unlock()
}

// isRelevantEvent reports whether the Event should be surfaced to the
// operator. The filter is intentionally strict:
//
//   - Type MUST be Warning. Normal-type events (Pulling, Pulled, Scheduled,
//     Started, Killing, Created, …) are NOT surfaced, because they describe
//     routine kubelet activity and would drown out the warnings that
//     indicate real problems (DiskPressure, MemoryPressure, PIDPressure,
//     FailedMount, BackOff, Evicted, …).
//   - involvedObject.Kind MUST be "Node" and involvedObject.Name MUST match
//     the local node name. This re-checks the informer field selector so a
//     future code path that bypasses the selector cannot leak another
//     node's events.
func (w *Watcher) isRelevantEvent(ev *corev1.Event) bool {
	if ev == nil {
		return false
	}
	if ev.Type != corev1.EventTypeWarning {
		return false
	}
	return ev.InvolvedObject.Kind == "Node" && ev.InvolvedObject.Name == w.nodeName
}

// eventSource collapses the Event source component to a single string for
// compact display (e.g. "kubelet", "kube-scheduler").
func eventSource(ev *corev1.Event) string {
	if ev.Source.Component != "" {
		return ev.Source.Component
	}
	if ev.Source.Host != "" {
		return ev.Source.Host
	}
	return ""
}

// report computes the current sliding-window snapshot of events and patches
// the management Node.status.events. Expired entries (older than
// eventRetention) are pruned before the patch is sent so the field self-cleans
// even between reconciler runs.
func (w *Watcher) report(ctx context.Context) {
	w.mu.Lock()
	now := time.Now()
	for key, entry := range w.events {
		last := entry.ev.LastTime.Time
		if last.IsZero() || now.Sub(last) > eventRetention {
			delete(w.events, key)
		}
	}
	snapshot := make([]rlarkv1alpha1.NodeEvent, 0, len(w.events))
	for _, entry := range w.events {
		snapshot = append(snapshot, entry.ev)
	}
	w.mu.Unlock()

	sort.Slice(snapshot, func(i, j int) bool {
		// Newest first by LastTime, fall back to Reason for stable ordering.
		ti := snapshot[i].LastTime.Time
		tj := snapshot[j].LastTime.Time
		if ti.Equal(tj) {
			return snapshot[i].Reason < snapshot[j].Reason
		}
		return ti.After(tj)
	})
	if len(snapshot) > maxEvents {
		snapshot = snapshot[:maxEvents]
	}

	if w.patcher == nil {
		return
	}
	if err := w.patcher.PatchNodeEvents(ctx, w.nodeName, w.mgmtNS, snapshot); err != nil {
		w.log.V(1).Info("failed to patch node events", "err", err.Error())
	}
}

// waitForCacheSync blocks until all informers have synced or ctx is cancelled.
func waitForCacheSync(ctx context.Context, informers []cache.SharedIndexInformer) error {
	for {
		allSynced := true
		for _, inf := range informers {
			if !inf.HasSynced() {
				allSynced = false
				break
			}
		}
		if allSynced {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// BuildEventsMergePatch creates a JSON merge patch body for the events field
// on the status subresource. The "status" wrapper is required: the status
// subresource endpoint applies the merge patch to the whole object and only
// permits .status modifications. Callers must pass "status" as the subresource.
func BuildEventsMergePatch(events []rlarkv1alpha1.NodeEvent) ([]byte, error) {
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"events": events,
		},
	}
	return json.Marshal(patch)
}
