package kubernetes_test

import (
	"context"
	"testing"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	appsv1 "k8s.io/api/apps/v1"
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

func TestObservedRootsAreBoundedToSupportedCarriers(t *testing.T) {
	adapter := raykubernetes.NewAdapter(taskapi.WorkloadTypeRay, func(agentruntime.TaskIdentity) string { return "runtime" })
	info, err := adapter.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Observations) != 1 || info.Observations[0].RuntimeKind != agentruntime.Kind("Kubernetes") {
		t.Fatalf("observations=%#v", info.Observations)
	}
	roots := info.Observations[0].Roots
	if len(roots) != 2 {
		t.Fatalf("roots=%d, want 2", len(roots))
	}
	want := map[string]string{
		appsv1.SchemeGroupVersion.WithKind(raykubernetes.CarrierDeployment).String():  taskapi.CarrierTypeKubernetesDeployment,
		appsv1.SchemeGroupVersion.WithKind(raykubernetes.CarrierStatefulSet).String(): taskapi.CarrierTypeKubernetesStatefulSet,
	}
	for _, root := range roots {
		gvk := appsv1.SchemeGroupVersion.WithKind(root.Kind)
		carrier, found := want[gvk.String()]
		if !found || carrier != root.CarrierType {
			t.Fatalf("unexpected root=%#v", root)
		}
		delete(want, gvk.String())
	}
	if len(want) != 0 {
		t.Fatalf("missing roots=%v", want)
	}
}
