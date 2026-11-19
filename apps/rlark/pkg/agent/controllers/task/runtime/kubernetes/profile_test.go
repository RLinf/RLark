package kubernetes

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type testProfileExecutor struct{ profile agentruntime.ObjectKind }

func (e testProfileExecutor) Profile() agentruntime.ObjectKind        { return e.profile }
func (testProfileExecutor) Validate(agentruntime.DesiredObject) error { return nil }
func (testProfileExecutor) Equal(agentruntime.TypedManifest, agentruntime.DesiredObject) (bool, error) {
	return true, nil
}
func (testProfileExecutor) Apply(client.Object, client.Object, agentruntime.TaskIdentity, agentruntime.DesiredObject) (bool, error) {
	return false, nil
}

func TestProfileRegistryRejectsDuplicateAndUnknownProfiles(t *testing.T) {
	profile := agentruntime.ObjectKind{Group: "test.io", Version: "v1", Kind: "Projection"}
	if _, err := NewProfileRegistry(testProfileExecutor{profile: profile}, testProfileExecutor{profile: profile}); !fault.Is(err, fault.Conflict) {
		t.Fatalf("duplicate error=%v", err)
	}
	registry, err := NewProfileRegistry(testProfileExecutor{profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	unknown := agentruntime.DesiredObject{ReconcileProfile: agentruntime.ObjectKind{Group: "test.io", Version: "v1", Kind: "Unknown"}}
	if err := registry.Validate(unknown); !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("unknown error=%v", err)
	}
}

func TestProfileRegistryDispatchesRegisteredExecutor(t *testing.T) {
	profile := agentruntime.ObjectKind{Group: "test.io", Version: "v1", Kind: "Projection"}
	registry, err := NewProfileRegistry(testProfileExecutor{profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	equal, err := registry.Equal(agentruntime.TypedManifest{}, agentruntime.DesiredObject{ReconcileProfile: profile})
	if err != nil || !equal {
		t.Fatalf("equal=%v error=%v", equal, err)
	}
}
