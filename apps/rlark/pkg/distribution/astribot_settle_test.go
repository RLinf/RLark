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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var targetConfigMaps = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

// writeCounter counts every write the engine sends to the declaration's cluster
// (the management plane): status ConfigMaps and the declaration itself.
type writeCounter struct{ writes []string }

func (w *writeCounter) client(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	record := func(verb string, object client.Object) {
		kind := "Secret"
		if _, ok := object.(*corev1.ConfigMap); ok {
			kind = "ConfigMap"
		}
		w.writes = append(w.writes, verb+" "+kind+" "+object.GetName())
	}
	return interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.CreateOption) error {
			record("create", object)
			return c.Create(ctx, object, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.UpdateOption) error {
			record("update", object)
			return c.Update(ctx, object, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, object client.Object, patch client.Patch, opts ...client.PatchOption) error {
			record("patch", object)
			return c.Patch(ctx, object, patch, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.DeleteOption) error {
			record("delete", object)
			return c.Delete(ctx, object, opts...)
		},
	})
}

func (w *writeCounter) take() []string {
	writes := w.writes
	w.writes = nil
	return writes
}

// newStoredEngine is an engine whose inventory goes through the real ConfigMap
// store into a management plane that counts writes.
func newStoredEngine(t *testing.T, mode Mode, declaration *corev1.Secret, namespaces staticNamespaces, stored ...client.Object) (*Engine, *writeCounter, client.Client) {
	t.Helper()
	engine, _, _, _ := newTestEngine(t, mode, declaration, namespaces)
	counter := &writeCounter{}
	management := counter.client(t, append([]client.Object{declaration}, stored...)...)
	engine.DeclarationClient = management
	engine.InventoryStore = ConfigMapInventoryStore{Client: management, Mode: mode}
	return engine, counter, management
}

// reconcileFresh reads the declaration back the way the controller does after a
// watch event and reconciles it.
func reconcileFresh(t *testing.T, engine *Engine, management client.Client, declaration *corev1.Secret) Result {
	t.Helper()
	current := &corev1.Secret{}
	require.NoError(t, management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	result, err := engine.Reconcile(context.Background(), current)
	require.NoError(t, err)
	return result
}

func storedInventory(t *testing.T, management client.Client, declaration *corev1.Secret) (string, string) {
	t.Helper()
	current := &corev1.Secret{}
	require.NoError(t, management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	annotation, err := base64.RawStdEncoding.DecodeString(current.Annotations[InventoryAnnotation])
	require.NoError(t, err)
	status := &corev1.ConfigMap{}
	require.NoError(t, management.Get(context.Background(), client.ObjectKey{Namespace: declaration.Namespace, Name: statusName(declaration)}, status))
	return string(annotation), status.Data["status.json"]
}

func TestEngineWritesNothingForAnUnchangedDeclaration(t *testing.T) {
	for _, mode := range []Mode{ReplicationMode, DeliveryMode} {
		t.Run(string(mode.SecretType), func(t *testing.T) {
			declaration := testDeclaration(mode.SecretType, mode)
			namespaces := staticNamespaces{items: []NamespaceIdentity{
				{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive},
				{Name: "tenant-b", UID: "ns-b", Phase: corev1.NamespaceActive},
			}}
			engine, counter, management := newStoredEngine(t, mode, declaration, namespaces)

			first := reconcileFresh(t, engine, management, declaration)
			require.Len(t, first.Inventory.Items, 2)
			assert.Equal(t, []string{"create ConfigMap definition-status", "patch Secret definition"}, counter.take())

			for range 10 {
				reconcileFresh(t, engine, management, declaration)
			}
			assert.Empty(t, counter.take())
		})
	}
}

func TestEngineRedeliversAChangedDeclarationAndSettlesAgain(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	namespaces := staticNamespaces{items: []NamespaceIdentity{
		{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive},
		{Name: "tenant-b", UID: "ns-b", Phase: corev1.NamespaceActive},
	}}
	engine, counter, management := newStoredEngine(t, DeliveryMode, declaration, namespaces)
	first := reconcileFresh(t, engine, management, declaration)
	counter.take()

	current := &corev1.Secret{}
	require.NoError(t, management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	current.Data["config"] = []byte("targets:\n  namespaces:\n    include: ['tenant-a']\napply:\n  mode: ServerSideApply")
	current.Data["manifests"] = []byte("- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: runtime-config\n  data:\n    mode: staging\n")
	require.NoError(t, management.Update(context.Background(), current))
	counter.take()

	second := reconcileFresh(t, engine, management, declaration)
	assert.NotEqual(t, first.Inventory.ObservedDeclarationDigest, second.Inventory.ObservedDeclarationDigest)
	require.Len(t, second.Inventory.Items, 1)
	assert.Equal(t, []string{"update ConfigMap definition-status", "patch Secret definition"}, counter.take())
	delivered, err := engine.TargetClient.Resource(targetConfigMaps).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "staging", delivered.Object["data"].(map[string]any)["mode"])
	_, err = engine.TargetClient.Resource(targetConfigMaps).Namespace("tenant-b").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "the target that left the selector is pruned")

	for range 5 {
		reconcileFresh(t, engine, management, declaration)
	}
	assert.Empty(t, counter.take())
}

func TestEngineCleansUpASettledDeclaration(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	namespaces := staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}}
	engine, _, management := newStoredEngine(t, ReplicationMode, declaration, namespaces)
	reconcileFresh(t, engine, management, declaration)
	reconcileFresh(t, engine, management, declaration)

	current := &corev1.Secret{}
	require.NoError(t, management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	require.NoError(t, management.Delete(context.Background(), current))
	require.NoError(t, management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	require.NotNil(t, current.DeletionTimestamp)
	_, err := engine.Reconcile(context.Background(), current)
	require.NoError(t, err)

	_, err = engine.TargetClient.Resource(targetConfigMaps).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	err = management.Get(context.Background(), client.ObjectKey{Namespace: declaration.Namespace, Name: statusName(declaration)}, &corev1.ConfigMap{})
	assert.True(t, apierrors.IsNotFound(err))
	err = management.Get(context.Background(), client.ObjectKeyFromObject(declaration), current)
	assert.True(t, apierrors.IsNotFound(err), "the finalizer is gone, so the declaration is deleted")
}

// Declarations written by the engine before the inventory carried a digest hold
// observedDeclarationResourceVersion in the annotation and the status
// ConfigMap. The first reconcile rewrites both once; after that it is quiet.
func TestEngineMigratesAResourceVersionInventoryOnce(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	declaration.ResourceVersion = ""
	items := []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", NamespaceUID: "ns-a", Name: "runtime-config", UID: "uid-tenant-a-runtime-config"}}
	legacy, err := json.Marshal(map[string]any{
		"observedDeclarationResourceVersion": "176684731",
		"observedRevision":                   inventoryRevision(items),
		"phase":                              "Applied",
		"items":                              items,
	})
	require.NoError(t, err)
	declaration.Annotations = map[string]string{InventoryAnnotation: base64.RawStdEncoding.EncodeToString(legacy)}
	status := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: statusName(declaration), Namespace: declaration.Namespace,
		Labels:          map[string]string{"replication.rlinf.io/status": "true", "replication.rlinf.io/definition-uid": string(declaration.UID)},
		OwnerReferences: []metav1.OwnerReference{ownerReference(declaration)},
	}, Data: map[string]string{"status.json": string(legacy)}}
	namespaces := staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}}
	engine, counter, management := newStoredEngine(t, ReplicationMode, declaration, namespaces, status)

	reconcileFresh(t, engine, management, declaration)
	assert.Equal(t, []string{"update ConfigMap definition-status", "patch Secret definition"}, counter.take())
	annotation, statusJSON := storedInventory(t, management, declaration)
	assert.Equal(t, annotation, statusJSON)
	assert.NotContains(t, annotation, "observedDeclarationResourceVersion")
	var migrated Inventory
	require.NoError(t, json.Unmarshal([]byte(annotation), &migrated))
	assert.Equal(t, declarationDigest(declaration), migrated.ObservedDeclarationDigest)
	assert.Equal(t, items, migrated.Items)

	for range 5 {
		reconcileFresh(t, engine, management, declaration)
	}
	assert.Empty(t, counter.take())
}
