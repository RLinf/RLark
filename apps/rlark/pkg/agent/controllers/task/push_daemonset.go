package task

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	phase, message, pods := daemonSetPhase(ctx, logger, r.c.LocalKubeClient, &ds)
	observedNodes := podNodeNames(pods)
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func daemonSetPhase(ctx context.Context, logger logr.Logger, localClient client.Client, ds *appsv1.DaemonSet) (rlarkv1alpha1.TaskPhase, string, []corev1.Pod) {
	var phase rlarkv1alpha1.TaskPhase
	var message string
	switch {
	case ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0:
		phase = rlarkv1alpha1.TaskPhaseRunning
	case ds.Status.NumberUnavailable > 0:
		phase = rlarkv1alpha1.TaskPhaseFailed
		message = "daemonset pods unavailable"
	default:
		phase = rlarkv1alpha1.TaskPhasePending
	}

	pods, err := listTaskPods(ctx, localClient, ds.Namespace, ds.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "failed to list pods")
	}
	// Override to Failed when any pod container is in an abnormal state
	// (CrashLoopBackOff, ImagePullBackOff, OOMKilled, etc.) so operators
	// see the failure immediately instead of a misleading Running/Pending.
	if podMsg, found := podFailureMessage(pods); found && phase != rlarkv1alpha1.TaskPhaseSucceeded {
		phase = rlarkv1alpha1.TaskPhaseFailed
		message = podMsg
	}
	return phase, message, pods
}
