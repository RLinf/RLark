package gateway

import (
	"context"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestManagementSecretsUseConfiguredNamespace(t *testing.T) {
	namespace := "rlark-system"
	gateway := &Gateway{
		config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: namespace}},
		rawClient: fake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: common.UIAuthSecretName, Namespace: namespace},
				Data: map[string][]byte{
					common.UIAuthAdminPasswordKey: []byte("admin"),
					common.UIAuthUserPasswordKey:  []byte("user"),
					common.UIAuthJWTSigningKey:    []byte("01234567890123456789012345678901"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: common.AdminCertSecretName, Namespace: namespace},
				Data:       map[string][]byte{"client.crt": []byte("cert"), "client.key": []byte("key")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: common.TLSCASecretName, Namespace: namespace},
				Data:       map[string][]byte{"ca.crt": []byte("ca")},
			},
		),
	}

	admin, user, err := gateway.readUIAuthSecret()
	if err != nil || admin != "admin" || user != "user" {
		t.Fatalf("readUIAuthSecret() = %q, %q, %v", admin, user, err)
	}
	cert, key, ca, err := gateway.getKCPAdminCerts()
	if err != nil || string(cert) != "cert" || string(key) != "key" || string(ca) != "ca" {
		t.Fatalf("getKCPAdminCerts() = %q, %q, %q, %v", cert, key, ca, err)
	}

	if _, err := gateway.rawClient.CoreV1().Secrets("default").Get(context.Background(), common.UIAuthSecretName, metav1.GetOptions{}); err == nil {
		t.Fatal("test unexpectedly found UI auth secret in default namespace")
	}
}
