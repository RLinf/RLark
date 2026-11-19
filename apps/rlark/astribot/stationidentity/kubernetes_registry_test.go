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
		rlarkClusterGVR: "ClusterList",
		stationListGVR:  "StationList",
	}, objects...)
}

func makeCluster(name, namespace string, delisted bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
		"metadata": map[string]interface{}{"name": name},
		"spec":     map[string]interface{}{"agentNamespace": namespace, "delisted": delisted},
	}}
}

func makeStation(name, namespace, nodeName, nodeUID, stationUID string, version int64, retired bool, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "Station",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace, "uid": stationUID},
		"spec": map[string]interface{}{
			"retired": retired,
			"binding": map[string]interface{}{"nodeName": nodeName, "nodeUID": nodeUID, "version": version},
		},
		"status": map[string]interface{}{"phase": phase},
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
		registered := reader.(registeredReader).cluster
		if registered.Registration.Namespace != "station-system" || registered.Registration.ServiceAccount != "station-agent" || registered.Registration.ServiceAccountUID != "service-account-uid" || registered.Registration.Audience != "station.rlark.io" {
			t.Fatalf("unexpected registration: %#v", registered.Registration)
		}
		if registered.Bindings == nil || registered.Client == nil {
			t.Fatal("registered reader is missing member client or binding resolver")
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

func TestKubernetesStationBindingResolverRequiresUniqueCurrentBinding(t *testing.T) {
	cluster := makeCluster("cluster-a", "agent-a", false)
	valid := makeStation("station-a", "agent-a", "node-a", "node-uid-a", "station-uid-a", 2, false, "Bound")
	resolver := KubernetesStationBindingResolver{Management: registryDynamic(cluster, valid)}
	binding, err := resolver.Resolve(context.Background(), "cluster-a", "node-a", "node-uid-a")
	if err != nil || binding.StationUID != "station-uid-a" || binding.Generation != 2 || binding.Inactive {
		t.Fatalf("binding=%#v err=%v", binding, err)
	}
	for _, tc := range []struct {
		name     string
		stations []*unstructured.Unstructured
		inactive bool
		valid    bool
	}{
		{name: "retired", stations: []*unstructured.Unstructured{makeStation("station-a", "agent-a", "node-a", "node-uid-a", "station-uid-a", 2, true, "Retired")}, inactive: true},
		{name: "a removed node's retired Station leaves the name to the machine that joined after it", stations: []*unstructured.Unstructured{makeStation("station-old", "agent-a", "node-a", "old-node-uid", "station-old-uid", 1, true, "Retired"), makeStation("station-new", "agent-a", "node-a", "node-uid-a", "station-new-uid", 1, false, "Bound")}, valid: true},
		{name: "conflict with current UID remains observable", stations: []*unstructured.Unstructured{makeStation("station-a", "agent-a", "node-a", "node-uid-a", "station-uid-a", 2, false, "Conflict")}, valid: true},
		{name: "ambiguous", stations: []*unstructured.Unstructured{valid, makeStation("station-b", "agent-a", "node-a", "node-uid-a", "station-uid-b", 3, false, "Bound")}},
		{name: "same name changed UID", stations: []*unstructured.Unstructured{makeStation("station-a", "agent-a", "node-a", "new-node-uid", "station-uid-a", 2, false, "Bound")}},
		{name: "same name replacement cannot bypass old binding", stations: []*unstructured.Unstructured{makeStation("station-old", "agent-a", "node-a", "old-node-uid", "station-old-uid", 1, false, "Conflict"), makeStation("station-new", "agent-a", "node-a", "node-uid-a", "station-new-uid", 1, false, "Bound")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := []runtime.Object{cluster}
			for _, station := range tc.stations {
				objects = append(objects, station)
			}
			r := KubernetesStationBindingResolver{Management: registryDynamic(objects...)}
			got, err := r.Resolve(context.Background(), "cluster-a", "node-a", "node-uid-a")
			if tc.inactive {
				if err != nil || !got.Inactive {
					t.Fatalf("inactive binding=%#v err=%v", got, err)
				}
				return
			}
			if tc.valid {
				if err != nil || got.Inactive {
					t.Fatalf("current conflicting binding is not observable: binding=%#v err=%v", got, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted stale or ambiguous binding: %#v", got)
			}
		})
	}
	unbound := KubernetesStationBindingResolver{Management: registryDynamic(cluster)}
	if _, err := unbound.Resolve(context.Background(), "cluster-a", "node-a", "node-uid-a"); !errors.Is(err, ErrStationBindingNotFound) {
		t.Fatalf("unbound node error=%v", err)
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
