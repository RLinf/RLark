package archive

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

func newWorkflowArchiveReconciler(t *testing.T, ttl time.Duration, objs ...client.Object) *Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return NewReconciler(c, scheme, &WorkflowResource{}, 1, ttl, controller.NewShell(nil, nil))
}

func stoppedWorkflow(name string, endTime time.Time) *rlarkv1alpha1.Workflow {
	end := metav1.NewTime(endTime)
	return &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: rlarkv1alpha1.WorkflowStatus{
			Phase:   rlarkv1alpha1.WorkflowPhaseStopped,
			EndTime: &end,
		},
	}
}

func workflowExists(t *testing.T, r *Reconciler, obj client.Object) bool {
	t.Helper()
	err := r.Get(context.Background(), client.ObjectKeyFromObject(obj), &rlarkv1alpha1.Workflow{})
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	t.Fatal(err)
	return false
}

// Succeeded and Failed are terminal for the state machine but are not
// archivable: the outcome stays readable, and deleting the Workflow would
// cascade onto the Jobs that carry it.
func TestWorkflowIsTerminalOnlyForStopped(t *testing.T) {
	res := &WorkflowResource{}
	tests := []struct {
		phase rlarkv1alpha1.WorkflowPhase
		want  bool
	}{
		{rlarkv1alpha1.WorkflowPhasePending, false},
		{rlarkv1alpha1.WorkflowPhaseRunning, false},
		{rlarkv1alpha1.WorkflowPhaseStopping, false},
		{rlarkv1alpha1.WorkflowPhaseStopped, true},
		{rlarkv1alpha1.WorkflowPhaseSucceeded, false},
		{rlarkv1alpha1.WorkflowPhaseFailed, false},
		{"", false},
	}
	for _, tt := range tests {
		workflow := &rlarkv1alpha1.Workflow{Status: rlarkv1alpha1.WorkflowStatus{Phase: tt.phase}}
		if got := res.IsTerminal(workflow); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
	if res.IsTerminal(&rlarkv1alpha1.Job{}) {
		t.Error("IsTerminal accepted an object of another kind")
	}
}

// A stopped Workflow with no recorded end time has no TTL origin. Archival
// deletes the object, so the reconciler must not treat a missing timestamp as
// "TTL already elapsed"; it waits a full TTL instead, which both protects the
// object and keeps it from accumulating forever.
func TestWorkflowWithoutEndTimeWaitsAFullTTL(t *testing.T) {
	ttl := time.Hour
	workflow := stoppedWorkflow("no-end-time", time.Now())
	workflow.Status.EndTime = nil
	r := newWorkflowArchiveReconciler(t, ttl, workflow)

	if got := (&WorkflowResource{}).GetEndTime(workflow); got != nil {
		t.Fatalf("GetEndTime = %v, want nil", got)
	}
	result, err := r.Reconcile(context.Background(), requestFor(workflow))
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter < ttl {
		t.Fatalf("RequeueAfter = %v, want >= %v", result.RequeueAfter, ttl)
	}
	if !workflowExists(t, r, workflow) {
		t.Fatal("workflow with no end time was archived immediately")
	}
}

func TestWorkflowArchiveWaitsForTTLThenDeletes(t *testing.T) {
	ttl := time.Hour
	endTime := time.Now()
	waiting := stoppedWorkflow("waiting", endTime)
	r := newWorkflowArchiveReconciler(t, ttl, waiting)

	result, err := r.Reconcile(context.Background(), requestFor(waiting))
	if err != nil {
		t.Fatal(err)
	}
	remaining := ttl - time.Since(endTime)
	if result.RequeueAfter < remaining {
		t.Fatalf("RequeueAfter = %v, want >= %v (jitter must not fire early)", result.RequeueAfter, remaining)
	}
	if !workflowExists(t, r, waiting) {
		t.Fatal("workflow was archived before its TTL elapsed")
	}

	elapsed := stoppedWorkflow("elapsed", time.Now().Add(-10*time.Minute))
	r = newWorkflowArchiveReconciler(t, 5*time.Minute, elapsed)
	if _, err := r.Reconcile(context.Background(), requestFor(elapsed)); err != nil {
		t.Fatal(err)
	}
	if workflowExists(t, r, elapsed) {
		t.Fatal("workflow still present after its TTL elapsed")
	}
}

// The manager wires every registered resource, so registration is what makes
// the archive controller exist at all.
func TestDefaultRegistryCoversJobAndWorkflow(t *testing.T) {
	kinds := map[string]bool{}
	for _, res := range DefaultRegistry.Resources() {
		if kinds[res.Kind()] {
			t.Fatalf("resource %q registered twice", res.Kind())
		}
		kinds[res.Kind()] = true
	}
	for _, want := range []string{"Job", "Workflow"} {
		if !kinds[want] {
			t.Errorf("%q is not registered for archiving", want)
		}
	}
	if kinds["Task"] {
		t.Error("Task must not be archived on a TTL; the Job reconciler recreates it")
	}
}
