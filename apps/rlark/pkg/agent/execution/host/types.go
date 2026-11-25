// Package host coordinates management Task lifecycle without depending on a
// particular Runtime implementation.
package host

import (
	"context"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
	"github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
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

// TaskObservation is the Task-specific observed payload owned by the Host.
//
// It exists so the Host can carry its observation through the shared status
// intent without the shared package having to know about Tasks.
type TaskObservation struct {
	// Nodes are the Nodes the Task was observed running on.
	Nodes []string
	// DrainRequests carries the bounded drain-status observation written by
	// the Host. Empty means no drain activity to report; a nil slice is
	// treated the same as empty on write.
	DrainRequests []rlarkv1alpha1.TaskDrainStatus
}

// The reconciliation vocabulary below is defined once in pkg/reconcile and is
// re-exported here as aliases. Aliases, not new types, so that a Host result
// and a control-plane result are the same type and the shared shell can handle
// both; the re-export keeps the existing host.Outcome / host.Result spelling
// usable by the Runtime assemblies and controller adapters.
type (
	// Outcome is the mutually exclusive result of one Host reconciliation.
	Outcome = reconcile.Outcome
	// RetryClass classifies retry behavior without inspecting error text.
	RetryClass = reconcile.RetryClass
	// RetryDirective describes when a result should be retried.
	RetryDirective = reconcile.RetryDirective
	// StatusPriority defines deterministic precedence when Host inputs provide
	// multiple status intents during one reconciliation.
	StatusPriority = reconcile.StatusPriority
	// ConditionMutation describes a typed condition upsert or removal.
	ConditionMutation = reconcile.ConditionMutation
	// StatusIntent is the Task instantiation of the shared status intent.
	StatusIntent = reconcile.StatusIntent[TaskObservation]
	// Result is the generic Host result returned to controller adapters.
	Result = reconcile.Result[TaskObservation]
	// StatusWriter is the single Task Status mutation boundary used by pull and
	// targeted observation reconcilers.
	StatusWriter = reconcile.StatusWriter[*rlarkv1alpha1.Task, TaskObservation]
)

// Go has no constant aliases, so the shared outcome, retry and priority
// constants are re-declared. They keep the shared types through the aliases
// above, so a host constant and a reconcile constant remain interchangeable.
const (
	OutcomeSucceeded         = reconcile.OutcomeSucceeded
	OutcomeWaitingForChange  = reconcile.OutcomeWaitingForChange
	OutcomeRetrying          = reconcile.OutcomeRetrying
	OutcomeCleanupInProgress = reconcile.OutcomeCleanupInProgress
	OutcomeBlocked           = reconcile.OutcomeBlocked
	OutcomeDropped           = reconcile.OutcomeDropped

	RetryNone       = reconcile.RetryNone
	RetryOnChange   = reconcile.RetryOnChange
	RetryConflict   = reconcile.RetryConflict
	RetryTransient  = reconcile.RetryTransient
	RetryInProgress = reconcile.RetryInProgress

	StatusPriorityEnvironment = reconcile.StatusPriorityEnvironment
	StatusPriorityWorkload    = reconcile.StatusPriorityWorkload
	StatusPriorityInvalid     = reconcile.StatusPriorityInvalid
	StatusPriorityLifecycle   = reconcile.StatusPriorityLifecycle
	StatusPriorityCleanup     = reconcile.StatusPriorityCleanup
)

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
	// Admit performs effect-free execution validation before normal claim and
	// lifecycle effects. Deletion and suspension retain identity-only cleanup.
	// It must not cache an execution decision across reconciliation boundaries.
	Admit func(context.Context, LifecycleInput) error
	// AdmissionRequired distinguishes execution from identity-only cleanup/reset.
	// A nil predicate requires admission whenever Admit is configured.
	AdmissionRequired func(LifecycleInput) bool
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
