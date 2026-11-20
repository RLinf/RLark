package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAPoseHoldReachesTheRobotsPowerServiceThroughItsNodeAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-03-x86", Labels: map[string]string{robotPowerStateLabel: "ready"}}}
	var paths, users, bodies []string
	answer := `{"pose":"home","moving":true,"started":true,"vendor":{"code":0}}`
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		paths, users, bodies = append(paths, r.URL.Path), append(users, r.Header.Get("X-Remote-User")), append(bodies, string(body))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer server.Close()
	g := &Gateway{config: Config{ServerAddress: server.URL}, serverTransport: http.DefaultTransport.(*http.Transport),
		accessors: map[string]*resourceAccessor{"nodes": {get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node, nil }}}}
	call := func(body string) (int, string) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "name", Value: "s1u-03-x86"}}
		c.Set(authRoleKey, roleOperator)
		c.Set(authSubjectKey, "alice")
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/rlinf.io/v1alpha1/nodes/s1u-03-x86/robot-pose?namespace=rlark-robot", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		g.handleRobotPose(c)
		return recorder.Code, recorder.Body.String()
	}

	if code, body := call(`{"pose":"home","action":"hold"}`); code != http.StatusOK || body != answer {
		t.Fatalf("hold: %d %s", code, body)
	}
	if paths[0] != "/api/nodeproxy/robot/s1u-03-x86/api/astribot/services/s1u-power/pose" || users[0] != "alice" || bodies[0] != `{"pose":"home","action":"hold"}` {
		t.Fatalf("forwarded: %v %v %v", paths, users, bodies)
	}
	// A refusal is the service's own answer; anything else the service did not answer is a 502.
	status, answer = http.StatusConflict, `{"reason":"claimed","state":{}}`
	if code, body := call(`{"pose":"home","action":"hold"}`); code != http.StatusConflict || body != answer {
		t.Fatalf("refused: %d %s", code, body)
	}
	status, answer = http.StatusBadGateway, ""
	if code, _ := call(`{"pose":"home","action":"stop"}`); code != http.StatusBadGateway {
		t.Fatalf("not reached: %d", code)
	}
	for _, body := range []string{`{"pose":"away","action":"hold"}`, `{"pose":"home","action":"go"}`} {
		if code, _ := call(body); code != http.StatusBadRequest || len(paths) != 3 {
			t.Fatalf("%s: %d", body, code)
		}
	}
	delete(node.Labels, robotPowerStateLabel)
	if code, _ := call(`{"pose":"home","action":"hold"}`); code != http.StatusBadRequest || len(paths) != 3 {
		t.Fatalf("not a robot: %d", code)
	}
}
