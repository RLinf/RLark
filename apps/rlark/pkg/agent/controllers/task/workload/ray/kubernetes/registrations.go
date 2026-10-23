package kubernetes

import (
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// Registrations returns the Ray v1 implementation for Kubernetes. The carrier
// is selected independently from each Task's workload.carrier envelope.
func Registrations(namespace NamespaceResolver, runtimeKind agentruntime.Kind) []coordinator.WorkloadRegistration {
	return []coordinator.WorkloadRegistration{{
		Type:        workload.TypeID(taskapi.WorkloadTypeRay),
		Version:     taskapi.WorkloadVersionRayV1,
		RuntimeKind: runtimeKind,
		Adapter:     NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace),
	}}
}
