package task

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
	taskUID := deploy.Annotations[ManagementTaskUIDAnnotation]

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

	// AgentType check
	if mgmtTask.Spec.AgentType != rlarkv1alpha1.AgentType(r.c.AgentType) {
		logger.Info(fmt.Sprintf("Task AgentType %s does not match controller AgentType %s, skipping", mgmtTask.Spec.AgentType, r.c.AgentType))
		return reconcile.Result{}, nil
	}

	// UID match check
	if string(mgmtTask.UID) != taskUID {
		logger.Info("management Task UID mismatch with annotation, skipping")
		return reconcile.Result{}, nil
	}

	phase, message := deploymentPhase(&deploy)
	observedNodes, err := collectObservedNode(ctx, r.c.LocalKubeClient, deploy.Namespace, deploy.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "failed to collect observed nodes")
		observedNodes = nil
	}
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func deploymentPhase(deploy *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string) {
	desired := computeDesiredReplicas(deploy.Spec.Replicas)
	if desired == 0 {
		return rlarkv1alpha1.TaskPhaseStopped, ""
	}
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			return rlarkv1alpha1.TaskPhaseFailed, cond.Message
		}
	}
	if deploy.Status.ReadyReplicas >= desired && deploy.Status.Replicas >= desired {
		return rlarkv1alpha1.TaskPhaseRunning, ""
	}
	if deploy.Status.UnavailableReplicas > 0 {
		return rlarkv1alpha1.TaskPhaseFailed, fmt.Sprintf("deployment replicas unavailable %d", deploy.Status.UnavailableReplicas)
	}
	return rlarkv1alpha1.TaskPhasePending, ""
}

// --- shared helper functions for push reconcilers ---

func collectObservedNode(ctx context.Context, localClient client.Client, namespace string, labels map[string]string) ([]string, error) {
	var podList corev1.PodList
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: labels})
	if err != nil {
		return nil, fmt.Errorf("failed to build label selector: %w", err)
	}

	if err := localClient.List(ctx, &podList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, fmt.Errorf("failed to list Pods: %w", err)
	}

	nodes := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != "" {
			nodes = append(nodes, pod.Spec.NodeName)
		}
	}
	return nodes, nil
}

func updateMgmtTaskStatus(ctx context.Context, logger logr.Logger, mgmtClient client.Client, mgmtTask *rlarkv1alpha1.Task, phase rlarkv1alpha1.TaskPhase, message string, observedNodes []string) (reconcile.Result, error) {
	if mgmtTask.Status.Phase == phase && mgmtTask.Status.Message == message {
		logger.V(1).Info("management Task status unchanged, skipping")
		return reconcile.Result{}, nil
	}

	mgmtTask.Status.Phase = phase
	mgmtTask.Status.Message = message
	if len(observedNodes) > 0 {
		mgmtTask.Status.ObservedNodes = observedNodes
	}

	if err := mgmtClient.Status().Update(ctx, mgmtTask); err != nil {
		logger.Error(err, "failed to report Task status to management cluster")
		return reconcile.Result{}, err
	}

	logger.Info(fmt.Sprintf("reported Task status: phase=%s message=%s", phase, message))
	return reconcile.Result{}, nil
}

func computeDesiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}
