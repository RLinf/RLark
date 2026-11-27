package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStationBodyState(t *testing.T) {
	const path = "/api/v1/station/stations/test-node/body-state?cluster_id=cluster-a&node_uid="
	const node = `{"metadata":{"name":"test-node","uid":"node-uid","labels":{"rlark.io/robot-model":"s1u","rlark.io/robot-body":"s1u_observer"}}}`
	const pod = `{"metadata":{"name":"s1u-robot-test","namespace":"rlark-system","ownerReferences":[{"apiVersion":"apps/v1","kind":"DaemonSet","controller":true,"name":"s1u-robot","uid":"addon-uid"}]},"spec":{"nodeName":"test-node"},"status":{"phase":"Running"}}`
	const snapshot = `{"version":1,"nodeUID":"node-uid","body":"s1u_observer","joints":[{"id":"torso","state":"live","position":[0,0,1,2]}],"endpoints":[]}`
	for _, tc := range []struct {
		name, role, uid, pod, snapshot, state string
		status, requests                      int
	}{
		{name: "live", role: "admin", uid: "node-uid", pod: pod, snapshot: snapshot, state: "available", status: 200, requests: 4},
		{name: "absent collector", role: "admin", uid: "node-uid", state: "unavailable", status: 200, requests: 3},
		{name: "resumed collector", role: "admin", uid: "node-uid", pod: pod, snapshot: snapshot, state: "available", status: 200, requests: 4},
		{name: "wrong collector binding", role: "admin", uid: "node-uid", pod: strings.ReplaceAll(pod, "addon-uid", "other-uid"), state: "unavailable", status: 200, requests: 3},
		{name: "older addon without body route", role: "admin", uid: "node-uid", pod: pod, state: "unavailable", status: 200, requests: 4},
		{name: "wrong snapshot", role: "admin", uid: "node-uid", pod: pod, snapshot: strings.ReplaceAll(snapshot, "s1u_observer", "s1u"), status: 502, requests: 4},
		{name: "stale node references forbidden", role: "admin", uid: "other-uid", status: 409},
		{name: "user denied", role: "user", uid: "node-uid", status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				switch strings.TrimPrefix(r.URL.Path, "/api/proxy/member/api/kubernetes/") {
				case "api/v1/nodes/test-node":
					_, _ = w.Write([]byte(node))
				case "apis/apps/v1/namespaces/rlark-system/daemonsets/s1u-robot":
					_, _ = w.Write([]byte(`{"metadata":{"name":"s1u-robot","namespace":"rlark-system","uid":"addon-uid"}}`))
				case "api/v1/namespaces/rlark-system/pods":
					if r.URL.Query().Get("labelSelector") != robotAddonSelector || r.URL.Query().Get("fieldSelector") != "spec.nodeName=test-node" {
						t.Errorf("unbounded Pod query: %s", r.URL.RawQuery)
					}
					_, _ = w.Write([]byte(`{"items":[` + tc.pod + `]}`))
				case "api/v1/namespaces/rlark-system/pods/s1u-robot-test:18891/proxy/body-state":
					if tc.snapshot == "" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_, _ = w.Write([]byte(tc.snapshot))
				default:
					t.Errorf("unexpected upstream request %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			g := &Gateway{
				config:          Config{ServerAddress: upstream.URL},
				serverTransport: http.DefaultTransport.(*http.Transport),
				astribotState: astribotState{stationClient: stationHandlerClient(
					stationHandlerCluster("cluster-a", "rlark-member"), stationHandlerRecord("test-node", "rlark-member"))},
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(authRoleKey, tc.role) })
			router.GET("/api/v1/station/stations/:name/body-state", requireAdmin(), g.handleStationBodyState)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+tc.uid, nil))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Equal(t, tc.requests, requests)
			if tc.state == "" {
				return
			}
			var payload struct {
				State        string
				Capabilities json.RawMessage
				Snapshot     *stationBodySnapshot
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			require.Equal(t, tc.state, payload.State)
			require.JSONEq(t, string(stationModelCapabilities("s1u")), string(payload.Capabilities))
			require.Equal(t, tc.state == "available", payload.Snapshot != nil)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		})
	}
}
