package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=domainpeers,scope=Namespaced,shortName=rldpeer
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DomainPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DomainPeerSpec   `json:"spec,omitempty"`
	Status            DomainPeerStatus `json:"status,omitempty"`
}

type DomainPodInfo struct {
	GlobalNamespace string `json:"globalNamespace,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	Name            string `json:"name,omitempty"`
	UID             string `json:"uid,omitempty"`
	Node            string `json:"node,omitempty"`
	IP              string `json:"ip,omitempty"`
	LocalIP         string `json:"localIP,omitempty"`
}

type DomainPeerSpec struct {
	PrefixLen int             `json:"prefixLen,omitempty"`
	Pods      []DomainPodInfo `json:"pods,omitempty"`
	Cert      string          `json:"cert,omitempty"`
	Key       string          `json:"key,omitempty"`
}

type DomainPeerStatus struct {
}

// +kubebuilder:object:root=true
type DomainPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DomainPeer `json:"items"`
}
