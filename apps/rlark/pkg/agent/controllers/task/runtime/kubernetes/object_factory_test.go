package kubernetes

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestObjectFactoryUsesTypedObjectsAndUnstructuredFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	provider := New(nil, scheme, nil)
	typed, err := provider.newObject("apps/v1, Kind=Deployment")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := typed.(*appsv1.Deployment); !ok {
		t.Fatalf("typed object=%T", typed)
	}
	dynamic, err := provider.newObject("custom.io/v1, Kind=Trainer")
	if err != nil {
		t.Fatal(err)
	}
	unstructuredObject, ok := dynamic.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("dynamic object=%T", dynamic)
	}
	if unstructuredObject.GroupVersionKind().String() != "custom.io/v1, Kind=Trainer" {
		t.Fatalf("dynamic object gvk=%s", unstructuredObject.GroupVersionKind())
	}
	list, err := provider.newObjectList(agentruntime.ObjectKind{Group: "custom.io", Version: "v1", Kind: "Trainer"})
	if err != nil {
		t.Fatal(err)
	}
	unstructuredList, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		t.Fatalf("dynamic list=%T", list)
	}
	if unstructuredList.GroupVersionKind().String() != "custom.io/v1, Kind=TrainerList" {
		t.Fatalf("dynamic list gvk=%s", unstructuredList.GroupVersionKind())
	}
}

func TestListObjectsExtractsTypedAndUnstructuredLists(t *testing.T) {
	typed, err := listObjects(&corev1.ServiceList{Items: []corev1.Service{{}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(typed) != 1 {
		t.Fatalf("typed objects=%d, want 1", len(typed))
	}
	if _, ok := typed[0].(*corev1.Service); !ok {
		t.Fatalf("typed object=%T, want *v1.Service", typed[0])
	}

	dynamicList := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{"apiVersion": "custom.io/v1", "kind": "Trainer"}}}}
	dynamic, err := listObjects(dynamicList)
	if err != nil {
		t.Fatal(err)
	}
	if len(dynamic) != 1 {
		t.Fatalf("dynamic objects=%d, want 1", len(dynamic))
	}
	if _, ok := dynamic[0].(*unstructured.Unstructured); !ok {
		t.Fatalf("dynamic object=%T, want *unstructured.Unstructured", dynamic[0])
	}
}
