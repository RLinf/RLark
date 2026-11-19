package distribution

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineReplacesStorageClassWhoseImmutableFieldsChanged(t *testing.T) {
	declaration := testDeclaration(DeliverySecretType, DeliveryMode)
	declaration.Data = map[string][]byte{
		"config":    []byte("security:\n  allowClusterScoped: true"),
		"manifests": []byte("- apiVersion: storage.k8s.io/v1\n  kind: StorageClass\n  metadata:\n    name: sc-1\n  provisioner: rclone\n  reclaimPolicy: Delete"),
	}
	engine, targetClient, _, _ := newTestEngine(t, DeliveryMode, declaration, staticNamespaces{})
	gvr := schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
	engine.TargetMapper.(*meta.DefaultRESTMapper).Add(gvr.GroupVersion().WithKind("StorageClass"), meta.RESTScopeRoot)
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass", "provisioner": "rclone", "reclaimPolicy": "Retain",
		"metadata": map[string]any{"name": "sc-1", "annotations": map[string]any{DeliveryMode.OwnershipPrefix + "definition-uid": "definition-uid"}},
	}}
	require.NoError(t, targetClient.Tracker().Create(gvr, existing, ""))
	var actions []string
	targetClient.PrependReactor("*", "storageclasses", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		actions = append(actions, action.GetVerb())
		if action.GetVerb() == "patch" && len(actions) == 2 {
			return true, nil, apierrors.NewInvalid(schema.GroupKind{Group: "storage.k8s.io", Kind: "StorageClass"}, "sc-1", nil)
		}
		return false, nil, nil
	})

	_, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	assert.Equal(t, []string{"get", "patch", "delete", "patch"}, actions)
	replaced, err := targetClient.Resource(gvr).Get(context.Background(), "sc-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Delete", replaced.Object["reclaimPolicy"])
}

// The cluster was rebuilt under the same id (robot cluster, 11-13): the inventory still lists the object as it
// was in the old cluster's namespace, and the object now under that name was delivered into the new namespace.
func TestEnginePruneSkipsItemsOfAReplacedNamespace(t *testing.T) {
	declaration := testDeclaration(ReplicationSecretType, ReplicationMode)
	engine, targetClient, store, _ := newTestEngine(t, ReplicationMode, declaration, staticNamespaces{items: []NamespaceIdentity{{Name: "tenant-a", UID: "ns-a", Phase: corev1.NamespaceActive}}})
	namespace := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{"name": "tenant-a", "uid": "ns-a"},
	}}
	require.NoError(t, targetClient.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, namespace, ""))
	store.inventory = Inventory{Phase: "Applied", Items: []InventoryItem{{Version: "v1", Resource: "configmaps", Namespace: "tenant-a", NamespaceUID: "ns-old", Name: "runtime-config", UID: "old-uid"}}}

	_, err := engine.Reconcile(context.Background(), declaration)
	require.NoError(t, err)
	assert.Equal(t, "Applied", store.inventory.Phase)
	require.Len(t, store.inventory.Items, 1)
	assert.Equal(t, types.UID("ns-a"), store.inventory.Items[0].NamespaceUID)
	_, err = targetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("tenant-a").Get(context.Background(), "runtime-config", metav1.GetOptions{})
	require.NoError(t, err)
}
