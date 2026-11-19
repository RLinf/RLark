package delivery

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestrictedPolicy(t *testing.T) {
	policy := restrictedPolicy{}
	allowed := &meta.RESTMapping{Resource: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}}
	denied := &meta.RESTMapping{Resource: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}}
	object := &unstructured.Unstructured{}

	require.NoError(t, policy.Validate(context.Background(), &corev1.Secret{}, allowed, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{}))
	assert.Error(t, policy.Validate(context.Background(), &corev1.Secret{}, denied, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{}))
	assert.Error(t, policy.Validate(context.Background(), &corev1.Secret{}, allowed, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{ConflictPolicy: "Force"}))
}

func TestRestrictedPolicyRejectsSensitiveSecretAndAdoption(t *testing.T) {
	policy := restrictedPolicy{}
	mapping := &meta.RESTMapping{Resource: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}}
	object := &unstructured.Unstructured{Object: map[string]any{"kind": "Secret", "type": string(corev1.SecretTypeServiceAccountToken)}}

	assert.ErrorContains(t, policy.Validate(context.Background(), &corev1.Secret{}, mapping, distribution.NamespaceIdentity{}, object, distribution.ApplyPolicy{}), "service account token")
	assert.ErrorContains(t, policy.Validate(context.Background(), &corev1.Secret{}, mapping, distribution.NamespaceIdentity{}, &unstructured.Unstructured{}, distribution.ApplyPolicy{AdoptExisting: true}), "disabled")
}

func TestDeliveryPredicateOnlyMatchesDeliverySecrets(t *testing.T) {
	p := secretPredicate()
	assert.True(t, p.Create(event.CreateEvent{Object: &corev1.Secret{Type: distribution.DeliverySecretType}}))
	assert.False(t, p.Create(event.CreateEvent{Object: &corev1.Secret{Type: distribution.ReplicationSecretType}}))
	assert.False(t, p.Create(event.CreateEvent{Object: &corev1.Secret{}}))
}
