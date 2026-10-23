package kubernetes

import (
	"maps"
	"testing"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestExecutionRevisionRoundTripPreservesUnrelatedMetadata(t *testing.T) {
	revision := validExecutionRevision()
	object := &appsv1.Deployment{}
	object.Annotations = map[string]string{"user.example/key": "keep"}
	object.Spec.Template.Annotations = map[string]string{"template.example/key": "unchanged"}
	templateAnnotations := maps.Clone(object.Spec.Template.Annotations)

	if err := WriteExecutionRevision(object, revision); err != nil {
		t.Fatal(err)
	}
	got, err := ReadExecutionRevision(object)
	if err != nil {
		t.Fatal(err)
	}
	if got != revision {
		t.Fatalf("revision = %#v, want %#v", got, revision)
	}
	if object.Annotations["user.example/key"] != "keep" {
		t.Fatalf("unrelated root annotation was changed: %#v", object.Annotations)
	}
	if !maps.Equal(object.Spec.Template.Annotations, templateAnnotations) {
		t.Fatalf("Pod template metadata changed: %#v", object.Spec.Template.Annotations)
	}
}

func TestWriteExecutionRevisionRejectsInvalidInputWithoutMutation(t *testing.T) {
	object := &appsv1.Deployment{}
	object.Annotations = map[string]string{"user.example/key": "keep"}
	before := maps.Clone(object.Annotations)
	revision := validExecutionRevision()
	revision.ExecutableDigest = "not-a-digest"

	if err := WriteExecutionRevision(object, revision); err == nil {
		t.Fatal("expected malformed digest error")
	}
	if !maps.Equal(object.Annotations, before) {
		t.Fatalf("invalid revision mutated annotations: %#v", object.Annotations)
	}
}

func TestReadExecutionRevisionRejectsMissingAndMalformedEvidence(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{name: "missing", annotations: nil},
		{name: "malformed digest", annotations: func() map[string]string {
			object := &appsv1.Deployment{}
			if err := WriteExecutionRevision(object, validExecutionRevision()); err != nil {
				t.Fatal(err)
			}
			result := maps.Clone(object.Annotations)
			result[BindingRevisionAnnotation] = "SHA256:ABC"
			return result
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := &appsv1.Deployment{}
			object.Annotations = test.annotations
			if _, err := ReadExecutionRevision(object); err == nil {
				t.Fatal("expected incomplete or malformed evidence error")
			}
		})
	}
}

func validExecutionRevision() agentruntime.ExecutionRevision {
	digest := string(evidence.NewDigest([]byte("revision")))
	return agentruntime.ExecutionRevision{
		RuntimeID:        "kubernetes/default",
		RuntimeKind:      "Kubernetes",
		WorkloadType:     "Deployment",
		WorkloadVersion:  "v1alpha1",
		CompiledDigest:   digest,
		ExecutableDigest: digest,
		BindingRevision:  digest,
	}
}
