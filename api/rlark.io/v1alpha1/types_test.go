package v1alpha1

import (
	"encoding/json"
	"testing"
)

func TestWorkflowSuspendedJSON(t *testing.T) {
	wf := Workflow{Spec: WorkflowSpec{Suspended: true}}
	data, err := json.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"metadata":{},"spec":{"suspended":true},"status":{}}` {
		t.Fatalf("unexpected JSON: %s", data)
	}

	var decoded Workflow
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Spec.Suspended {
		t.Fatal("spec.suspended was not preserved")
	}
}

func TestProposalPhaseValues(t *testing.T) {
	if PodPhaseUnknown != "Unknown" || WorkflowPhaseStopping != "Stopping" || WorkflowPhaseStopped != "Stopped" {
		t.Fatal("proposal phase values changed")
	}
}
