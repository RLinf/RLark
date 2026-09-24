package workflow

import (
	"context"
	"testing"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestWorkflowStopAndResumeTransitions(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseRunning}}
	f := newWorkflowStateMachine()
	f.SetState(string(wf.Status.Phase))

	if err := f.Event(context.Background(), EventStop, wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopping {
		t.Fatalf("phase = %s, want Stopping", wf.Status.Phase)
	}
	if err := f.Event(context.Background(), EventAllJobsStopped, wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopped {
		t.Fatalf("phase = %s, want Stopped", wf.Status.Phase)
	}
	if err := f.Event(context.Background(), EventResume, wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhasePending {
		t.Fatalf("phase = %s, want Pending", wf.Status.Phase)
	}
}

func TestEvaluateStoppedWorkflow(t *testing.T) {
	r := &Reconciler{}
	wf := &rlarkv1alpha1.Workflow{
		Spec: rlarkv1alpha1.WorkflowSpec{Stopped: true},
		Status: rlarkv1alpha1.WorkflowStatus{Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "one", Phase: rlarkv1alpha1.JobPhaseStopped},
			{Name: "two", Phase: rlarkv1alpha1.JobPhaseStopped},
		}},
	}
	if event := r.evaluateWorkflowEvent(wf); event != EventAllJobsStopped {
		t.Fatalf("event = %q, want %q", event, EventAllJobsStopped)
	}
}

func TestEvaluateStoppedWorkflowWithoutDispatchedJobs(t *testing.T) {
	r := &Reconciler{}
	wf := &rlarkv1alpha1.Workflow{Spec: rlarkv1alpha1.WorkflowSpec{Stopped: true}}
	if event := r.evaluateWorkflowEvent(wf); event != EventAllJobsStopped {
		t.Fatalf("event = %q, want %q", event, EventAllJobsStopped)
	}
}
