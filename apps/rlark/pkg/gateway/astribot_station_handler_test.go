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

func stationHandlerNode(name, namespace string, labels, annotations map[string]string) *unstructured.Unstructured {
	labelValues := make(map[string]interface{}, len(labels))
	for key, value := range labels {
		labelValues[key] = value
	}
	annotationValues := make(map[string]interface{}, len(annotations))
	for key, value := range annotations {
		annotationValues[key] = value
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Node",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace, "labels": labelValues, "annotations": annotationValues},
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

func TestGetStationRequiresCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerRecord("node-a", "agent-a"))
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
}

func TestListStationsFiltersServerSideAndReturnsMatchedCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first, second := stationHandlerRecord("robot-a", "agent-a"), stationHandlerRecord("robot-b", "agent-a")
	_ = unstructured.SetNestedField(first.Object, "Offline", "status", "node", "state")
	_ = unstructured.SetNestedField(second.Object, "Bound", "status", "node", "state")
	firstNode := stationHandlerNode("robot-a", "agent-a", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/power-state": "ready", "rlark.io/occupied": "true", "rlark.io/zone": "bay-1"}, map[string]string{"rlark.io/cameras": `{"cameras":{"head":{"state":"stale","step":3}}}`})
	secondNode := stationHandlerNode("robot-b", "agent-a", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/power-state": "fault"}, nil)
	g := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), first, second, firstNode, secondNode)}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/station/stations?q=ROBOT&phase=offline,BOUND&model=S1U&alert_only=true&sort=alert", nil)
	g.handleListStations(ctx)
	if recorder.Code != 200 {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []unstructured.Unstructured `json:"items"`
		Total int                         `json:"totalMatched"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 2 || response.Items[0].GetName() != "robot-b" || response.Total != 2 {
		t.Fatalf("filtered response %#v", response)
	}
}

func TestStationSummaryAggregatesClustersPhasesAndAlerts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bound := stationHandlerRecord("robot-a", "agent-a")
	_ = unstructured.SetNestedField(bound.Object, "Bound", "status", "node", "state")
	_ = unstructured.SetNestedField(bound.Object, "cluster-a", "spec", "clusterID")
	_ = unstructured.SetNestedField(bound.Object, []interface{}{map[string]interface{}{"type": "SelfCheck", "status": "True"}}, "status", "node", "conditions")
	offline := stationHandlerRecord("node-b", "agent-a")
	_ = unstructured.SetNestedField(offline.Object, "cluster-a", "spec", "clusterID")
	_ = unstructured.SetNestedField(offline.Object, "Offline", "status", "node", "state")
	conflict := stationHandlerRecord("node-c", "agent-b")
	_ = unstructured.SetNestedField(conflict.Object, "cluster-b", "spec", "clusterID")
	_ = unstructured.SetNestedField(conflict.Object, "Conflict", "status", "node", "state")
	retired := stationHandlerRecord("node-d", "agent-b")
	_ = unstructured.SetNestedField(retired.Object, "cluster-b", "spec", "clusterID")
	_ = unstructured.SetNestedField(retired.Object, true, "spec", "retired")
	boundNode := stationHandlerNode("robot-a", "agent-a", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/node-category": "robot", "rlark.io/power-state": "ready", "rlark.io/occupied": "true"}, map[string]string{"rlark.io/cameras": `{"cameras":{"head":{"state":"stale","step":3}}}`})
	conflictNode := stationHandlerNode("node-c", "agent-b", map[string]string{"rlark.io/node-category": "server"}, nil)
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerCluster("cluster-b", "agent-b"), bound, offline, conflict, retired, boundNode, conflictNode)
	g := &Gateway{astribotState: astribotState{stationClient: client}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/station/summary", nil)
	g.handleStationSummary(ctx)
	if recorder.Code != 200 {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var got struct {
		Total       int            `json:"total"`
		Robots      int            `json:"robots"`
		OtherNodes  int            `json:"otherNodes"`
		ByPhase     map[string]int `json:"byPhase"`
		Alerts      int            `json:"alerts"`
		DriverReady int            `json:"driverReady"`
		CameraFault int            `json:"cameraFault"`
		Occupied    int            `json:"occupied"`
		Clusters    []struct {
			ClusterID string `json:"clusterId"`
			Total     int    `json:"total"`
			Bound     int    `json:"bound"`
			Offline   int    `json:"offline"`
			Conflict  int    `json:"conflict"`
			Alerts    int    `json:"alerts"`
		} `json:"clusters"`
		GeneratedAt string `json:"generatedAt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 4 || got.Robots != 1 || got.OtherNodes != 3 || got.Alerts != 3 || got.DriverReady != 1 || got.CameraFault != 1 || got.Occupied != 1 || got.ByPhase["Bound"] != 1 || got.ByPhase["Offline"] != 1 || got.ByPhase["Conflict"] != 1 || got.ByPhase["Retired"] != 1 || len(got.Clusters) != 2 || got.Clusters[0].ClusterID != "cluster-a" || got.Clusters[0].Total != 2 || got.Clusters[0].Bound != 1 || got.Clusters[0].Offline != 1 || got.Clusters[0].Alerts != 2 || got.Clusters[1].ClusterID != "cluster-b" || got.Clusters[1].Conflict != 1 || got.Clusters[1].Alerts != 1 || got.Clusters[1].Total != 2 {
		t.Fatalf("summary mismatch: %+v", got)
	}
	if got.GeneratedAt == "" {
		t.Fatal("generatedAt is empty")
	}
}

func TestStationFiltersUseNodeTagsForPowerOccupancyAndCameraFault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	station := stationHandlerRecord("robot-a", "agent-a")
	_ = unstructured.SetNestedField(station.Object, "Bound", "status", "node", "state")
	node := stationHandlerNode("robot-a", "agent-a", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/power-state": "ready", "rlark.io/occupied": "true", "rlark.io/zone": "bay-1"}, map[string]string{"rlark.io/cameras": `{"cameras":{"head":{"state":"stale","step":3}}}`})
	g := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(station, node)}}
	for _, query := range []string{"power_state=running", "power_state=powered", "occupancy=occupied", "model=S1U", "zone=bay-1", "alert_only=true"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("GET", "/api/v1/station/stations?"+query, nil)
		g.handleListStations(ctx)
		var response struct {
			Items []unstructured.Unstructured `json:"items"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); recorder.Code != 200 || err != nil || len(response.Items) != 1 {
			t.Errorf("%s status=%d items=%d body=%s", query, recorder.Code, len(response.Items), recorder.Body.String())
		}
	}
}

func TestStationFiltersRejectInvalidValuesBeforeListingEmptyResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"phase=bad", "power_state=bad", "occupancy=bad", "sort=bad"} {
		g := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient()}}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("GET", "/api/v1/station/stations?"+query, nil)
		g.handleListStations(ctx)
		if recorder.Code != 400 {
			t.Errorf("%s on empty list returned %d: %s", query, recorder.Code, recorder.Body.String())
		}
	}
}

func TestListStationsLegacyResponseHasNoMatchedCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(stationHandlerRecord("legacy-node", "agent-a"))}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/station/stations?limit=10", nil)
	g.handleListStations(ctx)
	var response map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || response["items"] == nil || response["continue"] == nil || response["totalMatched"] != nil {
		t.Fatalf("legacy response %d: %s", recorder.Code, recorder.Body.String())
	}
}
