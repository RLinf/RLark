package kubernetes

import (
	"context"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRemoveManagementSecretFinalizers(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:       common.TLSCASecretName,
			Namespace:  "rlark-system",
			Finalizers: []string{"rlark.io/ca-secret-protection", "example.com/keep"},
		}},
	)
	if err := removeManagementSecretFinalizers(context.Background(), client, "rlark-system"); err != nil {
		t.Fatalf("removeManagementSecretFinalizers() error = %v", err)
	}
	secret, err := client.CoreV1().Secrets("rlark-system").Get(context.Background(), common.TLSCASecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(secret.Finalizers) != 1 || secret.Finalizers[0] != "example.com/keep" {
		t.Fatalf("finalizers = %#v, want unrelated finalizer preserved", secret.Finalizers)
	}
}
