package job

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{RunScript: "new"}}
	task := buildTask(job, template, "job-worker", "default")
	task.Spec.RunScript = "old"
	task.Annotations["user"] = "keep"
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.RunScript != "old" || got.Annotations["user"] != "keep" {
		t.Fatalf("terminal Task was rewritten: %#v", got)
	}
}

func TestReconcileTaskResumesControllerStoppedTerminalTask(t *testing.T) {
	replicas := int32(4)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: &replicas}}}}
	task := buildTask(job, template, "job-worker", "default")
	task.Spec = *task.Spec.DeepCopy()
	task.Spec.Kubernetes.Workload.Replicas = ptr.To(int32(0))
	task.Annotations[StoppedAnnotation] = "true"
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.Kubernetes.Workload.Replicas != replicas || got.Annotations[StoppedAnnotation] != "" {
		t.Fatalf("controller-stopped terminal Task was not resumed: replicas=%d annotations=%v", *got.Spec.Kubernetes.Workload.Replicas, got.Annotations)
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
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
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

func TestReconcileTaskDoesNotResumeTerminalTaskWithoutZeroReplicas(t *testing.T) {
	replicas := int32(4)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: &replicas}}}}
	task := buildTask(job, template, "job-worker", "default")
	task.Annotations[StoppedAnnotation] = "true"
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.Annotations[StoppedAnnotation] != "true" || *got.Spec.Kubernetes.Workload.Replicas != replicas {
		t.Fatalf("terminal Task without zero replicas changed: replicas=%d annotations=%v", *got.Spec.Kubernetes.Workload.Replicas, got.Annotations)
	}
}

func TestReconcileTaskAdoptsLegacyAndRejectsConflict(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker"}
	legacy := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-worker", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}, Annotations: map[string]string{"user": "keep"}}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()
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

func TestSyncTaskStatusesRejectsOwnershipMismatch(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}}}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhasePending}}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "default")
	task.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	task.Annotations[utils.ParentUIDAnnotation] = "other"
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
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
	task.Spec.Kubernetes = &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: ptr.To(int32(2))}}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	waiting, err := r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks() = %v, %v", waiting, err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), task); err != nil {
		t.Fatal(err)
	}
	if task.Annotations[StoppedAnnotation] != "true" || *task.Spec.Kubernetes.Workload.Replicas != 0 || !task.DeletionTimestamp.IsZero() {
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
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-removed", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}}, Spec: rlarkv1alpha1.TaskSpec{Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: ptr.To(int32(2))}}}}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	waiting, err := r.pruneTasks(context.Background(), job)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks() = %v, %v", waiting, err)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[StoppedAnnotation] != "true" || *got.Spec.Kubernetes.Workload.Replicas != 0 {
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
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ownerOnly, unrelated).Build()
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

func TestNamespaceResolutionFailurePreservesExistingTask(t *testing.T) {
	for _, tt := range []struct {
		name      string
		listFails bool
	}{
		{name: "list error", listFails: true},
		{name: "no match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{NodeSelector: map[string]string{"zone": "new"}}}}}}
			task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
			task.Spec.Kubernetes = &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: ptr.To(int32(2))}}
			controller := true
			task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
			scheme := jobTestScheme(t)
			baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
			var c client.Client = baseClient
			if tt.listFails {
				c = failingNodeListClient{Client: c}
			}
			r := &Reconciler{Client: c, Scheme: scheme}
			if _, err := r.pruneTasks(context.Background(), job); err == nil {
				t.Fatal("expected namespace resolution error")
			}
			var got rlarkv1alpha1.Task
			if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil || got.Annotations[StoppedAnnotation] != "" || *got.Spec.Kubernetes.Workload.Replicas != 2 {
				t.Fatalf("healthy Task changed after resolution failure: task=%#v err=%v", got, err)
			}
		})
	}
}

func TestResolvedNamespaceIsDeterministicAndMigratesTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{NodeSelector: map[string]string{"zone": "new"}}}}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
	task.Spec.Kubernetes = &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: ptr.To(int32(2))}}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	nodes := []client.Object{
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "workers-b", Labels: map[string]string{"zone": "new"}}},
		&rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "workers-a", Labels: map[string]string{"zone": "new"}}},
	}
	scheme := jobTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, task)...).Build()
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
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil || got.Annotations[StoppedAnnotation] != "true" || !reflect.DeepEqual(got.Spec.Kubernetes.Workload.Replicas, ptr.To(int32(0))) {
		t.Fatalf("old Task was not stopped for resolved migration: %#v err=%v", got, err)
	}
}

func TestDispatchResolvesNamespaceBeforeMutating(t *testing.T) {
	replicas := int32(2)
	template := rlarkv1alpha1.JobTaskTemplate{Name: "worker", TaskSpec: rlarkv1alpha1.TaskSpec{NodeSelector: map[string]string{"zone": "new"}, Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: &replicas}}}}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, template, "job-worker", "workers")
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Namespace: "workers", Labels: map[string]string{"zone": "new"}}}
	scheme := jobTestScheme(t)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task, node).Build()
	c := &failSecondNodeListClient{Client: baseClient}
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if c.lists != 1 {
		t.Fatalf("Node lists = %d, want 1", c.lists)
	}
}
