package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=domains,scope=Cluster,shortName=rldomain
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Domain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DomainSpec   `json:"spec,omitempty"`
	Status            DomainStatus `json:"status,omitempty"`
}

type DomainSpec struct {
	CIDR string `json:"cidr,omitempty"`
}

type DomainIPAllocation struct {
	IP   string `json:"ip,omitempty"`
	Job  string `json:"job,omitempty"`  // job name
	Task string `json:"task,omitempty"` // namespace / task name
	Pod  string `json:"pod,omitempty"`  // namespace / pod name
}

type DomainStatus struct {
	// ObservedGeneration 是 Domain 控制器最近一次完成调谐时所观测到的
	// metadata.generation。它是控制器契约而非 API Server 保证：消费者在依据
	// ipAllocations 做判断前，应比较本字段与 metadata.generation 以识别过期状态。
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions 记录 Domain 的结构化状态结论，当前仅包含 ConditionTypeValidated
	// （Spec.CIDR 是否合法）。它由 Spec 驱动，写入方必须填写 ObservedGeneration。
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// IPAllocations 由 Domain 控制器独占写入，其它组件不得修改。该列表规模随
	// Domain CIDR 增长（/16 可达数万条）。
	IPAllocations []DomainIPAllocation `json:"ipAllocations,omitempty"`
}

// +kubebuilder:object:root=true
type DomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Domain `json:"items"`
}
