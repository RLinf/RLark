package task

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
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

	if !pushOwnsTask(&mgmtTask, r.c.AgentType, &sts, raykubernetes.CarrierStatefulSet) {
		logger.Info("StatefulSet no longer owns management Task status, skipping")
		return reconcile.Result{}, nil
	}

	if err := r.c.emitRuntimeTrigger(ctx, &mgmtTask, kubernetesWorkloadRef(&mgmtTask, &sts, raykubernetes.CarrierStatefulSet)); err != nil {
		logger.Error(err, "failed to emit Task Runtime trigger")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
