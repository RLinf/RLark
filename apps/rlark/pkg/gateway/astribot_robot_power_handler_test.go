package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/zapr"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestARobotsDesiredPowerIsSetInOnePlaceAndRecordedWithItsActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-03-x86", Labels: map[string]string{robotPowerLabel: "ready.1794700000"}}}
	var patches [][]byte
	g := &Gateway{accessors: map[string]*resourceAccessor{"nodes": {
		get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node, nil },
		patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
			patches = append(patches, data)
			return node, nil
		},
		update: func(context.Context, string, string, json.RawMessage) (any, error) { return node, nil },
	}}}
	core, logged := observer.New(zapcore.InfoLevel)
	call := func(handler gin.HandlerFunc, method, body string) (int, string) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "name", Value: "s1u-03-x86"}}
		c.Set(authRoleKey, roleAdmin)
		c.Set(authSubjectKey, "alice")
		request := httptest.NewRequest(method, "/api/v1/rlinf.io/v1alpha1/nodes/s1u-03-x86?namespace=rlark-robot", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		c.Request = request.WithContext(log.WithLogger(request.Context(), zapr.NewLogger(zap.New(core))))
		handler(c)
		return recorder.Code, recorder.Body.String()
	}

	if status, body := call(g.handleSetRobotPower, http.MethodPut, `{"desired":"off"}`); status != http.StatusOK || len(patches) != 1 {
		t.Fatalf("set: %d %s, %d patches", status, body, len(patches))
	}
	var written struct {
		Metadata struct {
			Labels, Annotations map[string]string
		}
	}
	_ = json.Unmarshal(patches[0], &written)
	var record map[string]string
	_ = json.Unmarshal([]byte(written.Metadata.Annotations[robotPowerSetAnnotation]), &record)
	if value := written.Metadata.Labels[robotPowerLabel]; !strings.HasPrefix(value, "off.") || value == "off." || record["desired"] != "off" || record["by"] != "alice" || record["at"] == "" {
		t.Fatalf("the label and who set it: %s", patches[0])
	}
	entries := logged.FilterMessage("robot power").All()
	if len(entries) != 1 || entries[0].LoggerName != "audit" {
		t.Fatalf("one line of the audit log: %+v", logged.All())
	}
	fields := entries[0].ContextMap()
	if fields["actor"] != "alice" || fields["node"] != "s1u-03-x86" || fields["namespace"] != "rlark-robot" || fields["desired"] != "off" || fields["was"] != "ready.1794700000" || fields["set"] != true {
		t.Fatalf("the audit line: %+v", fields)
	}
	if status, _ := call(g.handleSetRobotPower, http.MethodPut, `{"desired":"on"}`); status != http.StatusBadRequest || len(patches) != 1 {
		t.Fatalf("a state that is none: %d", status)
	}

	// The Node's own PATCH and PUT do not change the label: every change of it is recorded.
	for _, body := range []string{`{"metadata":{"labels":{"rlark.io/robot-power":"off.1"}}}`, `{"metadata":{"labels":{"rlark.io/robot-power":null}}}`} {
		if status, answer := call(g.handleKubePatch("nodes"), http.MethodPatch, body); status != http.StatusBadRequest || !strings.Contains(answer, "robot-power") || len(patches) != 1 {
			t.Fatalf("PATCH %s: %d %s", body, status, answer)
		}
	}
	if status, answer := call(g.handleKubePatch("nodes"), http.MethodPatch, `{"metadata":{"labels":{"rlark.io/rollout-wave":"2"}}}`); status != http.StatusOK || len(patches) != 2 {
		t.Fatalf("PATCH of another label: %d %s", status, answer)
	}
	replaced := node.DeepCopy()
	for _, value := range []string{"off.2", ""} {
		replaced.Labels = map[string]string{robotPowerLabel: value}
		if value == "" {
			replaced.Labels = nil
		}
		body, _ := json.Marshal(replaced)
		if status, answer := call(g.handleKubeUpdate("nodes"), http.MethodPut, string(body)); status != http.StatusBadRequest || !strings.Contains(answer, "robot-power") {
			t.Fatalf("PUT with the label %q: %d %s", value, status, answer)
		}
	}
	body, _ := json.Marshal(node)
	if status, answer := call(g.handleKubeUpdate("nodes"), http.MethodPut, string(bytes.TrimSpace(body))); status != http.StatusOK {
		t.Fatalf("PUT that leaves the label: %d %s", status, answer)
	}
}
