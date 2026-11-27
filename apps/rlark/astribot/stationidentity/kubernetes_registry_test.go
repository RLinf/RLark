package stationidentity

import (
	"context"
	"errors"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func registryDynamic(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		rlarkClusterGVR:      "ClusterList",
		networkEnrollmentGVR: "NetworkEnrollmentList",
	}, objects...)
}

func makeCluster(name, namespace string, delisted bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
		"metadata": map[string]interface{}{"name": name},
		"spec":     map[string]interface{}{"agentNamespace": namespace, "delisted": delisted},
	}}
}

func memberCredentialSecret(clusterID, kubeconfig string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: stationmember.SecretName(clusterID), Namespace: "station-system",
		Annotations: map[string]string{memberClusterIDKey: clusterID},
	}, Data: map[string][]byte{
		memberKubeconfigKey:     []byte(kubeconfig),
		memberServiceAccountNS:  []byte("station-system"),
		memberServiceAccount:    []byte("station-agent"),
		memberServiceAccountUID: []byte("service-account-uid"),
		memberAudience:          []byte("station.rlark.io"),
	}}
}

func TestKubernetesClusterRegistryLoadsOnlyOwnedCurrentRegistration(t *testing.T) {
	management := registryDynamic(makeCluster("cluster-a", "agent-a", false))
	core := k8sfake.NewSimpleClientset(memberCredentialSecret("cluster-a", "test-kubeconfig"))
	factoryCalls := 0
	registry := NewKubernetesClusterRegistry(management, core, "station-system", func(kubeconfig []byte) (kubernetes.Interface, error) {
		factoryCalls++
		if string(kubeconfig) != "test-kubeconfig" {
			return nil, errors.New("unexpected kubeconfig")
		}
		return k8sfake.NewSimpleClientset(), nil
	})
	for i := 0; i < 2; i++ {
		reader, err := registry.Reader(context.Background(), "cluster-a")
		if err != nil {
			t.Fatal(err)
		}
		registered := reader.(registeredReader)
		if registered.registration.Namespace != "station-system" || registered.registration.ServiceAccount != "station-agent" || registered.registration.ServiceAccountUID != "service-account-uid" || registered.registration.Audience != "station.rlark.io" {
			t.Fatalf("unexpected registration: %#v", registered.registration)
		}
		if registered.client == nil {
			t.Fatal("registered reader is missing member client")
		}
	}
	if factoryCalls != 1 {
		t.Fatalf("member client was created %d times, want cached reuse", factoryCalls)
	}
}

func TestKubernetesClusterRegistryFailsClosedOnUnknownDelistedOrMisownedSecret(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cluster   *unstructured.Unstructured
		secret    *corev1.Secret
		wantErrIs error
	}{
		{name: "unknown cluster", cluster: makeCluster("other", "agent-a", false), wantErrIs: ErrClusterNotRegistered},
		{name: "delisted cluster", cluster: makeCluster("cluster-a", "agent-a", true), secret: memberCredentialSecret("cluster-a", "kubeconfig"), wantErrIs: ErrClusterNotRegistered},
		{name: "misowned Secret", cluster: makeCluster("cluster-a", "agent-a", false), secret: memberCredentialSecret("other", "kubeconfig")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			management := registryDynamic(tc.cluster)
			var secrets []runtime.Object
			if tc.secret != nil {
				secrets = append(secrets, tc.secret)
			}
			core := k8sfake.NewSimpleClientset(secrets...)
			registry := NewKubernetesClusterRegistry(management, core, "station-system", func([]byte) (kubernetes.Interface, error) { return k8sfake.NewSimpleClientset(), nil })
			_, err := registry.Reader(context.Background(), "cluster-a")
			if err == nil || (tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs)) {
				t.Fatalf("registry error = %v, wantErrIs %v", err, tc.wantErrIs)
			}
		})
	}
}

func TestKubernetesClusterRegistryRefreshesClientAfterSecretRotation(t *testing.T) {
	management := registryDynamic(makeCluster("cluster-a", "agent-a", false))
	core := k8sfake.NewSimpleClientset(memberCredentialSecret("cluster-a", "first"))
	factoryCalls := 0
	registry := NewKubernetesClusterRegistry(management, core, "station-system", func([]byte) (kubernetes.Interface, error) {
		factoryCalls++
		return k8sfake.NewSimpleClientset(), nil
	})
	if _, err := registry.Reader(context.Background(), "cluster-a"); err != nil {
		t.Fatal(err)
	}
	secret, err := core.CoreV1().Secrets("station-system").Get(context.Background(), stationmember.SecretName("cluster-a"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	secret.Data[memberKubeconfigKey] = []byte("rotated")
	if _, err := core.CoreV1().Secrets("station-system").Update(context.Background(), secret, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Reader(context.Background(), "cluster-a"); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 2 {
		t.Fatalf("member client factory calls = %d, want 2", factoryCalls)
	}
}

func TestSecureMemberRESTConfigRequiresVerifiedHTTPSAndInlineCredentials(t *testing.T) {
	for _, kubeconfig := range []string{
		"clusters: [{name: c, cluster: {server: http://member.example}}]\ncontexts: [{name: x, context: {cluster: c, user: u}}]\ncurrent-context: x\nusers: [{name: u, user: {token: token}}]\n",
		"clusters: [{name: c, cluster: {server: https://member.example, insecure-skip-tls-verify: true}}]\ncontexts: [{name: x, context: {cluster: c, user: u}}]\ncurrent-context: x\nusers: [{name: u, user: {token: token}}]\n",
		"clusters: [{name: c, cluster: {server: https://member.example}}]\ncontexts: [{name: x, context: {cluster: c, user: u}}]\ncurrent-context: x\nusers: [{name: u, user: {exec: {command: plugin}}}]\n",
		"clusters: [{name: c, cluster: {server: https://member.example}}]\ncontexts: [{name: x, context: {cluster: c, user: u}}]\ncurrent-context: x\nusers: [{name: u, user: {client-certificate: /host/cert, client-key: /host/key}}]\n",
	} {
		if _, err := secureMemberRESTConfig([]byte(kubeconfig)); err == nil {
			t.Fatal("accepted unsupported member kubeconfig")
		}
	}
	valid := "clusters: [{name: c, cluster: {server: https://member.example, certificate-authority-data: Y2E=}}]\ncontexts: [{name: x, context: {cluster: c, user: u}}]\ncurrent-context: x\nusers: [{name: u, user: {token: token}}]\n"
	config, err := secureMemberRESTConfig([]byte(valid))
	if err != nil || config.Host != "https://member.example" {
		t.Fatalf("config=%v err=%v", config, err)
	}
}

var _ dynamic.Interface = (*dynamicfake.FakeDynamicClient)(nil)
