package cluster

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			name:      "delisted overrides observed availability",
			cluster:   clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a", Delisted: true}),
			lease:     testLease(staleHeartbeat, &leaseDuration),
			wantPhase: rlarkv1alpha1.ClusterDelisted,
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

func TestReconcileReportsAgentNamespaceConflict(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	clusterA := clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "shared-agent"})
	clusterB := clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "shared-agent"})
	clusterB.Name = "cluster-b"

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&clusterA, &clusterB).Build()
	reconciler := Reconciler{Client: client, Now: func() time.Time { return now }}

	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(clusterA.Name)); err != nil {
		t.Fatalf("initialize Cluster finalizer: %v", err)
	}
	result, err := reconciler.Reconcile(context.Background(), reconcileRequest(clusterA.Name))
	if err != nil {
		t.Fatalf("reconcile conflict: %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Fatalf("requeue after = %s, want %s", result.RequeueAfter, time.Minute)
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: clusterA.Name}, &updated); err != nil {
		t.Fatalf("get Cluster: %v", err)
	}
	if updated.Status.Phase != rlarkv1alpha1.ClusterOffline {
		t.Fatalf("phase = %q, want %q", updated.Status.Phase, rlarkv1alpha1.ClusterOffline)
	}
	validated := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeValidated)
	if validated == nil || validated.Status != metav1.ConditionFalse || validated.Reason != "AgentNamespaceConflict" || validated.ObservedGeneration != updated.Generation {
		t.Fatalf("unexpected Validated condition: %#v", validated)
	}
	ready := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AgentNamespaceConflict" {
		t.Fatalf("unexpected Ready condition: %#v", ready)
	}
}

func TestReconcileMarksClusterValidatedWithoutAgentNamespaceConflict(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	cluster := testCluster()

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add coordination scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&cluster).Build()
	reconciler := Reconciler{Client: client, Now: func() time.Time { return now }}

	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("initialize Cluster finalizer: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("reconcile Cluster: %v", err)
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); err != nil {
		t.Fatalf("get Cluster: %v", err)
	}
	validated := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeValidated)
	if validated == nil || validated.Status != metav1.ConditionTrue || validated.Reason != "SpecValidated" || validated.ObservedGeneration != updated.Generation {
		t.Fatalf("unexpected Validated condition: %#v", validated)
	}
}

func TestReconcileRecoversConditionsAfterAgentNamespaceConflictClears(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	leaseDuration := int32(30)
	freshHeartbeat := metav1.NewMicroTime(now.Add(-5 * time.Second))
	clusterA := clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "shared-agent"})
	clusterB := clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "shared-agent"})
	clusterB.Name = "cluster-b"
	node := testNode("node-1", rlarkv1alpha1.NodeOnline)
	node.Namespace = clusterA.Spec.AgentNamespace
	lease := testLease(freshHeartbeat, &leaseDuration)
	lease.Namespace = clusterA.Spec.AgentNamespace

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add coordination scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&clusterA, &clusterB, &node, lease).Build()
	reconciler := Reconciler{Client: client, Now: func() time.Time { return now }}

	for range 2 {
		if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(clusterA.Name)); err != nil {
			t.Fatalf("reconcile conflicting Cluster: %v", err)
		}
	}

	if err := client.Delete(context.Background(), &clusterB); err != nil {
		t.Fatalf("delete conflicting Cluster: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(clusterA.Name)); err != nil {
		t.Fatalf("reconcile recovered Cluster: %v", err)
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: clusterA.Name}, &updated); err != nil {
		t.Fatalf("get Cluster: %v", err)
	}
	validated := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeValidated)
	if validated == nil || validated.Status != metav1.ConditionTrue || validated.Reason != "SpecValidated" || validated.ObservedGeneration != updated.Generation {
		t.Fatalf("unexpected Validated condition: %#v", validated)
	}
	ready := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != "ClusterOnline" {
		t.Fatalf("unexpected Ready condition: %#v", ready)
	}
	if updated.Status.Phase != rlarkv1alpha1.ClusterOnline {
		t.Fatalf("phase = %q, want %q", updated.Status.Phase, rlarkv1alpha1.ClusterOnline)
	}
}

func TestReconcileFinishesDrainAndRestoresObservedPhase(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	leaseDuration := int32(30)
	staleHeartbeat := metav1.NewMicroTime(now.Add(-time.Minute))
	cluster := clusterWithSpec(rlarkv1alpha1.ClusterSpec{
		AgentNamespace:        "rlark-agent-cluster-a",
		Cordoned:              true,
		Draining:              true,
		DrainPreviousCordoned: false,
	})

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add coordination scheme: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&cluster, testLease(staleHeartbeat, &leaseDuration)).Build()
	reconciler := Reconciler{Client: client, Now: func() time.Time { return now }}

	for range 2 {
		if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
			t.Fatalf("reconcile completed drain: %v", err)
		}
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if !updated.Spec.Draining || !updated.Spec.Cordoned || updated.Spec.DrainPreviousCordoned {
		t.Fatalf("expected drain request to remain in Spec, got %#v", updated.Spec)
	}
	if updated.Status.Phase != rlarkv1alpha1.ClusterDraining {
		t.Fatalf("expected phase %q while drain remains requested, got %q", rlarkv1alpha1.ClusterDraining, updated.Status.Phase)
	}
	condition := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if condition == nil || condition.Reason != "ClusterDraining" {
		t.Fatalf("unexpected Ready condition: %#v", condition)
	}
}

func TestReconcileReportsDrainTimeout(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	cluster := clusterWithSpec(rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a", Draining: true})
	task := drainTestTask("task-1", cluster.Spec.AgentNamespace)
	task.Annotations = map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true"}

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&cluster, task).Build()
	startedAt := now.Add(-drainTimeout)
	reconciler := Reconciler{Client: client, Now: func() time.Time { return startedAt }}

	if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("initialize Cluster finalizer: %v", err)
	}
	result, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name))
	if err != nil {
		t.Fatalf("start drain: %v", err)
	}
	if result.RequeueAfter != drainInitialBackoff {
		t.Fatalf("requeue after = %s, want %s", result.RequeueAfter, drainInitialBackoff)
	}

	reconciler.Now = func() time.Time { return now }
	result, err = reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name))
	if err != nil {
		t.Fatalf("report drain timeout: %v", err)
	}
	if result.RequeueAfter != drainMaxBackoff {
		t.Fatalf("requeue after = %s, want %s", result.RequeueAfter, drainMaxBackoff)
	}

	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); err != nil {
		t.Fatalf("get Cluster: %v", err)
	}
	condition := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeProgressing)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "DrainTimedOut" || condition.ObservedGeneration != updated.Generation {
		t.Fatalf("unexpected Progressing condition: %#v", condition)
	}
}

func TestReconcileResumesTimedOutDrainAfterRestart(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	cluster := clusterWithSpec(rlarkv1alpha1.ClusterSpec{
		AgentNamespace:        "rlark-agent-cluster-a",
		Cordoned:              true,
		Draining:              true,
		DrainPreviousCordoned: false,
	})
	task := drainTestTask("task-1", cluster.Spec.AgentNamespace)

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Cluster{}).WithRuntimeObjects(&cluster, task).Build()
	first := Reconciler{Client: client, Now: func() time.Time { return now }}

	for range 2 {
		if _, err := first.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
			t.Fatalf("start drain: %v", err)
		}
	}

	restarted := Reconciler{Client: client, Now: func() time.Time { return now.Add(drainTimeout) }}
	result, err := restarted.Reconcile(context.Background(), reconcileRequest(cluster.Name))
	if err != nil {
		t.Fatalf("resume drain after restart: %v", err)
	}
	if result.RequeueAfter != drainMaxBackoff {
		t.Fatalf("requeue after = %s, want %s", result.RequeueAfter, drainMaxBackoff)
	}

	var updatedTask rlarkv1alpha1.Task
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: task.Namespace, Name: task.Name}, &updatedTask); err != nil {
		t.Fatalf("get Task: %v", err)
	}
	updatedTask.Status.Phase = rlarkv1alpha1.TaskPhaseStopped
	updatedTask.Status.ObservedGeneration = updatedTask.Generation
	if err := client.Update(context.Background(), &updatedTask); err != nil {
		t.Fatalf("update stopped Task: %v", err)
	}

	if _, err := restarted.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
		t.Fatalf("finish drain after restart: %v", err)
	}
	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); err != nil {
		t.Fatalf("get Cluster: %v", err)
	}
	if !updated.Spec.Draining || !updated.Spec.Cordoned || updated.Spec.DrainPreviousCordoned {
		t.Fatalf("expected drain request to remain in Spec, got %#v", updated.Spec)
	}
	condition := apimeta.FindStatusCondition(updated.Status.Conditions, rlarkv1alpha1.ConditionTypeProgressing)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "DrainComplete" {
		t.Fatalf("unexpected Progressing condition: %#v", condition)
	}
}

func TestNextDrainRequeueAfter(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	startedAt := metav1.NewTime(now)
	if got := nextDrainRequeueAfter(now, &startedAt); got != drainInitialBackoff {
		t.Fatalf("initial drain requeue = %s, want %s", got, drainInitialBackoff)
	}
	startedAt = metav1.NewTime(now.Add(-2 * time.Second))
	if got := nextDrainRequeueAfter(now, &startedAt); got != 4*time.Second {
		t.Fatalf("exponential drain requeue = %s, want %s", got, 4*time.Second)
	}
	startedAt = metav1.NewTime(now.Add(-drainTimeout))
	if got := nextDrainRequeueAfter(now, &startedAt); got != drainMaxBackoff {
		t.Fatalf("timed out drain requeue = %s, want %s", got, drainMaxBackoff)
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

	for range 2 {
		if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
			t.Fatalf("reconcile cluster: %v", err)
		}
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
	const expectedRequeueAfter = 25 * time.Second
	if result.RequeueAfter != expectedRequeueAfter {
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

func TestReconcileDeletionRemovesFinalizerAfterResourcesAreGone(t *testing.T) {
	deletedAt := metav1.Now()
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cluster-a",
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{CleanupFinalizer},
		},
		Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "rlark-agent-cluster-a"},
	}
	node := testNode("node-1", rlarkv1alpha1.NodeOnline)
	node.Namespace = cluster.Spec.AgentNamespace
	task := drainTestTask("task-1", "rlark-agent-cluster-a")
	leaseDuration := int32(30)
	lease := testLease(metav1.NewMicroTime(time.Now()), &leaseDuration)
	lease.Namespace = cluster.Spec.AgentNamespace

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, &node, task, lease).Build()
	reconciler := Reconciler{Client: client}

	result, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name))
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != cleanupRequeue {
		t.Fatalf("requeue after = %s, want %s", result.RequeueAfter, cleanupRequeue)
	}

	for range 2 {
		if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
			t.Fatal(err)
		}
	}
	var updated rlarkv1alpha1.Cluster
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &updated); !apierrors.IsNotFound(err) {
		t.Fatalf("Cluster should be deleted after cleanup, got: %v", err)
	}
}

func TestReconcileDeletionRemovesResourcesWithStaleClusterLabels(t *testing.T) {
	deletedAt := metav1.Now()
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cluster-a",
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{CleanupFinalizer},
		},
		Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "shared-agent"},
	}
	node := testNode("node-b", rlarkv1alpha1.NodeOnline)
	node.Namespace = cluster.Spec.AgentNamespace
	node.Labels = map[string]string{rlarkv1alpha1.LabelClusterID: "cluster-b"}
	task := drainTestTask("task-b", cluster.Spec.AgentNamespace)
	task.Labels = map[string]string{rlarkv1alpha1.LabelClusterID: "cluster-b"}
	leaseDuration := int32(30)
	lease := testLease(metav1.NewMicroTime(time.Now()), &leaseDuration)
	lease.Namespace = cluster.Spec.AgentNamespace
	lease.Labels = map[string]string{rlarkv1alpha1.LabelClusterID: "cluster-b"}

	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, &node, task, lease).Build()
	reconciler := Reconciler{Client: client}

	for range 3 {
		if _, err := reconciler.Reconcile(context.Background(), reconcileRequest(cluster.Name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: node.Namespace, Name: node.Name}, &rlarkv1alpha1.Node{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Node with stale label should be deleted, got: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: task.Namespace, Name: task.Name}, &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Task with stale label should be deleted, got: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: lease.Namespace, Name: lease.Name}, &coordinationv1.Lease{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Lease with stale label should be deleted, got: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name}, &rlarkv1alpha1.Cluster{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Cluster should be deleted after namespace cleanup, got: %v", err)
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
