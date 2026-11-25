package node

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
)

const (
	drainRequeueInterval = 10 * time.Second
	maxTaskDrainRequests = 32
)

type pullNodeReconciler struct {
	c *Controller
}

// Reconcile watches management Node CRs and projects their one-shot operations
// onto the local Kubernetes Node or local Tasks. The reconciler has one job:
// translate Node Drain operations into Task DrainRequests.
//
// Label changes are no longer pulled from management CR metadata.labels — the
// Gateway relays PATCH requests directly to the data-plane Kubernetes Node via
// proxyKubeRequest, so the data-plane agent never needs to reconcile labels
// from a management CR that may be stale or incomplete.
func (r *pullNodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)

	var mgmtNode rlarkv1alpha1.Node
	if err := r.c.ManagementClient.Get(ctx, req.NamespacedName, &mgmtNode); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	var localNode corev1.Node
	if err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: mgmtNode.Name}, &localNode); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	result := reconcile.Result{}
	if mgmtNode.DeletionTimestamp == nil && activeDrainOperation(&mgmtNode) != nil {
		result.RequeueAfter = drainRequeueInterval
	}
	if err := r.reconcileDrainRequests(ctx, &localNode, &mgmtNode); err != nil {
		logger.Error(err, "failed to reconcile Node Drain requests")
		return result, err
	}

	// Keep the RLark-owned scheduling label in sync (rlark.io/unschedulable).
	// This label signals RLark node affinity; unlike a full metadata.labels
	// sync it only handles the single internal key, so other labels added
	// by kubelet or device plugins are never touched.
	if err := r.reconcileSchedulingLabel(ctx, logger, &localNode, &mgmtNode); err != nil {
		logger.Error(err, "failed to reconcile scheduling label")
		return result, err
	}

	return result, nil
}

// reconcileSchedulingLabel keeps the RLark-owned rlark.io/unschedulable label
// in sync with spec.unschedulable. Pause/resume RLark scheduling is distinct
// from Kubernetes cordoning — we only write the rlark.io/unschedulable label,
// never the local Node's spec.unschedulable, so that other platforms can still
// schedule Pods on the node.
func (r *pullNodeReconciler) reconcileSchedulingLabel(
	ctx context.Context,
	logger logr.Logger,
	localNode *corev1.Node,
	mgmtNode *rlarkv1alpha1.Node,
) error {
	if localNode.Labels == nil {
		localNode.Labels = make(map[string]string)
	}

	changed := false
	if mgmtNode.Spec.Unschedulable {
		if localNode.Labels[nodeUnschedulableLabel] != "true" {
			localNode.Labels[nodeUnschedulableLabel] = "true"
			changed = true
		}
	} else if _, exists := localNode.Labels[nodeUnschedulableLabel]; exists {
		delete(localNode.Labels, nodeUnschedulableLabel)
		changed = true
	}
	if !changed {
		return nil
	}
	if err := r.c.LocalKubeClient.Update(ctx, localNode); err != nil {
		return fmt.Errorf("update local K8s Node scheduling label: %w", err)
	}
	logger.Info("synced rlark.io/unschedulable label from spec.unschedulable")
	return nil
}

// activeDrainOperation returns the first Node Drain request that the Node
// Controller has not reported as completed for the current Node incarnation.
// Requeueing stops once the drain completes because a draining Node is not a
// scheduling candidate, so no new Pods can appear on it.
func activeDrainOperation(node *rlarkv1alpha1.Node) *rlarkv1alpha1.NodeOperationRequest {
	completed := make(map[string]struct{}, len(node.Status.DrainOperations))
	for _, status := range node.Status.DrainOperations {
		if status.Completed {
			completed[status.OperationID] = struct{}{}
		}
	}
	for i := range node.Spec.Operations {
		operation := &node.Spec.Operations[i]
		if operation.Type != rlarkv1alpha1.NodeOperationDrain || operation.ID == "" {
			continue
		}
		if _, done := completed[operation.ID]; done {
			continue
		}
		return operation
	}
	return nil
}

// reconcileDrainRequests projects the Node's current Drain operations onto the
// Tasks that run on this Node. A request exists if and only if the Node still
// carries the corresponding Drain operation, so undrain and recover release
// Tasks instead of leaving them blocked in LifecycleWait forever. A Node that is
// being deleted cannot be undrained, so termination also releases its requests.
func (r *pullNodeReconciler) reconcileDrainRequests(ctx context.Context, localNode *corev1.Node, mgmtNode *rlarkv1alpha1.Node) error {
	if mgmtNode.DeletionTimestamp == nil {
		if drain := activeDrainOperation(mgmtNode); drain != nil {
			if err := r.recordDrainRequests(ctx, localNode, drain); err != nil {
				return err
			}
		}
	}
	return r.removeStaleDrainRequests(ctx, localNode, mgmtNode)
}

// removeStaleDrainRequests deletes Task DrainRequests that target this Node name
// but no longer correspond to a Drain operation in the Node's Spec. Matching on the
// Node name also releases requests written for a previous incarnation of a
// re-created Node, because such an incarnation will never observe its own operations
// again. A terminating Node is treated as having no Drain operations so an in-flight
// Node Agent cannot re-add requests while the object waits for deletion finalizers.
//
// Note: Completed drain operations are deliberately kept in the present set. An
// operation that has finished draining but still remains in node.spec.operations
// (waiting for the operator to undrain) must continue to keep Tasks stopped, because
// the Node remains cordoned and Tasks must not auto-restart on it.
func (r *pullNodeReconciler) removeStaleDrainRequests(ctx context.Context, localNode *corev1.Node, mgmtNode *rlarkv1alpha1.Node) error {
	present := make(map[string]struct{}, len(mgmtNode.Spec.Operations))
	if mgmtNode.DeletionTimestamp == nil {
		for _, operation := range mgmtNode.Spec.Operations {
			if operation.Type == rlarkv1alpha1.NodeOperationDrain && operation.ID != "" {
				present[operation.ID] = struct{}{}
			}
		}
	}

	var tasks rlarkv1alpha1.TaskList
	if err := r.c.ManagementClient.List(ctx, &tasks, client.InNamespace(mgmtNode.Namespace)); err != nil {
		return fmt.Errorf("list Tasks for drain request cleanup: %w", err)
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		kept := make([]rlarkv1alpha1.TaskDrainRequest, 0, len(task.Spec.DrainRequests))
		for _, request := range task.Spec.DrainRequests {
			if request.NodeName != localNode.Name {
				kept = append(kept, request)
				continue
			}
			if _, ok := present[request.OperationID]; ok {
				kept = append(kept, request)
			}
		}
		if len(kept) == len(task.Spec.DrainRequests) {
			continue
		}
		before := task.DeepCopy()
		if len(kept) == 0 {
			task.Spec.DrainRequests = nil
		} else {
			task.Spec.DrainRequests = kept
		}
		if err := r.c.ManagementClient.Patch(ctx, task, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("remove stale drain request from Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return nil
}

// recordDrainRequests records the Node Drain request on every management Task
// that still has a Pod scheduled to the local Node. Pods rather than workload
// pod templates identify placement because the Kubernetes scheduler assigns the
// concrete Node after the workload is created.
func (r *pullNodeReconciler) recordDrainRequests(ctx context.Context, localNode *corev1.Node, drain *rlarkv1alpha1.NodeOperationRequest) error {
	podsClient := r.c.LocalKubeCachedClient
	if podsClient == nil {
		podsClient = r.c.LocalKubeClient
	}
	var pods corev1.PodList
	if err := podsClient.List(ctx, &pods, client.MatchingFields{podNodeNameField: localNode.Name}); err != nil {
		return fmt.Errorf("list Pods on Node %s: %w", localNode.Name, err)
	}

	seen := make(map[types.NamespacedName]struct{})
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName != localNode.Name {
			continue
		}
		taskNamespace := pod.Annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation]
		taskName := pod.Annotations[kubernetesobjects.ManagementTaskNameAnnotation]
		taskUID := pod.Annotations[kubernetesobjects.ManagementTaskUIDAnnotation]
		if taskNamespace == "" || taskName == "" || taskUID == "" {
			continue
		}
		key := types.NamespacedName{Namespace: taskNamespace, Name: taskName}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		var task rlarkv1alpha1.Task
		if err := r.c.ManagementClient.Get(ctx, key, &task); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("get Task %s: %w", key, err)
		}
		if string(task.UID) != taskUID || hasDrainRequest(&task, drain.ID, string(localNode.UID)) {
			continue
		}
		if len(task.Spec.DrainRequests) >= maxTaskDrainRequests {
			return fmt.Errorf("record drain request on Task %s: spec.drainRequests cannot exceed %d entries", key, maxTaskDrainRequests)
		}
		before := task.DeepCopy()
		task.Spec.DrainRequests = append(task.Spec.DrainRequests, rlarkv1alpha1.TaskDrainRequest{
			OperationID: drain.ID,
			NodeUID:     string(localNode.UID),
			NodeName:    localNode.Name,
		})
		if err := r.c.ManagementClient.Patch(ctx, &task, client.MergeFrom(before)); err != nil {
			return fmt.Errorf("record drain request on Task %s: %w", key, err)
		}
	}
	return nil
}

func hasDrainRequest(task *rlarkv1alpha1.Task, operationID, nodeUID string) bool {
	for _, request := range task.Spec.DrainRequests {
		if request.OperationID == operationID && request.NodeUID == nodeUID {
			return true
		}
	}
	return false
}
