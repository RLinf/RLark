package kubernetes

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// putImagePullCredentials puts in the image registry credentials RLark gave the cluster, before the agent that
// delivers them runs: its own image is pulled with them. The agent delivers each under the credential's own name and
// adopts nothing (its delivery policy), so the installer's copy has a name of its own, InstallCredentialName.
func putImagePullCredentials(ctx context.Context, clientset kubernetes.Interface, credentials []types.ImagePullCredential) error {
	for _, credential := range credentials {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: types.InstallCredentialName(credential.Name), Namespace: constants.Namespace},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(credential.DockerConfigJSON)},
		}
		existing, err := clientset.CoreV1().Secrets(constants.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, err = clientset.CoreV1().Secrets(constants.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		case err == nil:
			existing.Data, existing.Type = secret.Data, secret.Type
			_, err = clientset.CoreV1().Secrets(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("put image pull credential %s: %w", credential.Name, err)
		}
	}
	return nil
}
