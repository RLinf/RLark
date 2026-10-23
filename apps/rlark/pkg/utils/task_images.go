package utils

import (
	"sort"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// ExtractTaskImages returns the deduplicated, sorted list of container image
// references declared by a Task's common Pod template. Every Runtime consumes
// this model and decides which fields it supports; image discovery therefore
// does not branch on Runtime kind. The returned slice is sorted for
// deterministic ordering (stable logging and tests).
//
// This helper is shared between the data-plane image pre-puller (which decides
// what to pull) and the control-plane Task reconciler (which aggregates Node
// pull progress filtered by the Task's images). It is a pure function on the
// Task spec and does not touch any runtime state.
func ExtractTaskImages(task *rlarkv1alpha1.Task) []string {
	if task == nil || task.Spec.Workload.Template == nil {
		return nil
	}

	seen := make(map[string]struct{})
	collect := func(image string) {
		if image == "" {
			return
		}
		seen[image] = struct{}{}
	}

	podSpec := task.Spec.Workload.Template.Spec
	for i := range podSpec.InitContainers {
		collect(podSpec.InitContainers[i].Image)
	}
	for i := range podSpec.Containers {
		collect(podSpec.Containers[i].Image)
	}
	for i := range podSpec.EphemeralContainers {
		collect(podSpec.EphemeralContainers[i].Image)
	}

	images := make([]string, 0, len(seen))
	for img := range seen {
		images = append(images, img)
	}
	sort.Strings(images)
	return images
}
