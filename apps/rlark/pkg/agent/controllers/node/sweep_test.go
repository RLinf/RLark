package node

import (
	"context"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// TestOfflineSweeperDeletesUnmanagedNodes verifies that the sweeper removes
// stale RLark Node CRs whose local Kubernetes Node no longer has the
// rlark.io/enabled=true label. Managed nodes and nodes belonging to other
// clusters are left untouched.
func TestOfflineSweeperDeletesUnmanagedNodes(t *testing.T) {
	scheme := newTestScheme(t)
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		// node-disabled has no enabled label — should trigger a Delete
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-disabled", UID: "node-disabled-uid"}},
		// node-enabled is still managed — should stay
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-enabled",
			UID:    "node-enabled-uid",
			Labels: map[string]string{"rlark.io/enabled": "true"},
		}},
	).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-disabled", rlarkv1alpha1.NodeOnline),
		managementNode("node-enabled", rlarkv1alpha1.NodeOnline),
		// other-cluster carries a different cluster-id label — must NOT be
		// touched even though the sweeper's namespace is "cluster-a".
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "other-cluster", Namespace: "other-namespace", Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-b",
			rlarkv1alpha1.LabelAgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
		}}, Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline}},
	).Build()

	sweeper, now := newTestSweeper(t, management, local)
	sweeper.unmanagedGrace = time.Minute

	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The first observation only records the stale marker; it must not delete.
	assertNodeExistsInNamespace(t, management, "node-disabled", "cluster-a")
	assertNodeStaleSince(t, management, "node-disabled", true)

	advance(now, 2*time.Minute)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	// node-disabled was NOT enabled locally → CR must be deleted.
	assertNodeMissing(t, management, "node-disabled")
	// node-enabled is still managed → CR remains intact.
	assertNodePhase(t, management, "node-enabled", rlarkv1alpha1.NodeOnline)
	// other-cluster lives in a DIFFERENT namespace — must not be touched by
	// this agent's sweep even though no local Node matches it.
	assertNodeExistsInNamespace(t, management, "other-cluster", "other-namespace")
}

// TestOfflineSweeperDeletesMissingNodesAfterConfirmation verifies that a
// management Node CR whose local Kubernetes Node has been removed is deleted
// only after the missing-node confirmation window.
func TestOfflineSweeperDeletesMissingNodesAfterConfirmation(t *testing.T) {
	scheme := newTestScheme(t)
	// Local client is empty — node-missing does not exist.
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-missing", rlarkv1alpha1.NodeOnline),
	).Build()
	sweeper, now := newTestSweeper(t, management, local)

	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A single NotFound observation is not enough: the Node may be re-created.
	assertNodeExistsInNamespace(t, management, "node-missing", "cluster-a")
	assertNodeStaleSince(t, management, "node-missing", true)

	advance(now, nodeMissingConfirmationWindow+time.Second)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNodeMissing(t, management, "node-missing")
}

// TestOfflineSweeperKeepsNodeThatBecameManagedAgain verifies that the pending
// deletion is re-validated and cancelled when the local Node returns in a
// managed state before the confirmation window elapses.
func TestOfflineSweeperKeepsNodeThatBecameManagedAgain(t *testing.T) {
	scheme := newTestScheme(t)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-recovered", rlarkv1alpha1.NodeOnline),
	).Build()
	sweeper, now := newTestSweeper(t, management, local)

	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNodeStaleSince(t, management, "node-recovered", true)

	if err := local.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "node-recovered",
		UID:    "node-recovered-uid",
		Labels: map[string]string{"rlark.io/enabled": "true"},
	}}); err != nil {
		t.Fatal(err)
	}

	advance(now, nodeMissingConfirmationWindow+time.Second)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	assertNodePhase(t, management, "node-recovered", rlarkv1alpha1.NodeOnline)
	assertNodeStaleSince(t, management, "node-recovered", false)
}

// TestOfflineSweeperPreservesForeignFinalizers verifies that the sweeper never
// clears finalizers owned by other controllers. A normal Delete is issued and
// the CR stays in Terminating until its owner cleans up.
func TestOfflineSweeperPreservesForeignFinalizers(t *testing.T) {
	scheme := newTestScheme(t)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgmtNode := managementNode("node-with-finalizer", rlarkv1alpha1.NodeOnline)
	mgmtNode.Finalizers = []string{"sync.rlinf.io/persist"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		mgmtNode,
	).Build()

	sweeper, now := newTestSweeper(t, management, local)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	advance(now, nodeMissingConfirmationWindow+time.Second)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: "node-with-finalizer", Namespace: "cluster-a"}, &got); err != nil {
		t.Fatalf("Node must remain while a foreign finalizer is present: %v", err)
	}
	if !slices.Contains(got.Finalizers, "sync.rlinf.io/persist") {
		t.Fatalf("foreign finalizer was removed: %v", got.Finalizers)
	}
	if got.DeletionTimestamp == nil {
		t.Fatal("sweeper must issue a normal Delete and let the finalizer owner clean up")
	}
}

// TestOfflineSweeperSkipsNodesWithoutAgentReportedUID verifies that CRs this
// Agent has never reported (for example management-created registration intent)
// are not deleted, because ownership cannot be proven.
func TestOfflineSweeperSkipsNodesWithoutAgentReportedUID(t *testing.T) {
	scheme := newTestScheme(t)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	unreported := managementNode("node-unreported", rlarkv1alpha1.NodeOnline)
	unreported.Status.KubernetesNodeUID = ""
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		unreported,
	).Build()

	sweeper, now := newTestSweeper(t, management, local)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	advance(now, nodeMissingConfirmationWindow+time.Second)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	assertNodeExistsInNamespace(t, management, "node-unreported", "cluster-a")
	assertNodeStaleSince(t, management, "node-unreported", false)
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func newTestSweeper(t *testing.T, management, local client.Client) (*OfflineSweeper, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	sweeper := NewOfflineSweeper(&Controller{Controller: base.Controller{
		ManagementClient:    management,
		ManagementNamespace: "cluster-a",
		AgentType:           string(rlarkv1alpha1.AgentTypeKubernetes),
		LocalKubeClient:     local,
	}}, 0)
	sweeper.now = func() time.Time { return now }
	return sweeper, &now
}

func advance(now *time.Time, d time.Duration) {
	*now = now.Add(d)
}

func managementNode(name string, phase rlarkv1alpha1.NodePhase) *rlarkv1alpha1.Node {
	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "cluster-a", Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-a",
			rlarkv1alpha1.LabelAgentType: string(rlarkv1alpha1.AgentTypeKubernetes),
		}},
		Status: rlarkv1alpha1.NodeStatus{
			Phase:             phase,
			KubernetesNodeUID: types.UID(name + "-uid"),
		},
	}
}

func assertNodePhase(t *testing.T, management client.Client, name string, want rlarkv1alpha1.NodePhase) {
	t.Helper()
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "cluster-a"}, &node); err != nil {
		t.Fatalf("get Node %s: %v", name, err)
	}
	if node.Status.Phase != want {
		t.Fatalf("Node %s phase = %s, want %s", name, node.Status.Phase, want)
	}
}

func assertNodeStaleSince(t *testing.T, management client.Client, name string, want bool) {
	t.Helper()
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "cluster-a"}, &node); err != nil {
		t.Fatalf("get Node %s: %v", name, err)
	}
	if _, ok := node.Annotations[nodeStaleSinceAnnotation]; ok != want {
		t.Fatalf("Node %s stale marker present = %t, want %t (annotations=%v)", name, ok, want, node.Annotations)
	}
}

func assertNodeMissing(t *testing.T, management client.Client, name string) {
	t.Helper()
	var node rlarkv1alpha1.Node
	err := management.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "cluster-a"}, &node)
	if err == nil {
		t.Fatalf("Node %s still exists on management cluster (phase=%s, finalizers=%v, deletionTimestamp=%v)",
			name, node.Status.Phase, node.Finalizers, node.DeletionTimestamp)
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected error getting Node %s: %v", name, err)
	}
}

func assertNodeExistsInNamespace(t *testing.T, management client.Client, name, namespace string) {
	t.Helper()
	var node rlarkv1alpha1.Node
	if err := management.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &node); err != nil {
		t.Fatalf("Node %s/%s must not be touched by another agent's sweep: %v", namespace, name, err)
	}
}

// TestOfflineSweeperStartSweepsImmediatelyAndStopsOnCancellation verifies that
// Start runs one sweep before waiting and returns when the context is cancelled.
func TestOfflineSweeperStartSweepsImmediatelyAndStopsOnCancellation(t *testing.T) {
	scheme := newTestScheme(t)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(
		managementNode("node-missing", rlarkv1alpha1.NodeOnline),
	).Build()
	sweeper, _ := newTestSweeper(t, management, local)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sweeper.Start(ctx) }()

	deadline := time.After(2 * time.Second)
	for {
		var node rlarkv1alpha1.Node
		err := management.Get(context.Background(), types.NamespacedName{Name: "node-missing", Namespace: "cluster-a"}, &node)
		if err == nil && node.Annotations[nodeStaleSinceAnnotation] != "" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Start did not run the initial sweep")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}
