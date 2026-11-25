// Package evidence defines canonical execution revisions and conservative
// decisions for targeted Runtime observation.
package evidence

import (
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// Digest is a canonical SHA-256 content digest.
//
// The primitive lives in pkg/reconcile because the control plane compares
// desired-state digests too; this alias keeps the execution vocabulary local
// without defining a second digest format.
type Digest = sharedreconcile.Digest

// NewDigest hashes a stable byte projection.
func NewDigest(value []byte) Digest {
	return sharedreconcile.NewDigest(value)
}

// DigestJSON hashes the JSON representation of value. Callers must supply a
// deterministic projection: maps with string keys are ordered by encoding/json.
func DigestJSON(value any) (Digest, error) {
	return sharedreconcile.DigestJSON(value)
}

// Revision identifies the execution contract recorded on one current root.
type Revision struct {
	Runtime          coordination.RuntimeSelector
	Workload         coordination.WorkloadSelector
	CompiledDigest   Digest
	ExecutableDigest Digest
	BindingRevision  Digest
}

// Current is authoritative state read for one Task before fast-path selection.
type Current struct {
	Identity         agentruntime.TaskIdentity
	Runtime          coordination.RuntimeSelector
	Workload         coordination.WorkloadSelector
	Claimed          bool
	ClaimedByHost    bool
	Deleting         bool
	LifecyclePending bool
}

// Root is the identity and revision evidence observed for one Runtime root.
type Root struct {
	Ref       agentruntime.ObjectRef
	Task      agentruntime.TaskIdentity
	Revision  Revision
	Ownership agentruntime.ResourceOwnership
}

// Candidate combines a trigger hint with current root evidence.
type Candidate struct {
	ObservedTaskUID string
	Generation      int64
	RuntimeID       agentruntime.ID
	Root            Root
	Expected        Revision
	Conflicting     bool
	Periodic        bool
	DesiredChanged  bool
}

// Mode selects targeted observation or a conservative full reconciliation.
type Mode string

const (
	ModeObserveRoot   Mode = "ObserveRoot"
	ModeFullReconcile Mode = "FullReconcile"
)

// Reason is a stable low-cardinality explanation for a fast-path decision.
type Reason string

const (
	ReasonVerified         Reason = "Verified"
	ReasonPeriodic         Reason = "Periodic"
	ReasonDesiredChanged   Reason = "DesiredStateChanged"
	ReasonConflictingHints Reason = "ConflictingHints"
	ReasonTaskUnclaimed    Reason = "TaskUnclaimed"
	ReasonWrongClaimant    Reason = "WrongClaimant"
	ReasonTaskDeleting     Reason = "TaskDeleting"
	ReasonLifecyclePending Reason = "LifecyclePending"
	ReasonStaleTaskUID     Reason = "StaleTaskUID"
	ReasonGenerationChange Reason = "GenerationChanged"
	ReasonRuntimeMismatch  Reason = "RuntimeMismatch"
	ReasonRootMismatch     Reason = "RootIdentityMismatch"
	ReasonOwnership        Reason = "OwnershipUnproven"
	ReasonRevisionMissing  Reason = "RevisionMissing"
	ReasonRevisionChanged  Reason = "RevisionChanged"
)

// Decision is the pure result of evaluating fast-path evidence.
type Decision struct {
	Mode   Mode
	Reason Reason
	Root   *agentruntime.ObjectRef
}

// Decide returns ObserveRoot only when every current identity and revision is proven.
func Decide(current Current, candidate Candidate) Decision {
	full := func(reason Reason) Decision { return Decision{Mode: ModeFullReconcile, Reason: reason} }
	switch {
	case candidate.Periodic:
		return full(ReasonPeriodic)
	case candidate.DesiredChanged:
		return full(ReasonDesiredChanged)
	case candidate.Conflicting:
		return full(ReasonConflictingHints)
	case !current.Claimed:
		return full(ReasonTaskUnclaimed)
	case !current.ClaimedByHost:
		return full(ReasonWrongClaimant)
	case current.Deleting:
		return full(ReasonTaskDeleting)
	case current.LifecyclePending:
		return full(ReasonLifecyclePending)
	case candidate.ObservedTaskUID == "" || candidate.ObservedTaskUID != current.Identity.ObjectUID || candidate.Root.Task.ObjectUID != current.Identity.ObjectUID:
		return full(ReasonStaleTaskUID)
	case candidate.Generation == 0 || candidate.Generation != current.Identity.Generation || candidate.Root.Task.Generation != current.Identity.Generation:
		return full(ReasonGenerationChange)
	case candidate.RuntimeID == "" || candidate.RuntimeID != current.Runtime.ID || candidate.Root.Ref.RuntimeID != current.Runtime.ID || candidate.Root.Revision.Runtime != current.Runtime:
		return full(ReasonRuntimeMismatch)
	case candidate.Root.Ref.UID == "" || candidate.Root.Ref.LogicalTask != current.Identity.Logical || candidate.Root.Task.Logical != current.Identity.Logical:
		return full(ReasonRootMismatch)
	case candidate.Root.Ownership.Scope != agentruntime.IdentityTaskIncarnation || candidate.Root.Ownership.OwnerKey != current.Identity.ObjectUID:
		return full(ReasonOwnership)
	case revisionMissing(candidate.Root.Revision) || revisionMissing(candidate.Expected):
		return full(ReasonRevisionMissing)
	case candidate.Root.Revision != candidate.Expected || candidate.Expected.Runtime != current.Runtime || candidate.Expected.Workload != current.Workload:
		return full(ReasonRevisionChanged)
	default:
		root := candidate.Root.Ref
		return Decision{Mode: ModeObserveRoot, Reason: ReasonVerified, Root: &root}
	}
}

func revisionMissing(value Revision) bool {
	return value.Runtime.ID == "" || value.Runtime.Kind == "" || value.Workload.Type == "" || value.Workload.Version == "" || value.CompiledDigest.Validate() != nil || value.ExecutableDigest.Validate() != nil || value.BindingRevision.Validate() != nil
}
