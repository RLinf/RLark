// Package host coordinates management Task lifecycle without depending on a
// particular Runtime implementation.
package host

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// Executor is the narrow Runtime mutation and observation boundary used by a
// Task Host. Implementations must be idempotent and recover from current state.
type Executor interface {
	Reconcile(context.Context, coordination.TaskSnapshot) (coordinator.ReconcileResult, error)
	CleanupTask(context.Context, agentruntime.TaskIdentity, coordination.WorkloadSelector, coordination.CarrierSelector) (coordinator.CleanupResult, error)
	ObserveRoot(context.Context, coordination.TaskSnapshot, agentruntime.ObjectRef) (coordinator.ObserveResult, error)
}

// TaskDecoder converts one external Task representation into an immutable
// execution snapshot. The input remains opaque to the Runtime-neutral Host.
type TaskDecoder interface {
	Decode(any) (coordination.TaskSnapshot, error)
}

// Lifecycle determines Runtime-specific compatibility work around the generic
// Host state machine. It returns decisions only and must not mutate Tasks or
// primary Runtime objects directly.
type Lifecycle interface {
	BeforeReconcile(context.Context, LifecycleInput) (LifecycleDecision, error)
	AfterReconcile(context.Context, LifecycleInput) (*StatusIntent, error)
}

// LifecycleInput carries immutable execution identity plus an opaque external
// representation for compatibility lifecycles. Runtime-neutral Host code never
// interprets External; concrete assembly owns its type and validation.
type LifecycleInput struct {
	External any
	Snapshot coordination.TaskSnapshot
}

// LifecycleAction tells the Host which generic action to take next.
type LifecycleAction string

const (
	// LifecycleProceed permits normal execution reconciliation.
	LifecycleProceed LifecycleAction = "Proceed"
	// LifecycleCleanup requests identity-based cleanup before proceeding.
	LifecycleCleanup LifecycleAction = "Cleanup"
	// LifecycleWait waits for relevant state to change.
	LifecycleWait LifecycleAction = "Wait"
	// LifecycleBlock reports a stable incompatibility or ownership conflict.
	LifecycleBlock LifecycleAction = "Block"
)

// LifecycleDecision declares the next Host action and, for compatibility-only
// ordering requirements, one bounded effect that must run after Status.
type LifecycleDecision struct {
	// Action selects the next generic Host state-machine action.
	Action LifecycleAction
	// Reason is a stable, low-cardinality explanation for wait, cleanup, or
	// block decisions. It must describe the decision category rather than embed
	// object names or error text. Controller adapters may expose it in reconcile
	// summaries.
	Reason string
	// Status is the lifecycle-owned intent to persist before returning or
	// running AfterStatus.
	Status *StatusIntent
	// AfterStatus runs a bounded compatibility effect only after Status has
	// been persisted. It must be idempotent, context-aware, and must not write
	// Task fields or create/update primary Runtime objects.
	AfterStatus func(context.Context) error
	// Retry describes how the controller adapter should schedule this decision.
	Retry RetryDirective
	// AfterCleanup selects the action to take once requested cleanup is complete.
	AfterCleanup LifecycleAction
	// AfterCleanupStatus is persisted when cleanup completes before the Host
	// returns the AfterCleanup decision.
	AfterCleanupStatus *StatusIntent
}

// Outcome is the mutually exclusive result of one Host reconciliation.
type Outcome string

const (
	OutcomeSucceeded         Outcome = "Succeeded"
	OutcomeWaitingForChange  Outcome = "WaitingForChange"
	OutcomeRetrying          Outcome = "Retrying"
	OutcomeCleanupInProgress Outcome = "CleanupInProgress"
	OutcomeBlocked           Outcome = "Blocked"
	OutcomeDropped           Outcome = "Dropped"
)

// RetryClass classifies retry behavior without inspecting error text.
type RetryClass string

const (
	RetryNone       RetryClass = "None"
	RetryOnChange   RetryClass = "OnChange"
	RetryConflict   RetryClass = "Conflict"
	RetryTransient  RetryClass = "Transient"
	RetryInProgress RetryClass = "InProgress"
)

// RetryDirective describes when a result should be retried.
type RetryDirective struct {
	Class RetryClass
	After time.Duration
}

// StatusPriority defines deterministic precedence when Host inputs provide
// multiple status intents during one reconciliation.
type StatusPriority int

const (
	StatusPriorityEnvironment StatusPriority = iota + 10
	StatusPriorityWorkload
	StatusPriorityInvalid
	StatusPriorityLifecycle
	StatusPriorityCleanup
)

// ConditionMutation describes a typed condition upsert or removal.
type ConditionMutation struct {
	Condition metav1.Condition
	Remove    bool
}

// StatusIntent contains only Task status fields owned by the Host. TaskUID and
// Generation fence stale asynchronous observations.
type StatusIntent struct {
	TaskUID          string
	Generation       int64
	Phase            string
	MessageSet       bool
	Message          string
	ObservedNodesSet bool
	ObservedNodes    []string
	Conditions       []ConditionMutation
	Priority         StatusPriority
	ReasonCode       string
}

// Result is the generic Host result returned to controller adapters.
type Result struct {
	// Outcome is the mutually exclusive result of the Host state machine.
	Outcome Outcome
	// Retry tells the controller adapter whether and when to reconcile again.
	Retry RetryDirective
	// Reason is a stable, low-cardinality explanation for the result. It is
	// data for controller adapters, must not contain object names or error text,
	// and does not make Host responsible for logs.
	Reason string
	// Status is the merged Host-owned status intent applied during this
	// reconciliation, when one exists.
	Status *StatusIntent
}

// Bundle contains all immutable dependencies for one configured Runtime ID.
type Bundle struct {
	RuntimeSelector coordination.RuntimeSelector
	Decoder         TaskDecoder
	Executor        Executor
	Implementations ImplementationRegistry
	Lifecycle       Lifecycle
	// Observer receives best-effort diagnostics for Host lifecycle decisions.
	// Nil is equivalent to a no-op observer and never affects reconciliation.
	Observer observability.Observer
	// StatusFromReconcile optionally projects a full reconcile observation into
	// Task Status. A nil function preserves runtimes whose targeted observers
	// are the compatibility status source.
	StatusFromReconcile func(coordinator.ReconcileResult, coordination.TaskSnapshot) *StatusIntent
	// AfterExecution receives a successful full reconcile result for process-local
	// acceleration caches. It must be best-effort and must not perform external effects.
	AfterExecution func(coordination.TaskSnapshot, coordinator.ReconcileResult)
}

// ImplementationKey selects one exact Workload implementation for one Runtime
// kind. Workload and Runtime are orthogonal concepts but not freely combinable.
type ImplementationKey struct {
	WorkloadType    workload.TypeID
	WorkloadVersion string
	RuntimeKind     agentruntime.Kind
}

// ImplementationDescriptor is the immutable public description of one
// registered implementation.
type ImplementationDescriptor struct {
	Key  ImplementationKey
	Info workload.TypeInfo
}

// ImplementationRegistry selects exact Workload x Runtime implementations.
type ImplementationRegistry interface {
	Select(ImplementationKey) (workload.Adapter, error)
	Describe() []ImplementationDescriptor
}

// WorkloadRegistration declares one exact implementation at construction time.
type WorkloadRegistration struct {
	Key     ImplementationKey
	Adapter workload.Adapter
}
