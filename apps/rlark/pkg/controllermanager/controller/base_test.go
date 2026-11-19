package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

type cleanupWaitReconciler struct {
	client.Client
}

func (r *cleanupWaitReconciler) ReconcileStateMachine(context.Context, client.Object) (bool, error) {
	return false, ErrRequeueAfterChildCleanup
}

func (*cleanupWaitReconciler) IsTerminal(client.Object) bool { return false }

func TestReconcileWithRequeuesChildCleanupWithoutError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "workflow"}}
	r := &cleanupWaitReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(wf).Build()}

	result, err := ReconcileWith(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(wf)}, &rlarkv1alpha1.Workflow{}, "workflow", r)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != ChildCleanupRequeue {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, ChildCleanupRequeue)
	}
}
