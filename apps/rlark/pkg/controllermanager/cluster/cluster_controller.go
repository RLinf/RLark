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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	heartbeatLeaseName = "heartbeat"
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
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cluster rlarkv1alpha1.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := r.now()
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

	return ctrl.Result{RequeueAfter: nextRequeueAfter(now, heartbeat, leaseDuration)}, nil
}

func clusterReadyCondition(phase rlarkv1alpha1.ClusterPhase) (metav1.ConditionStatus, string) {
	switch phase {
	case rlarkv1alpha1.ClusterOnline:
		return metav1.ConditionTrue, "ClusterOnline"
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
		if clusters.Items[i].Spec.AgentNamespace == object.GetNamespace() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: clusters.Items[i].Name}})
		}
	}
	return requests
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
		if cluster.Spec.AgentNamespace == object.GetNamespace() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: cluster.Name}})
		}
	}
	return requests
}
