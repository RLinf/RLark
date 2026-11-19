package addon

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
)

// resolveStorage rewrites the volumes of a rendered workload that name an
// RLark storage, from the storage Views delivered to this cluster's namespace
// of the management plane. views is loaded on first use and reused for the
// Addon's other objects.
func (r *pullReconciler) resolveStorage(ctx context.Context, object *unstructured.Unstructured, cluster string, views *[]storageref.View) error {
	spec, path, err := storageref.PodSpecOf(object)
	if err != nil || spec == nil || !storageref.HasReferences(spec) {
		return err
	}
	if *views == nil {
		var configMaps corev1.ConfigMapList
		if err := r.c.ManagementClient.List(ctx, &configMaps, client.InNamespace(cluster), client.MatchingLabels{storageref.ViewLabel: "true"}); err != nil {
			return fmt.Errorf("list storage views: %w", err)
		}
		loaded := make([]storageref.View, 0, len(configMaps.Items))
		for i := range configMaps.Items {
			view, err := storageref.DecodeView(&configMaps.Items[i])
			if err != nil {
				return err
			}
			loaded = append(loaded, view)
		}
		*views = loaded
	}
	if err := storageref.Resolve(spec, *views, cluster); err != nil {
		return fmt.Errorf("%s %s: %w", object.GetKind(), object.GetName(), err)
	}
	return storageref.SetPodSpec(object, path, spec)
}
