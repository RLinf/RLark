package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func stationHandlerClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		stationClusterGVR:     "ClusterList",
		stationRecordGVR:      "StationList",
		stationObservationGVR: "StationObservationList",
		stationRebindGVR:      "StationRebindList",
		nodeOperationGVR:      "NodeOperationList",
	}, objects...)
}

func stationHandlerCluster(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
		"metadata": map[string]interface{}{"name": name},
		"spec":     map[string]interface{}{"agentNamespace": namespace},
	}}
}

func stationHandlerStation(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "Station",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace, "uid": "station-uid-a"},
		"spec": map[string]interface{}{
			"displayName": name,
			"sequence":    int64(1),
			"binding":     map[string]interface{}{"nodeName": "node", "nodeUID": "node-uid", "version": int64(1), "boundBy": "admin"},
		},
	}}
}

func stationHandlerObservation(name, namespace, nodeUID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace},
		"status":   map[string]interface{}{"nodeUID": nodeUID, "reportedAt": "2026-11-11T00:00:00Z"},
	}}
}

func TestListStationsUsesClusterNamespaceAndIgnoresRequestedNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(
		stationHandlerCluster("cluster-a", "agent-a"),
		stationHandlerCluster("cluster-b", "agent-b"),
		stationHandlerStation("station-a", "agent-a"),
		stationHandlerStation("station-b", "agent-b"),
	)}}
	request := httptest.NewRequest("GET", "/api/v1/station/stations?cluster_id=cluster-a&namespace=agent-b", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	gateway.handleListStations(ctx)
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Metadata.Name != "station-a" || response.Items[0].Metadata.Namespace != "agent-a" {
		t.Fatalf("Station list=%#v", response.Items)
	}
}

func TestGetStationRequiresClusterAndRejectsAmbiguousNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerCluster("cluster-b", "agent-a"), stationHandlerStation("station-a", "agent-a"))
	gateway := &Gateway{astribotState: astribotState{stationClient: client}}
	request := httptest.NewRequest("GET", "/api/v1/station/stations/station-a", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "name", Value: "station-a"}}
	gateway.handleGetStation(ctx)
	if recorder.Code != 400 {
		t.Fatalf("missing cluster ID status=%d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/api/v1/station/stations/station-a?cluster_id=cluster-a", nil)
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "name", Value: "station-a"}}
	gateway.handleGetStation(ctx)
	if recorder.Code != 409 {
		t.Fatalf("ambiguous namespace status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestListStationObservationsRequiresRegisteredClusterAndUsesMappedNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(
		stationHandlerCluster("cluster-a", "agent-a"),
		stationHandlerCluster("cluster-b", "agent-b"),
		stationHandlerObservation("node-a", "agent-a", "node-uid-a"),
		stationHandlerObservation("node-b", "agent-b", "node-uid-b"),
	)}}
	request := httptest.NewRequest("GET", "/api/v1/station/observations?cluster_id=cluster-a&namespace=agent-b", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	gateway.handleListStationObservations(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Metadata.Name != "node-a" || response.Items[0].Metadata.Namespace != "agent-a" {
		t.Fatalf("Station observations = %#v", response.Items)
	}
	request = httptest.NewRequest("GET", "/api/v1/station/observations", nil)
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = request
	gateway.handleListStationObservations(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing cluster ID status=%d", recorder.Code)
	}
}
