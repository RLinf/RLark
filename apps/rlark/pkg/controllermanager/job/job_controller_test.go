package job

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func jobTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestReconcileAddsCleanupFinalizer(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	c := fake.NewClientBuilder().WithScheme(jobTestScheme(t)).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: jobTestScheme(t)}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if !contains(job.Finalizers, CleanupFinalizer) {
		t.Fatalf("cleanup finalizer not added: %v", job.Finalizers)
	}
}

func TestDeleteOwnedTasksWaitsForCleanupAndFiltersOwnerUID(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	owned := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name:       "owned",
		Namespace:  "workers",
		Finalizers: []string{"rlark.io/agent-cleanup"},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
			Controller: ptrBool(true),
		}},
	}}
	unrelated := owned.DeepCopy()
	unrelated.Name = "unrelated"
	unrelated.UID = types.UID("unrelated")
	unrelated.OwnerReferences[0].UID = types.UID("other-job")
	c := fake.NewClientBuilder().WithScheme(jobTestScheme(t)).WithObjects(owned, unrelated).Build()
	r := &Reconciler{Client: c, Scheme: jobTestScheme(t)}

	pending, err := r.deleteOwnedTasks(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("owned Task should keep Job cleanup pending")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(owned), owned); err != nil {
		t.Fatal(err)
	}
	if owned.DeletionTimestamp.IsZero() {
		t.Fatal("owned Task deletion was not requested")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(unrelated), unrelated); err != nil {
		t.Fatalf("unrelated Task should not be deleted: %v", err)
	}
}

func TestStoppedJobDeletesTasksAndWaitsForCleanup(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Stopped: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}}},
	}
	owned := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "job-worker",
			Namespace:  "default",
			Finalizers: []string{"rlark.io/agent-cleanup"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
				Controller: ptrBool(true),
			}},
		},
		Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseRunning},
	}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), job); err == nil {
		t.Fatal("Task cleanup should requeue the Job")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(owned), owned); err != nil {
		t.Fatal(err)
	}
	if owned.DeletionTimestamp.IsZero() {
		t.Fatal("Task deletion was not requested")
	}
}

func TestStoppedInvalidJobDeletesTasksAndWaitsForCleanup(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec: rlarkv1alpha1.JobSpec{
			Stopped: true,
			Tasks: []rlarkv1alpha1.JobTaskTemplate{
				{Name: "head", Head: true},
				{Name: "another-head", Head: true},
			},
		},
		Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning},
	}
	owned := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "job-head", Namespace: "default", Finalizers: []string{"rlark.io/agent-cleanup"},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
			Controller: ptrBool(true),
		}},
	}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), job); err == nil {
		t.Fatal("Task cleanup should requeue the invalid stopped Job")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(owned), owned); err != nil {
		t.Fatal(err)
	}
	if owned.DeletionTimestamp.IsZero() {
		t.Fatal("Task deletion was not requested")
	}
	if job.Status.Phase == rlarkv1alpha1.JobPhaseFailed {
		t.Fatal("stopped Job should not be marked invalid before cleanup")
	}
}

func TestStoppedJobCompletesAfterTasksDisappear(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Stopped: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}}},
	}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.ReconcileStateMachine(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("stopped Job status not completed: %#v", job.Status)
	}
}

func TestStoppedJobPreservesTerminalTaskStatuses(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		Spec: rlarkv1alpha1.JobSpec{Stopped: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{
			{Name: "done"}, {Name: "failed"}, {Name: "running"},
		}},
		Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{
			{Name: "done", Phase: rlarkv1alpha1.TaskPhaseSucceeded},
			{Name: "failed", Phase: rlarkv1alpha1.TaskPhaseFailed},
			{Name: "running", Phase: rlarkv1alpha1.TaskPhaseRunning},
		}},
	}
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(jobTestScheme(t)).Build()}

	if _, err := r.ReconcileStateMachine(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseSucceeded ||
		job.Status.Tasks[1].Phase != rlarkv1alpha1.TaskPhaseFailed ||
		job.Status.Tasks[2].Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("unexpected Task phases after stop: %#v", job.Status.Tasks)
	}
}

func TestDeletingJobUpdatesStoppedStatusBeforeDeletingTasks(t *testing.T) {
	deletedAt := metav1.Now()
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "job",
			UID:               types.UID("job-uid"),
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{CleanupFinalizer},
		},
		Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status: rlarkv1alpha1.JobStatus{
			Phase: rlarkv1alpha1.JobPhaseRunning,
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}},
		},
	}
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "job-worker",
			Namespace:  "default",
			Finalizers: []string{"rlark.io/agent-cleanup"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
				Controller: ptrBool(true),
			}},
		},
		Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseStopped},
	}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(job).WithObjects(job, task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || job.Status.EndTime == nil {
		t.Fatalf("deleting Job status was not stopped: %#v", job.Status)
	}
	if job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("Task status was not aggregated: %#v", job.Status.Tasks)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), task); err != nil {
		t.Fatal(err)
	}
	if !task.DeletionTimestamp.IsZero() {
		t.Fatal("Task must remain until the stopped Job status is persisted")
	}
}

func TestDeletingStoppedJobAddsMissingEndTime(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status: rlarkv1alpha1.JobStatus{
			Phase: rlarkv1alpha1.JobPhaseStopped,
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseStopped}},
		},
	}
	if !jobTaskStatusesStopped(job) {
		t.Fatal("complete stopped Task statuses should not require resync")
	}
	if job.Status.EndTime != nil {
		t.Fatal("test requires a missing endTime")
	}
}

func TestDeletingTerminalJobPreservesPhaseAndEndTime(t *testing.T) {
	for _, phase := range []rlarkv1alpha1.JobPhase{
		rlarkv1alpha1.JobPhaseSucceeded,
		rlarkv1alpha1.JobPhaseFailed,
	} {
		t.Run(string(phase), func(t *testing.T) {
			deletedAt := metav1.Now()
			endTime := metav1.NewTime(time.Now().Truncate(time.Second))
			job := &rlarkv1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job",
					UID:               types.UID("job-uid"),
					DeletionTimestamp: &deletedAt,
					Finalizers:        []string{CleanupFinalizer, "test/finalizer"},
				},
				Status: rlarkv1alpha1.JobStatus{Phase: phase, EndTime: &endTime},
			}
			scheme := jobTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(job).WithObjects(job).Build()
			r := &Reconciler{Client: c, Scheme: scheme}

			if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
				t.Fatal(err)
			}
			if job.Status.Phase != phase || !job.Status.EndTime.Time.Equal(endTime.Time) {
				t.Fatalf("terminal status changed: %#v", job.Status)
			}
		})
	}
}

func ptrBool(v bool) *bool { return &v }

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func TestJobSpecChangedPredicateIgnoresStatusOnlyUpdate(t *testing.T) {
	p := jobSpecChangedPredicate()
	oldJob := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Generation: 1}}
	newJob := oldJob.DeepCopy()
	newJob.Status.Phase = rlarkv1alpha1.JobPhaseRunning
	if p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}) {
		t.Fatal("status-only update should not enqueue the Job")
	}

	newJob = oldJob.DeepCopy()
	newJob.Generation = 2
	if !p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}) {
		t.Fatal("generation update should enqueue the Job")
	}
}
