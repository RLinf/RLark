package job

import (
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
)

func TestSetRayExecutionPreservesScripts(t *testing.T) {
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{
		Role:    taskapi.RayRoleWorker,
		Prepare: &taskapi.InlineScript{Inline: "prepare-user-code"},
		Run:     &taskapi.InlineScript{Inline: "run-user-code"},
	})
	if err != nil {
		t.Fatal(err)
	}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "head", Head: true, TaskSpec: rlarkv1alpha1.TaskSpec{
		Workload: rlarkv1alpha1.TaskWorkloadSpec{
			Kind:       taskapi.WorkloadTypeRay,
			Version:    taskapi.WorkloadVersionRayV1,
			Parameters: apiextensionsv1.JSON{Raw: parameters},
		},
	}}
	setRayExecution(&template.TaskSpec, &rlarkv1alpha1.Job{}, template)
	got, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](template.Workload.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != taskapi.RayRoleHead || got.HeadTaskName != "" {
		t.Fatalf("Ray orchestration parameters = %#v", got)
	}
	if got.Prepare == nil || got.Prepare.Inline != "prepare-user-code" || got.Run == nil || got.Run.Inline != "run-user-code" {
		t.Fatalf("Ray scripts were not preserved: %#v", got)
	}
}

func TestSetRayExecutionDefaultsEmptyEnvelope(t *testing.T) {
	t.Parallel()

	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "training"}, Spec: rlarkv1alpha1.JobSpec{
		Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "head", Head: true}, {Name: "worker"}},
	}}
	template := job.Spec.Tasks[1]
	setRayExecution(&template.TaskSpec, job, template)

	if got := template.Workload.Kind; got != taskapi.WorkloadTypeRay {
		t.Fatalf("workload kind = %q", got)
	}
	if got := template.Workload.Version; got != taskapi.WorkloadVersionRayV1 {
		t.Fatalf("workload version = %q", got)
	}
	parameters, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](template.Workload.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Role != taskapi.RayRoleWorker || parameters.HeadTaskName != "training-head" || parameters.TotalNodes != 2 || parameters.NodeRankStart != 1 {
		t.Fatalf("default Ray parameters = %#v", parameters)
	}
}

func TestSetRayExecutionDoesNotReplaceExplicitAdapter(t *testing.T) {
	t.Parallel()

	template := rlarkv1alpha1.JobTaskTemplate{TaskSpec: rlarkv1alpha1.TaskSpec{Workload: rlarkv1alpha1.TaskWorkloadSpec{
		Kind:    "example.io/custom",
		Version: "v2",
	}}}
	setRayExecution(&template.TaskSpec, &rlarkv1alpha1.Job{}, template)
	if template.Workload.Kind != "example.io/custom" || template.Workload.Version != "v2" {
		t.Fatalf("explicit workload was replaced: %#v", template.Workload)
	}
}

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

func TestBuildTaskCarriesJobRestartOperation(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:        "job",
		Annotations: map[string]string{RestartedAtAnnotation: "2026-08-21T00:00:00Z"},
	}}
	task := buildTask(job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}, "job-worker", "default")

	if got := task.Annotations[RestartedAtAnnotation]; got != job.Annotations[RestartedAtAnnotation] {
		t.Fatalf("Job restart operation = %q, want %q", got, job.Annotations[RestartedAtAnnotation])
	}
	if !taskEqual(task, job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}) {
		t.Fatal("Task from the current Job restart operation should be equal")
	}

	job.Annotations[RestartedAtAnnotation] = "2026-08-21T00:01:00Z"
	if taskEqual(task, job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}) {
		t.Fatal("Task from an older Job restart operation should require replacement")
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

func TestBuildTaskLowersJobDomainToNetworkEnvironment(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		Spec: rlarkv1alpha1.JobSpec{
			Domain: "training-network",
		},
	}
	task := buildTask(job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}, "job-worker", "default")

	var found *rlarkv1alpha1.TaskEnvironment
	for i := range task.Spec.Environments {
		if task.Spec.Environments[i].Type == taskapi.EnvironmentTypeNetwork {
			found = &task.Spec.Environments[i]
			break
		}
	}
	if found == nil || found.Version != taskapi.EnvironmentVersionNetworkV1 {
		t.Fatalf("network environment = %#v", found)
	}
	parameters, err := taskapi.DecodeParametersStrict[taskapi.NetworkEnvironmentParametersV1](found.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Domain != job.Spec.Domain {
		t.Fatalf("network domain = %q, want %q", parameters.Domain, job.Spec.Domain)
	}
	job.Spec.Domain = "other-network"
	if taskEqual(task, job, rlarkv1alpha1.JobTaskTemplate{Name: "worker"}) {
		t.Fatal("Task with stale network environment matched changed Job domain")
	}
}

func TestBuildTaskWithoutJobDomainRemovesTemplateNetworkEnvironment(t *testing.T) {
	template := rlarkv1alpha1.JobTaskTemplate{
		Name: "worker",
		TaskSpec: rlarkv1alpha1.TaskSpec{
			Environments: []rlarkv1alpha1.TaskEnvironment{{
				Type:       taskapi.EnvironmentTypeNetwork,
				Version:    taskapi.EnvironmentVersionNetworkV1,
				Parameters: apiextensionsv1.JSON{Raw: []byte(`{"domain":"stale"}`)},
			}},
		},
	}
	task := buildTask(&rlarkv1alpha1.Job{}, template, "job-worker", "default")
	for _, environment := range task.Spec.Environments {
		if environment.Type == taskapi.EnvironmentTypeNetwork && environment.Version == taskapi.EnvironmentVersionNetworkV1 {
			t.Fatalf("Task retained network environment without Job domain: %#v", environment)
		}
	}
}

func TestValidateTaskTemplates(t *testing.T) {
	valid := func(name string) rlarkv1alpha1.JobTaskTemplate {
		return rlarkv1alpha1.JobTaskTemplate{
			Name: name,
			TaskSpec: rlarkv1alpha1.TaskSpec{
				Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: "custom-runtime"},
				Workload: rlarkv1alpha1.TaskWorkloadSpec{
					Kind:    "custom-workload",
					Version: "v1",
					Carrier: rlarkv1alpha1.TaskWorkloadCarrier{Type: "custom-carrier", Version: "v1"},
					Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "main", Image: "example:v1"},
					}}},
				},
			},
		}
	}

	tests := []struct {
		name      string
		mutate    func(*rlarkv1alpha1.Job)
		wantError string
	}{
		{name: "valid open identifiers"},
		{name: "multiple heads are not a Job validation concern", mutate: func(job *rlarkv1alpha1.Job) {
			job.Spec.Tasks[0].Head = true
			job.Spec.Tasks = append(job.Spec.Tasks, valid("head-b"))
			job.Spec.Tasks[1].Head = true
		}},
		{name: "empty name", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Name = " " }, wantError: "spec.tasks[0].name must not be empty"},
		{name: "duplicate name", mutate: func(job *rlarkv1alpha1.Job) {
			job.Spec.Tasks = append(job.Spec.Tasks, valid("worker"))
		}, wantError: `spec.tasks[1].name "worker" is duplicated`},
		{name: "empty runtime kind", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Runtime.Kind = "" }, wantError: "spec.tasks[0].runtime.kind must not be empty"},
		{name: "empty workload kind", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Workload.Kind = "" }, wantError: "spec.tasks[0].workload.kind must not be empty"},
		{name: "empty workload version", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Workload.Version = "" }, wantError: "spec.tasks[0].workload.version must not be empty"},
		{name: "empty carrier type", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Workload.Carrier.Type = "" }, wantError: "spec.tasks[0].workload.carrier.type must not be empty"},
		{name: "empty carrier version", mutate: func(job *rlarkv1alpha1.Job) { job.Spec.Tasks[0].Workload.Carrier.Version = "" }, wantError: "spec.tasks[0].workload.carrier.version must not be empty"},
		{name: "missing main container", mutate: func(job *rlarkv1alpha1.Job) {
			job.Spec.Tasks[0].Workload.Template.Spec.Containers[0].Name = "worker"
		}, wantError: `spec.tasks[0].workload.template.spec.containers must contain exactly one container named "main", got 0`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{valid("worker")}}}
			if tt.mutate != nil {
				tt.mutate(job)
			}
			err := validateTaskTemplates(job)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateTaskTemplates() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("validateTaskTemplates() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestMarkJobValid(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Generation: 3}}

	if !markJobValid(job) {
		t.Fatal("expected the job to change on first validation")
	}
	cond := findCondition(job.Status.Conditions, jobConditionValidated)
	if cond == nil {
		t.Fatal("expected a Validated condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Validated status = %q, want True", cond.Status)
	}
	if cond.Reason != jobReasonSpecValidated {
		t.Errorf("Validated reason = %q, want %q", cond.Reason, jobReasonSpecValidated)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("Validated observedGeneration = %d, want 3", cond.ObservedGeneration)
	}

	if markJobValid(job) {
		t.Fatal("expected no change when the valid state is unchanged")
	}

	// A new Spec generation must be recorded on the existing condition so
	// consumers can tell the conclusion applies to the current Spec.
	job.Generation = 4
	if !markJobValid(job) {
		t.Fatal("expected a change when the observed generation advances")
	}
	cond = findCondition(job.Status.Conditions, jobConditionValidated)
	if cond.ObservedGeneration != 4 {
		t.Errorf("Validated observedGeneration = %d, want 4", cond.ObservedGeneration)
	}
}

// markJobValid must clear a stale Validated=False left behind by a previously
// invalid Spec, otherwise clients keep seeing the old rejection.
func TestMarkJobValidClearsStaleInvalid(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	if !markJobInvalid(job, "boom") {
		t.Fatal("expected the job to change on first invalidation")
	}

	job.Generation = 2
	if !markJobValid(job) {
		t.Fatal("expected the job to change when the spec becomes valid")
	}
	cond := findCondition(job.Status.Conditions, jobConditionValidated)
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Validated status = %q, want True", cond.Status)
	}
	if cond.Reason != jobReasonSpecValidated {
		t.Errorf("Validated reason = %q, want %q", cond.Reason, jobReasonSpecValidated)
	}
	if cond.Message != "" {
		t.Errorf("Validated message = %q, want it cleared", cond.Message)
	}
	if cond.ObservedGeneration != 2 {
		t.Errorf("Validated observedGeneration = %d, want 2", cond.ObservedGeneration)
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
	job := &rlarkv1alpha1.Job{Spec: rlarkv1alpha1.JobSpec{Suspended: stopped}, Status: rlarkv1alpha1.JobStatus{Phase: phase}}
	for i, taskPhase := range phases {
		job.Status.Tasks = append(job.Status.Tasks, rlarkv1alpha1.JobTaskStatus{Name: string(rune('a' + i)), Phase: taskPhase})
	}
	return job
}
