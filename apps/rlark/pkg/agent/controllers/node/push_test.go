package node

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
)

func TestPullReconcileSyncsUnschedulableLabel(t *testing.T) {
	t.Parallel()

	// Pausing placement is RLark-scoped: the reconcile syncs the
	// rlark.io/unschedulable label but must never write the local Kubernetes
	// Node's spec.unschedulable, which would block Pods from other platforms.
	for _, tc := range []struct {
		name              string
		unschedulable     bool
		localLabels       map[string]string
		localCordoned     bool
		wantLabelPresent  bool
		wantUnschedulable bool
	}{
		{
			name:              "pause adds label without cordoning the node",
			unschedulable:     true,
			localLabels:       map[string]string{"example.com/keep": "value"},
			localCordoned:     false,
			wantLabelPresent:  true,
			wantUnschedulable: false,
		},
		{
			name:              "resume removes label and preserves an external cordon",
			unschedulable:     false,
			localLabels:       map[string]string{nodeUnschedulableLabel: "stale"},
			localCordoned:     true,
			wantLabelPresent:  false,
			wantUnschedulable: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}

			management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&rlarkv1alpha1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
				Spec:       rlarkv1alpha1.NodeSpec{Unschedulable: tc.unschedulable},
			}).Build()
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: tc.localLabels},
				Spec:       corev1.NodeSpec{Unschedulable: tc.localCordoned},
			}).Build()
			r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
				LocalKubeClient:  local,
				ManagementClient: management,
			}}}

			if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}}); err != nil {
				t.Fatal(err)
			}

			var got corev1.Node
			if err := local.Get(context.Background(), types.NamespacedName{Name: "node-a"}, &got); err != nil {
				t.Fatal(err)
			}
			_, labelPresent := got.Labels[nodeUnschedulableLabel]
			if labelPresent != tc.wantLabelPresent {
				t.Fatalf("unschedulable label present = %t, want %t", labelPresent, tc.wantLabelPresent)
			}
			if labelPresent && got.Labels[nodeUnschedulableLabel] != "true" {
				t.Fatalf("unschedulable label = %q, want true", got.Labels[nodeUnschedulableLabel])
			}
			if got.Spec.Unschedulable != tc.wantUnschedulable {
				t.Fatalf("local Node unschedulable = %t, want %t", got.Spec.Unschedulable, tc.wantUnschedulable)
			}
		})
	}
}

func TestReconcileListsOnlyPodsOnCurrentNode(t *testing.T) {
	t.Parallel()
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
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"rlark.io/enabled": "true"}}},
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

func TestReconcileRestoresEnabledNodeAfterOffline(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"rlark.io/enabled": "true"}}},
	).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-a", rlarkv1alpha1.NodeOffline),
	).Build()
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
	if node.Labels["rlark.io/enabled"] != "true" {
		t.Fatalf("management Node enabled label = %q, want true", node.Labels["rlark.io/enabled"])
	}
	if node.Status.Phase != rlarkv1alpha1.NodeOffline {
		t.Fatalf("management Node phase = %q, want Offline", node.Status.Phase)
	}
}

func TestReconcilePersistsKubernetesNodeUID(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	nodeUID := types.UID("k8s-node-uid")
	local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: nodeUID, Labels: map[string]string{"rlark.io/enabled": "true"}}},
	).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-a", rlarkv1alpha1.NodeOffline),
	).Build()
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
	if node.Status.KubernetesNodeUID != nodeUID {
		t.Fatalf("management Node kubernetesNodeUID = %q, want %q", node.Status.KubernetesNodeUID, nodeUID)
	}
}

// The Agent only reports the local Kubernetes Node's cordon state. It must never
// write that field, so a node cordoned outside RLark is surfaced instead of hidden.
func TestReconcileReportsKubernetesCordonState(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		unschedulable bool
	}{
		{name: "node cordoned outside RLark", unschedulable: true},
		{name: "node not cordoned", unschedulable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
				pod := obj.(*corev1.Pod)
				if pod.Spec.NodeName == "" {
					return nil
				}
				return []string{pod.Spec.NodeName}
			}).WithObjects(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("k8s-node-uid"), Labels: map[string]string{"rlark.io/enabled": "true"}},
				Spec:       corev1.NodeSpec{Unschedulable: tc.unschedulable},
			}).Build()
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
				managementNode("node-a", rlarkv1alpha1.NodeOffline),
			).Build()
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
			if node.Status.KubernetesCordoned == nil {
				t.Fatal("management Node kubernetesCordoned = nil, want reported value")
			}
			if *node.Status.KubernetesCordoned != tc.unschedulable {
				t.Fatalf("management Node kubernetesCordoned = %t, want %t", *node.Status.KubernetesCordoned, tc.unschedulable)
			}
		})
	}
}

func TestReconcileSkipsUnmanagedNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
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
	err := management.Get(context.Background(), types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}, &node)
	if client.IgnoreNotFound(err) != nil {
		t.Fatalf("management node get error = %v, want not found", err)
	}
}

func TestMergeManagementNodeAnnotations(t *testing.T) {
	managementAnnotations := map[string]string{
		"rlark.io/city":                "深圳市",
		"rlark.io/gpu-model":           "NVIDIA 4090",
		"rlark.io/device-model":        "Unitree G1",
		"rlark.io/node-category-cloud": "true",
		"rlark.io/node-category-robot": "true",
		"other.example/note":           "management-only",
	}
	discoveredAnnotations := map[string]string{
		"rlark.io/agent-note":          "reported",
		"rlark.io/city":                "stale-local-value",
		"rlark.io/node-category-cloud": "false",
		"rlark.io/node-category-robot": "false",
	}
	wantAnnotations := map[string]string{
		"rlark.io/city":                "深圳市",
		"rlark.io/gpu-model":           "NVIDIA 4090",
		"rlark.io/device-model":        "Unitree G1",
		"rlark.io/node-category-cloud": "true",
		"rlark.io/node-category-robot": "true",
		"rlark.io/agent-note":          "reported",
		"other.example/note":           "management-only",
	}
	if got := mergeManagementAnnotations(managementAnnotations, discoveredAnnotations); !reflect.DeepEqual(got, wantAnnotations) {
		t.Fatalf("merged annotations = %#v, want %#v", got, wantAnnotations)
	}
}

func TestCapabilitySnapshotContainsOnlyNodeLocalFacts(t *testing.T) {
	node := &corev1.Node{Status: corev1.NodeStatus{
		NodeInfo: corev1.NodeSystemInfo{ContainerRuntimeVersion: "containerd://1.7.0"},
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:                    resource.MustParse("8"),
			corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
		},
		Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:                    resource.MustParse("7"),
			corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("3"),
		},
	}}
	requested := corev1.ResourceList{
		corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
	}

	snapshot := capabilitySnapshot(node, string(rlarkv1alpha1.AgentTypeKubernetes), requested)
	if snapshot.Runtime.Type != rlarkv1alpha1.AgentTypeKubernetes || snapshot.Runtime.Version != "containerd://1.7.0" {
		t.Fatalf("runtime = %#v", snapshot.Runtime)
	}
	if len(snapshot.Runtime.WorkloadKinds) != 1 || snapshot.Runtime.WorkloadKinds[0] != taskapi.WorkloadTypeRay {
		t.Fatalf("workload kinds = %#v, want %q", snapshot.Runtime.WorkloadKinds, taskapi.WorkloadTypeRay)
	}
	if !snapshot.Network.HostNetwork || snapshot.Network.CrossCluster || snapshot.Network.ServiceExposure {
		t.Fatalf("network = %#v", snapshot.Network)
	}
	if snapshot.Operations.Logs || snapshot.Operations.Exec || snapshot.Operations.Terminal {
		t.Fatalf("operations = %#v", snapshot.Operations)
	}
	if len(snapshot.Storage.VolumeTypes) != 0 || len(snapshot.Storage.StorageClasses) != 0 {
		t.Fatalf("storage = %#v", snapshot.Storage)
	}
	if len(snapshot.Devices) != 1 {
		t.Fatalf("devices = %#v, want one GPU resource", snapshot.Devices)
	}
	device := snapshot.Devices[0]
	if device.Category != "nvidia.com/gpu" || device.Count != 4 || device.Available != 2 {
		t.Fatalf("device = %#v, want nvidia.com/gpu count=4 available=2", device)
	}
}

func TestPullReconcileRecordsDrainRequestsForTasksScheduledToLocalNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	taskUID := types.UID("task-uid")
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
			Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}}},
		},
		&rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: taskUID, Generation: 4}},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "runtime", Annotations: map[string]string{
					kubernetesobjects.ManagementTaskNamespaceAnnotation: "cluster-a",
					kubernetesobjects.ManagementTaskNameAnnotation:      "task-a",
					kubernetesobjects.ManagementTaskUIDAnnotation:       string(taskUID),
				}},
				Spec: corev1.PodSpec{NodeName: "node-a"},
			},
		).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
		ManagementNamespace:   "cluster-a",
	}}}

	for i := 0; i < 2; i++ {
		result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}})
		if err != nil {
			t.Fatal(err)
		}
		if result.RequeueAfter != drainRequeueInterval {
			t.Fatalf("requeue after = %s, want %s while draining", result.RequeueAfter, drainRequeueInterval)
		}
	}

	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != 1 {
		t.Fatalf("drain requests = %#v, want one idempotent request", task.Spec.DrainRequests)
	}
	request := task.Spec.DrainRequests[0]
	if request.OperationID != "drain-1" || request.NodeUID != "local-node-uid" || request.NodeName != "node-a" {
		t.Fatalf("drain request = %#v", request)
	}
}

func TestRecordDrainRequestsEnforcesLimitWithoutBreakingRetries(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	taskUID := types.UID("task-uid")
	requests := make([]rlarkv1alpha1.TaskDrainRequest, 0, maxTaskDrainRequests)
	for len(requests) < maxTaskDrainRequests-1 {
		requests = append(requests, rlarkv1alpha1.TaskDrainRequest{
			OperationID: fmt.Sprintf("drain-other-%d", len(requests)),
			NodeUID:     "other-node-uid",
			NodeName:    "node-b",
		})
	}
	requests = append(requests, rlarkv1alpha1.TaskDrainRequest{
		OperationID: "drain-current",
		NodeUID:     "local-node-uid",
		NodeName:    "node-a",
	})
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&rlarkv1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: taskUID},
			Spec:       rlarkv1alpha1.TaskSpec{DrainRequests: requests},
		},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		WithObjects(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "runtime", Annotations: map[string]string{
				kubernetesobjects.ManagementTaskNamespaceAnnotation: "cluster-a",
				kubernetesobjects.ManagementTaskNameAnnotation:      "task-a",
				kubernetesobjects.ManagementTaskUIDAnnotation:       string(taskUID),
			}},
			Spec: corev1.PodSpec{NodeName: "node-a"},
		}).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
	}}}
	localNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}}

	// Retrying an already-recorded operation remains successful at the limit.
	if err := r.recordDrainRequests(context.Background(), localNode, &rlarkv1alpha1.NodeOperationRequest{
		ID: "drain-current", Type: rlarkv1alpha1.NodeOperationDrain,
	}); err != nil {
		t.Fatalf("idempotent retry at limit: %v", err)
	}

	err := r.recordDrainRequests(context.Background(), localNode, &rlarkv1alpha1.NodeOperationRequest{
		ID: "drain-new", Type: rlarkv1alpha1.NodeOperationDrain,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot exceed") {
		t.Fatalf("new operation at limit error = %v, want drain request limit", err)
	}
	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != maxTaskDrainRequests {
		t.Fatalf("drain requests length = %d, want unchanged %d", len(task.Spec.DrainRequests), maxTaskDrainRequests)
	}
}

func TestPullReconcileSkipsCompletedDrainOperation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	taskUID := types.UID("task-uid")
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
			Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}}},
			Status: rlarkv1alpha1.NodeStatus{DrainOperations: []rlarkv1alpha1.NodeDrainOperationStatus{{
				OperationID: "drain-1",
				Completed:   true,
			}}},
		},
		// Task already has a DrainRequest from the earlier in-flight drain.
		// The completed operation is still in Node.Spec (operator has not
		// undrained yet), so the request must NOT be removed — the Task must
		// stay Stopped because the Node remains cordoned.
		&rlarkv1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: taskUID, Generation: 2},
			Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{{
				OperationID: "drain-1", NodeUID: "local-node-uid", NodeName: "node-a",
			}}},
		},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}},
		).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
		ManagementNamespace:   "cluster-a",
	}}}

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeue after = %s, want none after the drain completed", result.RequeueAfter)
	}

	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	// Completed drain operation remains in Node.Spec → DrainRequest must stay.
	// The Task stays Stopped until the operator removes the Drain operation
	// (undrain), at which point the pull controller drops the request.
	if len(task.Spec.DrainRequests) != 1 {
		t.Fatalf("drain requests = %#v, want retained for completed operation still in Node.Spec", task.Spec.DrainRequests)
	}
}

func TestPullReconcileRemovesDrainRequestsAfterUndrain(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		// Undrain removed every Drain operation from the Node Spec.
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"}},
		&rlarkv1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: types.UID("task-uid")},
			Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{
				{OperationID: "drain-1", NodeUID: "local-node-uid", NodeName: "node-a"},
				// A replaced Node incarnation will never observe this request again.
				{OperationID: "drain-old", NodeUID: "old-node-uid", NodeName: "node-a"},
				// Requests for other Nodes are not owned by this reconciler.
				{OperationID: "drain-2", NodeUID: "other-node-uid", NodeName: "node-b"},
			}},
		},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}},
	).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
		ManagementNamespace:   "cluster-a",
	}}}

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeue after = %s, want none after undrain", result.RequeueAfter)
	}

	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != 1 || task.Spec.DrainRequests[0].OperationID != "drain-2" {
		t.Fatalf("drain requests = %#v, want only the request for node-b", task.Spec.DrainRequests)
	}
}

func TestPullReconcileKeepsDrainRequestWhileOperationRemains(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	request := rlarkv1alpha1.TaskDrainRequest{OperationID: "drain-1", NodeUID: "local-node-uid", NodeName: "node-a"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		// An in-flight Drain (no completed status yet) keeps the Drain op in Spec
		// and the Task must remain stopped for it.
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
			Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}}},
			Status: rlarkv1alpha1.NodeStatus{DrainOperations: []rlarkv1alpha1.NodeDrainOperationStatus{{
				OperationID: "drain-1",
				Completed:   false,
			}}},
		},
		&rlarkv1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: types.UID("task-uid")},
			Spec:       rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{request}},
		},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}},
	).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
		ManagementNamespace:   "cluster-a",
	}}}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}}); err != nil {
		t.Fatal(err)
	}

	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != 1 || task.Spec.DrainRequests[0] != request {
		t.Fatalf("drain requests = %#v, want %#v", task.Spec.DrainRequests, request)
	}
}

func TestPullReconcileReleasesDrainRequestsWhenNodeIsTerminating(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node-a",
				Namespace:         "cluster-a",
				Finalizers:        []string{"test/finalizer"},
				DeletionTimestamp: &now,
			},
			Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}}},
		},
		&rlarkv1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", UID: types.UID("task-uid")},
			Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{{
				OperationID: "drain-1", NodeUID: "local-node-uid", NodeName: "node-a",
			}}},
		},
	).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("local-node-uid")}},
	).Build()
	r := &pullNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient:       local,
		LocalKubeCachedClient: local,
		ManagementClient:      management,
		ManagementNamespace:   "cluster-a",
	}}}

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("requeue after = %s, want none for a terminating Node", result.RequeueAfter)
	}

	var task rlarkv1alpha1.Task
	if err := management.Get(context.Background(), types.NamespacedName{Name: "task-a", Namespace: "cluster-a"}, &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != 0 {
		t.Fatalf("drain requests = %#v, want none for a terminating Node", task.Spec.DrainRequests)
	}
}

func TestNodeDeviceCapabilitiesClampNegativeAvailability(t *testing.T) {
	devices := nodeDeviceCapabilities(
		corev1.ResourceList{corev1.ResourceName("example.com/device"): resource.MustParse("1")},
		corev1.ResourceList{corev1.ResourceName("example.com/device"): resource.MustParse("1")},
		corev1.ResourceList{corev1.ResourceName("example.com/device"): resource.MustParse("2")},
	)
	if len(devices) != 1 || devices[0].Available != 0 {
		t.Fatalf("devices = %#v, want availability clamped to zero", devices)
	}
}

func TestRequestedResourcesForNode(t *testing.T) {
	// Used is scheduler allocation: the sum of non-terminal Pod requests, never
	// a sample of real-time CPU, memory, or device utilization.
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

// TestActiveWorkloadSummaryCountsRLarkManagedPods verifies that the Agent's
// workload summary distinguishes RLark-managed Pods (identified by the RLark
// task-management annotations) from non-RLark Pods. This count feeds the Node
// Controller's drain completion logic: zero RLark-managed Pods means there is
// nothing to drain on this Node.
func TestActiveWorkloadSummaryCountsRLarkManagedPods(t *testing.T) {
	t.Parallel()
	rlarkAnnotations := map[string]string{
		kubernetesobjects.ManagementTaskNameAnnotation:      "my-task",
		kubernetesobjects.ManagementTaskUIDAnnotation:       "task-uid",
		kubernetesobjects.ManagementTaskNamespaceAnnotation: "cluster-a",
	}
	systemAnnotations := map[string]string{
		"k8s-app": "kube-proxy",
	}
	pods := []corev1.Pod{
		// RLark-managed Running Pod
		{
			ObjectMeta: metav1.ObjectMeta{Annotations: rlarkAnnotations},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// RLark-managed Pending Pod
		{
			ObjectMeta: metav1.ObjectMeta{Annotations: rlarkAnnotations},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		// Non-RLark Running Pod (kube-proxy DaemonSet)
		{
			ObjectMeta: metav1.ObjectMeta{Annotations: systemAnnotations},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Non-RLark Running Pod (no annotations)
		{
			ObjectMeta: metav1.ObjectMeta{},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// RLark Pod with only Name annotation (UID missing → not counted)
		{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				kubernetesobjects.ManagementTaskNameAnnotation: "orphan",
			}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Terminal Pod (Succeeded) — excluded from Total
		{
			ObjectMeta: metav1.ObjectMeta{Annotations: rlarkAnnotations},
			Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	}

	got := activeWorkloadSummary(pods)
	if got.Total != 5 {
		t.Fatalf("Total = %d, want 5 (2 RLark proper + 2 non-RLark + 1 RLark-orphan active)", got.Total)
	}
	if got.Running != 4 {
		t.Fatalf("Running = %d, want 4 (orphan + RLark-running + 2 non-RLark)", got.Running)
	}
	if got.Pending != 1 {
		t.Fatalf("Pending = %d, want 1", got.Pending)
	}
	// RLarkManaged counts Pods that carry BOTH Name and UID annotations.
	// The orphan Pod (Name present, UID absent) and the Succeeded Pod both
	// fall outside this window.
	if got.RLarkManaged != 2 {
		t.Fatalf("RLarkManaged = %d, want 2 (one Running + one Pending RLark Pod with full annotations)", got.RLarkManaged)
	}
}

// TestActiveWorkloadSummaryReportsZeroForEmptyNode verifies that a Node with
// zero Pods (and therefore zero RLark workloads) reports both Total=0 and
// RLarkManaged=0. The Node Controller can safely treat this as "confirmed zero
// RLark workloads" because the Agent has scanned and found nothing.
func TestActiveWorkloadSummaryReportsZeroForEmptyNode(t *testing.T) {
	t.Parallel()
	got := activeWorkloadSummary(nil)
	if got.Total != 0 || got.Running != 0 || got.Pending != 0 || got.RLarkManaged != 0 {
		t.Fatalf("empty node summary = %#v, want all zeros", got)
	}
}

// TestPushNodeReconcilerReportsRLarkManagedToManagementNode verifies the full
// push pipeline: local Pods with RLark annotations → Agent scans and counts →
// Management Node Status.ActiveWorkloads.RLarkManaged carries the value up.
// This is evidence that the Agent has actually looked at the node, so the
// Node Controller can distinguish "Agent hasn't scanned" from "Agent scanned
// and found zero RLark workloads".
func TestPushNodeReconcilerReportsRLarkManagedToManagementNode(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	rlarkAnnotations := map[string]string{
		kubernetesobjects.ManagementTaskNameAnnotation:      "train-task",
		kubernetesobjects.ManagementTaskUIDAnnotation:       "uid-1",
		kubernetesobjects.ManagementTaskNamespaceAnnotation: "cluster-a",
	}
	rlarkAnnotations2 := map[string]string{
		kubernetesobjects.ManagementTaskNameAnnotation:      "infer-task",
		kubernetesobjects.ManagementTaskUIDAnnotation:       "uid-2",
		kubernetesobjects.ManagementTaskNamespaceAnnotation: "cluster-a",
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
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("k8s-node-a"), Labels: map[string]string{"rlark.io/enabled": "true"}}},
			// 2 RLark-managed Pods + 1 non-RLark system Pod
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "rlark-pod-1", Namespace: "cluster-a", Annotations: rlarkAnnotations},
				Spec:       corev1.PodSpec{NodeName: "node-a"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "rlark-pod-2", Namespace: "cluster-a", Annotations: rlarkAnnotations2},
				Spec:       corev1.PodSpec{NodeName: "node-a"},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy", Namespace: "kube-system", Annotations: map[string]string{"k8s-app": "kube-proxy"}},
				Spec:       corev1.PodSpec{NodeName: "node-a"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
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
	summary := node.Status.ActiveWorkloads
	if summary.Total != 3 {
		t.Fatalf("ActiveWorkloads.Total = %d, want 3 (2 RLark + 1 non-RLark)", summary.Total)
	}
	if summary.RLarkManaged != 2 {
		t.Fatalf("ActiveWorkloads.RLarkManaged = %d, want 2", summary.RLarkManaged)
	}
	if summary.Running != 2 {
		t.Fatalf("ActiveWorkloads.Running = %d, want 2", summary.Running)
	}
	if summary.Pending != 1 {
		t.Fatalf("ActiveWorkloads.Pending = %d, want 1", summary.Pending)
	}
	// LastHeartbeatTime belongs to the Node Controller, which derives it from the
	// heartbeat Lease through a single clock. The Agent's own scan marker is
	// KubernetesNodeUID, which is what the Drain completion check reads.
	if node.Status.LastHeartbeatTime != nil {
		t.Fatalf("LastHeartbeatTime = %v, want nil: the Agent must not write a control-plane field", node.Status.LastHeartbeatTime)
	}
	if node.Status.KubernetesNodeUID == "" {
		t.Fatal("KubernetesNodeUID = \"\", want populated so the Node Controller knows the Agent has scanned")
	}
}

// TestPushNodeReconcilerReportsZeroRLarkManagedForZeroRLarkPods verifies that
// when a Node has no RLark-managed Pods, the Agent reports RLarkManaged=0.
// Combined with the heartbeat being present, this lets the Node Controller
// safely mark a Drain operation as Completed because the Agent has confirmed
// there is nothing RLark-managed to drain.
func TestPushNodeReconcilerReportsZeroRLarkManagedForZeroRLarkPods(t *testing.T) {
	t.Parallel()
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
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"rlark.io/enabled": "true"}}},
			// Only system DaemonSet Pods — no RLark workload at all
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-proxy", Namespace: "kube-system", Annotations: map[string]string{"k8s-app": "kube-proxy"}},
				Spec:       corev1.PodSpec{NodeName: "node-a"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
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
	summary := node.Status.ActiveWorkloads
	if summary.RLarkManaged != 0 {
		t.Fatalf("ActiveWorkloads.RLarkManaged = %d, want 0 (no RLark Pods on this Node)", summary.RLarkManaged)
	}
	if summary.Total != 1 {
		t.Fatalf("ActiveWorkloads.Total = %d, want 1 (one non-RLark system Pod)", summary.Total)
	}
}
