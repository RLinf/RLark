package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddonPhase 是 Addon 安装与升级流程的聚合阶段，由数据面 Agent 观测后上报。
// 取值集合在 CRD Schema 层封闭，新增取值必须同步更新枚举校验与文档。
// +kubebuilder:validation:Enum=Pending;Installing;Ready;Failed;Upgrading
type AddonPhase string

const (
	AddonPhasePending    AddonPhase = "Pending"
	AddonPhaseInstalling AddonPhase = "Installing"
	AddonPhaseReady      AddonPhase = "Ready"
	AddonPhaseFailed     AddonPhase = "Failed"
	AddonPhaseUpgrading  AddonPhase = "Upgrading"
)

type AddonSpec struct {
	AddonName string            `json:"addonName"`
	Version   string            `json:"version,omitempty"`
	Values    map[string]string `json:"values,omitempty"`
}

type AddonStatus struct {
	Phase   AddonPhase `json:"phase,omitempty"`
	Version string     `json:"version,omitempty"`
	Message string     `json:"message,omitempty"`
	// ObservedGeneration 是 Addon 控制器最近一次完成调谐时所观测到的
	// metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取
	// phase / conditions 之前，应比较本字段与 metadata.generation 以识别过期状态。
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions 记录 Addon 的结构化状态结论，当前仅包含 ConditionTypeReady。
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=addons,scope=Namespaced,shortName=rladdon
// +kubebuilder:printcolumn:name="Addon",type=string,JSONPath=`.spec.addonName`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonSpec   `json:"spec,omitempty"`
	Status            AddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}
