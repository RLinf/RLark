package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

type testNodeReadiness struct {
	ready bool
	err   error
}

func (r testNodeReadiness) Ready(context.Context, string, string, string) (bool, error) {
	return r.ready, r.err
}

func newTestReconciler(t *testing.T, now *time.Time, objects ...client.Object) (*StationReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&stationv1alpha1.Station{}, &stationv1alpha1.StationObservation{}, &stationv1alpha1.StationSequence{}, &stationv1alpha1.StationRebind{}).
		WithObjects(objects...).Build()
	reconciler := NewStationReconciler(c, c, testRegistrations{
		testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace},
	}, NewSerializedIdentityOperations(), testNodeReadiness{ready: true}, DefaultOfflineAfter, func() time.Time { return *now })
	return reconciler, c
}

func testObservation(name, nodeUID string, at time.Time) *stationv1alpha1.StationObservation {
	return &stationv1alpha1.StationObservation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       stationv1alpha1.StationObservationSpec{},
		Status: stationv1alpha1.StationObservationStatus{
			ReportedAt: metav1.NewTime(at),
			NodeUID:    nodeUID,
			SystemUUID: "system-a",
			BootTime:   &metav1.Time{Time: at.Add(-time.Hour)},
			Probes:     []stationv1alpha1.StationProbe{{Target: "nas:445", OK: true}},
		},
	}
}

func requestFor(name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}}
}

func getStation(t *testing.T, c client.Client, name string) *stationv1alpha1.Station {
	t.Helper()
	station := &stationv1alpha1.Station{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: name}, station); err != nil {
		t.Fatal(err)
	}
	return station
}

func TestReconcileCreatesDeterministicStationAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	reconciler, c := newTestReconciler(t, &now, testObservation("node-a", "node-uid-a", now))
	result, err := reconciler.Reconcile(context.Background(), requestFor("node-a"))
	if err != nil {
		t.Fatal(err)
	}
	name := StationName("cluster-a", "node-uid-a")
	station := getStation(t, c, name)
	if station.Spec.Sequence != 1 || station.Spec.Binding.Version != 1 || station.Status.Phase != stationv1alpha1.StationPhaseBound {
		t.Fatalf("unexpected Station after first observation: %#v", station)
	}
	if result.RequeueAfter != DefaultOfflineAfter {
		t.Fatalf("fresh observation requeue=%s, want %s", result.RequeueAfter, DefaultOfflineAfter)
	}
	if condition := findCondition(station, "Observed"); condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatal("fresh observation did not set Observed=True")
	}
	if condition := findCondition(station, "RLarkLinked"); condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatal("ready RLark Node did not set RLarkLinked=True")
	}

	_, err = reconciler.Reconcile(context.Background(), requestFor("node-a"))
	if err != nil {
		t.Fatal(err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 || stations.Items[0].Name != name {
		t.Fatalf("duplicate notification created Stations: %#v", stations.Items)
	}
	sequence := &stationv1alpha1.StationSequence{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: SequenceName}, sequence); err != nil {
		t.Fatal(err)
	}
	if sequence.Status.LastAssigned != 1 {
		t.Fatalf("idempotent reconcile advanced sequence to %d", sequence.Status.LastAssigned)
	}
}

func TestConcurrentFirstObservationsCreateOneStableStation(t *testing.T) {
	now := time.Now().UTC()
	reconciler, c := newTestReconciler(t, &now, testObservation("node-a", "node-uid-a", now))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := reconciler.Reconcile(context.Background(), requestFor("node-a"))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent reconcile failed: %v", err)
		}
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 || stations.Items[0].Name != StationName("cluster-a", "node-uid-a") {
		t.Fatalf("concurrent reconcile created duplicate or unstable Station IDs: %#v", stations.Items)
	}
}

func TestChangedNodeUIDConflictsWithoutAutoAdoption(t *testing.T) {
	now := time.Now().UTC()
	old := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-previous", Namespace: testNamespace},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "Station-0001", Sequence: 1, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "old-uid", Version: 1, BoundBy: "system"}},
		Status:     stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseBound},
	}
	reconciler, c := newTestReconciler(t, &now, old, testObservation("node-a", "new-uid", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	station := getStation(t, c, old.Name)
	if station.Spec.Binding.NodeUID != "old-uid" || station.Status.Phase != stationv1alpha1.StationPhaseConflict {
		t.Fatalf("changed Node UID was adopted: spec=%#v status=%#v", station.Spec, station.Status)
	}
	if condition := apimeta.FindStatusCondition(station.Status.Conditions, "IdentityMatched"); condition == nil || !strings.Contains(condition.Message, "no join code or SSH join admitted") {
		t.Fatalf("the conflict must say what to do: %#v", condition)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 {
		t.Fatalf("replacement Station was automatically created: %d Stations", len(stations.Items))
	}
}

func TestDuplicateNodeBindingsBecomeConflict(t *testing.T) {
	now := time.Now().UTC()
	first := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-first", Namespace: testNamespace, UID: "first-uid"},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "First", Sequence: 1, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "old-a", Version: 1, BoundBy: "system"}},
	}
	second := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-second", Namespace: testNamespace, UID: "second-uid"},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "Second", Sequence: 2, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "old-b", Version: 1, BoundBy: "system"}},
	}
	reconciler, c := newTestReconciler(t, &now, first, second, testObservation("node-a", "new-uid", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stn-first", "stn-second"} {
		if got := getStation(t, c, name).Status.Phase; got != stationv1alpha1.StationPhaseConflict {
			t.Fatalf("Station %s phase=%s, want Conflict", name, got)
		}
	}
}

func TestDeterministicNameCollisionFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	name := StationName("cluster-a", "node-uid-a")
	existing := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "Unrelated", Sequence: 9, Binding: stationv1alpha1.StationBinding{NodeName: "node-z", NodeUID: "node-uid-z", Version: 1, BoundBy: "system"}},
	}
	reconciler, c := newTestReconciler(t, &now, existing, testObservation("node-a", "node-uid-a", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); !errors.Is(err, ErrStationNameCollision) {
		t.Fatalf("collision error=%v, want ErrStationNameCollision", err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 || stations.Items[0].Spec.Binding.NodeUID != "node-uid-z" {
		t.Fatalf("collision modified or adopted foreign Station: %#v", stations.Items)
	}
}

func TestHeartbeatExpiresAndRecoversFromCurrentObservation(t *testing.T) {
	now := time.Now().UTC()
	observation := testObservation("node-a", "node-uid-a", now)
	reconciler, c := newTestReconciler(t, &now, observation)
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	name := StationName("cluster-a", "node-uid-a")
	now = now.Add(DefaultOfflineAfter)
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	if got := getStation(t, c, name).Status.Phase; got != stationv1alpha1.StationPhaseOffline {
		t.Fatalf("stale observation phase=%s, want Offline", got)
	}
	var current stationv1alpha1.StationObservation
	key := types.NamespacedName{Namespace: testNamespace, Name: "node-a"}
	if err := c.Get(context.Background(), key, &current); err != nil {
		t.Fatal(err)
	}
	current.Status.ReportedAt = metav1.NewTime(now)
	if err := c.Status().Update(context.Background(), &current); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	if got := getStation(t, c, name).Status.Phase; got != stationv1alpha1.StationPhaseBound {
		t.Fatalf("fresh observation failed to recover Station, phase=%s", got)
	}
}

func TestMissingObservationMarksStationOfflineButPreservesBinding(t *testing.T) {
	now := time.Now().UTC()
	reconciler, c := newTestReconciler(t, &now, testObservation("node-a", "node-uid-a", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), &stationv1alpha1.StationObservation{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: testNamespace}}); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	station := getStation(t, c, StationName("cluster-a", "node-uid-a"))
	if station.Status.Phase != stationv1alpha1.StationPhaseOffline || station.Spec.Binding.NodeUID != "node-uid-a" {
		t.Fatalf("missing observation changed binding or failed to mark offline: %#v", station)
	}
}

func TestNodeReadinessFailureRecoversWithoutAllocatingAnotherStation(t *testing.T) {
	now := time.Now().UTC()
	reconciler, c := newTestReconciler(t, &now, testObservation("node-a", "node-uid-a", now))
	reconciler.nodeReadiness = testNodeReadiness{err: errors.New("temporary read failure")}
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err == nil {
		t.Fatal("readiness dependency failure was ignored")
	}
	name := StationName("cluster-a", "node-uid-a")
	station := &stationv1alpha1.Station{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: name}, station); err != nil {
		t.Fatalf("expected idempotently created Station before status retry: %v", err)
	}
	reconciler.nodeReadiness = testNodeReadiness{ready: true}
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 || stations.Items[0].Status.Phase != stationv1alpha1.StationPhaseBound {
		t.Fatalf("recovery created duplicate or failed to set status: %#v", stations.Items)
	}
	sequence := &stationv1alpha1.StationSequence{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: SequenceName}, sequence); err != nil {
		t.Fatal(err)
	}
	if sequence.Status.LastAssigned != 1 {
		t.Fatalf("retry allocated another sequence: %d", sequence.Status.LastAssigned)
	}
}

func TestRetiredStationIsNotReactivated(t *testing.T) {
	now := time.Now().UTC()
	retired := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-retired", Namespace: testNamespace},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "Retired", Sequence: 1, Retired: true, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "node-uid-a", Version: 1, BoundBy: "system"}},
		Status:     stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseRetired},
	}
	reconciler, c := newTestReconciler(t, &now, retired, testObservation("node-a", "node-uid-a", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 1 || stations.Items[0].Status.Phase != stationv1alpha1.StationPhaseRetired {
		t.Fatalf("heartbeat reactivated or replaced retired Station: %#v", stations.Items)
	}
}

func TestSequenceOwnerMismatchFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	sequence := &stationv1alpha1.StationSequence{
		ObjectMeta: metav1.ObjectMeta{Name: SequenceName, Namespace: testNamespace},
		Spec:       stationv1alpha1.StationSequenceSpec{ClusterID: "other-cluster"},
	}
	reconciler, c := newTestReconciler(t, &now, sequence, testObservation("node-a", "node-uid-a", now))
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); !errors.Is(err, ErrSequenceOwnerConflict) {
		t.Fatalf("sequence ownership error=%v, want ErrSequenceOwnerConflict", err)
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(stations.Items) != 0 {
		t.Fatalf("created Station despite sequence ownership conflict: %d", len(stations.Items))
	}
}

func findCondition(station *stationv1alpha1.Station, conditionType string) *metav1.Condition {
	return apimeta.FindStatusCondition(station.Status.Conditions, conditionType)
}

// Online is three separate facts. A fresh heartbeat does not say the tunnel
// works, and a node is not unverified because a peer of its is switched off.
func TestTunnelVerdictCountsOnlyPeersThatAreOnline(t *testing.T) {
	now := time.Date(2026, 11, 12, 4, 0, 0, 0, time.UTC)
	observation := testObservation("node-a", "node-uid-a", now)
	// 10.254.240.8 is a machine whose admission was redeemed before it came up: in the tunnel, with no enrollment yet.
	observation.Status.Probes = []stationv1alpha1.StationProbe{{Target: "10.254.240.1", Via: "st-wg0", OK: true}, {Target: "10.254.240.3", Via: "st-wg0"}, {Target: "10.254.240.8", Via: "st-wg0"}}
	peer := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Namespace: "other-cluster", Name: "stn-robot"},
		Spec:       stationv1alpha1.StationSpec{DisplayName: "robot", Sequence: 1, Binding: stationv1alpha1.StationBinding{NodeName: "robot", NodeUID: "u", Version: 1, BoundBy: "t"}},
		Status:     stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseBound},
	}
	enrollment := &stationv1alpha1.NetworkEnrollment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "other-cluster", Name: "robot"},
		Status:     stationv1alpha1.NetworkEnrollmentStatus{Phase: stationv1alpha1.NetworkEnrollmentEnrolled, TunnelAddress: "10.254.240.3"},
	}
	reconciler, c := newTestReconciler(t, &now, observation, peer, enrollment)
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	name := StationName("cluster-a", "node-uid-a")
	station := getStation(t, c, name)
	if condition := findCondition(station, "NetworkVerified"); station.Status.Phase != stationv1alpha1.StationPhaseBound || condition == nil || condition.Status != metav1.ConditionFalse || !strings.Contains(condition.Message, "10.254.240.3") || strings.Contains(condition.Message, "10.254.240.8") {
		t.Fatalf("a live heartbeat with an unreachable online peer: phase %s, %+v", station.Status.Phase, condition)
	}

	// The robot is switched off: its Station goes Offline, and this node is verified again.
	peer.Status.Phase = stationv1alpha1.StationPhaseOffline
	if err := c.Status().Update(context.Background(), peer); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
		t.Fatal(err)
	}
	if condition := findCondition(getStation(t, c, name), "NetworkVerified"); condition == nil || condition.Status != metav1.ConditionTrue || !strings.Contains(condition.Message, "reached 1 of 3") {
		t.Fatalf("an offline peer must not count: %+v", condition)
	}
}

// A restart shorter than the heartbeat window never shows as Offline; the boot
// time records it.
func TestRebootLeavesATrace(t *testing.T) {
	now := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	observation := testObservation("node-a", "node-uid-a", now)
	reconciler, c := newTestReconciler(t, &now, observation)
	reconcileOnce := func() *stationv1alpha1.Station {
		t.Helper()
		if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
			t.Fatal(err)
		}
		return getStation(t, c, StationName("cluster-a", "node-uid-a"))
	}
	reconcileOnce() // creates the Station
	if station := reconcileOnce(); station.Status.Reboots != 0 || findCondition(station, "Rebooted") != nil {
		t.Fatalf("no reboot yet: %+v", station.Status)
	}
	report := func(boot time.Time) {
		t.Helper()
		var current stationv1alpha1.StationObservation
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "node-a"}, &current); err != nil {
			t.Fatal(err)
		}
		current.Status.ReportedAt, current.Status.BootTime = metav1.NewTime(now), &metav1.Time{Time: boot}
		if err := c.Status().Update(context.Background(), &current); err != nil {
			t.Fatal(err)
		}
	}
	first := observation.Status.BootTime.Time
	// The clock was stepped by a second: the same boot.
	now = now.Add(30 * time.Second)
	report(first.Add(time.Second))
	if station := reconcileOnce(); station.Status.Reboots != 0 {
		t.Fatal("a stepped clock counted as a reboot")
	}
	// The machine restarted 40 s ago; its heartbeat never lapsed.
	now = now.Add(30 * time.Second)
	rebooted := now.Add(-40 * time.Second)
	report(rebooted)
	station := reconcileOnce()
	condition := findCondition(station, "Rebooted")
	if station.Status.Phase != stationv1alpha1.StationPhaseBound || station.Status.Reboots != 1 || condition == nil || !strings.Contains(condition.Message, rebooted.Format(time.RFC3339)) || !condition.LastTransitionTime.Time.Equal(now) {
		t.Fatalf("reboot not recorded: reboots %d, %+v", station.Status.Reboots, condition)
	}
	if station = reconcileOnce(); station.Status.Reboots != 1 {
		t.Fatal("the same boot counted twice")
	}
}

// A machine an administrator admitted under the name after the node of the
// Station was last seen is the node the name stands for (nb260706069 11-16:
// reset by hand, joined again with a code, its Station still bound to the
// gone UID). The same machine (system UUID) gets its Station back through a
// StationRebind; another machine gets a Station of its own and the old one
// is retired. Either is written on the admission for the console.
func TestANodeAdmittedAgainUnderItsNameResolvesItsStation(t *testing.T) {
	now := time.Now().UTC()
	lastSeen := metav1.NewTime(now.Add(-time.Hour))
	redeemed := metav1.NewTime(now.Add(-10 * time.Minute))
	station := func() *stationv1alpha1.Station {
		return &stationv1alpha1.Station{
			ObjectMeta: metav1.ObjectMeta{Name: "stn-previous", Namespace: testNamespace, UID: "station-uid"},
			Spec:       stationv1alpha1.StationSpec{DisplayName: "Station-0004", Sequence: 4, Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "old-uid", SystemUUID: "system-a", Version: 2, BoundBy: "system"}},
			Status:     stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseOffline, LastSeenTime: &lastSeen},
		}
	}
	admission := func() *stationv1alpha1.NodeAdmission {
		return &stationv1alpha1.NodeAdmission{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: testNamespace},
			Spec:       stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", CodeHash: strings.Repeat("0", 64), PublicKey: "key", RedeemedAt: &redeemed},
		}
	}
	readAdmission := func(c client.Client) *stationv1alpha1.NodeAdmission {
		stored := &stationv1alpha1.NodeAdmission{}
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "node-a"}, stored); err != nil {
			t.Fatal(err)
		}
		return stored
	}

	t.Run("the same machine is rebound", func(t *testing.T) {
		reconciler, c := newTestReconciler(t, &now, station(), admission(), testObservation("node-a", "new-uid", now))
		if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
			t.Fatal(err)
		}
		var rebinds stationv1alpha1.StationRebindList
		if err := c.List(context.Background(), &rebinds, client.InNamespace(testNamespace)); err != nil || len(rebinds.Items) != 1 {
			t.Fatalf("one StationRebind: %v %v", rebinds.Items, err)
		}
		rebind := rebinds.Items[0].Spec
		if rebind.StationRef.UID != "station-uid" || rebind.ExpectedBindingVersion != 2 || rebind.TargetNodeUID != "new-uid" || rebind.TargetSystemUUID != "system-a" || rebind.RequestedBy != SelfHealRequester {
			t.Fatalf("rebind: %+v", rebind)
		}
		if getStation(t, c, "stn-previous").Spec.Retired {
			t.Fatal("the Station of the same machine must not be retired")
		}
		if note := readAdmission(c).Annotations[SelfHealedAnnotation]; !strings.Contains(note, "Station-0004") || !strings.Contains(note, "重绑") {
			t.Fatalf("admission note: %q", note)
		}
		// Seen again before the rebind is applied: no second request.
		if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
			t.Fatal(err)
		}
		if err := c.List(context.Background(), &rebinds, client.InNamespace(testNamespace)); err != nil || len(rebinds.Items) != 1 {
			t.Fatalf("still one StationRebind: %d %v", len(rebinds.Items), err)
		}
	})

	t.Run("another machine gets a Station of its own", func(t *testing.T) {
		other := testObservation("node-a", "new-uid", now)
		other.Status.SystemUUID = "system-b"
		reconciler, c := newTestReconciler(t, &now, station(), admission(), other)
		for range 2 {
			if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
				t.Fatal(err)
			}
		}
		if gone := getStation(t, c, "stn-previous"); !gone.Spec.Retired || gone.Status.Phase != stationv1alpha1.StationPhaseRetired {
			t.Fatalf("the Station of the gone node must be retired and say so: %+v %+v", gone.Spec, gone.Status)
		}
		fresh := getStation(t, c, StationName("cluster-a", "new-uid"))
		if fresh.Spec.Binding.NodeUID != "new-uid" || fresh.Status.Phase != stationv1alpha1.StationPhaseBound {
			t.Fatalf("new Station: %+v %+v", fresh.Spec, fresh.Status)
		}
		if note := readAdmission(c).Annotations[SelfHealedAnnotation]; !strings.Contains(note, "退役") || !strings.Contains(note, "systemUUID 不同") {
			t.Fatalf("admission note: %q", note)
		}
	})

	t.Run("an admission redeemed before the node was last seen proves nothing", func(t *testing.T) {
		before := admission()
		early := metav1.NewTime(now.Add(-2 * time.Hour))
		before.Spec.RedeemedAt = &early
		reconciler, c := newTestReconciler(t, &now, station(), before, testObservation("node-a", "new-uid", now))
		if _, err := reconciler.Reconcile(context.Background(), requestFor("node-a")); err != nil {
			t.Fatal(err)
		}
		if got := getStation(t, c, "stn-previous"); got.Spec.Retired || got.Status.Phase != stationv1alpha1.StationPhaseConflict {
			t.Fatalf("station: %+v %+v", got.Spec, got.Status)
		}
	})
}
