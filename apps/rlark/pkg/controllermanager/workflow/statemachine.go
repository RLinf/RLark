package workflow

import (
	"context"

	"github.com/looplab/fsm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Constants used by the package.
const (
	EventInit             = "init"
	EventStart            = "start"
	EventStop             = "stop"
	EventAllJobsStopped   = "all-jobs-stopped"
	EventResume           = "resume"
	EventAllJobsSucceeded = "all-jobs-succeeded"
	EventAnyJobFailed     = "any-job-failed"
)

var workflowEvents = fsm.Events{
	{
		Name: EventInit,
		Src:  []string{""},
		Dst:  string(rlarkv1alpha1.WorkflowPhasePending),
	},
	{
		Name: EventStart,
		Src:  []string{string(rlarkv1alpha1.WorkflowPhasePending)},
		Dst:  string(rlarkv1alpha1.WorkflowPhaseRunning),
	},
	{
		Name: EventStop,
		Src: []string{
			string(rlarkv1alpha1.WorkflowPhasePending),
			string(rlarkv1alpha1.WorkflowPhaseRunning),
		},
		Dst: string(rlarkv1alpha1.WorkflowPhaseStopping),
	},
	{
		Name: EventAllJobsStopped,
		Src:  []string{string(rlarkv1alpha1.WorkflowPhaseStopping)},
		Dst:  string(rlarkv1alpha1.WorkflowPhaseStopped),
	},
	{
		Name: EventResume,
		Src: []string{
			string(rlarkv1alpha1.WorkflowPhaseStopping),
			string(rlarkv1alpha1.WorkflowPhaseStopped),
		},
		Dst: string(rlarkv1alpha1.WorkflowPhasePending),
	},
	{
		Name: EventAllJobsSucceeded,
		Src:  []string{string(rlarkv1alpha1.WorkflowPhaseRunning)},
		Dst:  string(rlarkv1alpha1.WorkflowPhaseSucceeded),
	},
	{
		Name: EventAnyJobFailed,
		Src:  []string{string(rlarkv1alpha1.WorkflowPhaseRunning)},
		Dst:  string(rlarkv1alpha1.WorkflowPhaseFailed),
	},
}

func newWorkflowStateMachine() *fsm.FSM {
	return fsm.NewFSM("", workflowEvents, fsm.Callbacks{
		"enter_state": func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			wf.Status.Phase = rlarkv1alpha1.WorkflowPhase(e.Dst)
		},
		"enter_" + string(rlarkv1alpha1.WorkflowPhasePending): func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			if e.Src != "" {
				now := metav1.Now()
				wf.Status.StartTime = &now
				wf.Status.EndTime = nil
			}
			syncJobStatusSnapshot(wf)
		},
		"enter_" + string(rlarkv1alpha1.WorkflowPhaseRunning): func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			if wf.Status.StartTime == nil {
				now := metav1.Now()
				wf.Status.StartTime = &now
			}
			wf.Status.EndTime = nil
		},
		"enter_" + string(rlarkv1alpha1.WorkflowPhaseSucceeded): func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			now := metav1.Now()
			wf.Status.EndTime = &now
		},
		"enter_" + string(rlarkv1alpha1.WorkflowPhaseFailed): func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			now := metav1.Now()
			wf.Status.EndTime = &now
		},
	})
}
