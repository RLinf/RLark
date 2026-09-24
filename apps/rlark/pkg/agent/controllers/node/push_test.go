package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestReconcileListsOnlyPodsOnCurrentNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	local := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default"}, Spec: corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}}}}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "default"}, Spec: corev1.PodSpec{NodeName: "node-b", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")}}}}}},
		).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).Build()
	r := &pushNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:     local,
		ManagementClient:    management,
		ManagementNamespace: "cluster-a",
		AgentType:           string(rlarkv1alpha1.AgentTypeKubernetes),
	}}}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}, &node); err != nil {
		t.Fatal(err)
	}
	if cpu := node.Status.Used.Cpu().String(); cpu != "1" {
		t.Fatalf("reported CPU requests = %s, want 1", cpu)
	}
}

func TestMergeManagementNodeMetadata(t *testing.T) {
	managementLabels := map[string]string{
		"rlark.io/node-category-cloud": "true",
		"admin.example/obsolete":       "keep-out",
	}
	discoveredLabels := map[string]string{
		"kubernetes.io/hostname":       "gpu47",
		"rlark.io/node-category-cloud": "false",
	}
	wantLabels := map[string]string{
		"kubernetes.io/hostname":       "gpu47",
		"rlark.io/node-category-cloud": "true",
	}
	if got := mergeManagementMetadata(managementLabels, discoveredLabels, isManagementOwnedNodeLabel); !reflect.DeepEqual(got, wantLabels) {
		t.Fatalf("merged labels = %#v, want %#v", got, wantLabels)
	}

	managementAnnotations := map[string]string{
		"rlark.io/city":         "深圳市",
		"rlark.io/gpu-model":    "NVIDIA 4090",
		"rlark.io/device-model": "Unitree G1",
		"other.example/note":    "management-only",
	}
	discoveredAnnotations := map[string]string{
		"rlark.io/agent-note": "reported",
		"rlark.io/city":       "stale-local-value",
	}
	wantAnnotations := map[string]string{
		"rlark.io/city":         "深圳市",
		"rlark.io/gpu-model":    "NVIDIA 4090",
		"rlark.io/device-model": "Unitree G1",
		"rlark.io/agent-note":   "reported",
		"other.example/note":    "management-only",
	}
	if got := mergeManagementAnnotations(managementAnnotations, discoveredAnnotations); !reflect.DeepEqual(got, wantAnnotations) {
		t.Fatalf("merged annotations = %#v, want %#v", got, wantAnnotations)
	}
}

func TestRequestedResourcesForNode(t *testing.T) {
	pods := []corev1.Pod{
		{
			Spec: corev1.PodSpec{
				NodeName: "gpu20",
				Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("1500m"),
					corev1.ResourceMemory:                 resource.MustParse("2Gi"),
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				}}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Spec: corev1.PodSpec{
				NodeName: "gpu20",
				Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				}}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			Spec: corev1.PodSpec{NodeName: "gpu20", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("8"),
			}}}}},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	}

	got := requestedResourcesForNode("gpu20", pods)
	if cpu := got.Cpu().String(); cpu != "2" {
		t.Fatalf("cpu requests = %s, want 2", cpu)
	}
	if memory := got.Memory().String(); memory != "2Gi" {
		t.Fatalf("memory requests = %s, want 2Gi", memory)
	}
	if gpu := got[corev1.ResourceName("nvidia.com/gpu")]; gpu.String() != "1" {
		t.Fatalf("gpu requests = %s, want 1", gpu.String())
	}
}

func TestNodeStorageStatus(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		want       [3]int64
		wantNil    bool
		wantErr    bool
	}{
		{
			name:       "node filesystem",
			statusCode: http.StatusOK,
			response:   `{"node":{"fs":{"capacityBytes":1000,"usedBytes":900,"availableBytes":100}}}`,
			want:       [3]int64{1000, 900, 100},
		},
		{
			name:       "derives used bytes",
			statusCode: http.StatusOK,
			response:   `{"node":{"fs":{"capacityBytes":1000,"availableBytes":250}}}`,
			want:       [3]int64{1000, 750, 250},
		},
		{
			name:       "deduplicates shared image filesystem",
			statusCode: http.StatusOK,
			response:   `{"node":{"fs":{"capacityBytes":1000,"usedBytes":600,"availableBytes":400},"runtime":{"imageFs":{"capacityBytes":1000,"usedBytes":300,"availableBytes":400}}}}`,
			want:       [3]int64{1000, 600, 400},
		},
		{
			name:       "aggregates dedicated image filesystem",
			statusCode: http.StatusOK,
			response:   `{"node":{"fs":{"capacityBytes":1000,"usedBytes":600,"availableBytes":400},"runtime":{"imageFs":{"capacityBytes":500,"usedBytes":450,"availableBytes":50}}}}`,
			want:       [3]int64{1500, 1050, 450},
		},
		{name: "missing filesystem", statusCode: http.StatusOK, response: `{"node":{}}`, wantNil: true},
		{name: "invalid response", statusCode: http.StatusOK, response: `{`, wantErr: true},
		{name: "http error", statusCode: http.StatusServiceUnavailable, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/api/v1/nodes/gpu20/proxy/stats/summary" {
					t.Fatalf("request path = %s", req.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			reconciler := &pushNodeReconciler{c: &Controller{Controller: base.Controller{
				LocalKubeHTTP:    server.Client(),
				LocalKubeAPIHost: server.URL,
			}}}
			got, err := reconciler.nodeStorageStatus(context.Background(), "gpu20")
			if (err != nil) != tt.wantErr {
				t.Fatalf("nodeStorageStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("nodeStorageStatus() = %#v, want nil", got)
				}
				return
			}
			if got == nil || [3]int64{got.CapacityBytes, got.UsedBytes, got.AvailableBytes} != tt.want {
				t.Fatalf("nodeStorageStatus() = %#v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiskPressure(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.NodeCondition
		want       *bool
	}{
		{name: "not reported", want: nil},
		{
			name: "unknown",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown,
			}},
			want: nil,
		},
		{
			name: "normal",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse,
			}},
			want: boolPointer(false),
		},
		{
			name: "pressure",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue,
			}},
			want: boolPointer(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diskPressure(&corev1.Node{Status: corev1.NodeStatus{Conditions: tt.conditions}})
			if tt.want == nil {
				if got != nil {
					t.Fatalf("diskPressure() = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("diskPressure() = %v, want %v", got, *tt.want)
			}
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
}
