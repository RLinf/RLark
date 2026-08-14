package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	Phase      AddonPhase         `json:"phase,omitempty"`
	Version    string             `json:"version,omitempty"`
	Message    string             `json:"message,omitempty"`
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
