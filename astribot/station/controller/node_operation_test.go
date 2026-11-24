package controller

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type nodeExecutorStub struct {
	calls []string
	done  bool
	uid   string
	err   error
}

func (s *nodeExecutorStub) Execute(_ context.Context, _ *stationv1alpha1.NodeOperation, stage string) (bool, string, error) {
	s.calls = append(s.calls, stage)
	return s.done, s.uid, s.err
}

func TestNodeOperationFailureIsTerminalUntilANewOperation(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: testNamespace, UID: "op-uid", Generation: 1}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "cluster-a", RequestID: "request", RequestedBy: "admin"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(op).WithObjects(op).Build()
	stub := &nodeExecutorStub{done: true}
	newReconciler := func() *NodeOperationReconciler {
		r := NewNodeOperationReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, stub)
		return r
	}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "op"}}
	if _, err := newReconciler().Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	stub.err = errors.New("executor unavailable")
	if _, err := newReconciler().Reconcile(ctx, req); err == nil {
		t.Fatal("expected error")
	}
	stored := &stationv1alpha1.NodeOperation{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(op), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Stage != "Preflight" {
		t.Fatalf("failure advanced stage: %+v", stored.Status)
	}
	if stored.Status.Phase != "Blocked" || len(stored.Status.Conditions) != 1 || stored.Status.Conditions[0].Reason != "StageFailed" {
		t.Fatalf("failure not recorded: %+v", stored.Status)
	}
	// A blocked operation is terminal: a later reconcile must not retry its
	// stage, or a second removal could race the same machine.
	stub.err = nil
	stub.uid = "node-uid"
	calls := len(stub.calls)
	for i := 0; i < 3; i++ {
		if _, err := newReconciler().Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	if len(stub.calls) != calls {
		t.Fatalf("a blocked operation was retried: %v", stub.calls)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(op), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != "Blocked" {
		t.Fatalf("blocked operation changed phase: %+v", stored.Status)
	}

	// The administrator's retry is a new operation with a new request ID.
	op2 := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Name: "op-2", Namespace: testNamespace, UID: "op-uid-2", Generation: 1}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "cluster-a", RequestID: "request-2", RequestedBy: "admin"}}
	if err := c.Create(ctx, op2); err != nil {
		t.Fatal(err)
	}
	req2 := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "op-2"}}
	for i := 0; i < 20; i++ {
		if _, err := newReconciler().Reconcile(ctx, req2); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(op2), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != "Succeeded" || stored.Status.CompletedAt == nil {
		t.Fatalf("not complete: %+v", stored.Status)
	}
	if strings.Join(stub.calls, " ") != "Preflight Uplink "+strings.Join(installerStages, " ") {
		t.Fatalf("unexpected calls: %v", stub.calls)
	}
}

func TestNodeOperationRejectsWrongClusterAndNodeUID(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = stationv1alpha1.AddToScheme(scheme)
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: testNamespace, UID: "op-uid"}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "other", NodeUID: "old"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(op).WithObjects(op).Build()
	stub := &nodeExecutorStub{done: true, uid: "new"}
	r := NewNodeOperationReconciler(c, c, testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}}, stub)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "op"}}
	if _, err := r.Reconcile(ctx, req); !errors.Is(err, ErrInvalidRegistration) || len(stub.calls) != 0 {
		t.Fatalf("cluster validation: %v, %v", err, stub.calls)
	}
	stored := &stationv1alpha1.NodeOperation{}
	_ = c.Get(ctx, client.ObjectKeyFromObject(op), stored)
	stored.Spec.ClusterID = "cluster-a"
	if err := c.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, req); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("UID mismatch: %v", err)
	}
}

func TestNodeOperationFailureConditionsAreStableAndDoNotExposeInternalErrors(t *testing.T) {
	for _, tc := range []struct {
		stage  string
		err    error
		reason string
	}{
		{stage: "Drain", err: ErrNodeIdentityChanged, reason: "NodeIdentityChanged"},
		{stage: "Drain", err: ErrNodeDrainBlocked, reason: "DrainBlocked"},
		{stage: "Drain", err: ErrNodeStageUnavailable, reason: "PrerequisiteUnavailable"},
		{stage: "Drain", err: errors.New("remote output includes sensitive internal detail"), reason: "StageFailed"},
		// A removal stage waits on the member API; a timeout there is an
		// unreachable cluster, not an unexplained stage failure.
		{stage: "Reset", err: context.DeadlineExceeded, reason: "MemberClusterUnreachable"},
		{stage: "DeleteNode", err: &net.DNSError{IsTimeout: true, Name: "member.example"}, reason: "MemberClusterUnreachable"},
		// An installer stage uses SSH; the same context error is not a
		// member-cluster diagnosis.
		{stage: "agent", err: context.DeadlineExceeded, reason: "StageFailed"},
	} {
		reason, message := nodeOperationFailureCondition(tc.stage, tc.err)
		if reason != tc.reason || strings.Contains(message, "sensitive internal detail") {
			t.Fatalf("stage=%q reason=%q message=%q", tc.stage, reason, message)
		}
		if tc.reason == "MemberClusterUnreachable" && !strings.Contains(message, "member cluster API") {
			t.Fatalf("member-cluster message does not name the cause: %q", message)
		}
	}
}
