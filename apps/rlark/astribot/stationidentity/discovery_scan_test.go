package stationidentity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestDiscoveryScanReportsRequireTargetProofAndFinalBatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	first, other := newDocumentNode(t), newDocumentNode(t)
	enrollment := func(name, site string, node documentNode) *unstructured.Unstructured {
		holder := documentHolder("NetworkEnrollment", "robots", name, "Enrolled", node.public(), `{}`)
		holder.SetUID(types.UID(name))
		_ = unstructured.SetNestedField(holder.Object, site, "status", "site")
		_ = unstructured.SetNestedField(holder.Object, "Hub", "status", "role")
		_ = unstructured.SetNestedField(holder.Object, "10.254.240.5", "status", "tunnelAddress")
		return holder
	}
	factory := "SHA256:" + strings.Repeat("f", 43)
	class := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": stationGroup + "/" + stationVersion, "kind": "NodeClass", "metadata": map[string]any{"name": "s1u"},
		"spec": map[string]any{"discovery": map[string]any{"hostKeyFingerprints": []any{factory}}},
	}}
	network := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": stationGroup + "/" + stationVersion, "kind": "SiteNetwork",
		"metadata": map[string]any{"name": "default", "namespace": "station-system"},
		"spec": map[string]any{"sites": []any{
			map[string]any{"name": "lab", "discovery": map[string]any{"enabled": true}},
		}},
	}}
	const id = "d3feca83-fccd-435d-b270-82f6072f27c7"
	if err := StoreDiscoveryScan(network, "lab", DiscoveryScan{ID: id, StartedAt: now, Hubs: []types.UID{"first"}}); err != nil {
		t.Fatal(err)
	}
	api, client := testNetworkDocuments(t, &now, enrollment("first", "lab", first), enrollment("other", "office", other), network, class)
	request := func(node documentNode, path, fields string) *httptest.ResponseRecorder {
		t.Helper()
		start := apiRequest(api, http.MethodPost, path, `{"public_key":"`+node.public()+`"}`, "")
		var challenge documentAnswer
		if start.Code != http.StatusUnauthorized || json.Unmarshal(start.Body.Bytes(), &challenge) != nil {
			t.Fatalf("proof challenge: %d %s", start.Code, start.Body.String())
		}
		return apiRequest(api, http.MethodPost, path, `{"public_key":"`+node.public()+`","challenge":"`+challenge.Challenge+`","proof":"`+node.prove(challenge)+`"`+fields+`}`, "")
	}
	check := "/v1/network/discovery-scan"
	report := "/v1/network/discovered"
	if response := request(first, check, ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), id) {
		t.Fatalf("target poll: %d %s", response.Code, response.Body.String())
	}
	if response := request(other, check, ""); response.Code != http.StatusNoContent {
		t.Fatalf("another site's poll got %d", response.Code)
	}
	final := `,"scan_id":"` + id + `","final":true,"hosts":[]`
	if response := request(other, report, final); response.Code != http.StatusConflict {
		t.Fatalf("another site cannot complete the scan, got %d", response.Code)
	}
	if response := request(first, report, `,"hosts":[]`); response.Code != http.StatusForbidden {
		t.Fatalf("legacy report cannot replace manual results, got %d", response.Code)
	}
	hosts := `,"hosts":[{"address":"192.168.31.56","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"` + factory + `"}]`
	if response := request(first, report, `,"scan_id":"`+id+`"`+hosts); response.Code != http.StatusOK {
		t.Fatalf("first batch: %d %s", response.Code, response.Body.String())
	}
	observed, err := client.Resource(machineGVR).Namespace("station-system").Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if scanID, _, _ := unstructured.NestedString(observed.Object, "status", "entry", "discoveryScanID"); scanID != id {
		t.Fatalf("stored scan identity: %q", scanID)
	}
	if response := request(first, check, ""); response.Code != http.StatusOK {
		t.Fatalf("an unfinished scan must survive a repeat poll, got %d", response.Code)
	}
	if response := request(first, report, final); response.Code != http.StatusOK {
		t.Fatalf("final batch: %d %s", response.Code, response.Body.String())
	}
	if response := request(first, check, ""); response.Code != http.StatusNoContent {
		t.Fatalf("completed scan should not run again, got %d", response.Code)
	}
}
