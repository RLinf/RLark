package supervision

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestInjectInstallsToolsWithoutEnvironmentRequirement(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	target := executionplan.MutationTarget{
		Name:       "pod",
		ObjectName: "workload",
		Kind:       executionplan.TargetPod,
		Locator:    agentruntime.ObjectLocator{Type: kind, Path: "."},
	}
	plan := executionplan.Plan{
		Objects: []agentruntime.DesiredObject{{
			LogicalName: "workload",
			Kind:        kind,
			Manifest: agentruntime.TypedManifest{
				Type:   kind,
				Object: []byte(`{"apiVersion":"v1","kind":"Pod","spec":{"containers":[{"name":"main","image":"workload"}]}}`),
			},
		}},
		Targets: []executionplan.MutationTarget{target},
	}
	unit := executionplan.WorkloadUnit{
		Name:       target.Name,
		ObjectName: target.ObjectName,
		Kind:       target.Kind,
		Locator:    target.Locator,
	}
	for range 2 {
		if err := installTools(&plan, unit, "rlark:test"); err != nil {
			t.Fatal(err)
		}
	}
	var pod corev1.Pod
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &pod); err != nil {
		t.Fatal(err)
	}
	if len(pod.Spec.InitContainers) != 1 || pod.Spec.InitContainers[0].Name != initContainerName || len(pod.Spec.Volumes) != 1 || len(pod.Spec.Containers[0].VolumeMounts) != 1 {
		t.Fatalf("unexpected tools mutation: %#v", pod.Spec)
	}
}

func TestInjectRejectsConflictingToolsMount(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	plan := executionplan.Plan{
		Objects: []agentruntime.DesiredObject{{
			LogicalName: "workload",
			Kind:        kind,
			Manifest: agentruntime.TypedManifest{
				Type:   kind,
				Object: []byte(`{"spec":{"containers":[{"name":"main","volumeMounts":[{"name":"other","mountPath":"/rlark-tools"}]}]}}`),
			},
		}},
		Targets: []executionplan.MutationTarget{{
			Name:       "pod",
			ObjectName: "workload",
			Kind:       executionplan.TargetPod,
			Locator:    agentruntime.ObjectLocator{Type: kind, Path: "."},
		}},
	}
	unit := executionplan.WorkloadUnit{
		Name:       "pod",
		ObjectName: "workload",
		Kind:       executionplan.TargetPod,
		Locator:    agentruntime.ObjectLocator{Type: kind, Path: "."},
	}
	if err := installTools(&plan, unit, "rlark:test"); !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}
