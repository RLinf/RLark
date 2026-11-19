package stationidentity

import (
	"context"
	"net/http"
	"testing"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestHTTPObservationBootstrapsStationAndThenBindsObjectUID(t *testing.T) {
	management := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		rlarkClusterGVR:       "ClusterList",
		stationListGVR:        "StationList",
		stationObservationGVR: "StationObservationList",
	}, makeCluster("cluster-a", "agent-a", false))
	member := k8sfake.NewSimpleClientset(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "station-agent", Namespace: "station-system", UID: "service-account-uid"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agent-pod", Namespace: "station-system", UID: "pod-uid"}, Spec: corev1.PodSpec{ServiceAccountName: "station-agent", NodeName: "node-a"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid"}},
	)
	member.PrependReactor("create", "tokenreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
		request := action.(ktesting.CreateAction).GetObject().(*authv1.TokenReview)
		if len(request.Spec.Audiences) != 1 || request.Spec.Audiences[0] != "station.rlark.io" {
			t.Fatalf("TokenReview audiences=%v", request.Spec.Audiences)
		}
		return true, &authv1.TokenReview{Status: authv1.TokenReviewStatus{
			Authenticated: true, Audiences: []string{"station.rlark.io"},
			User: authv1.UserInfo{
				Username: "system:serviceaccount:station-system:station-agent", UID: "service-account-uid",
				Extra: map[string]authv1.ExtraValue{
					"authentication.kubernetes.io/pod-name": {"agent-pod"},
					"authentication.kubernetes.io/pod-uid":  {"pod-uid"},
				},
			},
		}}, nil
	})
	credentials := memberCredentialSecret("cluster-a", "kubeconfig")
	credentials.Namespace = "station-member-credentials"
	registry := NewKubernetesClusterRegistry(management, k8sfake.NewSimpleClientset(credentials), "station-member-credentials", func([]byte) (kubernetes.Interface, error) {
		return member, nil
	})
	api := &API{
		Registry: registry,
		Observations: &KubernetesObservationSink{
			Client: management, Namespaces: KubernetesClusterNamespaceResolver{Client: management},
		},
	}
	observationBody := `{"cluster_id":"cluster-a","observation":{` + testAgent + `}}`
	if response := apiRequest(api, http.MethodPost, "/v1/observations", observationBody, "pod-token"); response.Code != http.StatusAccepted {
		t.Fatalf("bootstrap report status=%d body=%s", response.Code, response.Body.String())
	}
	bootstrap, err := management.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := bootstrap.GetAnnotations()[observationNodeUIDKey]; got != "node-uid" {
		t.Fatalf("bootstrap Node UID annotation=%q", got)
	}
	if got := bootstrap.GetAnnotations()[observationStationUIDKey]; got != "" {
		t.Fatalf("bootstrap unexpectedly claimed Station UID %q", got)
	}
	station := makeStation("station-a", "agent-a", "node-a", "node-uid", "station-object-uid", 1, false, "Bound")
	if _, err := management.Resource(stationListGVR).Namespace("agent-a").Create(context.Background(), station, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if response := apiRequest(api, http.MethodPost, "/v1/observations", observationBody, "pod-token"); response.Code != http.StatusAccepted {
		t.Fatalf("bound report status=%d body=%s", response.Code, response.Body.String())
	}
	bound, err := management.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.GetAnnotations()[observationStationUIDKey]; got != "station-object-uid" {
		t.Fatalf("bound Station UID annotation=%q", got)
	}
}
