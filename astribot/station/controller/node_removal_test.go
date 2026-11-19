package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestARemovalLogsInNowhere(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: testNamespace, UID: "op-uid", Generation: 1},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "n", NodeUID: "node-uid", RequestID: "request", RequestedBy: "admin"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(op).WithObjects(op).Build()
	stub := &nodeExecutorStub{done: true, uid: "node-uid"}
	want := []string{"Preflight", "PowerOff", "Drain", "RemovePeer", "Reset", "DeleteNode", "VerifyRemoval"}
	for range len(want) + 1 {
		r := NewNodeOperationReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, stub)
		if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(op)}); err != nil {
			t.Fatal(err)
		}
	}
	stored := &stationv1alpha1.NodeOperation{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(op), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != "Succeeded" || len(stub.calls) != len(want) {
		t.Fatalf("phase %s, stages %v", stored.Status.Phase, stub.calls)
	}
	for i := range want {
		if stub.calls[i] != want[i] {
			t.Fatalf("stages %v, want %v", stub.calls, want)
		}
	}

	// The checks are the cluster's, whether the kubelet reports or not.
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown}}}}
	member := kubefake.NewSimpleClientset(node)
	executor := &NodeLifecycleExecutor{Members: nodeMemberClients{"cluster-a": member}, RemovalReady: func(context.Context, *stationv1alpha1.NodeOperation) error { return nil }}
	if done, uid, err := executor.Execute(ctx, op, "Preflight"); !done || uid != "node-uid" || err != nil {
		t.Fatalf("a node that no longer reports: %v %q %v", done, uid, err)
	}
	node.Labels = map[string]string{"node-role.kubernetes.io/control-plane": ""}
	_, _ = member.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if _, _, err := executor.Execute(ctx, op, "Preflight"); !errors.Is(err, ErrNodeDrainBlocked) {
		t.Fatalf("a control plane: %v", err)
	}
}

func TestADrainOfANodeThatNoLongerReportsDeletesItsPods(t *testing.T) {
	ctx := context.Background()
	controller := true
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "jobs", Name: "trainer", UID: "p1", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "rs", UID: "rs", Controller: &controller}}},
		Spec: corev1.PodSpec{NodeName: "n"}}
	member := kubefake.NewSimpleClientset(node, pod)
	executor := &NodeLifecycleExecutor{Members: nodeMemberClients{"cluster-a": member}, RemovalReady: func(context.Context, *stationv1alpha1.NodeOperation) error { return nil }}
	op := &stationv1alpha1.NodeOperation{Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "n", NodeUID: "node-uid"}}
	if done, _, err := executor.Execute(ctx, op, "Drain"); done || err != nil {
		t.Fatalf("drain: %v %v", done, err)
	}
	for _, action := range member.Actions() {
		if action.GetSubresource() == "eviction" {
			t.Fatal("a pod was evicted on a node no kubelet confirms evictions on")
		}
	}
	if left, _ := member.CoreV1().Pods("jobs").List(ctx, metav1.ListOptions{}); len(left.Items) != 0 {
		t.Fatalf("pods left: %d", len(left.Items))
	}
}

func TestForceDrainDeletesWhatNoKubeletWillStop(t *testing.T) {
	ctx := context.Background()
	controller := true
	owned := func(kind, name string) []metav1.OwnerReference {
		return []metav1.OwnerReference{{Kind: kind, Name: name, UID: types.UID(name), Controller: &controller}}
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"}}
	pods := []runtime.Object{node,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "jobs", Name: "trainer", UID: "p1", OwnerReferences: owned("ReplicaSet", "trainer")},
			Spec: corev1.PodSpec{NodeName: "n", Volumes: []corev1.Volume{{Name: "shm", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "jobs", Name: "bare", UID: "p2"}, Spec: corev1.PodSpec{NodeName: "n"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "proxy", UID: "p3", OwnerReferences: owned("DaemonSet", "kube-proxy")}, Spec: corev1.PodSpec{NodeName: "n"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "jobs", Name: "elsewhere", UID: "p4"}, Spec: corev1.PodSpec{NodeName: "other"}},
	}
	member := kubefake.NewSimpleClientset(pods...)
	if done, err := ForceDrainWorker(ctx, member, "n", "node-uid"); done || err != nil {
		t.Fatalf("first pass: %v %v", done, err)
	}
	left, _ := member.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	names := map[string]bool{}
	for _, pod := range left.Items {
		names[pod.Name] = true
	}
	if names["trainer"] || names["bare"] || !names["proxy"] || !names["elsewhere"] {
		t.Fatalf("pods left: %v", names)
	}
	cordoned, _ := member.CoreV1().Nodes().Get(ctx, "n", metav1.GetOptions{})
	if !cordoned.Spec.Unschedulable {
		t.Fatal("node was not cordoned")
	}
	if done, err := ForceDrainWorker(ctx, member, "n", "node-uid"); !done || err != nil {
		t.Fatalf("second pass: %v %v", done, err)
	}
	if _, err := ForceDrainWorker(ctx, member, "n", "another-uid"); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("another node of the name: %v", err)
	}
}

func tunnelFixture(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&stationv1alpha1.NetworkEnrollment{}, &stationv1alpha1.Station{}).WithObjects(objects...).Build()
}

func TestTunnelReleaseRetiresTheStationAndForgetsTheMachinesKey(t *testing.T) {
	ctx := context.Background()
	station := &stationv1alpha1.Station{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "stn-a"},
		Spec: stationv1alpha1.StationSpec{DisplayName: "Station-0001", Sequence: 1, Binding: stationv1alpha1.StationBinding{NodeName: "s1u-01-x86", NodeUID: "node-uid", Version: 1}}}
	other := &stationv1alpha1.Station{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "stn-b"},
		Spec: stationv1alpha1.StationSpec{DisplayName: "Station-0002", Sequence: 2, Binding: stationv1alpha1.StationBinding{NodeName: "s1u-02-x86", NodeUID: "other-uid", Version: 1}}}
	redeemed := metav1.Now()
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "s1u-01-x86"},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", PublicKey: "key-of-the-machine", RedeemedAt: &redeemed, Serial: "1794925030610"}}
	enrollment := &stationv1alpha1.NetworkEnrollment{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "s1u-01-x86"},
		Spec:   stationv1alpha1.NetworkEnrollmentSpec{ClusterID: "cluster-a", NodeUID: "node-uid"},
		Status: stationv1alpha1.NetworkEnrollmentStatus{Phase: stationv1alpha1.NetworkEnrollmentEnrolled}}
	network := &stationv1alpha1.SiteNetwork{ObjectMeta: metav1.ObjectMeta{Namespace: "station-system", Name: "default"}}
	c := tunnelFixture(t, station, other, admission, enrollment, network)
	release := StationTunnelRelease{Reader: c, Writer: c, Network: types.NamespacedName{Namespace: "station-system", Name: "default"}}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "s1u-01-x86", NodeUID: "node-uid"}}

	// Its enrollment is still answered with a place in the tunnel.
	if done, err := release.Remove(ctx, op); done || err != nil {
		t.Fatalf("done before the SiteNetwork let go: %v %v", done, err)
	}
	stored := &stationv1alpha1.Station{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(station), stored)
	untouched := &stationv1alpha1.Station{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(other), untouched)
	if !stored.Spec.Retired || stored.Status.Phase != stationv1alpha1.StationPhaseRetired || untouched.Spec.Retired {
		t.Fatalf("retired: removed node %v (%s), other node %v", stored.Spec.Retired, stored.Status.Phase, untouched.Spec.Retired)
	}
	forgotten := &stationv1alpha1.NodeAdmission{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(admission), forgotten)
	if forgotten.Spec.PublicKey != "" || forgotten.Spec.RedeemedAt != nil || forgotten.Spec.Serial != "1794925030610" || !AdmissionNodeRemoved(forgotten) {
		t.Fatalf("admission after removal: %+v %v", forgotten.Spec, forgotten.Annotations)
	}

	_ = c.Get(ctx, client.ObjectKeyFromObject(enrollment), enrollment)
	enrollment.Status.Phase = stationv1alpha1.NetworkEnrollmentRejected
	if err := c.Status().Update(ctx, enrollment); err != nil {
		t.Fatal(err)
	}
	if done, err := release.Remove(ctx, op); !done || err != nil {
		t.Fatalf("after the SiteNetwork let go: %v %v", done, err)
	}
}

func TestTunnelReleaseLeavesANodeTheSiteNetworkDeclaresToItsAdministrator(t *testing.T) {
	network := &stationv1alpha1.SiteNetwork{ObjectMeta: metav1.ObjectMeta{Namespace: "station-system", Name: "default"},
		Spec: stationv1alpha1.SiteNetworkSpec{Nodes: []stationv1alpha1.SiteNetworkNode{{ClusterID: "cluster-a", NodeName: "hub", PublicKey: "key", Address: "10.254.240.1"}}}}
	c := tunnelFixture(t, network)
	release := StationTunnelRelease{Reader: c, Writer: c, Network: types.NamespacedName{Namespace: "station-system", Name: "default"}}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "hub", NodeUID: "node-uid"}}
	if err := release.Check(context.Background(), op); !errors.Is(err, ErrNodeDeclaredInSiteNetwork) {
		t.Fatalf("declared hub: %v", err)
	}
	op.Spec.NodeName = "leaf"
	if err := release.Check(context.Background(), op); err != nil {
		t.Fatalf("undeclared node: %v", err)
	}
}

func TestAMachineJoiningUnderARemovedNodesNameGetsAStationOfItsOwn(t *testing.T) {
	now := time.Now().UTC()
	retired := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-removed", Namespace: testNamespace},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "Station-0001", Sequence: 1, Retired: true, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "old-uid", Version: 1, BoundBy: "system"}},
		Status:     stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseRetired},
	}
	reconciler, c := newTestReconciler(t, &now, retired, testObservation("node-a", "new-uid", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 2 {
		t.Fatalf("stations: %d", len(stations.Items))
	}
	for _, station := range stations.Items {
		switch station.Spec.Binding.NodeUID {
		case "old-uid":
			if !station.Spec.Retired || station.Status.Phase != stationv1alpha1.StationPhaseRetired {
				t.Fatalf("the removed node's Station: %+v", station.Status)
			}
		case "new-uid":
			if station.Spec.Retired || station.Status.Phase == stationv1alpha1.StationPhaseConflict {
				t.Fatalf("the new node's Station: %+v", station.Status)
			}
		}
	}
}

func TestARemovedRobotIsAMachineToAdmitAgain(t *testing.T) {
	ctx := context.Background()
	// Admitted once (its request is still on it) and joined by that request's operation.
	machine := discoveredMachine(1, true)
	machine.Status.Phase, machine.Status.ClusterID, machine.Status.NodeName, machine.Status.Operation = stationv1alpha1.MachineJoined, "robots", "s1u-01-x86", "join-s1u-01-x86-1"
	joined := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "join-s1u-01-x86-1"},
		Spec:   stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "robots", NodeName: "s1u-01-x86", Admission: "s1u-01-x86", RequestedBy: "alice"},
		Status: stationv1alpha1.NodeOperationStatus{Phase: "Succeeded"}}
	machine.Status.Entry.HostKeyFingerprint = "SHA256:rotatedrotatedrotatedrotatedrotatedrotated"
	inspected := metav1.Now()
	machine.Status.InspectedAt, machine.Status.Serial = &inspected, "1794925030610"
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robots", Name: "s1u-01-x86", Annotations: map[string]string{RemovedNodeAnnotation: "old-uid"}},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robots", Serial: "1794925030610", NodeClass: "s1u"}}
	r, c, inspections := machineFixture(t, machine, admission, joined)
	// Removed while it could not be reached, it still holds the node it was.
	inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeName: "s1u-01-x86", NodeHostKey: "SHA256:c"}
	if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
		t.Fatal(err)
	}
	stored := &stationv1alpha1.Machine{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), stored); err != nil {
		t.Fatal(err)
	}
	// Its request joined it already: spent, the machine waits to be admitted again.
	if stored.Status.Phase != stationv1alpha1.MachineDiscovered || stored.Status.Problem != "" || stored.Status.NodeName != "" || inspections.calls != 1 {
		t.Fatalf("machine after its node was removed: %+v (inspected %d times)", stored.Status, inspections.calls)
	}
	if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(ctx, client.ObjectKeyFromObject(machine), stored)
	if stored.Status.Phase != stationv1alpha1.MachineDiscovered {
		t.Fatalf("the spent request admitted the machine again: %+v", stored.Status)
	}
	// Admitted again, it is the same node: the name bound to its serial.
	stored.Spec.Admit = &stationv1alpha1.MachineAdmit{ClusterID: "robots", RequestedBy: "alice", RequestedAt: metav1.Now()}
	stored.Generation = 2
	if err := c.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	inspections.seen[machine.Status.Entry.Address] = MachineInspection{Serial: "1794925030610", NodeName: "s1u-01-x86", NodeHostKey: "SHA256:c"}
	stored.Status.InspectedAt = nil
	if err := c.Status().Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, machineRequest(machine)); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != stationv1alpha1.MachineAdmitted || stored.Status.NodeName != "s1u-01-x86" || stored.Status.Operation != "join-s1u-01-x86-2" {
		t.Fatalf("machine admitted again: %+v", stored.Status)
	}
}

// A name its cluster has no Node of is removed all the same (nb260706069 11-16:
// a join that never made a node could neither join nor be removed). The
// removal names no UID; its Preflight finds no Node, nothing in the cluster is
// touched, and what RLark keeps of the name goes: the Stations bound to it are
// retired and an admission that never made a node is cleared. A Node of the
// name found by the Preflight is the one removed.
func TestARemovalOfANameWithoutANodeClearsWhatRLarkKeepsOfIt(t *testing.T) {
	ctx := context.Background()
	member := kubefake.NewSimpleClientset()
	checked := 0
	executor := &NodeLifecycleExecutor{Members: nodeMemberClients{"cluster-a": member}, Management: tunnelFixture(t),
		RemovalReady: func(context.Context, *stationv1alpha1.NodeOperation) error { checked++; return nil }}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, UID: "op-uid"},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "cailiang-desktop"}}
	for _, stage := range []string{"Preflight", "PowerOff", "Drain", "Reset", "DeleteNode", "VerifyRemoval"} {
		if done, uid, err := executor.Execute(ctx, op, stage); !done || uid != "" || err != nil {
			t.Fatalf("%s of a name without a Node: %v %q %v", stage, done, uid, err)
		}
	}
	if checked != 1 || len(member.Actions()) != 3 {
		t.Fatalf("the cluster is only read: checks %d, actions %v", checked, member.Actions())
	}

	// The Node a removal by name finds is the one it goes on with.
	_, _ = member.CoreV1().Nodes().Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cailiang-desktop", UID: "found-uid"}}, metav1.CreateOptions{})
	if done, uid, err := executor.Execute(ctx, op, "Preflight"); !done || uid != "found-uid" || err != nil {
		t.Fatalf("a Node found by name: %v %q %v", done, uid, err)
	}
	op.Spec.NodeUID = ""

	stale := &stationv1alpha1.Station{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "stn-stale"},
		Spec: stationv1alpha1.StationSpec{DisplayName: "Station-0004", Sequence: 4, Binding: stationv1alpha1.StationBinding{NodeName: "cailiang-desktop", NodeUID: "gone-uid", Version: 2}}}
	unfinished := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "cailiang-desktop", UID: "admission-uid"},
		Spec:   stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", PublicKey: "key"},
		Status: stationv1alpha1.NodeAdmissionStatus{Phase: stationv1alpha1.NodeAdmissionRedeemed, Address: "10.254.240.8"}}
	machine := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "s1u-03-x86"},
		Spec:   stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", PublicKey: "robot-key", Serial: "1423"},
		Status: stationv1alpha1.NodeAdmissionStatus{Phase: stationv1alpha1.NodeAdmissionRedeemed}}
	c := tunnelFixture(t, stale, unfinished, machine, &stationv1alpha1.SiteNetwork{ObjectMeta: metav1.ObjectMeta{Namespace: "station-system", Name: "default"}})
	release := StationTunnelRelease{Reader: c, Writer: c, Network: types.NamespacedName{Namespace: "station-system", Name: "default"}}
	if done, err := release.Remove(ctx, op); !done || err != nil {
		t.Fatalf("release of a name without a Node: %v %v", done, err)
	}
	stored := &stationv1alpha1.Station{}
	if _ = c.Get(ctx, client.ObjectKeyFromObject(stale), stored); !stored.Spec.Retired {
		t.Fatal("the Station of a name without a Node must be retired")
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(unfinished), &stationv1alpha1.NodeAdmission{}); err == nil {
		t.Fatal("the admission that never made a node must be cleared")
	}
	// A discovered machine's admission stays, without the machine's key.
	op.Spec.NodeName = "s1u-03-x86"
	if done, err := release.Remove(ctx, op); !done || err != nil {
		t.Fatalf("release of a discovered machine without a Node: %v %v", done, err)
	}
	kept := &stationv1alpha1.NodeAdmission{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machine), kept); err != nil || kept.Spec.PublicKey != "" || kept.Annotations[RemovedNodeAnnotation] != "op-uid" {
		t.Fatalf("discovered machine's admission: %+v %v %v", kept.Spec, kept.Annotations, err)
	}
}
