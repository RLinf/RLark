package v1alpha1

import "k8s.io/apimachinery/pkg/runtime"

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)
