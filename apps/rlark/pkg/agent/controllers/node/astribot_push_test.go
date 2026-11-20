package node

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestManagementOwnedNodeLabels(t *testing.T) {
	t.Cleanup(func() { siteOwnedLabels = nil })
	for _, entry := range []string{"", "robot-body", "kubernetes.io/hostname", "node-role.kubernetes.io/", "node.k8s.io/x", "/x"} {
		if err := SetManagementOwnedLabels([]string{entry}); err == nil {
			t.Errorf("entry %q was accepted", entry)
		}
	}
	if err := SetManagementOwnedLabels([]string{"fleet.astribot/robot-body", "site.example/"}); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"rlark.io/node-category": true, "rlark.io/node-category-robot": true,
		"rlark.io/robot-model": true, "rlark.io/robot-body": true, "rlark.io/robot-id": true,
		"fleet.astribot/robot-body": true, "site.example/anything": true,
		"rlark.io/rollout-wave": true, "rlark.io/restart-request": true, "rlark.io/accelerator": true,
		// What a machine reports of its rollout and restarts is its own.
		"rlark.io/rollout-state": false, "rlark.io/node-profile-state": false,
		// What a node says of itself stays the node's.
		"fleet.astribot/robot-body-state": false, "fleet.astribot/orin-state": false, "fleet.astribot/node-profile-state": false,
		"rlark.io/enabled": false, "rlark.io/cluster-id": false, "kubernetes.io/hostname": false,
	} {
		if got := isManagementOwnedNodeLabel(key); got != want {
			t.Errorf("%s: owned by the management Node = %v, want %v", key, got, want)
		}
	}
}

func TestReconcileKeepsTheTenantOfTheManagementNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		return []string{obj.(*corev1.Pod).Spec.NodeName}
	}).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"rlark.io/enabled": "true"}}},
	).Build()
	known := managementNode("node-a", rlarkv1alpha1.NodeOnline)
	// The tenant as the gateway wrote it, and a label the node had once.
	known.Labels = map[string]string{"rlark.io/tenant-id": "default", "example.com/gone": "x"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(known).Build()
	r := &pushNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient: local, ManagementClient: management, ManagementNamespace: "cluster-a", AgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
	}}}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}, &node); err != nil {
		t.Fatal(err)
	}
	if node.Labels["rlark.io/tenant-id"] != "default" || node.Labels["example.com/gone"] != "" || node.Labels["rlark.io/enabled"] != "true" {
		t.Fatalf("labels of the management Node after the report: %v", node.Labels)
	}
}

func TestReconcileKeepsTheDisplayNameOfTheManagementNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		return []string{obj.(*corev1.Pod).Spec.NodeName}
	}).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"rlark.io/enabled": "true"}}, Spec: corev1.NodeSpec{Unschedulable: true}},
	).Build()
	// The operator's name, set through the gateway; the agent type and
	// schedulability are stale and the report refreshes them.
	known := managementNode("node-a", rlarkv1alpha1.NodeOnline)
	known.Spec = rlarkv1alpha1.NodeSpec{DisplayName: "百度GPU-1", AgentType: rlarkv1alpha1.AgentTypeDocker}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(known).Build()
	r := &pushNodeReconciler{c: &Controller{Controller: base.Controller{
		LocalKubeClient: local, ManagementClient: management, ManagementNamespace: "cluster-a", AgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
	}}}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: "node-a", Namespace: "cluster-a"}, &node); err != nil {
		t.Fatal(err)
	}
	if node.Spec.DisplayName != "百度GPU-1" || node.Spec.AgentType != rlarkv1alpha1.AgentTypeKubernetes || !node.Spec.Unschedulable {
		t.Fatalf("spec of the management Node after the report: %+v", node.Spec)
	}
}

// Who set a robot's desired power is the management plane's word, which no cluster reports.
func TestRobotAnnotationsStayTheManagementPlanes(t *testing.T) {
	set := `{"desired":"off","by":"alice"}`
	merged := mergeManagementAnnotations(
		map[string]string{"rlark.io/robot-power-set": set},
		map[string]string{"rlark.io/robot-power-set": "not the cluster's to say"},
	)
	if merged["rlark.io/robot-power-set"] != set {
		t.Fatalf("annotations = %v", merged)
	}
}
