package kubernetes

import (
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// kubernetesCarrier is a Task's decoded Kubernetes carrier: the workload kind
// it renders and that kind's parameters.
type kubernetesCarrier struct {
	kind     string
	replicas *int32
	rollout  rlarkv1alpha1.RolloutPolicy
}
