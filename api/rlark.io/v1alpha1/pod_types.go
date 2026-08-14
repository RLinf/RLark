package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type PodPhase string

const (
	PodPhasePending   PodPhase = "Pending"
	PodPhaseRunning   PodPhase = "Running"
	PodPhaseSucceeded PodPhase = "Succeeded"
	PodPhaseFailed    PodPhase = "Failed"
)

const (
	PodLabelTaskName          = "rlark.io/task-name"
	PodLabelLocalPodName      = "rlark.io/local-pod-name"
	PodLabelLocalPodNamespace = "rlark.io/local-pod-namespace"
)

// PodSpec 包含 Pod 的标识和引用信息，由数据面上报时设置。
type PodSpec struct {
	Domain        string `json:"domain,omitempty"`        // 所属 Domain 名称
	TaskNamespace string `json:"taskNamespace,omitempty"` // 关联的 Task 命名空间
	TaskName      string `json:"taskName,omitempty"`      // 关联的 Task 名称
	PodNamespace  string `json:"podNamespace,omitempty"`  // 数据面 Pod 所在的命名空间
	PodName       string `json:"podName,omitempty"`       // 数据面 Pod 的名称
}

// PodStatus 包含 Pod 的运行状态信息（节点、IP、阶段等），
// 对应 k8s 中由调度器和 kubelet 设置的观测状态。
type PodStatus struct {
	Phase   PodPhase `json:"phase,omitempty"`
	Message string   `json:"message,omitempty"`
	Node    string   `json:"node,omitempty"` // 调度到的节点名（对应 k8s Pod.Spec.NodeName，由调度器分配）
	IP      string   `json:"ip,omitempty"`   // Pod IP（对应 k8s Pod.Status.PodIP，由运行时分配）
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=pods,scope=Namespaced,shortName=rlpod
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.node`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Pod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodSpec   `json:"spec,omitempty"`
	Status            PodStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pod `json:"items"`
}
