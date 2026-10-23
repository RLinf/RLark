package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func newTestUserKey(t *testing.T) (gossh.PublicKey, []byte) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := gossh.NewPublicKey(pub)
	require.NoError(t, err)
	return key, gossh.MarshalAuthorizedKey(key)
}

func TestAuthenticateUserKey(t *testing.T) {
	key, encoded := newTestUserKey(t)
	_, other := newTestUserKey(t)
	for _, tt := range []struct {
		name     string
		username string
		data     map[string][]byte
		wantID   string
		missing  bool
		apiError bool
	}{
		{name: "registered user", username: "my-workstation", data: map[string][]byte{"my-workstation": encoded}, wantID: "my-workstation-0"},
		{name: "wrong username", username: "admin", data: map[string][]byte{"my-workstation": encoded}},
		{name: "key belongs to another user", username: "admin", data: map[string][]byte{"admin": other, "my-workstation": encoded}},
		{name: "invalid and blank lines", username: "my-workstation", data: map[string][]byte{"my-workstation": append([]byte("invalid\n\n"), encoded...)}, wantID: "my-workstation-2"},
		{name: "empty secret", username: "my-workstation"},
		{name: "missing secret", username: "my-workstation", missing: true},
		{name: "API unavailable", username: "my-workstation", apiError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			if !tt.missing {
				_, err := client.CoreV1().Secrets("management").Create(context.Background(), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: common.SSHUserKeySecretName, Namespace: "management"},
					Data:       tt.data,
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			if tt.apiError {
				client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, fmt.Errorf("API unavailable")
				})
			}
			config := DefaultConfig()
			config.KubeClientConfig.Namespace = "management"
			config.DBConfigPath = "/etc/rlark/db-config.yaml"
			// A configured database must not override the Gateway's key store.
			// Deliberately omit its connection: authentication must not query it.
			s := &Server{config: config, kubeClient: client, dbClient: &db.DB{}}
			id, err := s.authenticateUserKey(tt.username, key)
			if tt.apiError {
				require.ErrorContains(t, err, "API unavailable")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantID, id)
		})
	}
}

func TestAuthenticateUserKeyRotationAndDeletion(t *testing.T) {
	key, encoded := newTestUserKey(t)
	replacement, replacementEncoded := newTestUserKey(t)
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.SSHUserKeySecretName, Namespace: "default"},
		Data:       map[string][]byte{"my-workstation": encoded},
	})
	s := &Server{config: DefaultConfig(), kubeClient: client}
	id, err := s.authenticateUserKey("my-workstation", key)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	secret, err := client.CoreV1().Secrets("default").Get(context.Background(), common.SSHUserKeySecretName, metav1.GetOptions{})
	require.NoError(t, err)
	secret.Data["my-workstation"] = replacementEncoded
	_, err = client.CoreV1().Secrets("default").Update(context.Background(), secret, metav1.UpdateOptions{})
	require.NoError(t, err)
	id, err = s.authenticateUserKey("my-workstation", key)
	require.NoError(t, err)
	require.Empty(t, id)
	id, err = s.authenticateUserKey("my-workstation", replacement)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.NoError(t, client.CoreV1().Secrets("default").Delete(context.Background(), common.SSHUserKeySecretName, metav1.DeleteOptions{}))
	id, err = s.authenticateUserKey("my-workstation", replacement)
	require.NoError(t, err)
	require.Empty(t, id)
}

func TestAuthenticateUserKeyWithoutClient(t *testing.T) {
	key, _ := newTestUserKey(t)
	s := &Server{config: DefaultConfig()}
	id, err := s.authenticateUserKey("my-workstation", key)
	require.ErrorContains(t, err, "kubernetes client not available")
	require.Empty(t, id)
}
