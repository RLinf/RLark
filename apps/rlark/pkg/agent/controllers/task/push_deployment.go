package task

import (
	"context"
	"fmt"
	"reflect"
	"sort"

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

	if !pushOwnsTask(&mgmtTask, r.c.AgentType, &deploy, rlarkv1alpha1.KubernetesWorkloadDeployment) {
		logger.Info("Deployment no longer owns management Task status, skipping")
		return reconcile.Result{}, nil
	}

	phase, message, pods, err := deploymentPhase(ctx, r.c.LocalKubeClient, &deploy)
	if err != nil {
		return reconcile.Result{}, err
	}
	observedNodes := podNodeNames(pods)
	// pullProgress aggregation is now performed by the control-plane Task
	// reconciler from Node.status.pullProgress, so the cluster-agent no
	// longer reads back Node.status.pullProgress here.
	return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, phase, message, observedNodes)
}

func deploymentPhase(ctx context.Context, localClient client.Client, deploy *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string, []corev1.Pod, error) {
	phase, message := deploymentStatusPhase(deploy)
	pods, err := listTaskPods(ctx, localClient, deploy, deploy.Spec.Selector.MatchLabels)
	if err != nil {
		return "", "", nil, err
	}
	// Override to Failed when any pod container is in an abnormal state
	// (CrashLoopBackOff, ImagePullBackOff, OOMKilled, etc.) so operators
	// see the failure immediately instead of a misleading Running/Pending.
	if podMsg, found := podFailureMessage(pods); found && phase != rlarkv1alpha1.TaskPhaseSucceeded {
		phase = rlarkv1alpha1.TaskPhaseFailed
		message = podMsg
	}
	if phase == rlarkv1alpha1.TaskPhasePending {
		if found, err := hasFailedSchedulingEvent(ctx, localClient, deploy.Namespace, pods); err != nil {
			return "", "", nil, err
		} else if found {
			message = "FailedScheduling"
		}
	}
	return phase, message, pods, nil
}

func deploymentStatusPhase(deploy *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string) {
	desired := computeDesiredReplicas(deploy.Spec.Replicas)
	if desired == 0 {
		return rlarkv1alpha1.TaskPhaseStopped, ""
	}
	if deploy.Status.ObservedGeneration < deploy.Generation {
		return rlarkv1alpha1.TaskPhasePending, ""
	}
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse && cond.Reason == "ProgressDeadlineExceeded" {
			return rlarkv1alpha1.TaskPhaseFailed, cond.Message
		}
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			return rlarkv1alpha1.TaskPhaseFailed, cond.Message
		}
	}
	if deploy.Status.UpdatedReplicas == desired && deploy.Status.ReadyReplicas == desired &&
		deploy.Status.AvailableReplicas == desired && deploy.Status.Replicas == desired {
		return rlarkv1alpha1.TaskPhaseRunning, ""
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

func hasFailedSchedulingEvent(ctx context.Context, localClient client.Client, namespace string, pods []corev1.Pod) (bool, error) {
	if len(pods) == 0 {
		return false, nil
	}
	podNames := make(map[string]struct{}, len(pods))
	for i := range pods {
		podNames[pods[i].Name] = struct{}{}
	}

	var events corev1.EventList
	if err := localClient.List(ctx, &events, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, event := range events.Items {
		if event.InvolvedObject.Kind != "Pod" || event.Reason != "FailedScheduling" {
			continue
		}
		if _, ok := podNames[event.InvolvedObject.Name]; ok && (event.InvolvedObject.UID == "" || event.InvolvedObject.UID == podUID(pods, event.InvolvedObject.Name)) {
			return true, nil
		}
	}
	return false, nil
}

func podUID(pods []corev1.Pod, name string) types.UID {
	for i := range pods {
		if pods[i].Name == name {
			return pods[i].UID
		}
	}
	return ""
}

// --- shared helper functions for push reconcilers ---

// listTaskPods lists the local pods backing a workload via its selector labels.
func listTaskPods(ctx context.Context, localClient client.Client, workload client.Object, labels map[string]string) ([]corev1.Pod, error) {
	var podList corev1.PodList
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: labels})
	if err != nil {
		return nil, fmt.Errorf("failed to build label selector: %w", err)
	}

	if err := localClient.List(ctx, &podList, client.InNamespace(workload.GetNamespace()), client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, fmt.Errorf("failed to list Pods: %w", err)
	}
	pods := podList.Items[:0]
	for i := range podList.Items {
		pod := &podList.Items[i]
		owner := metav1.GetControllerOf(pod)
		if owner == nil {
			continue
		}
		if owner.Kind == workloadKind(workload) && owner.Name == workload.GetName() && owner.UID == workload.GetUID() {
			pods = append(pods, *pod)
			continue
		}
		if _, ok := workload.(*appsv1.Deployment); !ok || owner.Kind != "ReplicaSet" {
			continue
		}
		var rs appsv1.ReplicaSet
		if err := localClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}, &rs); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return nil, err
			}
			continue
		}
		if rs.UID != owner.UID {
			continue
		}
		depOwner := metav1.GetControllerOf(&rs)
		if depOwner != nil && depOwner.Kind == "Deployment" && depOwner.Name == workload.GetName() && depOwner.UID == workload.GetUID() {
			pods = append(pods, *pod)
		}
	}
	return pods, nil
}

func workloadKind(obj client.Object) string {
	switch obj.(type) {
	case *appsv1.Deployment:
		return "Deployment"
	case *appsv1.StatefulSet:
		return "StatefulSet"
	case *appsv1.DaemonSet:
		return "DaemonSet"
	default:
		return ""
	}
}

func pushOwnsTask(task *rlarkv1alpha1.Task, agentType string, workload client.Object, kind rlarkv1alpha1.KubernetesWorkloadKind) bool {
	return task.Spec.AgentType == rlarkv1alpha1.AgentType(agentType) && workloadOwnershipForTask(workload, task) != workloadConflict &&
		task.Spec.Kubernetes != nil && task.Spec.Kubernetes.Workload != nil && task.Spec.Kubernetes.Workload.Kind == kind
}

// podNodeNames returns the node names where the given pods are scheduled.
func podNodeNames(pods []corev1.Pod) []string {
	nodes := make([]string, 0, len(pods))
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			nodes = append(nodes, pod.Spec.NodeName)
		}
	}
	sort.Strings(nodes)
	nodes = slicesCompact(nodes)
	return nodes
}

func slicesCompact(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// updateMgmtTaskStatus reports the workload phase/message/observedNodes to the
// management Task status. Pull progress and events are aggregated by the
// control-plane Task reconciler, and are cleared when the task stops.
//
// A strategic merge patch (client.MergeFrom) is used instead of Update so we
// only send the fields we own (phase, message, observedNodes, and terminal
// cleanup fields), preserving concurrent status writes.
func updateMgmtTaskStatus(ctx context.Context, logger logr.Logger, mgmtClient client.Client, mgmtTask *rlarkv1alpha1.Task, phase rlarkv1alpha1.TaskPhase, message string, observedNodes []string) (reconcile.Result, error) {
	stopped := phase == rlarkv1alpha1.TaskPhaseStopped
	unchanged := mgmtTask.Status.Phase == phase && mgmtTask.Status.Message == message &&
		reflect.DeepEqual(mgmtTask.Status.ObservedNodes, observedNodes) &&
		(!stopped || (len(mgmtTask.Status.PullProgress) == 0 && len(mgmtTask.Status.Events) == 0))

	if unchanged {
		logger.V(1).Info("management Task status unchanged, skipping")
		return reconcile.Result{}, nil
	}

	original := mgmtTask.DeepCopy()
	mgmtTask.Status.Phase = phase
	mgmtTask.Status.Message = message
	if stopped {
		mgmtTask.Status.PullProgress = nil
		mgmtTask.Status.Events = nil
	}
	mgmtTask.Status.ObservedNodes = observedNodes

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
