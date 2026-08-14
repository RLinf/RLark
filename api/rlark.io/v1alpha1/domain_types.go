package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=domains,scope=Cluster,shortName=rldomain
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
	IPAllocations []DomainIPAllocation `json:"ipAllocations,omitempty"`
}

// +kubebuilder:object:root=true
type DomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Domain `json:"items"`
}
