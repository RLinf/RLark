package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/rlinf/rlark/apps/rlark/astribot/stationidentity"
)

func TestDiscoveryScanRequestsAreSiteScopedAndCoalesced(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, network := siteNetworkGateway()
	client := g.stationClient.(*dynamicfake.FakeDynamicClient)
	hub := func(name, site string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
			"metadata": map[string]any{"namespace": "station-nodes", "name": name, "uid": name},
			"status":   map[string]any{"phase": "Enrolled", "role": "Hub", "site": site, "tunnelAddress": "10.254.240.2", "config": `{"discovery":{}}`},
		}}
	}
	for _, object := range []*unstructured.Unstructured{hub("office-hub", "office"), hub("lab-hub", "lab")} {
		if err := client.Tracker().Add(object); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	path := "/api/v1/station/site-network/sites/office/scan"
	const requestID = "d3feca83-fccd-435d-b270-82f6072f27c7"
	const nextID = "ef512a23-8c56-47b7-a55e-3f72da5ff401"
	call := func(method, role, body string) (int, []byte) {
		t.Helper()
		return siteNetworkCall(t, g, g.handleDiscoveryScan, method, path, body, role)
	}
	if status, _ := call(http.MethodGet, "user", ""); status != http.StatusForbidden {
		t.Fatalf("non-admin scan read got %d", status)
	}
	if status, _ := call(http.MethodPost, "user", `{"id":"`+requestID+`"}`); status != http.StatusForbidden {
		t.Fatalf("non-admin scan request got %d", status)
	}
	if status, body := call(http.MethodGet, "admin", ""); status != 200 || string(body) != "null" {
		t.Fatalf("idle: %d %s", status, body)
	}
	if status, _ := call(http.MethodPost, "admin", `{"id":"invalid"}`); status != 400 {
		t.Fatalf("invalid ID got %d", status)
	}
	check := func(method, request, id, phase string, code int) discoveryScanView {
		t.Helper()
		status, body := call(method, "admin", request)
		var view discoveryScanView
		if status != code || json.Unmarshal(body, &view) != nil || view.ID != id || view.Phase != phase || view.Total != 1 {
			t.Fatalf("scan %s: %d %s", id, status, body)
		}
		return view
	}
	check(http.MethodPost, `{"id":"`+requestID+`"}`, requestID, "Running", 202)
	check(http.MethodPost, `{"id":"`+nextID+`"}`, requestID, "Running", 202)
	resource := client.Resource(siteNetworkGVR).Namespace(network.GetNamespace())
	stored, err := resource.Get(ctx, network.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := stationidentity.DiscoveryScanFor(stored, "office")
	if err != nil || len(scan.Hubs) != 1 || scan.Hubs[0] != "office-hub" {
		t.Fatalf("target snapshot: %+v, %v", scan, err)
	}
	scan.StartedAt = time.Now().Add(-stationidentity.DiscoveryScanLifetime)
	if err := stationidentity.StoreDiscoveryScan(stored, "office", scan); err != nil {
		t.Fatal(err)
	}
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	check(http.MethodGet, "", requestID, "TimedOut", 200)
	check(http.MethodPost, `{"id":"`+requestID+`"}`, requestID, "TimedOut", 200)
	check(http.MethodPost, `{"id":"`+nextID+`"}`, nextID, "Running", 202)
}
