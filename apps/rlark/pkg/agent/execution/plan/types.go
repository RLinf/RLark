// Package plan owns the portable compiled Plan and the final Runtime-ready
// ExecutablePlan shared by Workload, Environment, coordination, and Runtime
// execution boundaries.
package plan

import (
	"encoding/json"

	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// ConsumerSelector selects the planned objects or execution units that need a
// binding. Targets is canonical; the remaining fields are compatibility inputs
// that a Resolver must reduce to exact targets.
type ConsumerSelector struct {
	Targets     []string `json:"targets,omitempty"`
	ObjectNames []string `json:"objectNames,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	UnitNames   []string `json:"unitNames,omitempty"`
}

// TargetPodTemplate identifies a Kubernetes-style Pod template, either embedded
// at a Runtime-defined static field or represented by the desired object itself
// at locator ".".
const TargetPodTemplate agentruntime.UnitKind = agentruntime.UnitPodTemplate

// TargetPod identifies a complete Kubernetes-style Pod value, either a directly
// declared Pod object or a Pod embedded at a Runtime-defined static field. It
// never refers to a running Pod created later by a workload controller.
const TargetPod agentruntime.UnitKind = agentruntime.UnitPod

// MutationTarget authorizes Environment mutation of one location in a desired
// object identified by logical name.
type MutationTarget struct {
	Name       string
	ObjectName string
	Kind       agentruntime.UnitKind
	Role       string
	Locator    agentruntime.ObjectLocator
}

// WorkloadUnit declares where one Pod-like execution unit exists in an
// Adapter-owned desired object. Locator is a closed Runtime contract, not an
// arbitrary JSONPath.
type WorkloadUnit struct {
	Name       string
	ObjectName string
	Kind       agentruntime.UnitKind
	Role       string
	Locator    agentruntime.ObjectLocator
}

// EnvironmentRequirement is the portable execution-plan form of a Task
// Environment declaration. Parameters contains only non-secret configuration;
// Task-scoped secret bytes are supplied through the material boundary.
type EnvironmentRequirement struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Version    string             `json:"version"`
	Parameters json.RawMessage    `json:"parameters,omitempty"`
	Consumers  []ConsumerSelector `json:"consumers,omitempty"`
}

// Requirements groups portable Environment declarations emitted by Task
// decoding and Workload Adapters.
type Requirements struct {
	Environments []EnvironmentRequirement `json:"environments,omitempty"`
}

// Empty reports whether no Environment requirement is declared.
func (r Requirements) Empty() bool {
	return len(r.Environments) == 0
}

// InstanceDiscovery tells a Provider how to find instances for plan roots.
type InstanceDiscovery struct {
	Roots   []string
	Options *agentruntime.TypedOptions
}

// UpdatePolicy reserves versioned Workload-specific update behavior.
type UpdatePolicy struct{}

// Plan is the declarative output of compiling a Task for a Runtime kind.
// Targets are the only locations an Environment Binder may mutate; they refer
// to Objects by logical name and remain internal unless a real remote Adapter
// protocol explicitly adds support for them.
type Plan struct {
	AdapterID      string
	AdapterVersion string
	CompiledHash   string
	Objects        []agentruntime.DesiredObject
	Environment    Requirements
	Targets        []MutationTarget
	Units          []WorkloadUnit
	Processes      []executionprocess.Declaration
	Discovery      InstanceDiscovery
	Update         UpdatePolicy
}

// Digests identify compiled and fully bound executable plan content.
type Digests struct {
	CompiledHash   string
	ExecutableHash string
	// BindingRevision identifies the non-secret environment binding projection.
	BindingRevision string
}

// ExecutablePlan contains only Runtime-ready desired objects and discovery state.
type ExecutablePlan struct {
	Digests   Digests
	Objects   []agentruntime.DesiredObject
	Discovery InstanceDiscovery
	Update    UpdatePolicy
}
