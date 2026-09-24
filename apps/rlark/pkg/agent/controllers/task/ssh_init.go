package task

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	rlarkToolsInitContainerName = "rlark-tools-init"
	rlarkToolsVolumeName        = "rlark-tools"
	rlarkToolsBinPath           = "/usr/local/bin/rlark-tools"
	rlarkToolsBinDst            = "/rlark-tools/rlark-tools"
	rlarkToolsDstDir            = "/rlark-tools"
)

func applyRLarkTools(template *corev1.PodTemplateSpec, image string) {
	if image == "" {
		return
	}

	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
		Name: rlarkToolsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	for i := range template.Spec.Containers {
		c := &template.Spec.Containers[i]
		if c.Name != "main" {
			continue
		}
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name:      rlarkToolsVolumeName,
			MountPath: rlarkToolsDstDir,
		})
		break
	}

	template.Spec.InitContainers = append(template.Spec.InitContainers, corev1.Container{
		Name:            rlarkToolsInitContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"cp", rlarkToolsBinPath, rlarkToolsBinDst},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      rlarkToolsVolumeName,
				MountPath: rlarkToolsDstDir,
			},
		},
	})
}
