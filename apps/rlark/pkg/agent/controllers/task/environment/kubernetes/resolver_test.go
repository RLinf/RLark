package kubernetes

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/network"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/tensorboard"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestResolverComposesSupportedBindings(t *testing.T) {
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Pod"}
	target := executionplan.MutationTarget{Name: "pod", Kind: executionplan.TargetPod, Locator: agentruntime.ObjectLocator{Type: kind, Path: "."}}
	requirements := executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:       "network",
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: mustJSON(t, taskapi.NetworkEnvironmentParametersV1{Domain: "domain"}),
		Consumers:  []executionplan.ConsumerSelector{{Targets: []string{"pod"}}},
	}, {
		Name:       tensorboard.ServiceTensorBoard,
		Type:       taskapi.EnvironmentTypeTensorBoard,
		Version:    taskapi.EnvironmentVersionTensorBoardV1,
		Parameters: tensorboard.Parameters("/logs"),
		Consumers:  []executionplan.ConsumerSelector{{Targets: []string{"pod"}}},
	}}}
	resolver := mustResolver(t,
		ResolverRegistration{Type: taskapi.EnvironmentTypeNetwork, Resolver: network.Resolver{RuntimeKind: "Kubernetes", Profile: network.Profile{Image: "rlark:test"}}},
		ResolverRegistration{Type: taskapi.EnvironmentTypeTensorBoard, Resolver: tensorboard.Resolver{RuntimeKind: "Kubernetes", Profile: tensorboard.Profile{Image: "tensorboard:test"}}},
	)
	resolved, err := resolver.Resolve(context.Background(), environment.ResolveRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Requirements: requirements, Targets: []executionplan.MutationTarget{target}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Bindings) != 2 {
		t.Fatalf("bindings=%#v", resolved.Bindings)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestResolverRejectsUnsupportedRequirements(t *testing.T) {
	resolver := mustResolver(t)
	_, err := resolver.Resolve(context.Background(), environment.ResolveRequest{
		Requirements: executionplan.Requirements{
			Environments: []executionplan.EnvironmentRequirement{{
				Name:    "unknown",
				Type:    "example.io/unknown",
				Version: "v1",
			}},
		},
	})
	if !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("error=%v, want failed precondition", err)
	}
}

func TestResolverRejectsUnsupportedEnvironmentVersion(t *testing.T) {
	resolver := mustResolver(t, ResolverRegistration{
		Type: taskapi.EnvironmentTypeNetwork,
		Resolver: network.Resolver{
			RuntimeKind: "Kubernetes",
			Profile:     network.Profile{Image: "rlark:test"},
		},
	})
	_, err := resolver.Resolve(context.Background(), environment.ResolveRequest{
		Runtime: agentruntime.Info{Kind: "Kubernetes"},
		Requirements: executionplan.Requirements{
			Environments: []executionplan.EnvironmentRequirement{{
				Name:       "network",
				Type:       taskapi.EnvironmentTypeNetwork,
				Version:    "v2",
				Parameters: []byte(`{"domain":"domain"}`),
			}},
		},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func TestResolverRejectsDuplicateBuiltinEnvironment(t *testing.T) {
	requirement := executionplan.EnvironmentRequirement{
		Name:       "network",
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: []byte(`{"domain":"domain"}`),
	}
	resolver := mustResolver(t, ResolverRegistration{Type: taskapi.EnvironmentTypeNetwork, Resolver: network.Resolver{RuntimeKind: "Kubernetes", Profile: network.Profile{Image: "rlark:test"}}})
	_, err := resolver.Resolve(context.Background(), environment.ResolveRequest{
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{requirement, requirement}},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func TestNewResolverRejectsInvalidRegistrations(t *testing.T) {
	tests := map[string][]ResolverRegistration{
		"empty type":   {{Resolver: network.Resolver{}}},
		"nil resolver": {{Type: "example.io/environment"}},
		"duplicate type": {
			{Type: "example.io/environment", Resolver: network.Resolver{}},
			{Type: "example.io/environment", Resolver: network.Resolver{}},
		},
	}
	for name, registrations := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewResolver(registrations); err == nil {
				t.Fatal("invalid Environment resolver registrations were accepted")
			}
		})
	}
}

func TestResolverGroupsRegisteredTypesInFirstAppearanceOrder(t *testing.T) {
	recorder := &recordingResolver{}
	resolver := mustResolver(t,
		ResolverRegistration{Type: "example.io/a", Resolver: recorder},
		ResolverRegistration{Type: "example.io/b", Resolver: recorder},
	)
	requirements := executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{
		{Name: "a-1", Type: "example.io/a"},
		{Name: "b-1", Type: "example.io/b"},
		{Name: "a-2", Type: "example.io/a"},
	}}
	if _, err := resolver.Resolve(context.Background(), environment.ResolveRequest{Requirements: requirements}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"a-1", "a-2"}, {"b-1"}}
	if !reflect.DeepEqual(recorder.calls, want) {
		t.Fatalf("resolver calls=%v, want %v", recorder.calls, want)
	}
}

type recordingResolver struct{ calls [][]string }

func (r *recordingResolver) Resolve(_ context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	names := make([]string, 0, len(request.Requirements.Environments))
	for _, requirement := range request.Requirements.Environments {
		names = append(names, requirement.Name)
	}
	r.calls = append(r.calls, names)
	return environment.ResolvedEnvironment{Task: request.Task}, nil
}

func mustResolver(t *testing.T, registrations ...ResolverRegistration) Resolver {
	t.Helper()
	resolver, err := NewResolver(registrations)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func TestProductionHandlersPreserveCompatibilityMutationOrder(t *testing.T) {
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{
		{Handler: network.Handler{RuntimeKind: "Kubernetes"}},
		{Handler: tensorboard.Handler{RuntimeKind: "Kubernetes"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []environment.ResolvedBinding{
		{Kind: environment.RequirementNetwork, RequirementName: "network", Profile: environment.ProfileReference{Type: network.ProfileType}},
		{Kind: environment.RequirementAuxiliaryService, RequirementName: "tensorboard", Profile: environment.ProfileReference{Type: tensorboard.ProfileType}},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]environment.HandlerID, 0, len(jobs))
	for _, job := range jobs {
		got = append(got, job.Handler.Info().ID)
	}
	want := []environment.HandlerID{"kubernetes-tensorboard", "kubernetes-network"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("handler order=%v, want %v", got, want)
	}
}
