package stationidentity

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func peerObservation(namespace, name string, reportedAt time.Time, peers ...map[string]interface{}) *unstructured.Unstructured {
	listed := make([]interface{}, len(peers))
	for i, peer := range peers {
		listed[i] = peer
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{"namespace": namespace, "name": name},
		"status":   map[string]interface{}{"reportedAt": reportedAt.UTC().Format(time.RFC3339Nano), "wireguard": map[string]interface{}{"interface": "st-wg0", "peers": listed}},
	}}
}

// A node that redeemed its code asks, with its own key, how far its key has reached the peers it dials: the peers'
// reports name it once their tunnels hold its key; the state it sends back keeps when each peer was first seen with it.
func TestPropagationAnswersThePeersReportsOfTheNode(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	node := newDocumentNode(t)
	hub, other := newDocumentNode(t).public(), newDocumentNode(t).public()
	document := `{"interface":"st-wg0","peers":[{"publicKey":"` + hub + `","endpoint":"1.2.3.4:51820","keepaliveSeconds":25},{"publicKey":"` + other + `","endpoint":"1.2.3.5:51820"},{"publicKey":"` + newDocumentNode(t).public() + `"}]}`
	api, _ := testNetworkDocuments(t, &now,
		documentHolder("NodeAdmission", "baidu", "gpu-2", "", node.public(), document),
		documentHolder("NetworkEnrollment", "ksyun", "10-0-2-57", "Enrolled", hub, "{}"),
		documentHolder("NetworkEnrollment", "office", "desk", "Enrolled", other, "{}"),
		peerObservation("ksyun", "10-0-2-57", now.Add(-10*time.Second), map[string]interface{}{"name": "baidu/gpu-2", "latestHandshake": now.Add(-time.Second).UTC().Format(time.RFC3339Nano)}),
		peerObservation("office", "desk", now),
	)
	ask := func(body map[string]any) (int, map[string]any) {
		t.Helper()
		body["public_key"] = node.public()
		data, _ := json.Marshal(body)
		response := apiRequest(api, http.MethodPost, "/v1/network/propagation", string(data), "")
		var answer map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &answer)
		if response.Code == http.StatusUnauthorized {
			body["challenge"] = answer["challenge"]
			body["proof"] = node.prove(documentAnswer{Challenge: answer["challenge"].(string), ServerKey: answer["server_key"].(string)})
			data, _ = json.Marshal(body)
			response = apiRequest(api, http.MethodPost, "/v1/network/propagation", string(data), "")
			answer = nil
			_ = json.Unmarshal(response.Body.Bytes(), &answer)
		}
		return response.Code, answer
	}
	status, answer := ask(map[string]any{})
	if status != http.StatusOK || answer["failure"] != nil || !strings.Contains(answer["message"].(string), "peer 拿到密钥 1/2（等 office/desk）") {
		t.Fatalf("%d %v", status, answer)
	}
	peers := answer["peers"].([]any)
	if len(peers) != 2 || peers[0].(map[string]any)["name"] != "ksyun/10-0-2-57" || peers[0].(map[string]any)["hasKey"] != true || peers[1].(map[string]any)["hasKey"] != false {
		t.Fatalf("peers %v", peers)
	}
	// Past the window, a peer whose fresh report still lacks the key is named.
	state := answer["state"].(map[string]any)
	now = now.Add(PropagationKeyWindow + time.Second)
	if status, answer = ask(map[string]any{"state": state}); status != http.StatusOK || !strings.Contains(answer["failure"].(string), "office/desk 在") {
		t.Fatalf("%d %v", status, answer)
	}
}

func TestJudgePropagationWaitsOnEvidenceAndFailsOnIt(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	seen := func(at time.Time) *time.Time { return &at }
	peers := func(now time.Time, keyed ...bool) []PeerPropagation {
		var out []PeerPropagation
		for i, has := range keyed {
			out = append(out, PeerPropagation{Name: []string{"a/hub-1", "b/hub-2"}[i], PublicKey: []string{"k1", "k2"}[i], ObservedAt: seen(now.Add(-5 * time.Second)), HasKey: has})
		}
		return out
	}
	state := &PropagationState{Since: start}
	// Peers take the key one by one: no failure while they do.
	if message, err := JudgePropagation(state, peers(start.Add(30*time.Second), true, false), nil, 25*time.Second, "1.2.3.4:51821", start.Add(30*time.Second)); err != nil || !strings.Contains(message, "1/2（等 b/hub-2）") {
		t.Fatalf("%q %v", message, err)
	}
	if _, err := JudgePropagation(state, peers(start.Add(90*time.Second), true, true), nil, 25*time.Second, "1.2.3.4:51821", start.Add(90*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Every peer has the key and none shakes hands within two keepalives and 15 s of the last one taking it.
	_, err := JudgePropagation(state, peers(start.Add(156*time.Second), true, true), nil, 25*time.Second, "1.2.3.4:51821", start.Add(156*time.Second))
	if err == nil || !strings.Contains(err.Error(), "UDP 1.2.3.4:51821") {
		t.Fatalf("err %v", err)
	}
	// A handshake of the node's own ends that.
	if message, err := JudgePropagation(state, peers(start.Add(156*time.Second), true, true), map[string]time.Time{"k1": start.Add(150 * time.Second)}, 25*time.Second, "", start.Add(156*time.Second)); err != nil || !strings.Contains(message, "握手：a/hub-1") {
		t.Fatalf("%q %v", message, err)
	}
	// Peers without a fresh report are waited for, up to PropagationLongest.
	silent := []PeerPropagation{{Name: "c/hub-3", PublicKey: "k3"}}
	quiet := &PropagationState{Since: start}
	if _, err := JudgePropagation(quiet, silent, nil, 0, "", start.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := JudgePropagation(quiet, silent, nil, 0, "", start.Add(PropagationLongest+time.Second)); err == nil || !strings.Contains(err.Error(), "无报告：c/hub-3") {
		t.Fatalf("err %v", err)
	}
	// A silent peer's last report of a handshake, with an earlier machine of the same name, says nothing of now.
	stale := []PeerPropagation{{Name: "c/hub-3", PublicKey: "k3", ObservedAt: seen(start.Add(-time.Hour)), HasKey: true, Handshake: seen(start.Add(-time.Hour))}}
	if _, err := JudgePropagation(&PropagationState{Since: start}, stale, nil, 0, "", start.Add(PropagationLongest+time.Second)); err == nil || !strings.Contains(err.Error(), "无报告：c/hub-3") {
		t.Fatalf("err %v", err)
	}
}
