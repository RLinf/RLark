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
	taskUID := sts.Annotations[ManagementTaskUIDAnnotation]

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

	if mgmtTask.Spec.AgentType != rlarkv1alpha1.AgentType(r.c.AgentType) {
		logger.Info(fmt.Sprintf("Task AgentType %s does not match controller AgentType %s, skipping", mgmtTask.Spec.AgentType, r.c.AgentType))
		return reconcile.Result{}, nil
	}

	if string(mgmtTask.UID) != taskUID {
		logger.Info("management Task UID mismatch with annotation, skipping")
		return reconcile.Result{}, nil
	}

	phase, message := statefulSetPhase(&sts)
	observedNodes, err := collectObservedNode(ctx, r.c.LocalKubeClient, sts.Namespace, sts.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "failed to collect observed nodes")
		observedNodes = nil
	}
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func statefulSetPhase(sts *appsv1.StatefulSet) (rlarkv1alpha1.TaskPhase, string) {
	desired := computeDesiredReplicas(sts.Spec.Replicas)
	if desired == 0 {
		return rlarkv1alpha1.TaskPhaseStopped, ""
	}
	if sts.Status.ReadyReplicas >= desired {
		return rlarkv1alpha1.TaskPhaseRunning, ""
	}
	return rlarkv1alpha1.TaskPhasePending, ""
}
