package api

import rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"

const (
	CarrierTypeKubernetesDaemonSet = "rlinf.io/kubernetes-daemonset"
)

// KubernetesDaemonSetCarrierParametersV1 configures the DaemonSet carrier of a
// per-node Task: how its instances are replaced when the Task changes, which
// is the rollout policy of every DaemonSet RLark places (canary,
// maxUnavailable, whenIdle; see rlarkv1alpha1.RolloutPolicy). An update never
// starts a second instance on a node before the old one is gone, because
// per-node instances commonly hold the node's own ports, devices and
// directories. Use whenIdle for a workload that must not be interrupted while
// its machine works, such as an agent on a robot.
type KubernetesDaemonSetCarrierParametersV1 struct {
	rlarkv1alpha1.RolloutPolicy `json:",inline"`
}
