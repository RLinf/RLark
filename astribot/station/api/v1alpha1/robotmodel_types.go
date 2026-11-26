package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RobotModel is an immutable catalog entry for a robot model. Its metadata
// name is the stable model ID referenced by NodeClass and node identity.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.spec == oldSelf.spec",message="RobotModel spec is immutable"
type RobotModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RobotModelSpec `json:"spec"`
}

type RobotModelSpec struct {
	// DisplayName is the operator-facing label for this model.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	DisplayName string `json:"displayName"`
	// +kubebuilder:validation:MaxLength=128
	Vendor string `json:"vendor,omitempty"`
	// +kubebuilder:validation:MaxLength=1024
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
type RobotModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RobotModel `json:"items"`
}
