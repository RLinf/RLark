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
			if wf.Status.Jobs == nil {
				wf.Status.Jobs = make([]rlarkv1alpha1.WorkflowJobStatus, 0, len(wf.Spec.JobTemplates))
				for _, jt := range wf.Spec.JobTemplates {
					wf.Status.Jobs = append(wf.Status.Jobs, rlarkv1alpha1.WorkflowJobStatus{
						Name: jt.Name,
					})
				}
			}
		},
		"enter_" + string(rlarkv1alpha1.WorkflowPhaseRunning): func(ctx context.Context, e *fsm.Event) {
			wf := e.Args[0].(*rlarkv1alpha1.Workflow)
			now := metav1.Now()
			wf.Status.StartTime = &now
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
