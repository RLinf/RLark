package rlarkctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

func registrySecret(name, namespace, credential string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{common.ImageRegistryReplicationLabel: "true"}},
		Data:       map[string][]byte{common.ImageRegistryCredentialDataKey: []byte(credential)},
	}
}

func TestRegistryLoginPicksTheCredentialOfTheServer(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		registrySecret("ir-old-replication", "default", `{"version":"v1","registry":"10.11.14.128:5443","username":"old","password":"p-old"}`),
		registrySecret("ir-new-replication", "default", `{"version":"v1","registry":"registry.rlark.internal:5443","username":"rlark","password":"p-new"}`),
	)
	for _, server := range []string{"registry.rlark.internal:5443\n", "https://registry.rlark.internal:5443", "registry.rlark.internal:5443"} {
		user, password, err := registryLogin(context.Background(), clientset, server)
		if err != nil || user != "rlark" || password != "p-new" {
			t.Fatalf("%q: %q %q %v", server, user, password, err)
		}
	}
	if _, _, err := registryLogin(context.Background(), clientset, "elsewhere:5000"); err == nil || !strings.Contains(err.Error(), "credentials not found") {
		t.Fatalf("unknown server: %v", err)
	}
	// A pusher allowed to read only its own credential object names it and does not list.
	t.Setenv("RLARK_REGISTRY_CREDENTIAL", "ir-new")
	if user, password, err := registryLogin(context.Background(), clientset, "registry.rlark.internal:5443"); err != nil || user != "rlark" || password != "p-new" {
		t.Fatalf("by name: %q %q %v", user, password, err)
	}
}

// The private directory skopeo runs with names the helper but holds no secret.
func TestWritePushToolsKeepsTheLoginOutOfFiles(t *testing.T) {
	dir := t.TempDir()
	if err := writePushTools(dir, "/usr/local/bin/rlarkctl", SiteRegistry, "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"); err != nil {
		t.Fatal(err)
	}
	auth, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
	if string(auth) != `{"credHelpers":{"registry.rlark.internal:5443":"rlark"}}` {
		t.Fatalf("auth.json: %s", auth)
	}
	helper, _ := os.ReadFile(filepath.Join(dir, "docker-credential-rlark"))
	if !strings.Contains(string(helper), `exec "/usr/local/bin/rlarkctl" registry credential-helper "$@"`) {
		t.Fatalf("helper: %s", helper)
	}
	if info, _ := os.Stat(filepath.Join(dir, "docker-credential-rlark")); info.Mode().Perm() != 0o700 {
		t.Fatalf("helper mode %v", info.Mode())
	}
	if ca, _ := os.ReadFile(filepath.Join(dir, "certs", "ca.crt")); !strings.Contains(string(ca), "AAAA") {
		t.Fatalf("ca: %s", ca)
	}
}
