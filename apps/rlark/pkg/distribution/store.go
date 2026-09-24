package distribution

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapInventoryStore struct {
	Client client.Client
	Mode   Mode
}

func (s ConfigMapInventoryStore) Load(ctx context.Context, declaration *corev1.Secret) (Inventory, error) {
	raw := declaration.Annotations[InventoryAnnotation]
	if raw == "" {
		var status corev1.ConfigMap
		if err := s.Client.Get(ctx, types.NamespacedName{Namespace: declaration.Namespace, Name: statusName(declaration)}, &status); err != nil {
			if apierrors.IsNotFound(err) {
				return Inventory{}, nil
			}
			return Inventory{}, err
		}
		raw = status.Data["status.json"]
	} else {
		decoded, err := base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return Inventory{}, fmt.Errorf("decode inventory annotation: %w", err)
		}
		raw = string(decoded)
	}
	var inventory Inventory
	if err := json.Unmarshal([]byte(raw), &inventory); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	return inventory, nil
}

func (s ConfigMapInventoryStore) Save(ctx context.Context, declaration *corev1.Secret, inventory Inventory) error {
	data, err := json.Marshal(inventory)
	if err != nil {
		return err
	}
	status := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      statusName(declaration),
		Namespace: declaration.Namespace,
		Labels: map[string]string{
			s.Mode.OwnershipPrefix + "status":         "true",
			s.Mode.OwnershipPrefix + "definition-uid": string(declaration.UID),
		},
		OwnerReferences: []metav1.OwnerReference{ownerReference(declaration)},
	}, Data: map[string]string{"status.json": string(data)}}
	var existing corev1.ConfigMap
	err = s.Client.Get(ctx, client.ObjectKeyFromObject(status), &existing)
	if apierrors.IsNotFound(err) {
		if err := s.Client.Create(ctx, status); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		status.ResourceVersion = existing.ResourceVersion
		if err := s.Client.Update(ctx, status); err != nil {
			return err
		}
	}

	current := &corev1.Secret{}
	if err := s.Client.Get(ctx, client.ObjectKeyFromObject(declaration), current); err != nil {
		return err
	}
	if current.UID != declaration.UID {
		return fmt.Errorf("declaration UID changed")
	}
	base := current.DeepCopy()
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[InventoryAnnotation] = base64.RawStdEncoding.EncodeToString(data)
	return s.Client.Patch(ctx, current, client.MergeFrom(base))
}

func (s ConfigMapInventoryStore) Delete(ctx context.Context, declaration *corev1.Secret) error {
	status := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: statusName(declaration), Namespace: declaration.Namespace}}
	if err := s.Client.Delete(ctx, status); client.IgnoreNotFound(err) != nil {
		return err
	}
	current := &corev1.Secret{}
	if err := s.Client.Get(ctx, client.ObjectKeyFromObject(declaration), current); err != nil {
		return client.IgnoreNotFound(err)
	}
	base := current.DeepCopy()
	delete(current.Annotations, InventoryAnnotation)
	if slices.EqualFunc(base.Finalizers, current.Finalizers, func(a, b string) bool { return a == b }) && len(base.Annotations) == len(current.Annotations) {
		return nil
	}
	return s.Client.Patch(ctx, current, client.MergeFrom(base))
}

func statusName(declaration *corev1.Secret) string {
	return declaration.Name + "-status"
}
