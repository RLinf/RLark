package cluster

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestDeleteClusterResourcesRejectsEmptyAgentNamespace(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := Reconciler{Client: client}
	cluster := clusterWithSpec(rlarkv1alpha1.ClusterSpec{})

	pending, err := reconciler.deleteClusterResources(context.Background(), &cluster)
	if err == nil {
		t.Fatal("expected empty agent namespace to be rejected")
	}
	if pending {
		t.Fatal("cleanup should not be pending when agent namespace is invalid")
	}
	if !strings.Contains(err.Error(), "agent namespace is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
