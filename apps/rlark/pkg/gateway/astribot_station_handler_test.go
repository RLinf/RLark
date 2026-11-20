package gateway

import (
	"encoding/json"
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
		stationClusterGVR:    "ClusterList",
		networkEnrollmentGVR: "NetworkEnrollmentList",
		siteNodeGVR:          "NodeList",
		nodeOperationGVR:     "NodeOperationList",
	}, objects...)
}

func stationHandlerCluster(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
		"metadata": map[string]interface{}{"name": name},
		"spec":     map[string]interface{}{"agentNamespace": namespace},
	}}
}

func stationHandlerRecord(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace},
		"spec":     map[string]interface{}{"clusterID": "cluster-a", "nodeUID": "node-uid", "publicKey": "key"},
		"status":   map[string]interface{}{"phase": "Enrolled", "config": "{}"},
	}}
}

func TestListStationsUsesClusterNamespaceAndIgnoresRequestedNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(
		stationHandlerCluster("cluster-a", "agent-a"),
		stationHandlerCluster("cluster-b", "agent-b"),
		stationHandlerRecord("node-a", "agent-a"),
		stationHandlerRecord("node-b", "agent-b"),
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
			Status map[string]string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Metadata.Name != "node-a" || response.Items[0].Metadata.Namespace != "agent-a" {
		t.Fatalf("Station list=%#v", response.Items)
	}
	if status := response.Items[0].Status; status["phase"] != "Enrolled" || status["config"] != "" {
		t.Fatalf("a listed record carries its answer without the network document: %v", status)
	}
}

func TestGetStationRequiresClusterAndRejectsAmbiguousNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerCluster("cluster-b", "agent-a"), stationHandlerRecord("node-a", "agent-a"))
	gateway := &Gateway{astribotState: astribotState{stationClient: client}}
	request := httptest.NewRequest("GET", "/api/v1/station/stations/node-a", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "name", Value: "node-a"}}
	gateway.handleGetStation(ctx)
	if recorder.Code != 400 {
		t.Fatalf("missing cluster ID status=%d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/api/v1/station/stations/node-a?cluster_id=cluster-a", nil)
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "name", Value: "node-a"}}
	gateway.handleGetStation(ctx)
	if recorder.Code != 409 {
		t.Fatalf("ambiguous namespace status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
