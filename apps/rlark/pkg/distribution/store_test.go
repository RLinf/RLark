package distribution

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigMapInventoryStoreSaveLoadAndDelete(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	declaration := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "definition", Namespace: "platform", UID: "definition-uid", Finalizers: []string{ReplicationMode.CleanupFinalizer}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration).Build()
	store := ConfigMapInventoryStore{Client: c, Mode: ReplicationMode}
	want := Inventory{Phase: "Applied", Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", Name: "runtime-config", UID: "local-uid"}}}

	require.NoError(t, store.Save(ctx, declaration, want))
	current := &corev1.Secret{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(declaration), current))
	assert.NotEmpty(t, current.Annotations[InventoryAnnotation])
	status := &corev1.ConfigMap{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "platform", Name: "definition-status"}, status))
	assert.Equal(t, "true", status.Labels["replication.rlinf.io/status"])
	assert.Equal(t, "definition-uid", status.Labels["replication.rlinf.io/definition-uid"])

	got, err := store.Load(ctx, current)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	require.NoError(t, store.Delete(ctx, current))
	err = c.Get(ctx, client.ObjectKey{Namespace: "platform", Name: "definition-status"}, status)
	assert.True(t, apierrors.IsNotFound(err))
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(declaration), current))
	assert.Empty(t, current.Annotations[InventoryAnnotation])
}

func TestConfigMapInventoryStoreLoadsStatusFallback(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	declaration := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "definition", Namespace: "platform", UID: "definition-uid"}}
	want := Inventory{Phase: "Applied"}
	raw, err := json.Marshal(want)
	require.NoError(t, err)
	status := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "definition-status", Namespace: "platform"}, Data: map[string]string{"status.json": string(raw)}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration, status).Build()

	got, err := (ConfigMapInventoryStore{Client: c, Mode: ReplicationMode}).Load(ctx, declaration)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestConfigMapInventoryStoreRejectsCorruptOrStaleInventory(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	declaration := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "definition", Namespace: "platform", UID: "new-uid", Annotations: map[string]string{InventoryAnnotation: "not-base64"}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration).Build()
	store := ConfigMapInventoryStore{Client: c, Mode: ReplicationMode}
	_, err := store.Load(ctx, declaration)
	require.ErrorContains(t, err, "decode inventory annotation")

	delete(declaration.Annotations, InventoryAnnotation)
	raw, err := json.Marshal(Inventory{})
	require.NoError(t, err)
	declaration.Annotations[InventoryAnnotation] = base64.RawStdEncoding.EncodeToString(raw)
	stale := declaration.DeepCopy()
	stale.UID = "old-uid"
	require.ErrorContains(t, store.Save(ctx, stale, Inventory{}), "UID changed")
}
