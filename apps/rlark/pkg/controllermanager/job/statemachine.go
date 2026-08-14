package job

import (
	"context"

	"github.com/looplab/fsm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Constants used by the package.
const (
	EventInit          = "init"
	EventTasksRunning  = "tasks-running"
	EventAllTasksDone  = "all-tasks-succeeded"
	EventAnyTaskFailed = "any-task-failed"
	EventJobStopped    = "job-stopped"
	EventJobResumed    = "job-resumed"
)

var jobEvents = fsm.Events{
	{
		Name: EventInit,
		Src:  []string{""},
		Dst:  string(rlarkv1alpha1.JobPhasePending),
	},
	{
		Name: EventTasksRunning,
		Src:  []string{string(rlarkv1alpha1.JobPhasePending)},
		Dst:  string(rlarkv1alpha1.JobPhaseRunning),
	},
	{
		Name: EventAllTasksDone,
		Src:  []string{string(rlarkv1alpha1.JobPhaseRunning)},
		Dst:  string(rlarkv1alpha1.JobPhaseSucceeded),
	},
	{
		Name: EventAnyTaskFailed,
		Src:  []string{string(rlarkv1alpha1.JobPhaseRunning)},
		Dst:  string(rlarkv1alpha1.JobPhaseFailed),
	},
	{
		Name: EventJobStopped,
		Src:  []string{string(rlarkv1alpha1.JobPhasePending), string(rlarkv1alpha1.JobPhaseRunning), string(rlarkv1alpha1.JobPhaseFailed)},
		Dst:  string(rlarkv1alpha1.JobPhaseStopped),
	},
	{
		Name: EventJobResumed,
		Src:  []string{string(rlarkv1alpha1.JobPhaseStopped)},
		Dst:  string(rlarkv1alpha1.JobPhasePending),
	},
}

func newJobStateMachine() *fsm.FSM {
	return fsm.NewFSM("", jobEvents, fsm.Callbacks{
		"enter_state": func(ctx context.Context, e *fsm.Event) {
			job := e.Args[0].(*rlarkv1alpha1.Job)
			job.Status.Phase = rlarkv1alpha1.JobPhase(e.Dst)
		},
		"enter_" + string(rlarkv1alpha1.JobPhasePending): func(ctx context.Context, e *fsm.Event) {
			job := e.Args[0].(*rlarkv1alpha1.Job)
			if job.Status.StartTime == nil {
				now := metav1.Now()
				job.Status.StartTime = &now
			}
			if job.Status.EndTime != nil {
				job.Status.EndTime = nil
			}
			if job.Status.Tasks == nil {
				job.Status.Tasks = make([]rlarkv1alpha1.JobTaskStatus, 0, len(job.Spec.Tasks))
				for _, t := range job.Spec.Tasks {
					job.Status.Tasks = append(job.Status.Tasks, rlarkv1alpha1.JobTaskStatus{
						Name: t.Name,
					})
				}
			}
		},
		"enter_" + string(rlarkv1alpha1.JobPhaseSucceeded): func(ctx context.Context, e *fsm.Event) {
			job := e.Args[0].(*rlarkv1alpha1.Job)
			now := metav1.Now()
			job.Status.EndTime = &now
		},
		"enter_" + string(rlarkv1alpha1.JobPhaseFailed): func(ctx context.Context, e *fsm.Event) {
			job := e.Args[0].(*rlarkv1alpha1.Job)
			now := metav1.Now()
			job.Status.EndTime = &now
		},
	})
}
