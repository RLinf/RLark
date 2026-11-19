package stationidentity

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

const testPublicKey = "Rpk1Y6lpJXIGOdNcgLMYs0J6mBG+67d85HVJs+upaR4="

type testEnroller struct {
	requests []NetworkEnrollmentRequest
	answer   NetworkEnrollmentAnswer
	err      error
}

func (e *testEnroller) Enroll(_ context.Context, _ Evidence, request NetworkEnrollmentRequest) (NetworkEnrollmentAnswer, error) {
	e.requests = append(e.requests, request)
	return e.answer, e.err
}

func TestHTTPEnrollmentRequiresVerifiedBoundIdentity(t *testing.T) {
	reader := &fakeReader{evidence: validEvidence()}
	enroller := &testEnroller{answer: NetworkEnrollmentAnswer{State: "enrolled", Address: "10.254.240.6", Config: json.RawMessage(`{"interface":"st-wg0"}`), ConfigHash: "h"}}
	api := &API{Registry: testRegistry{readers: map[string]IdentityReader{"c": reader}}, Enrollments: enroller}
	body := `{"cluster_id":"c","public_key":"` + testPublicKey + `","addresses":["10.11.13.192"]}`

	w := apiRequest(api, http.MethodPost, "/v1/network/enrollments", body, "token")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var answer NetworkEnrollmentAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil || answer.State != "enrolled" || string(answer.Config) != `{"interface":"st-wg0"}` {
		t.Fatalf("answer %+v, %v", answer, err)
	}
	if len(enroller.requests) != 1 || enroller.requests[0].PublicKey != testPublicKey || enroller.requests[0].ObservedSource != "192.0.2.1" {
		t.Fatalf("stored request %+v", enroller.requests)
	}

	for name, test := range map[string]struct {
		body   string
		mutate func(*Evidence)
		status int
	}{
		"other cluster":  {`{"cluster_id":"other","public_key":"` + testPublicKey + `"}`, nil, http.StatusUnauthorized},
		"not a key":      {`{"cluster_id":"c","public_key":"c2hvcnQ="}`, nil, http.StatusBadRequest},
		"not an address": {`{"cluster_id":"c","public_key":"` + testPublicKey + `","addresses":["example.com"]}`, nil, http.StatusBadRequest},
		"unknown field":  {`{"cluster_id":"c","public_key":"` + testPublicKey + `","role":"Hub"}`, nil, http.StatusBadRequest},
		"station unbound": {body, func(e *Evidence) {
			e.StationUID, e.RegisteredStationUID, e.Generation, e.RegisteredGeneration = "", "", 0, 0
		}, http.StatusConflict},
	} {
		reader.evidence = validEvidence()
		if test.mutate != nil {
			test.mutate(&reader.evidence)
		}
		before := len(enroller.requests)
		if w := apiRequest(api, http.MethodPost, "/v1/network/enrollments", test.body, "token"); w.Code != test.status || len(enroller.requests) != before {
			t.Errorf("%s: status %d, stored %d new requests", name, w.Code, len(enroller.requests)-before)
		}
	}
}

func TestKubernetesNetworkEnrollerStoresRequestAndReturnsItsAnswer(t *testing.T) {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{networkEnrollmentGVR: "NetworkEnrollmentList"})
	enroller := &KubernetesNetworkEnroller{Client: client, Namespaces: observationNamespaceMap{"c": "agent-c"}}
	evidence := validEvidence()
	request := NetworkEnrollmentRequest{PublicKey: testPublicKey, Addresses: []string{"10.11.13.192"}, ObservedSource: "10.11.13.192"}
	resource := client.Resource(networkEnrollmentGVR).Namespace("agent-c")

	answer, err := enroller.Enroll(context.Background(), evidence, request)
	if err != nil || answer.State != "pending" {
		t.Fatalf("first request: %+v, %v", answer, err)
	}
	stored, err := resource.Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if key, _, _ := unstructured.NestedString(stored.Object, "spec", "publicKey"); key != testPublicKey || stored.GetAnnotations()[observationClusterIDKey] != "c" {
		t.Fatalf("stored %v", stored.Object)
	}
	if uid, _, _ := unstructured.NestedString(stored.Object, "spec", "stationUID"); uid != "s" {
		t.Fatalf("the enrollment is not tied to the verified Station: %v", stored.Object["spec"])
	}

	// station-controller answers this generation.
	stored.Object["status"] = map[string]interface{}{
		"observedGeneration": stored.GetGeneration(), "phase": "Enrolled", "publicKey": testPublicKey,
		"address": "10.254.240.5", "tunnelAddress": "10.254.240.5", "config": `{"interface":"st-wg0"}`, "configHash": "abc",
	}
	if _, err := resource.Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	answer, err = enroller.Enroll(context.Background(), evidence, request)
	if err != nil || answer.State != "enrolled" || answer.Address != "10.254.240.5" || string(answer.Config) != `{"interface":"st-wg0"}` || answer.ConfigHash != "abc" {
		t.Fatalf("answered request: %+v, %v", answer, err)
	}

	// A new key is a new request: the answer for the old key is not returned.
	request.PublicKey = "DwjZS7W8Yab4SZN62rtBuvKzrzWwoa6nHwObl1Ck2XM="
	answer, err = enroller.Enroll(context.Background(), evidence, request)
	if err != nil || answer.State != "pending" || len(answer.Config) != 0 {
		t.Fatalf("rotated key: %+v, %v", answer, err)
	}
	stored, _ = resource.Get(context.Background(), "node", metav1.GetOptions{})
	if key, _, _ := unstructured.NestedString(stored.Object, "spec", "publicKey"); key != request.PublicKey {
		t.Fatalf("rotated key was not stored: %v", stored.Object["spec"])
	}
	if answer, err = enroller.Enroll(context.Background(), evidence, request); err != nil || answer.State != "pending" {
		t.Fatalf("status for the previous key must not be served: %+v, %v", answer, err)
	}

	// Another cluster cannot take over the object.
	foreign := validEvidence()
	foreign.ClusterID, foreign.RegisteredClusterID = "d", "d"
	if _, err := (&KubernetesNetworkEnroller{Client: client, Namespaces: observationNamespaceMap{"d": "agent-c"}}).Enroll(context.Background(), foreign, request); err == nil {
		t.Fatal("foreign cluster took over an enrollment")
	}
}
