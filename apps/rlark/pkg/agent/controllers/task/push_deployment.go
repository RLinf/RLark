package task

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
)

// pushDeploymentReconciler watches local Deployments and reports status to management Task.
type pushDeploymentReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pushDeploymentReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("deployment", req.NamespacedName)

	var deploy appsv1.Deployment
	if err := r.c.LocalKubeClient.Get(ctx, req.NamespacedName, &deploy); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		logger.V(1).Info("Deployment not found")
		return reconcile.Result{}, nil
	}

	taskName := deploy.Annotations[ManagementTaskNameAnnotation]
	taskNamespace := deploy.Annotations[ManagementTaskNamespaceAnnotation]

	if taskName == "" || taskNamespace == "" {
		logger.V(1).Info("Deployment has no management-task annotation, skipping")
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

	if !pushOwnsTask(&mgmtTask, r.c.AgentType, &deploy, raykubernetes.CarrierDeployment) {
		logger.Info("Deployment no longer owns management Task status, skipping")
		return reconcile.Result{}, nil
	}

	if err := r.c.emitRuntimeTrigger(ctx, &mgmtTask, kubernetesWorkloadRef(&mgmtTask, &deploy, raykubernetes.CarrierDeployment)); err != nil {
		logger.Error(err, "failed to emit Task Runtime trigger")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func pushOwnsTask(task *rlarkv1alpha1.Task, agentType string, workload client.Object, kind string) bool {
	carrierType := taskapi.CarrierTypeKubernetesDeployment
	switch kind {
	case raykubernetes.CarrierDaemonSet:
		carrierType = taskapi.CarrierTypeKubernetesDaemonSet
	case raykubernetes.CarrierStatefulSet:
		carrierType = taskapi.CarrierTypeKubernetesStatefulSet
	}
	return string(task.Spec.Runtime.Kind) == agentType && kubernetesobjects.OwnershipForTask(workload, task) != kubernetesobjects.OwnershipConflict &&
		task.Spec.Workload.Carrier.Type == carrierType
}
