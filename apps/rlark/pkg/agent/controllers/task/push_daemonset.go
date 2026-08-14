package task

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// pushDaemonSetReconciler watches local DaemonSets and reports status to management Task.
type pushDaemonSetReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pushDaemonSetReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("daemonset", req.NamespacedName)

	var ds appsv1.DaemonSet
	if err := r.c.LocalKubeClient.Get(ctx, req.NamespacedName, &ds); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		logger.V(1).Info("DaemonSet not found")
		return reconcile.Result{}, nil
	}

	taskName := ds.Annotations[ManagementTaskNameAnnotation]
	taskNamespace := ds.Annotations[ManagementTaskNamespaceAnnotation]
	taskUID := ds.Annotations[ManagementTaskUIDAnnotation]

	if taskName == "" || taskNamespace == "" {
		logger.V(1).Info("DaemonSet has no management-task annotation, skipping")
		return reconcile.Result{}, nil
	}

	var mgmtTask rlarkv1alpha1.Task
	if err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &mgmtTask); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to get management Task")
			return reconcile.Result{}, err
		}
		logger.Info("management Task not found, skipping")
		return reconcile.Result{}, nil
	}

	if mgmtTask.Spec.AgentType != rlarkv1alpha1.AgentType(r.c.AgentType) {
		logger.Info(fmt.Sprintf("Task AgentType %s does not match controller AgentType %s, skipping", mgmtTask.Spec.AgentType, r.c.AgentType))
		return reconcile.Result{}, nil
	}

	if string(mgmtTask.UID) != taskUID {
		logger.Info("management Task UID mismatch with annotation, skipping")
		return reconcile.Result{}, nil
	}

	phase, message := daemonSetPhase(&ds)
	observedNodes, err := collectObservedNode(ctx, r.c.LocalKubeClient, ds.Namespace, ds.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "failed to collect observed nodes")
		observedNodes = nil
	}
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func daemonSetPhase(ds *appsv1.DaemonSet) (rlarkv1alpha1.TaskPhase, string) {
	if ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
		return rlarkv1alpha1.TaskPhaseRunning, ""
	}
	if ds.Status.NumberUnavailable > 0 {
		return rlarkv1alpha1.TaskPhaseFailed, "daemonset pods unavailable"
	}
	return rlarkv1alpha1.TaskPhasePending, ""
}
