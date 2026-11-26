package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestTheStationIsTheAdmissionsOwnLabelAndUniqueInTheCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nodes := map[string]*rlarkv1alpha1.Node{}
	for name, station := range map[string]string{"s1u-01-x86": "", "s1u-02-x86": "S02"} {
		labels := map[string]string{"rlark.io/robot-id": name[:6]}
		if station != "" {
			labels[stationLabel] = station
		}
		nodes[name] = &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "agent-a", Labels: labels}}
	}
	admission := func(name, station string) *unstructured.Unstructured {
		labels := map[string]any{"rlark.io/robot-id": name[:6]}
		if station != "" {
			labels[stationLabel] = station
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]any{"name": name, "namespace": "agent-a", "annotations": map[string]any{admissionOwnLabels: "rlark.io/robot-id"}},
			"spec":     map[string]any{"labels": labels},
		}}
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		nodeAdmissionGVR: "NodeAdmissionList",
	}, admission("s1u-01-x86", ""), admission("s1u-02-x86", "S02"))
	g := &Gateway{astribotState: astribotState{stationClient: client}, accessors: map[string]*resourceAccessor{"nodes": {
		get: func(_ context.Context, _, name string, _ metav1.GetOptions) (any, error) { return nodes[name], nil },
		list: func(_ context.Context, _ string, options metav1.ListOptions) (any, error) {
			list := &rlarkv1alpha1.NodeList{}
			for _, node := range nodes {
				if stationLabel+"="+node.Labels[stationLabel] == options.LabelSelector {
					list.Items = append(list.Items, *node)
				}
			}
			return list, nil
		},
		patch: func(_ context.Context, _, name string, data []byte) (any, error) {
			var patch struct {
				Metadata struct{ Labels map[string]string }
			}
			_ = json.Unmarshal(data, &patch)
			for key, value := range patch.Metadata.Labels {
				nodes[name].Labels[key] = value
			}
			return nodes[name], nil
		},
	}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(authRoleKey, roleOperator); c.Set(authSubjectKey, "alice"); c.Next() })
	g.registerRobotRoutes(router.Group("/nodes"))
	put := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/nodes/s1u-01-x86/station?namespace=agent-a", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		return response
	}
	if response := put(`{"station":"station 1"}`); response.Code != 400 {
		t.Fatalf("a name that is no Station was taken: %d %s", response.Code, response.Body)
	}
	if response := put(`{"station":"S02"}`); response.Code != 409 {
		t.Fatalf("a second robot was put at S02: %d %s", response.Code, response.Body)
	}
	if response := put(`{"station":"S01"}`); response.Code != 200 || nodes["s1u-01-x86"].Labels[stationLabel] != "S01" {
		t.Fatalf("set S01: %d %s", response.Code, response.Body)
	}
	stored, err := client.Resource(nodeAdmissionGVR).Namespace("agent-a").Get(context.Background(), "s1u-01-x86", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	labels, _, _ := unstructured.NestedStringMap(stored.Object, "spec", "labels")
	if labels[stationLabel] != "S01" || stored.GetAnnotations()[admissionOwnLabels] != "rlark.io/robot-id,rlark.io/station" {
		t.Fatalf("the admission keeps the Station as the machine's own label: %v %v", labels, stored.GetAnnotations())
	}
}
