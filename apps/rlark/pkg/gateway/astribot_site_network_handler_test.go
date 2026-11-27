package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func siteNetworkGateway() (*Gateway, *unstructured.Unstructured) {
	network := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork",
		"metadata": map[string]any{"name": "default", "namespace": "station-system", "resourceVersion": "7", "generation": int64(4)},
		"spec": map[string]any{
			"pool": "10.254.240.0/24", "names": []any{map[string]any{"name": "office", "address": "10.254.240.1"}},
			"nodes": []any{
				map[string]any{"clusterID": "cluster-a", "nodeName": "hub-a", "role": "Hub", "address": "10.254.240.10"},
				map[string]any{"clusterID": "cluster-b", "nodeName": "hub-b", "role": "Hub", "address": "10.254.240.11", "site": "farm"},
				map[string]any{"clusterID": "cluster-a", "nodeName": "worker-a", "role": "Node", "address": "10.254.240.12"},
			},
			"sites": []any{
				map[string]any{"name": "office", "clusters": []any{"cluster-a"}, "discovery": map[string]any{"enabled": true, "ranges": []any{"192.168.1.9/24"}, "intervalSeconds": int64(30), "from": map[string]any{"clusterID": "cluster-a", "nodeName": "hub-a"}}},
				map[string]any{"name": "farm", "clusters": []any{"cluster-b"}, "leafUplink": map[string]any{"gateway": "192.168.0.11"}},
			},
		},
		"status": map[string]any{"observedGeneration": int64(4), "conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
			"declared": []any{map[string]any{"clusterID": "cluster-a", "nodeName": "hub-a", "config": map[string]any{"credential": "must-not-leak"}}}},
	}}
	class := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]any{"name": "s1u"},
		"spec": map[string]any{"access": map[string]any{"entry": map[string]any{"port": int64(22), "password": "private"}},
			"discovery": map[string]any{"hostKeyFingerprints": []any{"SHA256:public"}}},
	}}
	withoutDiscovery := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]any{"name": "hidden"}, "spec": map[string]any{"access": map[string]any{}}}}
	clusterA := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster", "metadata": map[string]any{"name": "cluster-a"}}}
	clusterB := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster", "metadata": map[string]any{"name": "cluster-b"}}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		networkEnrollmentGVR: "NetworkEnrollmentList", siteNetworkGVR: "SiteNetworkList", nodeClassGVR: "NodeClassList", stationClusterGVR: "ClusterList",
	}, network, class, withoutDiscovery, clusterA, clusterB)
	return &Gateway{astribotState: astribotState{stationClient: client}}, network
}

func siteNetworkCall(t *testing.T, g *Gateway, handler gin.HandlerFunc, method, path, body string, role string) (int, []byte) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	site := "office"
	if strings.Contains(path, "/sites/") {
		site = strings.Split(strings.TrimPrefix(strings.SplitN(path, "/sites/", 2)[1], "/"), "/")[0]
	}
	ctx.Params = gin.Params{{Key: "site", Value: site}}
	ctx.Set(authSubjectKey, "alice")
	if role != "" {
		ctx.Set(authRoleKey, role)
	}
	if role != "admin" {
		requireAdmin()(ctx)
		if ctx.IsAborted() {
			return recorder.Code, recorder.Body.Bytes()
		}
	}
	handler(ctx)
	return recorder.Code, recorder.Body.Bytes()
}

func TestGetSiteNetworkReturnsSafeView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := siteNetworkGateway()
	status, body := siteNetworkCall(t, g, g.handleGetSiteNetwork, "GET", "/api/v1/station/site-network", "", "admin")
	var view siteNetworkView
	if err := json.Unmarshal(body, &view); err != nil || status != 200 {
		t.Fatalf("GET status=%d err=%v body=%s", status, err, body)
	}
	if view.ResourceVersion != "7" || view.Generation != 4 || len(view.Sites) != 2 || len(view.Members) != 3 || len(view.NodeClasses) != 1 {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.Sites[0].Discovery == nil || len(view.Sites[0].Discovery.Ranges) != 1 || view.Sites[0].Discovery.Ranges[0] != "192.168.1.9/24" || view.Members[1]["site"] != "farm" || fmt.Sprint(view.NodeClasses[0]["entryPort"]) != "22" {
		t.Fatalf("unexpected projected fields: discovery=%+v member=%#v nodeClass=%#v", view.Sites[0].Discovery, view.Members[1], view.NodeClasses[0])
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "must-not-leak") || strings.Contains(string(encoded), "private") {
		t.Fatalf("sensitive fields leaked: %s", encoded)
	}
	declared := view.Status["declared"].([]any)
	if _, ok := declared[0].(map[string]any)["config"]; ok {
		t.Fatal("status.declared.config was not removed")
	}
}

func TestGetSiteNetworkNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := siteNetworkGateway()
	if err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Delete(context.Background(), "default", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, body := siteNetworkCall(t, g, g.handleGetSiteNetwork, "GET", "/api/v1/station/site-network", "", "admin"); status != 404 {
		t.Fatalf("status=%d body=%s", status, body)
	}
}

func TestUpdateSiteDiscoveryPreservesOtherNetworkFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, original := siteNetworkGateway()
	updateSiteNetworkVersion(g)
	beforeSpec, _, _ := unstructured.NestedMap(original.Object, "spec")
	status, body := siteNetworkCall(t, g, g.handleUpdateSiteDiscovery, "PUT", "/api/v1/station/site-network/sites/office/discovery",
		`{"resourceVersion":"7","discovery":{"enabled":false,"ranges":["192.168.1.99/24"],"from":{"clusterID":"cluster-b","nodeName":"hub-b"},"includeUnclassified":true,"sshPorts":[2222,8022]}}`, "admin")
	if status != 200 {
		t.Fatalf("PUT status=%d body=%s", status, body)
	}
	var view siteNetworkView
	if err := json.Unmarshal(body, &view); err != nil || view.ResourceVersion != "8" {
		t.Fatalf("updated view rv=%q err=%v body=%s", view.ResourceVersion, err, body)
	}
	stored, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Get(context.Background(), "default", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	afterSpec, _, _ := unstructured.NestedMap(stored.Object, "spec")
	for _, key := range []string{"pool", "names", "nodes"} {
		if !reflect.DeepEqual(beforeSpec[key], afterSpec[key]) {
			t.Errorf("spec.%s changed: before=%#v after=%#v", key, beforeSpec[key], afterSpec[key])
		}
	}
	beforeSites := beforeSpec["sites"].([]any)
	afterSites := afterSpec["sites"].([]any)
	if !reflect.DeepEqual(beforeSites[1], afterSites[1]) {
		t.Errorf("other site changed: before=%#v after=%#v", beforeSites[1], afterSites[1])
	}
	discovery, _, _ := unstructured.NestedMap(afterSites[0].(map[string]any), "discovery")
	if discovery["enabled"] != false || discovery["intervalSeconds"] != int64(30) || discovery["ranges"].([]any)[0] != "192.168.1.0/24" || discovery["from"].(map[string]any)["clusterID"] != "cluster-b" || discovery["includeUnclassified"] != true || !reflect.DeepEqual(discovery["sshPorts"], []interface{}{int64(2222), int64(8022)}) {
		t.Fatalf("stored discovery = %#v", discovery)
	}
}

func updateSiteNetworkVersion(g *Gateway) {
	client := g.stationClient.(*dynamicfake.FakeDynamicClient)
	client.PrependReactor("update", "sitenetworks", func(action k8stesting.Action) (bool, runtime.Object, error) {
		object := action.(k8stesting.UpdateAction).GetObject().(*unstructured.Unstructured)
		object.SetResourceVersion("8")
		if err := client.Tracker().Update(siteNetworkGVR, object, "station-system"); err != nil {
			return true, nil, err
		}
		return true, object, nil
	})
}

func TestUpdateSiteDiscoveryValidationAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `"discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}`
	cases := []struct {
		name, body string
		status     int
	}{
		{"missing resourceVersion", `{"discovery":{"enabled":false}}`, 400},
		{"missing enabled", `{"resourceVersion":"7","discovery":{"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"empty ranges", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":[],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"too many ranges", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22","11.0.0.0/22","12.0.0.0/22","13.0.0.0/22","14.0.0.0/22","15.0.0.0/22","16.0.0.0/22","17.0.0.0/22","18.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"ipv6", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["2001:db8::/32"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"short prefix", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/21"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"duplicate canonical prefix", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.1/22","10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"invalid prefix", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["bad"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"interval too low", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"intervalSeconds":9,"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"interval zero", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"intervalSeconds":0,"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"interval too high", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"intervalSeconds":3601,"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`, 400},
		{"duplicate ssh port", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"},"sshPorts":[2222,2222]}}`, 400},
		{"invalid ssh port", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"},"sshPorts":[65536]}}`, 400},
		{"from non hub", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"worker-a"}}}`, 400},
		{"from non member", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"missing"}}}`, 400},
		{"from unregistered cluster", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-c","nodeName":"hub-c"}}}`, 400},
		{"site not found", `{"resourceVersion":"7",` + valid + `}`, 404},
		{"resource version conflict", `{"resourceVersion":"old",` + valid + `}`, 409},
		{"unknown nested field", `{"resourceVersion":"7","discovery":{"enabled":true,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"},"secret":"no"}}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := siteNetworkGateway()
			if tc.name == "from unregistered cluster" {
				network, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Get(context.Background(), "default", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
				nodes = append(nodes, map[string]any{"clusterID": "cluster-c", "nodeName": "hub-c", "role": "Hub"})
				_ = unstructured.SetNestedSlice(network.Object, nodes, "spec", "nodes")
				if _, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(context.Background(), network, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			path := "/api/v1/station/site-network/sites/office/discovery"
			if tc.name == "site not found" {
				path = "/api/v1/station/site-network/sites/unknown/discovery"
			}
			status, body := siteNetworkCall(t, g, g.handleUpdateSiteDiscovery, "PUT", path, tc.body, "admin")
			if status != tc.status {
				t.Fatalf("status=%d want=%d body=%s", status, tc.status, body)
			}
		})
	}
	for _, body := range []string{
		`{"resourceVersion":"7","discovery":null}`,
		`{"resourceVersion":"7","discovery":{"enabled":false,"ranges":["10.0.0.0/22"],"from":{"clusterID":"cluster-a","nodeName":"hub-a"}}}`,
	} {
		g, _ := siteNetworkGateway()
		if status, response := siteNetworkCall(t, g, g.handleUpdateSiteDiscovery, "PUT", "/api/v1/station/site-network/sites/office/discovery", body, "admin"); status != 200 {
			t.Fatalf("close discovery status=%d body=%s", status, response)
		}
	}
}

func TestSiteNetworkRoutesRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := siteNetworkGateway()
	for _, tc := range []struct {
		method, path string
		handler      gin.HandlerFunc
		body         string
	}{
		{"GET", "/api/v1/station/site-network", g.handleGetSiteNetwork, ""},
		{"PUT", "/api/v1/station/site-network/sites/office/discovery", g.handleUpdateSiteDiscovery, `{"resourceVersion":"7","discovery":null}`},
	} {
		if status, _ := siteNetworkCall(t, g, tc.handler, tc.method, tc.path, tc.body, "user"); status != 403 {
			t.Errorf("%s %s status=%d", tc.method, tc.path, status)
		}
	}
}

func TestUpdateSiteDiscoveryReturnsConflictFromAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := siteNetworkGateway()
	g.stationClient.(*dynamicfake.FakeDynamicClient).PrependReactor("update", "sitenetworks", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(siteNetworkGVR.GroupResource(), "default", fmt.Errorf("stale"))
	})
	body := `{"resourceVersion":"7","discovery":null}`
	if status, response := siteNetworkCall(t, g, g.handleUpdateSiteDiscovery, "PUT", "/api/v1/station/site-network/sites/office/discovery", body, "admin"); status != 409 {
		t.Fatalf("status=%d body=%s", status, response)
	}
}
