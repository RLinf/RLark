package stationidentity

import (
	"context"
	"net/http"
	"testing"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// A node's first report makes its record, a report of another Node of the
// name is stored without taking the record, and neither Node enrolls until
// the record is its own.
func TestHTTPObservationMakesTheNodesRecord(t *testing.T) {
	management := registryDynamic(makeCluster("cluster-a", "agent-a", false))
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
	namespaces := KubernetesClusterNamespaceResolver{Client: management}
	api := &API{
		Registry:     registry,
		Observations: &KubernetesObservationSink{Client: management, Namespaces: namespaces},
		Enrollments:  &KubernetesNetworkEnroller{Client: management, Namespaces: namespaces},
	}
	enrollmentBody := `{"cluster_id":"cluster-a","public_key":"` + testPublicKey + `"}`
	if response := apiRequest(api, http.MethodPost, "/v1/network/enrollments", enrollmentBody, "pod-token"); response.Code != http.StatusConflict {
		t.Fatalf("enrollment before the first report: status=%d body=%s", response.Code, response.Body.String())
	}
	observationBody := `{"cluster_id":"cluster-a","observation":{"systemUUID":"system-a",` + testAgent + `}}`
	record := func() map[string]interface{} {
		t.Helper()
		if response := apiRequest(api, http.MethodPost, "/v1/observations", observationBody, "pod-token"); response.Code != http.StatusAccepted {
			t.Fatalf("report status=%d body=%s", response.Code, response.Body.String())
		}
		stored, err := management.Resource(networkEnrollmentGVR).Namespace("agent-a").Get(context.Background(), "node-a", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return stored.Object
	}
	first := record()
	spec := first["spec"].(map[string]interface{})
	if spec["nodeUID"] != "node-uid" || spec["systemUUID"] != "system-a" || spec["clusterID"] != "cluster-a" ||
		first["status"].(map[string]interface{})["observation"].(map[string]interface{})["nodeUID"] != "node-uid" {
		t.Fatalf("the record of a first report: %v", first)
	}
	if response := apiRequest(api, http.MethodPost, "/v1/network/enrollments", enrollmentBody, "pod-token"); response.Code != http.StatusOK {
		t.Fatalf("enrollment of the recorded Node: status=%d body=%s", response.Code, response.Body.String())
	}

	// The machine is reinstalled: the name's Node has another UID.
	if err := member.Tracker().Update(corev1.SchemeGroupVersion.WithResource("nodes"), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid-again"}}, ""); err != nil {
		t.Fatal(err)
	}
	again := record()
	spec = again["spec"].(map[string]interface{})
	if spec["nodeUID"] != "node-uid" || spec["publicKey"] != testPublicKey ||
		again["status"].(map[string]interface{})["observation"].(map[string]interface{})["nodeUID"] != "node-uid-again" {
		t.Fatalf("a report of another Node of the name: %v", again)
	}
	if response := apiRequest(api, http.MethodPost, "/v1/network/enrollments", enrollmentBody, "pod-token"); response.Code != http.StatusConflict {
		t.Fatalf("enrollment of a Node the record is not of: status=%d body=%s", response.Code, response.Body.String())
	}
}
