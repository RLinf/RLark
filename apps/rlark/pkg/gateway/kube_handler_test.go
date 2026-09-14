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
