package node

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestReconcileDerivesReadyFromLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	ready := true
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady: &ready,
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeReady {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeReady)
	}
	readyCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue || readyCondition.Reason != rlarkv1alpha1.ConditionReasonKubernetesNodeReady {
		t.Fatalf("Ready condition = %#v", readyCondition)
	}
	agentCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if agentCondition == nil || agentCondition.Status != metav1.ConditionTrue || agentCondition.Reason != rlarkv1alpha1.ConditionReasonLeaseValid {
		t.Fatalf("AgentConnected condition = %#v", agentCondition)
	}
	for _, conditionType := range []string{
		rlarkv1alpha1.ConditionTypeRuntimeReady,
		rlarkv1alpha1.ConditionTypeNetworkReady,
		rlarkv1alpha1.ConditionTypeStorageReady,
		rlarkv1alpha1.ConditionTypeDeviceReady,
		rlarkv1alpha1.ConditionTypeResourcePressure,
		rlarkv1alpha1.ConditionTypeDraining,
	} {
		if conditionByType(got.Status.Conditions, conditionType) == nil {
			t.Fatalf("missing condition %q", conditionType)
		}
	}
}

func TestReconcileRecoversAfterControllerRestart(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status: rlarkv1alpha1.NodeStatus{
			Phase:           rlarkv1alpha1.NodeOffline,
			KubernetesReady: &ready,
			Conditions: []metav1.Condition{{
				Type:               rlarkv1alpha1.ConditionTypeAgentConnected,
				Status:             metav1.ConditionFalse,
				Reason:             rlarkv1alpha1.ConditionReasonLeaseExpired,
				LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
			}},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew, HolderIdentity: stringPtr("connection-2")},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()

	// A newly constructed reconciler represents a controller-manager restart.
	restarted := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}
	if _, err := restarted.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if got.Status.Phase != rlarkv1alpha1.NodeReady || condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != rlarkv1alpha1.ConditionReasonLeaseValid {
		t.Fatalf("node after controller restart = %#v", got.Status)
	}
}

func TestReconcileRecoversAfterLeaseIsRecreated(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a", UID: types.UID("stable-node-uid")},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var offline rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &offline); err != nil {
		t.Fatal(err)
	}
	if offline.Status.Phase != rlarkv1alpha1.NodeOffline {
		t.Fatalf("phase after missing Lease = %q, want %q", offline.Status.Phase, rlarkv1alpha1.NodeOffline)
	}

	renew := metav1.NewMicroTime(now)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew, HolderIdentity: stringPtr("connection-2")},
	}
	if err := client.Create(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var restored rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &restored); err != nil {
		t.Fatal(err)
	}
	condition := conditionByType(restored.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if restored.UID != types.UID("stable-node-uid") || restored.Status.Phase != rlarkv1alpha1.NodeReady || condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("node after Lease recreation = %#v", restored)
	}
}

func TestReconcileUsesCurrentLeaseWhenAgentsCompete(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	firstRenew := metav1.NewMicroTime(now.Add(-2 * time.Minute))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a", UID: types.UID("stable-node-uid")},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: stringPtr("agent-1-instance"),
			RenewTime:      &firstRenew,
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var competingLease coordinationv1.Lease
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: nodeHeartbeatLeaseName("node-a")}, &competingLease); err != nil {
		t.Fatal(err)
	}
	secondRenew := metav1.NewMicroTime(now)
	competingLease.Spec.HolderIdentity = stringPtr("agent-2-instance")
	competingLease.Spec.RenewTime = &secondRenew
	if err := client.Update(context.Background(), &competingLease); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if got.UID != types.UID("stable-node-uid") || got.Status.Phase != rlarkv1alpha1.NodeReady || condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != rlarkv1alpha1.ConditionReasonLeaseValid {
		t.Fatalf("node after competing Agent renewal = %#v", got)
	}
}

func TestReconcileKeepsConnectionDuringLeaseGracePeriod(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-61 * time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{
		Client:           client,
		Now:              func() time.Time { return now },
		LeaseTimeout:     time.Minute,
		LeaseGracePeriod: 15 * time.Second,
	}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if got.Status.Phase != rlarkv1alpha1.NodeReady || condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != rlarkv1alpha1.ConditionReasonLeaseValid {
		t.Fatalf("node after grace-period reconcile = %#v", got.Status)
	}
}

func TestReconcileMarksExpiredLeaseOfflineAfterGracePeriod(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-76 * time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{
		Client:           client,
		Now:              func() time.Time { return now },
		LeaseTimeout:     time.Minute,
		LeaseGracePeriod: 15 * time.Second,
	}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if got.Status.Phase != rlarkv1alpha1.NodeOffline || condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != rlarkv1alpha1.ConditionReasonLeaseExpired {
		t.Fatalf("node after expired Lease reconcile = %#v", got.Status)
	}
}

func TestReconcileRestoresExistingNodeAfterLeaseRenewal(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a", UID: types.UID("stable-node-uid")},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}
	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	renew := metav1.NewMicroTime(now)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew, HolderIdentity: stringPtr("connection-2")},
	}
	if err := client.Create(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.UID != types.UID("stable-node-uid") || got.Status.Phase != rlarkv1alpha1.NodeReady {
		t.Fatalf("node identity or recovery status = %#v", got)
	}
}

func TestSetNodeConditions(t *testing.T) {
	t.Parallel()
	ready := true
	notReady := false
	pressure := true
	cordoned := true
	notCordoned := false
	storage := &rlarkv1alpha1.NodeStorageStatus{CapacityBytes: 100, AvailableBytes: 50}

	tests := []struct {
		name      string
		node      *rlarkv1alpha1.Node
		connected bool
		reason    string
		want      map[string]struct {
			status metav1.ConditionStatus
			reason string
		}
	}{
		{
			name:      "heartbeat never received",
			node:      &rlarkv1alpha1.Node{},
			connected: false,
			reason:    rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeAgentConnected:   {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived},
				rlarkv1alpha1.ConditionTypeReady:            {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived},
				rlarkv1alpha1.ConditionTypeRuntimeReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonRuntimeUnavailable},
				rlarkv1alpha1.ConditionTypeNetworkReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonNetworkUnavailable},
				rlarkv1alpha1.ConditionTypeStorageReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonStorageUnavailable},
				rlarkv1alpha1.ConditionTypeDeviceReady:      {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonDeviceUnavailable},
				rlarkv1alpha1.ConditionTypeResourcePressure: {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonResourcePressureUnknown},
				rlarkv1alpha1.ConditionTypeDraining:         {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoDrainRequest},
				rlarkv1alpha1.ConditionTypeSchedulable:      {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived},
			},
		},
		{
			name:      "connected but kubernetes readiness unobserved",
			node:      &rlarkv1alpha1.Node{},
			connected: true,
			reason:    rlarkv1alpha1.ConditionReasonLeaseValid,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeAgentConnected:   {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonLeaseValid},
				rlarkv1alpha1.ConditionTypeReady:            {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeReadyUnobserved},
				rlarkv1alpha1.ConditionTypeRuntimeReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonRuntimeUnavailable},
				rlarkv1alpha1.ConditionTypeNetworkReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonNetworkUnavailable},
				rlarkv1alpha1.ConditionTypeStorageReady:     {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonStorageUnavailable},
				rlarkv1alpha1.ConditionTypeDeviceReady:      {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonDeviceUnavailable},
				rlarkv1alpha1.ConditionTypeResourcePressure: {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonResourcePressureUnknown},
				rlarkv1alpha1.ConditionTypeDraining:         {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoDrainRequest},
				rlarkv1alpha1.ConditionTypeDegraded:         {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoResourcePressure},
				rlarkv1alpha1.ConditionTypeSchedulable:      {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeReadyUnobserved},
				rlarkv1alpha1.ConditionTypeKubernetesCordoned: {
					metav1.ConditionUnknown,
					rlarkv1alpha1.ConditionReasonKubernetesCordonUnobserved,
				},
			},
		},
		{
			name: "kubernetes node not ready",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{
				KubernetesReady: &notReady,
				Storage:         storage,
				DiskPressure:    &pressure,
			}},
			connected: true,
			reason:    rlarkv1alpha1.ConditionReasonLeaseValid,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeAgentConnected:   {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonLeaseValid},
				rlarkv1alpha1.ConditionTypeReady:            {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady},
				rlarkv1alpha1.ConditionTypeRuntimeReady:     {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonRuntimeUnavailable},
				rlarkv1alpha1.ConditionTypeNetworkReady:     {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNetworkUnavailable},
				rlarkv1alpha1.ConditionTypeStorageReady:     {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonStorageUnavailable},
				rlarkv1alpha1.ConditionTypeDeviceReady:      {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonDeviceUnavailable},
				rlarkv1alpha1.ConditionTypeResourcePressure: {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonResourcePressureDetected},
				rlarkv1alpha1.ConditionTypeDraining:         {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoDrainRequest},
				rlarkv1alpha1.ConditionTypeSchedulable:      {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady},
			},
		},
		{
			name: "drain requested",
			node: &rlarkv1alpha1.Node{
				Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{ID: "drain-1", Type: rlarkv1alpha1.NodeOperationDrain}}},
				Status: rlarkv1alpha1.NodeStatus{
					KubernetesReady: &ready,
					Storage:         storage,
				},
			},
			connected: true,
			reason:    rlarkv1alpha1.ConditionReasonLeaseValid,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeAgentConnected:   {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonLeaseValid},
				rlarkv1alpha1.ConditionTypeReady:            {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonDrainRequested},
				rlarkv1alpha1.ConditionTypeRuntimeReady:     {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonRuntimeAvailable},
				rlarkv1alpha1.ConditionTypeNetworkReady:     {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonNetworkAvailable},
				rlarkv1alpha1.ConditionTypeStorageReady:     {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonStorageAvailable},
				rlarkv1alpha1.ConditionTypeDeviceReady:      {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonDeviceAvailable},
				rlarkv1alpha1.ConditionTypeResourcePressure: {metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonResourcePressureUnknown},
				rlarkv1alpha1.ConditionTypeDraining:         {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonDrainRequested},
				rlarkv1alpha1.ConditionTypeSchedulable:      {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonDrainRequested},
			},
		},
		{
			name: "kubernetes node cordoned outside RLark",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{
				KubernetesReady:    &ready,
				KubernetesCordoned: &cordoned,
				Storage:            storage,
			}},
			connected: true,
			reason:    rlarkv1alpha1.ConditionReasonLeaseValid,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeKubernetesCordoned: {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesCordoned},
				rlarkv1alpha1.ConditionTypeReady:              {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesNodeReady},
				rlarkv1alpha1.ConditionTypeDegraded:           {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoResourcePressure},
				rlarkv1alpha1.ConditionTypeSchedulable:        {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesCordoned},
			},
		},
		{
			name: "kubernetes node not cordoned",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{
				KubernetesReady:    &ready,
				KubernetesCordoned: &notCordoned,
				Storage:            storage,
			}},
			connected: true,
			reason:    rlarkv1alpha1.ConditionReasonLeaseValid,
			want: map[string]struct {
				status metav1.ConditionStatus
				reason string
			}{
				rlarkv1alpha1.ConditionTypeKubernetesCordoned: {metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNotCordoned},
				rlarkv1alpha1.ConditionTypeSchedulable:        {metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonSchedulable},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
			setNodeConditions(tt.node, tt.connected, tt.reason, now)
			for conditionType, expected := range tt.want {
				condition := conditionByType(tt.node.Status.Conditions, conditionType)
				if condition == nil || condition.Status != expected.status || condition.Reason != expected.reason {
					t.Errorf("%s = %#v, want status=%s reason=%s", conditionType, condition, expected.status, expected.reason)
					continue
				}
				if !condition.LastTransitionTime.Time.Equal(now) {
					t.Errorf("%s lastTransitionTime = %v, want %v", conditionType, condition.LastTransitionTime, now)
				}
			}
		})
	}
}

func TestNodeSchedulableCondition(t *testing.T) {
	t.Parallel()
	connected := metav1.Condition{
		Type:   rlarkv1alpha1.ConditionTypeAgentConnected,
		Status: metav1.ConditionTrue,
		Reason: rlarkv1alpha1.ConditionReasonLeaseValid,
	}
	ready := metav1.Condition{
		Type:   rlarkv1alpha1.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: rlarkv1alpha1.ConditionReasonKubernetesNodeReady,
	}
	noPressure := metav1.Condition{
		Type:   rlarkv1alpha1.ConditionTypeResourcePressure,
		Status: metav1.ConditionFalse,
		Reason: rlarkv1alpha1.ConditionReasonNoResourcePressure,
	}
	tests := []struct {
		name       string
		node       *rlarkv1alpha1.Node
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "healthy node",
			node:       &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{connected, ready, noPressure}}},
			wantStatus: metav1.ConditionTrue,
			wantReason: rlarkv1alpha1.ConditionReasonSchedulable,
		},
		{
			name:       "spec blocks placement before any condition",
			node:       &rlarkv1alpha1.Node{Spec: rlarkv1alpha1.NodeSpec{Unschedulable: true}, Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{connected, ready, noPressure}}},
			wantStatus: metav1.ConditionFalse,
			wantReason: rlarkv1alpha1.ConditionReasonUnschedulable,
		},
		{
			name: "disconnected reuses the connectivity reason",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
				{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionFalse, Reason: rlarkv1alpha1.ConditionReasonLeaseExpired},
				ready,
				noPressure,
			}}},
			wantStatus: metav1.ConditionFalse,
			wantReason: rlarkv1alpha1.ConditionReasonLeaseExpired,
		},
		{
			name: "not ready reuses the readiness reason",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
				connected,
				{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady},
				noPressure,
			}}},
			wantStatus: metav1.ConditionFalse,
			wantReason: rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady,
		},
		{
			name: "resource pressure reuses the pressure reason",
			node: &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
				connected,
				ready,
				{Type: rlarkv1alpha1.ConditionTypeResourcePressure, Status: metav1.ConditionTrue, Reason: rlarkv1alpha1.ConditionReasonResourcePressureDetected},
			}}},
			wantStatus: metav1.ConditionFalse,
			wantReason: rlarkv1alpha1.ConditionReasonResourcePressureDetected,
		},
		{
			name:       "unknown pressure preserves the existing scheduler behavior",
			node:       &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{connected, ready, {Type: rlarkv1alpha1.ConditionTypeResourcePressure, Status: metav1.ConditionUnknown}}}},
			wantStatus: metav1.ConditionTrue,
			wantReason: rlarkv1alpha1.ConditionReasonSchedulable,
		},
		{
			name:       "missing prerequisite is fail-closed",
			node:       &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{connected}}},
			wantStatus: metav1.ConditionFalse,
			wantReason: rlarkv1alpha1.ConditionReasonNotSchedulable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason := nodeSchedulableCondition(tt.node)
			if status != tt.wantStatus || reason != tt.wantReason {
				t.Fatalf("nodeSchedulableCondition() = (%s, %s), want (%s, %s)", status, reason, tt.wantStatus, tt.wantReason)
			}
		})
	}
}

func TestDrainingCondition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		node   *rlarkv1alpha1.Node
		active bool
		status metav1.ConditionStatus
		reason string
	}{
		{
			name:   "no request",
			node:   &rlarkv1alpha1.Node{},
			active: false,
			status: metav1.ConditionFalse,
			reason: rlarkv1alpha1.ConditionReasonNoDrainRequest,
		},
		{
			name:   "blocked request",
			node:   &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{DrainOperations: []rlarkv1alpha1.NodeDrainOperationStatus{{OperationID: "drain", Blocked: 1}}}},
			active: true,
			status: metav1.ConditionTrue,
			reason: rlarkv1alpha1.ConditionReasonDrainBlocked,
		},
		{
			name:   "completed request",
			node:   &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{DrainOperations: []rlarkv1alpha1.NodeDrainOperationStatus{{OperationID: "drain", Completed: true}}}},
			active: true,
			status: metav1.ConditionFalse,
			reason: rlarkv1alpha1.ConditionReasonDrainComplete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason := drainingCondition(tt.node, tt.active)
			if status != tt.status || reason != tt.reason {
				t.Fatalf("drainingCondition() = (%s, %s), want (%s, %s)", status, reason, tt.status, tt.reason)
			}
		})
	}
}

func TestNodeConditionPreservesTransitionTimeWhenReasonIsUnchanged(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	previous := metav1.NewTime(now.Add(-time.Minute))
	node := &rlarkv1alpha1.Node{Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{{
		Type:               rlarkv1alpha1.ConditionTypeAgentConnected,
		Status:             metav1.ConditionTrue,
		Reason:             rlarkv1alpha1.ConditionReasonLeaseValid,
		LastTransitionTime: previous,
	}}}}

	setNodeConditions(node, true, rlarkv1alpha1.ConditionReasonLeaseValid, now)
	condition := conditionByType(node.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if condition == nil || !condition.LastTransitionTime.Equal(&previous) {
		t.Fatalf("AgentConnected transition time = %#v, want %v", condition, previous)
	}
}

func TestReconcileOperationControlsLifecyclePhase(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec:       rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{ID: "drain-1", Type: rlarkv1alpha1.NodeOperationDrain}}},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeDraining {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeDraining)
	}
}

func TestReconcileConvergesDegradedInOnePass(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	pressure := true
	renew := metav1.NewMicroTime(now)
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady: &ready,
			DiskPressure:    &pressure,
			Storage:         &rlarkv1alpha1.NodeStorageStatus{CapacityBytes: 100, AvailableBytes: 50},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeDegraded {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeDegraded)
	}
	condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeDegraded)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != rlarkv1alpha1.ConditionReasonResourcePressureDetected {
		t.Fatalf("Degraded condition = %#v", condition)
	}
	readyCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %#v, want True for a Node under resource pressure", readyCondition)
	}
}

// TestDegradedDoesNotBlockReadiness locks the contract that resource pressure
// degrades a Node without failing readiness, while Kubernetes Node NotReady
// still fails it. Placement excludes pressure separately in the job controller.
func TestDegradedDoesNotBlockReadiness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	renew := metav1.NewMicroTime(now)
	ready := true
	notReady := false
	pressure := true

	tests := []struct {
		name            string
		kubernetesReady *bool
		diskPressure    *bool
		wantReady       metav1.ConditionStatus
		wantReadyReason string
		wantPressure    metav1.ConditionStatus
		wantStorage     metav1.ConditionStatus
	}{
		{
			name:            "kubernetes node not ready",
			kubernetesReady: &notReady,
			diskPressure:    &notReady,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady,
			wantPressure:    metav1.ConditionFalse,
			wantStorage:     metav1.ConditionTrue,
		},
		{
			name:            "disk pressure keeps readiness",
			kubernetesReady: &ready,
			diskPressure:    &pressure,
			wantReady:       metav1.ConditionTrue,
			wantReadyReason: rlarkv1alpha1.ConditionReasonKubernetesNodeReady,
			wantPressure:    metav1.ConditionTrue,
			wantStorage:     metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := coordinationv1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			node := &rlarkv1alpha1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
				Status: rlarkv1alpha1.NodeStatus{
					KubernetesReady: tt.kubernetesReady,
					DiskPressure:    tt.diskPressure,
					Storage:         &rlarkv1alpha1.NodeStorageStatus{CapacityBytes: 100, AvailableBytes: 50},
				},
			}
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
				Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
			r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

			if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
				t.Fatal(err)
			}
			var got rlarkv1alpha1.Node
			if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
				t.Fatal(err)
			}
			degraded := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeDegraded)
			if degraded == nil || degraded.Status != metav1.ConditionTrue {
				t.Fatalf("Degraded condition = %#v, want True", degraded)
			}
			readyCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
			if readyCondition == nil || readyCondition.Status != tt.wantReady || readyCondition.Reason != tt.wantReadyReason {
				t.Fatalf("Ready condition = %#v, want status %s reason %s", readyCondition, tt.wantReady, tt.wantReadyReason)
			}
			pressureCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeResourcePressure)
			if pressureCondition == nil || pressureCondition.Status != tt.wantPressure {
				t.Fatalf("ResourcePressure condition = %#v, want status %s", pressureCondition, tt.wantPressure)
			}
			storageCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeStorageReady)
			if storageCondition == nil || storageCondition.Status != tt.wantStorage {
				t.Fatalf("StorageReady condition = %#v, want status %s", storageCondition, tt.wantStorage)
			}
			if got.Status.Phase != rlarkv1alpha1.NodeDegraded {
				t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeDegraded)
			}
		})
	}
}

func TestReconcileCordonedDoesNotChangeReadiness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now)
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec:       rlarkv1alpha1.NodeSpec{Unschedulable: true},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeCordoned {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeCordoned)
	}
	for _, conditionType := range []string{rlarkv1alpha1.ConditionTypeAgentConnected, rlarkv1alpha1.ConditionTypeReady} {
		condition := conditionByType(got.Status.Conditions, conditionType)
		if condition == nil || condition.Status != metav1.ConditionTrue {
			t.Fatalf("%s condition = %#v, want True", conditionType, condition)
		}
	}
}

func TestNodePhaseOperationPriority(t *testing.T) {
	t.Parallel()
	node := &rlarkv1alpha1.Node{Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{
		{ID: "maintenance", Type: rlarkv1alpha1.NodeOperationEnterMaintenance},
		{ID: "drain", Type: rlarkv1alpha1.NodeOperationDrain},
	}}}
	if phase := nodePhase(node, true); phase != rlarkv1alpha1.NodeDraining {
		t.Fatalf("phase = %q, want %q", phase, rlarkv1alpha1.NodeDraining)
	}
}

func TestReconcilePreservesAgentObservations(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now)
	heartbeat := metav1.NewTime(now.Add(-time.Minute))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			LastHeartbeatTime: &heartbeat,
			ActiveWorkloads:   rlarkv1alpha1.NodeWorkloadSummary{Total: 3, Running: 2, Pending: 1},
			CapabilitySnapshot: rlarkv1alpha1.NodeCapabilitySnapshot{
				Network: rlarkv1alpha1.NodeNetworkCapability{CrossCluster: true},
			},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.ActiveWorkloads != node.Status.ActiveWorkloads || !reflect.DeepEqual(got.Status.CapabilitySnapshot, node.Status.CapabilitySnapshot) {
		t.Fatalf("agent observations changed: %#v", got.Status)
	}
	// LastHeartbeatTime is not an agent observation: this controller owns it
	// and republishes the renewal the Lease recorded, so the stale value the
	// agent left behind is replaced rather than preserved.
	if got.Status.LastHeartbeatTime == nil || !got.Status.LastHeartbeatTime.Equal(&metav1.Time{Time: renew.Time}) {
		t.Fatalf("expected the Lease renewal to be published, got %v", got.Status.LastHeartbeatTime)
	}
}

func TestReconcileReportsDrainCompletionWithoutWritingNodeSpec(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}},
		},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", Generation: 3},
		Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{{
			OperationID: "drain-1",
			NodeUID:     "node-uid",
			NodeName:    "node-a",
		}}},
		Status: rlarkv1alpha1.TaskStatus{DrainRequests: []rlarkv1alpha1.TaskDrainStatus{{
			OperationID:        "drain-1",
			NodeUID:            "node-uid",
			ObservedGeneration: 3,
			State:              rlarkv1alpha1.TaskDrainStateWaitingForReschedule,
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease, task).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Unschedulable {
		t.Fatal("controller must not clear spec.unschedulable")
	}
	if len(got.Status.DrainOperations) != 1 || !got.Status.DrainOperations[0].Completed {
		t.Fatalf("drain operations = %#v, want completed drain-1", got.Status.DrainOperations)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeCordoned {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeCordoned)
	}
	if condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady); condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != rlarkv1alpha1.ConditionReasonKubernetesNodeReady {
		t.Fatalf("Ready condition = %#v, want healthy after drain completion", condition)
	}
	if condition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeDraining); condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != rlarkv1alpha1.ConditionReasonDrainComplete {
		t.Fatalf("Draining condition = %#v, want DrainComplete", condition)
	}
}

// TestReconcileRetiresCompletedDrainOperation locks the retirement of a finished
// Drain: the operation leaves Spec so the Node stops reporting Draining, but the
// Node keeps its cordon and the drained Task stays suspended behind a stop
// marker, so releasing the DrainRequests cannot restart it.
func TestReconcileRetiresCompletedDrainOperation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}},
		},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", Generation: 3},
		Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{{
			OperationID: "drain-1",
			NodeUID:     "node-uid",
			NodeName:    "node-a",
		}}},
		Status: rlarkv1alpha1.TaskStatus{DrainRequests: []rlarkv1alpha1.TaskDrainStatus{{
			OperationID:        "drain-1",
			NodeUID:            "node-uid",
			ObservedGeneration: 3,
			State:              rlarkv1alpha1.TaskDrainStateWaitingForReschedule,
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease, task).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	// The first pass marks the drained Task, the second retires the operation
	// once that marker is visible, and the third clears its status entry.
	for range 3 {
		if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
			t.Fatal(err)
		}
	}

	var gotNode rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &gotNode); err != nil {
		t.Fatal(err)
	}
	if len(gotNode.Spec.Operations) != 0 {
		t.Fatalf("spec.operations = %#v, want the completed drain retired", gotNode.Spec.Operations)
	}
	if !gotNode.Spec.Unschedulable {
		t.Fatal("retiring a completed drain must not uncordon the Node")
	}
	if len(gotNode.Status.DrainOperations) != 0 {
		t.Fatalf("status.drainOperations = %#v, want none after retirement", gotNode.Status.DrainOperations)
	}

	var gotTask rlarkv1alpha1.Task
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "task-a"}, &gotTask); err != nil {
		t.Fatal(err)
	}
	if gotTask.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] != "node-a" {
		t.Fatalf("node drain marker = %q, want node-a", gotTask.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation])
	}
	if gotTask.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] != "node-a" {
		t.Fatalf("node drain suspension marker = %q, want node-a", gotTask.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation])
	}
	if !gotTask.Spec.Suspended {
		t.Fatal("drained Task must stay suspended after its Drain operation is retired")
	}
	if len(gotTask.Spec.DrainRequests) != 0 {
		t.Fatalf("drain requests = %#v, want released", gotTask.Spec.DrainRequests)
	}
}

// TestReconcileRetiresCompletedDrainWithoutDrainedTasks covers a Drain that the
// Node completed when no Task carried a request for it (the Agent scanned the
// Node and found nothing to evict). Nothing has to stay stopped, so the
// operation is simply retired.
func TestReconcileRetiresCompletedDrainWithoutDrainedTasks(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}},
		},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
			DrainOperations: []rlarkv1alpha1.NodeDrainOperationStatus{{
				OperationID: "drain-1",
				Completed:   true,
			}},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	for range 2 {
		if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
			t.Fatal(err)
		}
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Operations) != 0 {
		t.Fatalf("spec.operations = %#v, want the completed drain retired", got.Spec.Operations)
	}
	if !got.Spec.Unschedulable {
		t.Fatal("retiring a completed drain must not uncordon the Node")
	}
	if len(got.Status.DrainOperations) != 0 {
		t.Fatalf("status.drainOperations = %#v, want none after retirement", got.Status.DrainOperations)
	}
}

func TestReconcileWaitsForAgentNodeIdentityBeforeCompletingDrain(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
			ID:   "drain-1",
			Type: rlarkv1alpha1.NodeOperationDrain,
		}}},
		Status: rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.DrainOperations) != 1 || got.Status.DrainOperations[0].Completed {
		t.Fatalf("drain operations = %#v, want pending until the Agent reports its Node UID", got.Status.DrainOperations)
	}
}

// TestReconcileDoesNotMarkDrainCompletedWhenAgentHasNotDispatchedRequests locks
// the regression described in the code review: a Drain operation must NOT be
// marked Completed=true merely because no Task has a matching DrainRequest yet.
// The Agent may not have scanned the node yet.
//
// Scenario: Node has RLark-managed workloads (ActiveWorkloads.RLarkManaged > 0),
// Agent has already reported heartbeat, but no Task has been issued a
// DrainRequest yet. Completed must stay false.
func TestReconcileDoesNotMarkDrainCompletedWhenAgentHasNotDispatchedRequests(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	heartbeat := metav1.NewTime(now.Add(-30 * time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
			ID:   "drain-1",
			Type: rlarkv1alpha1.NodeOperationDrain,
		}}},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
			LastHeartbeatTime: &heartbeat,
			ActiveWorkloads: rlarkv1alpha1.NodeWorkloadSummary{
				Total:        3,
				Running:      3,
				RLarkManaged: 2, // Agent sees RLark workloads but hasn't dispatched Drain yet
			},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	// No Task with DrainRequest — simulates "Drain just requested, Agent hasn't acted yet".
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.DrainOperations) != 1 {
		t.Fatalf("drain operations count = %d, want 1", len(got.Status.DrainOperations))
	}
	op := got.Status.DrainOperations[0]
	if op.Completed {
		t.Fatalf("Completed = true, want false — Agent has RLark workloads but hasn't dispatched DrainRequests yet (ActiveWorkloads.RLarkManaged=%d)", node.Status.ActiveWorkloads.RLarkManaged)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeDraining {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeDraining)
	}
}

// TestReconcileMarksDrainCompletedWhenAgentConfirmedZeroRLarkWorkloads verifies
// that a Drain operation is correctly marked Completed when the Agent has
// scanned the node and confirmed there are zero RLark-managed Pods.
func TestReconcileMarksDrainCompletedWhenAgentConfirmedZeroRLarkWorkloads(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	heartbeat := metav1.NewTime(now.Add(-30 * time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []rlarkv1alpha1.NodeOperationRequest{{
				ID:   "drain-1",
				Type: rlarkv1alpha1.NodeOperationDrain,
			}}},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
			LastHeartbeatTime: &heartbeat,
			ActiveWorkloads: rlarkv1alpha1.NodeWorkloadSummary{
				Total:        1, // one system Pod (DaemonSet)
				Running:      1,
				RLarkManaged: 0, // Agent confirmed zero RLark workloads
			},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	// No Task with DrainRequest — correct because there's nothing RLark-managed to drain.
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.DrainOperations) != 1 || !got.Status.DrainOperations[0].Completed {
		t.Fatalf("drain operations = %#v, want Completed=true since Agent confirmed zero RLark workloads", got.Status.DrainOperations)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeCordoned {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeCordoned)
	}
}

// TestReconcileKeepsDrainPendingWhenBlockedTasksExist verifies that the original
// semantics hold: once DrainRequests are dispatched, Completed follows the
// Blocked/Waiting counter. Blocked > 0 means not completed.
func TestReconcileKeepsDrainPendingWhenBlockedTasksExist(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	ready := true
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	heartbeat := metav1.NewTime(now.Add(-30 * time.Second))
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
			ID:   "drain-1",
			Type: rlarkv1alpha1.NodeOperationDrain,
		}}},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			KubernetesNodeUID: types.UID("node-uid"),
			LastHeartbeatTime: &heartbeat,
			ActiveWorkloads: rlarkv1alpha1.NodeWorkloadSummary{
				Total:        3,
				Running:      2,
				Stopping:     1,
				RLarkManaged: 3,
			},
		},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	// One Task is still Blocked (agent hasn't observed the DrainRequest yet or reported Blocked).
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a", Generation: 3},
		Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{{
			OperationID: "drain-1",
			NodeUID:     "node-uid",
			NodeName:    "node-a",
		}}},
		Status: rlarkv1alpha1.TaskStatus{DrainRequests: []rlarkv1alpha1.TaskDrainStatus{{
			OperationID:        "drain-1",
			NodeUID:            "node-uid",
			ObservedGeneration: 3,
			State:              rlarkv1alpha1.TaskDrainStateBlocked,
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease, task).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.DrainOperations) != 1 {
		t.Fatalf("drain operations count = %d, want 1", len(got.Status.DrainOperations))
	}
	op := got.Status.DrainOperations[0]
	if op.Completed {
		t.Fatalf("Completed = true, want false when Blocked > 0 (Blocked=%d)", op.Blocked)
	}
	if op.Blocked != 1 {
		t.Fatalf("Blocked = %d, want 1", op.Blocked)
	}
}

func TestReconcileAddsDrainCleanupFinalizer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Finalizers, nodeDrainCleanupFinalizer) {
		t.Fatalf("finalizers = %v, want %q", got.Finalizers, nodeDrainCleanupFinalizer)
	}
}

func TestReconcileReleasesDrainRequestsAndRemovesFinalizerOnDeletion(t *testing.T) {
	t.Parallel()
	now := metav1.Now()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "node-a",
			Namespace:         "cluster-a",
			DeletionTimestamp: &now,
			Finalizers:        []string{"example.com/other", nodeDrainCleanupFinalizer},
		},
		Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{
			ID:   "drain-1",
			Type: rlarkv1alpha1.NodeOperationDrain,
		}}},
	}
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "cluster-a"},
		Spec: rlarkv1alpha1.TaskSpec{DrainRequests: []rlarkv1alpha1.TaskDrainRequest{
			{OperationID: "drain-1", NodeUID: "node-uid", NodeName: "node-a"},
			{OperationID: "drain-2", NodeUID: "other-uid", NodeName: "node-b"},
		}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, task).Build()
	r := &Reconciler{Client: client, Now: time.Now, LeaseTimeout: time.Minute}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Task
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "task-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.DrainRequests) != 1 || got.Spec.DrainRequests[0].OperationID != "drain-2" {
		t.Fatalf("drain requests = %#v, want only the request for node-b", got.Spec.DrainRequests)
	}

	var terminating rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &terminating); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(terminating.Finalizers, nodeDrainCleanupFinalizer) {
		t.Fatalf("finalizers = %v, want the drain cleanup finalizer removed", terminating.Finalizers)
	}
	if !slices.Contains(terminating.Finalizers, "example.com/other") {
		t.Fatalf("finalizers = %v, want unrelated finalizers preserved", terminating.Finalizers)
	}
}

// TestReconcileRequeuesAtLeaseDeadline pins the deadline this controller wakes
// up on: a renewal seen halfway through the validity window must not buy
// another whole window of silence, otherwise a Node that dies right after
// renewing stays Ready for almost twice its timeout.
func TestReconcileRequeuesAtLeaseDeadline(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	renew := metav1.NewMicroTime(now.Add(-5 * time.Second))
	duration := int32(30)
	ready := true
	scheme := newNodeTestScheme(t)
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew, LeaseDurationSeconds: &duration},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }}

	result, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a"))
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 25*time.Second {
		t.Fatalf("RequeueAfter = %s, want the 25s left on this Lease", result.RequeueAfter)
	}
}

// TestReconcileConditionsCarryNoObservedGeneration pins the freshness contract:
// these conditions are derived from observed state, not from the Spec, so
// stamping them with the Node's Generation would advertise a generation
// contract this controller does not honour.
func TestReconcileConditionsCarryNoObservedGeneration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	renew := metav1.NewMicroTime(now.Add(-time.Second))
	ready := true
	scheme := newNodeTestScheme(t)
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a", Generation: 7},
		Spec:       rlarkv1alpha1.NodeSpec{Unschedulable: true},
		Status:     rlarkv1alpha1.NodeStatus{KubernetesReady: &ready},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeHeartbeatLeaseName("node-a"), Namespace: "cluster-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node, lease).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Generation == 0 {
		t.Fatal("expected the Node to carry a generation so the assertion below is meaningful")
	}
	if len(got.Status.Conditions) == 0 {
		t.Fatal("expected conditions to be written")
	}
	for _, condition := range got.Status.Conditions {
		if condition.ObservedGeneration != 0 {
			t.Fatalf("condition %q claims generation %d; observation-driven conditions must leave it unset",
				condition.Type, condition.ObservedGeneration)
		}
	}
}

// TestReconcileKeepsRecordedHeartbeatWhenLeaseIsGone pins that the published
// renewal only ever moves forward. A deleted Lease means the Agent is gone, not
// that it was never there, so erasing the last known renewal would destroy the
// only evidence of when contact was lost.
func TestReconcileKeepsRecordedHeartbeatWhenLeaseIsGone(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	recorded := metav1.NewTime(now.Add(-10 * time.Minute))
	ready := true
	scheme := newNodeTestScheme(t)
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "cluster-a"},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesReady:   &ready,
			Phase:             rlarkv1alpha1.NodeReady,
			LastHeartbeatTime: &recorded,
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Node{}).WithObjects(node).Build()
	r := &Reconciler{Client: client, Now: func() time.Time { return now }}

	if _, err := r.Reconcile(context.Background(), requestFor("cluster-a", "node-a")); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "cluster-a", Name: "node-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.LastHeartbeatTime == nil || !got.Status.LastHeartbeatTime.Time.Equal(recorded.Time) {
		t.Fatalf("LastHeartbeatTime = %v, want the recorded renewal %v", got.Status.LastHeartbeatTime, recorded.Time)
	}
	if got.Status.Phase != rlarkv1alpha1.NodeOffline {
		t.Fatalf("phase = %q, want %q", got.Status.Phase, rlarkv1alpha1.NodeOffline)
	}
	agentCondition := conditionByType(got.Status.Conditions, rlarkv1alpha1.ConditionTypeAgentConnected)
	if agentCondition == nil || agentCondition.Status != metav1.ConditionFalse ||
		agentCondition.Reason != rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived {
		t.Fatalf("AgentConnected condition = %#v", agentCondition)
	}
}

func newNodeTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func stringPtr(value string) *string {
	return &value
}

func requestFor(namespace, name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
}

func conditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
