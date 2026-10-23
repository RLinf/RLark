package kubernetes

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// DeploymentStatusPhase projects a Deployment's current generation and conditions.
func DeploymentStatusPhase(deployment *appsv1.Deployment) (rlarkv1alpha1.TaskPhase, string) {
	desired := desiredReplicas(deployment.Spec.Replicas)
	if desired == 0 {
		return rlarkv1alpha1.TaskPhaseStopped, ""
	}
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return rlarkv1alpha1.TaskPhasePending, ""
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
			return rlarkv1alpha1.TaskPhaseFailed, condition.Message
		}
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
			return rlarkv1alpha1.TaskPhaseFailed, condition.Message
		}
	}
	if deployment.Status.UpdatedReplicas == desired && deployment.Status.ReadyReplicas == desired && deployment.Status.AvailableReplicas == desired && deployment.Status.Replicas == desired {
		return rlarkv1alpha1.TaskPhaseRunning, ""
	}
	return rlarkv1alpha1.TaskPhasePending, ""
}

// DaemonSetStatusPhase projects a DaemonSet's current generation and replica counts.
func DaemonSetStatusPhase(daemonSet *appsv1.DaemonSet) rlarkv1alpha1.TaskPhase {
	if daemonSet.Status.ObservedGeneration >= daemonSet.Generation && daemonSet.Status.DesiredNumberScheduled > 0 && daemonSet.Status.UpdatedNumberScheduled == daemonSet.Status.DesiredNumberScheduled && daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled && daemonSet.Status.NumberAvailable == daemonSet.Status.DesiredNumberScheduled {
		return rlarkv1alpha1.TaskPhaseRunning
	}
	return rlarkv1alpha1.TaskPhasePending
}

// StatefulSetStatusPhase projects a StatefulSet's current revision and replica counts.
func StatefulSetStatusPhase(statefulSet *appsv1.StatefulSet) rlarkv1alpha1.TaskPhase {
	desired := desiredReplicas(statefulSet.Spec.Replicas)
	if desired == 0 {
		return rlarkv1alpha1.TaskPhaseStopped
	}
	if statefulSet.Status.ObservedGeneration >= statefulSet.Generation && statefulSet.Status.UpdatedReplicas == desired && statefulSet.Status.ReadyReplicas == desired && statefulSet.Status.CurrentReplicas == desired && statefulSet.Status.CurrentRevision != "" && statefulSet.Status.CurrentRevision == statefulSet.Status.UpdateRevision {
		return rlarkv1alpha1.TaskPhaseRunning
	}
	return rlarkv1alpha1.TaskPhasePending
}

var abnormalReasons = map[string]struct{}{"ImagePullBackOff": {}, "ErrImagePull": {}, "InvalidImageName": {}, "CreateContainerConfigError": {}, "CreateContainerError": {}, "CrashLoopBackOff": {}, "OOMKilled": {}, "ContainerCannotRun": {}, "DeadlineExceeded": {}}

// InstanceFailureMessage returns the first recognized terminal container failure.
func InstanceFailureMessage(snapshot agentruntime.InstanceSnapshot) (string, bool) {
	for _, instance := range snapshot.Instances {
		// TODO(execution): Improve failure selection for Pods with multiple
		// simultaneously abnormal containers. The current projection returns the
		// first recognized condition after Provider normalization, so a sidecar can
		// become the user-visible failure instead of the workload's primary
		// container. Extend the Instance/observation contract with each container's
		// workload role and declaration order (or bind that metadata from the
		// executable plan), then select failures by an explicit priority: primary
		// workload container first, other workload containers next, and injected
		// sidecars last. Use container name as a final stable tie-breaker and retain
		// all concurrent failures as structured conditions so status can expose a
		// concise primary reason without losing diagnostic detail.
		for _, condition := range instance.Conditions {
			if condition.Scope != "container" {
				continue
			}
			if _, found := abnormalReasons[condition.Reason]; !found {
				continue
			}
			detail := condition.Message
			if detail == "" {
				detail = condition.Reason
			}
			return fmt.Sprintf("pod %s container %s: %s", instance.Name, condition.Subject, detail), true
		}
		// Preserve compatibility with providers implementing the original flat
		// Instance reason contract.
		if _, found := abnormalReasons[instance.Reason]; found {
			detail := instance.Message
			if detail == "" {
				detail = instance.Reason
			}
			// Runtime Instance currently has no container-name field. Preserve the
			// legacy shape using the workload's primary container name until the
			// projection contract carries container-level failures explicitly.
			return fmt.Sprintf("pod %s container main: %s", instance.Name, detail), true
		}
	}
	return "", false
}

// HasFailedScheduling reports whether any instance has a scheduling failure condition.
func HasFailedScheduling(snapshot agentruntime.InstanceSnapshot) bool {
	for _, instance := range snapshot.Instances {
		for _, condition := range instance.Conditions {
			if condition.Scope == "instance" && condition.Reason == "FailedScheduling" {
				return true
			}
		}
	}
	return false
}

func desiredReplicas(value *int32) int32 {
	if value == nil {
		return 1
	}
	return *value
}
