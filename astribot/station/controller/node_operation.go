package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NodeOperationExecutor must observe the requested effect before taking action.
// It returns done only after the effect has been independently verified.
type NodeOperationExecutor interface {
	Execute(context.Context, *stationv1alpha1.NodeOperation, string) (done bool, nodeUID string, err error)
}

type NodeOperationReconciler struct {
	client        client.Client
	reader        client.Reader
	registrations ClusterResolver
	executor      NodeOperationExecutor
	// Concurrency is how many operations run at a time: each is its own machine.
	Concurrency int
}

func NewNodeOperationReconciler(writer client.Client, reader client.Reader, registrations ClusterResolver, executor NodeOperationExecutor) *NodeOperationReconciler {
	return &NodeOperationReconciler{client: writer, reader: reader, registrations: registrations, executor: executor, Concurrency: 1}
}

var nodeOperationStages = map[string][]string{
	"Join":   installerStages,
	"Remove": {"Preflight", "PowerOff", "Drain", "RemovePeer", "Reset", "DeleteNode", "VerifyRemoval"},
}

func (r *NodeOperationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	op := &stationv1alpha1.NodeOperation{}
	if err := r.reader.Get(ctx, req.NamespacedName, op); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read node operation: %w", err)
	}
	if op.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	// Blocked is terminal: the stage stopped with a recorded reason. An
	// administrator retries by asking for a new operation (a new request ID),
	// which starts at Preflight and cannot race this one. Retrying here would
	// let a second operation run concurrently.
	if op.Status.Phase == "Succeeded" || op.Status.Phase == "Failed" || op.Status.Phase == "Blocked" {
		return reconcile.Result{}, nil
	}
	registration, err := r.registrations.ResolveNamespace(ctx, op.Namespace)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve cluster: %w", err)
	}
	if registration.ClusterID != op.Spec.ClusterID {
		return reconcile.Result{}, ErrInvalidRegistration
	}
	stages, ok := nodeOperationStages[op.Spec.Action]
	if !ok {
		return reconcile.Result{}, errors.New("unsupported node operation action")
	}
	next := 0
	if op.Status.Stage != "" {
		found := false
		for i, stage := range stages {
			if stage == op.Status.Stage {
				next, found = i+1, true
				break
			}
		}
		if !found {
			return reconcile.Result{}, errors.New("unknown node operation stage")
		}
	}
	if next == len(stages) {
		updated := op.DeepCopy()
		updated.Status.Phase = "Succeeded"
		updated.Status.ObservedGeneration = op.Generation
		now := metav1.Now()
		updated.Status.CompletedAt = &now
		if err := r.client.Status().Patch(ctx, updated, client.MergeFromWithOptions(op, client.MergeFromWithOptimisticLock{})); err != nil {
			return reconcile.Result{}, fmt.Errorf("complete node operation: %w", err)
		}
		return reconcile.Result{}, nil
	}
	// A removal named by the node's name alone is of the Node its Preflight
	// found, or, when it found none, of what RLark keeps of the name.
	target := op.DeepCopy()
	if target.Spec.Action == "Remove" && target.Spec.NodeUID == "" {
		target.Spec.NodeUID = op.Status.NodeUID
	}
	done, uid, err := r.executor.Execute(ctx, target, stages[next])
	if err != nil {
		if ctx.Err() != nil {
			return reconcile.Result{}, ctx.Err()
		}
		updated := op.DeepCopy()
		updated.Status.Phase = "Blocked"
		updated.Status.ObservedGeneration = op.Generation
		reason, message := nodeOperationFailureCondition(stages[next], err)
		apimeta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{Type: "Progressing", Status: metav1.ConditionFalse, Reason: reason, Message: message, ObservedGeneration: op.Generation})
		if patchErr := r.client.Status().Patch(ctx, updated, client.MergeFromWithOptions(op, client.MergeFromWithOptimisticLock{})); patchErr != nil {
			return reconcile.Result{}, fmt.Errorf("record node operation failure: %w", patchErr)
		}
		return reconcile.Result{}, fmt.Errorf("node operation %s: %w", stages[next], err)
	}
	if !done {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if op.Spec.NodeUID != "" && uid != "" && op.Spec.NodeUID != uid {
		return reconcile.Result{}, ErrNodeIdentityChanged
	}
	if op.Status.NodeUID != "" && uid != "" && op.Status.NodeUID != uid {
		return reconcile.Result{}, ErrNodeIdentityChanged
	}
	updated := op.DeepCopy()
	updated.Status.Phase = "Running"
	updated.Status.Stage = stages[next]
	updated.Status.ObservedGeneration = op.Generation
	message := "Verified node operation stage " + stages[next]
	if target.Spec.Action == "Remove" && target.Spec.NodeUID == "" && uid == "" {
		message += ": the node is not in its cluster; what RLark keeps of " + op.Spec.NodeName + " is cleared"
	}
	apimeta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{Type: "Progressing", Status: metav1.ConditionTrue, Reason: "StageCompleted", Message: message, ObservedGeneration: op.Generation})
	if uid != "" {
		updated.Status.NodeUID = uid
	}
	if err := r.client.Status().Patch(ctx, updated, client.MergeFromWithOptions(op, client.MergeFromWithOptimisticLock{})); err != nil {
		return reconcile.Result{}, fmt.Errorf("record node operation stage: %w", err)
	}
	return reconcile.Result{Requeue: true}, nil
}

// removalStageUsesMemberAPI reports whether a removal stage waits on the
// member cluster API. A Join's own stages use SSH instead.
func removalStageUsesMemberAPI(stage string) bool {
	switch stage {
	case "Preflight", "PowerOff", "Drain", "RemovePeer", "Reset", "DeleteNode", "VerifyRemoval":
		return true
	default:
		return false
	}
}

// memberClusterUnreachable separates a member API that cannot be reached from
// a stage that failed for another reason. Without this distinction the
// operation message would send an administrator to controller logs while the
// real cause is an offline cluster or an API endpoint on the node itself.
func memberClusterUnreachable(stage string, err error) bool {
	if !removalStageUsesMemberAPI(stage) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func nodeOperationFailureCondition(stage string, err error) (string, string) {
	if problem := machineProblem(err); problem != "" {
		return problem, "Node operation stage " + stage + " could not log in to the machine: " + err.Error()
	}
	if memberClusterUnreachable(stage, err) {
		return "MemberClusterUnreachable", "Node operation stage " + stage + " could not reach the member cluster API before the timeout. The cluster, its API endpoint, or its credentials may be offline; retry once the cluster is reachable."
	}
	switch {
	case errors.Is(err, ErrInstallerFailed):
		return "InstallerFailed", err.Error()
	case errors.Is(err, ErrNodeIdentityChanged):
		return "NodeIdentityChanged", "Node UID changed or the target Node is deleting; no different Node was modified. If the node's record shows a conflict, delete the record and the node registers again."
	case errors.Is(err, ErrNodeDrainBlocked):
		return "DrainBlocked", "Drain is blocked by a protected Node, workload, local emptyDir data or a disruption budget."
	case errors.Is(err, ErrNodeStageUnavailable):
		return "PrerequisiteUnavailable", "A required node-operation integration or permission is not configured."
	case errors.Is(err, ErrNodeInUse):
		return "NodeInUse", err.Error() + ". Remove it once that Run or session has ended."
	case errors.Is(err, ErrNodeDeclaredInSiteNetwork):
		return "DeclaredInSiteNetwork", err.Error()
	case errors.Is(err, ErrNodeAlreadyConfigured):
		return "NodeAlreadyConfigured", "The target already has Kubernetes configuration; automatic adoption is not allowed."
	default:
		return "StageFailed", "Node operation stage " + stage + " failed; inspect controller logs and current Node state before retrying."
	}
}

func (r *NodeOperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&stationv1alpha1.NodeOperation{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.Concurrency}).Complete(r)
}
