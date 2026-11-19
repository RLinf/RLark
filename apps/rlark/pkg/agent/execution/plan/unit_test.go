package plan

import (
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestProjectUnitsSupportsPodTemplateAndDirectPod(t *testing.T) {
	deployment := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	pod := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	objects := []agentruntime.DesiredObject{
		{LogicalName: "workload", Kind: deployment, Manifest: agentruntime.TypedManifest{Type: deployment}},
		{LogicalName: "pod", Kind: pod, Manifest: agentruntime.TypedManifest{Type: pod}},
	}
	projected, err := ProjectUnits([]WorkloadUnit{
		{Name: "template", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Locator: agentruntime.ObjectLocator{Type: deployment, Path: "spec.template"}},
		{Name: "pod", ObjectName: "pod", Kind: agentruntime.UnitPod, Locator: agentruntime.ObjectLocator{Type: pod, Path: "."}},
	}, objects)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected[0].Units) != 1 || projected[0].Units[0].Kind != agentruntime.UnitPodTemplate || len(projected[1].Units) != 1 || projected[1].Units[0].Kind != agentruntime.UnitPod {
		t.Fatalf("projected=%#v", projected)
	}
	if len(objects[0].Units) != 0 || len(objects[1].Units) != 0 {
		t.Fatal("ProjectUnits mutated input")
	}
}

func TestProjectUnitsSupportsEmbeddedPod(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	projected, err := ProjectUnits([]WorkloadUnit{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.embeddedPod"}}}, []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected[0].Units) != 1 || projected[0].Units[0].Locator.Path != "spec.embeddedPod" {
		t.Fatalf("projected=%#v", projected)
	}
	_, err = ProjectUnits([]WorkloadUnit{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPod, Locator: agentruntime.ObjectLocator{Type: kind}}}, []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("empty path error=%v", err)
	}
}

func TestProjectUnitsSupportsWholeObjectPodTemplate(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "runtime.example.io", Version: "v1", Kind: "PodTemplateWorkload"}
	projected, err := ProjectUnits([]WorkloadUnit{{Name: "template", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}}, []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected[0].Units) != 1 || projected[0].Units[0].Kind != agentruntime.UnitPodTemplate || projected[0].Units[0].Locator.Path != "." {
		t.Fatalf("projected=%#v", projected)
	}
}
