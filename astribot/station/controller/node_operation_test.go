package controller

import (
	"context"
	"errors"
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

func TestNodeOperationResumeAndFailure(t *testing.T) {
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
	stub.err = nil
	stub.uid = "node-uid"
	for i := 0; i < 8; i++ {
		if _, err := newReconciler().Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(op), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != "Succeeded" || stored.Status.CompletedAt == nil {
		t.Fatalf("not complete: %+v", stored.Status)
	}
	if strings.Join(stub.calls, " ") != "Preflight Uplink "+strings.Join(installerStages[1:], " ") {
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
		err    error
		reason string
	}{
		{ErrNodeIdentityChanged, "NodeIdentityChanged"},
		{ErrNodeDrainBlocked, "DrainBlocked"},
		{ErrNodeStageUnavailable, "PrerequisiteUnavailable"},
		{errors.New("remote output includes sensitive internal detail"), "StageFailed"},
	} {
		reason, message := nodeOperationFailureCondition("Drain", tc.err)
		if reason != tc.reason || strings.Contains(message, "sensitive internal detail") {
			t.Fatalf("reason=%q message=%q", reason, message)
		}
	}
}
