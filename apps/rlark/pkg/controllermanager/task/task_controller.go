package task

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// pullProgressRequeueInterval is how often the reconciler re-reconciles a
// Task while it is Pending, so that Task.status.pullProgress and
// Task.status.events stay fresh without depending on Node status events
// (Node.status.pullProgress/events are updated by data-plane node-agents,
// but those updates do not trigger a Task reconcile; requeuing gives a
// deterministic upper bound on staleness).
const pullProgressRequeueInterval = 3 * time.Second

// maxAggregatedEvents caps how many Node events are surfaced on a single
// Task. ObservedNodes is usually small (1-8 nodes) and each node caps its
// own events slice at 50 (see nodeevents.maxEvents); this guard prevents a
// pathological noisy node from drowning out the rest of the Task status.
const maxAggregatedEvents = 100

// Reconciler reconciles Task resources.
//
// It owns two slices of Task.status while the Task is Pending:
//   - pullProgress: aggregated from Node.status.pullProgress entries reported
//     by data-plane node-agents, filtered to images referenced by the Task spec.
//   - events: aggregated from Node.status.events entries reported by data-plane
//     node-agents (DiskPressure warnings, FailedScheduling, image-pull
//     BackOff, etc.) so operators can see "why is my pod still Pending".
//
// Other status fields (phase, message, observedNodes) are written by the
// data-plane cluster-agent's push reconcilers; this controller must not touch
// them, and uses a strategic merge patch (client.MergeFrom) so concurrent
// writes by the agent are preserved.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch

// Reconcile aggregates image pull progress and node events for a Task.
//
// While the Task is Pending, it collects:
//   - Node.status.pullProgress entries from every node in
//     Task.status.observedNodes, filtered to images referenced by the Task
//     spec, and writes the aggregated slice to Task.status.pullProgress.
//   - Node.status.events entries from the same observed nodes and writes the
//     aggregated slice to Task.status.events so operators can inspect
//     DiskPressure/FailedScheduling/image-pull warnings without leaving the
//     Task view.
//
// When the Task leaves Pending, both fields are cleared. The reconciler
// requeues itself every pullProgressRequeueInterval while Pending so the
// aggregated values stay fresh.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName)

	var task rlarkv1alpha1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only aggregate while Pending. Once the task leaves Pending, any
	// previously-reported pullProgress/events are stale and must be cleared.
	var aggregatedProgress []rlarkv1alpha1.PullProgress
	var aggregatedEvents []rlarkv1alpha1.NodeEvent
	if task.Status.Phase == rlarkv1alpha1.TaskPhasePending {
		aggregatedProgress = r.aggregatePullProgress(ctx, &task)
		aggregatedEvents = r.aggregateNodeEvents(ctx, &task)
	}

	if pullProgressEqual(task.Status.PullProgress, aggregatedProgress) &&
		nodeEventsEqual(task.Status.Events, aggregatedEvents) {
		// No status change. Keep polling while Pending so progress and
		// events stay fresh even when no Task event fires (Node.status
		// updates do not trigger this reconciler).
		if task.Status.Phase == rlarkv1alpha1.TaskPhasePending {
			return ctrl.Result{RequeueAfter: pullProgressRequeueInterval}, nil
		}
		return ctrl.Result{}, nil
	}

	original := task.DeepCopy()
	task.Status.PullProgress = aggregatedProgress
	task.Status.Events = aggregatedEvents
	// Use a merge patch so we only touch pullProgress and events. Other
	// status fields (phase, message, observedNodes) are owned by the
	// data-plane cluster-agent's push reconcilers and must not be clobbered.
	if err := r.Status().Patch(ctx, &task, client.MergeFrom(original)); err != nil {
		logger.Error(err, "failed to patch Task pullProgress/events")
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("updated Task status: pullProgress=%d events=%d phase=%s",
		len(aggregatedProgress), len(aggregatedEvents), task.Status.Phase))
	if task.Status.Phase == rlarkv1alpha1.TaskPhasePending {
		return ctrl.Result{RequeueAfter: pullProgressRequeueInterval}, nil
	}
	return ctrl.Result{}, nil
}

// aggregatePullProgress collects Node.status.pullProgress entries from every
// node listed in task.Status.ObservedNodes, filtered to images referenced by
// the task spec. Returns nil when no in-flight pulls are observed.
func (r *Reconciler) aggregatePullProgress(ctx context.Context, task *rlarkv1alpha1.Task) []rlarkv1alpha1.PullProgress {
	if len(task.Status.ObservedNodes) == 0 {
		return nil
	}

	images := make(map[string]struct{})
	for _, img := range utils.ExtractTaskImages(task) {
		images[img] = struct{}{}
	}
	if len(images) == 0 {
		return nil
	}

	// Deduplicate node names: ObservedNodes may list a node multiple times
	// (e.g. multi-replica workloads scheduling several pods per node).
	seen := make(map[string]struct{}, len(task.Status.ObservedNodes))
	for _, n := range task.Status.ObservedNodes {
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
	}

	var aggregated []rlarkv1alpha1.PullProgress
	for nodeName := range seen {
		var node rlarkv1alpha1.Node
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName, Namespace: task.Namespace}, &node); err != nil {
			continue
		}
		for _, p := range node.Status.PullProgress {
			if _, ok := images[p.Image]; !ok {
				continue
			}
			aggregated = append(aggregated, p)
		}
	}
	return aggregated
}

// pullProgressEqual reports whether two pull-progress slices are equal.
// Ordering matters for stable comparison.
func pullProgressEqual(a, b []rlarkv1alpha1.PullProgress) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// aggregateNodeEvents collects Node.status.events entries from every node
// listed in task.Status.ObservedNodes. Events are deduplicated by
// (objectKind, objectName, reason): when multiple nodes report the same
// event (e.g. a Node-kind DiskPressure event mirrored on several observed
// nodes), the entry with the latest LastTime wins. Results are sorted
// newest-first and capped to maxAggregatedEvents.
//
// Returns nil when no events are observed, so the field is cleared on the
// next reconcile (rather than persisting a stale non-nil slice).
func (r *Reconciler) aggregateNodeEvents(ctx context.Context, task *rlarkv1alpha1.Task) []rlarkv1alpha1.NodeEvent {
	if len(task.Status.ObservedNodes) == 0 {
		return nil
	}

	// Deduplicate node names: ObservedNodes may list a node multiple times
	// (e.g. multi-replica workloads scheduling several pods per node).
	seen := make(map[string]struct{}, len(task.Status.ObservedNodes))
	for _, n := range task.Status.ObservedNodes {
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
	}

	// Merge by (kind, name, reason); keep the entry with the latest LastTime
	// so a re-observed event refreshes rather than duplicates.
	type dedupKey struct {
		kind, name, reason string
	}
	merged := make(map[dedupKey]rlarkv1alpha1.NodeEvent)
	for nodeName := range seen {
		var node rlarkv1alpha1.Node
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName, Namespace: task.Namespace}, &node); err != nil {
			continue
		}
		for i := range node.Status.Events {
			ev := node.Status.Events[i]
			key := dedupKey{kind: ev.ObjectKind, name: ev.ObjectName, reason: ev.Reason}
			if existing, ok := merged[key]; ok {
				if !ev.LastTime.IsZero() && (existing.LastTime.IsZero() || ev.LastTime.After(existing.LastTime.Time)) {
					merged[key] = ev
				}
				continue
			}
			merged[key] = ev
		}
	}
	if len(merged) == 0 {
		return nil
	}

	out := make([]rlarkv1alpha1.NodeEvent, 0, len(merged))
	for _, ev := range merged {
		out = append(out, ev)
	}
	// Sort newest-first by LastTime; fall back to Reason for stable ordering
	// when timestamps tie (or are zero).
	sort.Slice(out, func(i, j int) bool {
		ti := out[i].LastTime.Time
		tj := out[j].LastTime.Time
		if ti.Equal(tj) {
			return out[i].Reason < out[j].Reason
		}
		return ti.After(tj)
	})
	if len(out) > maxAggregatedEvents {
		out = out[:maxAggregatedEvents]
	}
	return out
}

// nodeEventsEqual reports whether two node-events slices are equal. Ordering
// matters for stable comparison (slices are sorted newest-first by the
// aggregator, so equal input produces equal output).
func nodeEventsEqual(a, b []rlarkv1alpha1.NodeEvent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].LastTime.Time.Equal(b[i].LastTime.Time) {
			return false
		}
		if a[i].Type != b[i].Type || a[i].Reason != b[i].Reason ||
			a[i].Message != b[i].Message || a[i].Source != b[i].Source ||
			a[i].ObjectKind != b[i].ObjectKind || a[i].ObjectName != b[i].ObjectName ||
			a[i].Count != b[i].Count {
			return false
		}
	}
	return true
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Task{}).
		Named("task").
		Complete(r)
}
