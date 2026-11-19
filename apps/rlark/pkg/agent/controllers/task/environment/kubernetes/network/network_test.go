package network

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestResolverAndHandlerBindCrossDomainSidecarToArbitraryWorkload(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "example.io", Version: "v1", Kind: "CustomWorkload"}
	manifest := []byte(`{"apiVersion":"example.io/v1","kind":"CustomWorkload","spec":{"worker":{"pod":{"metadata":{"labels":{"role":"worker"}},"spec":{"containers":[{"name":"main","image":"workload"}]}}}}}`)
	locator := agentruntime.ObjectLocator{Type: kind, Path: "spec.worker.pod"}
	plan := executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: manifest}}}, Targets: []executionplan.MutationTarget{{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: locator}}, Environment: networkRequirements("domain", "pod")}
	resolver := Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "rlark:test"}}
	resolved, err := resolver.Resolve(context.Background(), environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: plan.Environment, Targets: plan.Targets})
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]executionplan.MutationTarget{"pod": plan.Targets[0]}
	if err := (Handler{RuntimeKind: "Kubernetes"}).Apply(context.Background(), environment.HandlerRequest{Binding: resolved.Bindings[0], Targets: targets, Plan: &plan}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Spec struct {
			Worker struct {
				Pod corev1.PodTemplateSpec `json:"pod"`
			} `json:"worker"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &got); err != nil {
		t.Fatal(err)
	}
	containers := got.Spec.Worker.Pod.Spec.InitContainers
	if len(containers) != 1 || containers[0].Name != sidecarName || containers[0].RestartPolicy == nil || *containers[0].RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Fatalf("init containers=%#v", containers)
	}
	if got.Spec.Worker.Pod.Annotations[kubernetesobjects.ManagementTaskDomainAnnotation] != "domain" {
		t.Fatalf("annotations=%#v", got.Spec.Worker.Pod.Annotations)
	}
	if err := (Handler{RuntimeKind: "Kubernetes"}).Apply(context.Background(), environment.HandlerRequest{Binding: resolved.Bindings[0], Targets: targets, Plan: &plan}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &got); err != nil || len(got.Spec.Worker.Pod.Spec.InitContainers) != 1 {
		t.Fatalf("idempotent apply: object=%#v error=%v", got, err)
	}
}

func TestResolverRevisionAndHandlerFollowEnvironmentDomain(t *testing.T) {
	kind := agentruntime.ObjectKind{Group: "example.io", Version: "v1", Kind: "CustomWorkload"}
	locator := agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}
	target := executionplan.MutationTarget{Name: "pod", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: locator}
	resolver := Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "rlark:test"}}
	resolve := func(domain string) environment.ResolvedBinding {
		resolved, err := resolver.Resolve(context.Background(), environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: networkRequirements(domain, "pod"), Targets: []executionplan.MutationTarget{target}})
		if err != nil {
			t.Fatal(err)
		}
		return resolved.Bindings[0]
	}
	first, second := resolve("domain-a"), resolve("domain-b")
	if first.Profile.Revision == second.Profile.Revision {
		t.Fatal("domain change did not change network profile revision")
	}
	plan := executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: []byte(`{"spec":{"template":{"metadata":{"annotations":{"rlark.io/management-task-domain":"legacy"}},"spec":{"containers":[{"name":"main"}]}}}}`)}}}}
	request := environment.HandlerRequest{Binding: second, Targets: map[string]executionplan.MutationTarget{"pod": target}, Plan: &plan}
	if err := (Handler{}).Apply(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Template.Annotations[kubernetesobjects.ManagementTaskDomainAnnotation] != "domain-b" {
		t.Fatalf("annotations=%#v", got.Spec.Template.Annotations)
	}
}

func TestResolverRejectsUnsupportedRequirementsAndInvalidTarget(t *testing.T) {
	resolver := Resolver{RuntimeKind: "Kubernetes", Profile: Profile{Image: "rlark:test"}}
	base := environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: networkRequirements("domain", "pod"), Targets: []executionplan.MutationTarget{{Name: "pod", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Path: "spec.template"}}}}
	request := base
	request.Requirements.Environments = []executionplan.EnvironmentRequirement{{
		Name:    "unsupported",
		Type:    "example.io/storage",
		Version: "v1",
	}}
	if _, err := resolver.Resolve(context.Background(), request); !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("unsupported requirement error=%v", err)
	}
	request = base
	request.Targets[0].Locator.Path = "spec.workers[0].template"
	if _, err := resolver.Resolve(context.Background(), request); !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("invalid target error=%v", err)
	}
}

func networkRequirements(domain, target string) executionplan.Requirements {
	parameters, _ := json.Marshal(taskapi.NetworkEnvironmentParametersV1{Domain: domain})
	return executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:       "network",
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: parameters,
		Consumers:  []executionplan.ConsumerSelector{{Targets: []string{target}}},
	}}}
}
