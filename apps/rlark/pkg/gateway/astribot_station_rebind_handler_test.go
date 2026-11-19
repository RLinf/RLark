package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func stationRebindTestGateway(objects ...runtime.Object) *Gateway {
	return &Gateway{astribotState: astribotState{stationClient: stationHandlerClient(objects...)}}
}

func stationRebindTestRequest() createStationRebindRequest {
	return createStationRebindRequest{
		ClusterID: "cluster-a", RequestID: "request-001", StationName: "station-a",
		StationUID: "station-uid-a", ExpectedBindingVersion: 1,
		TargetNodeName: "node-new", TargetNodeUID: "node-uid-new", Reason: "node replacement approved",
	}
}

func stationRebindCall(g *Gateway, request createStationRebindRequest, actor string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(request)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/station/rebinds", strings.NewReader(string(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(authSubjectKey, actor)
	g.handleCreateStationRebind(ctx)
	return recorder
}

func TestCreateStationRebindRejectsUnknownOrTrailingJSON(t *testing.T) {
	base, err := json.Marshal(stationRebindTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"unknown field":   strings.TrimSuffix(string(base), "}") + `,"namespace":"other"}`,
		"trailing object": string(base) + ` {}`,
		"oversized body":  strings.TrimSuffix(string(base), "}") + `,"reason":"` + strings.Repeat("x", maxStationRebindRequestBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			gateway := stationRebindTestGateway(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerStation("station-a", "agent-a"))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/station/rebinds", strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(authSubjectKey, "admin")
			gateway.handleCreateStationRebind(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			for _, action := range gateway.stationClient.(*dynamicfake.FakeDynamicClient).Actions() {
				if action.GetVerb() == "create" {
					t.Fatal("invalid JSON created an operation")
				}
			}
		})
	}
}

func TestCreateStationRebindIsIdempotentAndRejectsRequestIDReuse(t *testing.T) {
	gateway := stationRebindTestGateway(
		stationHandlerCluster("cluster-a", "agent-a"),
		stationHandlerStation("station-a", "agent-a"),
	)
	request := stationRebindTestRequest()
	first := stationRebindCall(gateway, request, "admin")
	if first.Code != http.StatusAccepted || !strings.Contains(first.Body.String(), `"created":true`) {
		t.Fatalf("first response status=%d body=%s", first.Code, first.Body.String())
	}
	second := stationRebindCall(gateway, request, "admin")
	if second.Code != http.StatusAccepted || !strings.Contains(second.Body.String(), `"created":false`) {
		t.Fatalf("replay response status=%d body=%s", second.Code, second.Body.String())
	}
	operationName := stationRebindOperationName(request.ClusterID, request.RequestID)
	operation, err := gateway.stationClient.Resource(stationRebindGVR).Namespace("agent-a").Get(t.Context(), operationName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if operation.GetKind() != "StationRebind" || operation.GetNamespace() != "agent-a" || operation.Object["spec"].(map[string]interface{})["requestedBy"] != "admin" {
		t.Fatalf("unexpected operation: %#v", operation.Object)
	}
	request.Reason = "different request payload"
	conflict := stationRebindCall(gateway, request, "admin")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("reused request ID status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestCreateStationRebindRecoversCreateRace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := stationHandlerClient(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerStation("station-a", "agent-a"))
	operationName := stationRebindOperationName("cluster-a", "request-001")
	client.PrependReactor("create", "stationrebinds", func(action ktesting.Action) (bool, runtime.Object, error) {
		create := action.(ktesting.CreateAction)
		if err := client.Tracker().Create(stationRebindGVR, create.GetObject(), create.GetNamespace()); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: stationRebindGVR.Group, Resource: stationRebindGVR.Resource}, operationName)
	})
	gateway := &Gateway{astribotState: astribotState{stationClient: client}}
	response := stationRebindCall(gateway, stationRebindTestRequest(), "admin")
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"created":false`) {
		t.Fatalf("create race response status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateStationRebindRejectsStaleOrUnsafeRequests(t *testing.T) {
	objects := []runtime.Object{stationHandlerCluster("cluster-a", "agent-a"), stationHandlerStation("station-a", "agent-a")}
	for name, mutate := range map[string]func(*createStationRebindRequest){
		"station UID changed":      func(r *createStationRebindRequest) { r.StationUID = "old-station-uid" },
		"binding version changed":  func(r *createStationRebindRequest) { r.ExpectedBindingVersion = 2 },
		"invalid target node name": func(r *createStationRebindRequest) { r.TargetNodeName = "../node" },
		"blank reason":             func(r *createStationRebindRequest) { r.Reason = "   " },
	} {
		t.Run(name, func(t *testing.T) {
			gateway := stationRebindTestGateway(objects...)
			request := stationRebindTestRequest()
			mutate(&request)
			response := stationRebindCall(gateway, request, "admin")
			want := http.StatusConflict
			if name == "invalid target node name" || name == "blank reason" {
				want = http.StatusBadRequest
			}
			if response.Code != want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
			}
			for _, action := range gateway.stationClient.(*dynamicfake.FakeDynamicClient).Actions() {
				if action.GetVerb() == "create" {
					t.Fatal("invalid rebind request created an operation")
				}
			}
		})
	}
	retired := stationHandlerStation("station-a", "agent-a")
	if err := unstructured.SetNestedField(retired.Object, true, "spec", "retired"); err != nil {
		t.Fatal(err)
	}
	response := stationRebindCall(stationRebindTestGateway(stationHandlerCluster("cluster-a", "agent-a"), retired), stationRebindTestRequest(), "admin")
	if response.Code != http.StatusConflict {
		t.Fatalf("retired Station status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStationRebindRouteRequiresAdministratorJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte("01234567890123456789012345678901")
	gateway := stationRebindTestGateway(stationHandlerCluster("cluster-a", "agent-a"), stationHandlerStation("station-a", "agent-a"))
	gateway.jwtSigningKey = key
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
	body, _ := json.Marshal(stationRebindTestRequest())
	for _, test := range []struct {
		name   string
		token  string
		status int
	}{{"missing token", "", http.StatusUnauthorized}, {"user role", userToken, http.StatusForbidden}, {"admin role", adminToken, http.StatusAccepted}} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/station/rebinds", strings.NewReader(string(body)))
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
	operationName := stationRebindOperationName("cluster-a", "request-001")
	for _, path := range []string{
		"/api/v1/station/rebinds/" + operationName + "?cluster_id=cluster-a",
		"/api/v1/station/observations?cluster_id=cluster-a",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}
