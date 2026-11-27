package v1alpha1

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// RolloutAnnotation on a DaemonSet says that RLark replaces its Pods, and how:
// a RolloutPolicy as JSON ("{}" for the defaults). Such a DaemonSet's update
// strategy is OnDelete, and the cluster Agent deletes its outdated Pods
// (package resident). Every DaemonSet RLark places carries it: node Agents,
// per-node Tasks, the DaemonSets of Addons, the Station components.
const RolloutAnnotation = "rlark.io/rollout"

// The rollout a DaemonSet gets when its policy leaves a field out. They are
// the same for a cluster of three nodes and for one of a hundred: one node
// first, then a tenth of the ready nodes at a time, rounded up.
const (
	DefaultRolloutCanary         int32 = 1
	DefaultRolloutMaxUnavailable       = "10%"
)

// RolloutPolicy is how the Pods of a DaemonSet are replaced after its template
// changed. Only nodes that are Ready count and are touched: a node that is
// off neither uses up the bound nor holds the others back, and gets the new
// Pod when it is back.
type RolloutPolicy struct {
	// Canary is how many nodes get the new Pod first. Until that many new Pods
	// are available (ready for the DaemonSet's minReadySeconds), no other node
	// is touched, so a template that does not work stops there. Defaults to 1.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Canary *int32 `json:"canary,omitempty"`
	// MaxUnavailable bounds how many ready nodes may be without an available
	// Pod at once after the canary: a number, or a percentage of the ready
	// nodes rounded up. Defaults to 10%.
	// +kubebuilder:validation:XIntOrString
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// WhenIdle replaces a Pod only while its Node's rlark.io/in-use annotation
	// is reported and empty: for a workload that must not be interrupted while
	// its machine works, such as what runs on a robot.
	// +optional
	WhenIdle bool `json:"whenIdle,omitempty"`
}

// Annotation is the policy as the value of RolloutAnnotation.
func (p RolloutPolicy) Annotation() string {
	data, err := json.Marshal(p)
	if err != nil {
		panic(err) // a RolloutPolicy has nothing that does not marshal
	}
	return string(data)
}

// ParseRolloutPolicy reads the value of RolloutAnnotation.
func ParseRolloutPolicy(annotation string) (RolloutPolicy, error) {
	var policy RolloutPolicy
	if err := json.Unmarshal([]byte(annotation), &policy); err != nil {
		return RolloutPolicy{}, fmt.Errorf("%s %q is not a rollout policy: %w", RolloutAnnotation, annotation, err)
	}
	return policy, nil
}

// CanaryNodes is Canary or its default.
func (p RolloutPolicy) CanaryNodes() int {
	if p.Canary != nil {
		return int(*p.Canary)
	}
	return int(DefaultRolloutCanary)
}

// Unavailable is the bound MaxUnavailable (or its default) gives among readyNodes nodes.
func (p RolloutPolicy) Unavailable(readyNodes int) (int, error) {
	bound := intstr.FromString(DefaultRolloutMaxUnavailable)
	if p.MaxUnavailable != nil {
		bound = *p.MaxUnavailable
	}
	return intstr.GetScaledValueFromIntOrPercent(&bound, readyNodes, true)
}
