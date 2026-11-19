package controller

import (
	"context"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMemberCredentialFollowsTheDeclaredName(t *testing.T) {
	credential := func(cluster, server, name string) *corev1.Secret {
		data, err := clientcmd.Write(clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"member": {Server: server, TLSServerName: name, CertificateAuthorityData: []byte("ca")}},
			AuthInfos:      map[string]*clientcmdapi.AuthInfo{"reader": {Token: "token"}},
			Contexts:       map[string]*clientcmdapi.Context{"member": {Cluster: "member", AuthInfo: "reader"}},
			CurrentContext: "member",
		})
		if err != nil {
			t.Fatal(err)
		}
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "credentials", Name: MemberKubeconfigSecretName(cluster), Annotations: map[string]string{ClusterIDAnnotation: cluster}},
			Data:       map[string][]byte{MemberKubeconfigSecretKey: data, "audience": []byte("station.rlark.io")},
		}
	}
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Names = []stationv1alpha1.SiteNetworkName{{Name: "robots.rlark.internal", Address: "10.254.240.5"}}
	// robot-01 is reached by a declared name, at the address the name had
	// before; robot-02 by a name nothing declares.
	reconciler, c := newNetworkTest(t, siteNetwork,
		credential("robot-01", "https://10.254.240.9:6443", "robots.rlark.internal"),
		credential("robot-02", "https://10.254.240.8:6443", "elsewhere.example"),
		// The release's, named its own way, without the cluster's annotation.
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "credentials", Name: "release-robot-01"}, Data: credential("robot-01", "https://10.254.240.9:6443", "robots.rlark.internal").Data},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "credentials", Name: "not-a-credential"}, Data: map[string][]byte{"token": []byte("x")}})
	reconciler.MemberCredentialNamespaces = []string{"credentials"}
	server := func(cluster string) (string, string) {
		var secret corev1.Secret
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "credentials", Name: MemberKubeconfigSecretName(cluster)}, &secret); err != nil {
			t.Fatal(err)
		}
		config, err := clientcmd.Load(secret.Data[MemberKubeconfigSecretKey])
		if err != nil {
			t.Fatal(err)
		}
		if string(secret.Data["audience"]) != "station.rlark.io" || config.AuthInfos["reader"].Token != "token" || string(config.Clusters["member"].CertificateAuthorityData) != "ca" {
			t.Fatalf("the credential of %s lost what it held", cluster)
		}
		return config.Clusters["member"].Server, secret.ResourceVersion
	}
	pass := func() {
		if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: testNetworkKey}); err != nil {
			t.Fatal(err)
		}
	}
	pass()
	moved, version := server("robot-01")
	if moved != "https://10.254.240.5:6443" {
		t.Fatalf("robot-01 is reached at %s", moved)
	}
	var release corev1.Secret
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "credentials", Name: "release-robot-01"}, &release); err != nil {
		t.Fatal(err)
	}
	if config, _ := clientcmd.Load(release.Data[MemberKubeconfigSecretKey]); config.Clusters["member"].Server != "https://10.254.240.5:6443" {
		t.Fatalf("the release's credential is at %s", config.Clusters["member"].Server)
	}
	if kept, _ := server("robot-02"); kept != "https://10.254.240.8:6443" {
		t.Fatalf("robot-02 is reached at %s", kept)
	}
	pass()
	if _, again := server("robot-01"); again != version {
		t.Fatal("a credential that is where its name is was written again")
	}

	// The control plane moves: one line of the declaration.
	var declared stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &declared); err != nil {
		t.Fatal(err)
	}
	declared.Spec.Names[0].Address = "10.254.240.7"
	if err := c.Update(context.Background(), &declared); err != nil {
		t.Fatal(err)
	}
	pass()
	if moved, _ := server("robot-01"); moved != "https://10.254.240.7:6443" {
		t.Fatalf("after the move robot-01 is reached at %s", moved)
	}
}
