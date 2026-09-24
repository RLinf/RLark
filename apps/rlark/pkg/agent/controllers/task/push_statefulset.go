package task

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// pushStatefulSetReconciler watches local StatefulSets and reports status to management Task.
type pushStatefulSetReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pushStatefulSetReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("statefulset", req.NamespacedName)

	var sts appsv1.StatefulSet
	if err := r.c.LocalKubeClient.Get(ctx, req.NamespacedName, &sts); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		logger.V(1).Info("StatefulSet not found")
		return reconcile.Result{}, nil
	}

	taskName := sts.Annotations[ManagementTaskNameAnnotation]
	taskNamespace := sts.Annotations[ManagementTaskNamespaceAnnotation]
	if taskName == "" || taskNamespace == "" {
		logger.V(1).Info("StatefulSet has no management-task annotation, skipping")
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

	if !pushOwnsTask(&mgmtTask, r.c.AgentType, &sts, rlarkv1alpha1.KubernetesWorkloadStatefulSet) {
		logger.Info("StatefulSet no longer owns management Task status, skipping")
		return reconcile.Result{}, nil
	}

	phase, message, pods, err := statefulSetPhase(ctx, r.c.LocalKubeClient, &sts)
	if err != nil {
		return reconcile.Result{}, err
	}
	observedNodes := podNodeNames(pods)
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func statefulSetPhase(ctx context.Context, localClient client.Client, sts *appsv1.StatefulSet) (rlarkv1alpha1.TaskPhase, string, []corev1.Pod, error) {
	desired := computeDesiredReplicas(sts.Spec.Replicas)
	var phase rlarkv1alpha1.TaskPhase
	switch {
	case desired == 0:
		phase = rlarkv1alpha1.TaskPhaseStopped
	case sts.Status.ObservedGeneration >= sts.Generation && sts.Status.UpdatedReplicas == desired &&
		sts.Status.ReadyReplicas == desired && sts.Status.CurrentReplicas == desired &&
		sts.Status.CurrentRevision != "" && sts.Status.CurrentRevision == sts.Status.UpdateRevision:
		phase = rlarkv1alpha1.TaskPhaseRunning
	default:
		phase = rlarkv1alpha1.TaskPhasePending
	}

	pods, err := listTaskPods(ctx, localClient, sts, sts.Spec.Selector.MatchLabels)
	if err != nil {
		return "", "", nil, err
	}
	// Override to Failed when any pod container is in an abnormal state
	// (CrashLoopBackOff, ImagePullBackOff, OOMKilled, etc.) so operators
	// see the failure immediately instead of a misleading Running/Pending.
	var message string
	if podMsg, found := podFailureMessage(pods); found && phase != rlarkv1alpha1.TaskPhaseSucceeded {
		phase = rlarkv1alpha1.TaskPhaseFailed
		message = podMsg
	}
	if phase == rlarkv1alpha1.TaskPhasePending {
		if found, err := hasFailedSchedulingEvent(ctx, localClient, sts.Namespace, pods); err != nil {
			return "", "", nil, err
		} else if found {
			message = "FailedScheduling"
		}
	}
	return phase, message, pods, nil
}
