package workflow

import (
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// TestMarkWorkflowProgressTracksAndWithdraws pins the Progressing contract: it
// is published while the controller still has work to drive, and withdrawn --
// not set to False -- once the Workflow settles, so that "idle" never
// masquerades as "failed".
func TestMarkWorkflowProgressTracksAndWithdraws(t *testing.T) {
	cases := []struct {
		name      string
		suspended bool
		phase     rlarkv1alpha1.WorkflowPhase
		reason    string
	}{
		{name: "pending awaits jobs", phase: rlarkv1alpha1.WorkflowPhasePending, reason: workflowReasonAwaitingJobs},
		{name: "running reports jobs", phase: rlarkv1alpha1.WorkflowPhaseRunning, reason: workflowReasonJobsRunning},
		{name: "stopping leftovers reports stopping", phase: rlarkv1alpha1.WorkflowPhaseStopping, reason: workflowReasonStoppingJobs},
		{name: "suspended reports stopping", suspended: true, phase: rlarkv1alpha1.WorkflowPhaseRunning, reason: workflowReasonStoppingJobs},
		{name: "suspended and stopped withdraws", suspended: true, phase: rlarkv1alpha1.WorkflowPhaseStopped},
		{name: "succeeded withdraws", phase: rlarkv1alpha1.WorkflowPhaseSucceeded},
		{name: "failed withdraws", phase: rlarkv1alpha1.WorkflowPhaseFailed},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			wf := &rlarkv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Generation: 4},
				Spec:       rlarkv1alpha1.WorkflowSpec{Suspended: testCase.suspended},
				Status: rlarkv1alpha1.WorkflowStatus{
					Phase: testCase.phase,
					// Seed a stale condition so a withdrawal is observable.
					Conditions: []metav1.Condition{{
						Type:               workflowConditionProgressing,
						Status:             metav1.ConditionTrue,
						Reason:             "Stale",
						ObservedGeneration: 3,
						LastTransitionTime: metav1.Now(),
					}},
				},
			}

			if !markWorkflowProgress(wf) {
				t.Fatal("expected the seeded condition to be rewritten or withdrawn")
			}

			condition := apimeta.FindStatusCondition(wf.Status.Conditions, workflowConditionProgressing)
			if testCase.reason == "" {
				if condition != nil {
					t.Fatalf("expected Progressing to be withdrawn, got %#v", condition)
				}
				return
			}
			if condition == nil {
				t.Fatal("expected Progressing to be published")
				return
			}
			if condition.Status != metav1.ConditionTrue {
				t.Fatalf("Progressing status = %q, want True", condition.Status)
			}
			if condition.Reason != testCase.reason {
				t.Fatalf("Progressing reason = %q, want %q", condition.Reason, testCase.reason)
			}
			if condition.ObservedGeneration != wf.Generation {
				t.Fatalf("Progressing observedGeneration = %d, want %d", condition.ObservedGeneration, wf.Generation)
			}
			if markWorkflowProgress(wf) {
				t.Fatal("expected a second call on unchanged state to report no change")
			}
		})
	}
}

// TestMarkWorkflowProgressDistinguishesStoppingMessages keeps the two stopping
// paths apart: a suspended Workflow is stopping because its spec asked for it,
// whereas a Stopping phase without suspension is clearing the Jobs left over
// from a previous run. Both are progress, but an operator reading the message
// needs to know which one is happening.
func TestMarkWorkflowProgressDistinguishesStoppingMessages(t *testing.T) {
	suspendedReason, suspendedMessage := workflowProgress(&rlarkv1alpha1.Workflow{
		Spec:   rlarkv1alpha1.WorkflowSpec{Suspended: true},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseRunning},
	})
	leftoverReason, leftoverMessage := workflowProgress(&rlarkv1alpha1.Workflow{
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseStopping},
	})

	if suspendedReason != leftoverReason {
		t.Fatalf("reasons = %q and %q, want both %q", suspendedReason, leftoverReason, workflowReasonStoppingJobs)
	}
	if suspendedMessage == leftoverMessage {
		t.Fatalf("both stopping paths report the same message %q", suspendedMessage)
	}
}
