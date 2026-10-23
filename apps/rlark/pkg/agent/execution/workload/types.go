package workload

import (
	"context"
	"encoding/json"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// TypeID identifies a Workload semantic independently of its Runtime.
type TypeID string

// ConfigurationContract describes a versioned Adapter configuration schema.
type ConfigurationContract struct {
	Version string
	Schema  json.RawMessage
}

// TypeInfo advertises a Workload Adapter and the Runtime kinds it supports.
type TypeInfo struct {
	ID                    TypeID
	Version               string
	SupportedRuntimeKinds []agentruntime.Kind
	Configuration         ConfigurationContract
	Capabilities          []agentruntime.Capability
}

// CompileRequest supplies the immutable Task snapshot, including the complete
// Standard, Extensions, and Native configuration, and the selected Runtime.
type CompileRequest struct {
	Task    coordination.TaskSnapshot
	Runtime agentruntime.Info
}

// InventoryRequest supplies only durable execution identity and Runtime
// selection. It intentionally excludes current Workload configuration.
type InventoryRequest struct {
	Task    agentruntime.TaskIdentity
	Runtime agentruntime.Info
	Carrier coordination.CarrierSelector
}

// Inventory is the bounded set of Runtime candidates owned by an Adapter.
// Candidates are in create order. Identity cleanup considers only exact-name
// candidates declared DeleteOnTaskCleanup and removes the last currently
// present candidate per reconciliation, thereby reversing this order. Label
// candidates may be used for retained discovery but cannot authorize deletion.
type Inventory struct {
	Candidates []agentruntime.ObjectCandidate
}

// Phase is a Workload Adapter's normalized lifecycle result.
type Phase string

// Observation is the current Workload state derived from complete Runtime snapshots.
type Observation struct {
	Phase   Phase
	Reason  string
	Message string
}

// ObserveRequest supplies desired and actual state to a Workload Adapter.
type ObserveRequest struct {
	Task      coordination.TaskSnapshot
	Plan      executionplan.ExecutablePlan
	Objects   agentruntime.ObjectSnapshot
	Instances agentruntime.InstanceSnapshot
}

// Adapter owns compilation and observation semantics for one Workload Type. It
// must validate and interpret every applicable configuration entry itself and
// return one complete Plan; unsupported required configuration must fail closed.
type Adapter interface {
	Info(context.Context) (TypeInfo, error)
	Compile(context.Context, CompileRequest) (executionplan.Plan, error)
	Observe(context.Context, ObserveRequest) (Observation, error)
}

// InventoryAdapter reconstructs managed-resource locations without compiling
// current Task configuration. Cleanup uses this after selecting the claimed
// Workload implementation.
type InventoryAdapter interface {
	Inventory(context.Context, InventoryRequest) (Inventory, error)
}
