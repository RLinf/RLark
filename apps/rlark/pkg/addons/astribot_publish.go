package addons

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	// ErrNotInCatalog is a version neither the catalog holds nor the cluster was ever given.
	ErrNotInCatalog = errors.New("not in the catalog")
	// ErrPublishedOtherwise is a version the cluster was given with other content than the catalog now holds.
	ErrPublishedOtherwise = errors.New("published with other content")
)

// Publish makes the Package of name at version (the catalog's current one when version is empty) available to
// the cluster whose management namespace packages are in, and returns the item's description. A cluster's
// agent renders an Addon from its published Package and from nothing else, so it is published before the Addon
// is written; whoever writes an Addon (the Gateway, the release command) publishes it so. The Package comes
// from the catalog; a version the catalog no longer holds is still what was published to the cluster when the
// Addon was installed, and stays usable there. An item's content at a version does not change.
func Publish(ctx context.Context, packages corev1client.ConfigMapInterface, cluster, name, version string) (ItemMeta, error) {
	item, inCatalog := Catalog.GetVersion(name, version)
	if !inCatalog {
		if version == "" {
			return ItemMeta{}, fmt.Errorf("addon %q: %w", name, ErrNotInCatalog)
		}
		published, err := packages.Get(ctx, PackageName(name, version), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return ItemMeta{}, fmt.Errorf("version %s of addon %q was never published to cluster %s: %w", version, name, cluster, ErrNotInCatalog)
		}
		if err != nil {
			return ItemMeta{}, err
		}
		if item, err = FromPackage(published.Data); err != nil {
			return ItemMeta{}, fmt.Errorf("package %s of cluster %s: %w", published.Name, cluster, err)
		}
		return item.Meta(), nil
	}
	desired := PackageOf(item).ConfigMap(cluster)
	published, err := packages.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = packages.Create(ctx, desired, metav1.CreateOptions{})
	} else if err == nil && !reflect.DeepEqual(published.Data, desired.Data) {
		return ItemMeta{}, fmt.Errorf("version %s of addon %q, cluster %s: give the changed item a new version: %w", item.Meta().Version, name, cluster, ErrPublishedOtherwise)
	}
	if err != nil {
		return ItemMeta{}, err
	}
	return item.Meta(), nil
}
