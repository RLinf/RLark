package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func siteNodeRequest(t *testing.T, gateway *Gateway, node, query string) (int, siteNode) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/site-nodes/"+node+query, nil)
	ctx.Params = gin.Params{{Key: "name", Value: node}}
	gateway.handleGetSiteNode(ctx)
	var result siteNode
	_ = json.Unmarshal(recorder.Body.Bytes(), &result)
	return recorder.Code, result
}

func TestGetSiteNodeJoinsIdentityNetworkAndRobot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enrollment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a"},
		"spec":     map[string]interface{}{"nodeUID": "uid-1", "publicKey": "key"},
		"status": map[string]interface{}{"phase": "Enrolled", "site": "lab", "role": "Leaf", "address": "10.254.240.3", "tunnelAddress": "10.254.240.3", "config": "{}",
			"linkSummary": "nothing arrives from this node at any hub it dials since 2026-11-12T07:50:00Z", "links": []interface{}{map[string]interface{}{
				"hub": "dev/cailiang", "state": "Silent", "since": "2026-11-12T07:50:00Z", "latestHandshake": "2026-11-12T07:46:00Z", "receivedBytes": int64(2444), "observedAt": "2026-11-12T07:50:00Z"}},
			"node": map[string]interface{}{"state": "Bound", "lastSeenTime": "2026-11-12T08:00:00Z", "bootTime": "2026-11-12T03:58:58Z", "reboots": int64(1), "conditions": []interface{}{
				map[string]interface{}{"type": "NetworkVerified", "status": "False"}, map[string]interface{}{"type": "Observed", "status": "True"}}}},
	}}
	node := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Node",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a", "labels": map[string]interface{}{"rlark.io/robot-id": "s1u-01"},
			"annotations": map[string]interface{}{"rlark.io/cameras": `{"state":"ok"}`}},
	}}
	admission := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a"},
		"spec":     map[string]interface{}{"machine": map[string]interface{}{"category": "Robot", "model": "s1u"}},
	}}
	machine := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "Machine",
		"metadata": map[string]interface{}{"name": "machine-1", "namespace": machineNamespace},
		"status":   map[string]interface{}{"clusterID": "cluster-a", "nodeName": "s1u-01-x86", "observedIdentity": map[string]interface{}{"category": "Robot", "model": "s1u-v2"}, "identityVerified": "False", "identityReason": "ModelMismatch"},
	}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		stationClusterGVR: "ClusterList", networkEnrollmentGVR: "NetworkEnrollmentList", siteNodeGVR: "NodeList", nodeAdmissionGVR: "NodeAdmissionList", machineGVR: "MachineList",
	}, stationHandlerCluster("cluster-a", "agent-a"), enrollment, node, admission, machine)
	gateway := &Gateway{astribotState: astribotState{stationClient: client}}

	code, got := siteNodeRequest(t, gateway, "s1u-01-x86", "?cluster_id=cluster-a")
	want := siteNode{ClusterID: "cluster-a", NodeName: "s1u-01-x86", NodeUID: "uid-1", Phase: "Bound", Online: true, DeclaredType: "Robot", DeclaredModel: "s1u", ObservedType: "Robot", ObservedModel: "s1u-v2", IdentityVerified: "False", IdentityReason: "ModelMismatch",
		LastSeenTime: "2026-11-12T08:00:00Z", RobotID: "s1u-01", Cameras: `{"state":"ok"}`, Tunnel: "failed", RLarkNode: "unknown", BootTime: "2026-11-12T03:58:58Z", Reboots: 1}
	if code != 200 || !reflect.DeepEqual(got.Network, &siteNodeNetwork{State: "Enrolled", Site: "lab", Role: "Leaf", Address: "10.254.240.3", TunnelAddress: "10.254.240.3", PublicKey: "key",
		LinkSummary: "nothing arrives from this node at any hub it dials since 2026-11-12T07:50:00Z",
		Links:       []siteNodeLink{{Hub: "dev/cailiang", State: "Silent", Since: "2026-11-12T07:50:00Z", LatestHandshake: "2026-11-12T07:46:00Z"}}}) {
		t.Fatalf("status %d, network %+v", code, got.Network)
	}
	if got.Labels["rlark.io/robot-id"] != "s1u-01" {
		t.Fatalf("labels %v", got.Labels)
	}
	got.Network, got.Labels = nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("site node %+v, want %+v", got, want)
	}

	// The whole cluster in one call, and by label.
	list := func(query string) []siteNode {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("GET", "/api/v1/site-nodes"+query, nil)
		gateway.handleListSiteNodes(ctx)
		var response struct {
			Items []siteNode `json:"items"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || recorder.Code != 200 {
			t.Fatalf("list %s: %d %v", query, recorder.Code, err)
		}
		return response.Items
	}
	if items := list("?cluster_id=cluster-a&label=rlark.io/robot-id=s1u-01"); len(items) != 1 || items[0].NodeName != "s1u-01-x86" || items[0].Network.Address != "10.254.240.3" {
		t.Fatalf("list by label: %+v", items)
	}
	if items := list("?cluster_id=cluster-a&label=rlark.io/robot-id=other"); len(items) != 0 {
		t.Fatalf("label filter: %+v", items)
	}

	// A node that never enrolled a key is in no tunnel network; a removed node is not served.
	unstructured.RemoveNestedField(enrollment.Object, "spec", "publicKey")
	if _, err := client.Resource(networkEnrollmentGVR).Namespace("agent-a").Update(t.Context(), enrollment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if code, got = siteNodeRequest(t, gateway, "s1u-01-x86", "?cluster_id=cluster-a"); code != 200 || got.Network != nil || got.RobotID != "s1u-01" {
		t.Fatalf("a node without a key: %d %+v", code, got)
	}
	_ = unstructured.SetNestedField(enrollment.Object, true, "spec", "retired")
	if _, err := client.Resource(networkEnrollmentGVR).Namespace("agent-a").Update(t.Context(), enrollment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if code, _ = siteNodeRequest(t, gateway, "s1u-01-x86", "?cluster_id=cluster-a"); code != 404 {
		t.Fatalf("a removed node returned %d", code)
	}
	if code, _ = siteNodeRequest(t, gateway, "unknown", "?cluster_id=cluster-a"); code != 404 {
		t.Fatalf("unknown node returned %d", code)
	}
	if code, _ = siteNodeRequest(t, gateway, "s1u-01-x86", ""); code != 400 {
		t.Fatalf("missing cluster returned %d", code)
	}
}
