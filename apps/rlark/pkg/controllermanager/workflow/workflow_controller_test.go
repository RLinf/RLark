package workflow

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func workflowTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestReconcileAddsCleanupFinalizer(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "workflow", UID: types.UID("workflow-uid")}}
	scheme := workflowTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wf).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: wf.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: wf.Name}, wf); err != nil {
		t.Fatal(err)
	}
	if !hasFinalizer(wf.Finalizers, CleanupFinalizer) {
		t.Fatalf("cleanup finalizer not added: %v", wf.Finalizers)
	}
}

func TestDeleteOwnedJobsWaitsForCleanupAndFiltersOwnerUID(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "workflow", UID: types.UID("workflow-uid")}}
	owned := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:       "owned",
		Finalizers: []string{"jobs.rlinf.io/task-cleanup"},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Workflow", Name: wf.Name, UID: wf.UID,
			Controller: boolPtr(true),
		}},
	}}
	unrelated := owned.DeepCopy()
	unrelated.Name = "unrelated"
	unrelated.UID = types.UID("unrelated")
	unrelated.OwnerReferences[0].UID = types.UID("other-workflow")
	scheme := workflowTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned, unrelated).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	pending, err := r.deleteOwnedJobs(context.Background(), wf)
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("owned Job should keep Workflow cleanup pending")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(owned), owned); err != nil {
		t.Fatal(err)
	}
	if owned.DeletionTimestamp.IsZero() {
		t.Fatal("owned Job deletion was not requested")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(unrelated), unrelated); err != nil {
		t.Fatalf("unrelated Job should not be deleted: %v", err)
	}
}

func TestStoppedWorkflowDeletesAllJobsIncludingTerminal(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "workflow", UID: types.UID("workflow-uid")},
		Spec: rlarkv1alpha1.WorkflowSpec{Stopped: true, JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{
			{Name: "running"}, {Name: "succeeded"},
		}},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseRunning, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "running", Phase: rlarkv1alpha1.JobPhaseRunning}, {Name: "succeeded", Phase: rlarkv1alpha1.JobPhaseSucceeded},
		}},
	}
	owned := func(name string, phase rlarkv1alpha1.JobPhase, stopped bool) *rlarkv1alpha1.Job {
		return &rlarkv1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Finalizers: []string{"jobs.rlinf.io/task-cleanup"}, OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Workflow", Name: wf.Name, UID: wf.UID, Controller: boolPtr(true),
			}}},
			Spec: rlarkv1alpha1.JobSpec{Stopped: stopped}, Status: rlarkv1alpha1.JobStatus{Phase: phase},
		}
	}
	running := owned("running", rlarkv1alpha1.JobPhaseRunning, false)
	succeeded := owned("succeeded", rlarkv1alpha1.JobPhaseSucceeded, false)
	scheme := workflowTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(running, succeeded).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), wf); err == nil {
		t.Fatal("Job cleanup should requeue the Workflow")
	}
	for _, job := range []*rlarkv1alpha1.Job{running, succeeded} {
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), job); err != nil {
			t.Fatal(err)
		}
		if job.DeletionTimestamp.IsZero() {
			t.Fatalf("Job %s deletion was not requested", job.Name)
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func hasFinalizer(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
