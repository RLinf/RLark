// Package kubernetesobjects preserves legacy Kubernetes metadata, ownership,
// selector, and reconcile-profile behavior shared by the current Workload and
// Runtime implementations. It is temporary migration compatibility, not a
// general Kubernetes Workload API; remove it when supported upgrades no longer
// require old objects to be read or adopted.
package kubernetesobjects

const (
	ManagementTaskNameAnnotation              = "rlark.io/management-task-name"
	ManagementTaskNamespaceAnnotation         = "rlark.io/management-task-namespace"
	ManagementTaskGenerationAnnotation        = "rlark.io/management-task-generation"
	ManagementTaskUIDAnnotation               = "rlark.io/management-task-uid"
	ManagementTaskDomainAnnotation            = "rlark.io/management-task-domain"
	ManagementTaskAdoptedGenerationAnnotation = "rlark.io/management-task-adopted-generation"
	ManagementTaskUIDLabel                    = "rlark.io/management-task-uid"
)
