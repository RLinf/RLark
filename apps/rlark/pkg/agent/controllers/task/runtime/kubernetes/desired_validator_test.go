package kubernetes

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestValidateDesiredRequiresNoRuntimeIO(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kind := agentruntime.ObjectKind{Group: appsv1.GroupName, Version: appsv1.SchemeGroupVersion.Version, Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName:      "workload",
		Kind:             kind,
		Manifest:         agentruntime.TypedManifest{Type: kind, Object: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"task"}}`)},
		Ownership:        agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle:        agentruntime.LifecycleDeleteOnTaskCleanup,
		UpdateMode:       agentruntime.UpdateApplyInPlace,
		ReconcileProfile: kubernetesobjects.WorkloadReconcileProfile,
	}
	provider := New(nil, scheme, nil)
	if err := provider.ValidateDesired(object); err != nil {
		t.Fatal(err)
	}
	object.Manifest.Type.Kind = "Service"
	if err := provider.ValidateDesired(object); !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want InvalidArgument", err)
	}
}

func TestValidateDesiredRejectsNilProvider(t *testing.T) {
	var provider *Provider
	if err := provider.ValidateDesired(agentruntime.DesiredObject{}); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("error=%v", err)
	}
}
