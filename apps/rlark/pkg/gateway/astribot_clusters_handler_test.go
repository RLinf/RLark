package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/zapr"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestClusterDisplayMetadata(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, displayName, wantName string
	}{
		{"explicit metadata", "金山云西北宁夏裸金属集群", "金山云西北宁夏裸金属集群"},
		{"legacy fallback", "", "cluster-a"},
		{"blank metadata fallback", "  ", "cluster-a"},
		{"trim metadata", " 深圳公司本地开发集群 ", "深圳公司本地开发集群"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cluster := rlarkv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
				Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "agent-a", DisplayName: tt.displayName},
			}
			nodes := []rlarkv1alpha1.Node{{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"rlark.io/region": "node-region"}}}}
			info := buildClusterInfo(cluster, nodes)
			if info.Name != tt.wantName || info.Region != "node-region" {
				t.Fatalf("name/region = %q/%q, want %q/node-region", info.Name, info.Region, tt.wantName)
			}
			if info.ID != "cluster-a" || info.AgentNamespace != "agent-a" {
				t.Fatalf("display metadata changed cluster identity: %#v", info)
			}
			if cluster.Spec.DisplayName != tt.displayName {
				t.Fatal("response normalization mutated the source Cluster")
			}
		})
	}
}

func TestSetClusterNodeAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		body       string
		existing   *rlarkv1alpha1.ClusterNodeAgents
		wantStatus int
		want       *rlarkv1alpha1.ClusterNodeAgents
	}{
		{name: "enable with the default selector", body: `{"enabled":true,"image":"registry/rlark@sha256:1"}`, wantStatus: http.StatusOK,
			want: &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "registry/rlark@sha256:1"}},
		{name: "enable with a selector", body: `{"enabled":true,"image":"registry/rlark@sha256:1","nodeSelector":{"rlark.io/node-category":"robot"}}`, wantStatus: http.StatusOK,
			want: &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "registry/rlark@sha256:1", NodeSelector: map[string]string{"rlark.io/node-category": "robot"}}},
		{name: "disable replaces the selector", body: `{"enabled":false}`, wantStatus: http.StatusOK,
			existing: &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "registry/rlark@sha256:1", NodeSelector: map[string]string{"a": "b"}},
			want:     &rlarkv1alpha1.ClusterNodeAgents{Enabled: false}},
		{name: "the image is declared with them", body: `{"enabled":true}`, wantStatus: http.StatusBadRequest},
		{name: "with how they are rolled", body: `{"enabled":true,"image":"i","rollout":{"canary":2,"maxUnavailable":"25%"}}`, wantStatus: http.StatusOK,
			want: &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: "i", Rollout: &rlarkv1alpha1.RolloutPolicy{Canary: ptr.To(int32(2)), MaxUnavailable: ptr.To(intstr.FromString("25%"))}}},
		{name: "a bound that is none is rejected", body: `{"enabled":true,"image":"i","rollout":{"maxUnavailable":"a tenth"}}`, wantStatus: http.StatusBadRequest},
		{name: "enabled is required", body: `{"nodeSelector":{"a":"b"}}`, wantStatus: http.StatusBadRequest},
		{name: "empty selector is rejected", body: `{"enabled":true,"image":"i","nodeSelector":{}}`, wantStatus: http.StatusBadRequest},
		{name: "invalid label key is rejected", body: `{"enabled":true,"image":"i","nodeSelector":{"bad key":"x"}}`, wantStatus: http.StatusBadRequest},
		{name: "invalid label value is rejected", body: `{"enabled":true,"image":"i","nodeSelector":{"a":"not valid!"}}`, wantStatus: http.StatusBadRequest},
		{name: "malformed body is rejected", body: `{"enabled":"yes"}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &rlarkv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
				Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-cluster-a", Cordoned: true, NodeAgents: tt.existing},
			}
			client := clientset.NewSimpleClientset(cluster)
			gateway := &Gateway{kubeClient: client}
			router := gin.New()
			router.PUT("/clusters/:cluster_id/node-agents", gateway.setClusterNodeAgents)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/clusters/cluster-a/node-agents", strings.NewReader(tt.body)))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			stored, err := client.RlinfV1alpha1().Clusters().Get(t.Context(), "cluster-a", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := tt.want
			if tt.wantStatus != http.StatusOK {
				want = tt.existing
			}
			if !reflect.DeepEqual(stored.Spec.NodeAgents, want) {
				t.Fatalf("spec.nodeAgents = %#v, want %#v", stored.Spec.NodeAgents, want)
			}
			if !stored.Spec.Cordoned {
				t.Fatal("other spec fields must be kept")
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var response struct {
				Data ClusterInfo `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(response.Data.NodeAgents, want) {
				t.Fatalf("response nodeAgents = %#v, want %#v", response.Data.NodeAgents, want)
			}
		})
	}
}

func TestSetClusterDisplayName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, cluster, body string
		wantStatus          int
		wantDisplayName     string
		wantName            string
	}{
		{name: "set", cluster: "cluster-a", body: `{"displayName":" 百度GPU-1 "}`, wantStatus: http.StatusOK, wantDisplayName: "百度GPU-1", wantName: "百度GPU-1"},
		{name: "clear falls back to the ID", cluster: "cluster-a", body: `{"displayName":""}`, wantStatus: http.StatusOK, wantName: "cluster-a"},
		{name: "own name again", cluster: "cluster-a", body: `{"displayName":"宁夏"}`, wantStatus: http.StatusOK, wantDisplayName: "宁夏", wantName: "宁夏"},
		{name: "own ID", cluster: "cluster-a", body: `{"displayName":"cluster-a"}`, wantStatus: http.StatusOK, wantDisplayName: "cluster-a", wantName: "cluster-a"},
		{name: "a character outside the class", cluster: "cluster-a", body: `{"displayName":"a/b"}`, wantStatus: http.StatusBadRequest, wantDisplayName: "宁夏"},
		{name: "65 characters", cluster: "cluster-a", body: `{"displayName":"` + strings.Repeat("名", 65) + `"}`, wantStatus: http.StatusBadRequest, wantDisplayName: "宁夏"},
		{name: "another cluster's display name", cluster: "cluster-a", body: `{"displayName":"深圳"}`, wantStatus: http.StatusConflict, wantDisplayName: "宁夏"},
		{name: "another cluster's ID", cluster: "cluster-a", body: `{"displayName":"cluster-b"}`, wantStatus: http.StatusConflict, wantDisplayName: "宁夏"},
		{name: "unknown cluster", cluster: "cluster-z", body: `{"displayName":"x"}`, wantStatus: http.StatusNotFound, wantDisplayName: "宁夏"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := clientset.NewSimpleClientset(
				&rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-cluster-a", DisplayName: "宁夏", Cordoned: true}},
				&rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-cluster-b", DisplayName: "深圳"}},
			)
			core, logged := observer.New(zapcore.InfoLevel)
			gateway := &Gateway{kubeClient: client}
			router := gin.New()
			router.PUT("/clusters/:cluster_id/display-name", func(c *gin.Context) {
				c.Set(authSubjectKey, "alice")
				c.Request = c.Request.WithContext(log.WithLogger(c.Request.Context(), zapr.NewLogger(zap.New(core))))
				gateway.setClusterDisplayName(c)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/clusters/"+tt.cluster+"/display-name", strings.NewReader(tt.body)))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			stored, err := client.RlinfV1alpha1().Clusters().Get(t.Context(), "cluster-a", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if stored.Spec.DisplayName != tt.wantDisplayName || !stored.Spec.Cordoned {
				t.Fatalf("stored spec = %+v, want displayName %q and the rest kept", stored.Spec, tt.wantDisplayName)
			}
			if tt.wantStatus == http.StatusConflict && !strings.Contains(recorder.Body.String(), "display name already used by cluster-b") {
				t.Fatalf("conflict names the other cluster: %s", recorder.Body.String())
			}
			if tt.wantStatus == http.StatusBadRequest {
				return
			}
			entries := logged.FilterMessage("cluster display name").All()
			if len(entries) != 1 || entries[0].LoggerName != "audit" {
				t.Fatalf("one line of the audit log: %+v", logged.All())
			}
			fields := entries[0].ContextMap()
			if fields["actor"] != "alice" || fields["cluster"] != tt.cluster || fields["set"] != (tt.wantStatus == http.StatusOK) {
				t.Fatalf("the audit line: %+v", fields)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if fields["was"] != "宁夏" || fields["now"] != tt.wantDisplayName {
				t.Fatalf("the audit line: %+v", fields)
			}
			var response struct {
				Data ClusterInfo `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.ID != "cluster-a" || response.Data.Name != tt.wantName || response.Data.DisplayName != tt.wantDisplayName {
				t.Fatalf("response = %+v", response.Data)
			}
		})
	}
}
