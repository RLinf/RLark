package reconcile

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testObservation stands in for a resource-specific observed payload. It is
// deliberately a struct rather than a bare slice so the tests exercise the
// generic parameter the way real callers instantiate it.
type testObservation struct {
	Nodes []string
}

func TestMergeStatusIntentsUsesPriorityAndFencing(t *testing.T) {
	intents := []StatusIntent[testObservation]{
		{ObjectUID: "uid", Generation: 2, Phase: "Running", MessageSet: true, Message: "running", Priority: StatusPriorityWorkload},
		{ObjectUID: "uid", Generation: 2, Phase: "Pending", MessageSet: true, Message: "cleanup", Priority: StatusPriorityCleanup, Conditions: []ConditionMutation{{Condition: metav1.Condition{Type: "Ready"}}}},
		{ObjectUID: "other", Generation: 2, Phase: "Failed", Priority: StatusPriorityCleanup},
		{ObjectUID: "uid", Generation: 1, Phase: "Failed", Priority: StatusPriorityCleanup},
	}

	merged := MergeStatusIntents("uid", 2, intents...)
	if merged == nil {
		t.Fatal("expected merged intent")
		return
	}
	if merged.Phase != "Pending" {
		t.Fatalf("unexpected phase %q", merged.Phase)
	}
	if merged.Message != "cleanup" {
		t.Fatalf("unexpected message %q", merged.Message)
	}
	if len(merged.Conditions) != 1 {
		t.Fatalf("unexpected conditions %d", len(merged.Conditions))
	}
}

func TestMergeStatusIntentsMergesEqualPriority(t *testing.T) {
	intents := []StatusIntent[testObservation]{
		{ObjectUID: "uid", Generation: 1, Phase: "Running", Priority: StatusPriorityWorkload},
		{ObjectUID: "uid", Generation: 1, MessageSet: true, Message: "ready", ObservedSet: true, Observed: testObservation{Nodes: []string{"node"}}, Priority: StatusPriorityWorkload},
	}

	merged := MergeStatusIntents("uid", 1, intents...)
	if merged == nil {
		t.Fatal("expected merged intent")
		return
	}
	if merged.Phase != "Running" {
		t.Fatalf("unexpected phase %q", merged.Phase)
	}
	if merged.Message != "ready" {
		t.Fatalf("unexpected message %q", merged.Message)
	}
	if len(merged.Observed.Nodes) != 1 {
		t.Fatalf("unexpected observed nodes %v", merged.Observed.Nodes)
	}
}

func TestMergeStatusIntentsPreservesExplicitEmptyValues(t *testing.T) {
	intents := []StatusIntent[testObservation]{
		{MessageSet: true, Message: "", ObservedSet: true, Priority: StatusPriorityWorkload},
	}

	merged := MergeStatusIntents("uid", 3, intents...)
	if merged == nil {
		t.Fatal("expected merged intent")
		return
	}
	if merged.ObjectUID != "uid" || merged.Generation != 3 {
		t.Fatalf("unexpected identity %q/%d", merged.ObjectUID, merged.Generation)
	}
	if !merged.MessageSet || merged.Message != "" {
		t.Fatalf("expected explicit empty message, got %v/%q", merged.MessageSet, merged.Message)
	}
	if !merged.ObservedSet || merged.Observed.Nodes != nil {
		t.Fatalf("expected explicit empty observation, got %v/%v", merged.ObservedSet, merged.Observed.Nodes)
	}
}

func TestMergeStatusIntentsReturnsNilWhenEveryIntentIsFencedOut(t *testing.T) {
	intents := []StatusIntent[testObservation]{
		{ObjectUID: "stale", Generation: 1, Phase: "Running", Priority: StatusPriorityWorkload},
		{ObjectUID: "uid", Generation: 1, Phase: "Running", Priority: StatusPriorityCleanup},
	}

	if merged := MergeStatusIntents("uid", 2, intents...); merged != nil {
		t.Fatalf("expected no merged intent, got %+v", merged)
	}
}

func TestStatusPriorityOrdersControlPlaneRollUpBetweenEnvironmentAndWorkload(t *testing.T) {
	// A roll-up computed by the control plane already accounts for an
	// environment probe, and the data plane remains the authority on what the
	// workload is doing. Asserting the order keeps a later insertion from
	// silently reshuffling who wins a merge.
	if StatusPriorityEnvironment >= StatusPriorityControlPlane || StatusPriorityControlPlane >= StatusPriorityWorkload {
		t.Fatalf("unexpected priority order: environment=%d controlPlane=%d workload=%d",
			StatusPriorityEnvironment, StatusPriorityControlPlane, StatusPriorityWorkload)
	}
}

func TestApplyConditionMutationsUpsertsRemovesAndReportsChange(t *testing.T) {
	var conditions []metav1.Condition

	changed := ApplyConditionMutations(&conditions, 7, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Validated", Status: metav1.ConditionTrue, Reason: "SpecValidated"}},
		{Condition: metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Online"}},
	})
	if !changed {
		t.Fatal("adding conditions must report a change")
	}
	if len(conditions) != 2 {
		t.Fatalf("unexpected conditions %d", len(conditions))
	}
	for _, condition := range conditions {
		if condition.ObservedGeneration != 7 {
			t.Fatalf("condition %q should have been backfilled, got %d", condition.Type, condition.ObservedGeneration)
		}
	}

	if ApplyConditionMutations(&conditions, 7, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Online"}},
	}) {
		t.Fatal("re-applying an identical condition must not report a change")
	}

	if !ApplyConditionMutations(&conditions, 7, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Validated"}, Remove: true},
	}) {
		t.Fatal("removing a present condition must report a change")
	}
	if len(conditions) != 1 || conditions[0].Type != "Ready" {
		t.Fatalf("unexpected conditions after removal %+v", conditions)
	}

	if ApplyConditionMutations(&conditions, 7, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Validated"}, Remove: true},
	}) {
		t.Fatal("removing an absent condition must not report a change")
	}
}

func TestApplyConditionMutationsKeepsAnExplicitObservedGeneration(t *testing.T) {
	var conditions []metav1.Condition

	ApplyConditionMutations(&conditions, 9, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Progressing", Status: metav1.ConditionTrue, Reason: "Rolling", ObservedGeneration: 4}},
	})

	if len(conditions) != 1 || conditions[0].ObservedGeneration != 4 {
		t.Fatalf("an explicit generation must not be overwritten, got %+v", conditions)
	}
}

func TestApplyConditionMutationsLeavesObservationDrivenConditionsWithoutAGeneration(t *testing.T) {
	var conditions []metav1.Condition

	// Observation-driven writers pass 0 because their conditions carry no
	// generation contract; claiming one would let a consumer believe the
	// condition reflects the latest Spec.
	ApplyConditionMutations(&conditions, 0, []ConditionMutation{
		{Condition: metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "HeartbeatExpired"}},
	})

	if len(conditions) != 1 || conditions[0].ObservedGeneration != 0 {
		t.Fatalf("expected no generation contract, got %+v", conditions)
	}
}

func TestMergeStatusIntentsDoesNotAliasConditionsOfTheWinningIntent(t *testing.T) {
	winner := StatusIntent[testObservation]{
		ObjectUID:  "uid",
		Generation: 1,
		Priority:   StatusPriorityCleanup,
		Conditions: []ConditionMutation{{Condition: metav1.Condition{Type: "Ready"}}},
	}
	intents := []StatusIntent[testObservation]{
		winner,
		{ObjectUID: "uid", Generation: 1, Priority: StatusPriorityCleanup, Conditions: []ConditionMutation{{Condition: metav1.Condition{Type: "Degraded"}}}},
	}

	merged := MergeStatusIntents("uid", 1, intents...)
	if merged == nil {
		t.Fatal("expected merged intent")
		return
	}
	if len(merged.Conditions) != 2 {
		t.Fatalf("unexpected conditions %d", len(merged.Conditions))
	}
	if len(winner.Conditions) != 1 {
		t.Fatalf("merging mutated the caller's intent: %d conditions", len(winner.Conditions))
	}
}
