package task

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// taskObservedNodeField indexes every Task by each distinct node name in
// Task.status.observedNodes. A Node report can then be mapped back to exactly
// the Tasks whose aggregated status depends on that Node, instead of listing
// every Task in the cluster on every Node update.
const taskObservedNodeField = "status.observedNodes"

// indexTaskByObservedNodes extracts the deduplicated, non-empty node names a
// Task currently observes. ObservedNodes may repeat a node (multi-replica
// workloads can schedule several pods onto one node), and a duplicated index
// key would make the mapper emit duplicate requests for the same Task.
func indexTaskByObservedNodes(obj client.Object) []string {
	task, ok := obj.(*rlarkv1alpha1.Task)
	if !ok {
		return nil
	}
	seen := make(map[string]struct{}, len(task.Status.ObservedNodes))
	nodes := make([]string, 0, len(task.Status.ObservedNodes))
	for _, name := range task.Status.ObservedNodes {
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		nodes = append(nodes, name)
	}
	if len(nodes) == 0 {
		return nil
	}
	return nodes
}

// nodeReportChangedPredicate keeps Node updates out of the Task queue unless
// one of the two fields this controller aggregates actually changed.
//
// Node objects also carry heartbeat, capacity and condition updates written by
// node-agents on every tick. Without this gate, a single Node would wake every
// Task observing it on every unrelated tick, which is the churn the removed
// fixed-interval requeue used to hide rather than avoid.
func nodeReportChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, oldOK := e.ObjectOld.(*rlarkv1alpha1.Node)
			newNode, newOK := e.ObjectNew.(*rlarkv1alpha1.Node)
			if !oldOK || !newOK {
				// Unexpected type: fall back to the conservative direction and
				// let the reconciler decide from current state.
				return true
			}
			return !pullProgressEqual(oldNode.Status.PullProgress, newNode.Status.PullProgress) ||
				!nodeEventsEqual(oldNode.Status.Events, newNode.Status.Events)
		},
	}
}

// mapNodeToTasks resolves a Node report to the Pending Tasks observing it.
//
// Non-Pending Tasks are skipped: the aggregated fields are only owned while
// the Task is Pending, and clearing them on the Pending exit is already driven
// by the Task's own watch event.
func (r *Reconciler) mapNodeToTasks(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*rlarkv1alpha1.Node)
	if !ok {
		return nil
	}

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks,
		client.InNamespace(node.Namespace),
		client.MatchingFields{taskObservedNodeField: node.Name},
	); err != nil {
		// A failed mapping would silently lose the wakeup. Log it and rely on
		// the periodic freshness resync as the bounded fallback.
		log.FromContext(ctx).Error(err, "failed to map Node to observing Tasks",
			"node", node.Namespace+"/"+node.Name)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(tasks.Items))
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if task.Status.Phase != rlarkv1alpha1.TaskPhasePending {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(task),
		})
	}
	return requests
}
