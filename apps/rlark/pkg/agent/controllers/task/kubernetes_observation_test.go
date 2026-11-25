package task

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/observation"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestKubernetesObservationsDrivePushReconcilers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	controller := NewTaskController(baseControllerForObservationTest(scheme))
	adapter := raykubernetes.NewAdapter(taskapi.WorkloadTypeRay, func(agentruntime.TaskIdentity) string { return "runtime" })
	info, err := adapter.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := host.NewImplementationRegistry(context.Background(), []host.WorkloadRegistration{{Key: host.ImplementationKey{WorkloadType: info.ID, WorkloadVersion: info.Version, RuntimeKind: agentruntime.Kind("Kubernetes")}, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := KubernetesObservationsFromImplementations(registry, agentruntime.Kind("Kubernetes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.WithKubernetesObservations(scheme, observationTestRESTMapper(), registrations); err != nil {
		t.Fatal(err)
	}
	controller.PushMaxConcurrentReconciles = 11
	scope := KubernetesObservationScope{RuntimeID: kubernetesruntime.DefaultID, Namespace: "runtime", ManagementNamespace: "management"}
	plans, err := validateObservationAccess(context.Background(), observationDependencyMapper(), &recordingAccessReviewer{}, scope, controller.observations, kubernetesruntime.BuiltinObservationProfiles())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := controller.newObservationRunner(&failedObservationCache{}, scope, plans)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.controllers) != 2 {
		t.Fatalf("push controllers=%d, want 2", len(runner.controllers))
	}
	if len(controller.AsKubePushReconcilers()) != 0 {
		t.Fatal("unscoped base path exposed Task push reconcilers")
	}
	want := map[string]bool{"Deployment": false, "StatefulSet": false}
	for _, plan := range plans {
		push, err := newPushRuntimeReconciler(controller, plan.root, scope, controller.LocalKubeClient)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := want[push.observation.GVK.Kind]; !found {
			t.Fatalf("unexpected observed kind %q", push.observation.GVK.Kind)
		}
		want[push.observation.GVK.Kind] = true
	}
	for kind, found := range want {
		if !found {
			t.Fatalf("missing %s observation", kind)
		}
	}
}

func baseControllerForObservationTest(scheme *runtime.Scheme) base.Controller {
	return base.Controller{LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).Build()}
}

func TestKubernetesObservationsFailClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	deployment := observation.Kubernetes{GVK: appsv1.SchemeGroupVersion.WithKind("Deployment"), CarrierType: taskapi.CarrierTypeKubernetesDeployment}
	tests := []struct {
		name          string
		registrations []observation.Kubernetes
		mapper        meta.RESTMapper
	}{
		{name: "empty"},
		{name: "duplicate gvk", registrations: []observation.Kubernetes{deployment, deployment}},
		{name: "duplicate carrier", registrations: []observation.Kubernetes{deployment, {GVK: appsv1.SchemeGroupVersion.WithKind("StatefulSet"), CarrierType: deployment.CarrierType}}},
		{name: "unknown gvk", registrations: []observation.Kubernetes{{GVK: schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Unknown"}, CarrierType: "example.io/unknown"}}, mapper: observationTestRESTMapper()},
		{name: "missing carrier", registrations: []observation.Kubernetes{{GVK: deployment.GVK}}},
		{name: "cluster scoped", registrations: []observation.Kubernetes{deployment}, mapper: clusterScopedObservationTestRESTMapper()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapper := test.mapper
			if mapper == nil {
				mapper = observationTestRESTMapper()
			}
			if _, _, err := newKubernetesObservations(scheme, mapper, test.registrations); err == nil {
				t.Fatal("expected registration validation error")
			}
		})
	}
}

func TestKubernetesObservationUsesUnstructuredForMappedCRD(t *testing.T) {
	scheme := runtime.NewScheme()
	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "TrainingJob"}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gvk.GroupVersion()})
	mapper.Add(gvk, meta.RESTScopeNamespace)

	observations, byID, err := newKubernetesObservations(scheme, mapper, []observation.Kubernetes{{GVK: gvk, CarrierType: "example.io/training-job"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("observations=%d, want 1", len(observations))
	}
	typeObject, ok := observations[0].typeObject.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("watch type=%T, want *unstructured.Unstructured", observations[0].typeObject)
	}
	if typeObject.GroupVersionKind() != gvk {
		t.Fatalf("watch GVK=%s, want %s", typeObject.GroupVersionKind(), gvk)
	}
	first := byID[gvk.String()].newObject()
	second := byID[gvk.String()].newObject()
	if first == second || first.GetObjectKind().GroupVersionKind() != gvk || second.GetObjectKind().GroupVersionKind() != gvk {
		t.Fatalf("object factory did not return independent %s objects", gvk)
	}
}

func TestUnstructuredKubernetesObservationEnqueuesRuntimeTrigger(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "TrainingJob"}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gvk.GroupVersion()})
	mapper.Add(gvk, meta.RESTScopeNamespace)
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "management", UID: types.UID("task-uid")},
		Spec: rlarkv1alpha1.TaskSpec{
			Runtime:  rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKind(rlarkv1alpha1.AgentTypeKubernetes)},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{Carrier: rlarkv1alpha1.TaskWorkloadCarrier{Type: "example.io/training-job"}},
		},
	}
	root := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "TrainingJob",
		"metadata": map[string]any{
			"name":      "root",
			"namespace": "runtime",
			"uid":       "root-uid",
			"annotations": map[string]any{
				ManagementTaskNameAnnotation:      task.Name,
				ManagementTaskNamespaceAnnotation: task.Namespace,
				ManagementTaskUIDAnnotation:       string(task.UID),
			},
		},
	}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(root).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	controller := NewTaskController(base.Controller{
		AgentType:        string(rlarkv1alpha1.AgentTypeKubernetes),
		LocalKubeClient:  local,
		ManagementClient: management,
	})
	if err := controller.WithKubernetesObservations(scheme, mapper, []observation.Kubernetes{{GVK: gvk, CarrierType: task.Spec.Workload.Carrier.Type}}); err != nil {
		t.Fatal(err)
	}
	push, err := newPushRuntimeReconciler(controller, controller.observations[0], KubernetesObservationScope{RuntimeID: kubernetesruntime.DefaultID, Namespace: "runtime", ManagementNamespace: "management"}, local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := push.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: root.GetName(), Namespace: root.GetNamespace()}}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-controller.triggers.events:
		if event.Object.GetName() != task.Name || event.Object.GetNamespace() != task.Namespace {
			t.Fatalf("trigger object=%s/%s", event.Object.GetNamespace(), event.Object.GetName())
		}
	case <-time.After(time.Second):
		t.Fatal("unstructured Runtime root did not enqueue a Task trigger")
	}
}

func clusterScopedObservationTestRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion})
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeRoot)
	return mapper
}

func observationTestRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion})
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("StatefulSet"), meta.RESTScopeNamespace)
	return mapper
}
