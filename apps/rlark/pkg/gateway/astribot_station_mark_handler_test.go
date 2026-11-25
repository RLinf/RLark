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

func TestStationMarkReleasesTheClaimAndBlocksNewClaimsUntilCleared(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "s1u-01-x86", Namespace: "robots", ResourceVersion: "7",
		Labels:      map[string]string{"rlark.io/robot-id": "s1u_01", robotClaimLabel: "run_old"},
		Annotations: map[string]string{robotClaimSetAnnotation: `{"holder":"run_old"}`},
	}}
	var patches int
	g := &Gateway{accessors: map[string]*resourceAccessor{"nodes": {
		get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node, nil },
		patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
			patches++
			var patch struct {
				Metadata struct {
					ResourceVersion     string
					Labels, Annotations map[string]*string
				}
			}
			if err := json.Unmarshal(data, &patch); err != nil || patch.Metadata.ResourceVersion != "7" {
				t.Fatalf("expected versioned patch: %s, %v", data, err)
			}
			for _, pair := range []struct {
				target map[string]string
				values map[string]*string
			}{{node.Labels, patch.Metadata.Labels}, {node.Annotations, patch.Metadata.Annotations}} {
				for key, value := range pair.values {
					if value == nil {
						delete(pair.target, key)
					} else {
						pair.target[key] = *value
					}
				}
			}
			return node, nil
		},
	}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authRoleKey, c.GetHeader("X-Test-Role"))
		c.Set(authSubjectKey, "operator")
		c.Next()
	})
	g.registerRobotRoutes(router.Group("/nodes"))
	request := func(path, role, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/nodes/s1u-01-x86/"+path+"?namespace=robots", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		router.ServeHTTP(response, req)
		return response
	}
	if response := request("station-mark", "user", `{"mark":"repair","reason":"camera"}`); response.Code != 403 {
		t.Fatalf("a user may not mark: %d %s", response.Code, response.Body)
	}
	for range 2 {
		if response := request("station-mark", roleAdmin, `{"mark":"repair","reason":"camera"}`); response.Code != 200 {
			t.Fatalf("set mark: %d %s", response.Code, response.Body)
		}
	}
	if patches != 1 || node.Labels[stationMarkLabel] != "repair" || node.Labels[robotClaimLabel] != "" || node.Annotations[robotClaimSetAnnotation] != "" {
		t.Fatalf("mark and release should be one versioned write: %+v", node.ObjectMeta)
	}
	var mark stationMark
	if err := json.Unmarshal([]byte(node.Annotations[stationMarkAnnotation]), &mark); err != nil || mark.Reason != "camera" || mark.By != "operator" || mark.At.IsZero() {
		t.Fatalf("mark must include the reason, actor and time: %+v, %v", mark, err)
	}
	claim := `{"holder":"run_new","leaseSeconds":30}`
	if response := request("robot-claim", "user", claim); response.Code != 409 {
		t.Fatalf("marked station accepted a claim: %d %s", response.Code, response.Body)
	}
	if response := request("station-mark", roleAdmin, `{"mark":""}`); response.Code != 200 || node.Annotations[stationMarkAnnotation] != "" {
		t.Fatalf("clear mark: %d %s", response.Code, response.Body)
	}
	if response := request("robot-claim", "user", claim); response.Code != 200 {
		t.Fatalf("clear station could not be claimed: %d %s", response.Code, response.Body)
	}
}
