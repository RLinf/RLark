package cluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

const (
	heartbeatLeaseName  = "heartbeat"
	CleanupFinalizer    = "clusters.rlinf.io/cleanup"
	cleanupRequeue      = time.Second
	drainInitialBackoff = time.Second
	drainMaxBackoff     = time.Minute
	drainTimeout        = 30 * time.Minute
)

// Reason codes this controller attaches both to the Ready condition and to the
// classified pass. Closed set: they are used as a metric label.
const (
	reasonClusterOnline          = "ClusterOnline"
	reasonClusterCordoned        = "ClusterCordoned"
	reasonClusterDraining        = "ClusterDraining"
	reasonClusterDelisted        = "ClusterDelisted"
	reasonNodeOffline            = "RegisteredNodeOffline"
	reasonNodeDegraded           = "RegisteredNodeDegraded"
	reasonNoSchedulableNode      = "NoSchedulableNode"
	reasonClusterUnavailable     = "ClusterUnavailable"
	reasonHeartbeatMissing       = "AgentHeartbeatMissingOrExpired"
	reasonObserveFailed          = "ClusterObserveFailed"
	reasonAgentNamespaceConflict = "AgentNamespaceConflict"
)

// Reconciler reconciles Cluster status from the agent heartbeat and registered Nodes.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	Now                     func() time.Time
	// Shell classifies every pass, turns a retry class into a jittered interval
	// and records the result. The zero value is silent, so a test may leave it
	// unset.
	Shell controller.Shell
}

// +kubebuilder:rbac:groups=rlinf.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;delete
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Shell.Run(ctx, "cluster", func(ctx context.Context) (controller.Result, error) {
		return r.reconcile(ctx, req)
	})
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (controller.Result, error) {
	var cluster rlarkv1alpha1.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return controller.Result{
				Outcome: sharedreconcile.OutcomeDropped,
				Reason:  controller.ReasonObjectGone,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, &cluster)
	}
	if !controllerutil.ContainsFinalizer(&cluster, CleanupFinalizer) {
		controllerutil.AddFinalizer(&cluster, CleanupFinalizer)
		if err := r.Update(ctx, &cluster); err != nil {
			return controller.Result{Reason: controller.ReasonReconcileFailed}, fmt.Errorf("add Cluster cleanup finalizer: %w", err)
		}
		// The Update re-drives this controller through its own Cluster watch, so
		// there is nothing to poll for.
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  controller.ReasonSynced,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	now := r.now()
	conflict, err := r.agentNamespaceConflict(ctx, &cluster)
	if err != nil {
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}
	if conflict {
		return r.reconcileAgentNamespaceConflict(ctx, &cluster, now)
	}
	beforeValidation := cluster.DeepCopy()
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             "SpecValidated",
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.NewTime(now),
	})
	if !reflect.DeepEqual(cluster.Status.Conditions, beforeValidation.Status.Conditions) {
		if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(beforeValidation)); err != nil {
			return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch Cluster validation status: %w", err)
		}
	}
	var drainRequeueAfter *time.Duration
	if !cluster.Spec.Draining {
		beforeCancel := cluster.DeepCopy()
		retireDrainCondition(&cluster.Status.Conditions)
		if !reflect.DeepEqual(cluster.Status.Conditions, beforeCancel.Status.Conditions) {
			if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(beforeCancel)); err != nil {
				return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch cancelled cluster drain status: %w", err)
			}
		}
	}
	// drainComplete records this pass's drain observation so the phase derivation
	// below can tell an in-flight drain from a finished one. It stays false when no
	// drain is requested or the cluster is delisted, where the drain branch is
	// skipped and the phase is decided by other spec fields.
	drainComplete := false
	if cluster.Spec.Draining && !cluster.Spec.Delisted {
		var pendingTasks []string
		drainComplete, pendingTasks, err = r.drainTasks(ctx, &cluster)
		if err != nil {
			return controller.Result{Reason: controller.ReasonReconcileFailed}, err
		}
		if drainComplete {
			before := cluster.DeepCopy()
			clearDrainCondition(&cluster.Status.Conditions, now, cluster.Generation)
			if !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
				if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
					return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch completed cluster drain status: %w", err)
				}
			}
		} else {
			before := cluster.DeepCopy()
			startedAt := setDrainCondition(&cluster, now, pendingTasks)
			delay := nextDrainRequeueAfter(now, startedAt)
			drainRequeueAfter = &delay
			if !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
				if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
					return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch cluster drain status: %w", err)
				}
			}
		}
	}
	if cluster.Spec.AgentNamespace == "" {
		return controller.Result{Reason: controller.ReasonReconcileFailed}, fmt.Errorf("cluster agent namespace is empty")
	}
	phase, heartbeat, summary, err := r.observe(ctx, &cluster, now, drainComplete)
	if err != nil {
		return controller.Result{Reason: reasonObserveFailed}, err
	}
	if heartbeat.ClockSkew {
		log.FromContext(ctx).Info("agent heartbeat Lease is dated in the future; honouring it but the agent clock runs ahead", "cluster", cluster.Name)
	}

	before := cluster.DeepCopy()
	phaseChanged := phase != before.Status.Phase
	cluster.Status.Phase = phase
	cluster.Status.ObservedGeneration = cluster.Generation
	lastHeartbeat := controller.HeartbeatTime(heartbeat)
	cluster.Status.LastHeartbeatTime = lastHeartbeat
	if phaseChanged {
		statusChangeTime := metav1.NewTime(now)
		cluster.Status.LastStatusChangeTime = &statusChangeTime
	}
	cluster.Status.TotalNodes = summary.totalNodes
	cluster.Status.OnlineNodes = summary.readyNodes
	cluster.Status.ReadyNodes = summary.readyNodes
	cluster.Status.DegradedNodes = summary.degradedNodes
	cluster.Status.DisconnectedNodes = summary.disconnectedNodes
	cluster.Status.UnschedulableNodes = summary.unschedulableNodes
	cluster.Status.DominantFailureReason = summary.dominantFailureReason
	connected := !heartbeat.Expired()
	connectionReason := rlarkv1alpha1.ConditionReasonLeaseValid
	switch heartbeat.State {
	case sharedreconcile.HeartbeatMissing:
		connectionReason = rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived
	case sharedreconcile.HeartbeatExpired:
		connectionReason = rlarkv1alpha1.ConditionReasonLeaseExpired
	}
	readyStatus, reason := clusterReadyCondition(phase, summary.dominantFailureReason)
	// Both conditions are observation-driven: they summarise the heartbeat Lease
	// and the registered Nodes, neither of which is derived from this Cluster's
	// Spec. Passing generation 0 keeps them from claiming an ObservedGeneration
	// they did not compute. Agent connectivity is tracked independently from
	// node-derived availability so an expired cluster heartbeat stays visible
	// while registered Nodes still report ready.
	sharedreconcile.ApplyConditionMutations(&cluster.Status.Conditions, 0, []sharedreconcile.ConditionMutation{
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeAgentConnected,
			Status:             boolCondition(connected),
			Reason:             connectionReason,
			Message:            "Agent connectivity is derived from the cluster heartbeat Lease.",
			LastTransitionTime: metav1.NewTime(now),
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeReady,
			Status:             readyStatus,
			Reason:             reason,
			Message:            "Cluster availability is derived from registered Node conditions.",
			LastTransitionTime: metav1.NewTime(now),
		}},
	})
	if phaseChanged || cluster.Status.ObservedGeneration != before.Status.ObservedGeneration || !sameTime(lastHeartbeat, before.Status.LastHeartbeatTime) || cluster.Status.TotalNodes != before.Status.TotalNodes || cluster.Status.OnlineNodes != before.Status.OnlineNodes || cluster.Status.ReadyNodes != before.Status.ReadyNodes || cluster.Status.DegradedNodes != before.Status.DegradedNodes || cluster.Status.DisconnectedNodes != before.Status.DisconnectedNodes || cluster.Status.UnschedulableNodes != before.Status.UnschedulableNodes || cluster.Status.DominantFailureReason != before.Status.DominantFailureReason || !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
		if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
			if apierrors.IsConflict(err) {
				return controller.Result{Reason: controller.ReasonStatusWriteConflict}, err
			}
			return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch cluster status: %w", err)
		}
	}

	// A drain is a self-driven operation: the Tasks it stopped have not reported
	// Stopped yet, so this pass must come back on the drain's own schedule
	// instead of waiting for the heartbeat deadline.
	if drainRequeueAfter != nil {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  reason,
			Retry: sharedreconcile.RetryDirective{
				Class: sharedreconcile.RetryInProgress,
				After: *drainRequeueAfter,
			},
		}, nil
	}
	// Heartbeat expiry produces no event, so the deadline can only be observed
	// by looking again. The shell jitters the interval so that Clusters sharing
	// a lease duration do not re-check in lockstep.
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reason,
		Retry: sharedreconcile.RetryDirective{
			Class: sharedreconcile.RetryOnChange,
			After: controller.NextHeartbeatCheck(now, heartbeat),
		},
	}, nil
}

func (r *Reconciler) agentNamespaceConflict(ctx context.Context, cluster *rlarkv1alpha1.Cluster) (bool, error) {
	var clusters rlarkv1alpha1.ClusterList
	if err := r.List(ctx, &clusters); err != nil {
		return false, fmt.Errorf("list Clusters for agent namespace conflict: %w", err)
	}
	for i := range clusters.Items {
		other := &clusters.Items[i]
		if other.Name != cluster.Name && other.DeletionTimestamp.IsZero() && other.Spec.AgentNamespace == cluster.Spec.AgentNamespace {
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) reconcileAgentNamespaceConflict(ctx context.Context, cluster *rlarkv1alpha1.Cluster, now time.Time) (controller.Result, error) {
	before := cluster.DeepCopy()
	cluster.Status.Phase = rlarkv1alpha1.ClusterOffline
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             reasonAgentNamespaceConflict,
		Message:            "Another active Cluster claims this agent namespace.",
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.NewTime(now),
	})
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             reasonAgentNamespaceConflict,
		Message:            "Cluster availability cannot be determined while its agent namespace is claimed by another Cluster.",
		LastTransitionTime: metav1.NewTime(now),
	})
	if !reflect.DeepEqual(cluster.Status, before.Status) {
		if err := r.Status().Patch(ctx, cluster, client.MergeFrom(before)); err != nil {
			return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch Cluster namespace conflict status: %w", err)
		}
	}
	// The competing Cluster may be deleted without this controller observing an
	// event for it, so re-check instead of parking on the conflict forever.
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonAgentNamespaceConflict,
		Retry: sharedreconcile.RetryDirective{
			Class: sharedreconcile.RetryOnChange,
			After: time.Minute,
		},
	}, nil
}

func (r *Reconciler) reconcileDeletion(ctx context.Context, cluster *rlarkv1alpha1.Cluster) (controller.Result, error) {
	if !controllerutil.ContainsFinalizer(cluster, CleanupFinalizer) {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  controller.ReasonObjectGone,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	pending, err := r.deleteClusterResources(ctx, cluster)
	if err != nil {
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}
	if pending {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  controller.ReasonChildCleanupInProgress,
			Retry: sharedreconcile.RetryDirective{
				Class: sharedreconcile.RetryInProgress,
				After: cleanupRequeue,
			},
		}, nil
	}

	controllerutil.RemoveFinalizer(cluster, CleanupFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return controller.Result{Reason: controller.ReasonReconcileFailed}, fmt.Errorf("remove Cluster cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  controller.ReasonSynced,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

func (r *Reconciler) deleteClusterResources(ctx context.Context, cluster *rlarkv1alpha1.Cluster) (bool, error) {
	if cluster.Spec.AgentNamespace == "" {
		return false, fmt.Errorf("cluster agent namespace is empty")
	}

	pending := false

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return false, fmt.Errorf("list Tasks in agent namespace: %w", err)
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		pending = true
		if task.DeletionTimestamp.IsZero() {
			if err := client.IgnoreNotFound(r.Delete(ctx, task)); err != nil {
				return false, fmt.Errorf("delete Task %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
	}

	var nodes rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodes, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return false, fmt.Errorf("list Nodes in agent namespace: %w", err)
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		pending = true
		if node.DeletionTimestamp.IsZero() {
			if err := client.IgnoreNotFound(r.Delete(ctx, node)); err != nil {
				return false, fmt.Errorf("delete Node %s/%s: %w", node.Namespace, node.Name, err)
			}
		}
	}

	var heartbeat coordinationv1.Lease
	if err := r.Get(ctx, client.ObjectKey{Namespace: cluster.Spec.AgentNamespace, Name: heartbeatLeaseName}, &heartbeat); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("get agent heartbeat Lease: %w", err)
		}
	} else {
		pending = true
		if heartbeat.DeletionTimestamp.IsZero() {
			if err := client.IgnoreNotFound(r.Delete(ctx, &heartbeat)); err != nil {
				return false, fmt.Errorf("delete agent heartbeat Lease: %w", err)
			}
		}
	}

	return pending, nil
}

func clusterReadyCondition(phase rlarkv1alpha1.ClusterPhase, dominantFailureReason string) (metav1.ConditionStatus, string) {
	switch phase {
	case rlarkv1alpha1.ClusterOnline:
		return metav1.ConditionTrue, reasonClusterOnline
	case rlarkv1alpha1.ClusterDelisted:
		return metav1.ConditionFalse, reasonClusterDelisted
	case rlarkv1alpha1.ClusterCordoned:
		return metav1.ConditionFalse, reasonClusterCordoned
	case rlarkv1alpha1.ClusterDraining:
		return metav1.ConditionFalse, reasonClusterDraining
	case rlarkv1alpha1.ClusterDowngraded:
		return metav1.ConditionFalse, reasonNodeDegraded
	case rlarkv1alpha1.ClusterOffline:
		if dominantFailureReason != "" {
			return metav1.ConditionFalse, dominantFailureReason
		}
		return metav1.ConditionFalse, reasonNoSchedulableNode
	default:
		return metav1.ConditionFalse, reasonClusterUnavailable
	}
}

func taskTerminal(task *rlarkv1alpha1.Task) bool {
	return task.Status.Phase == rlarkv1alpha1.TaskPhaseSucceeded || task.Status.Phase == rlarkv1alpha1.TaskPhaseFailed
}

// taskCanHoldWorkload reports whether the Task declares a Runtime. The Job
// scheduler only ever places Tasks that declare one, so a Task without a Runtime
// has never held and cannot hold a workload on any Node (for example a Task
// created before spec.runtime became required). Such an inert Task can never be
// stopped, so it must not block a cluster drain forever.
func taskCanHoldWorkload(task *rlarkv1alpha1.Task) bool {
	return task.Spec.Runtime.Kind != ""
}

func taskHasJobOwner(task *rlarkv1alpha1.Task) bool {
	for _, owner := range task.OwnerReferences {
		if owner.APIVersion == rlarkv1alpha1.GroupVersion.String() && owner.Kind == "Job" {
			return true
		}
	}
	return false
}

func (r *Reconciler) drainTasks(ctx context.Context, cluster *rlarkv1alpha1.Cluster) (bool, []string, error) {
	if cluster.Spec.AgentNamespace == "" {
		return false, nil, fmt.Errorf("cluster agent namespace is empty")
	}

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return false, nil, fmt.Errorf("list Tasks in agent namespace: %w", err)
	}

	complete := true
	pendingTasks := make([]string, 0, len(tasks.Items))
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !task.DeletionTimestamp.IsZero() || taskTerminal(task) || !taskCanHoldWorkload(task) {
			continue
		}
		if task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] == "true" {
			if task.Status.ObservedGeneration >= task.Generation && task.Status.Phase == rlarkv1alpha1.TaskPhaseStopped {
				continue
			}
			complete = false
			pendingTasks = append(pendingTasks, task.Namespace+"/"+task.Name)
			continue
		}
		complete = false
		pendingTasks = append(pendingTasks, task.Namespace+"/"+task.Name)
		before := task.DeepCopy()
		if task.Annotations == nil {
			task.Annotations = map[string]string{}
		}
		task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] = "true"
		if !taskHasJobOwner(task) && !task.Spec.Suspended {
			// Standalone Tasks have no Job controller to honour the stopped marker, so
			// the drain owns their suspension and records that it did.
			task.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] = "true"
			task.Spec.Suspended = true
		}
		if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
			return false, nil, fmt.Errorf("mark Task %s/%s for cluster drain: %w", task.Namespace, task.Name, err)
		}
	}
	return complete, pendingTasks, nil
}

type nodeSummary struct {
	totalNodes            int32
	readyNodes            int32
	degradedNodes         int32
	disconnectedNodes     int32
	unschedulableNodes    int32
	dominantFailureReason string
}

func conditionStatus(conditions []metav1.Condition, conditionType string) (metav1.ConditionStatus, string) {
	condition := apimeta.FindStatusCondition(conditions, conditionType)
	if condition == nil {
		return metav1.ConditionUnknown, ""
	}
	return condition.Status, condition.Reason
}

func dominantReason(reasons map[string]int) string {
	var dominant string
	for reason, count := range reasons {
		if count > reasons[dominant] || (count == reasons[dominant] && (dominant == "" || reason < dominant)) {
			dominant = reason
		}
	}
	return dominant
}

func summarizeNodes(nodes []rlarkv1alpha1.Node) nodeSummary {
	summary := nodeSummary{}
	failureReasons := map[string]int{}
	for _, node := range nodes {
		summary.totalNodes++
		if node.Spec.Unschedulable {
			summary.unschedulableNodes++
			continue
		}

		connected, connectedReason := conditionStatus(node.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
		ready, readyReason := conditionStatus(node.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
		degraded, degradedReason := conditionStatus(node.Status.Conditions, rlarkv1alpha1.ConditionTypeDegraded)
		if connected == metav1.ConditionTrue && ready == metav1.ConditionTrue {
			summary.readyNodes++
			if degraded == metav1.ConditionTrue {
				summary.degradedNodes++
			}
			continue
		}
		if connected != metav1.ConditionTrue {
			summary.disconnectedNodes++
			if connectedReason != "" {
				failureReasons[connectedReason]++
			}
			continue
		}
		summary.degradedNodes++
		if readyReason != "" {
			failureReasons[readyReason]++
		} else if degradedReason != "" {
			failureReasons[degradedReason]++
		} else {
			failureReasons["NodeNotReady"]++
		}
	}
	summary.dominantFailureReason = dominantReason(failureReasons)
	return summary
}

func (r *Reconciler) observe(ctx context.Context, cluster *rlarkv1alpha1.Cluster, now time.Time, drainComplete bool) (rlarkv1alpha1.ClusterPhase, sharedreconcile.HeartbeatObservation, nodeSummary, error) {
	var nodes rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodes, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return "", sharedreconcile.HeartbeatObservation{}, nodeSummary{}, fmt.Errorf("list Nodes in agent namespace: %w", err)
	}
	summary := summarizeNodes(nodes.Items)

	heartbeat, err := r.observeHeartbeat(ctx, cluster, now)
	if err != nil {
		return "", sharedreconcile.HeartbeatObservation{}, nodeSummary{}, err
	}
	if cluster.Spec.Delisted {
		return rlarkv1alpha1.ClusterDelisted, heartbeat, summary, nil
	}
	// Draining is a bounded operation, so the phase reports it only while the drain
	// is still in flight. Once the drain has observed every eligible Task stopped,
	// fall through to the durable scheduling intent instead of pinning "Draining"
	// forever; the cordon the drain set remains in Spec, so the cluster reports
	// Cordoned. This mirrors the Node Controller, which reports a cordoned Node as
	// Cordoned once its drain operation completes.
	if cluster.Spec.Draining && !drainComplete {
		return rlarkv1alpha1.ClusterDraining, heartbeat, summary, nil
	}
	if cluster.Spec.Cordoned {
		return rlarkv1alpha1.ClusterCordoned, heartbeat, summary, nil
	}
	if summary.readyNodes == 0 {
		return rlarkv1alpha1.ClusterOffline, heartbeat, summary, nil
	}
	if summary.readyNodes != summary.totalNodes || summary.degradedNodes != 0 || summary.disconnectedNodes != 0 || summary.unschedulableNodes != 0 {
		return rlarkv1alpha1.ClusterDowngraded, heartbeat, summary, nil
	}
	return rlarkv1alpha1.ClusterOnline, heartbeat, summary, nil
}

// observeHeartbeat classifies the agent heartbeat Lease through the shared
// primitive so that the Cluster and Node controllers cannot disagree about what
// the same Lease means.
func (r *Reconciler) observeHeartbeat(ctx context.Context, cluster *rlarkv1alpha1.Cluster, now time.Time) (sharedreconcile.HeartbeatObservation, error) {
	var lease coordinationv1.Lease
	err := r.Get(ctx, client.ObjectKey{Namespace: cluster.Spec.AgentNamespace, Name: heartbeatLeaseName}, &lease)
	switch {
	case err == nil:
		return controller.ObserveLeaseHeartbeat(&lease, cluster.Status.LastHeartbeatTime, now, sharedreconcile.HeartbeatTimeouts{}), nil
	case apierrors.IsNotFound(err):
		// A missing Lease is a de-registration signal, not a read failure.
		return controller.ObserveLeaseHeartbeat(nil, cluster.Status.LastHeartbeatTime, now, sharedreconcile.HeartbeatTimeouts{}), nil
	default:
		return sharedreconcile.HeartbeatObservation{}, fmt.Errorf("get agent heartbeat lease: %w", err)
	}
}

func boolCondition(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func (r *Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func sameTime(left, right *metav1.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(right)
}

func setDrainCondition(cluster *rlarkv1alpha1.Cluster, now time.Time, pendingTasks []string) *metav1.Time {
	condition := apimeta.FindStatusCondition(cluster.Status.Conditions, rlarkv1alpha1.ConditionTypeProgressing)
	startedAt := now
	if condition != nil && condition.Status == metav1.ConditionTrue && condition.ObservedGeneration == cluster.Generation && (condition.Reason == "DrainInProgress" || condition.Reason == "DrainTimedOut") {
		startedAt = condition.LastTransitionTime.Time
	}

	reason := "DrainInProgress"
	message := fmt.Sprintf("Waiting for %d Task(s) to report Stopped for their current generation.", len(pendingTasks))
	if now.Sub(startedAt) >= drainTimeout {
		reason = "DrainTimedOut"
		message = fmt.Sprintf("Drain exceeded the %s deadline; waiting for Task(s): %v.", drainTimeout, pendingTasks)
	}
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeProgressing,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.NewTime(now),
	})
	return &metav1.Time{Time: startedAt}
}

// clearDrainCondition publishes the completed drain for the observed generation.
// Completion must be recorded even when no Task ever needed to stop, otherwise a
// drain of an empty cluster never satisfies consumers that gate on a
// current-generation DrainComplete condition.
func clearDrainCondition(conditions *[]metav1.Condition, now time.Time, observedGeneration int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeProgressing,
		Status:             metav1.ConditionFalse,
		Reason:             "DrainComplete",
		Message:            "No Task remains running for its current generation.",
		ObservedGeneration: observedGeneration,
		LastTransitionTime: metav1.NewTime(now),
	})
}

// retireDrainCondition removes a drain progress condition once draining is
// switched off. Keeping the condition would leave the Cluster reporting
// Progressing after the drain was cancelled, and its LastTransitionTime would
// be picked up as the deadline start of the next drain, which would then report
// DrainTimedOut immediately.
func retireDrainCondition(conditions *[]metav1.Condition) {
	condition := apimeta.FindStatusCondition(*conditions, rlarkv1alpha1.ConditionTypeProgressing)
	if condition == nil || condition.Reason != "DrainInProgress" && condition.Reason != "DrainTimedOut" {
		return
	}
	apimeta.RemoveStatusCondition(conditions, rlarkv1alpha1.ConditionTypeProgressing)
}

func nextDrainRequeueAfter(now time.Time, startedAt *metav1.Time) time.Duration {
	if startedAt == nil {
		return drainInitialBackoff
	}
	elapsed := now.Sub(startedAt.Time)
	if elapsed >= drainTimeout {
		return drainMaxBackoff
	}

	delay := drainInitialBackoff
	for elapsed >= delay && delay < drainMaxBackoff {
		delay *= 2
	}
	if remaining := drainTimeout - elapsed; remaining < delay {
		return remaining
	}
	return delay
}

// SetupWithManager registers Cluster, Node, and heartbeat Lease watches.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Cluster{}).
		Watches(&rlarkv1alpha1.Node{}, handler.EnqueueRequestsFromMapFunc(r.clusterForNode)).
		Watches(&rlarkv1alpha1.Task{}, handler.EnqueueRequestsFromMapFunc(r.clusterForTask)).
		Watches(&coordinationv1.Lease{}, handler.EnqueueRequestsFromMapFunc(r.clustersForHeartbeatLease)).
		Named("cluster").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *Reconciler) clusterForNode(ctx context.Context, object client.Object) []reconcile.Request {
	var clusters rlarkv1alpha1.ClusterList
	if err := r.List(ctx, &clusters); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, 1)
	for i := range clusters.Items {
		if clusters.Items[i].Spec.AgentNamespace == object.GetNamespace() && clusters.Items[i].DeletionTimestamp.IsZero() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: clusters.Items[i].Name}})
		}
	}
	return requests
}

func (r *Reconciler) clusterForTask(ctx context.Context, object client.Object) []reconcile.Request {
	var clusters rlarkv1alpha1.ClusterList
	if err := r.List(ctx, &clusters); err != nil {
		return nil
	}
	for i := range clusters.Items {
		if clusters.Items[i].Spec.AgentNamespace == object.GetNamespace() && clusters.Items[i].Spec.Draining {
			return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: clusters.Items[i].Name}}}
		}
	}
	return nil
}

func (r *Reconciler) clustersForHeartbeatLease(ctx context.Context, object client.Object) []reconcile.Request {
	if object.GetName() != heartbeatLeaseName {
		return nil
	}

	var clusters rlarkv1alpha1.ClusterList
	if err := r.List(ctx, &clusters); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, 1)
	for _, cluster := range clusters.Items {
		if cluster.Spec.AgentNamespace == object.GetNamespace() && cluster.DeletionTimestamp.IsZero() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: cluster.Name}})
		}
	}
	return requests
}
