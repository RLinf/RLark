package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestStationRunningImages(t *testing.T) {
	const prefix = "/api/proxy/member/api/kubernetes/api/v1"
	const path = "/api/v1/station/stations/test-node/running-images?cluster_id=cluster-a&node_uid="
	for _, tc := range []struct {
		name, role, uid string
		conflict        bool
		replaced        bool
		podsStatus      int
		wantStatus      int
		wantPages       int
	}{
		{name: "running containers only across pages", role: "admin", uid: "node-uid", wantStatus: http.StatusOK, wantPages: 2},
		{name: "non-admin denied before proxy access", role: "user", uid: "node-uid", wantStatus: http.StatusForbidden},
		{name: "stale station UID rejected", role: "admin", uid: "old-uid", wantStatus: http.StatusConflict},
		{name: "ambiguous cluster namespace rejected", role: "admin", uid: "node-uid", conflict: true, wantStatus: http.StatusConflict},
		{name: "member node replaced during snapshot", role: "admin", uid: "node-uid", replaced: true, wantStatus: http.StatusConflict, wantPages: 2},
		{name: "unavailable member API is distinct from empty result", role: "admin", uid: "node-uid", podsStatus: http.StatusServiceUnavailable, wantStatus: http.StatusBadGateway, wantPages: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodeReads, podPages := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case prefix + "/nodes/test-node":
					nodeReads++
					uid := types.UID("node-uid")
					if tc.replaced && nodeReads == 2 {
						uid = "replacement"
					}
					_ = json.NewEncoder(w).Encode(corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node", UID: uid}})
				case prefix + "/pods":
					podPages++
					if r.URL.Query().Get("fieldSelector") != "spec.nodeName=test-node" || r.URL.Query().Get("limit") != "200" {
						t.Errorf("unexpected Pod query %s", r.URL.RawQuery)
					}
					if tc.podsStatus != 0 {
						w.WriteHeader(tc.podsStatus)
						return
					}
					running := corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
					container := corev1.ContainerStatus{Name: "device", Image: "registry:5443/fleet/device:v1", ImageID: "registry:5443/fleet/device@sha256:abc", State: running}
					pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-system", Name: "robot"},
						Spec: corev1.PodSpec{NodeName: "test-node"},
						Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
							container, {Name: "stopped", Image: "registry:5443/old:v0", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
						}}}
					switch r.URL.Query().Get("continue") {
					case "":
						otherNode := *pod.DeepCopy()
						otherNode.Spec.NodeName = "another-node"
						_ = json.NewEncoder(w).Encode(corev1.PodList{ListMeta: metav1.ListMeta{Continue: "next"}, Items: []corev1.Pod{pod, otherNode}})
					case "next":
						pod.Namespace, pod.Name = "bench", "capture"
						pod.Status.ContainerStatuses = nil
						pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "sidecar", Image: "capture@sha256:123", State: running}}
						_ = json.NewEncoder(w).Encode(corev1.PodList{Items: []corev1.Pod{pod}})
					default:
						t.Errorf("unexpected cursor %q", r.URL.Query().Get("continue"))
						w.WriteHeader(http.StatusBadRequest)
					}
				default:
					t.Errorf("unexpected upstream request %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			records := []runtime.Object{stationHandlerCluster("cluster-a", "rlark-member"), stationHandlerRecord("test-node", "rlark-member")}
			if tc.conflict {
				records = append(records, stationHandlerCluster("cluster-b", "rlark-member"))
			}
			g := &Gateway{
				config:          Config{ServerAddress: upstream.URL},
				serverTransport: http.DefaultTransport.(*http.Transport),
				astribotState:   astribotState{stationClient: stationHandlerClient(records...)},
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(authRoleKey, tc.role) })
			router.GET("/api/v1/station/stations/:name/running-images", requireAdmin(), g.handleStationRunningImages)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+tc.uid, nil))
			require.Equal(t, tc.wantStatus, response.Code, response.Body.String())
			require.Equal(t, tc.wantPages, podPages)
			if tc.wantPages == 0 {
				require.Zero(t, nodeReads)
			}
			if tc.wantStatus != http.StatusOK {
				require.NotContains(t, response.Body.String(), "fleet/device")
				return
			}
			require.Equal(t, 2, nodeReads)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			require.JSONEq(t, `{"items":[
				{"namespace":"bench","pod":"capture","container":"sidecar","image":"capture@sha256:123"},
				{"namespace":"rlark-system","pod":"robot","container":"device","image":"registry:5443/fleet/device:v1","imageID":"registry:5443/fleet/device@sha256:abc"}
			]}`, response.Body.String())
		})
	}
}
