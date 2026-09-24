package job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestStoppedJobRecordsEndTime(t *testing.T) {
	job := &rlarkv1alpha1.Job{}
	job.Status.Phase = rlarkv1alpha1.JobPhaseRunning
	fsm := newJobStateMachine()
	fsm.SetState(string(rlarkv1alpha1.JobPhaseRunning))

	if err := fsm.Event(context.Background(), EventJobStopped, job); err != nil {
		t.Fatalf("stop job: %v", err)
	}
	if job.Status.EndTime == nil {
		t.Fatal("stopped job did not record endTime")
	}
}

func TestRestartedJobResetsRunTimes(t *testing.T) {
	oldStart := metav1.NewTime(time.Now().Add(-time.Hour))
	oldEnd := metav1.NewTime(time.Now().Add(-time.Minute))
	job := &rlarkv1alpha1.Job{Status: rlarkv1alpha1.JobStatus{
		Phase: rlarkv1alpha1.JobPhaseStopped, StartTime: &oldStart, EndTime: &oldEnd,
	}}
	f := newJobStateMachine()
	f.SetState(string(job.Status.Phase))

	if err := f.Event(context.Background(), EventTasksPending, job); err != nil {
		t.Fatalf("restart job: %v", err)
	}
	if job.Status.StartTime == nil || !job.Status.StartTime.After(oldStart.Time) {
		t.Fatalf("startTime was not reset: %v", job.Status.StartTime)
	}
	if job.Status.EndTime != nil {
		t.Fatalf("endTime was not cleared: %v", job.Status.EndTime)
	}
}

func newJob(phase rlarkv1alpha1.JobPhase) *rlarkv1alpha1.Job {
	return &rlarkv1alpha1.Job{Status: rlarkv1alpha1.JobStatus{Phase: phase}}
}

// transition fires event from start and reports the resulting phase.
func transition(t *testing.T, start rlarkv1alpha1.JobPhase, event string) rlarkv1alpha1.JobPhase {
	t.Helper()
	f := newJobStateMachine()
	f.SetState(string(start))
	job := newJob(start)
	if err := f.Event(context.Background(), event, job); err != nil {
		t.Fatalf("unexpected error firing %s from %s: %v", event, start, err)
	}
	return job.Status.Phase
}

// A Job stuck in Pending must be able to reach Failed when any Task fails —
// this is the crash-loop scenario where a Task's pod enters CrashLoopBackOff
// before the Job ever reaches Running.
func TestJobCanFailFromPending(t *testing.T) {
	assert.Equal(t, rlarkv1alpha1.JobPhaseFailed, transition(t, rlarkv1alpha1.JobPhasePending, EventAnyTaskFailed))
	// Pre-existing behavior must remain intact.
	assert.Equal(t, rlarkv1alpha1.JobPhaseFailed, transition(t, rlarkv1alpha1.JobPhaseRunning, EventAnyTaskFailed))
}

// A Job in Pending must also be able to reach Succeeded when all Tasks finish
// before the Job reaches Running (e.g. very short-lived Tasks).
func TestJobCanSucceedFromPending(t *testing.T) {
	assert.Equal(t, rlarkv1alpha1.JobPhaseSucceeded, transition(t, rlarkv1alpha1.JobPhasePending, EventAllTasksDone))
	assert.Equal(t, rlarkv1alpha1.JobPhaseSucceeded, transition(t, rlarkv1alpha1.JobPhaseRunning, EventAllTasksDone))
}

// Pending -> Running transition must still work.
func TestJobCanRunFromPending(t *testing.T) {
	assert.Equal(t, rlarkv1alpha1.JobPhaseRunning, transition(t, rlarkv1alpha1.JobPhasePending, EventTasksRunning))
}

// evaluateJobEvent short-circuits on AnyFailed; combined with the
// EventAnyTaskFailed source fix, a Pending job with a failed Task must be
// able to transition to Failed (the event is now allowed from Pending).
func TestEvaluateJobEventFailedShortCircuitsAndFiresFromPending(t *testing.T) {
	f := newJobStateMachine()
	f.SetState(string(rlarkv1alpha1.JobPhasePending))
	job := newJob(rlarkv1alpha1.JobPhasePending)
	job.Status.Tasks = []rlarkv1alpha1.JobTaskStatus{
		{Name: "actor", Phase: rlarkv1alpha1.TaskPhaseFailed},
		{Name: "learner", Phase: rlarkv1alpha1.TaskPhaseRunning},
	}
	event := (&Reconciler{}).evaluateJobEvent(job)
	assert.Equal(t, EventAnyTaskFailed, event)
	assert.True(t, f.Can(event), "EventAnyTaskFailed must be allowed from Pending")
}
