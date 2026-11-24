package gateway

import (
	"encoding/json"
	rlarkfake "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"
)

func nodeOperationTestRequest() nodeOperationCreateRequest {
	return nodeOperationCreateRequest{ClusterID: "cluster-a", RequestID: "request-001", Action: "Remove", NodeName: "worker-a", NodeUID: "node-uid"}
}

func nodeOperationCall(g *Gateway, request nodeOperationCreateRequest) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(request)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/station/node-operations", strings.NewReader(string(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(authSubjectKey, "admin")
	g.handleCreateNodeOperation(ctx)
	return recorder
}

func TestCreateNodeOperationStrictJSONAndValidation(t *testing.T) {
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"))
	for name, body := range map[string]string{
		"unknown field":   `{"cluster_id":"cluster-a","request_id":"r","action":"Remove","node_name":"worker","node_uid":"u","namespace":"other"}`,
		"a join":          `{"cluster_id":"cluster-a","request_id":"r","action":"Join","node_name":"worker","node_uid":"u"}`,
		"trailing object": `{}` + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/station/node-operations", strings.NewReader(body))
			ctx.Set(authSubjectKey, "admin")
			(&Gateway{astribotState: astribotState{stationClient: client}, rawClient: fake.NewSimpleClientset(), kubeClient: rlarkfake.NewSimpleClientset()}).handleCreateNodeOperation(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	request := nodeOperationTestRequest()
	request.NodeName = "bad name"
	if got := nodeOperationCall(&Gateway{astribotState: astribotState{stationClient: client}, kubeClient: rlarkfake.NewSimpleClientset()}, request); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid node name status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestCreateNodeOperationRejectsRequestIDReuse(t *testing.T) {
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"))
	gateway := &Gateway{astribotState: astribotState{stationClient: client}, kubeClient: rlarkfake.NewSimpleClientset()}
	request := nodeOperationTestRequest()
	if got := nodeOperationCall(gateway, request); got.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", got.Code, got.Body.String())
	}
	if got := nodeOperationCall(gateway, request); got.Code != http.StatusAccepted {
		t.Fatalf("replayed status=%d body=%s", got.Code, got.Body.String())
	}
	request.NodeName = "worker-b"
	if got := nodeOperationCall(gateway, request); got.Code != http.StatusConflict {
		t.Fatalf("reused request ID status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestCreateNodeOperationRejectsASecondRemovalWhileOneIsActive(t *testing.T) {
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"))
	gateway := &Gateway{astribotState: astribotState{stationClient: client}, kubeClient: rlarkfake.NewSimpleClientset()}
	first := nodeOperationTestRequest()
	if got := nodeOperationCall(gateway, first); got.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", got.Code, got.Body.String())
	}
	second := first
	second.RequestID = "request-002"
	if got := nodeOperationCall(gateway, second); got.Code != http.StatusConflict {
		t.Fatalf("an active removal accepted a duplicate: status=%d body=%s", got.Code, got.Body.String())
	}
	// A different node is not blocked by the first node's operation.
	other := second
	other.RequestID, other.NodeName = "request-003", "worker-b"
	if got := nodeOperationCall(gateway, other); got.Code != http.StatusAccepted {
		t.Fatalf("another node status=%d body=%s", got.Code, got.Body.String())
	}
	// A terminal operation may be retried with a new request ID.
	stored, err := client.Resource(nodeOperationGVR).Namespace("agent-a").Get(t.Context(), nodeOperationName(first.ClusterID, first.RequestID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored.Object["status"] = map[string]any{"phase": "Blocked"}
	if _, err := client.Resource(nodeOperationGVR).Namespace("agent-a").Update(t.Context(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := nodeOperationCall(gateway, second); got.Code != http.StatusAccepted {
		t.Fatalf("retry after a terminal operation status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestNodeOperationRouteRequiresAdministratorJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte(strings.Repeat("k", 32))
	gateway := &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"))}, rawClient: fake.NewSimpleClientset(), kubeClient: rlarkfake.NewSimpleClientset(), jwtSigningKey: key}
	router := gin.New()
	gateway.RegisterRoutes(router)
	userToken, _, err := gateway.issueJWT("user", "user")
	if err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := gateway.issueJWT("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(nodeOperationTestRequest())
	for _, test := range []struct {
		name   string
		token  string
		status int
	}{{"missing token", "", http.StatusUnauthorized}, {"user role", userToken, http.StatusForbidden}, {"admin role", adminToken, http.StatusAccepted}} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/station/node-operations", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}

func TestARemovalLogsInNowhere(t *testing.T) {
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"))
	gateway := &Gateway{astribotState: astribotState{stationClient: client}, kubeClient: rlarkfake.NewSimpleClientset()}
	request := nodeOperationCreateRequest{ClusterID: "cluster-a", RequestID: "remove-nb260706069", Action: "Remove", NodeName: "nb260706069", NodeUID: "node-uid"}
	recorder := nodeOperationCall(gateway, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	stored, err := client.Resource(nodeOperationGVR).Namespace("agent-a").Get(t.Context(), nodeOperationName(request.ClusterID, request.RequestID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _, _ := unstructured.NestedMap(stored.Object, "spec")
	if spec["ssh"] != nil || spec["nodeUID"] != "node-uid" || spec["requestedBy"] != "admin" {
		t.Fatalf("spec %#v", spec)
	}
	// A name RLark never verified a Node of (no site node record) is removed by its name.
	byName := request
	byName.RequestID, byName.NodeName, byName.NodeUID = "remove-cailiang-desktop", "cailiang-desktop", ""
	if got := nodeOperationCall(gateway, byName); got.Code != http.StatusAccepted {
		t.Fatalf("a removal by name: status=%d body=%s", got.Code, got.Body.String())
	}
	stored, err = client.Resource(nodeOperationGVR).Namespace("agent-a").Get(t.Context(), nodeOperationName(byName.ClusterID, byName.RequestID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := unstructured.NestedString(stored.Object, "spec", "nodeUID"); found {
		t.Fatalf("a removal by name names no UID: %#v", stored.Object["spec"])
	}
}
