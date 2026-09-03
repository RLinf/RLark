package job

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestEvaluateJobEventWaitsForAllTasks(t *testing.T) {
	tests := []struct {
		name string
		job  *rlarkv1alpha1.Job
		want string
	}{
		{
			name: "running waits for every task",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhasePending,
				rlarkv1alpha1.TaskPhaseRunning, rlarkv1alpha1.TaskPhasePending),
		},
		{
			name: "running when every task runs",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhasePending,
				rlarkv1alpha1.TaskPhaseRunning, rlarkv1alpha1.TaskPhaseRunning),
			want: EventTasksRunning,
		},
		{
			name: "running returns to pending when tasks are mixed",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhaseRunning,
				rlarkv1alpha1.TaskPhaseRunning, rlarkv1alpha1.TaskPhasePending),
			want: EventTasksPending,
		},
		{
			name: "stopping becomes pending while tasks are mixed",
			job: jobWithTaskPhases(true, rlarkv1alpha1.JobPhaseRunning,
				rlarkv1alpha1.TaskPhaseStopped, rlarkv1alpha1.TaskPhaseRunning),
			want: EventTasksPending,
		},
		{
			name: "stopped when every task stops",
			job: jobWithTaskPhases(true, rlarkv1alpha1.JobPhasePending,
				rlarkv1alpha1.TaskPhaseStopped, rlarkv1alpha1.TaskPhaseStopped),
			want: EventJobStopped,
		},
		{
			name: "starting becomes pending while tasks are mixed",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhaseStopped,
				rlarkv1alpha1.TaskPhaseStopped, rlarkv1alpha1.TaskPhasePending),
			want: EventTasksPending,
		},
		{
			name: "pending may fail",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhasePending,
				rlarkv1alpha1.TaskPhasePending, rlarkv1alpha1.TaskPhaseFailed),
			want: EventAnyTaskFailed,
		},
		{
			name: "pending may succeed",
			job: jobWithTaskPhases(false, rlarkv1alpha1.JobPhasePending,
				rlarkv1alpha1.TaskPhaseSucceeded, rlarkv1alpha1.TaskPhaseSucceeded),
			want: EventAllTasksDone,
		},
	}

	r := &Reconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.evaluateJobEvent(tt.job); got != tt.want {
				t.Fatalf("evaluateJobEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPendingCanReachTerminalStates(t *testing.T) {
	for _, event := range []string{EventAnyTaskFailed, EventAllTasksDone} {
		f := newJobStateMachine()
		f.SetState(string(rlarkv1alpha1.JobPhasePending))
		if !f.Can(event) {
			t.Fatalf("Pending should allow event %q", event)
		}
	}
}

func TestBuildTaskCarriesRestartAnnotation(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:        "job",
		Annotations: map[string]string{RestartedAtAnnotation: "2026-08-21T00:00:00Z"},
	}}
	task := buildTask(job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}, "job-worker", "default")

	if got := task.Annotations[RestartedAtAnnotation]; got != job.Annotations[RestartedAtAnnotation] {
		t.Fatalf("restart annotation = %q, want %q", got, job.Annotations[RestartedAtAnnotation])
	}
	if !taskEqual(task, job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}) {
		t.Fatal("task with the same restart annotation should be equal")
	}

	job.Annotations[RestartedAtAnnotation] = "2026-08-21T00:01:00Z"
	if taskEqual(task, job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}) {
		t.Fatal("task with an old restart annotation should require an update")
	}
}

func jobWithTaskPhases(stopped bool, phase rlarkv1alpha1.JobPhase, phases ...rlarkv1alpha1.TaskPhase) *rlarkv1alpha1.Job {
	job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Stopped: stopped}, Status: rlarkv1alpha1.JobStatus{Phase: phase}}
	for i, taskPhase := range phases {
		job.Status.Tasks = append(job.Status.Tasks, rlarkv1alpha1.JobTaskStatus{Name: string(rune('a' + i)), Phase: taskPhase})
	}
	return job
}
