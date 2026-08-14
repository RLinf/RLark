package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the API group and version used by this package.
var GroupVersion = schema.GroupVersion{Group: "rlinf.io", Version: "v1alpha1"}

// SchemeGroupVersion is deprecated, use GroupVersion instead.
var SchemeGroupVersion = GroupVersion

// Resource takes an unqualified resource and returns a Group-qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Node{},
		&NodeList{},
		&Task{},
		&TaskList{},
		&Job{},
		&JobList{},
		&Workflow{},
		&WorkflowList{},

		&Domain{},
		&DomainList{},
		&DomainPeer{},
		&DomainPeerList{},

		&Pod{},
		&PodList{},

		&Addon{},
		&AddonList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
