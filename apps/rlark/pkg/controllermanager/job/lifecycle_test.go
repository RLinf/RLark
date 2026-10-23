package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	controllerbase "github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

type failingNodeListClient struct{ client.Client }

func (c failingNodeListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*rlarkv1alpha1.NodeList); ok {
		return fmt.Errorf("node list failed")
	}
	return c.Client.List(ctx, list, opts...)
}

type failSecondNodeListClient struct {
	client.Client
	lists int
}

func (c *failSecondNodeListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*rlarkv1alpha1.NodeList); ok {
		c.lists++
		if c.lists > 1 {
			return fmt.Errorf("unexpected second node list")
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func TestSyncTaskStatusSnapshotIsExactAndOrdered(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "b"}, {Name: "a"}}},
		Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{
			{Name: "a", Phase: rlarkv1alpha1.TaskPhaseSucceeded, Message: "done"}, {Name: "removed"},
		}},
	}
	if !syncTaskStatusSnapshot(job) {
		t.Fatal("expected snapshot change")
	}
	if len(job.Status.Tasks) != 2 || job.Status.Tasks[0].Name != "b" || job.Status.Tasks[1].Name != "a" || job.Status.Tasks[1].Message != "done" {
		t.Fatalf("unexpected snapshot: %#v", job.Status.Tasks)
	}
}

func TestReconcileTaskPreservesAnnotationsAndNaturalTerminalSpec(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rayTaskTemplate(t, "worker", false, nil, "new")
	task := buildTask(job, template, "job-worker", "default")
	parameters := rayParameters(t, task.Spec.Workload.Parameters)
	parameters.Run.Inline = "old"
	raw, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Parameters.Raw = raw
	task.Annotations["user"] = "keep"
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if rayParameters(t, got.Spec.Workload.Parameters).Run.Inline != "old" || got.Annotations["user"] != "keep" {
		t.Fatalf("terminal Task was rewritten: %#v", got)
	}
}

func rayTaskTemplate(t *testing.T, name string, head bool, replicas *int32, run string) rlarkv1alpha1.JobTaskTemplate {
	t.Helper()
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{Run: &taskapi.InlineScript{Inline: run}})
	if err != nil {
		t.Fatal(err)
	}
	carrierParameters, err := json.Marshal(taskapi.KubernetesReplicatedCarrierParametersV1{Replicas: replicas})
	if err != nil {
		t.Fatal(err)
	}
	return rlarkv1alpha1.JobTaskTemplate{Name: name, Head: head, TaskSpec: rlarkv1alpha1.TaskSpec{
		Workload: rlarkv1alpha1.TaskWorkloadSpec{
			Kind:       taskapi.WorkloadTypeRay,
			Version:    taskapi.WorkloadVersionRayV1,
			Parameters: apiextensionsv1.JSON{Raw: parameters},
			Carrier: rlarkv1alpha1.TaskWorkloadCarrier{
				Type:       taskapi.CarrierTypeKubernetesDeployment,
				Version:    taskapi.CarrierVersionKubernetesV1,
				Parameters: apiextensionsv1.JSON{Raw: carrierParameters},
			},
		},
	}}
}

func rayParameters(t *testing.T, parameters apiextensionsv1.JSON) taskapi.RayExecutionParametersV1 {
	t.Helper()
	result, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](parameters)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestReconcileTaskResumesControllerStoppedTerminalTask(t *testing.T) {
	replicas := int32(4)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rayTaskTemplate(t, "worker", false, &replicas, "")
	task := buildTask(job, template, "job-worker", "default")
	task.Spec = *task.Spec.DeepCopy()
	task.Spec.Suspended = true
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Suspended {
		t.Fatal("controller-suspended terminal Task was not resumed")
	}
}

func TestReconcileTaskWaitsForTerminatingTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker"}
	task := buildTask(job, template, "job-worker", "default")
	task.UID = types.UID("task-uid")
	task.Finalizers = []string{"rlark.io/agent-cleanup"}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	if err := c.Delete(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileTask(context.Background(), job, template, logr.Discard()); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatalf("reconcile terminating Task error = %v, want cleanup requeue", err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if got.DeletionTimestamp.IsZero() {
		t.Fatal("test Task is not terminating")
	}
}

func TestReconcileTaskRestartDeletesThenRecreatesTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", UID: types.UID("job-uid"), Annotations: map[string]string{RestartedAtAnnotation: "new-operation"},
	}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker"}
	oldTask := buildTask(job, template, "job-worker", "default")
	oldTask.UID = types.UID("old-task-uid")
	oldTask.Annotations[RestartedAtAnnotation] = "old-operation"
	controller := true
	oldTask.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(oldTask).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.reconcileTask(context.Background(), job, template, logr.Discard()); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatalf("restart reconcile error = %v, want cleanup requeue", err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(oldTask), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("old Task still exists: %v", err)
	}
	newTask, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if newTask.Annotations[RestartedAtAnnotation] != "new-operation" {
		t.Fatalf("new Task operation = %q", newTask.Annotations[RestartedAtAnnotation])
	}
	if newTask.Status.Phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatalf("new Task phase = %q, want Pending", newTask.Status.Phase)
	}
	if newTask.UID == oldTask.UID {
		t.Fatalf("restart reused old Task UID %q", newTask.UID)
	}
	gotAgain, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if gotAgain.Name != newTask.Name || gotAgain.Annotations[RestartedAtAnnotation] != "new-operation" {
		t.Fatalf("duplicate reconcile replaced the current Task: %#v", gotAgain.ObjectMeta)
	}
}

func TestReconcileTaskRestartWaitsForAgentCleanupFinalizer(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", UID: types.UID("job-uid"), Annotations: map[string]string{RestartedAtAnnotation: "new-operation"},
	}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker"}
	oldTask := buildTask(job, template, "job-worker", "default")
	oldTask.UID = types.UID("old-task-uid")
	oldTask.Annotations[RestartedAtAnnotation] = "old-operation"
	oldTask.Finalizers = []string{common.TaskAgentCleanupFinalizer}
	controller := true
	oldTask.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(oldTask).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	for range 2 {
		if _, err := r.reconcileTask(context.Background(), job, template, logr.Discard()); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
			t.Fatalf("restart reconcile error = %v, want cleanup requeue", err)
		}
	}
	var terminating rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(oldTask), &terminating); err != nil {
		t.Fatal(err)
	}
	if terminating.DeletionTimestamp.IsZero() {
		t.Fatal("old Task is not waiting for Agent cleanup")
	}
	terminating.Finalizers = nil
	if err := c.Update(context.Background(), &terminating); err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(oldTask), &rlarkv1alpha1.Task{}); err == nil {
		t.Fatal("old Task still exists after Agent finalizer release")
	} else if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	newTask, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if newTask.Annotations[RestartedAtAnnotation] != "new-operation" || newTask.Name != oldTask.Name {
		t.Fatalf("replacement Task = %#v", newTask.ObjectMeta)
	}
}

func TestReconcileTaskResumesTerminalSuspendedTask(t *testing.T) {
	replicas := int32(4)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rayTaskTemplate(t, "worker", false, &replicas, "")
	task := buildTask(job, template, "job-worker", "default")
	task.Spec.Suspended = true
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Suspended {
		t.Fatal("terminal suspended Task was not resumed")
	}
}

func TestReconcileTaskAdoptsLegacyAndRejectsConflict(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker"}
	legacy := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-worker", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}, Annotations: map[string]string{"user": "keep"}}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(legacy).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	task, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if task.Annotations["user"] != "keep" || task.Annotations[utils.ParentUIDAnnotation] != string(job.UID) || metav1.GetControllerOf(task).UID != job.UID {
		t.Fatalf("legacy Task not adopted safely: %#v", task.ObjectMeta)
	}
	task.Annotations[utils.ParentUIDAnnotation] = "other"
	if err := c.Update(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if _, err := r.reconcileTask(context.Background(), job, template, logr.Discard()); err == nil {
		t.Fatal("expected conflicting UID to be rejected")
	}
}

func TestAdoptedOwnerReferenceOnlyTaskBackfillsJobLabel(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}},
	}
	controller := true
	legacy := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name:      "job-worker",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job",
			Name: job.Name, UID: job.UID, Controller: &controller,
		}},
	}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(legacy).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	task, err := r.reconcileTask(context.Background(), job, job.Spec.Tasks[0], logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if task.Labels[jobLabel] != job.Name {
		t.Fatalf("adopted Task did not get the Job label backfilled: %#v", task.Labels)
	}

	// The backfill is what keeps the adopted Task discoverable through the
	// taskJobField index, which only indexes the Job label.
	tasks, err := r.listJobTasks(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Name != legacy.Name {
		t.Fatalf("adopted Task not discoverable by Job label: %#v", tasks)
	}
}

func TestSyncTaskStatusesRejectsOwnershipMismatch(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhasePending}}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	task.Annotations[utils.ParentUIDAnnotation] = "other"
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.syncTaskStatuses(context.Background(), job); err == nil {
		t.Fatal("expected ownership mismatch")
	}
	if job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatal("mismatched Task changed Job status")
	}
}

func TestPruneTasksStopsNamespaceDriftBeforeDelete(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
	task.Spec.Suspended = false
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	waiting, err := r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks() = %v, %v", waiting, err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), task); err != nil {
		t.Fatal(err)
	}
	if !task.Spec.Suspended || !task.DeletionTimestamp.IsZero() {
		t.Fatalf("drifted Task was not stopped before deletion: %#v", task)
	}
	task.Status.Phase = rlarkv1alpha1.TaskPhaseStopped
	if err := c.Update(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	waiting, err = r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("stopped pruneTasks() = %v, %v", waiting, err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), task); err == nil {
		t.Fatal("stopped drifted Task was not deleted")
	}
}

func TestPruneTasksStopsOwnerOnlyRemovedTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "removed"}}}}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-removed", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}}}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	waiting, err := r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks() = %v, %v", waiting, err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended {
		t.Fatalf("owner-only removed Task was not stopped: %#v", got)
	}
}

func TestJobReconcileUsesOldStatusToPruneSafely(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseSucceeded, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "removed"}}},
	}
	controller := true
	ownerOnly := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "legacy-name", Namespace: "default", Labels: map[string]string{jobLabel: job.Name},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}},
	}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseStopped}}
	unrelated := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-unrelated", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseStopped}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(ownerOnly, unrelated).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.ReconcileStateMachine(context.Background(), job); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ownerOnly), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owner-only removed Task was not pruned: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(unrelated), &rlarkv1alpha1.Task{}); err != nil {
		t.Fatalf("unrelated label-only prefixed Task was touched: %v", err)
	}
}

// A stopped Job must converge even when the Nodes its task template selected
// have been removed from the management API (for example after the data plane
// that hosted them was decommissioned). Observing and deleting the Job's own
// Tasks must not depend on Node lookup.
func TestStoppedJobConvergesAfterSelectedNodesDisappear(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec: rlarkv1alpha1.JobSpec{
			Suspended: true,
			Tasks: []rlarkv1alpha1.JobTaskTemplate{{
				Name:     "worker",
				TaskSpec: rlarkv1alpha1.TaskSpec{Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"kubernetes.io/hostname": "retired-node"}}},
			}},
		},
		Status: rlarkv1alpha1.JobStatus{
			Phase: rlarkv1alpha1.JobPhasePending,
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhasePending}},
		},
	}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller,
	}}
	task.Status = rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseRunning}
	scheme := jobTestScheme(t)
	// Only the Job and its Task survive; the selected Node no longer exists.
	c := jobTestClientBuilder(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.ReconcileStateMachine(context.Background(), job); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatalf("stopped Job should requeue while its Task is deleted: %v", err)
	}
	if _, err := r.ReconcileStateMachine(context.Background(), job); err != nil {
		t.Fatalf("stopped Job did not converge without its Nodes: %v", err)
	}
	if job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || job.Status.EndTime == nil {
		t.Fatalf("stopped Job status not completed: %#v", job.Status)
	}
	if job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("Task status was not stopped: %#v", job.Status.Tasks)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owned Task was not deleted: %v", err)
	}
}

func TestNamespaceResolutionFailurePreservesExistingTask(t *testing.T) {
	for _, tt := range []struct {
		name      string
		listFails bool
	}{
		{name: "list error", listFails: true},
		{name: "no match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}}}}}}}
			task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
			task.Spec.Suspended = false
			controller := true
			task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
			scheme := jobTestScheme(t)
			baseClient := jobTestClientBuilder(scheme).WithObjects(task).Build()
			var c client.Client = baseClient
			if tt.listFails {
				c = failingNodeListClient{Client: c}
			}
			r := &Reconciler{Client: c, Scheme: scheme}
			if _, err := r.pruneTasks(context.Background(), job); err == nil {
				t.Fatal("expected namespace resolution error")
			}
			var got rlarkv1alpha1.Task
			if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil || got.Spec.Suspended {
				t.Fatalf("healthy Task changed after resolution failure: task=%#v err=%v", got, err)
			}
		})
	}
}

func TestResolvedNamespaceIsDeterministicAndMigratesTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}}}}}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
	task.Spec.Suspended = false
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	nodes := []client.Object{
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "workers-b", Labels: map[string]string{"zone": "new"}}},
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "workers-a", Labels: map[string]string{"zone": "new"}}},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(append(nodes, task)...).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	namespace, err := r.resolveTaskNamespace(context.Background(), &job.Spec.Tasks[0])
	if err != nil || namespace != "workers-a" {
		t.Fatalf("namespace=%q err=%v", namespace, err)
	}
	waiting, err := r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks()=%v, %v", waiting, err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil || !got.Spec.Suspended {
		t.Fatalf("old Task was not stopped for resolved migration: %#v err=%v", got, err)
	}
}

func TestDispatchResolvesNamespaceBeforeMutating(t *testing.T) {
	replicas := int32(2)
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{
		Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}},
		Workload:  rayTaskTemplate(t, "worker", false, &replicas, "").Workload,
	}}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, template, "job-worker", "workers")
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Namespace: "workers", Labels: map[string]string{"zone": "new"}}}
	scheme := jobTestScheme(t)
	baseClient := jobTestClientBuilder(scheme).WithObjects(task, node).Build()
	c := &failSecondNodeListClient{Client: baseClient}
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if c.lists != 1 {
		t.Fatalf("Node lists = %d, want 1", c.lists)
	}
}
