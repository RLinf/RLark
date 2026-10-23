package tensorboard

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestResolverAndHandlerReuseCoveringVolume(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "example.io", Version: "v1", Kind: "Workload"}
	target := executionplan.MutationTarget{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.pod"}}
	requirements := tensorBoardRequirements("/data/logs")
	resolved, err := (Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "tensorboard:test"}}).Resolve(context.Background(), environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: requirements, Targets: []executionplan.MutationTarget{target}})
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"spec":{"pod":{"spec":{"containers":[{"name":"main","volumeMounts":[{"name":"data","mountPath":"/data","subPath":"tenant"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"data"}}]}}}}`)
	plan := executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: manifest}}}}
	request := environment.HandlerRequest{Binding: resolved.Bindings[0], Targets: map[string]executionplan.MutationTarget{"pod": target}, Plan: &plan}
	for range 2 {
		if err := (Handler{}).Apply(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	var got struct {
		Spec struct {
			Pod corev1.PodTemplateSpec `json:"pod"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Pod.Spec.InitContainers) != 1 || got.Spec.Pod.Spec.InitContainers[0].RestartPolicy == nil || got.Spec.Pod.Spec.InitContainers[0].VolumeMounts[0].Name != "data" || len(got.Spec.Pod.Spec.Volumes) != 1 {
		t.Fatalf("unexpected TensorBoard mutation: %#v", got.Spec.Pod.Spec)
	}
}

func TestResolverRejectsRelativeDirectory(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	target := executionplan.MutationTarget{Name: "pod", Kind: executionplan.TargetPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}
	requirements := tensorBoardRequirements("logs")
	_, err := (Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "tensorboard:test"}}).Resolve(context.Background(), environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: requirements, Targets: []executionplan.MutationTarget{target}})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func TestResolverRejectsUnknownParameters(t *testing.T) {
	target := executionplan.MutationTarget{
		Name: "pod",
		Kind: executionplan.TargetPodTemplate,
		Locator: agentruntime.ObjectLocator{
			Path: "spec.template",
		},
	}
	requirements := tensorBoardRequirements("/logs")
	requirements.Environments[0].Parameters = []byte(`{"logDir":"/logs","unknown":true}`)
	_, err := (Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "tensorboard:test"}}).Resolve(context.Background(), environment.ResolveRequest{
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: requirements,
		Targets:      []executionplan.MutationTarget{target},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func tensorBoardRequirements(directory string) executionplan.Requirements {
	return executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:       ServiceTensorBoard,
		Type:       taskapi.EnvironmentTypeTensorBoard,
		Version:    taskapi.EnvironmentVersionTensorBoardV1,
		Parameters: Parameters(directory),
		Consumers:  []executionplan.ConsumerSelector{{Targets: []string{"pod"}}},
	}}}
}

func TestHandlerRejectsConflictingSidecar(t *testing.T) {
	profile, _ := json.Marshal(profileValue{Image: "tensorboard:test", Directory: "/logs"})
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	target := executionplan.MutationTarget{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}
	plan := executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: []byte(`{"spec":{"containers":[{"name":"main"},{"name":"tensorboard","image":"other"}]}}`)}}}}
	binding := environment.ResolvedBinding{Consumers: []environment.ResolvedConsumer{{TargetName: "pod"}}, Value: agentruntime.TypedOptions{Value: profile}}
	err := (Handler{}).Apply(context.Background(), environment.HandlerRequest{Binding: binding, Targets: map[string]executionplan.MutationTarget{"pod": target}, Plan: &plan})
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}
