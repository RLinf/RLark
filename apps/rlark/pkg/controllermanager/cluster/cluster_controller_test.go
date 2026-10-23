package cluster

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestObserve(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	leaseDuration := int32(30)
	freshHeartbeat := metav1.NewMicroTime(now.Add(-5 * time.Second))
	staleHeartbeat := metav1.NewMicroTime(now.Add(-time.Minute))

	tests := []struct {
		name      string
		cluster   rlarkv1alpha1.Cluster
		nodes     []rlarkv1alpha1.Node
		lease     *coordinationv1.Lease
		wantPhase rlarkv1alpha1.ClusterPhase
	}{
		{
			name:      "online when heartbeat is current and all nodes are online",
			cluster:   testCluster(),
			nodes:     []rlarkv1alpha1.Node{testNode("node-1", rlarkv1alpha1.NodeOnline)},
			lease:     testLease(freshHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterOnline,
		},
		{
			name:      "offline when heartbeat is stale",
			cluster:   testCluster(),
			nodes:     []rlarkv1alpha1.Node{testNode("node-1", rlarkv1alpha1.NodeOnline)},
			lease:     testLease(staleHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterOffline,
		},
		{
			name:      "offline when heartbeat has no renewal time",
			cluster:   testCluster(),
			nodes:     []rlarkv1alpha1.Node{testNode("node-1", rlarkv1alpha1.NodeOnline)},
			lease:     testLeaseWithoutRenewal(&leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterOffline,
		},
		{
			name:      "offline when heartbeat is absent",
			cluster:   testCluster(),
			nodes:     []rlarkv1alpha1.Node{testNode("node-1", rlarkv1alpha1.NodeOnline)},
			wantPhase: rlarkv1alpha1.ClusterOffline,
		},
		{
			name:      "downgraded when a registered node is offline",
			cluster:   testCluster(),
			nodes:     []rlarkv1alpha1.Node{testNode("node-1", rlarkv1alpha1.NodeOnline), testNode("node-2", rlarkv1alpha1.NodeOffline)},
			lease:     testLease(freshHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterDowngraded,
		},
		{
			name:      "cordoned overrides observed availability",
			cluster:   clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a", Cordoned: true}),
			lease:     testLease(staleHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterCordoned,
		},
		{
			name:      "draining overrides cordoned",
			cluster:   clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a", Cordoned: true, Draining: true}),
			lease:     testLease(staleHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterDraining,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("add RLark scheme: %v", err)
			}
			if err := coordinationv1.AddToScheme(scheme); err != nil {
				t.Fatalf("add coordination scheme: %v", err)
			}

			objects := []runtime.Object{&tt.cluster}
			for index := range tt.nodes {
				objects = append(objects, &tt.nodes[index])
			}
			if tt.lease != nil {
				objects = append(objects, tt.lease)
			}
			reconciler := Reconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(),
				Now:    func() time.Time { return now },
			}

			phase, _, _, _, _, err := reconciler.observe(context.Background(), &tt.cluster, now)
			if err != nil {
				t.Fatalf("observe cluster: %v", err)
			}
			if phase != tt.wantPhase {
				t.Fatalf("expected phase %q, got %q", tt.wantPhase, phase)
			}
		})
	}
}

func testCluster() rlarkv1alpha1.Cluster {
	return clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a"})
}

func clusterWithSpec(spec rlarkv1alpha1.ClusterSpec) rlarkv1alpha1.Cluster {
	return rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       spec,
	}
}

func testNode(name string, phase rlarkv1alpha1.NodePhase) rlarkv1alpha1.Node {
	return rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "rlark-agent-cluster-a",
		},
		Status: rlarkv1alpha1.NodeStatus{Phase: phase},
	}
}

func testLease(renewTime metav1.MicroTime, leaseDuration *int32) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: heartbeatLeaseName, Namespace: "rlark-agent-cluster-a"},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renewTime,
			LeaseDurationSeconds: leaseDuration,
		},
	}
}

func testLeaseWithoutRenewal(leaseDuration *int32) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: heartbeatLeaseName, Namespace: "rlark-agent-cluster-a"},
		Spec:       coordinationv1.LeaseSpec{LeaseDurationSeconds: leaseDuration},
	}
}

func TestReconcileUpdatesStatus(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	freshHeartbeat := metav1.NewMicroTime(now.Add(-5 * time.Second))
	cluster := testCluster()
	node := testNode("node-1", rlarkv1alpha1.NodeOnline)

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add coordination scheme: %v", err)
	}
	leaseDuration := int32(30)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&cluster, &node, testLease(freshHeartbeat, &leaseDuration)).Build()
	reconciler := Reconciler{Client: client, Now: func() time.Time { return now }}

	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("reconcile cluster: %v", err)
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if updated.Status.Phase != rlarkv1alpha1.ClusterOnline || updated.Status.TotalNodes != 1 || updated.Status.OnlineNodes != 1 {
		t.Fatalf("unexpected status: %#v", updated.Status)
	}
	condition := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "ClusterOnline" {
		t.Fatalf("unexpected Ready condition: %#v", condition)
	}

	result, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name))
	if err != nil {
		t.Fatalf("reconcile unchanged cluster: %v", err)
	}
	if result.RequeueAfter != 25*time.Second {
		t.Fatalf("expected requeue after 25s, got %s", result.RequeueAfter)
	}
	var unchanged rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &unchanged); err != nil {
		t.Fatalf("get unchanged cluster: %v", err)
	}
	if updated.ResourceVersion != unchanged.ResourceVersion {
		t.Fatalf("expected unchanged status resource version, got %q then %q", updated.ResourceVersion, unchanged.ResourceVersion)
	}
}

func TestNextRequeueAfter(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	heartbeat := metav1.NewTime(now.Add(-5 * time.Second))
	if got := nextRequeueAfter(now, &heartbeat, 30*time.Second); got != 25*time.Second {
		t.Fatalf("expected requeue after 25s, got %s", got)
	}
	if got := nextRequeueAfter(now, nil, 0); got != time.Minute {
		t.Fatalf("expected fallback requeue after 1m, got %s", got)
	}
}

func reconcileRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name}}
}
