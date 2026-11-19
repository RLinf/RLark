package kubernetes

import (
	"context"
	"slices"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	corev1 "k8s.io/api/core/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"
)

func TestKubectlCommandUsesKubeconfig(t *testing.T) {
	cmd := kubectlCommand("/tmp/test-kubeconfig", "get", "pods")
	want := []string{"kubectl", "--kubeconfig", "/tmp/test-kubeconfig", "get", "pods"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("command args = %v, want %v", cmd.Args, want)
	}
}

func TestUIAuthSecretManifestPreservesSpecialCharacters(t *testing.T) {
	adminPassword := "#admin: password"
	userPassword := "user#password"
	signingKey := []byte("01234567890123456789012345678901")
	manifest, err := uiAuthSecretManifest(adminPassword, userPassword, signingKey)
	if err != nil {
		t.Fatalf("uiAuthSecretManifest() error = %v", err)
	}

	var secret corev1.Secret
	if err := yaml.Unmarshal(manifest, &secret); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if string(secret.Data[common.UIAuthAdminPasswordKey]) != adminPassword || string(secret.Data[common.UIAuthUserPasswordKey]) != userPassword {
		t.Fatalf("passwords were not preserved: %#v", secret.Data)
	}
	if string(secret.Data[common.UIAuthJWTSigningKey]) != string(signingKey) {
		t.Fatal("JWT signing key was not preserved")
	}
}

func TestKubectlCommandUsesDefaultLoadingRules(t *testing.T) {
	cmd := kubectlCommand("", "get", "pods")
	want := []string{"kubectl", "get", "pods"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("command args = %v, want %v", cmd.Args, want)
	}
}

func TestInstallCRDsToKubernetes(t *testing.T) {
	client := apiextensionsfake.NewSimpleClientset()
	if err := installCRDsToKubernetes(context.Background(), client); err != nil {
		t.Fatalf("installCRDsToKubernetes() error = %v", err)
	}
	crds, err := client.ApiextensionsV1().CustomResourceDefinitions().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list CRDs: %v", err)
	}
	if len(crds.Items) == 0 {
		t.Fatal("no CRDs installed")
	}
}

func TestEnsureUIAuthSecretInKubernetes(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset()
	ctx := context.Background()
	if err := ensureUIAuthSecretInKubernetes(ctx, client, constants.Namespace); err != nil {
		t.Fatalf("ensureUIAuthSecretInKubernetes() error = %v", err)
	}
	secret, err := client.CoreV1().Secrets(constants.Namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get UI auth secret: %v", err)
	}
	if len(secret.Data["admin-password"]) != 16 || len(secret.Data["user-password"]) != 16 {
		t.Fatalf("unexpected generated passwords: %#v", secret.Data)
	}
	admin := append([]byte(nil), secret.Data["admin-password"]...)
	if err := ensureUIAuthSecretInKubernetes(ctx, client, constants.Namespace); err != nil {
		t.Fatalf("second ensureUIAuthSecretInKubernetes() error = %v", err)
	}
	secret, err = client.CoreV1().Secrets(constants.Namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get preserved UI auth secret: %v", err)
	}
	if string(secret.Data["admin-password"]) != string(admin) {
		t.Fatal("existing UI auth secret was changed")
	}
}

func TestEnsureUIAuthSecretInKubernetesRepairsMissingFields(t *testing.T) {
	ctx := context.Background()
	signingKey := []byte("01234567890123456789012345678901")
	client := kubernetesfake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.UIAuthSecretName, Namespace: constants.Namespace},
		Data: map[string][]byte{
			common.UIAuthJWTSigningKey: signingKey,
		},
	})

	if err := ensureUIAuthSecretInKubernetes(ctx, client, constants.Namespace); err != nil {
		t.Fatalf("ensureUIAuthSecretInKubernetes() error = %v", err)
	}
	secret, err := client.CoreV1().Secrets(constants.Namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get repaired UI auth secret: %v", err)
	}
	if len(secret.Data[common.UIAuthAdminPasswordKey]) != 16 || len(secret.Data[common.UIAuthUserPasswordKey]) != 16 {
		t.Fatalf("missing passwords were not repaired: %#v", secret.Data)
	}
	if string(secret.Data[common.UIAuthJWTSigningKey]) != string(signingKey) {
		t.Fatal("valid JWT signing key was changed")
	}
}
