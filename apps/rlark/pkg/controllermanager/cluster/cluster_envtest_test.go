package cluster

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	rlarkenvtest "github.com/rlinf/rlark/apps/rlark/pkg/testutil/envtest"
)

// TestReconcileObservedGenerationAgainstAPIServer asserts the ObservedGeneration
// contract against a real API server: only the API server bumps
// metadata.generation on a Spec change, so a fake client cannot show that a
// stale status is detectable. This is the envtest acceptance case for the
// Cluster status contract.
func TestReconcileObservedGenerationAgainstAPIServer(t *testing.T) {
	cfg := rlarkenvtest.Start(t)
	scheme := rlarkenvtest.Scheme(t)

	ctx := context.Background()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	const namespace = "rlark-agent-cluster-a"
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}); err != nil {
		t.Fatalf("create agent namespace: %v", err)
	}

	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: namespace},
	}
	if err := c.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Namespace: namespace}}
	if err := c.Create(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	node.Status.Phase = rlarkv1alpha1.NodeOnline
	if err := c.Status().Update(ctx, node); err != nil {
		t.Fatalf("update node status: %v", err)
	}

	now := time.Now()
	leaseDuration := int32(30)
	renewTime := metav1.NewMicroTime(now.Add(-5 * time.Second))
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: heartbeatLeaseName, Namespace: namespace},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renewTime,
			LeaseDurationSeconds: &leaseDuration,
		},
	}
	if err := c.Create(ctx, lease); err != nil {
		t.Fatalf("create heartbeat lease: %v", err)
	}

	reconciler := Reconciler{Client: c, Now: func() time.Time { return now }}
	if _, err := reconciler.Reconcile(ctx, reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("reconcile cluster: %v", err)
	}

	observed := getCluster(ctx, t, c, cluster.Name)
	firstGeneration := observed.Generation
	if observed.Status.ObservedGeneration != firstGeneration {
		t.Fatalf("expected observedGeneration %d, got %d", firstGeneration, observed.Status.ObservedGeneration)
	}
	if observed.Status.Phase != rlarkv1alpha1.ClusterOnline {
		t.Fatalf("expected phase %s, got %s", rlarkv1alpha1.ClusterOnline, observed.Status.Phase)
	}
	if observed.Status.LastStatusChangeTime == nil {
		t.Fatal("expected lastStatusChangeTime to be set on the first phase transition")
	}
	firstChangeTime := observed.Status.LastStatusChangeTime.Time

	// Flipping a Spec switch bumps metadata.generation, so the status written by
	// the previous reconciliation is now detectably stale.
	observed.Spec.Cordoned = true
	if err := c.Update(ctx, observed); err != nil {
		t.Fatalf("cordon cluster: %v", err)
	}
	if observed.Generation == firstGeneration {
		t.Fatalf("expected the API server to bump generation past %d", firstGeneration)
	}
	stale := getCluster(ctx, t, c, cluster.Name)
	if stale.Status.ObservedGeneration == stale.Generation {
		t.Fatal("expected status to be detectably stale before the next reconciliation")
	}

	now = now.Add(time.Minute)
	if _, err := reconciler.Reconcile(ctx, reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("reconcile cordoned cluster: %v", err)
	}

	cordoned := getCluster(ctx, t, c, cluster.Name)
	if cordoned.Status.ObservedGeneration != cordoned.Generation {
		t.Fatalf("expected observedGeneration %d, got %d", cordoned.Generation, cordoned.Status.ObservedGeneration)
	}
	if cordoned.Status.Phase != rlarkv1alpha1.ClusterCordoned {
		t.Fatalf("expected phase %s, got %s", rlarkv1alpha1.ClusterCordoned, cordoned.Status.Phase)
	}
	if cordoned.Status.LastStatusChangeTime == nil || !cordoned.Status.LastStatusChangeTime.After(firstChangeTime) {
		t.Fatalf("expected lastStatusChangeTime to advance past %s, got %#v", firstChangeTime, cordoned.Status.LastStatusChangeTime)
	}
}

func getCluster(ctx context.Context, t *testing.T, c client.Client, name string) *rlarkv1alpha1.Cluster {
	t.Helper()

	var cluster rlarkv1alpha1.Cluster
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &cluster); err != nil {
		t.Fatalf("get cluster %s: %v", name, err)
	}
	return &cluster
}
