package replication

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretPredicate(t *testing.T) {
	p := secretPredicate(distribution.ReplicationSecretType)
	replication := &corev1.Secret{Type: distribution.ReplicationSecretType}
	delivery := &corev1.Secret{Type: distribution.DeliverySecretType}

	assert.True(t, p.Create(event.CreateEvent{Object: replication}))
	assert.False(t, p.Create(event.CreateEvent{Object: delivery}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: replication, ObjectNew: delivery}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: replication}))
	assert.True(t, p.Generic(event.GenericEvent{Object: replication}))
}

func TestRestrictedPolicy(t *testing.T) {
	policy := restrictedPolicy{}
	configMap := &meta.RESTMapping{}
	object := &unstructured.Unstructured{}
	canonical := distribution.ApplyPolicy{
		Mode:           distribution.ApplyModeServerSideApply,
		ConflictPolicy: distribution.ConflictPolicyFail,
		DeletionPolicy: distribution.DeletionPolicyDelete,
	}
	validate := func(object *unstructured.Unstructured, apply distribution.ApplyPolicy) error {
		return policy.Validate(context.Background(), &corev1.Secret{}, configMap, distribution.NamespaceIdentity{}, object, apply)
	}

	require.NoError(t, validate(object, canonical))

	adopting := canonical
	adopting.AdoptExisting = true
	assert.ErrorContains(t, validate(object, adopting), "disabled")

	forcing := canonical
	forcing.ConflictPolicy = distribution.ConflictPolicyForce
	assert.ErrorContains(t, validate(object, forcing), "disabled")
}

// A policy value that this implementation cannot interpret must be refused
// instead of being read as its closest known neighbour, and a missing desired
// object must be reported rather than dereferenced.
func TestRestrictedPolicyRejectsNonCanonicalInput(t *testing.T) {
	policy := restrictedPolicy{}
	configMap := &meta.RESTMapping{}
	object := &unstructured.Unstructured{}
	canonical := distribution.ApplyPolicy{
		Mode:           distribution.ApplyModeServerSideApply,
		ConflictPolicy: distribution.ConflictPolicyFail,
		DeletionPolicy: distribution.DeletionPolicyDelete,
	}
	validate := func(object *unstructured.Unstructured, apply distribution.ApplyPolicy) error {
		return policy.Validate(context.Background(), &corev1.Secret{}, configMap, distribution.NamespaceIdentity{}, object, apply)
	}

	assert.ErrorContains(t, validate(nil, canonical), "desired object is required")

	for name, mutate := range map[string]func(*distribution.ApplyPolicy){
		"empty mode":              func(p *distribution.ApplyPolicy) { p.Mode = "" },
		"unknown mode":            func(p *distribution.ApplyPolicy) { p.Mode = "ClientSideApply" },
		"empty conflict policy":   func(p *distribution.ApplyPolicy) { p.ConflictPolicy = "" },
		"unknown conflict policy": func(p *distribution.ApplyPolicy) { p.ConflictPolicy = "force" },
		"empty deletion policy":   func(p *distribution.ApplyPolicy) { p.DeletionPolicy = "" },
		"unknown deletion policy": func(p *distribution.ApplyPolicy) { p.DeletionPolicy = "Retain" },
	} {
		t.Run(name, func(t *testing.T) {
			apply := canonical
			mutate(&apply)
			assert.ErrorContains(t, validate(object, apply), "unsupported")
		})
	}
}

func TestMapNamespaceReturnsOnlyReplicationSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Secret{}, secretTypeField, indexSecretByType).
		WithObjects(
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "platform"}, Type: distribution.ReplicationSecretType},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "platform"}, Type: distribution.DeliverySecretType},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "other"}, Type: distribution.ReplicationSecretType},
		).Build()
	r := &Reconciler{Client: c}

	requests := r.mapNamespace(context.Background(), &corev1.Namespace{})
	require.Len(t, requests, 2)
	assert.ElementsMatch(t, []string{"platform/a", "other/c"}, []string{
		requests[0].Namespace + "/" + requests[0].Name,
		requests[1].Namespace + "/" + requests[1].Name,
	})
}

func TestNamespaceResolver(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-a", UID: "namespace-uid", Labels: map[string]string{"managed": "true"}},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}).Build()

	items, err := (namespaceResolver{client: c}).List(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "tenant-a", items[0].Name)
	assert.Equal(t, "namespace-uid", string(items[0].UID))
	assert.Equal(t, "true", items[0].Labels["managed"])
}
