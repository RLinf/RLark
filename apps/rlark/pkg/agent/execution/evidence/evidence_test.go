package evidence

import (
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestDigestJSONIsStableAndCanonical(t *testing.T) {
	left, err := DigestJSON(map[string]any{"b": 2, "a": []string{"one", "two"}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := DigestJSON(map[string]any{"a": []string{"one", "two"}, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if left != right || left.Validate() != nil {
		t.Fatalf("left=%q right=%q validation=%v", left, right, left.Validate())
	}
}

func TestDecideAllowsOnlyCompleteVerifiedEvidence(t *testing.T) {
	current, candidate := verifiedEvidence()
	decision := Decide(current, candidate)
	if decision.Mode != ModeObserveRoot || decision.Reason != ReasonVerified || decision.Root == nil {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDecideConservativelyFallsBack(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Current, *Candidate)
		reason Reason
	}{
		{"periodic", func(_ *Current, value *Candidate) { value.Periodic = true }, ReasonPeriodic},
		{"conflict", func(_ *Current, value *Candidate) { value.Conflicting = true }, ReasonConflictingHints},
		{"stale uid", func(_ *Current, value *Candidate) { value.ObservedTaskUID = "old" }, ReasonStaleTaskUID},
		{"generation", func(_ *Current, value *Candidate) { value.Generation++ }, ReasonGenerationChange},
		{"runtime", func(_ *Current, value *Candidate) { value.RuntimeID = "other" }, ReasonRuntimeMismatch},
		{"ownership", func(_ *Current, value *Candidate) { value.Root.Ownership.OwnerKey = "other" }, ReasonOwnership},
		{"missing revision", func(_ *Current, value *Candidate) { value.Root.Revision.CompiledDigest = "" }, ReasonRevisionMissing},
		{"changed revision", func(_ *Current, value *Candidate) { value.Root.Revision.CompiledDigest = NewDigest([]byte("other")) }, ReasonRevisionChanged},
		{"lifecycle", func(value *Current, _ *Candidate) { value.LifecyclePending = true }, ReasonLifecyclePending},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, candidate := verifiedEvidence()
			test.mutate(&current, &candidate)
			decision := Decide(current, candidate)
			if decision.Mode != ModeFullReconcile || decision.Reason != test.reason || decision.Root != nil {
				t.Fatalf("decision=%#v", decision)
			}
		})
	}
}

func verifiedEvidence() (Current, Candidate) {
	identity := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 3}
	runtimeSelector := coordination.RuntimeSelector{ID: "kubernetes", Kind: "Kubernetes"}
	workloadSelector := coordination.WorkloadSelector{Type: "Deployment", Version: "v1alpha1"}
	revision := Revision{Runtime: runtimeSelector, Workload: workloadSelector, CompiledDigest: NewDigest([]byte("compiled")), ExecutableDigest: NewDigest([]byte("executable")), BindingRevision: NewDigest([]byte("binding"))}
	current := Current{Identity: identity, Runtime: runtimeSelector, Workload: workloadSelector, Claimed: true, ClaimedByHost: true}
	root := Root{Ref: agentruntime.ObjectRef{RuntimeID: "kubernetes", ID: "apps/v1, Kind=Deployment", Namespace: "runtime", Name: "task", UID: "root-uid", LogicalTask: identity.Logical}, Task: identity, Revision: revision, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: identity.ObjectUID}}
	return current, Candidate{ObservedTaskUID: identity.ObjectUID, Generation: identity.Generation, RuntimeID: runtimeSelector.ID, Root: root, Expected: revision}
}
