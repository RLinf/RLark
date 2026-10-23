package tenant

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIDOfDefaultsLegacyResources(t *testing.T) {
	legacy := &metav1.ObjectMeta{Name: "legacy"}
	if got := IDOf(legacy); got != DefaultID {
		t.Fatalf("IDOf() = %q, want %q", got, DefaultID)
	}
}

func TestInheritTenantMetadata(t *testing.T) {
	parent := &metav1.ObjectMeta{Labels: map[string]string{IDLabel: "team"}, Annotations: map[string]string{CreatedByAnnotation: "alice"}}
	child := &metav1.ObjectMeta{Labels: map[string]string{"keep": "true"}}
	Inherit(parent, child)
	if child.Labels[IDLabel] != "team" || child.Labels["keep"] != "true" || child.Annotations[CreatedByAnnotation] != "alice" {
		t.Fatalf("unexpected child metadata: labels=%v annotations=%v", child.Labels, child.Annotations)
	}
}

func TestStaticProviderDefaults(t *testing.T) {
	provider := StaticProvider{}
	namespace, err := provider.ResolveNamespace(context.Background(), "", "cluster")
	if err != nil || namespace != DefaultNamespace {
		t.Fatalf("ResolveNamespace() = %q, %v", namespace, err)
	}
}
