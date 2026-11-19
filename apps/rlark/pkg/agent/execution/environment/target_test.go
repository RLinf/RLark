package environment

import (
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestValidateTargetsIndexesAuthorizedObjects(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	objects := []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}}
	target := executionplan.MutationTarget{Name: "workload-pod-template", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Role: "worker", Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}

	indexed, err := ValidateTargets([]executionplan.MutationTarget{target}, objects)
	if err != nil {
		t.Fatal(err)
	}
	if indexed[target.Name] != target {
		t.Fatalf("target=%#v, want %#v", indexed[target.Name], target)
	}
}

func TestValidateTargetsSupportsWholeObjectPodTemplate(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "runtime.example.io", Version: "v1", Kind: "PodTemplateWorkload"}
	object := agentruntime.DesiredObject{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}
	target := executionplan.MutationTarget{Name: "template", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}
	result, err := ValidateTargets([]executionplan.MutationTarget{target}, []agentruntime.DesiredObject{object})
	if err != nil {
		t.Fatal(err)
	}
	if result["template"] != target {
		t.Fatalf("result=%#v", result)
	}
}

func TestValidateTargetsRejectsInvalidDeclarations(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	otherKind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}
	objects := []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}}
	valid := executionplan.MutationTarget{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}

	tests := map[string][]executionplan.MutationTarget{
		"empty name":        {{ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}},
		"duplicate name":    {valid, valid},
		"missing object":    {{Name: "pod", ObjectName: "missing", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}},
		"unknown kind":      {{Name: "pod", ObjectName: "workload", Kind: "Arbitrary", Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}},
		"mismatched type":   {{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: otherKind, Path: "spec.template"}}},
		"empty static path": {{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind}}},
	}
	for name, targets := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateTargets(targets, objects); !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestValidateTargetsSupportsDirectPod(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	object := agentruntime.DesiredObject{LogicalName: "pod", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind}}
	target := executionplan.MutationTarget{Name: "pod", ObjectName: "pod", Kind: executionplan.TargetPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}
	result, err := ValidateTargets([]executionplan.MutationTarget{target}, []agentruntime.DesiredObject{object})
	if err != nil {
		t.Fatal(err)
	}
	if result["pod"] != target {
		t.Fatalf("result=%#v", result)
	}
	customKind := agentruntime.ObjectKind{Group: "workloads.example.io", Version: "v1", Kind: "PodCarrier"}
	custom := agentruntime.DesiredObject{LogicalName: "custom", Kind: customKind, Manifest: agentruntime.TypedManifest{Type: customKind}}
	customTarget := executionplan.MutationTarget{Name: "embedded-pod", ObjectName: "custom", Kind: executionplan.TargetPod, Locator: agentruntime.ObjectLocator{Type: customKind, Path: "spec.pod"}}
	if _, err := ValidateTargets([]executionplan.MutationTarget{customTarget}, []agentruntime.DesiredObject{custom}); err != nil {
		t.Fatalf("embedded Pod target error=%v", err)
	}
}

func TestResolveConsumersCanonicalizesExactTargets(t *testing.T) {
	targets := map[string]executionplan.MutationTarget{
		"worker": {Name: "worker", ObjectName: "workload", Role: "worker"},
		"head":   {Name: "head", ObjectName: "workload", Role: "head"},
	}
	selectors := []executionplan.ConsumerSelector{{Targets: []string{"worker", "head"}}, {Targets: []string{"worker"}}}
	consumers, err := ResolveConsumers(selectors, targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 2 || consumers[0].TargetName != "head" || consumers[1].TargetName != "worker" {
		t.Fatalf("consumers=%#v", consumers)
	}
}

func TestResolveConsumersRejectsUnknownAndLegacySelectors(t *testing.T) {
	targets := map[string]executionplan.MutationTarget{"pod": {Name: "pod", ObjectName: "workload"}}
	for name, selector := range map[string]executionplan.ConsumerSelector{
		"unknown": {Targets: []string{"missing"}},
		"empty":   {},
		"legacy":  {ObjectNames: []string{"workload"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveConsumers([]executionplan.ConsumerSelector{selector}, targets); !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
