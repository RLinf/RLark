package job

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func validJobTaskTemplate(name string) rlarkv1alpha1.JobTaskTemplate {
	return rlarkv1alpha1.JobTaskTemplate{
		Name: name,
		TaskSpec: rlarkv1alpha1.TaskSpec{
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: "custom-runtime"},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:    "custom-workload",
				Version: "v1",
				Carrier: rlarkv1alpha1.TaskWorkloadCarrier{Type: "custom-carrier", Version: "v1"},
				Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "main", Image: "example:v1"},
				}}},
			},
		},
	}
}

func jobTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

// jobTestClientBuilder returns a fake client builder with the Task index the
// Job controller relies on registered, mirroring what the manager does in
// SetupWithManager.
func jobTestClientBuilder(scheme *runtime.Scheme) *fake.ClientBuilder {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&rlarkv1alpha1.Task{}, taskJobField, indexTaskByJob)
}

func TestReconcileAddsCleanupFinalizer(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(job).Build()
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

func TestInvalidTaskTemplatePreventsAllTaskDispatch(t *testing.T) {
	valid := validJobTaskTemplate("valid")
	invalid := validJobTaskTemplate("invalid")
	invalid.Runtime.Kind = ""
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid"), Generation: 4},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{valid, invalid}},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.ReconcileStateMachine(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || job.Status.Phase != rlarkv1alpha1.JobPhaseFailed {
		t.Fatalf("invalid Job was not failed: %#v", job.Status)
	}
	condition := findCondition(job.Status.Conditions, jobConditionValidated)
	if condition == nil || condition.Status != metav1.ConditionFalse ||
		condition.ObservedGeneration != job.Generation ||
		condition.Message != "spec.tasks[1].runtime.kind must not be empty" {
		t.Fatalf("unexpected validation condition: %#v", condition)
	}
	var tasks rlarkv1alpha1.TaskList
	if err := c.List(context.Background(), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks.Items) != 0 {
		t.Fatalf("invalid Job dispatched Tasks: %#v", tasks.Items)
	}
}

func TestClusterDrainAndJobReconcileDoNotFlipTaskSuspended(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{validJobTaskTemplate("worker")}},
	}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	task.Annotations = map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true"}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], task.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended {
		t.Fatalf("Job controller did not apply the cluster drain stop request: %#v", got.Spec)
	}
	generation := got.Generation

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], got.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(&got), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended || got.Generation != generation {
		t.Fatalf("Job reconciliation flipped or rewrote drained Task: generation %d -> %d, spec %#v", generation, got.Generation, got.Spec)
	}
}

func TestNodeDrainMarkerKeepsTaskSuspendedAndUnmarkingResumesIt(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{validJobTaskTemplate("worker")}},
	}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	// The Node controller wrote this marker and this suspension when the drain
	// completed, then retired the Drain operation. The Job controller must not
	// resume the Task just because the Job itself is not suspended.
	task.Annotations = map[string]string{
		rlarkv1alpha1.NodeDrainStoppedAnnotation:   "node-a",
		rlarkv1alpha1.NodeDrainSuspendedAnnotation: "node-a",
	}
	task.Spec.Suspended = true
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], task.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended {
		t.Fatalf("Job controller resumed a Task stopped by a completed node drain: %#v", got.Spec)
	}
	generation := got.Generation

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], got.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(&got), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended || got.Generation != generation {
		t.Fatalf("Job reconciliation rewrote node-drained Task: generation %d -> %d, spec %#v", generation, got.Generation, got.Spec)
	}

	// Cancelling the drain clears the marker, which is what lets the Job resume.
	delete(got.Annotations, rlarkv1alpha1.NodeDrainStoppedAnnotation)
	delete(got.Annotations, rlarkv1alpha1.NodeDrainSuspendedAnnotation)
	got.Spec.Suspended = false
	if err := c.Update(context.Background(), &got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], got.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(&got), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Suspended {
		t.Fatalf("cancelled node drain left Task suspended: %#v", got.Spec)
	}
}

// TestNodeDrainAfterClusterDrainResumesOnceMarkersCleared guards the stacked
// drain case. A cluster drain suspends the Task outside the compiled desired
// state, so the Task keeps the digest of its running spec. If a node drain then
// completes while the Task is already suspended, the Node controller only writes
// the stopped marker (not the suspended marker), and releasing both markers at
// Job start cannot rely on the gateway rewriting spec.suspended. Resuming must
// therefore not be skipped because the stale digest still matches.
func TestNodeDrainAfterClusterDrainResumesOnceMarkersCleared(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{validJobTaskTemplate("worker")}},
	}
	desired, err := desiredTask(job, job.Spec.Tasks[0], "job-worker", "default", false)
	if err != nil {
		t.Fatal(err)
	}
	runningDigest := desired.Annotations[DesiredDigestAnnotation]
	if runningDigest == "" {
		t.Fatal("test setup: desired Task has no digest")
	}
	// The Task was created from that running desired state, so it carries the
	// digest of the non-suspended spec. The other annotations are what the two
	// drains left behind: the cluster stopped marker plus the node stopped marker,
	// with no node-drain-suspended marker because the Task was already suspended.
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	task.Annotations = map[string]string{
		DesiredDigestAnnotation:                  runningDigest,
		rlarkv1alpha1.ClusterStoppedAnnotation:   "true",
		rlarkv1alpha1.NodeDrainStoppedAnnotation: "node-a",
	}
	task.Spec.Suspended = true
	if digest := task.Annotations[DesiredDigestAnnotation]; digest != runningDigest {
		t.Fatalf("test setup: drain changed the digest %q -> %q", runningDigest, digest)
	}

	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	ctx := context.Background()
	var got rlarkv1alpha1.Task
	if err := c.Get(ctx, client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}

	// Job start clears both markers. There is no node-drain-suspended marker to
	// make the gateway unsuspend the Task, so spec.suspended stays true and only
	// the Job controller can resume it.
	delete(got.Annotations, rlarkv1alpha1.ClusterStoppedAnnotation)
	delete(got.Annotations, rlarkv1alpha1.NodeDrainStoppedAnnotation)
	if err := c.Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.reconcileTaskInNamespace(ctx, job, job.Spec.Tasks[0], got.Namespace, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(&got), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Suspended {
		t.Fatal("Task stayed suspended after the cluster and node drain markers were cleared")
	}
}

func TestClusterUndrainAllowsJobResume(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{validJobTaskTemplate("worker")}},
	}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	task.Annotations = map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true"}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], task.Namespace, logr.Discard()); err != nil {
		t.Fatalf("apply drain stop request: %v", err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatalf("get suspended Task: %v", err)
	}
	if !got.Spec.Suspended {
		t.Fatalf("drain did not suspend Task: %#v", got.Spec)
	}

	delete(got.Annotations, rlarkv1alpha1.ClusterStoppedAnnotation)
	if err := c.Update(context.Background(), &got); err != nil {
		t.Fatalf("clear drain marker: %v", err)
	}
	if _, err := r.reconcileTaskInNamespace(context.Background(), job, job.Spec.Tasks[0], got.Namespace, logr.Discard()); err != nil {
		t.Fatalf("resume Job after undrain: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(&got), &got); err != nil {
		t.Fatalf("get resumed Task: %v", err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" || got.Spec.Suspended {
		t.Fatalf("undrained Task did not resume with Job: %#v", got)
	}
}

func TestDeleteOwnedTasksWaitsForCleanupAndFiltersOwnerUID(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	owned := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name:       "owned",
		Namespace:  "workers",
		Labels:     map[string]string{jobLabel: job.Name},
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
	c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(owned, unrelated).Build()
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

// A Task that only carries the Job label and is not recorded in Status cannot
// be attributed to this Job. Cleanup must refuse rather than skip it: skipping
// would release the Job finalizer while a Task that may well belong to it is
// still running, and deleting could destroy another parent's child.
func TestJobCleanupRefusesUnattributableLabelOnlyTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	orphan := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "job-t1", Namespace: "workers",
		Labels:     map[string]string{jobLabel: job.Name},
		Finalizers: []string{common.TaskAgentCleanupFinalizer},
	}}
	c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(orphan).Build()
	r := &Reconciler{Client: c, Scheme: jobTestScheme(t)}

	if _, err := r.deleteOwnedTasks(context.Background(), job); err == nil {
		t.Fatal("deleteOwnedTasks accepted a Task whose ownership cannot be proven")
	}
	if _, err := r.stopOwnedTasks(context.Background(), job); err == nil {
		t.Fatal("stopOwnedTasks accepted a Task whose ownership cannot be proven")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(orphan), orphan); err != nil {
		t.Fatalf("unattributable Task should be left alone: %v", err)
	}
	if !orphan.DeletionTimestamp.IsZero() {
		t.Fatal("unattributable Task was deleted")
	}
}

// A Job created before the ownership annotations existed only reaches its Tasks
// through the label plus the names it recorded in Status. Cleanup must still
// tear those down, otherwise upgrading leaks every pre-existing Task.
func TestJobCleanupDeletesLegacyTaskRecordedInStatus(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{
			{Name: "t1"},
		}},
	}
	legacy := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: utils.ChildName(job.Name, "t1"), Namespace: "workers",
		Labels:     map[string]string{jobLabel: job.Name},
		Finalizers: []string{common.TaskAgentCleanupFinalizer},
	}}
	c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(legacy).Build()
	r := &Reconciler{Client: c, Scheme: jobTestScheme(t)}

	pending, err := r.deleteOwnedTasks(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("legacy Task should keep Job cleanup pending")
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(legacy), legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.DeletionTimestamp.IsZero() {
		t.Fatal("legacy Task deletion was not requested")
	}
}

func TestStopOwnedTasksDeletesUnclaimedTaskWithoutWaitingForStoppedStatus(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	unclaimed := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "unsupported", Namespace: "workers", Labels: map[string]string{jobLabel: job.Name},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: ptrBool(true),
		}},
	}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhasePending}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(unclaimed).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	stopped, err := r.stopOwnedTasks(context.Background(), job)
	if err != nil || !stopped {
		t.Fatalf("stopOwnedTasks() = %v, %v, want complete", stopped, err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(unclaimed), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("unclaimed Task still exists: %v", err)
	}
}

func TestStopOwnedTasksStillWaitsForAgentClaimedTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	claimed := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "claimed", Namespace: "workers", Labels: map[string]string{jobLabel: job.Name}, Finalizers: []string{common.TaskAgentCleanupFinalizer},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: ptrBool(true),
		}},
	}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhasePending}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(claimed).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	stopped, err := r.stopOwnedTasks(context.Background(), job)
	if err != nil || stopped {
		t.Fatalf("stopOwnedTasks() = %v, %v, want cleanup pending", stopped, err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(claimed), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended || !contains(got.Finalizers, common.TaskAgentCleanupFinalizer) {
		t.Fatalf("claimed Task was not stopped safely: %#v", got.ObjectMeta)
	}
}

func TestStoppedJobDeletesTasksAndWaitsForCleanup(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Suspended: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}}},
	}
	owned := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "job-worker",
			Namespace:  "default",
			Labels:     map[string]string{jobLabel: job.Name},
			Finalizers: []string{"rlark.io/agent-cleanup"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
				Controller: ptrBool(true),
			}},
		},
		Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseRunning},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(owned).Build()
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
			Suspended: true,
			Tasks: []rlarkv1alpha1.JobTaskTemplate{
				{Name: "head", Head: true},
				{Name: "another-head", Head: true},
			},
		},
		Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning},
	}
	owned := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "job-head", Namespace: "default", Finalizers: []string{"rlark.io/agent-cleanup"},
		Labels: map[string]string{jobLabel: job.Name},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
			Controller: ptrBool(true),
		}},
	}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(owned).Build()
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
		Spec:       rlarkv1alpha1.JobSpec{Suspended: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}}},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).Build()
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
		Spec: rlarkv1alpha1.JobSpec{Suspended: true, Tasks: []rlarkv1alpha1.JobTaskTemplate{
			{Name: "done"}, {Name: "failed"}, {Name: "running"},
		}},
		Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{
			{Name: "done", Phase: rlarkv1alpha1.TaskPhaseSucceeded},
			{Name: "failed", Phase: rlarkv1alpha1.TaskPhaseFailed},
			{Name: "running", Phase: rlarkv1alpha1.TaskPhaseRunning},
		}},
	}
	r := &Reconciler{Client: jobTestClientBuilder(jobTestScheme(t)).Build()}

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
			Labels:     map[string]string{jobLabel: job.Name},
			Finalizers: []string{"rlark.io/agent-cleanup"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID,
				Controller: ptrBool(true),
			}},
		},
		Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseStopped},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithStatusSubresource(job).WithObjects(job, task).Build()
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

func TestDeletingJobConvergesWithUnsupportedUnclaimedTask(t *testing.T) {
	deletedAt := metav1.Now()
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid"), DeletionTimestamp: &deletedAt, Finalizers: []string{CleanupFinalizer, "test/finalizer"}},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhasePending}}},
	}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "job-worker", Namespace: "default", Labels: map[string]string{jobLabel: job.Name},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: ptrBool(true)}},
	}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhasePending}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithStatusSubresource(job).WithObjects(job, task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(job)}

	for range 5 {
		if _, err := r.Reconcile(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), job); err != nil {
			t.Fatal(err)
		}
		if !contains(job.Finalizers, CleanupFinalizer) {
			break
		}
	}
	if contains(job.Finalizers, CleanupFinalizer) {
		t.Fatalf("Job cleanup finalizer was not removed: %v", job.Finalizers)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("unclaimed Task still exists: %v", err)
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

// A Job stopped through the documented decommission flow can still be
// impossible to delete once the Nodes its template selected are gone.
// markTaskStatusesStopped keeps terminal Task phases, so a Job whose Task
// succeeded before it was stopped keeps reporting Succeeded in status.tasks.
// The deletion path therefore re-enters syncTaskStatuses, which must not fail
// just because the selected Nodes no longer exist, otherwise the cleanup
// finalizer is never removed and the Job can never be deleted.
func TestDeletingStoppedJobWithTerminalTaskConvergesWithoutNodes(t *testing.T) {
	deletedAt := metav1.Now()
	endTime := metav1.NewTime(time.Now().Truncate(time.Second))
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "job",
			UID:               types.UID("job-uid"),
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{CleanupFinalizer, "test/finalizer"},
		},
		Spec: rlarkv1alpha1.JobSpec{
			Suspended: true,
			Tasks: []rlarkv1alpha1.JobTaskTemplate{{
				Name:     "worker",
				TaskSpec: rlarkv1alpha1.TaskSpec{Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"kubernetes.io/hostname": "retired-node"}}},
			}},
		},
		Status: rlarkv1alpha1.JobStatus{
			Phase:   rlarkv1alpha1.JobPhaseStopped,
			EndTime: &endTime,
			// The Task reached a terminal phase before the Job was stopped, so
			// markTaskStatusesStopped preserved Succeeded here.
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseSucceeded}},
		},
	}
	scheme := jobTestScheme(t)
	// The owned Tasks were already deleted when the Job was stopped, and the
	// Node the template selected no longer exists.
	c := jobTestClientBuilder(scheme).WithStatusSubresource(job).WithObjects(job).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatalf("deleting a stopped Job must not depend on its Nodes: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if contains(job.Finalizers, CleanupFinalizer) {
		t.Fatalf("cleanup finalizer was not removed: %v", job.Finalizers)
	}
	if job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || !job.Status.EndTime.Time.Equal(endTime.Time) {
		t.Fatalf("stopped status changed during deletion: %#v", job.Status)
	}
	if job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseSucceeded {
		t.Fatalf("terminal Task phase was overwritten: %#v", job.Status.Tasks)
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
			c := jobTestClientBuilder(scheme).WithStatusSubresource(job).WithObjects(job).Build()
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

func TestJobsForSchedulingChangeEnqueuesOnlyJobsAwaitingPlacement(t *testing.T) {
	makeJob := func(name string, jobPhase rlarkv1alpha1.JobPhase, taskPhase rlarkv1alpha1.TaskPhase) *rlarkv1alpha1.Job {
		job := &rlarkv1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-uid")},
			Status:     rlarkv1alpha1.JobStatus{Phase: jobPhase},
		}
		if taskPhase != "" {
			job.Status.Tasks = []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: taskPhase}}
		}
		return job
	}
	fresh := makeJob("fresh", "", "")
	pending := makeJob("pending", rlarkv1alpha1.JobPhasePending, rlarkv1alpha1.TaskPhasePending)
	running := makeJob("running", rlarkv1alpha1.JobPhaseRunning, rlarkv1alpha1.TaskPhaseRunning)
	succeeded := makeJob("succeeded", rlarkv1alpha1.JobPhaseSucceeded, rlarkv1alpha1.TaskPhaseSucceeded)
	suspended := makeJob("suspended", rlarkv1alpha1.JobPhasePending, rlarkv1alpha1.TaskPhasePending)
	suspended.Spec.Suspended = true
	deletedAt := metav1.Now()
	deleting := makeJob("deleting", rlarkv1alpha1.JobPhasePending, rlarkv1alpha1.TaskPhasePending)
	deleting.DeletionTimestamp = &deletedAt
	deleting.Finalizers = []string{CleanupFinalizer}

	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(fresh, pending, running, succeeded, suspended, deleting).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	got := map[string]bool{}
	for _, request := range r.jobsForSchedulingChange(context.Background(), &rlarkv1alpha1.Node{}) {
		got[request.Name] = true
	}
	want := map[string]bool{"fresh": true, "pending": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jobsForSchedulingChange() enqueued %v, want %v", got, want)
	}
}

func TestNodeSchedulingPredicateIgnoresHeartbeatOnlyUpdates(t *testing.T) {
	p := nodeSchedulingPredicate()
	heartbeat := metav1.NewTime(time.Now().Truncate(time.Second))
	later := metav1.NewTime(heartbeat.Add(time.Minute))
	oldNode := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node"},
		Status: rlarkv1alpha1.NodeStatus{
			Phase:              rlarkv1alpha1.NodeOnline,
			LastHeartbeatTime:  &heartbeat,
			ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": "n1"},
			Conditions: []metav1.Condition{
				{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue, LastTransitionTime: heartbeat},
				{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue, LastTransitionTime: heartbeat},
				{Type: rlarkv1alpha1.ConditionTypeResourcePressure, Status: metav1.ConditionFalse, LastTransitionTime: heartbeat},
				{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionTrue, Reason: rlarkv1alpha1.ConditionReasonSchedulable, LastTransitionTime: heartbeat},
			},
		},
	}

	heartbeatOnly := oldNode.DeepCopy()
	heartbeatOnly.Status.LastHeartbeatTime = &later
	heartbeatOnly.Status.Conditions[0].LastTransitionTime = later
	heartbeatOnly.Status.Conditions[1].Message = "kubelet heartbeat refreshed"
	heartbeatOnly.Status.ActiveWorkloads = rlarkv1alpha1.NodeWorkloadSummary{Total: 3}
	heartbeatOnly.Status.Phase = rlarkv1alpha1.NodeDegraded
	if p.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: heartbeatOnly}) {
		t.Fatal("heartbeat-only Node update should not enqueue Jobs")
	}

	tests := map[string]func(*rlarkv1alpha1.Node){
		"Schedulable": func(node *rlarkv1alpha1.Node) {
			node.Status.Conditions[3].Status = metav1.ConditionFalse
			node.Status.Conditions[3].Reason = rlarkv1alpha1.ConditionReasonUnschedulable
		},
		"CapabilitySnapshot": func(node *rlarkv1alpha1.Node) {
			node.Status.CapabilitySnapshot.Devices = []rlarkv1alpha1.NodeDeviceCapability{{Category: "nvidia.com/gpu", Available: 1}}
		},
		"Labels": func(node *rlarkv1alpha1.Node) {
			node.Status.ObservedNodeLabels["kubernetes.io/hostname"] = "n2"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := oldNode.DeepCopy()
			mutate(changed)
			if !p.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: changed}) {
				t.Fatalf("%s change should enqueue Jobs", name)
			}
		})
	}

	// Underlying conditions only matter through the controller's Schedulable
	// conclusion; changing them alone must not enqueue Jobs.
	nonSchedulingChanges := map[string]func(*rlarkv1alpha1.Node){
		"Ready": func(node *rlarkv1alpha1.Node) {
			node.Status.Conditions[1].Status = metav1.ConditionFalse
		},
		"ResourcePressure": func(node *rlarkv1alpha1.Node) {
			node.Status.Conditions[2].Status = metav1.ConditionTrue
		},
		"AgentConnected": func(node *rlarkv1alpha1.Node) {
			node.Status.Conditions[0].Status = metav1.ConditionFalse
		},
		"Unschedulable": func(node *rlarkv1alpha1.Node) {
			node.Spec.Unschedulable = true
		},
	}
	for name, mutate := range nonSchedulingChanges {
		t.Run(name+" without Schedulable change", func(t *testing.T) {
			changed := oldNode.DeepCopy()
			mutate(changed)
			if p.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: changed}) {
				t.Fatalf("%s change without a Schedulable change should not enqueue Jobs", name)
			}
		})
	}
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
