package kubernetes

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestUnitLowererPreservesRestartableInitContainersWhenSupported(t *testing.T) {
	object := customWorkloadWithUnits(t, []corev1.Container{restartable("sidecar")})
	result, err := (UnitLowerer{RestartableInitContainers: true}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Runtime: agentruntime.Info{Kind: Kind}, Object: object})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, object) {
		t.Fatalf("result=%#v, want %#v", result, object)
	}
}

func TestUnitLowererMovesAllRestartableInitContainersToContainers(t *testing.T) {
	object := customWorkloadWithUnits(t, []corev1.Container{{Name: "setup", Image: "setup"}, restartable("network"), restartable("metrics")})
	result, err := (UnitLowerer{}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Runtime: agentruntime.Info{Kind: Kind}, Object: object})
	if err != nil {
		t.Fatal(err)
	}
	var workload struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(result.Manifest.Object, &workload); err != nil {
		t.Fatal(err)
	}
	if len(workload.Spec.Template.Spec.InitContainers) != 1 || workload.Spec.Template.Spec.InitContainers[0].Name != "setup" {
		t.Fatalf("init containers=%#v", workload.Spec.Template.Spec.InitContainers)
	}
	containers := workload.Spec.Template.Spec.Containers
	if len(containers) != 3 || containers[0].Name != "main" || containers[1].Name != "network" || containers[2].Name != "metrics" || containers[1].RestartPolicy != nil || containers[2].RestartPolicy != nil {
		t.Fatalf("containers=%#v", containers)
	}
}

func TestUnitLowererSupportsWholeObjectPod(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}, InitContainers: []corev1.Container{restartable("sidecar")}}}
	manifest, _ := json.Marshal(pod)
	object := agentruntime.DesiredObject{LogicalName: "pod", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: manifest}, Units: []agentruntime.RuntimeUnit{{Name: "pod", Kind: agentruntime.UnitPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}}}
	result, err := (UnitLowerer{}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object})
	if err != nil {
		t.Fatal(err)
	}
	var lowered corev1.Pod
	if err := json.Unmarshal(result.Manifest.Object, &lowered); err != nil {
		t.Fatal(err)
	}
	if len(lowered.Spec.InitContainers) != 0 || len(lowered.Spec.Containers) != 2 || lowered.Spec.Containers[1].Name != "sidecar" {
		t.Fatalf("pod spec=%#v", lowered.Spec)
	}
}

func TestUnitLowererRejectsConflictsAndUnsupportedLocators(t *testing.T) {
	t.Run("container name conflict", func(t *testing.T) {
		object := customWorkloadWithUnits(t, []corev1.Container{restartable("main")})
		if _, err := (UnitLowerer{}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object}); !fault.Is(err, fault.Conflict) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("unsupported locator", func(t *testing.T) {
		object := customWorkloadWithUnits(t, nil)
		object.Units[0].Locator.Path = "spec.templates[0]"
		if _, err := (UnitLowerer{}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object}); !fault.Is(err, fault.InvalidArgument) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("unsupported locator when sidecars are supported", func(t *testing.T) {
		object := customWorkloadWithUnits(t, nil)
		object.Units[0].Locator.Path = "spec.templates[0]"
		if _, err := (UnitLowerer{RestartableInitContainers: true}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object}); !fault.Is(err, fault.InvalidArgument) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("locator type mismatch when sidecars are supported", func(t *testing.T) {
		object := customWorkloadWithUnits(t, nil)
		object.Units[0].Locator.Type.Kind = "OtherWorkload"
		if _, err := (UnitLowerer{RestartableInitContainers: true}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object}); !fault.Is(err, fault.InvalidArgument) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("invalid unit shape when sidecars are supported", func(t *testing.T) {
		object := customWorkloadWithUnits(t, nil)
		object.Manifest.Object = []byte(`{"spec":{"template":{"metadata":{}}}}`)
		if _, err := (UnitLowerer{RestartableInitContainers: true}).Lower(context.Background(), agentruntime.UnitLoweringRequest{Object: object}); !fault.Is(err, fault.InvalidArgument) {
			t.Fatalf("error=%v", err)
		}
	})
}

func customWorkloadWithUnits(t *testing.T, initContainers []corev1.Container) agentruntime.DesiredObject {
	t.Helper()
	kind := agentruntime.ObjectKind{Group: "example.io", Version: "v1", Kind: "CustomWorkload"}
	object := struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}{}
	object.Spec.Template.Spec = corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "workload"}}, InitContainers: initContainers}
	manifest, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return agentruntime.DesiredObject{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: manifest}, Units: []agentruntime.RuntimeUnit{{Name: "pod", Kind: agentruntime.UnitPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}}}
}

func restartable(name string) corev1.Container {
	policy := corev1.ContainerRestartPolicyAlways
	return corev1.Container{Name: name, Image: name, RestartPolicy: &policy}
}
