package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "station.rlark.io", Version: "v1alpha1"}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&NodeOperation{}, &NodeOperationList{},
		&SiteNetwork{}, &SiteNetworkList{},
		&NetworkEnrollment{}, &NetworkEnrollmentList{},
		&NodeAdmission{}, &NodeAdmissionList{},
		&NodeClass{}, &NodeClassList{},
		&Machine{}, &MachineList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
