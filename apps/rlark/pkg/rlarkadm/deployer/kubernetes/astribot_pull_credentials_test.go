package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// The credentials RLark gives a new cluster are its pull Secrets before its agent runs, under names of their own (the
// agent delivers each under the credential's name and adopts nothing), and the agent's Pod names both: its own
// image is pulled with the first. Put in again, they are replaced, not doubled.
func TestTheClustersCredentialsAreInBeforeItsAgent(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset()
	credentials := []types.ImagePullCredential{{Name: "ir-0123456789abcdef", DockerConfigJSON: `{"auths":{"registry.rlark.internal:5443":{"auth":"x"}}}`}}
	for range 2 {
		if err := putImagePullCredentials(context.Background(), client, credentials); err != nil {
			t.Fatal(err)
		}
	}
	secret, err := client.CoreV1().Secrets(constants.Namespace).Get(context.Background(), "ir-0123456789abcdef-install", metav1.GetOptions{})
	if err != nil || secret.Type != corev1.SecretTypeDockerConfigJson || string(secret.Data[corev1.DockerConfigJsonKey]) != credentials[0].DockerConfigJSON || secret.Labels[common.ImageRegistryCredentialLabel] != "" {
		t.Fatalf("secret %+v, %v", secret, err)
	}
	cfg := &types.DeployConfig{Plane: types.PlaneData, Kubernetes: &types.KubernetesEnv{AstribotKubernetesEnv: types.AstribotKubernetesEnv{ImagePullCredentials: credentials}}}
	for _, c := range component.ComponentsForPlane(cfg) {
		if c.Name != constants.ComponentAgent {
			continue
		}
		pulls := component.Deployment(cfg, &c).Spec.Template.Spec.ImagePullSecrets
		if len(pulls) != 2 || pulls[0].Name != "ir-0123456789abcdef-install" || pulls[1].Name != "ir-0123456789abcdef" {
			t.Fatalf("the agent pulls with %v", pulls)
		}
	}
}
