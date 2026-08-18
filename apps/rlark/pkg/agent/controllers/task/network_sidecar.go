package task

import (
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

const (
	sidecarContainerName  = "rlark-network-sidecar"
	sidecarUnixSocketPath = "/var/run/rlark"
	sidecarVolumeName     = "rlark-nodeserver-socket"
)

func applyNetworkSidecar(template *corev1.PodTemplateSpec, mgmtTask *rlarkv1alpha1.Task, image string) {
	if image == "" || mgmtTask.Spec.Domain == "" {
		return
	}

	if findSidecar(&template.Spec, sidecarContainerName) != nil {
		return
	}

	sidecar := corev1.Container{
		Name:            sidecarContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"network-sidecar"},
		Env: []corev1.EnvVar{
			{
				Name:  "LOG_LEVEL",
				Value: "debug",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: utils.Ptr(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"NET_ADMIN"},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      sidecarVolumeName,
				MountPath: sidecarUnixSocketPath,
			},
		},
	}
	applySidecar(&template.Spec, sidecar)

	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
		Name: sidecarVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: sidecarUnixSocketPath,
			},
		},
	})
}
