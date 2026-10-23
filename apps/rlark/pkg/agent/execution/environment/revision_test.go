package environment

import (
	"encoding/json"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestBindingRevisionUsesCanonicalEvidenceDigest(t *testing.T) {
	resolved := ResolvedEnvironment{Bindings: []ResolvedBinding{{Kind: RequirementSecret, RequirementName: "credential", Profile: ProfileReference{Type: "Opaque", Name: "default"}}}}
	got, err := BindingRevision(resolved)
	if err != nil {
		t.Fatal(err)
	}
	projection := []bindingRevisionProjection{{Kind: RequirementSecret, RequirementName: "credential", Profile: ProfileReference{Type: "Opaque", Name: "default"}}}
	data, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	want := string(evidence.NewDigest(data))
	if got != want {
		t.Fatalf("revision=%q, want %q", got, want)
	}
}

func TestBindingRevisionIsCanonicalAndExcludesValue(t *testing.T) {
	first := ResolvedEnvironment{Bindings: []ResolvedBinding{
		{
			Kind: RequirementSecret, RequirementName: "credential", Profile: ProfileReference{Type: "Opaque", Name: "task", Version: "v1", Revision: "profile-a"},
			Consumers: []ResolvedConsumer{{TargetName: "worker", ObjectName: "workload", Role: "worker"}, {TargetName: "head", ObjectName: "workload", Role: "head"}},
			Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "cluster/task"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
			SourceRevision: "source-a", Algorithm: "opaque/v1", Value: agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Kind: "OpaqueMaterial"}, Value: []byte("raw-key-one")},
		},
		{Kind: RequirementNetwork, RequirementName: "cross-domain", Profile: ProfileReference{Type: "CrossDomain", Name: "builtin", Version: "v1", Revision: "network-a"}, Algorithm: "network/v1"},
	}}
	second := ResolvedEnvironment{Bindings: []ResolvedBinding{first.Bindings[1], first.Bindings[0]}}
	second.Bindings[1].Consumers = []ResolvedConsumer{first.Bindings[0].Consumers[1], first.Bindings[0].Consumers[0]}
	second.Bindings[1].Value.Value = []byte("raw-key-two")

	left, err := BindingRevision(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BindingRevision(second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("revision changed with ordering or raw value: %q != %q", left, right)
	}
}

func TestBindingRevisionChangesWithPublicSemantics(t *testing.T) {
	base := ResolvedEnvironment{Bindings: []ResolvedBinding{{Kind: RequirementSecret, RequirementName: "credential", Profile: ProfileReference{Type: "Opaque", Name: "task", Version: "v1", Revision: "profile-a"}, SourceRevision: "source-a", Algorithm: "opaque/v1"}}}
	original, err := BindingRevision(base)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*ResolvedBinding){
		"profile":   func(binding *ResolvedBinding) { binding.Profile.Revision = "profile-b" },
		"source":    func(binding *ResolvedBinding) { binding.SourceRevision = "source-b" },
		"algorithm": func(binding *ResolvedBinding) { binding.Algorithm = "opaque/v2" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Bindings = append([]ResolvedBinding(nil), base.Bindings...)
			mutate(&changed.Bindings[0])
			revision, err := BindingRevision(changed)
			if err != nil {
				t.Fatal(err)
			}
			if revision == original {
				t.Fatalf("revision did not change: %q", revision)
			}
		})
	}
}
