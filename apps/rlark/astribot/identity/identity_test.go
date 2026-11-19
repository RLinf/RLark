package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
)

// kcpWithTokens is a management plane whose token controller signs every token Secret it is given,
// each with a token of its own.
func kcpWithTokens() *fake.Clientset {
	kcp := fake.NewClientset()
	n := 0
	kcp.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		secret := action.(k8stesting.CreateAction).GetObject().(*corev1.Secret).DeepCopy()
		if secret.Type != corev1.SecretTypeServiceAccountToken {
			return false, nil, nil
		}
		n++
		secret.Name = secret.GenerateName + string(rune('a'+n-1))
		secret.CreationTimestamp = metav1.Unix(int64(n), 0)
		secret.Data = map[string][]byte{corev1.ServiceAccountTokenKey: []byte("token-" + secret.Name)}
		if err := kcp.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret, secret.Namespace); err != nil {
			return true, nil, err
		}
		return true, secret, nil
	})
	return kcp
}

var gateway = Component{
	Name: "rlark-gateway", Namespace: "rlark-system", ServiceAccount: "rlark-gateway",
	Rules: []rbacv1.PolicyRule{{APIGroups: []string{"rlinf.io"}, Resources: []string{"clusters"}, Verbs: []string{"get"}}},
}

// A component is given an account of its own, bound to its own role, and a kubeconfig that reads a
// token kcp signed for that account from the file beside it.
func TestIssueGivesAnIdentityOfItsOwn(t *testing.T) {
	ctx := context.Background()
	kcp := kcpWithTokens()
	cred, err := Issue(ctx, kcp, gateway, "https://kcp:6443/clusters/root", []byte("ca"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kcp.CoreV1().ServiceAccounts("rlark-system").Get(ctx, "rlark-gateway", metav1.GetOptions{}); err != nil {
		t.Fatalf("service account: %v", err)
	}
	role, err := kcp.RbacV1().ClusterRoles().Get(ctx, "rlark-gateway", metav1.GetOptions{})
	if err != nil || len(role.Rules) != 1 {
		t.Fatalf("cluster role = %+v, %v", role, err)
	}
	binding, err := kcp.RbacV1().ClusterRoleBindings().Get(ctx, "rlark-gateway", metav1.GetOptions{})
	if err != nil || binding.RoleRef.Name != "rlark-gateway" || binding.Subjects[0].Name != "rlark-gateway" || binding.Subjects[0].Namespace != "rlark-system" {
		t.Fatalf("binding = %+v, %v", binding, err)
	}

	dir := t.TempDir()
	if err := Dir(dir).Put(ctx, gateway, cred); err != nil {
		t.Fatal(err)
	}
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(dir, "rlark-gateway", KubeconfigKey))
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "https://kcp:6443/clusters/root" || string(config.CAData) != "ca" {
		t.Fatalf("config = %s %q", config.Host, config.CAData)
	}
	// client-go reads the token from this file again while it runs (every minute, and after a 401).
	if config.BearerTokenFile != filepath.Join(dir, "rlark-gateway", TokenKey) {
		t.Fatalf("token file = %q", config.BearerTokenFile)
	}
	token, _ := os.ReadFile(config.BearerTokenFile)
	if string(token) != "token-"+cred.Secret {
		t.Fatalf("token = %q", token)
	}

	// Issuing again gives the same token: nothing changes for a running component.
	again, err := Issue(ctx, kcp, gateway, "https://kcp:6443/clusters/root", []byte("ca"))
	if err != nil || again.Secret != cred.Secret {
		t.Fatalf("issued again: %v, %v", again, err)
	}
}

// A rotation adds a token and keeps the old one until it is retired.
func TestRotateThenRetire(t *testing.T) {
	ctx := context.Background()
	kcp := kcpWithTokens()
	first, err := Issue(ctx, kcp, gateway, "https://kcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Rotate(ctx, kcp, gateway, "https://kcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Secret == first.Secret || string(second.Token) == string(first.Token) {
		t.Fatal("rotation gave the same token")
	}
	tokens, _ := Tokens(ctx, kcp, gateway)
	if len(tokens) != 2 {
		t.Fatalf("tokens = %d, want both until the old one is retired", len(tokens))
	}
	retired, err := Retire(ctx, kcp, gateway, second.Secret)
	if err != nil || len(retired) != 1 || retired[0] != first.Secret {
		t.Fatalf("retired %v, %v", retired, err)
	}
	if again, _ := Issue(ctx, kcp, gateway, "https://kcp", nil); again.Secret != second.Secret {
		t.Fatalf("issued %s after the rotation, want %s", again.Secret, second.Secret)
	}
}

// A component bound to a role it does not own (the server, to cluster-admin) gets no role of its
// own, and a binding to another role is replaced, since a binding's role cannot change in place.
func TestBindingToAnExistingRole(t *testing.T) {
	ctx := context.Background()
	kcp := kcpWithTokens()
	server := Component{Name: "rlark-server", Namespace: "rlark-system", ServiceAccount: "rlark-server", ClusterRole: "view"}
	if err := Ensure(ctx, kcp, server); err != nil {
		t.Fatal(err)
	}
	server.ClusterRole = "cluster-admin"
	if err := Ensure(ctx, kcp, server); err != nil {
		t.Fatal(err)
	}
	if _, err := kcp.RbacV1().ClusterRoles().Get(ctx, "rlark-server", metav1.GetOptions{}); err == nil {
		t.Fatal("a role of its own was made")
	}
	binding, err := kcp.RbacV1().ClusterRoleBindings().Get(ctx, "rlark-server", metav1.GetOptions{})
	if err != nil || binding.RoleRef.Name != "cluster-admin" {
		t.Fatalf("binding = %+v, %v", binding, err)
	}
}

// In a cluster the credential is a Secret holding both files.
func TestSecretsStore(t *testing.T) {
	ctx := context.Background()
	cluster := fake.NewClientset()
	store := Secrets{Client: cluster, Namespace: "rlark-system", Name: func(c Component) string { return c.Name + "-kcp" }}
	for _, token := range []string{"one", "two"} {
		if err := store.Put(ctx, gateway, &Credential{Kubeconfig: []byte("kc"), Token: []byte(token)}); err != nil {
			t.Fatal(err)
		}
	}
	secret, err := cluster.CoreV1().Secrets("rlark-system").Get(ctx, "rlark-gateway-kcp", metav1.GetOptions{})
	if err != nil || string(secret.Data[TokenKey]) != "two" || string(secret.Data[KubeconfigKey]) != "kc" {
		t.Fatalf("secret = %+v, %v", secret, err)
	}
}

// A credential written again keeps who may read it: a component that runs as a user of its own was
// let read the first one, and a rotation must not take that away.
func TestDirKeepsWhoMayRead(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := Dir(dir)
	if err := store.Put(ctx, gateway, &Credential{Kubeconfig: []byte("kc"), Token: []byte("one")}); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(dir, "rlark-gateway", TokenKey)
	if info, _ := os.Stat(token); info.Mode().Perm() != 0o600 {
		t.Fatalf("a new token is %v, want 0600", info.Mode().Perm())
	}
	if err := os.Chmod(token, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, gateway, &Credential{Kubeconfig: []byte("kc"), Token: []byte("two")}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(token)
	data, _ := os.ReadFile(token)
	if info.Mode().Perm() != 0o644 || string(data) != "two" {
		t.Fatalf("token %q is %v, want two and 0644", data, info.Mode().Perm())
	}
}
