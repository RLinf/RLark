package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TaskPhase string

const (
	TaskPhasePending   TaskPhase = "Pending"
	TaskPhaseRunning   TaskPhase = "Running"
	TaskPhaseSucceeded TaskPhase = "Succeeded"
	TaskPhaseFailed    TaskPhase = "Failed"
	TaskPhaseStopped   TaskPhase = "Stopped"
)

type KubernetesTaskSpec struct {
	Workload *KubernetesWorkloadSpec `json:"workload,omitempty"`
}

type KubernetesWorkloadKind string

const (
	KubernetesWorkloadDeployment  KubernetesWorkloadKind = "Deployment"
	KubernetesWorkloadDaemonSet   KubernetesWorkloadKind = "DaemonSet"
	KubernetesWorkloadStatefulSet KubernetesWorkloadKind = "StatefulSet"
	KubernetesWorkloadCloneSet    KubernetesWorkloadKind = "CloneSet"
)

type TaskRole string

const (
	TaskRoleActor   TaskRole = "Actor"
	TaskRoleRollout TaskRole = "Rollout"
	TaskRoleEnv     TaskRole = "Env"
)

type KubernetesWorkloadSpec struct {
	Kind          KubernetesWorkloadKind `json:"kind,omitempty"`
	Replicas      *int32                 `json:"replicas,omitempty"`
	Template      corev1.PodTemplateSpec `json:"template,omitempty"`
	PvcStorageMap map[string]string      `json:"pvcStorageMap,omitempty"`
}

type DockerTaskSpec struct {
	Containers []DockerContainerSpec `json:"containers,omitempty"`
}

type RawTaskSpec struct {
	Artifact    string   `json:"artifact,omitempty"`
	Entrypoint  []string `json:"entrypoint,omitempty"`
	Environment []EnvVar `json:"environment,omitempty"`
}

type DockerContainerSpec struct {
	Name        string            `json:"name,omitempty"`
	Image       string            `json:"image,omitempty"`
	Command     []string          `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Environment []EnvVar          `json:"environment,omitempty"`
	Volumes     []DockerVolumeRef `json:"volumes,omitempty"`
}

type DockerVolumeRef struct {
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type TaskSpec struct {
	AgentType      AgentType           `json:"agentType,omitempty"`
	Role           TaskRole            `json:"role"`
	Domain         string              `json:"domain,omitempty"` // 所属 Domain 名称，从 Job 继承
	DownstreamName string              `json:"downstreamName,omitempty"`
	NodeSelector   map[string]string   `json:"nodeSelector,omitempty"` // value 可以是 ',' 分隔的多个值
	Kubernetes     *KubernetesTaskSpec `json:"kubernetes,omitempty"`
	Docker         *DockerTaskSpec     `json:"docker,omitempty"`
	Raw            *RawTaskSpec        `json:"raw,omitempty"`
	TensorBoardDir *string             `json:"tensorBoardDir,omitempty"`
	PrepareScript  string              `json:"prepareScript,omitempty"` // Ray 集群启动前执行的脚本
	RunScript      string              `json:"runScript,omitempty"`     // Ray 集群就绪后执行的脚本（仅 head 节点）
	SSHPublicKey   string              `json:"sshPublicKey,omitempty"`  // 注入到 Pod authorized_keys 的 SSH 公钥
}

type TaskStatus struct {
	Phase            TaskPhase          `json:"phase,omitempty"`
	ObservedNodes    []string           `json:"observedNodes,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	StartTime        *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime   *metav1.Time       `json:"completionTime,omitempty"`
	Message          string             `json:"message,omitempty"`
	RetryCount       int32              `json:"retryCount,omitempty"`
	TensorBoardProxy string             `json:"tensorBoardProxy,omitempty"` // 由 gateway 计算填入，指向对应 Pod 6006 端口的代理相对地址
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=tasks,scope=Namespaced,shortName=task
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TaskSpec   `json:"spec,omitempty"`
	Status            TaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}
