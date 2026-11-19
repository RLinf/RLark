package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type testRebindTarget struct {
	uid        string
	systemUUID string
	exists     bool
	err        error
}

func (r testRebindTarget) VerifyNode(context.Context, string, string, string) (string, string, bool, error) {
	return r.uid, r.systemUUID, r.exists, r.err
}

type testRebindAudit struct {
	mu     sync.Mutex
	events map[string]RebindAuditEvent
	err    error
}

func (a *testRebindAudit) Record(_ context.Context, event RebindAuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	if a.events == nil {
		a.events = make(map[string]RebindAuditEvent)
	}
	if previous, ok := a.events[event.OperationUID]; ok {
		if previous.StationUID != event.StationUID || previous.ObservedStationUID != event.ObservedStationUID || previous.OldNodeUID != event.OldNodeUID || previous.NewNodeUID != event.NewNodeUID || previous.NewVersion != event.NewVersion {
			return errors.New("non-idempotent audit event")
		}
		return nil
	}
	a.events[event.OperationUID] = event
	return nil
}

func newTestRebindReconciler(t *testing.T, c client.Client, target RebindTargetReader, audit RebindAuditSink, now func() time.Time, coordinators ...IdentityOperationCoordinator) *RebindReconciler {
	t.Helper()
	coordinator := IdentityOperationCoordinator(NewSerializedIdentityOperations())
	if len(coordinators) > 0 {
		coordinator = coordinators[0]
	}
	reconciler := NewRebindReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, coordinator, target, audit, now)
	return reconciler
}

func testRebindObjects() (*stationv1alpha1.Station, *stationv1alpha1.StationRebind) {
	station := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-a", Namespace: testNamespace, UID: "station-uid"},
		Spec: stationv1alpha1.StationSpec{
			DisplayName: "Station 1", Sequence: 1,
			Binding: stationv1alpha1.StationBinding{NodeName: "node-a", NodeUID: "node-a-uid", Version: 3, BoundBy: "system"},
		},
		Status: stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseConflict},
	}
	operation := &stationv1alpha1.StationRebind{
		ObjectMeta: metav1.ObjectMeta{Name: "rebind-req-1", Namespace: testNamespace, UID: "rebind-object-uid"},
		Spec: stationv1alpha1.StationRebindSpec{
			RequestID: "request-1", StationRef: stationv1alpha1.StationReference{Name: station.Name, UID: string(station.UID)},
			ExpectedBindingVersion: 3, TargetNodeName: "node-b", TargetNodeUID: "node-b-uid",
			Reason: "node reinstalled", RequestedBy: "alice",
		},
	}
	return station, operation
}

func TestRebindPersistsOldBindingAndRetriesAuditAfterFailure(t *testing.T) {
	now := time.Date(2026, 11, 11, 1, 2, 3, 0, time.UTC)
	station, operation := testRebindObjects()
	c := newTestClient(t, station, operation)
	audit := &testRebindAudit{err: errors.New("temporary audit outage")}
	reconciler := newTestRebindReconciler(t, c, testRebindTarget{uid: "node-b-uid", systemUUID: "system-b", exists: true}, audit, func() time.Time { return now })
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Fatalf("audit retry delay=%s", result.RequeueAfter)
	}
	updated := &stationv1alpha1.Station{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: station.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Binding.NodeUID != "node-b-uid" || updated.Spec.Binding.Version != 4 || updated.Spec.Binding.BoundBy != "admin:alice" {
		t.Fatalf("rebind did not apply expected binding: %#v", updated.Spec.Binding)
	}
	currentOperation := &stationv1alpha1.StationRebind{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, currentOperation); err != nil {
		t.Fatal(err)
	}
	if currentOperation.Status.PreviousBinding == nil || currentOperation.Status.PreviousBinding.NodeUID != "node-a-uid" {
		t.Fatalf("old binding not persisted before update: %#v", currentOperation.Status.PreviousBinding)
	}

	audit.err = nil
	now = now.Add(5 * time.Second)
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events=%d, want exactly one", len(audit.events))
	}
	event := audit.events[string(operation.UID)]
	if event.OldNodeUID != "node-a-uid" || event.NewNodeUID != "node-b-uid" || event.OldVersion != 3 || event.NewVersion != 4 {
		t.Fatalf("audit did not retain the transition: %#v", event)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, currentOperation); err != nil {
		t.Fatal(err)
	}
	if currentOperation.Status.Phase != stationv1alpha1.StationRebindPhaseApplied {
		t.Fatalf("operation phase=%s, want Applied", currentOperation.Status.Phase)
	}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(audit.events) != 1 {
		t.Fatalf("terminal retry duplicated audit event: %d", len(audit.events))
	}
}

func TestRebindRejectsStaleNodeAndBindingVersion(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*stationv1alpha1.Station, *stationv1alpha1.StationRebind)
		target testRebindTarget
	}{
		{name: "target is current Node", mutate: func(_ *stationv1alpha1.Station, operation *stationv1alpha1.StationRebind) {
			operation.Spec.TargetNodeUID = "node-a-uid"
		}, target: testRebindTarget{uid: "node-a-uid", exists: true}},
		{name: "wrong target UID", target: testRebindTarget{uid: "replacement-uid", exists: true}},
		{name: "missing target", target: testRebindTarget{uid: "node-b-uid", exists: false}},
		{name: "stale binding version", mutate: func(station *stationv1alpha1.Station, _ *stationv1alpha1.StationRebind) {
			station.Spec.Binding.Version = 4
		}, target: testRebindTarget{uid: "node-b-uid", exists: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			station, operation := testRebindObjects()
			if test.mutate != nil {
				test.mutate(station, operation)
			}
			c := newTestClient(t, station, operation)
			audit := &testRebindAudit{}
			rc := newTestRebindReconciler(t, c, test.target, audit, func() time.Time { return now })
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
			if _, err := rc.Reconcile(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			updated := &stationv1alpha1.Station{}
			if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: station.Name}, updated); err != nil {
				t.Fatal(err)
			}
			if updated.Spec.Binding.NodeUID != "node-a-uid" {
				t.Fatalf("invalid rebind changed Station binding: %#v", updated.Spec.Binding)
			}
			currentOperation := &stationv1alpha1.StationRebind{}
			if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, currentOperation); err != nil {
				t.Fatal(err)
			}
			if currentOperation.Status.Phase != stationv1alpha1.StationRebindPhaseRejected {
				t.Fatalf("phase=%s, want Rejected", currentOperation.Status.Phase)
			}
			if len(audit.events) != 1 {
				t.Fatalf("rejected operation audit count=%d, want 1", len(audit.events))
			}
		})
	}
}

func TestRebindRejectsNodeUIDAlreadyOwnedByAnotherStation(t *testing.T) {
	station, operation := testRebindObjects()
	other := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-other", Namespace: testNamespace, UID: "station-other-uid"},
		Spec: stationv1alpha1.StationSpec{
			DisplayName: "Station 2", Sequence: 2,
			Binding: stationv1alpha1.StationBinding{NodeName: "node-c", NodeUID: "node-b-uid", Version: 1, BoundBy: "system"},
		},
	}
	c := newTestClient(t, station, operation, other)
	audit := &testRebindAudit{}
	reconciler := newTestRebindReconciler(t, c, testRebindTarget{uid: "node-b-uid", exists: true}, audit, time.Now)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	updated := &stationv1alpha1.Station{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: station.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Binding.NodeUID != "node-a-uid" {
		t.Fatalf("duplicate target rebind changed binding: %#v", updated.Spec.Binding)
	}
	currentOperation := &stationv1alpha1.StationRebind{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, currentOperation); err != nil {
		t.Fatal(err)
	}
	if currentOperation.Status.Phase != stationv1alpha1.StationRebindPhaseRejected {
		t.Fatalf("phase=%s, want Rejected", currentOperation.Status.Phase)
	}
	event := audit.events[string(operation.UID)]
	if event.Outcome != "Rejected" || event.ResultReason != "TargetNodeAlreadyBound" {
		t.Fatalf("missing duplicate binding audit: %#v", event)
	}
}

func TestRebindRejectsStationObjectUIDReplacement(t *testing.T) {
	now := time.Now().UTC()
	station, operation := testRebindObjects()
	station.UID = "replacement-station-uid"
	c := newTestClient(t, station, operation)
	audit := &testRebindAudit{}
	rc := newTestRebindReconciler(t, c, testRebindTarget{uid: "node-b-uid", exists: true}, audit, func() time.Time { return now })
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
	if _, err := rc.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	current := &stationv1alpha1.StationRebind{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != stationv1alpha1.StationRebindPhaseRejected {
		t.Fatalf("phase=%s, want Rejected", current.Status.Phase)
	}
	event := audit.events[string(operation.UID)]
	if event.StationUID != "station-uid" || event.ObservedStationUID != "replacement-station-uid" {
		t.Fatalf("rejection audit conflated requested and observed Station UIDs: %#v", event)
	}
}

func TestConcurrentDuplicateRebindAppliesAndAuditsOnce(t *testing.T) {
	now := time.Now().UTC()
	station, operation := testRebindObjects()
	c := newTestClient(t, station, operation)
	audit := &testRebindAudit{}
	target := testRebindTarget{uid: "node-b-uid", systemUUID: "system-b", exists: true}
	identityOps := NewSerializedIdentityOperations()
	first := newTestRebindReconciler(t, c, target, audit, func() time.Time { return now }, identityOps)
	second := newTestRebindReconciler(t, c, target, audit, func() time.Time { return now }, identityOps)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, rc := range []*RebindReconciler{first, second} {
		wg.Add(1)
		go func(rc *RebindReconciler) {
			defer wg.Done()
			_, err := rc.Reconcile(context.Background(), req)
			errs <- err
		}(rc)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Rebind reconcile failed: %v", err)
		}
	}
	updated := &stationv1alpha1.Station{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: station.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Binding.Version != 4 || updated.Spec.Binding.NodeUID != "node-b-uid" {
		t.Fatalf("concurrent rebind did not converge: %#v", updated.Spec.Binding)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit event count=%d, want one idempotent event", len(audit.events))
	}
	currentOperation := &stationv1alpha1.StationRebind{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: operation.Name}, currentOperation); err != nil {
		t.Fatal(err)
	}
	if currentOperation.Status.Phase != stationv1alpha1.StationRebindPhaseApplied {
		t.Fatalf("operation phase=%s, want Applied", currentOperation.Status.Phase)
	}
}

func TestConcurrentRebindsCannotClaimTheSameNodeUID(t *testing.T) {
	now := time.Now().UTC()
	stationA, operationA := testRebindObjects()
	stationB := &stationv1alpha1.Station{
		ObjectMeta: metav1.ObjectMeta{Name: "stn-b", Namespace: testNamespace, UID: "station-b-uid"},
		Spec: stationv1alpha1.StationSpec{
			DisplayName: "Station B", Sequence: 2,
			Binding: stationv1alpha1.StationBinding{NodeName: "node-b", NodeUID: "node-b-old-uid", Version: 1, BoundBy: "system"},
		},
		Status: stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseConflict},
	}
	operationB := &stationv1alpha1.StationRebind{
		ObjectMeta: metav1.ObjectMeta{Name: "rebind-req-2", Namespace: testNamespace, UID: "rebind-object-uid-2"},
		Spec: stationv1alpha1.StationRebindSpec{
			RequestID: "request-2", StationRef: stationv1alpha1.StationReference{Name: stationB.Name, UID: string(stationB.UID)},
			ExpectedBindingVersion: 1, TargetNodeName: "node-c", TargetNodeUID: "node-c-uid",
			Reason: "second replacement", RequestedBy: "alice",
		},
	}
	c := newTestClient(t, stationA, operationA, stationB, operationB)
	audit := &testRebindAudit{}
	target := testRebindTarget{uid: "node-c-uid", systemUUID: "system-c", exists: true}
	identityOps := NewSerializedIdentityOperations()
	first := newTestRebindReconciler(t, c, target, audit, func() time.Time { return now }, identityOps)
	second := newTestRebindReconciler(t, c, target, audit, func() time.Time { return now }, identityOps)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, item := range []struct {
		rc   *RebindReconciler
		name string
	}{{first, operationA.Name}, {second, operationB.Name}} {
		wg.Add(1)
		go func(rc *RebindReconciler, name string) {
			defer wg.Done()
			_, err := rc.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
			errs <- err
		}(item.rc, item.name)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	owners := 0
	for i := range stations.Items {
		if stations.Items[i].Spec.Binding.NodeUID == "node-c-uid" {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("Node UID has %d Station owners, want exactly one: %#v", owners, stations.Items)
	}
	if len(audit.events) != 2 {
		t.Fatalf("audit events=%d, want one result per operation", len(audit.events))
	}
}

func TestStationDiscoveryAndRebindShareIdentitySerialization(t *testing.T) {
	now := time.Now().UTC()
	station, operation := testRebindObjects()
	operation.Spec.TargetNodeName = "node-c"
	operation.Spec.TargetNodeUID = "node-c-uid"
	observation := testObservation("node-c", "node-c-uid", now)
	c := newTestClient(t, station, operation, observation)
	identityOps := NewSerializedIdentityOperations()
	audit := &testRebindAudit{}
	rebind := newTestRebindReconciler(t, c, testRebindTarget{uid: "node-c-uid", systemUUID: "system-a", exists: true}, audit, func() time.Time { return now }, identityOps)
	stationController := NewStationReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, identityOps, testNodeReadiness{ready: true}, DefaultOfflineAfter, func() time.Time { return now })
	rebindRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: operation.Name}}
	stationRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: observation.Name}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, reconcileErr := rebind.Reconcile(context.Background(), rebindRequest)
		errs <- reconcileErr
	}()
	go func() {
		defer wg.Done()
		_, reconcileErr := stationController.Reconcile(context.Background(), stationRequest)
		errs <- reconcileErr
	}()
	wg.Wait()
	close(errs)
	for reconcileErr := range errs {
		if reconcileErr != nil {
			t.Fatal(reconcileErr)
		}
	}
	var stations stationv1alpha1.StationList
	if err := c.List(context.Background(), &stations, client.InNamespace(testNamespace)); err != nil {
		t.Fatal(err)
	}
	owners := 0
	for i := range stations.Items {
		if stations.Items[i].Spec.Binding.NodeUID == "node-c-uid" {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("Station discovery/Rebind left %d owners for one Node UID: %#v", owners, stations.Items)
	}
}

func newTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	now := time.Now().UTC()
	_, c := newTestReconciler(t, &now, objects...)
	return c
}
