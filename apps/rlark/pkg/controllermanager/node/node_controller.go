package node

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	nodeHeartbeatLeasePrefix = "node-heartbeat-"
)

func nodeHeartbeatLeaseName(nodeName string) string {
	return nodeHeartbeatLeasePrefix + nodeName
}

// Reconciler reconciles Node resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	Now                     func() time.Time
	LeaseTimeout            time.Duration
}

// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/finalizers,verbs=update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch

// Reconcile derives node connection state from its dedicated node agent Lease.
// Nodes without a dedicated Lease are offline.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)

	var node rlarkv1alpha1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := r.now()
	lease, err := r.getHeartbeatLease(ctx, node.Namespace, node.Name)
	if err != nil {
		logger.Error(err, "get node heartbeat lease")
		return ctrl.Result{}, err
	}

	lastHeartbeat := node.Status.LastHeartbeatTime
	var phase rlarkv1alpha1.NodePhase
	var reason string
	leaseTimeout := r.leaseTimeout(lease)
	if lease != nil {
		if lease.Spec.RenewTime != nil {
			renewTime := metav1.NewTime(lease.Spec.RenewTime.Time)
			lastHeartbeat = &renewTime
		}
		if isLeaseActive(lease, now, leaseTimeout) {
			phase = rlarkv1alpha1.NodeOnline
			reason = "AgentLeaseActive"
		} else {
			phase = rlarkv1alpha1.NodeOffline
			reason = "AgentLeaseExpired"
		}
	} else {
		phase = rlarkv1alpha1.NodeOffline
		reason = "NodeAgentLeaseMissing"
	}

	readyStatus := metav1.ConditionFalse
	if phase == rlarkv1alpha1.NodeOnline {
		readyStatus = metav1.ConditionTrue
	}
	changed := phase != node.Status.Phase || reason != node.Status.Reason
	before := node.DeepCopy()
	node.Status.Phase = phase
	node.Status.Reason = reason
	node.Status.LastHeartbeatTime = lastHeartbeat
	if changed {
		statusChangeTime := metav1.NewTime(now)
		node.Status.LastStatusChangeTime = &statusChangeTime
	}
	apimeta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type:               rlarkv1alpha1.ConditionTypeReady,
		Status:             readyStatus,
		Reason:             reason,
		Message:            "Node agent connectivity is derived from its heartbeat Lease.",
		LastTransitionTime: metav1.NewTime(now),
	})
	if changed || !sameTime(lastHeartbeat, before.Status.LastHeartbeatTime) || !conditionsEqual(node.Status.Conditions, before.Status.Conditions) {
		if err := r.Status().Patch(ctx, &node, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch node connection status: %w", err)
		}
		logger.Info("updated node connection status", "phase", phase, "reason", reason)
	}

	return ctrl.Result{RequeueAfter: leaseTimeout}, nil
}

func (r *Reconciler) getHeartbeatLease(ctx context.Context, namespace, nodeName string) (*coordinationv1.Lease, error) {
	var lease coordinationv1.Lease
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: nodeHeartbeatLeaseName(nodeName)}, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent heartbeat lease: %w", err)
	}
	return &lease, nil
}

func (r *Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Reconciler) leaseTimeout(lease *coordinationv1.Lease) time.Duration {
	if r.LeaseTimeout > 0 {
		return r.LeaseTimeout
	}
	if lease != nil && lease.Spec.LeaseDurationSeconds != nil {
		return time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	}
	return time.Minute
}

func isLeaseActive(lease *coordinationv1.Lease, now time.Time, timeout time.Duration) bool {
	return lease.Spec.RenewTime != nil && !lease.Spec.RenewTime.Add(timeout).Before(now)
}

func sameTime(left, right *metav1.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(right)
}

func conditionsEqual(left, right []metav1.Condition) bool {
	return reflect.DeepEqual(left, right)
}

// SetupWithManager registers the controller with Node and Agent Lease watches.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Node{}).
		Watches(
			&coordinationv1.Lease{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
				nodeName, ok := strings.CutPrefix(object.GetName(), nodeHeartbeatLeasePrefix)
				if !ok || nodeName == "" {
					return nil
				}
				return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: object.GetNamespace(), Name: nodeName}}}
			}),
		).
		Named("node").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
