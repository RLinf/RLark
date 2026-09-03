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

	phase, message, pods := deploymentPhase(ctx, logger, r.c.LocalKubeClient, &deploy)
	observedNodes := podNodeNames(pods)
	// pullProgress aggregation is now performed by the control-plane Task
	// reconciler from Node.status.pullProgress, so the cluster-agent no
	// longer reads back Node.status.pullProgress here.
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func deploymentPhase(ctx context.Context, logger logr.Logger, localClient client.Client, deploy *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string, []corev1.Pod) {
	phase, message := deploymentStatusPhase(deploy)
	pods, err := listTaskPods(ctx, localClient, deploy.Namespace, deploy.Spec.Selector.MatchLabels)
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

func deploymentStatusPhase(deploy *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string) {
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

// abnormalContainerReasons lists Pod container waiting/terminated reasons that
// indicate the workload will not reach Running on its own. When any container
// of any pod backing a Task is in one of these states, the Task is marked Failed
// so operators see the problem immediately instead of an indefinite Pending or
// a misleading Running (a Pod can be phase=Running while a container is in
// CrashLoopBackOff).
var abnormalContainerReasons = map[string]struct{}{
	// Waiting states — container cannot start or is stuck in a restart loop.
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"InvalidImageName":           {},
	"CreateContainerConfigError": {},
	"CreateContainerError":       {},
	"CrashLoopBackOff":           {},
	// Terminated states — container exited abnormally.
	"OOMKilled":          {},
	"ContainerCannotRun": {},
	"DeadlineExceeded":   {},
}

// podFailureMessage inspects pods backing a Task for container states that
// indicate a terminal failure (CrashLoopBackOff, ImagePullBackOff, OOMKilled,
// etc.). If any such state is found it returns a human-readable message and
// true; otherwise it returns "", false.
func podFailureMessage(pods []corev1.Pod) (string, bool) {
	for i := range pods {
		pod := &pods[i]
		for _, cs := range pod.Status.ContainerStatuses {
			if waiting := cs.State.Waiting; waiting != nil {
				if _, ok := abnormalContainerReasons[waiting.Reason]; ok {
					detail := waiting.Message
					if detail == "" {
						detail = waiting.Reason
					}
					return fmt.Sprintf("pod %s container %s: %s", pod.Name, cs.Name, detail), true
				}
			}
			if terminated := cs.State.Terminated; terminated != nil {
				if _, ok := abnormalContainerReasons[terminated.Reason]; ok {
					detail := terminated.Message
					if detail == "" {
						detail = terminated.Reason
					}
					return fmt.Sprintf("pod %s container %s: %s", pod.Name, cs.Name, detail), true
				}
			}
		}
	}
	return "", false
}

// --- shared helper functions for push reconcilers ---

// listTaskPods lists the local pods backing a workload via its selector labels.
func listTaskPods(ctx context.Context, localClient client.Client, namespace string, labels map[string]string) ([]corev1.Pod, error) {
	var podList corev1.PodList
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: labels})
	if err != nil {
		return nil, fmt.Errorf("failed to build label selector: %w", err)
	}

	if err := localClient.List(ctx, &podList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, fmt.Errorf("failed to list Pods: %w", err)
	}
	return podList.Items, nil
}

// podNodeNames returns the node names where the given pods are scheduled.
func podNodeNames(pods []corev1.Pod) []string {
	nodes := make([]string, 0, len(pods))
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			nodes = append(nodes, pod.Spec.NodeName)
		}
	}
	return nodes
}

// updateMgmtTaskStatus reports the workload phase/message/observedNodes to the
// management Task status. It does not touch pullProgress: that field is now
// aggregated by the control-plane Task reconciler from Node.status.pullProgress.
//
// A strategic merge patch (client.MergeFrom) is used instead of Update so we
// only send the fields we own (phase, message, observedNodes). Concurrent
// writes to Task.status.pullProgress by the control-plane reconciler are
// preserved.
func updateMgmtTaskStatus(ctx context.Context, logger logr.Logger, mgmtClient client.Client, mgmtTask *rlarkv1alpha1.Task, phase rlarkv1alpha1.TaskPhase, message string, observedNodes []string) (reconcile.Result, error) {
	unchanged := mgmtTask.Status.Phase == phase && mgmtTask.Status.Message == message

	if unchanged {
		logger.V(1).Info("management Task status unchanged, skipping")
		return reconcile.Result{}, nil
	}

	original := mgmtTask.DeepCopy()
	mgmtTask.Status.Phase = phase
	mgmtTask.Status.Message = message
	if len(observedNodes) > 0 {
		mgmtTask.Status.ObservedNodes = observedNodes
	}

	if err := mgmtClient.Status().Patch(ctx, mgmtTask, client.MergeFrom(original)); err != nil {
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
