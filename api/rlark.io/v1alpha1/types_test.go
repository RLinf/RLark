package v1alpha1

import (
	"encoding/json"
	"testing"
)

func TestWorkflowStoppedJSON(t *testing.T) {
	wf := Workflow{Spec: WorkflowSpec{Stopped: true}}
	data, err := json.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"metadata":{},"spec":{"stopped":true},"status":{}}` {
		t.Fatalf("unexpected JSON: %s", data)
	}

	var decoded Workflow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Spec.Stopped {
		t.Fatal("spec.stopped was not preserved")
	}
}

func TestProposalPhaseValues(t *testing.T) {
	if PodPhaseUnknown != "Unknown" || WorkflowPhaseStopping != "Stopping" || WorkflowPhaseStopped != "Stopped" {
		t.Fatal("proposal phase values changed")
	}
}
