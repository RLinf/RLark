package gateway

import (
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

func TestANodesDisplayNameIsSetInOnePlaceUniqueAndRecordedWithItsActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-baidu", Name: "instance-gt4l6j5v-1"}, Spec: rlarkv1alpha1.NodeSpec{DisplayName: "百度GPU-1"}}
	others := []rlarkv1alpha1.Node{
		*node,
		{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-02-x86"}, Spec: rlarkv1alpha1.NodeSpec{DisplayName: "机器人2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-03-x86"}},
	}
	var patches [][]byte
	g := &Gateway{accessors: map[string]*resourceAccessor{"nodes": {
		list: func(_ context.Context, namespace string, _ metav1.ListOptions) (any, error) {
			if namespace != metav1.NamespaceAll {
				t.Fatalf("uniqueness lists the Nodes of namespace %q, not of all", namespace)
			}
			return &rlarkv1alpha1.NodeList{Items: others}, nil
		},
		get: func(context.Context, string, string, metav1.GetOptions) (any, error) { return node, nil },
		patch: func(_ context.Context, _, _ string, data []byte) (any, error) {
			patches = append(patches, data)
			return node, nil
		},
		update: func(context.Context, string, string, json.RawMessage) (any, error) { return node, nil },
	}}}
	core, logged := observer.New(zapcore.InfoLevel)
	call := func(handler gin.HandlerFunc, role, method, body string) (int, string) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "name", Value: "instance-gt4l6j5v-1"}}
		c.Set(authRoleKey, role)
		c.Set(authSubjectKey, "alice")
		request := httptest.NewRequest(method, "/api/v1/rlinf.io/v1alpha1/nodes/instance-gt4l6j5v-1?namespace=rlark-baidu", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		c.Request = request.WithContext(log.WithLogger(request.Context(), zapr.NewLogger(zap.New(core))))
		requireAdmin()(c)
		if !c.IsAborted() {
			handler(c)
		}
		return recorder.Code, recorder.Body.String()
	}
	written := func(i int) any {
		var patch struct {
			Spec map[string]any `json:"spec"`
		}
		_ = json.Unmarshal(patches[i], &patch)
		value, named := patch.Spec["displayName"]
		if !named || len(patch.Spec) != 1 {
			t.Fatalf("the patch names spec.displayName alone: %s", patches[i])
		}
		return value
	}

	if status, body := call(g.handleSetNodeDisplayName, roleAdmin, http.MethodPut, `{"displayName":" 百度GPU-2 "}`); status != http.StatusOK || len(patches) != 1 || written(0) != "百度GPU-2" {
		t.Fatalf("set: %d %s, %d patches", status, body, len(patches))
	}
	entries := logged.FilterMessage("node display name").All()
	if len(entries) != 1 || entries[0].LoggerName != "audit" {
		t.Fatalf("one line of the audit log: %+v", logged.All())
	}
	fields := entries[0].ContextMap()
	if fields["actor"] != "alice" || fields["node"] != "instance-gt4l6j5v-1" || fields["namespace"] != "rlark-baidu" || fields["was"] != "百度GPU-1" || fields["now"] != "百度GPU-2" || fields["set"] != true {
		t.Fatalf("the audit line: %+v", fields)
	}
	// Its own name, as display name or as metadata.name, is no conflict.
	for i, body := range []string{`{"displayName":"百度GPU-1"}`, `{"displayName":"instance-gt4l6j5v-1"}`} {
		if status, answer := call(g.handleSetNodeDisplayName, roleAdmin, http.MethodPut, body); status != http.StatusOK || len(patches) != 2+i {
			t.Fatalf("own name %s: %d %s", body, status, answer)
		}
	}
	// Cleared, the field is taken away: the CRD refuses an empty display name.
	if status, answer := call(g.handleSetNodeDisplayName, roleAdmin, http.MethodPut, `{"displayName":"  "}`); status != http.StatusOK || written(3) != nil {
		t.Fatalf("clear: %d %s", status, answer)
	}
	for body, want := range map[string]int{
		`{"displayName":"机器人2"}`:                            http.StatusConflict,
		`{"displayName":"s1u-03-x86"}`:                      http.StatusConflict,
		`{"displayName":"a b"}`:                             http.StatusBadRequest,
		`{"displayName":"` + strings.Repeat("a", 65) + `"}`: http.StatusBadRequest,
		`{"displayName":7}`:                                 http.StatusBadRequest,
	} {
		if status, answer := call(g.handleSetNodeDisplayName, roleAdmin, http.MethodPut, body); status != want || len(patches) != 4 {
			t.Fatalf("%s: %d %s, want %d", body, status, answer, want)
		} else if want == http.StatusConflict && !strings.Contains(answer, "display name already used by rlark-robot/s1u-0") {
			t.Fatalf("the conflict names the other Node: %s", answer)
		}
	}
	if status, _ := call(g.handleSetNodeDisplayName, "user", http.MethodPut, `{"displayName":"x"}`); status != http.StatusForbidden || len(patches) != 4 {
		t.Fatalf("a tenant user: %d", status)
	}

	// The Node's own PATCH and PUT do not change the display name.
	for _, body := range []string{`{"spec":{"displayName":"x"}}`, `{"spec":{"displayName":null}}`} {
		if status, answer := call(g.handleKubePatch("nodes"), roleAdmin, http.MethodPatch, body); status != http.StatusBadRequest || !strings.Contains(answer, "display-name") || len(patches) != 4 {
			t.Fatalf("PATCH %s: %d %s", body, status, answer)
		}
	}
	for _, body := range []string{`{"spec":{"displayName":"百度GPU-1","unschedulable":true}}`, `{"spec":{"unschedulable":true}}`} {
		if status, answer := call(g.handleKubePatch("nodes"), roleAdmin, http.MethodPatch, body); status != http.StatusOK {
			t.Fatalf("PATCH that leaves the display name %s: %d %s", body, status, answer)
		}
	}
	for _, displayName := range []string{"x", ""} {
		replaced := node.DeepCopy()
		replaced.Spec.DisplayName = displayName
		body, _ := json.Marshal(replaced)
		if status, answer := call(g.handleKubeUpdate("nodes"), roleAdmin, http.MethodPut, string(body)); status != http.StatusBadRequest || !strings.Contains(answer, "display-name") {
			t.Fatalf("PUT with the display name %q: %d %s", displayName, status, answer)
		}
	}
	body, _ := json.Marshal(node)
	if status, answer := call(g.handleKubeUpdate("nodes"), roleAdmin, http.MethodPut, string(body)); status != http.StatusOK {
		t.Fatalf("PUT that leaves the display name: %d %s", status, answer)
	}
}
