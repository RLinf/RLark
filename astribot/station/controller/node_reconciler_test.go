package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const testNamespace = "rlark-agent-test"

type testRegistrations map[string]ClusterRegistration

func (m testRegistrations) ResolveNamespace(_ context.Context, namespace string) (ClusterRegistration, error) {
	registration, ok := m[namespace]
	if !ok {
		return ClusterRegistration{}, fmt.Errorf("unregistered namespace %q", namespace)
	}
	return registration, nil
}

type testNodeReadiness struct{ ready bool }

func (r testNodeReadiness) Ready(context.Context, string, string, string) (bool, error) {
	return r.ready, nil
}

// testNode is the record of node-a as station-api leaves it after a report of nodeUID at the given time.
func testNode(recordedUID, reportedUID string, at time.Time) *stationv1alpha1.NetworkEnrollment {
	return &stationv1alpha1.NetworkEnrollment{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: testNamespace},
		Spec:       stationv1alpha1.NetworkEnrollmentSpec{ClusterID: "cluster-a", NodeUID: recordedUID, SystemUUID: "system-a"},
		Status: stationv1alpha1.NetworkEnrollmentStatus{Phase: stationv1alpha1.NetworkEnrollmentEnrolled, Observation: &stationv1alpha1.NodeObservation{
			ReportedAt: metav1.NewTime(at), NodeUID: reportedUID, SystemUUID: "system-b", BootTime: &metav1.Time{Time: at.Add(-time.Hour)},
			Probes: []stationv1alpha1.StationProbe{{Target: "10.254.240.1", OK: true}},
		}},
	}
}

func newNodeTest(t *testing.T, now *time.Time, objects ...client.Object) (func() (reconcile.Result, *stationv1alpha1.NetworkEnrollment), client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&stationv1alpha1.NetworkEnrollment{}).WithObjects(objects...).Build()
	r := NewNodeReconciler(c, c, testNodeReadiness{ready: true}, DefaultOfflineAfter, func() time.Time { return *now })
	key := types.NamespacedName{Namespace: testNamespace, Name: "node-a"}
	return func() (reconcile.Result, *stationv1alpha1.NetworkEnrollment) {
		t.Helper()
		result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
		if err != nil {
			t.Fatal(err)
		}
		stored := &stationv1alpha1.NetworkEnrollment{}
		if err := c.Get(context.Background(), key, stored); err != nil {
			t.Fatal(err)
		}
		return result, stored
	}, c
}

// report stores what station-api would after another report of the node.
func report(t *testing.T, c client.Client, enrollment *stationv1alpha1.NetworkEnrollment, change func(*stationv1alpha1.NodeObservation)) {
	t.Helper()
	change(enrollment.Status.Observation)
	if err := c.Status().Update(context.Background(), enrollment); err != nil {
		t.Fatal(err)
	}
}

func TestANodesReportsMakeItBoundCountItsRebootsAndLetItGoOffline(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	reconcileNode, c := newNodeTest(t, &now, testNode("uid-a", "uid-a", now))
	result, stored := reconcileNode()
	node := stored.Status.Node
	if node.State != stationv1alpha1.NodeStateBound || !node.LastSeenTime.Time.Equal(now) || node.Reboots != 0 || result.RequeueAfter != DefaultOfflineAfter {
		t.Fatalf("first report: %+v, requeue %s", node, result.RequeueAfter)
	}
	for condition, want := range map[string]metav1.ConditionStatus{"Observed": metav1.ConditionTrue, "IdentityMatched": metav1.ConditionTrue, "NetworkVerified": metav1.ConditionTrue, "RLarkLinked": metav1.ConditionTrue} {
		if got := apimeta.FindStatusCondition(node.Conditions, condition); got == nil || got.Status != want {
			t.Fatalf("%s: %+v", condition, got)
		}
	}
	// The network's answer is not this reconciler's and stays.
	if stored.Status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled {
		t.Fatalf("the network's answer was touched: %+v", stored.Status)
	}

	// It restarts and reports that a peer that is online does not answer.
	peer := &stationv1alpha1.NetworkEnrollment{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Namespace: testNamespace}, Spec: stationv1alpha1.NetworkEnrollmentSpec{ClusterID: "cluster-a", NodeUID: "uid-b"},
		Status: stationv1alpha1.NetworkEnrollmentStatus{TunnelAddress: "10.254.240.1", Node: &stationv1alpha1.NetworkEnrollmentNode{State: stationv1alpha1.NodeStateBound}}}
	if err := c.Create(context.Background(), peer); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	report(t, c, stored, func(o *stationv1alpha1.NodeObservation) {
		o.ReportedAt, o.BootTime, o.Probes[0].OK = metav1.NewTime(now), &metav1.Time{Time: now.Add(-10 * time.Second)}, false
	})
	_, stored = reconcileNode()
	if node = stored.Status.Node; node.Reboots != 1 || apimeta.FindStatusCondition(node.Conditions, "Rebooted") == nil ||
		apimeta.FindStatusCondition(node.Conditions, "NetworkVerified").Status != metav1.ConditionFalse {
		t.Fatalf("after a restart: %+v", node)
	}

	// Its reports stop.
	now = now.Add(DefaultOfflineAfter)
	result, stored = reconcileNode()
	if node = stored.Status.Node; node.State != stationv1alpha1.NodeStateOffline || node.Reboots != 1 || result.RequeueAfter != 0 {
		t.Fatalf("no report for the window: %+v, requeue %s", node, result.RequeueAfter)
	}
}

func TestAnotherNodeOfTheNameTakesTheRecordOverOnlyByAnAdmission(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	seen := metav1.NewTime(now.Add(-time.Hour))
	enrollment := testNode("uid-old", "uid-new", now)
	enrollment.Status.Node = &stationv1alpha1.NetworkEnrollmentNode{State: stationv1alpha1.NodeStateOffline, LastSeenTime: &seen, Reboots: 3}
	before := metav1.NewTime(seen.Add(-time.Minute))
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: testNamespace},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", RedeemedAt: &before}}
	reconcileNode, c := newNodeTest(t, &now, enrollment, admission)

	// The admission is of the record's own node: nothing let the new one in.
	_, stored := reconcileNode()
	identity := apimeta.FindStatusCondition(stored.Status.Node.Conditions, "IdentityMatched")
	if stored.Status.Node.State != stationv1alpha1.NodeStateConflict || identity.Reason != "NodeUIDChanged" || !strings.Contains(identity.Message, "delete this node record") ||
		stored.Spec.NodeUID != "uid-old" || !stored.Status.Node.LastSeenTime.Equal(&seen) {
		t.Fatalf("an unadmitted Node of the name: spec %+v node %+v", stored.Spec, stored.Status.Node)
	}

	// A machine is admitted under the name after the record's node was last seen.
	after := metav1.NewTime(seen.Add(time.Minute))
	admission.Spec.RedeemedAt = &after
	if err := c.Update(context.Background(), admission); err != nil {
		t.Fatal(err)
	}
	_, stored = reconcileNode()
	if stored.Spec.NodeUID != "uid-new" || stored.Spec.SystemUUID != "system-b" || stored.Status.Node.Reboots != 0 || stored.Status.Node.State != "" ||
		apimeta.FindStatusCondition(stored.Status.Node.Conditions, "Replaced") == nil {
		t.Fatalf("the record must pass to the admitted Node: spec %+v node %+v", stored.Spec, stored.Status.Node)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(admission), admission); err != nil || !strings.Contains(admission.Annotations[SelfHealedAnnotation], "uid-old") {
		t.Fatalf("the admission says what was put right: %v %v", admission.Annotations, err)
	}
	// The spec change reconciles it again, as the new Node's.
	if _, stored = reconcileNode(); stored.Status.Node.State != stationv1alpha1.NodeStateBound || stored.Status.Node.Reboots != 0 {
		t.Fatalf("the new Node's first pass: %+v", stored.Status.Node)
	}
}

func TestARemovedNodeStaysRetiredUntilAMachineIsAdmittedAgain(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	seen := metav1.NewTime(now.Add(-time.Minute))
	enrollment := testNode("uid-a", "uid-a", now)
	enrollment.Spec.Retired = true
	enrollment.Status.Node = &stationv1alpha1.NetworkEnrollmentNode{State: stationv1alpha1.NodeStateBound, LastSeenTime: &seen}
	reconcileNode, c := newNodeTest(t, &now, enrollment)
	// It still reports while it is being reset.
	_, stored := reconcileNode()
	if stored.Status.Node.State != stationv1alpha1.NodeStateRetired || !stored.Spec.Retired || !stored.Status.Node.LastSeenTime.Equal(&seen) {
		t.Fatalf("a removed node that reports: spec %+v node %+v", stored.Spec, stored.Status.Node)
	}
	redeemed := metav1.NewTime(now)
	if err := c.Create(context.Background(), &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: testNamespace},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", RedeemedAt: &redeemed}}); err != nil {
		t.Fatal(err)
	}
	if _, stored = reconcileNode(); stored.Spec.Retired {
		t.Fatalf("admitted again, the record is the node's: %+v", stored.Spec)
	}
}
