package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func TestSyncJobStatusSnapshotIsExactAndOrdered(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		Spec: rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "b"}, {Name: "a"}}},
		Status: rlarkv1alpha1.WorkflowStatus{Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "a", Phase: rlarkv1alpha1.JobPhaseSucceeded}, {Name: "removed", Phase: rlarkv1alpha1.JobPhaseFailed},
		}},
	}
	if !syncJobStatusSnapshot(wf) {
		t.Fatal("expected snapshot change")
	}
	if len(wf.Status.Jobs) != 2 || wf.Status.Jobs[0].Name != "b" || wf.Status.Jobs[1].Name != "a" || wf.Status.Jobs[1].Phase != rlarkv1alpha1.JobPhaseSucceeded {
		t.Fatalf("unexpected snapshot: %#v", wf.Status.Jobs)
	}
}

func TestReconcileJobAdoptsLegacyAndRejectsConflict(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")}}
	template := rlarkv1alpha1.WorkflowJobTemplate{Name: "train"}
	legacy := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "wf-train", Labels: map[string]string{workflowLabel: wf.Name}, Annotations: map[string]string{"user": "keep"}}}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(legacy).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	job, err := r.reconcileJob(context.Background(), wf, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if job.Annotations["user"] != "keep" || job.Annotations[utils.ParentUIDAnnotation] != string(wf.UID) || metav1.GetControllerOf(job).UID != wf.UID {
		t.Fatalf("legacy Job not adopted safely: %#v", job.ObjectMeta)
	}

	conflict := job.DeepCopy()
	conflict.Annotations[utils.ParentUIDAnnotation] = "other"
	if err := c.Update(context.Background(), conflict); err != nil {
		t.Fatal(err)
	}
	if _, err := r.reconcileJob(context.Background(), wf, template, logr.Discard()); err == nil {
		t.Fatal("expected conflicting UID to be rejected")
	}
}

func TestReconcileJobBackfillsMatchingOwnerReference(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")}}
	template := rlarkv1alpha1.WorkflowJobTemplate{Name: "train"}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "wf-train", Annotations: map[string]string{"user": "keep"}}}
	if err := setWorkflowOwner(wf, job); err != nil {
		t.Fatal(err)
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileJob(context.Background(), wf, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.Annotations[utils.ParentUIDAnnotation] != string(wf.UID) || got.Annotations[utils.ChildTemplateAnnotation] != template.Name || got.Annotations["user"] != "keep" {
		t.Fatalf("owner-only Job was not backfilled: %#v", got.Annotations)
	}
}

func TestReconcileJobSyncsMutableSpecAndPreservesTerminalJob(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")}}
	template := rlarkv1alpha1.WorkflowJobTemplate{Name: "train", Spec: rlarkv1alpha1.JobSpec{Domain: "new"}}
	for _, phase := range []rlarkv1alpha1.JobPhase{rlarkv1alpha1.JobPhaseRunning, rlarkv1alpha1.JobPhaseSucceeded} {
		t.Run(string(phase), func(t *testing.T) {
			job := buildJob(wf, template, "wf-train")
			job.Spec.Domain = "old"
			job.Status.Phase = phase
			job.Annotations["user"] = "keep"
			if err := setWorkflowOwner(wf, job); err != nil {
				t.Fatal(err)
			}
			scheme := workflowTestScheme(t)
			c := workflowTestClientBuilder(scheme).WithObjects(job).Build()
			r := &Reconciler{Client: c, Scheme: scheme}
			got, err := r.reconcileJob(context.Background(), wf, template, logr.Discard())
			if err != nil {
				t.Fatal(err)
			}
			want := "new"
			if phase == rlarkv1alpha1.JobPhaseSucceeded {
				want = "old"
			}
			if got.Spec.Domain != want || got.Annotations["user"] != "keep" {
				t.Fatalf("Job sync = domain %q, annotations %#v", got.Spec.Domain, got.Annotations)
			}
		})
	}
}

// A template that never produced a Job has nothing left to run, so a suspended
// Workflow owning no Jobs settles in one pass.
func TestSuspendedWorkflowWithoutJobsCompletesImmediately(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")},
		Spec: rlarkv1alpha1.WorkflowSpec{
			Suspended:    true,
			JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "train"}},
		},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseStopping, Jobs: []rlarkv1alpha1.WorkflowJobStatus{{Name: "train", Phase: rlarkv1alpha1.JobPhaseRunning}}},
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.ReconcileStateMachine(context.Background(), wf)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopped || wf.Status.Jobs[0].Phase != rlarkv1alpha1.JobPhaseStopped {
		t.Fatalf("stopped Workflow status not completed: %#v", wf.Status)
	}
}

// Suspending a Workflow is a durable desired-state switch, not a request to
// delete its children: the owned Jobs are asked to suspend themselves and must
// survive so a later resume can restart them.
//
// The stop must also never end in ErrRequeueAfterChildCleanup. A Job whose
// Tasks the Agent has not reported as stopped cannot finish deleting, so a stop
// that deleted its children left every reconcile awaiting cleanup and the
// shell never stamped Status.ObservedGeneration, which made the Workflow look
// like it had not observed a Spec it had in fact already acted on.
func TestSuspendedWorkflowStopsJobsWithoutDeletingThem(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid"), Generation: 3},
		Spec: rlarkv1alpha1.WorkflowSpec{
			Suspended:    true,
			JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "prepare"}, {Name: "train"}},
		},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseRunning, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "prepare", Phase: rlarkv1alpha1.JobPhaseSucceeded},
			{Name: "train", Phase: rlarkv1alpha1.JobPhaseRunning},
		}},
	}
	done := buildJob(wf, wf.Spec.JobTemplates[0], "wf-prepare")
	done.Status.Phase = rlarkv1alpha1.JobPhaseSucceeded
	job := buildJob(wf, wf.Spec.JobTemplates[1], "wf-train")
	job.Status.Phase = rlarkv1alpha1.JobPhaseRunning
	// The Agent has not reported this Job's Tasks as stopped, so deleting it
	// would only park it in Terminating behind its cleanup finalizer.
	job.Finalizers = []string{"example.com/agent-cleanup"}
	for _, j := range []*rlarkv1alpha1.Job{done, job} {
		if err := setWorkflowOwner(wf, j); err != nil {
			t.Fatal(err)
		}
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(done, job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.ReconcileStateMachine(context.Background(), wf)
	if err != nil {
		t.Fatalf("suspend reconcile returned an error: %v", err)
	}
	if !changed || wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopping {
		t.Fatalf("suspend did not start stopping: changed=%v status=%#v", changed, wf.Status)
	}
	if wf.Status.Jobs[1].Phase != rlarkv1alpha1.JobPhaseRunning {
		t.Fatalf("a Job that is still running was reported as stopped: %#v", wf.Status.Jobs)
	}
	var stored rlarkv1alpha1.Job
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), &stored); err != nil {
		t.Fatalf("suspended Workflow deleted the Job it must keep: %v", err)
	}
	if !stored.Spec.Suspended || !stored.DeletionTimestamp.IsZero() {
		t.Fatalf("Job was not asked to suspend itself in place: suspended=%v deletionTimestamp=%v", stored.Spec.Suspended, stored.DeletionTimestamp)
	}
	// A Job that already finished keeps its own outcome instead of being
	// restated as a stop.
	var storedDone rlarkv1alpha1.Job
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(done), &storedDone); err != nil {
		t.Fatal(err)
	}
	if storedDone.Spec.Suspended {
		t.Fatalf("a finished Job was suspended after the fact: %#v", storedDone.Spec)
	}

	// Repeating the pass while the Job is still coming to rest must stay a
	// clean reconcile, so the shell can record the observed generation.
	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatalf("waiting for a Job to stop returned an error: %v", err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopping {
		t.Fatalf("Workflow left Stopping before its Job stopped: %#v", wf.Status)
	}

	stored.Status.Phase = rlarkv1alpha1.JobPhaseStopped
	if err := c.Update(context.Background(), &stored); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseStopped || wf.Status.Jobs[1].Phase != rlarkv1alpha1.JobPhaseStopped {
		t.Fatalf("Workflow did not settle once its Job stopped: %#v", wf.Status)
	}
	if wf.Status.Jobs[0].Phase != rlarkv1alpha1.JobPhaseSucceeded {
		t.Fatalf("settled stop overwrote a finished Job status: %#v", wf.Status.Jobs)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), &rlarkv1alpha1.Job{}); err != nil {
		t.Fatalf("settled stop deleted the Job it must keep: %v", err)
	}
}

// Resuming withdraws the stop the same way it was applied: by reconciling the
// Job Spec back to what the template asks for, so no separate un-suspend path
// can be skipped or left behind.
func TestResumedWorkflowClearsJobSuspension(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")},
		Spec:       rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "train"}}},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseStopped, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "train", Phase: rlarkv1alpha1.JobPhaseStopped},
		}},
	}
	job := buildJob(wf, wf.Spec.JobTemplates[0], "wf-train")
	job.Spec.Suspended = true
	job.Status.Phase = rlarkv1alpha1.JobPhaseStopped
	if err := setWorkflowOwner(wf, job); err != nil {
		t.Fatal(err)
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	// The first pass resets the run, the second one dispatches the DAG again.
	for range 2 {
		if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
			t.Fatal(err)
		}
	}
	var stored rlarkv1alpha1.Job
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.Suspended {
		t.Fatalf("resumed Workflow left its Job suspended: %#v", stored.Spec)
	}
}

func TestWorkflowResumeRerunsDAGFromBeginning(t *testing.T) {
	oldStart := metav1.NewTime(time.Now().Add(-time.Hour))
	oldEnd := metav1.NewTime(time.Now().Add(-time.Minute))
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")},
		Spec: rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{
			{Name: "prepare"},
			{Name: "train", Dependencies: []string{"prepare"}},
		}},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseStopped, StartTime: &oldStart, EndTime: &oldEnd, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "prepare", Phase: rlarkv1alpha1.JobPhaseStopped},
			{Name: "train", Phase: rlarkv1alpha1.JobPhaseStopped},
		}},
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhasePending || len(wf.Status.Jobs) != 0 {
		t.Fatalf("resumed Workflow did not reset its run: %#v", wf.Status)
	}
	if wf.Status.StartTime == nil || !wf.Status.StartTime.After(oldStart.Time) || wf.Status.EndTime != nil {
		t.Fatalf("resumed Workflow did not reset run times: %#v", wf.Status)
	}
	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseRunning {
		t.Fatalf("second resume reconcile phase = %s, want Running", wf.Status.Phase)
	}
	var prepare rlarkv1alpha1.Job
	if err := c.Get(context.Background(), client.ObjectKey{Name: "wf-prepare"}, &prepare); err != nil {
		t.Fatalf("first DAG Job was not recreated: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "wf-train"}, &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
		t.Fatalf("dependent Job should wait for the new prepare run: %v", err)
	}
}

func TestWorkflowResumeWaitsForTerminatingJob(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")},
		Spec:       rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "prepare"}}},
		Status: rlarkv1alpha1.WorkflowStatus{
			Phase: rlarkv1alpha1.WorkflowPhaseRunning,
			Jobs:  []rlarkv1alpha1.WorkflowJobStatus{{Name: "prepare", Phase: rlarkv1alpha1.JobPhasePending}},
		},
	}
	job := buildJob(wf, wf.Spec.JobTemplates[0], "wf-prepare")
	deletedAt := metav1.Now()
	job.DeletionTimestamp = &deletedAt
	job.Finalizers = []string{"example.com/cleanup"}
	if err := setWorkflowOwner(wf, job); err != nil {
		t.Fatal(err)
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileJob(context.Background(), wf, wf.Spec.JobTemplates[0], logr.Discard()); !errors.Is(err, controller.ErrRequeueAfterChildCleanup) {
		t.Fatalf("resume with terminating Job error = %v, want cleanup requeue", err)
	}
	if wf.Status.Jobs[0].Phase != rlarkv1alpha1.JobPhasePending {
		t.Fatalf("terminating Job advanced Workflow status: %#v", wf.Status.Jobs)
	}
}

func TestStoppedWorkflowPreservesTerminalJobStatuses(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		Spec: rlarkv1alpha1.WorkflowSpec{Suspended: true, JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{
			{Name: "done"}, {Name: "failed"}, {Name: "running"},
		}},
		Status: rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseStopping, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
			{Name: "done", Phase: rlarkv1alpha1.JobPhaseSucceeded},
			{Name: "failed", Phase: rlarkv1alpha1.JobPhaseFailed},
			{Name: "running", Phase: rlarkv1alpha1.JobPhaseRunning},
		}},
	}
	r := &Reconciler{Client: workflowTestClientBuilder(workflowTestScheme(t)).Build()}

	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	if wf.Status.Jobs[0].Phase != rlarkv1alpha1.JobPhaseSucceeded ||
		wf.Status.Jobs[1].Phase != rlarkv1alpha1.JobPhaseFailed ||
		wf.Status.Jobs[2].Phase != rlarkv1alpha1.JobPhaseStopped {
		t.Fatalf("unexpected Job phases after stop: %#v", wf.Status.Jobs)
	}
}

func TestTerminalWorkflowReconcilePrunesWithoutDispatch(t *testing.T) {
	for _, phase := range []rlarkv1alpha1.WorkflowPhase{rlarkv1alpha1.WorkflowPhaseSucceeded, rlarkv1alpha1.WorkflowPhaseFailed} {
		t.Run(string(phase), func(t *testing.T) {
			wf := &rlarkv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")},
				Spec:       rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "kept"}, {Name: "added"}}},
				Status: rlarkv1alpha1.WorkflowStatus{Phase: phase, Jobs: []rlarkv1alpha1.WorkflowJobStatus{
					{Name: "removed", Phase: rlarkv1alpha1.JobPhaseSucceeded},
					{Name: "kept", Phase: rlarkv1alpha1.JobPhaseSucceeded},
				}},
			}
			removed := buildJob(wf, rlarkv1alpha1.WorkflowJobTemplate{Name: "removed"}, "wf-removed")
			removed.Status.Phase = rlarkv1alpha1.JobPhaseSucceeded
			if err := setWorkflowOwner(wf, removed); err != nil {
				t.Fatal(err)
			}
			kept := buildJob(wf, wf.Spec.JobTemplates[0], "wf-kept")
			kept.Status.Phase = rlarkv1alpha1.JobPhaseSucceeded
			if err := setWorkflowOwner(wf, kept); err != nil {
				t.Fatal(err)
			}
			scheme := workflowTestScheme(t)
			c := workflowTestClientBuilder(scheme).WithObjects(removed, kept).Build()
			r := &Reconciler{Client: c, Scheme: scheme}

			if r.IsTerminal(wf) {
				t.Fatal("Workflow must remain eligible for lifecycle reconciliation")
			}
			changed, err := r.ReconcileStateMachine(context.Background(), wf)
			if !errors.Is(err, controller.ErrRequeueAfterChildCleanup) {
				t.Fatalf("terminal reconciliation returned an FSM error: %v", err)
			}
			changedAgain, err := r.ReconcileStateMachine(context.Background(), wf)
			if err != nil {
				t.Fatalf("terminal reconciliation after pruning returned an FSM error: %v", err)
			}
			if !changed || wf.Status.Phase != phase {
				t.Fatalf("terminal status was not normalized safely: changed=%v status=%#v", changed, wf.Status)
			}
			if changedAgain {
				t.Fatal("stable terminal reconciliation unexpectedly changed status")
			}
			if len(wf.Status.Jobs) != 2 || wf.Status.Jobs[0].Name != "kept" || wf.Status.Jobs[1].Name != "added" {
				t.Fatalf("unexpected terminal status snapshot: %#v", wf.Status.Jobs)
			}
			if err := c.Get(context.Background(), client.ObjectKey{Name: removed.Name}, &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
				t.Fatalf("removed terminal child still exists: %v", err)
			}
			if err := c.Get(context.Background(), client.ObjectKey{Name: "wf-added"}, &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
				t.Fatalf("new template was dispatched for terminal Workflow: %v", err)
			}
		})
	}
}

func TestWorkflowReconcileUsesOldStatusToPruneSafely(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: "wf-uid"},
		Status:     rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseSucceeded, Jobs: []rlarkv1alpha1.WorkflowJobStatus{{Name: "removed"}}},
	}
	ownerOnly := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "legacy-name", Labels: map[string]string{workflowLabel: wf.Name}}}
	if err := setWorkflowOwner(wf, ownerOnly); err != nil {
		t.Fatal(err)
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(ownerOnly).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.ReconcileStateMachine(context.Background(), wf); !errors.Is(err, controller.ErrRequeueAfterChildCleanup) {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ownerOnly), &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owner-only removed Job was not pruned: %v", err)
	}
}

// A Job that only carries the Workflow label cannot be attributed to a removed
// template unless its name is one the Workflow itself recorded. Such a Job must
// neither be deleted on a name-prefix guess nor silently ignored: the reconcile
// is blocked so the ambiguity is reported instead of acted on.
func TestWorkflowReconcileRefusesUnattributableLabelOnlyJob(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: "wf-uid"},
		Status:     rlarkv1alpha1.WorkflowStatus{Phase: rlarkv1alpha1.WorkflowPhaseSucceeded, Jobs: []rlarkv1alpha1.WorkflowJobStatus{{Name: "removed"}}},
	}
	unrelated := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "wf-unrelated", Labels: map[string]string{workflowLabel: wf.Name}}}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(unrelated).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	_, err := r.ReconcileStateMachine(context.Background(), wf)
	if err == nil || errors.Is(err, controller.ErrRequeueAfterChildCleanup) {
		t.Fatalf("unattributable Job did not block the reconcile: %v", err)
	}
	if !strings.Contains(err.Error(), string(utils.ReasonUnprovenOwnership)) {
		t.Fatalf("reconcile was not blocked on unproven ownership: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(unrelated), &rlarkv1alpha1.Job{}); err != nil {
		t.Fatalf("unrelated label-only prefixed Job was touched: %v", err)
	}
}

func TestPruneJobsDeletesOnlyOwned(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid")}}
	owned := buildJob(wf, rlarkv1alpha1.WorkflowJobTemplate{Name: "old"}, "wf-old")
	if err := setWorkflowOwner(wf, owned); err != nil {
		t.Fatal(err)
	}
	conflict := owned.DeepCopy()
	conflict.Name = "wf-conflict"
	conflict.UID = ""
	conflict.OwnerReferences[0].UID = "other"
	conflict.Annotations[utils.ParentUIDAnnotation] = "other"
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(owned, conflict).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.pruneJobs(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: owned.Name}, &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owned removed Job still exists: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: conflict.Name}, &rlarkv1alpha1.Job{}); err != nil {
		t.Fatalf("conflicting Job was touched: %v", err)
	}
}

func TestPruneJobsDeletesOwnerOnlyRemovedJob(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: "wf-uid"}, Status: rlarkv1alpha1.WorkflowStatus{Jobs: []rlarkv1alpha1.WorkflowJobStatus{{Name: "removed"}}}}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "wf-removed", Labels: map[string]string{workflowLabel: wf.Name}}}
	if err := setWorkflowOwner(wf, job); err != nil {
		t.Fatal(err)
	}
	scheme := workflowTestScheme(t)
	c := workflowTestClientBuilder(scheme).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	waiting, err := r.pruneJobs(context.Background(), wf)
	if err != nil || !waiting {
		t.Fatalf("pruneJobs() = %v, %v", waiting, err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), &rlarkv1alpha1.Job{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owner-only removed Job still exists: %v", err)
	}
}

func TestInvalidDAGFailsWorkflowWithoutRequeue(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid"), Generation: 3},
		Spec: rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{
			{Name: "train", Dependencies: []string{"missing"}},
		}},
	}
	scheme := workflowTestScheme(t)
	r := &Reconciler{Client: workflowTestClientBuilder(scheme).Build(), Scheme: scheme}

	// An invalid Spec is not repairable by retrying, so the reconciler must not
	// return an error (which would requeue forever) and must instead record the
	// conclusion in the Validated condition.
	changed, err := r.ReconcileStateMachine(context.Background(), wf)
	if err != nil {
		t.Fatalf("invalid DAG returned error: %v", err)
	}
	if !changed || wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseFailed || wf.Status.EndTime == nil {
		t.Fatalf("invalid DAG did not fail the Workflow: changed=%v status=%#v", changed, wf.Status)
	}
	cond := apimeta.FindStatusCondition(wf.Status.Conditions, workflowConditionValidated)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != workflowReasonInvalidDAG {
		t.Fatalf("unexpected Validated condition: %#v", wf.Status.Conditions)
	}
	// Only the condition's ObservedGeneration is asserted here. Status.
	// ObservedGeneration is now stamped by the shell after the state machine
	// returns, and this test calls the state machine directly; the shell's
	// contract is covered in pkg/controllermanager/controller.
	if cond.ObservedGeneration != wf.Generation {
		t.Fatalf("stale condition observedGeneration: got=%d want=%d", cond.ObservedGeneration, wf.Generation)
	}
	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatalf("second reconcile of invalid DAG returned error: %v", err)
	}
}

func TestValidSpecClearsStaleValidatedCondition(t *testing.T) {
	wf := &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", UID: types.UID("wf-uid"), Generation: 4},
		Spec:       rlarkv1alpha1.WorkflowSpec{JobTemplates: []rlarkv1alpha1.WorkflowJobTemplate{{Name: "train"}}},
		Status: rlarkv1alpha1.WorkflowStatus{
			Conditions: []metav1.Condition{{
				Type:               workflowConditionValidated,
				Status:             metav1.ConditionFalse,
				Reason:             workflowReasonInvalidDAG,
				ObservedGeneration: 3,
			}},
		},
	}
	scheme := workflowTestScheme(t)
	r := &Reconciler{Client: workflowTestClientBuilder(scheme).Build(), Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(wf.Status.Conditions, workflowConditionValidated)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != workflowReasonSpecValidated {
		t.Fatalf("stale Validated=False was not cleared: %#v", wf.Status.Conditions)
	}
	if cond.ObservedGeneration != wf.Generation {
		t.Fatalf("Validated condition observedGeneration = %d, want %d", cond.ObservedGeneration, wf.Generation)
	}
}

func setWorkflowOwner(wf *rlarkv1alpha1.Workflow, job *rlarkv1alpha1.Job) error {
	controller := true
	job.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Workflow", Name: wf.Name, UID: wf.UID, Controller: &controller}}
	return nil
}
