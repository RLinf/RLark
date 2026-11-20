package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

func declarationDigest(declaration *corev1.Secret) string {
	data, _ := json.Marshal(declaration.Data)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// replaceStorageClass takes the result of applying object. A StorageClass cannot be changed in place (parameters,
// reclaim policy), so one that exists and refuses the change is deleted and applied anew. It is only read when a
// volume is provisioned, so replacing it leaves existing volumes as they are.
func replaceStorageClass(ctx context.Context, target dynamic.ResourceInterface, existing, object *unstructured.Unstructured, data []byte, options metav1.PatchOptions, applied *unstructured.Unstructured, err error) (*unstructured.Unstructured, error) {
	if !apierrors.IsInvalid(err) || existing == nil || object.GetKind() != "StorageClass" {
		return applied, err
	}
	if err := target.Delete(ctx, object.GetName(), metav1.DeleteOptions{}); err != nil {
		return nil, err
	}
	return target.Patch(ctx, object.GetName(), types.ApplyPatchType, data, options)
}

// namespaceReplaced reports whether the namespace an item was recorded in has since been replaced (the cluster was
// rebuilt under the same id, or the namespace was deleted and created again). The item went away with that
// namespace: the object now under its name belongs to the new namespace and is not this item.
func (e *Engine) namespaceReplaced(ctx context.Context, item InventoryItem) (bool, error) {
	if item.Namespace == "" || item.NamespaceUID == "" {
		return false, nil
	}
	namespace, err := e.TargetClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, item.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return namespace.GetUID() != item.NamespaceUID, nil
}
