package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A record is deleted when its node was removed, when another Node reports under its name, or when its node does
// not report and RLark has no Node of the name; the record of a node in the cluster, or of one that reports, stays,
// and one that is gone answers as deleted.
func TestDeleteStationDeletesARecordNoNodeStandsOn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	retired := stationHandlerRecord("retired", "agent-a")
	_ = unstructured.SetNestedField(retired.Object, true, "spec", "retired")
	conflict := stationHandlerRecord("conflict", "agent-a")
	_ = unstructured.SetNestedField(conflict.Object, "Conflict", "status", "node", "state")
	node := func(name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "rlinf.io/v1alpha1", "kind": "Node", "metadata": map[string]interface{}{"name": name, "namespace": "agent-a"}}}
	}
	reporting := stationHandlerRecord("reporting", "agent-a")
	_ = unstructured.SetNestedField(reporting.Object, "Bound", "status", "node", "state")
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), retired, conflict, reporting, stationHandlerRecord("bound", "agent-a"), stationHandlerRecord("left", "agent-a"),
		node("retired"), node("conflict"), node("bound"))
	g := &Gateway{astribotState: astribotState{stationClient: client}}
	for name, want := range map[string]int{"bound": http.StatusConflict, "reporting": http.StatusConflict, "gone": http.StatusOK, "retired": http.StatusOK, "conflict": http.StatusOK, "left": http.StatusOK} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/station/stations/"+name+"?cluster_id=cluster-a", nil)
		ctx.Params = gin.Params{{Key: "name", Value: name}}
		g.handleDeleteStation(ctx)
		if recorder.Code != want {
			t.Fatalf("%s = %d, want %d", name, recorder.Code, want)
		}
		_, err := client.Resource(networkEnrollmentGVR).Namespace("agent-a").Get(t.Context(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) != (want == http.StatusOK) {
			t.Fatalf("%s after the request: %v", name, err)
		}
	}
}
