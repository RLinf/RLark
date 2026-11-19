package component

import (
	"github.com/rlinf/rlark/apps/rlark/astribot/identity"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	corev1 "k8s.io/api/core/v1"
)

// kcpCredentialVolume mounts the component's own kcp credential (rlarkadm identity), whole and not by
// subPath, so a rotated token reaches the running Pod.
func kcpCredentialVolume(name string) ([]corev1.Volume, []corev1.VolumeMount) {
	secret := identity.RLarkSecretName(identity.Component{Name: name})
	return []corev1.Volume{{
			Name:         "kcp-credential",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}},
		}},
		[]corev1.VolumeMount{{Name: "kcp-credential", MountPath: constants.KCPCredentialDir, ReadOnly: true}}
}
