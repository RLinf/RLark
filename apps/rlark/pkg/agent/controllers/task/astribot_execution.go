package task

import (
	"sort"

	corev1 "k8s.io/api/core/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// perNodeInstances projects a per-node Task's observed Pods into its status
// instances, sorted by node. An instance is Running when its Pod runs and is
// ready, Failed when a container cannot start or keeps crashing, and Pending
// otherwise. An instance on a node that is not Ready says so in its message:
// it does not count toward the Task's phase.
func perNodeInstances(instances agentruntime.InstanceSnapshot) []rlarkv1alpha1.TaskInstanceStatus {
	result := make([]rlarkv1alpha1.TaskInstanceStatus, 0, len(instances.Instances))
	for _, instance := range instances.Instances {
		status := rlarkv1alpha1.TaskInstanceStatus{Name: instance.Name, Phase: rlarkv1alpha1.TaskInstancePending}
		ready, nodeReady, waiting := false, true, ""
		for _, condition := range instance.Conditions {
			switch {
			case condition.Scope == "node" && condition.Reason == "NotReady":
				nodeReady = false
			case condition.Scope == "node":
				status.Node = condition.Subject
			case condition.Scope == "image" && condition.Subject == "main":
				status.Image = condition.Message
			case condition.Scope == "instance" && condition.Reason == "Ready":
				ready = true
			case condition.Scope == "container" && waiting == "":
				waiting = condition.Reason
			}
		}
		single := agentruntime.InstanceSnapshot{Instances: []agentruntime.Instance{instance}}
		if failure, found := raykubernetes.InstanceFailureMessage(single); found {
			status.Phase, status.Message = rlarkv1alpha1.TaskInstanceFailed, failure
		} else if ready && instance.Phase == agentruntime.InstancePhase(corev1.PodRunning) {
			status.Phase = rlarkv1alpha1.TaskInstanceRunning
		} else if reason, found := raykubernetes.PendingReason(single); found {
			status.Message = reason
		} else if waiting != "" {
			status.Message = waiting
		} else if instance.Phase == agentruntime.InstancePhase(corev1.PodRunning) {
			status.Message = "not ready"
		}
		if !nodeReady {
			status.Message = "node not ready"
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Node != result[j].Node {
			return result[i].Node < result[j].Node
		}
		return result[i].Name < result[j].Name
	})
	return result
}
