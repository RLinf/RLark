package task

import (
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	sshServerInitContainerName = "rlark-sshd-init"
	sshServerVolumeName        = "rlark-sshd"
	sshServerBinPath           = "/usr/local/bin/rlark-sshd"
	sshServerBinDst            = "/sshd/rlark-sshd"
	sshServerDstDir            = "/sshd"
)

func applySSHServer(template *corev1.PodTemplateSpec, mgmtTask *rlarkv1alpha1.Task, image string) {
	if image == "" || mgmtTask.Spec.SSHPublicKey == "" {
		return
	}

	role := ""
	if mgmtTask.Annotations != nil {
		role = mgmtTask.Annotations[rlarkv1alpha1.RayRoleAnnotation]
	}
	if role != rlarkv1alpha1.RayRoleHead {
		return
	}

	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
		Name: sshServerVolumeName,
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
			Name:      sshServerVolumeName,
			MountPath: sshServerDstDir,
		})
		break
	}

	template.Spec.InitContainers = append(template.Spec.InitContainers, corev1.Container{
		Name:            sshServerInitContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"cp", sshServerBinPath, sshServerBinDst},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      sshServerVolumeName,
				MountPath: sshServerDstDir,
			},
		},
	})
}
