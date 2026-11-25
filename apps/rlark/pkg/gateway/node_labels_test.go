package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	v1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// nodeLabelsClusterFixture builds a minimal fake clientset with a Cluster whose
// AgentNamespace resolves to "rlark-agent-a" so Gateway strips "rlark-" yielding
// agentID "agent-a". Callers can override the cluster spec for format-error tests.
func nodeLabelsClusterFixture(namespace, agentNamespace string) *clientset.Clientset {
	return clientset.NewSimpleClientset(
		&v1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:   namespace,
				Labels: map[string]string{tenant.IDLabel: tenant.DefaultID},
			},
			Spec: v1alpha1.ClusterSpec{AgentNamespace: agentNamespace},
		},
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-a",
				Namespace: namespace,
				Labels:    map[string]string{tenant.IDLabel: tenant.DefaultID},
			},
		},
	)
}

// newProxyFixture spins up a test server that acts as both the "Server" (the
// gateway's proxy target) and as a fake data-plane k8s handler. It records the
// patch body the Gateway sent so tests can assert strategic merge patch shape.
// Returned transport is just http.DefaultTransport but points at a ServerAddress
// whose proxy path this test server handles.
func newProxyFixture(t *testing.T) (*httptest.Server, *http.Transport) {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server's /api/proxy/{agentID}/api/kubernetes/{path} — the Gateway
		// constructs this URL; we parse it directly.
		// In tests we don't actually validate the agent ID — the test server
		// just echoes whatever the Gateway sent to /api/v1/nodes/{name} PATCH.
		if strings.HasPrefix(r.URL.Path, "/api/proxy/agent-a/api/kubernetes/api/v1/nodes/") {
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"metadata":{"labels":{"patched":"true"},"patchBody":%q}}`, string(body))
			return
		}
		// Simulate agent not found / upstream failure for malformed IDs.
		if strings.HasPrefix(r.URL.Path, "/api/proxy/missing-agent/") {
			http.Error(w, "agent not found", http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	return server, server.Client().Transport.(*http.Transport)
}

func TestPatchNodeLabelsRelaySuccess(t *testing.T) {
	server, transport := newProxyFixture(t)
	client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")

	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: transport,
		config:          Config{ServerAddress: server.URL},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"team.example/zone":"us-west","old-key":null}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Success  bool   `json:"success"`
		AgentID  string `json:"agentId"`
		NodeName string `json:"nodeName"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, recorder.Body.String())
	}
	if !resp.Success {
		t.Fatalf("success = false")
	}
	if resp.AgentID != "agent-a" {
		t.Fatalf("agentId = %q, want agent-a (stripped from rlark-agent-a)", resp.AgentID)
	}
	if resp.NodeName != "node-a" {
		t.Fatalf("nodeName = %q, want node-a", resp.NodeName)
	}
}

func TestPatchNodeLabelsRelayRequiresNamespace(t *testing.T) {
	// The Gateway must refuse before hitting any IO.
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels",
		strings.NewReader(`{"labels":{"a":"b"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g := &Gateway{
		kubeClient:      clientset.NewSimpleClientset(),
		accessors:       registerAccessors(clientset.NewSimpleClientset()),
		serverTransport: &http.Transport{},
	}
	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// rlark.io/* labels are no longer reserved; they are relayed to the data plane
// like any other label key.
func TestPatchNodeLabelsRelayAllowsReservedPrefix(t *testing.T) {
	for _, label := range []string{"rlark.io/zone", "rlark.io/cluster-id"} {
		t.Run(label, func(t *testing.T) {
			server, transport := newProxyFixture(t)
			client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")
			g := &Gateway{
				kubeClient:      client,
				accessors:       registerAccessors(client),
				serverTransport: transport,
				config:          Config{ServerAddress: server.URL},
				users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
				tenants:         tenant.StaticProvider{},
			}

			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPatch,
				"/nodes/node-a/patch-labels?namespace=cluster-a",
				strings.NewReader(fmt.Sprintf(`{"labels":{%q:"allowed"}}`, label)))
			c.Params = gin.Params{{Key: "name", Value: "node-a"}}
			c.Set(authSubjectKey, "user")

			g.patchNodeLabelsRelay(c)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
			}
			var resp struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v\nbody: %s", err, recorder.Body.String())
			}
			if !resp.Success {
				t.Fatalf("success = false; label %q must be relayed, not rejected", label)
			}
		})
	}
}

func TestPatchNodeLabelsRelayRejectsEmptyLabels(t *testing.T) {
	server, transport := newProxyFixture(t)
	client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")
	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: transport,
		config:          Config{ServerAddress: server.URL},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`not json`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g := &Gateway{
		kubeClient:      clientset.NewSimpleClientset(),
		accessors:       registerAccessors(clientset.NewSimpleClientset()),
		serverTransport: &http.Transport{},
	}
	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayClusterNotFound(t *testing.T) {
	// No Cluster CR in clientset → relay should fail with 404 before hitting proxy.
	client := clientset.NewSimpleClientset()
	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: &http.Transport{},
		config:          Config{ServerAddress: "https://server"},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=missing",
		strings.NewReader(`{"labels":{"k":"v"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayBadAgentNamespaceFormat(t *testing.T) {
	// Cluster.Spec.AgentNamespace doesn't start with "rlark-" → the handler
	// should return 500 InternalError because it can't derive an agentID.
	client := nodeLabelsClusterFixture("cluster-a", "agent-a") // missing "rlark-" prefix
	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: &http.Transport{},
		config:          Config{ServerAddress: "https://server"},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"k":"v"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayServerTransportMissing(t *testing.T) {
	// When serverTransport is not configured (Gateway built for kube-only use)
	// the handler must fail fast before touching the proxy URL.
	client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		// serverTransport intentionally nil
		users:   stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants: tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"k":"v"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayRelayUpstreamError(t *testing.T) {
	// We construct a dedicated test server that returns an HTTP error so we
	// can verify the Gateway forwards the upstream response verbatim.
	failServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kubernetes validation: label value too long", http.StatusUnprocessableEntity)
	}))
	defer failServer.Close()
	transport := failServer.Client().Transport.(*http.Transport)

	client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")
	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: transport,
		config:          Config{ServerAddress: failServer.URL},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"k":"v"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	// Our handler forwards upstream responses ≥300 verbatim, so the caller sees
	// the kubernetes API's 422 (Unprocessable Entity) directly.
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "kubernetes validation") {
		t.Fatalf("body %q missing upstream error message", recorder.Body.String())
	}
}

func TestPatchNodeLabelsRelayForwardsStrategicMergePatch(t *testing.T) {
	// The handler must wrap caller labels into a strategic merge patch of
	// metadata.labels and forward it to the agent. Assert on the exact body
	// shape the server receives.
	var receivedBody []byte
	var receivedContentType string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport)

	client := nodeLabelsClusterFixture("cluster-a", "rlark-agent-a")
	g := &Gateway{
		kubeClient:      client,
		accessors:       registerAccessors(client),
		serverTransport: transport,
		config:          Config{ServerAddress: server.URL},
		users:           stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:         tenant.StaticProvider{},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"team.example/zone":"us-west","obsolete":null}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if receivedContentType != string(types.StrategicMergePatchType) {
		t.Fatalf("Content-Type = %q, want StrategicMergePatchType %q", receivedContentType, types.StrategicMergePatchType)
	}

	var got map[string]any
	if err := json.Unmarshal(receivedBody, &got); err != nil {
		t.Fatalf("invalid forwarded body %q: %v", string(receivedBody), err)
	}
	meta, _ := got["metadata"].(map[string]any)
	if meta == nil {
		t.Fatalf("forwarded body missing metadata wrapper: %s", string(receivedBody))
	}
	labels, _ := meta["labels"].(map[string]any)
	if labels == nil {
		t.Fatalf("forwarded body missing metadata.labels: %s", string(receivedBody))
	}
	if labels["team.example/zone"] != "us-west" {
		t.Fatalf("missing 'team.example/zone' in forwarded labels: %#v", labels)
	}
	if labels["obsolete"] != nil {
		t.Fatalf("null marker for delete must survive JSON round-trip, got %#v", labels["obsolete"])
	}
}

// TestPatchNodeLabelsRelayTranslatesClusterNotFoundError verifies that a
// legitimate "cluster does not exist" error from the kube client stays a 404
// instead of surfacing as an opaque 500. The handler must distinguish between
// apierrors.IsNotFound (user error) and unexpected transport errors.
func TestPatchNodeLabelsRelayTranslatesClusterNotFoundError(t *testing.T) {
	client := clientset.NewSimpleClientset()
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:    tenant.StaticProvider{},
	}

	// Inject a synthetic apierrors.IsNotFound via a reactor so we don't depend
	// on the real fake client's "no such Cluster" path.
	client.PrependReactor("get", "clusters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(v1alpha1.Resource("cluster"), "cluster-a")
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch,
		"/nodes/node-a/patch-labels?namespace=cluster-a",
		strings.NewReader(`{"labels":{"k":"v"}}`))
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	c.Set(authSubjectKey, "user")

	g.patchNodeLabelsRelay(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
}
