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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	heartbeatLeaseName  = "heartbeat"
	CleanupFinalizer    = "clusters.rlinf.io/cleanup"
	cleanupRequeue      = time.Second
	drainInitialBackoff = time.Second
	drainMaxBackoff     = time.Minute
	drainTimeout        = 30 * time.Minute
)

// Reconciler reconciles Cluster status from the agent heartbeat and registered Nodes.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	Now                     func() time.Time
}

// +kubebuilder:rbac:groups=rlinf.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;delete
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cluster rlarkv1alpha1.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, &cluster)
	}
	if !controllerutil.ContainsFinalizer(&cluster, CleanupFinalizer) {
		controllerutil.AddFinalizer(&cluster, CleanupFinalizer)
		if err := r.Update(ctx, &cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("add Cluster cleanup finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	now := r.now()
	conflict, err := r.agentNamespaceConflict(ctx, &cluster)
	if err != nil {
		return ctrl.Result{}, err
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
			return ctrl.Result{}, fmt.Errorf("patch Cluster validation status: %w", err)
		}
	}
	var drainRequeueAfter *time.Duration
	if !cluster.Spec.Draining {
		if err := r.clearDrainTaskMarkers(ctx, &cluster); err != nil {
			return ctrl.Result{}, err
		}
	}
	if cluster.Spec.Draining && !cluster.Spec.Delisted {
		var complete bool
		var pendingTasks []string
		complete, pendingTasks, err = r.drainTasks(ctx, &cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
		if complete {
			before := cluster.DeepCopy()
			clearDrainCondition(&cluster.Status.Conditions, now)
			if !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
				if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
					return ctrl.Result{}, fmt.Errorf("patch completed cluster drain status: %w", err)
				}
			}
		} else {
			before := cluster.DeepCopy()
			startedAt := setDrainCondition(&cluster, now, pendingTasks)
			delay := nextDrainRequeueAfter(now, startedAt)
			drainRequeueAfter = &delay
			if !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
				if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
					return ctrl.Result{}, fmt.Errorf("patch cluster drain status: %w", err)
				}
			}
		}
	}
	if cluster.Spec.AgentNamespace == "" {
		return ctrl.Result{}, fmt.Errorf("cluster agent namespace is empty")
	}
	phase, heartbeat, leaseDuration, totalNodes, onlineNodes, err := r.observe(ctx, &cluster, now)
	if err != nil {
		return ctrl.Result{}, err
	}

	before := cluster.DeepCopy()
	cluster.Status.Phase = phase
	cluster.Status.LastHeartbeatTime = heartbeat
	cluster.Status.TotalNodes = totalNodes
	cluster.Status.OnlineNodes = onlineNodes
	readyStatus, reason := clusterReadyCondition(phase)
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeReady,
		Status:             readyStatus,
		Reason:             reason,
		Message:            "Cluster availability is derived from its agent heartbeat Lease and registered Nodes.",
		LastTransitionTime: metav1.NewTime(now),
	})
	if phase != before.Status.Phase || !sameTime(heartbeat, before.Status.LastHeartbeatTime) || totalNodes != before.Status.TotalNodes || onlineNodes != before.Status.OnlineNodes || !reflect.DeepEqual(cluster.Status.Conditions, before.Status.Conditions) {
		if err := r.Status().Patch(ctx, &cluster, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch cluster status: %w", err)
		}
	}

	if drainRequeueAfter != nil {
		return ctrl.Result{RequeueAfter: *drainRequeueAfter}, nil
	}
	return ctrl.Result{RequeueAfter: nextRequeueAfter(now, heartbeat, leaseDuration)}, nil
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

func (r *Reconciler) reconcileAgentNamespaceConflict(ctx context.Context, cluster *rlarkv1alpha1.Cluster, now time.Time) (ctrl.Result, error) {
	before := cluster.DeepCopy()
	cluster.Status.Phase = rlarkv1alpha1.ClusterOffline
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             "AgentNamespaceConflict",
		Message:            "Another active Cluster claims this agent namespace.",
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.NewTime(now),
	})
	apimeta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             "AgentNamespaceConflict",
		Message:            "Cluster availability cannot be determined while its agent namespace is claimed by another Cluster.",
		LastTransitionTime: metav1.NewTime(now),
	})
	if !reflect.DeepEqual(cluster.Status, before.Status) {
		if err := r.Status().Patch(ctx, cluster, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch Cluster namespace conflict status: %w", err)
		}
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *Reconciler) reconcileDeletion(ctx context.Context, cluster *rlarkv1alpha1.Cluster) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cluster, CleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	pending, err := r.deleteClusterResources(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
	}

	controllerutil.RemoveFinalizer(cluster, CleanupFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove Cluster cleanup finalizer: %w", err)
	}
	return ctrl.Result{}, nil
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

func clusterReadyCondition(phase rlarkv1alpha1.ClusterPhase) (metav1.ConditionStatus, string) {
	switch phase {
	case rlarkv1alpha1.ClusterOnline:
		return metav1.ConditionTrue, "ClusterOnline"
	case rlarkv1alpha1.ClusterDelisted:
		return metav1.ConditionFalse, "ClusterDelisted"
	case rlarkv1alpha1.ClusterCordoned:
		return metav1.ConditionFalse, "ClusterCordoned"
	case rlarkv1alpha1.ClusterDraining:
		return metav1.ConditionFalse, "ClusterDraining"
	case rlarkv1alpha1.ClusterDowngraded:
		return metav1.ConditionFalse, "RegisteredNodeOffline"
	default:
		return metav1.ConditionFalse, "AgentHeartbeatMissingOrExpired"
	}
}

func (r *Reconciler) clearDrainTaskMarkers(ctx context.Context, cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.AgentNamespace == "" {
		return fmt.Errorf("cluster agent namespace is empty")
	}

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return fmt.Errorf("list Tasks in agent namespace: %w", err)
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		stopped := task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] == "true"
		drainSuspended := task.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] == "true"
		if !task.DeletionTimestamp.IsZero() || (!stopped && !drainSuspended) {
			continue
		}
		before := task.DeepCopy()
		delete(task.Annotations, rlarkv1alpha1.ClusterStoppedAnnotation)
		if drainSuspended {
			// Only resume Tasks whose suspension was written by the drain; a Task the
			// user suspended before the drain keeps its own desired state.
			delete(task.Annotations, rlarkv1alpha1.ClusterDrainSuspendedAnnotation)
			task.Spec.Suspended = false
		}
		if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
			return fmt.Errorf("clear cluster drain marker from Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return nil
}

func taskTerminal(task *rlarkv1alpha1.Task) bool {
	return task.Status.Phase == rlarkv1alpha1.TaskPhaseSucceeded || task.Status.Phase == rlarkv1alpha1.TaskPhaseFailed
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
		if !task.DeletionTimestamp.IsZero() || taskTerminal(task) {
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

func (r *Reconciler) observe(ctx context.Context, cluster *rlarkv1alpha1.Cluster, now time.Time) (rlarkv1alpha1.ClusterPhase, *metav1.Time, time.Duration, int32, int32, error) {
	var nodes rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodes, client.InNamespace(cluster.Spec.AgentNamespace)); err != nil {
		return "", nil, 0, 0, 0, fmt.Errorf("list Nodes in agent namespace: %w", err)
	}

	var totalNodes, onlineNodes int32
	for _, node := range nodes.Items {
		totalNodes++
		if node.Status.Phase == rlarkv1alpha1.NodeOnline {
			onlineNodes++
		}
	}

	heartbeat, leaseDuration, err := r.lastHeartbeat(ctx, cluster.Spec.AgentNamespace)
	if err != nil {
		return "", nil, 0, 0, 0, err
	}
	if cluster.Spec.Delisted {
		return rlarkv1alpha1.ClusterDelisted, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
	}
	if cluster.Spec.Draining {
		return rlarkv1alpha1.ClusterDraining, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
	}
	if cluster.Spec.Cordoned {
		return rlarkv1alpha1.ClusterCordoned, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
	}
	if heartbeat == nil || heartbeat.Add(leaseDuration).Before(now) {
		return rlarkv1alpha1.ClusterOffline, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
	}
	if onlineNodes != totalNodes {
		return rlarkv1alpha1.ClusterDowngraded, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
	}
	return rlarkv1alpha1.ClusterOnline, heartbeat, leaseDuration, totalNodes, onlineNodes, nil
}

func (r *Reconciler) lastHeartbeat(ctx context.Context, namespace string) (*metav1.Time, time.Duration, error) {
	var lease coordinationv1.Lease
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: heartbeatLeaseName}, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("get agent heartbeat lease: %w", err)
	}
	if lease.Spec.RenewTime == nil {
		return nil, 0, nil
	}
	if lease.Spec.LeaseDurationSeconds == nil {
		return nil, 0, fmt.Errorf("agent heartbeat Lease has no duration")
	}
	heartbeat := metav1.NewTime(lease.Spec.RenewTime.Time)
	leaseDuration := time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	return &heartbeat, leaseDuration, nil
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

func clearDrainCondition(conditions *[]metav1.Condition, now time.Time) {
	condition := apimeta.FindStatusCondition(*conditions, rlarkv1alpha1.ConditionTypeProgressing)
	if condition == nil || condition.Status != metav1.ConditionTrue || (condition.Reason != "DrainInProgress" && condition.Reason != "DrainTimedOut") {
		return
	}
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeProgressing,
		Status:             metav1.ConditionFalse,
		Reason:             "DrainComplete",
		Message:            "All Tasks reported Stopped for their current generation.",
		ObservedGeneration: condition.ObservedGeneration,
		LastTransitionTime: metav1.NewTime(now),
	})
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

func nextRequeueAfter(now time.Time, heartbeat *metav1.Time, leaseDuration time.Duration) time.Duration {
	if heartbeat == nil || leaseDuration <= 0 {
		return time.Minute
	}
	remaining := heartbeat.Add(leaseDuration).Sub(now)
	if remaining <= 0 {
		return time.Minute
	}
	return remaining
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
