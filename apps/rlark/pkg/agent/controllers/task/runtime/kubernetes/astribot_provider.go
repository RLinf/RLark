package kubernetes

import (
	corev1 "k8s.io/api/core/v1"

	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
)

// keepPlatformImages gives desired's platform containers the images they have in current.
func keepPlatformImages(current, desired *corev1.PodTemplateSpec) {
	images := map[string]string{}
	for _, container := range append(append([]corev1.Container(nil), current.Spec.InitContainers...), current.Spec.Containers...) {
		if kubernetesobjects.PlatformContainer(container.Name) {
			images[container.Name] = container.Image
		}
	}
	for _, containers := range [][]corev1.Container{desired.Spec.InitContainers, desired.Spec.Containers} {
		for i := range containers {
			if image, runs := images[containers[i].Name]; runs {
				containers[i].Image = image
			}
		}
	}
}
