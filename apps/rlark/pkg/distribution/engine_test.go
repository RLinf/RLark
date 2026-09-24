package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgotesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticNamespaces struct {
	items []NamespaceIdentity
	err   error
}

func (s staticNamespaces) List(context.Context) ([]NamespaceIdentity, error) { return s.items, s.err }

type memoryInventoryStore struct {
	inventory Inventory
	saves     []Inventory
	deleted   bool
}

func (s *memoryInventoryStore) Load(context.Context, *corev1.Secret) (Inventory, error) {
	return s.inventory, nil
}

func (s *memoryInventoryStore) Save(_ context.Context, _ *corev1.Secret, inventory Inventory) error {
	s.inventory = inventory
	s.saves = append(s.saves, inventory)
	return nil
}

func (s *memoryInventoryStore) Delete(context.Context, *corev1.Secret) error {
	s.deleted = true
	return nil
}

func newTestEngine(t *testing.T, mode Mode, declaration *corev1.Secret, namespaces staticNamespaces) (*Engine, *dynamicfake.FakeDynamicClient, *memoryInventoryStore, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	declarationClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration).Build()
	targetClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "configmaps"}: "ConfigMapList",
		{Version: "v1", Resource: "namespaces"}: "NamespaceList",
	})
	installApplyReactor(t, targetClient)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
	store := &memoryInventoryStore{}
	return &Engine{
		DeclarationClient: declarationClient,
		TargetClient:      targetClient,
		TargetMapper:      mapper,
		Namespaces:        namespaces,
		InventoryStore:    store,
		Policy:            NoopPolicy{},
		Mode:              mode,
	}, targetClient, store, declarationClient
}

func installApplyReactor(t *testing.T, targetClient *dynamicfake.FakeDynamicClient) {
	t.Helper()
	targetClient.PrependReactor("patch", "*", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		patch := action.(clientgotesting.PatchAction)
		object := &unstructured.Unstructured{}
		require.NoError(t, json.Unmarshal(patch.GetPatch(), &object.Object))
		object.SetNamespace(action.GetNamespace())
		object.SetUID(types.UID("uid-" + action.GetNamespace() + "-" + object.GetName()))
		gvr := action.GetResource()
		current, err := targetClient.Tracker().Get(gvr, object.GetNamespace(), object.GetName())
		if err == nil {
			object.SetResourceVersion(current.(metav1.Object).GetResourceVersion())
			require.NoError(t, targetClient.Tracker().Update(gvr, object, object.GetNamespace()))
		} else {
			require.NoError(t, targetClient.Tracker().Create(gvr, object, object.GetNamespace()))
		}
		return true, object, nil
	})
}

func testDeclaration(secretType corev1.SecretType, mode Mode) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "definition", Namespace: "platform", UID: "definition-uid", Finalizers: []string{mode.CleanupFinalizer}}, Type: secretType, Data: validData()}
}

func TestEngineAppliesAndPrunesNamespacedObjects(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	declaration := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "replicate", Namespace: "platform", UID: "definition-uid", Finalizers: []string{ReplicationMode.CleanupFinalizer}}, Type: ReplicationSecretType, Data: validData()}
	declarationClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration).Build()
	targetClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "configmaps"}: "ConfigMapList",
	})
	targetClient.PrependReactor("patch", "configmaps", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		patch := action.(clientgotesting.PatchAction)
		object := &corev1.ConfigMap{}
		require.NoError(t, json.Unmarshal(patch.GetPatch(), object))
		object.Namespace = action.GetNamespace()
		object.UID = types.UID("local-uid")
		gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
		current, err := targetClient.Tracker().Get(gvr, object.Namespace, object.Name)
		if err == nil {
			object.ResourceVersion = current.(*corev1.ConfigMap).ResourceVersion
			require.NoError(t, targetClient.Tracker().Update(gvr, object, object.Namespace))
		} else {
			require.NoError(t, targetClient.Tracker().Create(gvr, object, object.Namespace))
		}
		return true, object, nil
	})
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	store := ConfigMapInventoryStore{Client: declarationClient, Mode: ReplicationMode}
	engine := Engine{
		DeclarationClient: declarationClient,
		TargetClient:      targetClient,
		TargetMapper:      mapper,
		Namespaces: staticNamespaces{items: []NamespaceIdentity{
			{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive},
			{Name: "other", UID: "ns-b", Phase: corev1.NamespaceActive},
		}},
		InventoryStore: store,
		Policy:         NoopPolicy{},
		Mode:           ReplicationMode,
	}

	result, err := engine.Reconcile(ctx, declaration.DeepCopy())
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	object, err := targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(ctx, "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "definition-uid", object.GetAnnotations()["replication.rlinf.io/definition-uid"])

	current := &corev1.Secret{}
	require.NoError(t, declarationClient.Get(ctx, types.NamespacedName{Namespace: "platform", Name: "replicate"}, current))
	data := validData()
	data["config"] = []byte("targets:\n  namespaces:\n    include: ['new-*']")
	current.Data = data
	require.NoError(t, declarationClient.Update(ctx, current))
	_, err = engine.Reconcile(ctx, current)
	require.NoError(t, err)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(ctx, "runtime-config", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestEngineRejectsClusterScopedTargetInReplicationMode(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	data := validData()
	data["manifests"] = []byte("- apiVersion: v1\n  kind: Namespace\n  metadata:\n    name: distributed")
	data["config"] = []byte("security:\n  allowClusterScoped: true")
	declaration := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "replicate", Namespace: "platform", UID: "definition-uid", Finalizers: []string{ReplicationMode.CleanupFinalizer}}, Type: ReplicationSecretType, Data: data}
	declarationClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(declaration).Build()
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
	engine := Engine{DeclarationClient: declarationClient, TargetClient: dynamicfake.NewSimpleDynamicClient(scheme), TargetMapper: mapper, InventoryStore: ConfigMapInventoryStore{Client: declarationClient, Mode: ReplicationMode}, Policy: NoopPolicy{}, Mode: ReplicationMode}

	_, err := engine.Reconcile(context.Background(), declaration.DeepCopy())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster-scoped target is not allowed")
}

func TestEngineIgnoresWrongSecretType(t *testing.T) {
	declaration := testDeclaration(corev1.SecretTypeOpaque, ReplicationMode)
	engine, targetClient, store, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	assert.Empty(t, result.Inventory.Items)
	assert.Empty(t, store.saves)
	assert.Empty(t, targetClient.Actions())
}

func TestEngineAddsFinalizerBeforeApplying(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	declaration.Finalizers = nil
	engine, targetClient, store, declarationClient := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}})

	result, err := engine.Reconcile(context.Background(), declaration.DeepCopy())
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Empty(t, targetClient.Actions())
	assert.Empty(t, store.saves)
	current := &corev1.Secret{}
	require.NoError(t, declarationClient.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	assert.Contains(t, current.Finalizers, ReplicationMode.CleanupFinalizer)
}

func TestEngineDoesNotAddFinalizerWhenNoResourcesAreManaged(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	declaration.Finalizers = nil
	engine, targetClient, store, declarationClient := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})

	result, err := engine.Reconcile(context.Background(), declaration.DeepCopy())
	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, "NoTargetsMatched", result.Inventory.Phase)
	assert.Empty(t, targetClient.Actions())
	require.Len(t, store.saves, 1)
	current := &corev1.Secret{}
	require.NoError(t, declarationClient.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	assert.NotContains(t, current.Finalizers, ReplicationMode.CleanupFinalizer)
}

func TestEngineKeepsFinalizerWhenPreviousInventoryNeedsPruning(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	declaration.Finalizers = nil
	engine, targetClient, store, declarationClient := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})
	owned := ownedConfigMap(declaration, ReplicationMode, "tenant-a", "old-uid")
	require.NoError(t, targetClient.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, owned, "tenant-a"))
	store.inventory = Inventory{Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", Name: "runtime-config", UID: "old-uid"}}}

	result, err := engine.Reconcile(context.Background(), declaration.DeepCopy())
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	current := &corev1.Secret{}
	require.NoError(t, declarationClient.Get(context.Background(), client.ObjectKeyFromObject(declaration), current))
	assert.Contains(t, current.Finalizers, ReplicationMode.CleanupFinalizer)
}

func TestEngineFiltersNamespaces(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	data := validData()
	data["config"] = []byte(`targets:
  namespaces:
    include: ["tenant-*"]
    exclude: ["tenant-disabled"]
requireTargetNamespaceLabels:
  managed: "true"
`)
	declaration.Data = data
	engine, _, _, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{items: []NamespaceIdentity{
		{Name: "tenant-a", UID: "a", Labels: map[string]string{"managed": "true"}, Phase: corev1.NamespaceActive},
		{Name: "tenant-disabled", UID: "b", Labels: map[string]string{"managed": "true"}, Phase: corev1.NamespaceActive},
		{Name: "tenant-unmanaged", UID: "c", Phase: corev1.NamespaceActive},
		{Name: "tenant-terminating", UID: "d", Labels: map[string]string{"managed": "true"}, Phase: corev1.NamespaceTerminating},
		{Name: "platform", UID: "e", Labels: map[string]string{"managed": "true"}, Phase: corev1.NamespaceActive},
	}})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	assert.Equal(t, "tenant-a", result.Inventory.Items[0].Namespace)
}

func TestEngineFailsClosedWhenNamespaceListFails(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	engine, targetClient, store, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{err: errors.New("list failed")})
	store.inventory = Inventory{Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", Name: "runtime-config", UID: "local-uid"}}}

	_, err := engine.Reconcile(context.Background(), declaration)
	require.ErrorContains(t, err, "list failed")
	assert.Equal(t, "TargetsResolutionFailed", store.inventory.Phase)
	assert.Len(t, store.inventory.Items, 1)
	for _, action := range targetClient.Actions() {
		assert.NotEqual(t, "delete", action.GetVerb())
	}
}

func TestEngineRejectsOwnershipConflict(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	engine, targetClient, store, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}})
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "runtime-config", "namespace": "tenant-a"},
	}}
	require.NoError(t, targetClient.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, existing, "tenant-a"))

	_, err := engine.Reconcile(context.Background(), declaration)
	require.ErrorContains(t, err, "ownership conflict")
	assert.Equal(t, "ApplyFailed", store.inventory.Phase)
}

func TestEngineOrphansRemovedTarget(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	data := validData()
	data["config"] = []byte("targets:\n  namespaces:\n    include: ['new-*']\napply:\n  deletionPolicy: Orphan")
	declaration.Data = data
	engine, targetClient, store, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})
	old := ownedConfigMap(declaration, ReplicationMode, "tenant-a", "old-uid")
	require.NoError(t, targetClient.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, old, "tenant-a"))
	store.inventory = Inventory{Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", NamespaceUID: "ns-a", Name: "runtime-config", UID: "old-uid"}}}

	_, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineAppliesClusterScopedDelivery(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config":    []byte("security:\n  allowClusterScoped: true"),
		"manifests": []byte("- apiVersion: v1\n  kind: Namespace\n  metadata:\n    name: delivered"),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{err: errors.New("must not list")})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	assert.Empty(t, result.Inventory.Items[0].Namespace)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(context.Background(), "delivered", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineClusterScopedBundleIgnoresNamespaceSelector(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte(`targets:
  namespaces:
    include: ["tenant-*"]
security:
  allowClusterScoped: true
`),
		"manifests": []byte("- apiVersion: v1\n  kind: Namespace\n  metadata:\n    name: delivered"),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{err: errors.New("must not list")})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(context.Background(), "delivered", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineExplicitNamespaceBundleDoesNotExpandSelector(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte("targets:\n  namespaces:\n    include: ['tenant-*']"),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: fixed
    namespace: shared
`),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "shared", UID: "ns-shared", Phase: corev1.NamespaceActive}}})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("shared").Get(context.Background(), "fixed", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineAppliesMixedScopeBundleWithNamespaceSelector(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte(`targets:
  namespaces:
    include: ["tenant-*"]
security:
  allowClusterScoped: true
`),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: runtime-config
- apiVersion: v1
  kind: Namespace
  metadata:
    name: shared
`),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: []NamespaceIdentity{
		{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive},
		{Name: "tenant-b", UID: "ns-b", Phase: corev1.NamespaceActive},
		{Name: "other", UID: "ns-c", Phase: corev1.NamespaceActive},
	}})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 3)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-b").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(context.Background(), "shared", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineAppliesNamespacedBundleWithoutSelector(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte("{}"),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: runtime-config
    namespace: tenant-a
`),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 1)
	assert.Equal(t, types.UID("ns-a"), result.Inventory.Items[0].NamespaceUID)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestEngineExplicitNamespaceOverridesSelector(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte("targets:\n  namespaces:\n    include: ['tenant-*']"),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: expanded
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: fixed
    namespace: shared
`),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: []NamespaceIdentity{
		{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive},
		{Name: "tenant-b", UID: "ns-b", Phase: corev1.NamespaceActive},
		{Name: "shared", UID: "ns-shared", Phase: corev1.NamespaceActive},
	}})

	result, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, result.Inventory.Items, 3)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("shared").Get(context.Background(), "fixed", metav1.GetOptions{})
	require.NoError(t, err)
	for _, namespace := range []string{"tenant-a", "tenant-b"} {
		_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(namespace).Get(context.Background(), "expanded", metav1.GetOptions{})
		require.NoError(t, err)
		_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(namespace).Get(context.Background(), "fixed", metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func TestEngineRejectsInvalidBundleNamespaceRules(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		manifests  string
		want       string
		namespaces []NamespaceIdentity
	}{
		{
			name: "missing explicit namespace", config: "{}",
			manifests: "- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: one", want: "requires metadata.namespace",
		},
		{
			name: "unknown explicit namespace", config: "{}",
			manifests: "- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: one\n    namespace: missing", want: "not active or does not exist",
		},
		{
			name: "cluster resource with namespace", config: "security:\n  allowClusterScoped: true",
			manifests: "- apiVersion: v1\n  kind: Namespace\n  metadata:\n    name: one\n    namespace: invalid", want: "must not set metadata.namespace",
		},
		{
			name: "duplicate identity", config: "{}",
			manifests: "- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: one\n    namespace: tenant-a\n- apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: one\n    namespace: tenant-a", want: "duplicate desired resource",
			namespaces: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declaration := testDeclaration(DeliverySecretType, DeliveryMode)
			declaration.Data = map[string][]byte{"config": []byte(tt.config), "manifests": []byte(tt.manifests)}
			engine, targetClient, store, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: tt.namespaces})
			_, err := engine.Reconcile(context.Background(), declaration)
			require.ErrorContains(t, err, tt.want)
			assert.Empty(t, targetClient.Actions())
			assert.Empty(t, store.inventory.Items)
		})
	}
}

func TestEngineRejectsClusterScopedResourceInsideReplicationBundle(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	declaration.Data = map[string][]byte{
		"config":    []byte("security:\n  allowClusterScoped: true"),
		"manifests": []byte("- apiVersion: v1\n  kind: Namespace\n  metadata:\n    name: shared"),
	}
	engine, _, _, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})
	_, err := engine.Reconcile(context.Background(), declaration)
	require.ErrorContains(t, err, "cluster-scoped target is not allowed")
}

func TestEnginePrunesRemovedBundleMember(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config": []byte("{}"),
		"manifests": []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: retained
    namespace: tenant-a
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: removed
    namespace: tenant-a
`),
	}
	engine, targetClient, store, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}})
	first, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, first.Inventory.Items, 2)

	declaration.Data["manifests"] = []byte(`
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: retained
    namespace: tenant-a
`)
	second, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	require.Len(t, second.Inventory.Items, 1)
	assert.Equal(t, "retained", second.Inventory.Items[0].Name)
	assert.Len(t, store.inventory.Items, 1)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(context.Background(), "removed", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestEngineCleanupVerifiesOwnershipAndRemovesFinalizer(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	now := metav1.Now()
	declaration.DeletionTimestamp = &now
	engine, targetClient, store, declarationClient := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{})
	owned := ownedConfigMap(declaration, ReplicationMode, "tenant-a", "old-uid")
	require.NoError(t, targetClient.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, owned, "tenant-a"))
	store.inventory = Inventory{Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", Name: "runtime-config", UID: "old-uid"}}}

	_, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	assert.True(t, store.deleted)
	current := &corev1.Secret{}
	err = declarationClient.Get(context.Background(), client.ObjectKeyFromObject(declaration), current)
	if err == nil {
		assert.NotContains(t, current.Finalizers, ReplicationMode.CleanupFinalizer)
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func ownedConfigMap(declaration *corev1.Secret, mode Mode, namespace string, uid types.UID) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{
			"name": "runtime-config", "namespace": namespace, "uid": string(uid),
			"annotations": map[string]any{mode.OwnershipPrefix + "definition-uid": string(declaration.UID)},
		},
	}}
}
