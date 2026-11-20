package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// DefaultOfflineAfter is the heartbeat freshness window in the API contract.
	DefaultOfflineAfter = 90 * time.Second
	// rebootMargin is how much later a reported boot time must be to count as a reboot.
	rebootMargin = 30 * time.Second
)

// SelfHealedAnnotation is, on a NodeAdmission, what RLark found of the
// machine's earlier attempt or earlier node and put right by itself, as the
// console shows it. Station API writes it too.
const SelfHealedAnnotation = "station.rlark.io/self-healed"

// NodeReconciler writes status.node of a NetworkEnrollment from the report
// station-api stored in status.observation, and passes the record to another
// Node of the name when an admission let that Node in.
type NodeReconciler struct {
	client        client.Client
	reader        client.Reader
	nodeReadiness NodeReadiness
	offlineAfter  time.Duration
	now           func() time.Time
}

func NewNodeReconciler(writer client.Client, reader client.Reader, readiness NodeReadiness, offlineAfter time.Duration, now func() time.Time) *NodeReconciler {
	return &NodeReconciler{client: writer, reader: reader, nodeReadiness: readiness, offlineAfter: offlineAfter, now: now}
}

func (r *NodeReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("node").
		For(&stationv1alpha1.NetworkEnrollment{}, builder.WithPredicates(predicate.Funcs{
			// The network's answer and this reconciler's own block change nothing here.
			UpdateFunc: func(e event.UpdateEvent) bool {
				before, after := e.ObjectOld.(*stationv1alpha1.NetworkEnrollment), e.ObjectNew.(*stationv1alpha1.NetworkEnrollment)
				return before.Generation != after.Generation || !reflect.DeepEqual(before.Status.Observation, after.Status.Observation)
			},
		})).
		Complete(r)
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	enrollment := &stationv1alpha1.NetworkEnrollment{}
	if err := r.reader.Get(ctx, req.NamespacedName, enrollment); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read NetworkEnrollment: %w", err)
	}
	observation := enrollment.Status.Observation
	if observation == nil {
		return reconcile.Result{}, nil
	}
	now := r.now().UTC()
	node := &stationv1alpha1.NetworkEnrollmentNode{}
	if enrollment.Status.Node != nil {
		node = enrollment.Status.Node.DeepCopy()
	}
	if observation.NodeUID != enrollment.Spec.NodeUID || enrollment.Spec.Retired {
		return reconcile.Result{}, r.otherNode(ctx, enrollment, node, now)
	}

	age := now.Sub(observation.ReportedAt.Time)
	fresh := age >= 0 && age < r.offlineAfter
	node.State = stationv1alpha1.NodeStateOffline
	observed := metav1.Condition{Type: "Observed", Status: metav1.ConditionFalse, Reason: "HeartbeatExpired", Message: "last authenticated observation is outside the freshness window"}
	if fresh {
		node.State = stationv1alpha1.NodeStateBound
		observed = metav1.Condition{Type: "Observed", Status: metav1.ConditionTrue, Reason: "HeartbeatFresh", Message: "a fresh authenticated observation is available"}
	}
	setNodeCondition(node, now, observed)
	reported := observation.ReportedAt.Rfc3339Copy()
	node.LastSeenTime = &reported
	if observation.BootTime != nil {
		// A restart shorter than the heartbeat window never shows as Offline;
		// the boot time is what records it. The margin absorbs the kernel
		// recomputing its boot time when the clock is stepped.
		if before := node.BootTime; before != nil && observation.BootTime.Time.After(before.Time.Add(rebootMargin)) {
			node.Reboots++
			setNodeCondition(node, now, metav1.Condition{Type: "Rebooted", Status: metav1.ConditionTrue, Reason: "BootTimeChanged", Message: fmt.Sprintf("booted at %s; the boot before was at %s",
				observation.BootTime.UTC().Format(time.RFC3339), before.UTC().Format(time.RFC3339))})
		}
		booted := observation.BootTime.Rfc3339Copy()
		node.BootTime = &booted
	}
	setNodeCondition(node, now, metav1.Condition{Type: "IdentityMatched", Status: metav1.ConditionTrue, Reason: "NodeUIDMatched", Message: "the reporting Node is the one this record is of"})
	network := metav1.Condition{Type: "NetworkVerified", Status: metav1.ConditionUnknown, Reason: "NoProbeResults", Message: "no network probes were reported"}
	if len(observation.Probes) > 0 {
		var failed []string
		for _, probe := range observation.Probes {
			if !probe.OK {
				failed = append(failed, probe.Target)
			}
		}
		unreachable, err := r.livePeers(ctx, failed)
		if err != nil {
			return reconcile.Result{}, err
		}
		reached := len(observation.Probes) - len(failed)
		network = metav1.Condition{Type: "NetworkVerified", Status: metav1.ConditionTrue, Reason: "AllLivePeersReached",
			Message: fmt.Sprintf("reached %d of %d peers through the tunnel; the other %d are offline themselves", reached, len(observation.Probes), len(failed))}
		if len(unreachable) > 0 {
			network = metav1.Condition{Type: "NetworkVerified", Status: metav1.ConditionFalse, Reason: "ProbeFailed",
				Message: fmt.Sprintf("reached %d of %d peers through the tunnel; no reply from %d that are online: %s", reached, len(observation.Probes), len(unreachable), strings.Join(unreachable[:min(5, len(unreachable))], ", "))}
		}
	}
	setNodeCondition(node, now, network)
	ready, err := r.nodeReadiness.Ready(ctx, enrollment.Spec.ClusterID, enrollment.Namespace, enrollment.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("read RLark Node readiness: %w", err)
	}
	linked := metav1.Condition{Type: "RLarkLinked", Status: metav1.ConditionFalse, Reason: "NodeNotReady", Message: "the corresponding RLark Node is missing or not Ready"}
	if ready {
		linked = metav1.Condition{Type: "RLarkLinked", Status: metav1.ConditionTrue, Reason: "NodeReady", Message: "the corresponding RLark Node is Ready"}
	}
	setNodeCondition(node, now, linked)
	if err := r.patchNode(ctx, enrollment, node); err != nil {
		return reconcile.Result{}, err
	}
	if fresh {
		return reconcile.Result{RequeueAfter: r.offlineAfter - age}, nil
	}
	return reconcile.Result{}, nil
}

// otherNode answers a report from a Node the record is not of: station-api
// verified the name with another UID, or the record's node was removed
// (retired). A machine an administrator admitted under the name since the
// record's node was last seen (its NodeAdmission was redeemed after) is the
// node the name stands for now, and the record passes to it. A Node no
// admission let in is left to an administrator, who deletes the record.
func (r *NodeReconciler) otherNode(ctx context.Context, enrollment *stationv1alpha1.NetworkEnrollment, node *stationv1alpha1.NetworkEnrollmentNode, now time.Time) error {
	observation, name := enrollment.Status.Observation, enrollment.Name
	admission := &stationv1alpha1.NodeAdmission{}
	err := r.reader.Get(ctx, client.ObjectKeyFromObject(enrollment), admission)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("read NodeAdmission %s: %w", name, err)
	}
	if err != nil || admission.Spec.RedeemedAt == nil || node.LastSeenTime != nil && !admission.Spec.RedeemedAt.After(node.LastSeenTime.Time) {
		node.State = stationv1alpha1.NodeStateRetired
		if observation.NodeUID != enrollment.Spec.NodeUID {
			node.State = stationv1alpha1.NodeStateConflict
			setNodeCondition(node, now, metav1.Condition{Type: "IdentityMatched", Status: metav1.ConditionFalse, Reason: "NodeUIDChanged",
				Message: fmt.Sprintf("Node %s now has UID %s, the record is of UID %s, and no admission let a machine in under the name since: delete this node record in the console and the node registers again as a new machine", name, observation.NodeUID, enrollment.Spec.NodeUID)})
		}
		return r.patchNode(ctx, enrollment, node)
	}
	healed := fmt.Sprintf("节点 %s 移除后又以同一个 Node 入网，场站节点记录已恢复", name)
	if observation.NodeUID != enrollment.Spec.NodeUID {
		healed = fmt.Sprintf("节点 %s 的场站节点记录原属 UID %s 的节点（已不在集群）；这次入网的节点 UID %s，记录已自动转给它", name, enrollment.Spec.NodeUID, observation.NodeUID)
	}
	if admission.Annotations[SelfHealedAnnotation] != healed {
		noted := admission.DeepCopy()
		if noted.Annotations == nil {
			noted.Annotations = map[string]string{}
		}
		noted.Annotations[SelfHealedAnnotation] = healed
		if err := r.client.Patch(ctx, noted, client.MergeFrom(admission)); err != nil {
			return fmt.Errorf("note on NodeAdmission %s what was put right: %w", name, err)
		}
	}
	// The record's block starts over before its spec names the new Node: a
	// pass that stops in between finds the same admission and does it again.
	passed := &stationv1alpha1.NetworkEnrollmentNode{}
	setNodeCondition(passed, now, metav1.Condition{Type: "Replaced", Status: metav1.ConditionTrue, Reason: "AdmittedAgain",
		Message: fmt.Sprintf("Node UID %s took the record over from UID %s by the admission redeemed at %s", observation.NodeUID, enrollment.Spec.NodeUID, admission.Spec.RedeemedAt.UTC().Format(time.RFC3339))})
	if err := r.patchNode(ctx, enrollment, passed); err != nil {
		return err
	}
	// The spec change reconciles the name again, as this Node's.
	base := enrollment.DeepCopy()
	enrollment.Spec.NodeUID, enrollment.Spec.SystemUUID, enrollment.Spec.Retired = observation.NodeUID, observation.SystemUUID, false
	if err := r.client.Patch(ctx, enrollment, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("pass NetworkEnrollment %s to Node UID %s: %w", name, observation.NodeUID, err)
	}
	return nil
}

func (r *NodeReconciler) patchNode(ctx context.Context, enrollment *stationv1alpha1.NetworkEnrollment, node *stationv1alpha1.NetworkEnrollmentNode) error {
	if reflect.DeepEqual(enrollment.Status.Node, node) {
		return nil
	}
	base := enrollment.DeepCopy()
	enrollment.Status.Node = node
	if err := r.client.Status().Patch(ctx, enrollment, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("patch the node status of NetworkEnrollment %s: %w", enrollment.Name, err)
	}
	return nil
}

func setNodeCondition(node *stationv1alpha1.NetworkEnrollmentNode, now time.Time, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.NewTime(now)
	apimeta.SetStatusCondition(&node.Conditions, condition)
}

// nodeState is the state the node reconciler gave an enrollment's node.
func nodeState(enrollment *stationv1alpha1.NetworkEnrollment) stationv1alpha1.NodeState {
	if enrollment.Status.Node == nil {
		return ""
	}
	return enrollment.Status.Node.State
}

// livePeers returns the probe targets that failed although the peer is online
// by its own heartbeat. A node is not unverified because a peer is switched
// off, or has not come up yet (a machine whose admission put it in the
// tunnel before it reported).
func (r *NodeReconciler) livePeers(ctx context.Context, failed []string) ([]string, error) {
	if len(failed) == 0 {
		return nil, nil
	}
	var enrollments stationv1alpha1.NetworkEnrollmentList
	if err := r.client.List(ctx, &enrollments); err != nil {
		return nil, fmt.Errorf("list NetworkEnrollments for probe targets: %w", err)
	}
	online := map[string]bool{}
	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		if enrollment.Status.TunnelAddress != "" && nodeState(enrollment) == stationv1alpha1.NodeStateBound {
			online[enrollment.Status.TunnelAddress] = true
		}
	}
	var live []string
	for _, target := range failed {
		if online[target] {
			live = append(live, target)
		}
	}
	return live, nil
}
