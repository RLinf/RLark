package podunit

import (
	"bytes"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	unitaccessor "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime/unit"
)

func TestMutatePodSpecSupportsArbitraryEnclosingKindAndPath(t *testing.T) {
	manifest := []byte(`{"apiVersion":"custom.io/v2","kind":"Trainer","spec":{"replica":{"nested":{"template":{"metadata":{"labels":{"role":"worker"}},"spec":{"containers":[{"name":"main"}]}}}}}}`)
	result, err := MutatePodSpec(manifest, agentruntime.UnitPodTemplate, agentruntime.ObjectLocator{Type: agentruntime.ObjectKind{Group: "custom.io", Version: "v2", Kind: "Trainer"}, Path: "spec.replica.nested.template"}, func(spec *corev1.PodSpec) error {
		spec.Containers = append(spec.Containers, corev1.Container{Name: "sidecar"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := unitaccessor.Read(result, "spec.replica.nested.template")
	if err != nil {
		t.Fatal(err)
	}
	var template corev1.PodTemplateSpec
	if err := json.Unmarshal(value, &template); err != nil || len(template.Spec.Containers) != 2 || template.Spec.Containers[1].Name != "sidecar" {
		t.Fatalf("template=%#v error=%v", template, err)
	}
}

func TestMutateSupportsUnitMetadata(t *testing.T) {
	result, err := Mutate([]byte(`{"metadata":{},"spec":{"containers":[{"name":"main"}]}}`), agentruntime.UnitPod, agentruntime.ObjectLocator{Path: "."}, func(metadata *metav1.ObjectMeta, _ *corev1.PodSpec) error {
		metadata.Annotations = map[string]string{"example.io/unit": "value"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var pod corev1.Pod
	if err := json.Unmarshal(result, &pod); err != nil || pod.Annotations["example.io/unit"] != "value" {
		t.Fatalf("pod=%#v error=%v", pod, err)
	}
}

func TestMutatePodSpecSupportsRootPodTemplate(t *testing.T) {
	result, err := MutatePodSpec([]byte(`{"metadata":{"name":"template"},"spec":{"containers":[{"name":"main"}]}}`), agentruntime.UnitPodTemplate, agentruntime.ObjectLocator{Path: "."}, func(spec *corev1.PodSpec) error {
		spec.NodeName = "node"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var template corev1.PodTemplateSpec
	if err := json.Unmarshal(result, &template); err != nil || template.Spec.NodeName != "node" {
		t.Fatalf("template=%#v error=%v", template, err)
	}
}

func TestMutatePodSpecRejectsInvalidOrMissingPath(t *testing.T) {
	manifest := []byte(`{"spec":{"templates":[]}}`)
	for _, path := range []string{"", "spec.templates[0]", "spec.missing", ".spec"} {
		t.Run(path, func(t *testing.T) {
			_, err := MutatePodSpec(manifest, agentruntime.UnitPodTemplate, agentruntime.ObjectLocator{Path: path}, func(*corev1.PodSpec) error { return nil })
			if !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMutatePodSpecRejectsValueWithoutObjectSpec(t *testing.T) {
	for _, manifest := range []string{`{"template":{"metadata":{}}}`, `{"template":{"spec":[]}}`} {
		_, err := MutatePodSpec([]byte(manifest), agentruntime.UnitPodTemplate, agentruntime.ObjectLocator{Path: "template"}, func(*corev1.PodSpec) error { return nil })
		if !fault.Is(err, fault.InvalidArgument) {
			t.Fatalf("manifest=%s error=%v", manifest, err)
		}
	}
}

func TestValidateChecksUnitWithoutRewritingManifest(t *testing.T) {
	manifest := []byte(`{ "spec": { "template": { "spec": { "containers": [{"name":"main"}] } } } }`)
	original := append([]byte(nil), manifest...)
	if err := Validate(manifest, agentruntime.UnitPodTemplate, agentruntime.ObjectLocator{Path: "spec.template"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifest, original) {
		t.Fatalf("Validate rewrote manifest: got %q, want %q", manifest, original)
	}
	if err := Validate(manifest, agentruntime.UnitKind("Unknown"), agentruntime.ObjectLocator{Path: "spec.template"}); !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("unsupported kind error=%v", err)
	}
}
