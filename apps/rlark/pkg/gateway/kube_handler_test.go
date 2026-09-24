package gateway

import (
	"encoding/json"
	"regexp"
	"testing"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestPrepareJobForCreateGeneratesResourceName(t *testing.T) {
	body := []byte(`{"apiVersion":"rlinf.io/v1alpha1","kind":"Job","metadata":{"name":"Readable Job"}}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^jo-[0-9a-f]{16}$`).MatchString(job.Name) {
		t.Fatalf("generated job name %q does not match expected format", job.Name)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "Readable Job" {
		t.Fatalf("display name = %q, want %q", got, "Readable Job")
	}
}

func TestPrepareJobForCreatePreservesDisplayNameAnnotation(t *testing.T) {
	body := []byte(`{"metadata":{"name":"client-name","annotations":{"rlark.io/display-name":"Training Run"}}}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "Training Run" {
		t.Fatalf("display name = %q, want %q", got, "Training Run")
	}
}

func TestPrepareJobForCreateRejectsLongDisplayName(t *testing.T) {
	body := []byte(`{"metadata":{"name":"client-name","annotations":{"rlark.io/display-name":"123456789012345678901234567890123456789012345678901"}}}`)

	if _, err := prepareJobForCreate(body); err == nil {
		t.Fatal("expected error for a job name longer than 50 characters")
	}
}

func TestPrepareJobForCreatePreservesFrontendJobRequest(t *testing.T) {
	body := []byte(`{
		"apiVersion":"rlinf.io/v1alpha1",
		"kind":"Job",
		"metadata":{"name":"jo-7bdbef36d2624b03","annotations":{"rlark.io/display-name":"tagtest"}},
		"spec":{
			"domain":"lky-test-domain",
			"tags":[{"key":"user","values":["tang","yanhan"]},{"key":"usage","values":["test","tag"]}],
			"tasks":[{
				"name":"actor","head":true,"agentType":"Kubernetes","role":"Actor",
				"nodeSelector":{"kubernetes.io/hostname":"hgx-049"},
				"prepareScript":"sleep 600","runScript":"python train.py",
				"kubernetes":{"workload":{"kind":"StatefulSet","replicas":1,"template":{"spec":{"containers":[{"name":"main","image":"example.com/rlark:latest","env":[{"name":"RLARK_TASK_ROLE","value":"Actor"}],"resources":{"requests":{},"limits":{}}}],"volumes":[]}}}}
			}]
		}
	}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if job.Spec.Domain != "lky-test-domain" {
		t.Fatalf("domain = %q, want lky-test-domain", job.Spec.Domain)
	}
	if len(job.Spec.Tags) != 2 || job.Spec.Tags[0].Key != "user" || len(job.Spec.Tags[0].Values) != 2 {
		t.Fatalf("tags = %#v, want frontend tags preserved", job.Spec.Tags)
	}
	if len(job.Spec.Tasks) != 1 {
		t.Fatalf("tasks = %#v, want one task", job.Spec.Tasks)
	}
	task := job.Spec.Tasks[0]
	if task.AgentType != rlarkiov1alpha1.AgentTypeKubernetes || task.Role != rlarkiov1alpha1.TaskRoleActor {
		t.Fatalf("task = %#v, want Kubernetes Actor task", task)
	}
	if task.Kubernetes == nil || task.Kubernetes.Workload == nil || task.Kubernetes.Workload.Replicas == nil || *task.Kubernetes.Workload.Replicas != 1 {
		t.Fatalf("kubernetes workload = %#v, want StatefulSet with one replica", task.Kubernetes)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "tagtest" {
		t.Fatalf("display name = %q, want tagtest", got)
	}
}
