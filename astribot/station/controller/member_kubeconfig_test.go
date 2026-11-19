package controller

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretKubeconfigSourceChecksOwnershipAndCopiesData(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "station-system", Name: MemberKubeconfigSecretName("cluster-a"), Annotations: map[string]string{ClusterIDAnnotation: "cluster-a"}},
		Data:       map[string][]byte{MemberKubeconfigSecretKey: []byte("kubeconfig")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	source := SecretKubeconfigSource{Reader: c, Namespace: "station-system"}
	data, err := source.Kubeconfig(context.Background(), "cluster-a")
	if err != nil || string(data) != "kubeconfig" {
		t.Fatalf("read kubeconfig=%q err=%v", data, err)
	}
	data[0] = 'X'
	if string(secret.Data[MemberKubeconfigSecretKey]) != "kubeconfig" {
		t.Fatal("returned kubeconfig aliases Secret data")
	}
	if _, err := source.Kubeconfig(context.Background(), "cluster-b"); err == nil {
		t.Fatal("accepted an absent or foreign-owned member Secret")
	}
}

func TestKubeconfigNodeReaderUsesVerifiedTLSAndRefreshesRotatedCredentials(t *testing.T) {
	currentUID := "node-uid-a"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/node-a" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]interface{}{"name": "node-a", "uid": currentUID},
			"status":   map[string]interface{}{"nodeInfo": map[string]interface{}{"systemUUID": "system-a"}},
		})
	}))
	defer server.Close()
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	data, err := clientcmd.Write(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{"member": {Server: server.URL, CertificateAuthorityData: caData}},
		Contexts:       map[string]*clientcmdapi.Context{"member": {Cluster: "member", AuthInfo: "token"}},
		CurrentContext: "member",
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"token": {Token: "short-lived-test-token"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := &mutableKubeconfigSource{data: data}
	reader := NewKubeconfigNodeReader(source)
	uid, systemUUID, exists, err := reader.VerifyNode(context.Background(), "cluster-a", "agent-a", "node-a")
	if err != nil || !exists || uid != "node-uid-a" || systemUUID != "system-a" {
		t.Fatalf("verify returned uid=%q systemUUID=%q exists=%t err=%v", uid, systemUUID, exists, err)
	}
	currentUID = "node-uid-recreated"
	uid, _, exists, err = reader.VerifyNode(context.Background(), "cluster-a", "agent-a", "node-a")
	if err != nil || !exists || uid != currentUID {
		t.Fatalf("reader returned stale Node UID %q, exists=%t err=%v", uid, exists, err)
	}
	source.data = []byte("invalid kubeconfig")
	if _, _, _, err := reader.VerifyNode(context.Background(), "cluster-a", "agent-a", "node-a"); err == nil {
		t.Fatal("accepted malformed rotated kubeconfig")
	}
	if _, _, _, err := reader.VerifyNode(context.Background(), "cluster-a", "", "node-a"); err == nil {
		t.Fatal("accepted missing cluster identity")
	}
}

func TestSecureRESTConfigRejectsUnverifiedOrHostPathCredentials(t *testing.T) {
	for _, test := range []struct {
		name    string
		cluster *clientcmdapi.Cluster
		user    *clientcmdapi.AuthInfo
	}{
		{name: "http API", cluster: &clientcmdapi.Cluster{Server: "http://member.example:6443"}, user: &clientcmdapi.AuthInfo{Token: "token"}},
		{name: "insecure TLS", cluster: &clientcmdapi.Cluster{Server: "https://member.example:6443", InsecureSkipTLSVerify: true}, user: &clientcmdapi.AuthInfo{Token: "token"}},
		{name: "exec plugin", cluster: &clientcmdapi.Cluster{Server: "https://member.example:6443"}, user: &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "external-auth"}}},
		{name: "host CA path", cluster: &clientcmdapi.Cluster{Server: "https://member.example:6443", CertificateAuthority: "/host/ca.crt"}, user: &clientcmdapi.AuthInfo{Token: "token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := clientcmd.Write(clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"member": test.cluster},
				Contexts:       map[string]*clientcmdapi.Context{"member": {Cluster: "member", AuthInfo: "user"}},
				CurrentContext: "member",
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"user": test.user},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := secureRESTConfigFromKubeconfig(data); err == nil {
				t.Fatal("accepted unsupported credential or transport configuration")
			}
		})
	}
}

type mutableKubeconfigSource struct{ data []byte }

func (s *mutableKubeconfigSource) Kubeconfig(context.Context, string) ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}
