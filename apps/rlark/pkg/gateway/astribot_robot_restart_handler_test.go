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
)

func TestRobotRestartIsTheClaimHoldersAndLeavesTheDesiredPower(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "s1u-01-x86", Namespace: "robots", ResourceVersion: "7",
		Labels:      map[string]string{"rlark.io/robot-id": "s1u_01", robotClaimLabel: "run_a", robotPowerLabel: "ready.1"},
		Annotations: map[string]string{robotClaimSetAnnotation: `{"holder":"run_a"}`},
	}}
	var patches []string
	g := &Gateway{accessors: map[string]*resourceAccessor{"nodes": {
		get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node, nil },
		patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
			patches = append(patches, string(data))
			var patch struct {
				Metadata struct {
					ResourceVersion string
					Labels          map[string]string
					Annotations     map[string]string
				}
			}
			if err := json.Unmarshal(data, &patch); err != nil || patch.Metadata.ResourceVersion != "7" || len(patch.Metadata.Labels) != 0 {
				t.Fatalf("expected a versioned patch of the annotation alone: %s, %v", data, err)
			}
			for key, value := range patch.Metadata.Annotations {
				node.Annotations[key] = value
			}
			return node, nil
		},
	}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(authSubjectKey, "service-account:fleet-runner"); c.Next() })
	g.registerRobotRoutes(router.Group("/nodes"))
	post := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/nodes/s1u-01-x86/robot-restart?namespace=robots", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		return response
	}
	if response := post(`{"level":"reboot","holder":"run_a","request_id":"20"}`); response.Code != 400 {
		t.Fatalf("an unknown level was accepted: %d %s", response.Code, response.Body)
	}
	if response := post(`{"level":"driver","holder":"run_b","request_id":"20"}`); response.Code != 409 {
		t.Fatalf("another Run restarted the robot: %d %s", response.Code, response.Body)
	}
	for range 2 {
		if response := post(`{"level":"power","holder":"run_a","request_id":"20"}`); response.Code != 200 {
			t.Fatalf("the holder's restart: %d %s", response.Code, response.Body)
		}
	}
	var restart robotRestart
	if err := json.Unmarshal([]byte(node.Annotations[robotRestartAnnotation]), &restart); err != nil || len(patches) != 1 ||
		restart != (robotRestart{ID: "20", Level: "power", Holder: "run_a", By: "service-account:fleet-runner", At: restart.At}) || restart.At.IsZero() {
		t.Fatalf("one request recorded with its holder and actor: %+v, %d patches, %v", restart, len(patches), err)
	}
	if node.Labels[robotPowerLabel] != "ready.1" {
		t.Fatalf("the desired power changed: %s", node.Labels[robotPowerLabel])
	}
	node.Labels[stationMarkLabel] = "repair"
	if response := post(`{"level":"driver","holder":"run_a","request_id":"21"}`); response.Code != 409 {
		t.Fatalf("a marked station restarted: %d %s", response.Code, response.Body)
	}
}
