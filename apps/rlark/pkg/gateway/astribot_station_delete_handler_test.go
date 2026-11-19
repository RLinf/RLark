package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Only a retired Station is asked to be deleted; who asked is written on it for station-controller, an earlier
// refusal is taken off, and a Station that is gone answers as deleted.
func TestDeleteStationAsksStationControllerForARetiredOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	retired := stationHandlerStation("stn-retired", "agent-a")
	_ = unstructured.SetNestedField(retired.Object, true, "spec", "retired")
	retired.SetAnnotations(map[string]string{stationDeleteRefusedAnnotation: "its node is in the cluster"})
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), retired, stationHandlerStation("stn-bound", "agent-a"))
	g := &Gateway{astribotState: astribotState{stationClient: client}}
	call := func(name string) int {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/station/stations/"+name+"?cluster_id=cluster-a", nil)
		ctx.Params = gin.Params{{Key: "name", Value: name}}
		ctx.Set(authSubjectKey, "alice")
		g.handleDeleteStation(ctx)
		return recorder.Code
	}
	if code := call("stn-bound"); code != http.StatusConflict {
		t.Fatalf("a Station that is not retired = %d, want 409", code)
	}
	if code := call("stn-gone"); code != http.StatusOK {
		t.Fatalf("a Station that is gone = %d, want 200", code)
	}
	for range 2 {
		if code := call("stn-retired"); code != http.StatusAccepted {
			t.Fatalf("a retired Station = %d, want 202", code)
		}
	}
	stored, err := client.Resource(stationRecordGVR).Namespace("agent-a").Get(t.Context(), "stn-retired", metav1.GetOptions{})
	if err != nil || stored.GetAnnotations()[stationDeleteRequestedAnnotation] != "alice" || stored.GetAnnotations()[stationDeleteRefusedAnnotation] != "" {
		t.Fatalf("annotations %v %v", stored.GetAnnotations(), err)
	}
}
