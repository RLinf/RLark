package task

import (
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var KUBERNETES_SUPPORT_SIDECAR_FEATURE = os.Getenv("KUBERNETES_SUPPORT_SIDECAR_FEATURE") != "false"

func findSidecar(podSpec *v1.PodSpec, sidecarName string) *v1.Container {
	for _, container := range podSpec.Containers {
		if container.Name == sidecarName {
			return &container
		}
	}
	for _, container := range podSpec.InitContainers {
		if container.Name == sidecarName {
			return &container
		}
	}
	return nil
}

func applySidecar(podSpec *v1.PodSpec, sidecarContainer v1.Container) {
	if findSidecar(podSpec, sidecarContainer.Name) != nil {
		return
	}
	if KUBERNETES_SUPPORT_SIDECAR_FEATURE {
		sidecarContainer.RestartPolicy = ptr.To(v1.ContainerRestartPolicyAlways)
		podSpec.InitContainers = append(podSpec.InitContainers, sidecarContainer)
	} else {
		podSpec.Containers = append(podSpec.Containers, sidecarContainer)
	}
}
