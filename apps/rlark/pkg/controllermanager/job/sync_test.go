package job

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

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

func TestBuildTaskCarriesJobTags(t *testing.T) {
	job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{
		Tags: []rlarkv1alpha1.JobTag{{Key: "team", Values: []string{"research", "platform"}}},
	}}
	task := buildTask(job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}, "job-worker", "default")

	if !reflect.DeepEqual(task.Spec.Tags, job.Spec.Tags) {
		t.Fatalf("task tags = %#v, want %#v", task.Spec.Tags, job.Spec.Tags)
	}

	job.Spec.Tags[0].Values[0] = "infrastructure"
	if task.Spec.Tags[0].Values[0] != "research" {
		t.Fatalf("task tags should not share backing storage with job tags: %#v", task.Spec.Tags)
	}
}

func TestValidateHeadTask(t *testing.T) {
	tmpl := func(name string, head bool, replicas *int32) rlarkv1alpha1.JobTaskTemplate {
		return rlarkv1alpha1.JobTaskTemplate{
			Name: name,
			Head: head,
			TaskSpec: rlarkv1alpha1.TaskSpec{
				Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{
					Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: replicas},
				},
			},
		}
	}

	tests := []struct {
		name    string
		tasks   []rlarkv1alpha1.JobTaskTemplate
		wantErr bool
	}{
		{
			name: "head with single pod is valid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{
				tmpl("head", true, ptr.To(int32(1))),
				tmpl("worker", false, ptr.To(int32(4))),
			},
		},
		{
			name: "head with unset replicas defaults to one and is valid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{
				{Name: "head", Head: true},
			},
		},
		{
			name: "head with multiple pods is invalid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{
				tmpl("head", true, ptr.To(int32(2))),
			},
			wantErr: true,
		},
		{
			name: "head with zero pods is invalid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{
				tmpl("head", true, ptr.To(int32(0))),
			},
			wantErr: true,
		},
		{
			name: "multiple head tasks are invalid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{
				tmpl("head-a", true, ptr.To(int32(1))),
				tmpl("head-b", true, ptr.To(int32(1))),
			},
			wantErr: true,
		},
		{
			name:  "no head task is valid",
			tasks: []rlarkv1alpha1.JobTaskTemplate{tmpl("worker", false, ptr.To(int32(3)))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Tasks: tt.tasks}}
			err := validateHeadTask(job)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestMarkJobInvalid(t *testing.T) {
	job := &rlarkv1alpha1.Job{}
	if !markJobInvalid(job, "boom") {
		t.Fatal("expected the job to change on first invalidation")
	}
	if job.Status.Phase != rlarkv1alpha1.JobPhaseFailed {
		t.Fatalf("phase = %q, want %q", job.Status.Phase, rlarkv1alpha1.JobPhaseFailed)
	}
	if job.Status.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}
	cond := findCondition(job.Status.Conditions, jobConditionValidated)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Message != "boom" {
		t.Fatalf("unexpected validated condition: %+v", cond)
	}

	if markJobInvalid(job, "boom") {
		t.Fatal("expected no change when the invalid state is unchanged")
	}
}

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func jobWithTaskPhases(stopped bool, phase rlarkv1alpha1.JobPhase, phases ...rlarkv1alpha1.TaskPhase) *rlarkv1alpha1.Job {
	job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Stopped: stopped}, Status: rlarkv1alpha1.JobStatus{Phase: phase}}
	for i, taskPhase := range phases {
		job.Status.Tasks = append(job.Status.Tasks, rlarkv1alpha1.JobTaskStatus{Name: string(rune('a' + i)), Phase: taskPhase})
	}
	return job
}
