package release

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// The record of the release a site runs, in its management plane: what a cluster made after it is given (the
// Gateway makes a cluster with these versions and refuses to while there is none). A run writes it when it is done;
// a run back writes back the one there was before.
const (
	// RecordNamespace and RecordName are where the record is: the Gateway reads it there.
	RecordNamespace = "rlark-system"
	RecordName      = "rlark-release-current"
)

// CurrentRelease is the record of release r built from tree, as environment e runs it: its name and commit, its
// images (component -> image), the registry its images are named by, every Addon of the tree's catalog with its
// version there and the values the release gives it (rendered), and the tree's recordedFiles as they are.
func CurrentRelease(e Environment, r Release, tree Tree) (map[string]string, error) {
	images, err := json.Marshal(r.Images)
	if err != nil {
		return nil, err
	}
	type addon struct {
		Version string            `json:"version"`
		Values  map[string]string `json:"values,omitempty"`
	}
	catalog := map[string]addon{}
	for name, version := range tree.AddonVersions {
		values, err := r.render(r.Addons[name].Values)
		if err != nil {
			return nil, fmt.Errorf("Addon %s: %w", name, err)
		}
		catalog[name] = addon{Version: version, Values: values}
	}
	addonVersions, err := json.Marshal(catalog)
	if err != nil {
		return nil, err
	}
	record := map[string]string{"release": r.Name, "commit": tree.Commit, "registry": e.Registry.Name, "images.json": string(images), "addons.json": string(addonVersions)}
	maps.Copy(record, tree.Files)
	return record, nil
}

func readRecord(ctx context.Context, core kubernetes.Interface) (map[string]string, error) {
	current, err := core.CoreV1().ConfigMaps(RecordNamespace).Get(ctx, RecordName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return current.Data, nil
}

func writeRecord(ctx context.Context, core kubernetes.Interface, record map[string]string) error {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RecordNamespace}}
	if _, err := core.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	configMaps := core.CoreV1().ConfigMaps(RecordNamespace)
	current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: RecordNamespace, Name: RecordName}, Data: record}
	_, err := configMaps.Create(ctx, current, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = configMaps.Update(ctx, current, metav1.UpdateOptions{})
	}
	return err
}
