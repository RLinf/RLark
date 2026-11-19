package kubernetes_test

import (
	"context"
	"testing"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestRegistrationsSelectRayIndependentlyFromCarrier(t *testing.T) {
	runtimeKind := agentruntime.Kind("kubernetes")
	registrations := raykubernetes.Registrations(func(agentruntime.TaskIdentity) string { return "runtime" }, runtimeKind)
	if len(registrations) != 1 {
		t.Fatalf("registrations=%d, want 1", len(registrations))
	}
	registration := registrations[0]
	if string(registration.Type) != taskapi.WorkloadTypeRay || registration.Version != taskapi.WorkloadVersionRayV1 || registration.RuntimeKind != runtimeKind {
		t.Fatalf("registration=%#v", registration)
	}
	info, err := registration.Adapter.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(info.ID) != taskapi.WorkloadTypeRay || info.Version != taskapi.WorkloadVersionRayV1 {
		t.Fatalf("adapter info=%#v", info)
	}
}
