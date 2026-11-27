package controller

import (
	"context"
	"errors"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type memberClientStub struct {
	client  kubernetes.Interface
	cluster string
}

type joinedReadinessStub struct {
	ready   bool
	err     error
	nodeUID string
}

func (s *joinedReadinessStub) Ready(_ context.Context, _, _, _, uid string) (bool, error) {
	s.nodeUID = uid
	return s.ready, s.err
}

func (s *memberClientStub) Client(_ context.Context, cluster string) (kubernetes.Interface, error) {
	s.cluster = cluster
	return s.client, nil
}

func TestNodeLifecycleExecutorDrain(t *testing.T) {
	member := &memberClientStub{client: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "uid"}})}
	executor := &NodeLifecycleExecutor{Members: member, RemovalReady: func(context.Context, *stationv1alpha1.NodeOperation) error { return nil }}
	op := &stationv1alpha1.NodeOperation{Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster", NodeName: "worker", NodeUID: "uid"}}
	done, uid, err := executor.Execute(context.Background(), op, "Drain")
	if err != nil || !done || uid != "uid" || member.cluster != "cluster" {
		t.Fatalf("drain: %v %q %v", done, uid, err)
	}
}

func TestNodeLifecycleExecutorVerifyJoinedNode(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "uid"}}
	member := &memberClientStub{client: fake.NewSimpleClientset(node)}
	executor := &NodeLifecycleExecutor{Members: member}
	op := &stationv1alpha1.NodeOperation{Spec: stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "cluster", NodeName: "worker"}, Status: stationv1alpha1.NodeOperationStatus{NodeUID: "uid"}}
	if done, _, err := executor.Execute(ctx, op, "VerifyNode"); err != nil || done {
		t.Fatalf("unready: %v %v", done, err)
	}
	node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	if _, err := member.client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if done, _, err := executor.Execute(ctx, op, "VerifyNode"); err != nil || !done {
		t.Fatalf("ready: %v %v", done, err)
	}
	op.Status.NodeUID = "old"
	if _, _, err := executor.Execute(ctx, op, "VerifyNode"); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("replaced: %v", err)
	}
}

func TestNodeLifecycleExecutorVerifyRLarkRequiresCurrentNodeIdentity(t *testing.T) {
	ctx := context.Background()
	member := &memberClientStub{client: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "uid", Labels: map[string]string{"rlark.io/enabled": "true", "rlark.io/containerd": "containerd", "rlark.io/cluster-id": "cluster"}}})}
	readiness := &joinedReadinessStub{}
	executor := &NodeLifecycleExecutor{Members: member, JoinedReadiness: readiness}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: "agent"}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Join", ClusterID: "cluster", NodeName: "worker"}, Status: stationv1alpha1.NodeOperationStatus{NodeUID: "uid"}}
	if done, _, err := executor.Execute(ctx, op, "VerifyRLark"); err != nil || done || readiness.nodeUID != "uid" {
		t.Fatalf("pending: done=%v err=%v uid=%q", done, err, readiness.nodeUID)
	}
	readiness.ready = true
	if done, uid, err := executor.Execute(ctx, op, "VerifyRLark"); err != nil || !done || uid != "uid" {
		t.Fatalf("ready: done=%v uid=%q err=%v", done, uid, err)
	}
	op.Status.NodeUID = "old"
	readiness.nodeUID = ""
	if _, _, err := executor.Execute(ctx, op, "VerifyRLark"); !errors.Is(err, ErrNodeIdentityChanged) || readiness.nodeUID != "" {
		t.Fatalf("replaced: %v", err)
	}
}

func TestNodeLifecycleExecutorDeleteWaitsAndRejectsReplacement(t *testing.T) {
	ctx := context.Background()
	member := &memberClientStub{client: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "uid"}})}
	listed := &unstructured.Unstructured{}
	listed.SetGroupVersionKind(rlarkNodeGVK)
	listed.SetNamespace(testNamespace)
	listed.SetName("worker")
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	management := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(listed).Build()
	executor := &NodeLifecycleExecutor{Members: member, Management: management}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster", NodeName: "worker", NodeUID: "uid"}}
	if done, _, err := executor.Execute(ctx, op, "DeleteNode"); err != nil || done {
		t.Fatalf("delete must wait: done=%v err=%v", done, err)
	}
	// The member Node is gone; the management plane's Node of it goes after it.
	if done, _, err := executor.Execute(ctx, op, "DeleteNode"); err != nil || done {
		t.Fatalf("the management plane's Node: done=%v err=%v", done, err)
	}
	if err := management.Get(ctx, client.ObjectKeyFromObject(listed), listed.DeepCopy()); !apierrors.IsNotFound(err) {
		t.Fatalf("the management plane's Node was not deleted: %v", err)
	}
	for _, stage := range []string{"DeleteNode", "VerifyRemoval"} {
		if done, _, err := executor.Execute(ctx, op, stage); err != nil || !done {
			t.Fatalf("%s of a deleted node: done=%v err=%v", stage, done, err)
		}
	}
	if _, err := member.client.CoreV1().Nodes().Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "new"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executor.Execute(ctx, op, "VerifyRemoval"); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("replacement: %v", err)
	}
}
