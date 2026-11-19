package stationidentity

import (
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// documentNode is a node as the route sees it: a WireGuard key and nothing else.
type documentNode struct {
	t   *testing.T
	key *ecdh.PrivateKey
}

func newDocumentNode(t *testing.T) documentNode {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return documentNode{t, key}
}

func (n documentNode) public() string {
	return base64.StdEncoding.EncodeToString(n.key.PublicKey().Bytes())
}

type documentAnswer struct {
	Challenge  string          `json:"challenge"`
	ServerKey  string          `json:"server_key"`
	Config     json.RawMessage `json:"config"`
	ConfigHash string          `json:"config_hash"`
}

// ask sends the node's key with a challenge and its proof, or with neither.
func (n documentNode) ask(server http.Handler, challenge, proof string) (int, documentAnswer) {
	n.t.Helper()
	body, _ := json.Marshal(documentRequest{PublicKey: n.public(), Challenge: challenge, Proof: proof})
	response := apiRequest(server, http.MethodPost, "/v1/network/document", string(body), "")
	var answer documentAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		n.t.Fatalf("answer %d %s: %v", response.Code, response.Body, err)
	}
	return response.Code, answer
}

// prove is what the gateway computes from a challenge.
func (n documentNode) prove(challenged documentAnswer) string {
	n.t.Helper()
	challenge, _ := base64.StdEncoding.DecodeString(challenged.Challenge)
	raw, _ := base64.StdEncoding.DecodeString(challenged.ServerKey)
	serverKey, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		n.t.Fatal(err)
	}
	shared, err := n.key.ECDH(serverKey)
	if err != nil {
		n.t.Fatal(err)
	}
	mac := hmac.New(sha256.New, shared)
	mac.Write([]byte(DocumentProofLabel))
	mac.Write(challenge)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func documentHolder(kind, namespace, name, phase, key, config string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": kind,
		"metadata": map[string]interface{}{"namespace": namespace, "name": name},
		"spec":     map[string]interface{}{"clusterID": namespace},
		"status":   map[string]interface{}{"phase": phase, "publicKey": key, "config": config, "configHash": "hash of " + config, "observedGeneration": int64(3)},
	}}
}

func testNetworkDocuments(t *testing.T, now *time.Time, objects ...runtime.Object) (*API, *dynamicfake.FakeDynamicClient) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{networkEnrollmentGVR: "NetworkEnrollmentList", nodeAdmissionGVR: "NodeAdmissionList", nodeClassGVR: "NodeClassList", machineGVR: "MachineList"}, objects...)
	documents, err := NewNetworkDocuments(client, EnrolledKeys(client))
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return *now }
	documents.Now, documents.Enrolled.Now, documents.Admitted.Now = clock, clock, clock
	// Through the API: the route asks for no token.
	return &API{Documents: documents}, client
}

func TestNetworkDocumentIsGivenForAProofOfTheNodesKey(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	node, stranger := newDocumentNode(t), newDocumentNode(t)
	api, client := testNetworkDocuments(t, &now, documentHolder("NetworkEnrollment", "farm", "s1u-05-x86", "Enrolled", node.public(), `{"interface":"st-wg0"}`))

	status, challenged := node.ask(api, "", "")
	if status != http.StatusUnauthorized || challenged.Challenge == "" || challenged.ServerKey == "" || challenged.Config != nil {
		t.Fatalf("first request: %d %+v", status, challenged)
	}
	status, answer := node.ask(api, challenged.Challenge, node.prove(challenged))
	if status != http.StatusOK || string(answer.Config) != `{"interface":"st-wg0"}` || answer.ConfigHash != `hash of {"interface":"st-wg0"}` {
		t.Fatalf("proof: %d %+v", status, answer)
	}

	// The document is the enrollment's of now, not of when the keys were listed.
	moved := documentHolder("NetworkEnrollment", "farm", "s1u-05-x86", "Enrolled", node.public(), `{"interface":"st-wg0","names":[]}`)
	if _, err := client.Resource(networkEnrollmentGVR).Namespace("farm").Update(context.Background(), moved, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, answer := node.ask(api, challenged.Challenge, node.prove(challenged)); status != http.StatusOK || string(answer.Config) != `{"interface":"st-wg0","names":[]}` {
		t.Fatalf("after the document changed: %d %+v", status, answer)
	}

	// A wrong proof, and another node's proof of the same challenge, are
	// answered like a first request.
	for name, proof := range map[string]string{"wrong": base64.StdEncoding.EncodeToString(make([]byte, 32)), "of another key": stranger.prove(challenged)} {
		if status, again := node.ask(api, challenged.Challenge, proof); status != http.StatusUnauthorized || again.Challenge == "" || again.Challenge == challenged.Challenge || again.Config != nil {
			t.Fatalf("a proof that is %s: %d %+v", name, status, again)
		}
	}

	// A challenge lasts a minute.
	now = now.Add(documentChallengeLife)
	if status, _ := node.ask(api, challenged.Challenge, node.prove(challenged)); status != http.StatusOK {
		t.Fatalf("a challenge a minute old: %d", status)
	}
	now = now.Add(time.Second)
	status, again := node.ask(api, challenged.Challenge, node.prove(challenged))
	if status != http.StatusUnauthorized || again.Challenge == "" {
		t.Fatalf("an expired challenge: %d %+v", status, again)
	}
	if status, _ := node.ask(api, again.Challenge, node.prove(again)); status != http.StatusOK {
		t.Fatalf("the challenge given for the expired one: %d", status)
	}

	// Any key is given a challenge; one that proves itself and is no node's
	// is told so, and no sooner.
	status, challenged = stranger.ask(api, "", "")
	if status != http.StatusUnauthorized || challenged.Challenge == "" {
		t.Fatalf("a stranger's first request: %d %+v", status, challenged)
	}
	if response := apiRequest(api, http.MethodPost, "/v1/network/document", `{"public_key":"`+stranger.public()+`","challenge":"`+challenged.Challenge+`","proof":"`+stranger.prove(challenged)+`"}`, ""); response.Code != http.StatusNotFound {
		t.Fatalf("a stranger's proof: %d %s", response.Code, response.Body)
	}
	if response := apiRequest(api, http.MethodPost, "/v1/network/document", `{"public_key":"c2hvcnQ="}`, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("no key: %d", response.Code)
	}
}

// A Station API started since the challenge was given does not know it.
func TestNetworkDocumentChallengeOfAnotherProcessIsAskedAgain(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	node := newDocumentNode(t)
	enrollment := documentHolder("NetworkEnrollment", "farm", "s1u-05-x86", "Enrolled", node.public(), `{"interface":"st-wg0"}`)
	before, _ := testNetworkDocuments(t, &now, enrollment)
	after, _ := testNetworkDocuments(t, &now, enrollment)
	_, challenged := node.ask(before, "", "")
	status, again := node.ask(after, challenged.Challenge, node.prove(challenged))
	if status != http.StatusUnauthorized || again.Challenge == "" || again.ServerKey == challenged.ServerKey {
		t.Fatalf("the other process: %d %+v", status, again)
	}
	if status, answer := node.ask(after, again.Challenge, node.prove(again)); status != http.StatusOK || string(answer.Config) != `{"interface":"st-wg0"}` {
		t.Fatalf("its own challenge: %d %+v", status, answer)
	}
}

// A machine that has redeemed its code and not yet enrolled has its document
// in its admission; an enrollment it left behind elsewhere does not speak for it.
func TestNetworkDocumentOfAMachineThatHasNotEnrolledIsItsAdmissions(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	node, joined := newDocumentNode(t), newDocumentNode(t)
	api, client := testNetworkDocuments(t, &now,
		documentHolder("NetworkEnrollment", "old", "desk", "Rejected", node.public(), `{"interface":"old"}`),
		documentHolder("NodeAdmission", "farm", "desk", "Redeemed", node.public(), `{"interface":"st-wg0"}`),
		// Joined: the admission holds no document any more.
		documentHolder("NodeAdmission", "farm", "s1u-05-x86", "Joined", joined.public(), ""))
	_, challenged := node.ask(api, "", "")
	if status, answer := node.ask(api, challenged.Challenge, node.prove(challenged)); status != http.StatusOK || string(answer.Config) != `{"interface":"st-wg0"}` {
		t.Fatalf("admitted: %d %+v", status, answer)
	}
	_, challenged = joined.ask(api, "", "")
	if response := apiRequest(api, http.MethodPost, "/v1/network/document", `{"public_key":"`+joined.public()+`","challenge":"`+challenged.Challenge+`","proof":"`+joined.prove(challenged)+`"}`, ""); response.Code != http.StatusNotFound {
		t.Fatalf("joined and not enrolled: %d %s", response.Code, response.Body)
	}
	// The nodes ask every 30 s; the objects are listed once for all of them.
	_, challenged = node.ask(api, "", "")
	for range 5 {
		if status, _ := node.ask(api, challenged.Challenge, node.prove(challenged)); status != http.StatusOK {
			t.Fatalf("asked again: %d", status)
		}
	}
	lists := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "list" {
			lists++
		}
	}
	if lists != 2 {
		t.Fatalf("%d lists of the two kinds for seven proofs", lists)
	}
}

// The same numbers stand in astribot/station-gateway (TestOwnDocumentProof):
// the two modules share no code, and the proof that program computes must be
// one this one takes. The server's key is the one of a process whose secret
// is 32 bytes of 0x42, for this challenge and this node.
const (
	vectorNodeKey   = "ERERERERERERERERERERERERERERERERERERERERERE="
	vectorChallenge = "AAAAAGtJ0gABAgMEBQYHCAkKCwwNDg8Q"
	vectorServerKey = "gUE5D2aX/M/z3wvCtPds4qmBUkN1tN4AHT3f576yRQI="
	vectorProof     = "Luhm9kDL1+eJcvRKcPOXv3rnD4va9kmRfHmfozQWXX4="
)

func TestNetworkDocumentProofIsTheGatewaysProof(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	raw, _ := base64.StdEncoding.DecodeString(vectorNodeKey)
	key, _ := ecdh.X25519().NewPrivateKey(raw)
	node := documentNode{t, key}
	api, _ := testNetworkDocuments(t, &now, documentHolder("NetworkEnrollment", "farm", "s1u-05-x86", "Enrolled", node.public(), `{}`))
	for i := range api.Documents.secret {
		api.Documents.secret[i] = 0x42
	}
	challenge, _ := base64.StdEncoding.DecodeString(vectorChallenge)
	if got := base64.StdEncoding.EncodeToString(api.Documents.serverKey(challenge, key.PublicKey().Bytes()).PublicKey().Bytes()); got != vectorServerKey {
		t.Fatalf("server key %s", got)
	}
	if got := node.prove(documentAnswer{Challenge: vectorChallenge, ServerKey: vectorServerKey}); got != vectorProof {
		t.Fatalf("proof %s", got)
	}
	if status, _ := node.ask(api, vectorChallenge, vectorProof); status != http.StatusOK {
		t.Fatalf("the gateway's proof: %d", status)
	}
}
