package environment

import (
	"context"
	"reflect"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type testHandler struct {
	info      HandlerInfo
	intents   []WriteIntent
	intentErr error
	applyErr  error
	applied   *[]HandlerID
}

func (h *testHandler) Info() HandlerInfo { return h.info }

func (h *testHandler) Intents(context.Context, HandlerRequest) ([]WriteIntent, error) {
	return append([]WriteIntent(nil), h.intents...), h.intentErr
}

func (h *testHandler) Apply(_ context.Context, _ HandlerRequest) error {
	if h.applied != nil {
		*h.applied = append(*h.applied, h.info.ID)
	}
	return h.applyErr
}

func handler(id HandlerID, kind RequirementKind, profile string) *testHandler {
	return &testHandler{info: HandlerInfo{ID: id, Key: HandlerKey{RuntimeKind: "Kubernetes", BindingKind: kind, ProfileType: profile}, Algorithm: string(id) + "/v1"}}
}

func binding(kind RequirementKind, name, profile string) ResolvedBinding {
	return ResolvedBinding{Kind: kind, RequirementName: name, Profile: ProfileReference{Type: profile}}
}

func TestNewRegistryRejectsInvalidRegistrations(t *testing.T) {
	valid := handler("network", RequirementNetwork, "CrossDomain")
	selfRequire := handler("self-require", RequirementNetwork, "SelfRequire")
	selfRequire.info.Requires = []HandlerID{selfRequire.info.ID}
	selfBefore := handler("self-before", RequirementNetwork, "SelfBefore")
	selfBefore.info.Before = []HandlerID{selfBefore.info.ID}
	selfAfter := handler("self-after", RequirementNetwork, "SelfAfter")
	selfAfter.info.After = []HandlerID{selfAfter.info.ID}
	selfMultiple := handler("self-multiple", RequirementNetwork, "SelfMultiple")
	selfMultiple.info.Requires = []HandlerID{selfMultiple.info.ID}
	selfMultiple.info.Before = []HandlerID{selfMultiple.info.ID}
	selfMultiple.info.After = []HandlerID{selfMultiple.info.ID}
	tests := []struct {
		name          string
		registrations []HandlerRegistration
		code          fault.Code
		message       string
	}{
		{name: "nil handler", registrations: []HandlerRegistration{{}}, code: fault.FailedPrecondition},
		{name: "empty ID", registrations: []HandlerRegistration{{Handler: &testHandler{info: HandlerInfo{Key: valid.info.Key, Algorithm: "v1"}}}}, code: fault.FailedPrecondition},
		{name: "incomplete key", registrations: []HandlerRegistration{{Handler: &testHandler{info: HandlerInfo{ID: "network", Algorithm: "v1"}}}}, code: fault.FailedPrecondition},
		{name: "empty algorithm", registrations: []HandlerRegistration{{Handler: &testHandler{info: HandlerInfo{ID: "network", Key: valid.info.Key}}}}, code: fault.FailedPrecondition},
		{name: "duplicate ID", registrations: []HandlerRegistration{{Handler: valid}, {Handler: valid}}, code: fault.AlreadyExists},
		{name: "duplicate key", registrations: []HandlerRegistration{{Handler: valid}, {Handler: handler("other", RequirementNetwork, "CrossDomain")}}, code: fault.AlreadyExists},
		{name: "self require", registrations: []HandlerRegistration{{Handler: selfRequire}}, code: fault.FailedPrecondition, message: `environment handler "self-require" cannot reference itself in Requires`},
		{name: "self before", registrations: []HandlerRegistration{{Handler: selfBefore}}, code: fault.FailedPrecondition, message: `environment handler "self-before" cannot reference itself in Before`},
		{name: "self after", registrations: []HandlerRegistration{{Handler: selfAfter}}, code: fault.FailedPrecondition, message: `environment handler "self-after" cannot reference itself in After`},
		{name: "multiple self references use stable priority", registrations: []HandlerRegistration{{Handler: selfMultiple}}, code: fault.FailedPrecondition, message: `environment handler "self-multiple" cannot reference itself in Requires`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry(test.registrations)
			if !fault.Is(err, test.code) {
				t.Fatalf("error=%v, want %s", err, test.code)
			}
			if test.message != "" && err.Error() != test.message {
				t.Fatalf("error=%q, want %q", err, test.message)
			}
		})
	}
}

func TestRegistryFreezesHandlerOrderingMetadata(t *testing.T) {
	a := handler("a", RequirementNetwork, "A")
	b := handler("b", RequirementSecret, "B")
	a.info.Before = []HandlerID{"b"}
	registry, err := NewRegistry([]HandlerRegistration{{Handler: a}, {Handler: b}})
	if err != nil {
		t.Fatal(err)
	}
	a.info.Before[0] = "missing"
	jobs, err := registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []ResolvedBinding{binding(RequirementNetwork, "a", "A"), binding(RequirementSecret, "b", "B")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Handler.Info().ID != "a" || jobs[1].Handler.Info().ID != "b" {
		t.Fatalf("registry metadata changed after construction: %q, %q", jobs[0].Handler.Info().ID, jobs[1].Handler.Info().ID)
	}
}

func TestRegistryJobsUsesStableCustomDependencyOrder(t *testing.T) {
	var applied []HandlerID
	network := handler("network", RequirementNetwork, "CrossDomain")
	ssh := handler("ssh", RequirementSecret, "SSH")
	security := handler("security", RequirementConfig, "Security")
	validation := handler("validation", RequirementConfig, "Validation")
	network.info.Requires = []HandlerID{"security"}
	network.info.Before = []HandlerID{"ssh", "missing-optional"}
	ssh.info.After = []HandlerID{"network"}
	security.info.Before = []HandlerID{"validation"}
	for _, item := range []*testHandler{network, ssh, security, validation} {
		item.applied = &applied
	}

	registry, err := NewRegistry([]HandlerRegistration{{Handler: validation}, {Handler: ssh}, {Handler: network}, {Handler: security}})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := registry.Jobs(
		agentruntime.Info{Kind: "Kubernetes"},
		[]ResolvedBinding{binding(RequirementSecret, "ssh", "SSH"), binding(RequirementNetwork, "network", "CrossDomain"), binding(RequirementConfig, "security", "Security"), binding(RequirementConfig, "validation", "Validation")},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]HandlerID, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Handler.Info().ID)
	}
	want := []HandlerID{"security", "network", "ssh", "validation"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("jobs=%v, want %v", ids, want)
	}
	if jobs[0].Request.Binding.RequirementName != "security" {
		t.Fatalf("required handler binding=%#v", jobs[0].Request.Binding)
	}
	if err := Run(context.Background(), agentruntime.TaskIdentity{}, nil, jobs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(applied, want) {
		t.Fatalf("applied=%v, want %v", applied, want)
	}
}

func TestRegistryJobsOrdersUnconstrainedHandlersByIDAndBindingsByIdentity(t *testing.T) {
	a := handler("a", RequirementNetwork, "A")
	b := handler("b", RequirementSecret, "B")
	registry, err := NewRegistry([]HandlerRegistration{{Handler: b}, {Handler: a}})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []ResolvedBinding{
		binding(RequirementSecret, "z", "B"), binding(RequirementNetwork, "z", "A"), binding(RequirementNetwork, "a", "A"),
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(jobs))
	for _, job := range jobs {
		got = append(got, string(job.Handler.Info().ID)+"/"+job.Request.Binding.RequirementName)
	}
	want := []string{"a/a", "a/z", "b/z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs=%v, want %v", got, want)
	}
}

func TestRegistryJobsRejectsMissingHandlerRequirementAndCycle(t *testing.T) {
	t.Run("unknown selected handler", func(t *testing.T) {
		registry, err := NewRegistry(nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []ResolvedBinding{binding(RequirementNetwork, "network", "CrossDomain")}, nil, nil)
		if !fault.Is(err, fault.FailedPrecondition) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("missing required handler", func(t *testing.T) {
		network := handler("network", RequirementNetwork, "CrossDomain")
		network.info.Requires = []HandlerID{"security"}
		registry, err := NewRegistry([]HandlerRegistration{{Handler: network}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []ResolvedBinding{binding(RequirementNetwork, "network", "CrossDomain")}, nil, nil)
		if !fault.Is(err, fault.FailedPrecondition) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		a := handler("a", RequirementNetwork, "A")
		b := handler("b", RequirementSecret, "B")
		a.info.Before = []HandlerID{"b"}
		b.info.Before = []HandlerID{"a"}
		registry, err := NewRegistry([]HandlerRegistration{{Handler: a}, {Handler: b}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = registry.Jobs(agentruntime.Info{Kind: "Kubernetes"}, []ResolvedBinding{binding(RequirementNetwork, "a", "A"), binding(RequirementSecret, "b", "B")}, nil, nil)
		if !fault.Is(err, fault.FailedPrecondition) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestValidateWriteIntentsAllowsEquivalentWritesAndRejectsConflicts(t *testing.T) {
	a := handler("a", RequirementNetwork, "A")
	b := handler("b", RequirementSecret, "B")
	jobs := []HandlerJob{{Handler: a}, {Handler: b}}
	if err := ValidateWriteIntents(jobs, [][]WriteIntent{{{Slot: "container/sidecar", Value: []byte("same")}}, {{Slot: "container/sidecar", Value: []byte("same")}}}); err != nil {
		t.Fatalf("equivalent writes failed: %v", err)
	}
	if err := ValidateWriteIntents(jobs, [][]WriteIntent{{{Slot: "container/sidecar", Value: []byte("one")}}, {{Slot: "container/sidecar", Value: []byte("two")}}}); !fault.Is(err, fault.Conflict) {
		t.Fatalf("conflict error=%v", err)
	}
	if err := ValidateWriteIntents(jobs[:1], [][]WriteIntent{{{}}}); !fault.Is(err, fault.Internal) {
		t.Fatalf("empty slot error=%v", err)
	}
}

func TestRunCollectsAllIntentsBeforeApplyAndHonorsCancellation(t *testing.T) {
	applied := []HandlerID{}
	a := handler("a", RequirementNetwork, "A")
	b := handler("b", RequirementSecret, "B")
	a.applied, b.applied = &applied, &applied
	a.intents = []WriteIntent{{Slot: "slot", Value: []byte("one")}}
	b.intents = []WriteIntent{{Slot: "slot", Value: []byte("two")}}
	jobs := []HandlerJob{{Handler: a}, {Handler: b}}
	if err := Run(context.Background(), agentruntime.TaskIdentity{}, nil, jobs); !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("handlers applied before global intent validation: %v", applied)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, agentruntime.TaskIdentity{}, nil, jobs[:1]); !fault.Is(err, fault.DeadlineExceeded) {
		t.Fatalf("cancel error=%v", err)
	}
}
