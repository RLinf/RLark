package host

import (
	"context"
	"errors"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

func TestImplementationRegistrySelectsExactTriple(t *testing.T) {
	adapter := registryAdapter{info: workload.TypeInfo{ID: "Job", Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Remote"}}}
	key := ImplementationKey{WorkloadType: "Job", WorkloadVersion: "v1", RuntimeKind: "Remote"}
	registry, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: key, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	if selected, err := registry.Select(key); err != nil || selected == nil {
		t.Fatalf("selected=%v error=%v", selected, err)
	}
	for _, wrong := range []ImplementationKey{
		{WorkloadType: "Other", WorkloadVersion: "v1", RuntimeKind: "Remote"},
		{WorkloadType: "Job", WorkloadVersion: "v2", RuntimeKind: "Remote"},
		{WorkloadType: "Job", WorkloadVersion: "v1", RuntimeKind: "Kubernetes"},
	} {
		if _, err := registry.Select(wrong); !errors.Is(err, ErrImplementationNotFound) {
			t.Fatalf("Select(%v) error=%v", wrong, err)
		}
		if _, err := registry.Select(wrong); !errors.Is(err, workload.ErrAdapterNotFound) {
			t.Fatalf("Select(%v) did not preserve Adapter not-found category: %v", wrong, err)
		}
	}
}

func TestImplementationRegistryValidatesAndCopiesDescriptions(t *testing.T) {
	adapter := registryAdapter{info: workload.TypeInfo{ID: "Job", Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Remote"}, Configuration: workload.ConfigurationContract{Schema: []byte(`{"type":"object"}`)}}}
	key := ImplementationKey{WorkloadType: "Job", WorkloadVersion: "v1", RuntimeKind: "Remote"}
	registry, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: key, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	descriptions := registry.Describe()
	descriptions[0].Info.SupportedRuntimeKinds[0] = "mutated"
	descriptions[0].Info.Configuration.Schema[0] = 'x'
	fresh := registry.Describe()
	if fresh[0].Info.SupportedRuntimeKinds[0] != "Remote" || fresh[0].Info.Configuration.Schema[0] != '{' {
		t.Fatalf("registry description was mutable: %#v", fresh)
	}
	if _, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: key, Adapter: adapter}, {Key: key, Adapter: adapter}}); err == nil {
		t.Fatal("duplicate registration was accepted")
	}
	mismatch := registryAdapter{info: workload.TypeInfo{ID: "Other", Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Remote"}}}
	if _, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: key, Adapter: mismatch}}); err == nil {
		t.Fatal("Adapter Info mismatch was accepted")
	}
}

func TestBundleValidationRejectsIncompleteAssembly(t *testing.T) {
	valid := Bundle{
		RuntimeSelector: coordination.RuntimeSelector{ID: "runtime", Kind: "Remote"},
		Decoder:         fakeDecoder{},
		Executor:        &testExecutor{},
		Implementations: &implementationRegistry{adapters: map[ImplementationKey]workload.Adapter{}},
		Lifecycle:       testLifecycle{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Bundle: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Bundle)
	}{
		{name: "runtime ID", mutate: func(bundle *Bundle) { bundle.RuntimeSelector.ID = "" }},
		{name: "runtime kind", mutate: func(bundle *Bundle) { bundle.RuntimeSelector.Kind = "" }},
		{name: "decoder", mutate: func(bundle *Bundle) { bundle.Decoder = nil }},
		{name: "executor", mutate: func(bundle *Bundle) { bundle.Executor = nil }},
		{name: "registry", mutate: func(bundle *Bundle) { bundle.Implementations = nil }},
		{name: "lifecycle", mutate: func(bundle *Bundle) { bundle.Lifecycle = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := valid
			tc.mutate(&bundle)
			if err := bundle.Validate(); err == nil {
				t.Fatal("incomplete Bundle was accepted")
			}
		})
	}
}

type registryAdapter struct{ info workload.TypeInfo }

func (a registryAdapter) Info(context.Context) (workload.TypeInfo, error) { return a.info, nil }
func (registryAdapter) Compile(context.Context, workload.CompileRequest) (executionplan.Plan, error) {
	return executionplan.Plan{}, nil
}
func (registryAdapter) Observe(context.Context, workload.ObserveRequest) (workload.Observation, error) {
	return workload.Observation{}, nil
}

type fakeDecoder struct{}

func (fakeDecoder) Decode(any) (coordination.TaskSnapshot, error) {
	return coordination.TaskSnapshot{}, nil
}
