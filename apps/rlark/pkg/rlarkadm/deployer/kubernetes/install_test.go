package kubernetes

import (
	"context"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

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
