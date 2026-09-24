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

	require.NoError(t, policy.Validate(context.Background(), &corev1.Secret{}, configMap, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{}))
	assert.ErrorContains(t, policy.Validate(context.Background(), &corev1.Secret{}, configMap, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{AdoptExisting: true}), "disabled")
}

func TestMapNamespaceReturnsOnlyReplicationSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
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
