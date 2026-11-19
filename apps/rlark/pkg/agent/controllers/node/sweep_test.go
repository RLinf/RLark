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

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestOfflineSweeperDoesNotWriteConnectivityStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-disabled"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-enabled", Labels: map[string]string{"rlark.io/enabled": "true"}}},
	).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-disabled", rlarkv1alpha1.NodeOnline),
		managementNode("node-enabled", rlarkv1alpha1.NodeOnline),
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "other-cluster", Namespace: "cluster-a", Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-b",
			rlarkv1alpha1.LabelAgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
		}}, Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline}},
	).Build()

	sweeper := NewOfflineSweeper(&Controller{Controller: base.Controller{
		ManagementClient:    management,
		ManagementNamespace: "cluster-a",
		AgentType:           string(rlarkv1alpha1.AgentTypeKubernetes),
		LocalKubeClient:     local,
	}}, 0)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	assertNodePhase(t, management, "node-disabled", rlarkv1alpha1.NodeOnline)
	assertNodePhase(t, management, "node-enabled", rlarkv1alpha1.NodeOnline)
	assertNodePhase(t, management, "other-cluster", rlarkv1alpha1.NodeOnline)
}

func TestOfflineSweeperDoesNotWriteMissingNodeConnectivity(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-missing", rlarkv1alpha1.NodeOnline),
	).Build()
	sweeper := NewOfflineSweeper(&Controller{Controller: base.Controller{
		ManagementClient:    management,
		ManagementNamespace: "cluster-a",
		AgentType:           string(rlarkv1alpha1.AgentTypeKubernetes),
		LocalKubeClient:     local,
	}}, 0)

	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNodePhase(t, management, "node-missing", rlarkv1alpha1.NodeOnline)
}

func managementNode(name string, phase rlarkv1alpha1.NodePhase) *rlarkv1alpha1.Node {
	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "cluster-a", Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-a",
			rlarkv1alpha1.LabelAgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
		}},
		Status: rlarkv1alpha1.NodeStatus{Phase: phase},
	}
}

func assertNodePhase(t *testing.T, management client.Client, name string, want rlarkv1alpha1.NodePhase) {
	t.Helper()
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "cluster-a"}, &node); err != nil {
		t.Fatal(err)
	}
	if node.Status.Phase != want {
		t.Fatalf("Node %s phase = %s, want %s", name, node.Status.Phase, want)
	}
}
