package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/gin-gonic/gin"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestClusterTransitionErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		update  func(*rlarkv1alpha1.Cluster) error
		cluster rlarkv1alpha1.Cluster
		wantErr error
	}{
		{
			name:    "cordon rejects delisted cluster",
			update:  cordonClusterSpec,
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}},
			wantErr: errClusterDelisted,
		},
		{
			name:    "uncordon rejects incomplete drain",
			update:  uncordonClusterSpec,
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Draining: true}},
			wantErr: errClusterDraining,
		},
		{
			name:   "uncordon accepts completed drain",
			update: uncordonClusterSpec,
			cluster: rlarkv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       rlarkv1alpha1.ClusterSpec{Draining: true, Cordoned: true},
				Status: rlarkv1alpha1.ClusterStatus{Conditions: []metav1.Condition{{
					Type:               rlarkv1alpha1.ConditionTypeProgressing,
					Status:             metav1.ConditionFalse,
					Reason:             "DrainComplete",
					ObservedGeneration: 1,
				}}},
			},
		},
		{
			name:    "drain rejects delisted cluster",
			update:  drainClusterSpec,
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}},
			wantErr: errClusterDelisted,
		},
		{
			name:    "undrain rejects delisted cluster",
			update:  undrainClusterSpec,
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}},
			wantErr: errClusterDelisted,
		},
		{
			name:    "delist rejects incomplete drain",
			update:  delistClusterSpec,
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Draining: true}},
			wantErr: errClusterDraining,
		},
		{
			name:   "delist accepts completed drain",
			update: delistClusterSpec,
			cluster: rlarkv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       rlarkv1alpha1.ClusterSpec{Draining: true, Cordoned: true},
				Status: rlarkv1alpha1.ClusterStatus{Conditions: []metav1.Condition{{
					Type:               rlarkv1alpha1.ConditionTypeProgressing,
					Status:             metav1.ConditionFalse,
					Reason:             "DrainComplete",
					ObservedGeneration: 1,
				}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.update(tt.cluster.DeepCopy())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

func TestClusterLifecycleBusinessErrorCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		path       string
		cluster    *rlarkv1alpha1.Cluster
		wantStatus int
		wantCode   string
	}{
		{
			name:       "drain rejects delisted cluster",
			path:       "/clusters/cluster-a/drain",
			cluster:    &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}},
			wantStatus: http.StatusConflict,
			wantCode:   CodeResourceConflict,
		},
		{
			name:       "undrain rejects delisted cluster",
			path:       "/clusters/cluster-a/undrain",
			cluster:    &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkv1alpha1.ClusterSpec{Delisted: true, Draining: true}},
			wantStatus: http.StatusConflict,
			wantCode:   CodeResourceConflict,
		},
		{
			name:       "delist rejects draining cluster",
			path:       "/clusters/cluster-a/delist",
			cluster:    &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkv1alpha1.ClusterSpec{Draining: true}},
			wantStatus: http.StatusConflict,
			wantCode:   CodeResourceConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(tt.cluster)}
			router := gin.New()
			router.POST("/clusters/:cluster_id/drain", gateway.drainCluster)
			router.POST("/clusters/:cluster_id/undrain", gateway.undrainCluster)
			router.POST("/clusters/:cluster_id/uncordon", gateway.uncordonCluster)
			router.POST("/clusters/:cluster_id/delist", gateway.delistCluster)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, tt.path, nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var response ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != tt.wantCode {
				t.Fatalf("error code = %q, want %q", response.Code, tt.wantCode)
			}
		})
	}
}

func TestClusterLifecycleResponseIncludesClusterNodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-a"},
		Status:     rlarkv1alpha1.ClusterStatus{TotalNodes: 1, OnlineNodes: 1, ReadyNodes: 1},
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-a",
			Namespace: "rlark-agent-a",
			Labels: map[string]string{
				rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
			},
		},
		Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
	}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(cluster, node)}
	router := gin.New()
	router.POST("/clusters/:cluster_id/cordon", gateway.cordonCluster)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/clusters/cluster-a/cordon", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		Data    ClusterInfo `json:"data"`
		Success bool        `json:"success"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode lifecycle response: %v", err)
	}
	if !response.Success || response.Data.TotalNodes != 1 || response.Data.OnlineNodes != 1 || response.Data.CloudNodes != 1 || response.Data.Type != "Cloud" {
		t.Fatalf("unexpected lifecycle response: %#v", response)
	}
}

func TestDelistClusterTransition(t *testing.T) {
	cluster := &rlarkv1alpha1.Cluster{}
	if err := delistClusterSpec(cluster); err != nil {
		t.Fatalf("delistClusterSpec() error = %v", err)
	}
	if !cluster.Spec.Delisted {
		t.Fatal("expected Delisted to be true")
	}
}

func TestDrainClusterTransition(t *testing.T) {
	tests := []struct {
		name         string
		cluster      rlarkv1alpha1.Cluster
		wantErr      string
		wantCordoned bool
		wantPrevious bool
	}{
		{
			name:         "cordons and saves uncordoned state",
			wantCordoned: true,
		},
		{
			name:         "preserves previously cordoned state",
			cluster:      rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Cordoned: true}},
			wantCordoned: true,
			wantPrevious: true,
		},
		{
			name:         "does not overwrite state during repeated drain",
			cluster:      rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Cordoned: true, Draining: true}},
			wantCordoned: true,
		},
		{
			name:    "rejects delisted cluster",
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}},
			wantErr: "delisted cluster must be restored before it can be drained",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := tt.cluster.DeepCopy()
			err := drainClusterSpec(cluster)
			if tt.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr) {
					t.Fatalf("drainClusterSpec() error = %v, want prefix %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("drainClusterSpec() error = %v", err)
			}
			if !cluster.Spec.Draining || cluster.Spec.Cordoned != tt.wantCordoned || cluster.Spec.DrainPreviousCordoned != tt.wantPrevious {
				t.Fatalf("unexpected drain state: %#v", cluster.Spec)
			}
		})
	}
}

func TestUndrainClusterTransition(t *testing.T) {
	cluster := &rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{
		Cordoned:              true,
		Draining:              true,
		DrainPreviousCordoned: false,
	}}
	if err := undrainClusterSpec(cluster); err != nil {
		t.Fatalf("undrainClusterSpec() error = %v", err)
	}
	if cluster.Spec.Draining || cluster.Spec.Cordoned || cluster.Spec.DrainPreviousCordoned {
		t.Fatalf("unexpected undrain state: %#v", cluster.Spec)
	}
}

func TestUncordonClusterTransition(t *testing.T) {
	tests := []struct {
		name    string
		cluster rlarkv1alpha1.Cluster
		wantErr string
	}{
		{
			name:    "clears cordon",
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Cordoned: true}},
		},
		{
			name:    "rejects delisted cluster",
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true, Cordoned: true}},
			wantErr: "delisted cluster must be restored before it can be uncordoned",
		},
		{
			name:    "rejects draining cluster",
			cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Draining: true, Cordoned: true}},
			wantErr: "draining cluster must complete draining before it can be uncordoned",
		},
		{
			name: "cancels completed drain",
			cluster: rlarkv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: rlarkv1alpha1.ClusterSpec{
					Draining:              true,
					Cordoned:              true,
					DrainPreviousCordoned: true,
				},
				Status: rlarkv1alpha1.ClusterStatus{Conditions: []metav1.Condition{{
					Type:               rlarkv1alpha1.ConditionTypeProgressing,
					Status:             metav1.ConditionFalse,
					Reason:             "DrainComplete",
					ObservedGeneration: 3,
				}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := tt.cluster.DeepCopy()
			err := uncordonClusterSpec(cluster)
			if tt.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr) {
					t.Fatalf("uncordonClusterSpec() error = %v, want prefix %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("uncordonClusterSpec() error = %v", err)
			}
			if cluster.Spec.Cordoned {
				t.Fatal("expected Cordoned to be false")
			}
			if cluster.Spec.Draining {
				t.Fatal("expected Draining to be false so the cluster becomes schedulable again")
			}
			if cluster.Spec.DrainPreviousCordoned {
				t.Fatal("expected DrainPreviousCordoned to be cleared with the drain intent")
			}
		})
	}
}

func TestUncordonClusterKeepsDrainStoppedTasksStopped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Generation: 2},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-a", Cordoned: true, Draining: true},
		Status: rlarkv1alpha1.ClusterStatus{Conditions: []metav1.Condition{{
			Type:               rlarkv1alpha1.ConditionTypeProgressing,
			Status:             metav1.ConditionFalse,
			Reason:             "DrainComplete",
			ObservedGeneration: 2,
		}}},
	}
	evicted := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "rlark-agent-a", Annotations: map[string]string{
			rlarkv1alpha1.ClusterStoppedAnnotation:        "true",
			rlarkv1alpha1.ClusterDrainSuspendedAnnotation: "true",
		}},
		Spec: rlarkv1alpha1.TaskSpec{Suspended: true},
	}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(cluster, evicted)}
	router := gin.New()
	router.POST("/clusters/:cluster_id/uncordon", gateway.uncordonCluster)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/clusters/cluster-a/uncordon", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	gotCluster, err := gateway.kubeClient.RlinfV1alpha1().Clusters().Get(context.Background(), "cluster-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotCluster.Spec.Cordoned || gotCluster.Spec.Draining {
		t.Fatalf("uncordon must restore scheduling: %#v", gotCluster.Spec)
	}
	gotTask, err := gateway.kubeClient.RlinfV1alpha1().Tasks("rlark-agent-a").Get(context.Background(), "standalone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !gotTask.Spec.Suspended {
		t.Fatal("uncordon must not resume a Task the cluster drain stopped")
	}
	if gotTask.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" ||
		gotTask.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] != "true" {
		t.Fatalf("uncordon must keep the drain stop markers: %#v", gotTask.Annotations)
	}
}

func TestUndrainClusterReleasesDrainStoppedTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-a", Cordoned: true, Draining: true},
	}
	drained := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "drain-suspended", Namespace: "rlark-agent-a", Annotations: map[string]string{
			rlarkv1alpha1.ClusterStoppedAnnotation:        "true",
			rlarkv1alpha1.ClusterDrainSuspendedAnnotation: "true",
		}},
		Spec: rlarkv1alpha1.TaskSpec{Suspended: true},
	}
	userSuspended := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "user-suspended", Namespace: "rlark-agent-a", Annotations: map[string]string{
			rlarkv1alpha1.ClusterStoppedAnnotation: "true",
		}},
		Spec: rlarkv1alpha1.TaskSpec{Suspended: true},
	}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(cluster, drained, userSuspended)}
	router := gin.New()
	router.POST("/clusters/:cluster_id/undrain", gateway.undrainCluster)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/clusters/cluster-a/undrain", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	gotDrained, err := gateway.kubeClient.RlinfV1alpha1().Tasks("rlark-agent-a").Get(context.Background(), "drain-suspended", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotDrained.Spec.Suspended ||
		gotDrained.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" ||
		gotDrained.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] != "" {
		t.Fatalf("cancelling the drain must resume the Task it suspended: %#v", gotDrained)
	}
	gotUser, err := gateway.kubeClient.RlinfV1alpha1().Tasks("rlark-agent-a").Get(context.Background(), "user-suspended", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !gotUser.Spec.Suspended || gotUser.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" {
		t.Fatalf("cancelling the drain must keep a user suspension: %#v", gotUser)
	}
}

func TestClearClusterDrainStopsForJobReleasesOnlyOwnedTasks(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job-a", Namespace: "tenant-a", UID: types.UID("job-a-uid")},
	}
	owned := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-worker",
			Namespace: "rlark-agent-a",
			Labels:    map[string]string{taskJobLabel: job.Name},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(),
				Kind:       "Job",
				Name:       job.Name,
				UID:        job.UID,
				Controller: ptrBool(true),
			}},
			Annotations: map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true"},
		},
		Spec: rlarkv1alpha1.TaskSpec{Suspended: true},
	}
	otherJobTask := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-b-worker",
			Namespace: "rlark-agent-a",
			Labels:    map[string]string{taskJobLabel: "job-b"},
			Annotations: map[string]string{
				rlarkv1alpha1.ClusterStoppedAnnotation: "true",
			},
		},
	}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(owned, otherJobTask)}

	stopped, err := gateway.jobHasClusterDrainStops(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("expected the Job to report drain-stopped Tasks")
	}
	if err := gateway.clearClusterDrainStopsForJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	gotOwned, err := gateway.kubeClient.RlinfV1alpha1().Tasks("rlark-agent-a").Get(context.Background(), owned.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotOwned.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" || !gotOwned.Spec.Suspended {
		t.Fatalf("owned Task marker not cleared without touching its suspension: %#v", gotOwned)
	}
	gotOther, err := gateway.kubeClient.RlinfV1alpha1().Tasks("rlark-agent-a").Get(context.Background(), otherJobTask.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotOther.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" {
		t.Fatalf("Task owned by another Job was changed: %#v", gotOther.Annotations)
	}
}

func ptrBool(value bool) *bool {
	return &value
}

func TestBuildClusterInfoUsesCityMetadata(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID: "cluster-a",
				"rlark.io/city":              "label-city",
				"rlark.io/location":          "legacy-location",
			},
			Annotations: map[string]string{
				"rlark.io/city":        "annotation-city",
				"rlark.io/ip-location": `{"city":"legacy-ip-city"}`,
			},
		},
	}}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if info.Location != "annotation-city" {
		t.Fatalf("expected annotation city, got %q", info.Location)
	}
}

func TestBuildClusterInfoFallsBackToCityLabel(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-a",
			"rlark.io/city":              "label-city",
		}},
	}}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if info.Location != "label-city" {
		t.Fatalf("expected label city, got %q", info.Location)
	}
}

// 回归：批量编辑使用布尔位 label（rlark.io/node-category-cloud=true），
// 而不是枚举值 label（rlark.io/node-category=cloud）。buildClusterInfo
// 必须识别布尔位，否则 Type 会错判为 Hybrid。
func TestBuildClusterInfoBooleanCategoryLabel(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if info.Type != "Cloud" {
		t.Fatalf("expected Cloud, got %q (cloud=%d embodied=%d robots=%d)",
			info.Type, info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
	if info.CloudNodes != 2 {
		t.Fatalf("expected 2 cloud nodes, got %d", info.CloudNodes)
	}
}

// 分类由管理面写入 Node CR 的 Annotation；Annotation 优先于旧 Label 形式。
func TestBuildClusterInfoBooleanCategoryAnnotation(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					rlarkv1alpha1.LabelClusterID:               "cluster-a",
					rlarkv1alpha1.LabelNodeCategory + "-cloud": "false",
				},
				Annotations: map[string]string{
					rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
				},
			},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if info.Type != "Cloud" {
		t.Fatalf("expected Cloud, got %q (cloud=%d embodied=%d robots=%d)",
			info.Type, info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
}

// GPU / 设备型号同样由管理面 Annotation 提供；旧 rlark.io/model Label 仅作回退。
func TestBuildClusterInfoModelsFromAnnotations(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					rlarkv1alpha1.LabelClusterID:               "cluster-a",
					rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
					"rlark.io/model":                           "legacy-gpu",
				},
				Annotations: map[string]string{
					"rlark.io/gpu-model": "A100",
				},
			},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					rlarkv1alpha1.LabelClusterID:               "cluster-a",
					rlarkv1alpha1.LabelNodeCategory + "-robot": "true",
				},
				Annotations: map[string]string{
					"rlark.io/device-model": "Unitree G1",
				},
			},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if len(info.GPUModels) != 1 || info.GPUModels[0] != "A100" {
		t.Fatalf("expected GPU models from annotation, got %v", info.GPUModels)
	}
	if len(info.RobotModels) != 1 || info.RobotModels[0] != "Unitree G1" {
		t.Fatalf("expected robot models from annotation, got %v", info.RobotModels)
	}
}

// 两种 label 形式同时存在时也要合并识别（去重）
func TestBuildClusterInfoUsesClusterStatus(t *testing.T) {
	cluster := rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-a"},
		Status:     rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterDowngraded},
	}

	info := buildClusterInfo(cluster, nil, 0)
	if info.Phase != rlarkv1alpha1.ClusterDowngraded {
		t.Fatalf("expected Downgraded, got %q", info.Phase)
	}
	if info.AgentNamespace != "rlark-agent-a" {
		t.Fatalf("agent namespace = %q, want %q", info.AgentNamespace, "rlark-agent-a")
	}
}

func TestBuildClusterInfoMixedCategoryLabelForms(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:    "cluster-a",
				rlarkv1alpha1.LabelNodeCategory: "cloud,edge",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-robot": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo(rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}, nodes, 0)
	if info.Type != "Hybrid" {
		t.Fatalf("expected Hybrid, got %q (cloud=%d embodied=%d robots=%d)",
			info.Type, info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
	if info.CloudNodes != 1 || info.EmbodiedNodes != 1 || info.Robots != 1 {
		t.Fatalf("expected 1/1/1, got %d/%d/%d",
			info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
}
