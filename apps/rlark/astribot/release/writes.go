package release

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

// ErrRefused is a write the management plane must not take as it is (a required value missing, a version
// the catalog does not have): asking again does not help.
var ErrRefused = errors.New("refused")

// PutAddon writes an Addon of a cluster at the version and values given as the Gateway writes one, as the command's
// own identity in kcp: its Package published first, its required values there (addons.Publish).
func (s *Site) PutAddon(ctx context.Context, cluster, name string, addon AddonState, install bool) error {
	managed, err := s.cluster(ctx, cluster)
	if err != nil {
		return err
	}
	namespace := managed.Spec.AgentNamespace
	client := s.Management.Resource(addonGVR).Namespace(namespace)
	var object *unstructured.Unstructured
	addonName := name
	if install {
		object = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "rlinf.io/v1alpha1", "kind": "Addon",
			"metadata": map[string]any{"name": name, "namespace": namespace}}}
	} else {
		if object, err = client.Get(ctx, name, metav1.GetOptions{}); err != nil {
			return err
		}
		addonName, _, _ = unstructured.NestedString(object.Object, "spec", "addonName")
	}
	meta, err := addons.Publish(ctx, s.Core.CoreV1().ConfigMaps(namespace), namespace, addonName, addon.Version)
	if errors.Is(err, addons.ErrNotInCatalog) || errors.Is(err, addons.ErrPublishedOtherwise) {
		return fmt.Errorf("%s/%s: %w: %w", cluster, name, ErrRefused, err)
	}
	if err != nil {
		return err
	}
	if missing := addons.MissingRequired(meta, addon.Values); len(missing) > 0 {
		return fmt.Errorf("%s/%s: %w: required parameters are missing: %v", cluster, name, ErrRefused, missing)
	}
	values := map[string]any{}
	for key, value := range addon.Values {
		values[key] = value
	}
	spec := map[string]any{"addonName": addonName, "version": meta.Version, "values": values}
	if err := unstructured.SetNestedMap(object.Object, spec, "spec"); err != nil {
		return err
	}
	if install {
		_, err = client.Create(ctx, object, metav1.CreateOptions{})
	} else {
		_, err = client.Update(ctx, object, metav1.UpdateOptions{})
	}
	return err
}
