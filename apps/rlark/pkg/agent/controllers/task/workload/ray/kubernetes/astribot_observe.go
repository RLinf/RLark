package kubernetes

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// DaemonSetStatusPhase projects a per-node Task's DaemonSet and its instances.
// Only instances on Ready nodes count: the Task is Running when each of them
// runs the current template and is ready, which holds when there are none as
// well, and Pending while one is still being updated, started or keeps
// failing. An instance on a node that is switched off or still joining is
// listed in the Task's instances but does not hold the Task back, and no
// instance ever fails it: the DaemonSet keeps restarting a failing one. Where
// the Task updates only on idle nodes, an instance of the old version that
// runs on a node in use does not hold it back either.
func DaemonSetStatusPhase(daemonSet *appsv1.DaemonSet, instances agentruntime.InstanceSnapshot) (rlarkv1alpha1.TaskPhase, string) {
	if daemonSet.Status.ObservedGeneration < daemonSet.Generation {
		return rlarkv1alpha1.TaskPhasePending, ""
	}
	generation := daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration]
	// An annotation that does not parse is the zero policy here; the rollout controller reports it.
	rollout, _ := rlarkv1alpha1.ParseRolloutPolicy(daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation])
	whenIdle := rollout.WhenIdle
	counted, current, offline, waiting := 0, 0, 0, 0
	for _, instance := range instances.Instances {
		nodeReady, nodeInUse, updated, ready := true, false, false, false
		for _, condition := range instance.Conditions {
			switch {
			case condition.Scope == "node" && condition.Reason == "NotReady":
				nodeReady = false
			case condition.Scope == "node" && condition.Reason == "InUse":
				nodeInUse = true
			case condition.Scope == "revision":
				updated = condition.Message == generation
			case condition.Scope == "instance" && condition.Reason == "Ready":
				ready = true
			}
		}
		if !nodeReady {
			offline++
			continue
		}
		if failure, found := InstanceFailureMessage(agentruntime.InstanceSnapshot{Instances: []agentruntime.Instance{instance}}); found {
			return rlarkv1alpha1.TaskPhasePending, failure
		}
		counted++
		switch {
		case updated && ready:
			current++
		case ready && whenIdle && nodeInUse:
			// The old version, working on a node that is in use: it is replaced
			// when the node is idle, which may be hours away, and until then the
			// Task runs as it should.
			waiting++
		}
	}
	switch {
	case current+waiting < counted:
		return rlarkv1alpha1.TaskPhasePending, fmt.Sprintf("%d of %d instances on ready nodes updated and available", current, counted)
	case waiting > 0:
		return rlarkv1alpha1.TaskPhaseRunning, fmt.Sprintf("%d of %d instances on ready nodes wait for their nodes to be idle before they are updated", waiting, counted)
	case offline > 0:
		return rlarkv1alpha1.TaskPhaseRunning, fmt.Sprintf("%d of %d instances on nodes that are not ready", offline, len(instances.Instances))
	case daemonSet.Status.DesiredNumberScheduled == 0:
		return rlarkv1alpha1.TaskPhaseRunning, "no node matches the Task's node selector"
	}
	return rlarkv1alpha1.TaskPhaseRunning, ""
}

// pendingReasons are the Pod events that say why an instance does not start,
// the first found winning: a volume that cannot be mounted (a missing
// host-path directory, a storage the node cannot reach) comes before a
// scheduling failure, since a Pod that has one was scheduled.
var pendingReasons = []string{"FailedMount", "FailedAttachVolume", "FailedScheduling"}

// PendingReason is why a pending instance does not start, as
// "<event reason>: <event message>", from the instances' event conditions.
func PendingReason(snapshot agentruntime.InstanceSnapshot) (string, bool) {
	for _, reason := range pendingReasons {
		for _, instance := range snapshot.Instances {
			for _, condition := range instance.Conditions {
				if condition.Scope != "instance" || condition.Reason != reason {
					continue
				}
				if condition.Message == "" {
					return reason, true
				}
				return reason + ": " + condition.Message, true
			}
		}
	}
	return "", false
}
