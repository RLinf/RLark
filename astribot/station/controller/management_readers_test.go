package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newManagementReader(t *testing.T, objects ...*unstructured.Unstructured) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(clusterGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(clusterListGVK, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(rlarkNodeGVK, &unstructured.Unstructured{})
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, object := range objects {
		builder = builder.WithObjects(object)
	}
	return builder
}

func TestManagementClusterResolverUsesAgentNamespaceAndRejectsDuplicates(t *testing.T) {
	cluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
		"metadata": map[string]interface{}{"name": "cluster-a"},
		"spec":     map[string]interface{}{"agentNamespace": "agent-a"},
	}}
	c := newManagementReader(t, cluster).Build()
	resolver := ManagementClusterResolver{Reader: c}
	registration, err := resolver.ResolveNamespace(context.Background(), "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if registration.ClusterID != "cluster-a" || registration.Namespace != "agent-a" {
		t.Fatalf("unexpected registration: %#v", registration)
	}
	if _, err := resolver.ResolveNamespace(context.Background(), "unclaimed"); err == nil {
		t.Fatal("resolved an unclaimed namespace")
	}

	duplicate := cluster.DeepCopy()
	duplicate.SetName("cluster-b")
	c = newManagementReader(t, cluster, duplicate).Build()
	if _, err := (ManagementClusterResolver{Reader: c}).ResolveNamespace(context.Background(), "agent-a"); err == nil {
		t.Fatal("accepted duplicate Cluster agentNamespace claims")
	}
}

func TestManagementNodeReadinessUsesConditionAndScope(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Node",
		"metadata": map[string]interface{}{"name": "worker-a", "namespace": "agent-a"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		}},
	}}
	c := newManagementReader(t, node).Build()
	reader := ManagementNodeReadiness{Reader: c}
	ready, err := reader.Ready(context.Background(), "cluster-a", "agent-a", "worker-a")
	if err != nil || !ready {
		t.Fatalf("ready node returned %t, %v", ready, err)
	}
	ready, err = reader.Ready(context.Background(), "cluster-a", "other-agent", "worker-a")
	if err != nil || ready {
		t.Fatalf("cross-namespace Node read returned %t, %v", ready, err)
	}
}
