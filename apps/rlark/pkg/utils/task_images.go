package utils

import (
	"sort"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// ExtractTaskImages returns the deduplicated, sorted list of container image
// references referenced by a Task across its Kubernetes and Docker specs.
//
// Raw tasks carry no container images and are ignored. The returned slice is
// sorted for deterministic ordering (stable logging and tests).
//
// This helper is shared between the data-plane image pre-puller (which decides
// what to pull) and the control-plane Task reconciler (which aggregates Node
// pull progress filtered by the Task's images). It is a pure function on the
// Task spec and does not touch any runtime state.
func ExtractTaskImages(task *rlarkv1alpha1.Task) []string {
	if task == nil {
		return nil
	}

	seen := make(map[string]struct{})
	collect := func(image string) {
		if image == "" {
			return
		}
		seen[image] = struct{}{}
	}

	// Kubernetes workload: both init and main containers.
	if task.Spec.Kubernetes != nil && task.Spec.Kubernetes.Workload != nil {
		podSpec := task.Spec.Kubernetes.Workload.Template.Spec
		for i := range podSpec.InitContainers {
			collect(podSpec.InitContainers[i].Image)
		}
		for i := range podSpec.Containers {
			collect(podSpec.Containers[i].Image)
		}
	}

	// Docker task containers.
	if task.Spec.Docker != nil {
		for i := range task.Spec.Docker.Containers {
			collect(task.Spec.Docker.Containers[i].Image)
		}
	}

	images := make([]string, 0, len(seen))
	for img := range seen {
		images = append(images, img)
	}
	sort.Strings(images)
	return images
}
