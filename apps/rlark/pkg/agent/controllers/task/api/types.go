// Package api defines versioned Task workload, carrier, and environment
// parameter contracts.
package api

const (
	WorkloadTypeRay                  = "rlinf.io/ray"
	WorkloadVersionRayV1             = "v1"
	CarrierTypeKubernetesDeployment  = "rlinf.io/kubernetes-deployment"
	CarrierTypeKubernetesDaemonSet   = "rlinf.io/kubernetes-daemonset"
	CarrierTypeKubernetesStatefulSet = "rlinf.io/kubernetes-statefulset"
	CarrierVersionKubernetesV1       = "v1"
	EnvironmentTypeSSH               = "rlinf.io/ssh"
	EnvironmentVersionSSHV1          = "v1"
	EnvironmentTypeNetwork           = "rlinf.io/network"
	EnvironmentVersionNetworkV1      = "v1"
	EnvironmentTypeTensorBoard       = "rlinf.io/tensorboard"
	EnvironmentVersionTensorBoardV1  = "v1"
)

// RayRole 表示 Task 在 Ray 集群中的角色。
type RayRole string

const (
	RayRoleHead   RayRole = "head"
	RayRoleWorker RayRole = "worker"
)

// InlineScript 表示由兼容 shell 执行的用户脚本。
type InlineScript struct {
	Inline string `json:"inline"`
}

// RayExecutionParametersV1 是 rlinf.io/ray/v1 的参数合同。
type RayExecutionParametersV1 struct {
	Role          RayRole       `json:"role"`
	HeadTaskName  string        `json:"headTaskName,omitempty"`
	TotalNodes    int32         `json:"totalNodes,omitempty"`
	NodeRankStart int32         `json:"nodeRankStart,omitempty"`
	Prepare       *InlineScript `json:"prepare,omitempty"`
	Run           *InlineScript `json:"run,omitempty"`
}

// KubernetesReplicatedCarrierParametersV1 configures replicated Kubernetes
// carriers such as Deployment and StatefulSet.
type KubernetesReplicatedCarrierParametersV1 struct {
	Replicas *int32 `json:"replicas,omitempty"`
}

// KubernetesDaemonSetCarrierParametersV1 is intentionally empty because a
// DaemonSet derives its instance count from eligible nodes.
type KubernetesDaemonSetCarrierParametersV1 struct{}

// SSHEnvironmentParametersV1 是 rlinf.io/ssh/v1 的参数合同。
// AuthorizedKeys contains public authentication material. It may be copied to
// the local execution snapshot, but not to portable Plans or remote transport.
type SSHEnvironmentParametersV1 struct {
	AuthorizedKeys []string `json:"authorizedKeys,omitempty"`
}

// NetworkEnvironmentParametersV1 configures the virtual network joined by a
// Task. Domain is network identity, not workload placement or Runtime identity.
type NetworkEnvironmentParametersV1 struct {
	Domain string `json:"domain"`
}

// TensorBoardEnvironmentParametersV1 是 rlinf.io/tensorboard/v1 的参数合同。
type TensorBoardEnvironmentParametersV1 struct {
	LogDir string `json:"logDir"`
}
