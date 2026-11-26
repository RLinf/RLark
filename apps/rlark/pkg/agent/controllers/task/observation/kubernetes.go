// Package observation defines trusted Runtime observation registrations used
// to assemble Task push controllers.
package observation

import "k8s.io/apimachinery/pkg/runtime/schema"

// Kubernetes declares one namespaced workload root that may wake Task
// reconciliation. Registrations come from trusted Workload implementations and
// are never discovered from arbitrary cluster resources.
type Kubernetes struct {
	GVK             schema.GroupVersionKind
	CarrierType     string
	InstanceProfile string
}
