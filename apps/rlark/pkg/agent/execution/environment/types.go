package environment

import (
	"context"

	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// RequirementKind identifies the portable category represented by a resolved binding.
type RequirementKind string

const (
	// RequirementNetwork identifies a network binding.
	RequirementNetwork RequirementKind = "Network"
	// RequirementStorage identifies a storage binding.
	RequirementStorage RequirementKind = "Storage"
	// RequirementDevice identifies a device binding.
	RequirementDevice RequirementKind = "Device"
	// RequirementSecret identifies a secret binding.
	RequirementSecret RequirementKind = "Secret"
	// RequirementConfig identifies a non-secret configuration binding.
	RequirementConfig RequirementKind = "Config"
	// RequirementAuxiliaryService identifies an auxiliary workload service binding.
	RequirementAuxiliaryService RequirementKind = "AuxiliaryService"
)

// ResolvedConsumer identifies one concrete, validated mutation target selected
// for a binding. TargetName is the authoritative identity; the remaining fields
// are descriptive projections and must not be used to select another target.
type ResolvedConsumer struct {
	TargetName string
	ObjectName string
	Role       string
	UnitName   string
}

// ProfileReference identifies a trusted, non-secret binding profile.
type ProfileReference struct {
	Type     string
	Name     string
	Version  string
	Revision string
}

// ResolvedBinding is Runtime-ready, non-secret binding metadata with explicit
// lifecycle. Value may contain only controlled, serializable, non-sensitive
// options. Raw keys, tokens, credentials, and other secret material must use a
// separately restricted in-process mechanism and must never be stored here.
type ResolvedBinding struct {
	Kind            RequirementKind
	RequirementName string
	Profile         ProfileReference
	Consumers       []ResolvedConsumer
	Ownership       agentruntime.ResourceOwnership
	Lifecycle       agentruntime.ResourceLifecycle
	SourceRevision  string
	Algorithm       string
	Value           agentruntime.TypedOptions
}

// ResolvedEnvironment contains all bindings authorized and resolved for a Task.
type ResolvedEnvironment struct {
	Task     agentruntime.TaskIdentity
	Bindings []ResolvedBinding
}

// ResolveRequest supplies Task, Runtime, portable requirements, and the only
// Adapter-authorized mutation targets to a Resolver.
type ResolveRequest struct {
	Task         agentruntime.TaskIdentity
	Runtime      agentruntime.Info
	Requirements executionplan.Requirements
	Targets      []executionplan.MutationTarget
}

// Resolver validates and resolves every portable requirement and target without
// mutating a plan or accessing Runtime state.
type Resolver interface {
	Resolve(context.Context, ResolveRequest) (ResolvedEnvironment, error)
}
