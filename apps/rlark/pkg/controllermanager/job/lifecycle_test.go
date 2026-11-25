package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	r, _ := reconcilerWithDefaultPlacement(t, task)
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if rayParameters(t, got.Spec.Workload.Parameters).Run.Inline != "old" || got.Annotations["user"] != "keep" {
		t.Fatalf("terminal Task was rewritten: %#v", got)
	}
}

func TestReconcileTaskPreservesAgentDrainRequests(t *testing.T) {
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
	drainRequests := []rlarkv1alpha1.TaskDrainRequest{{OperationID: "drain-1", NodeUID: "node-uid", NodeName: "node-a"}}
	task.Spec.DrainRequests = drainRequests
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	r, _ := reconcilerWithDefaultPlacement(t, task)
	got, err := r.reconcileTask(context.Background(), job, template, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.DrainRequests) != 1 || got.Spec.DrainRequests[0] != drainRequests[0] {
		t.Fatalf("drain requests = %#v, want %#v", got.Spec.DrainRequests, drainRequests)
	}
	if rayParameters(t, got.Spec.Workload.Parameters).Run.Inline != "new" {
		t.Fatalf("controller-owned spec was not updated: %#v", got.Spec.Workload.Parameters)
	}
}

func schedulableNode(name, namespace string, labels map[string]string) *rlarkv1alpha1.Node {
	node := &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     schedulableNodeStatus(),
	}
	node.Status.ObservedNodeLabels = labels
	return node
}

func schedulableNodeStatus() rlarkv1alpha1.NodeStatus {
	return rlarkv1alpha1.NodeStatus{
		Conditions: []metav1.Condition{
			{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
			{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
			{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionTrue, Reason: rlarkv1alpha1.ConditionReasonSchedulable},
		},
		CapabilitySnapshot: rlarkv1alpha1.NodeCapabilitySnapshot{
			Runtime: rlarkv1alpha1.NodeRuntimeCapability{
				Type:          rlarkv1alpha1.AgentTypeKubernetes,
				WorkloadKinds: []string{taskapi.WorkloadTypeRay},
			},
		},
	}
}

func reconcilerWithDefaultPlacement(t *testing.T, task *rlarkv1alpha1.Task) (*Reconciler, client.Client) {
	t.Helper()
	scheme := jobTestScheme(t)
	node := schedulableNode("node", "default", nil)
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "default"},
		Status:     rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline},
	}
	objects := []client.Object{node, cluster}
	if task != nil {
		objects = append(objects, task)
	}
	c := jobTestClientBuilder(scheme).WithObjects(objects...).Build()
	return &Reconciler{Client: c, Scheme: scheme}, c
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
		Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKind(rlarkv1alpha1.AgentTypeKubernetes)},
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

func placementRayTaskTemplate(t *testing.T, name string, placement rlarkv1alpha1.TaskPlacementSpec) rlarkv1alpha1.JobTaskTemplate {
	template := rayTaskTemplate(t, name, false, nil, "")
	template.Placement = placement
	return template
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
	r, _ := reconcilerWithDefaultPlacement(t, task)
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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	task := buildTask(job, template, "job-worker", "default")
	task.UID = types.UID("task-uid")
	task.Finalizers = []string{"rlark.io/agent-cleanup"}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	r, c := reconcilerWithDefaultPlacement(t, task)
	if err := c.Delete(context.Background(), task); err != nil {
		t.Fatal(err)
	}

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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	oldTask := buildTask(job, template, "job-worker", "default")
	oldTask.UID = types.UID("old-task-uid")
	oldTask.Annotations[RestartedAtAnnotation] = "old-operation"
	controller := true
	oldTask.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	r, c := reconcilerWithDefaultPlacement(t, oldTask)

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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	oldTask := buildTask(job, template, "job-worker", "default")
	oldTask.UID = types.UID("old-task-uid")
	oldTask.Annotations[RestartedAtAnnotation] = "old-operation"
	oldTask.Finalizers = []string{common.TaskAgentCleanupFinalizer}
	controller := true
	oldTask.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	r, c := reconcilerWithDefaultPlacement(t, oldTask)

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
	r, _ := reconcilerWithDefaultPlacement(t, task)
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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	legacy := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-worker", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}, Annotations: map[string]string{"user": "keep"}}}
	r, c := reconcilerWithDefaultPlacement(t, legacy)
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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}},
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
	r, _ := reconcilerWithDefaultPlacement(t, legacy)

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
	template := rayTaskTemplate(t, "worker", false, nil, "")
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
	task.Spec.Suspended = false
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	r, c := reconcilerWithDefaultPlacement(t, task)
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

func TestPrunedTaskResumesWhenItsTemplateIsAddedBack(t *testing.T) {
	template := rayTaskTemplate(t, "worker", false, nil, "")
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}},
	}
	taskName := utils.ChildName(job.Name, template.Name)
	task, err := desiredTask(job, template, taskName, "default", false)
	if err != nil {
		t.Fatal(err)
	}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: rlarkv1alpha1.GroupVersion.String(),
		Kind:       "Job",
		Name:       job.Name,
		UID:        job.UID,
		Controller: &controller,
	}}
	r, c := reconcilerWithDefaultPlacement(t, task)

	withoutTemplate := job.DeepCopy()
	withoutTemplate.Spec.Tasks = nil
	waiting, err := r.pruneTasks(context.Background(), withoutTemplate)
	if err != nil || !waiting {
		t.Fatalf("pruneTasks() = %v, %v", waiting, err)
	}
	var stopped rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &stopped); err != nil {
		t.Fatal(err)
	}
	if !stopped.Spec.Suspended {
		t.Fatalf("pruned Task was not stopped: %#v", stopped)
	}
	if digest := stopped.Annotations[DesiredDigestAnnotation]; digest != "" {
		t.Fatalf("suspending a pruned Task kept a digest it never compiled: %q", digest)
	}

	if _, err := r.reconcileTaskInNamespace(context.Background(), job, template, "default", logr.Discard()); err != nil {
		t.Fatal(err)
	}
	var resumed rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Spec.Suspended {
		t.Fatalf("Task stayed suspended after its template came back: %#v", resumed)
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
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(ownerOnly).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.ReconcileStateMachine(context.Background(), job); !errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ownerOnly), &rlarkv1alpha1.Task{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owner-only removed Task was not pruned: %v", err)
	}
}

// A Task that only carries the Job label cannot be attributed to a removed
// template unless its name is one the Job itself recorded. Such a Task must
// neither be deleted on a name-prefix guess nor silently ignored: the reconcile
// is blocked so the ambiguity is reported instead of acted on.
func TestJobReconcileRefusesUnattributableLabelOnlyTask(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseSucceeded, Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "removed"}}},
	}
	unrelated := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "job-unrelated", Namespace: "default", Labels: map[string]string{jobLabel: job.Name}}, Status: rlarkv1alpha1.TaskStatus{Phase: rlarkv1alpha1.TaskPhaseStopped}}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(unrelated).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	_, err := r.ReconcileStateMachine(context.Background(), job)
	if err == nil || errors.Is(err, controllerbase.ErrRequeueAfterChildCleanup) {
		t.Fatalf("unattributable Task did not block the reconcile: %v", err)
	}
	if !strings.Contains(err.Error(), string(utils.ReasonUnprovenOwnership)) {
		t.Fatalf("reconcile was not blocked on unproven ownership: %v", err)
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
			template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
			job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
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
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "old")
	task.Spec.Suspended = false
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	nodes := []client.Object{
		schedulableNode("z", "workers-b", map[string]string{"zone": "new"}),
		schedulableNode("a", "workers-a", map[string]string{"zone": "new"}),
	}
	clusters := []client.Object{
		&rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers-b"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}},
		&rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers-a"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}},
	}
	scheme := jobTestScheme(t)
	objects := append(nodes, task)
	objects = append(objects, clusters...)
	c := jobTestClientBuilder(scheme).WithObjects(objects...).Build()
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

func TestResolveTaskNamespaceUsesSchedulingConditions(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	readyConditions := []metav1.Condition{
		{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
		{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
		{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionTrue},
	}
	for _, tt := range []struct {
		name      string
		node      rlarkv1alpha1.Node
		wantError bool
	}{
		{
			name: "cordoned node remains healthy but is excluded",
			node: rlarkv1alpha1.Node{
				Spec: rlarkv1alpha1.NodeSpec{Unschedulable: true},
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionFalse, Reason: rlarkv1alpha1.ConditionReasonUnschedulable},
				}},
			},
			wantError: true,
		},
		{
			name: "offline phase cannot override a schedulable condition",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOffline, Conditions: readyConditions},
			},
		},
		{
			name: "disconnected node is excluded by the controller conclusion",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionFalse},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionFalse, Reason: rlarkv1alpha1.ConditionReasonLeaseExpired},
				}},
			},
			wantError: true,
		},
		{
			name: "resource pressure excludes a ready node from new placement",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeResourcePressure, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionFalse, Reason: rlarkv1alpha1.ConditionReasonResourcePressureDetected},
				}},
			},
			wantError: true,
		},
		{
			name: "degraded node without pressure remains eligible",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeDegraded, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionTrue},
				}},
			},
		},
		{
			name: "schedulable condition is authoritative",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionFalse},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionFalse},
					{Type: rlarkv1alpha1.ConditionTypeSchedulable, Status: metav1.ConditionTrue},
				}},
			},
		},
		{
			name: "node without a schedulable condition is fail-closed",
			node: rlarkv1alpha1.Node{
				Status: rlarkv1alpha1.NodeStatus{Conditions: []metav1.Condition{
					{Type: rlarkv1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
					{Type: rlarkv1alpha1.ConditionTypeReady, Status: metav1.ConditionTrue},
				}},
			},
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.node.Name = "node"
			tt.node.Namespace = "workers"
			tt.node.Status.ObservedNodeLabels = map[string]string{"zone": "new"}
			tt.node.Status.CapabilitySnapshot = schedulableNodeStatus().CapabilitySnapshot
			cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}}
			c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(&tt.node, cluster).Build()
			r := &Reconciler{Client: c}
			_, err := r.resolveTaskNamespace(context.Background(), &template)
			if (err != nil) != tt.wantError {
				t.Fatalf("resolveTaskNamespace() error = %v, wantError %t", err, tt.wantError)
			}
		})
	}
}

func TestResolveTaskNamespaceFallsBackForLegacyAgentNode(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	//nolint:staticcheck // Exercises the legacy fallback that reads the deprecated Node field.
	node.Spec.AgentType = rlarkv1alpha1.AgentTypeKubernetes
	node.Status.CapabilitySnapshot = rlarkv1alpha1.NodeCapabilitySnapshot{}
	cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}}
	c := jobTestClientBuilder(jobTestScheme(t)).WithObjects(node, cluster).Build()
	r := &Reconciler{Client: c}

	namespace, err := r.resolveTaskNamespace(context.Background(), &template)
	if err != nil {
		t.Fatalf("resolveTaskNamespace() error = %v, want successful legacy fallback", err)
	}
	if namespace != "workers" {
		t.Fatalf("namespace = %q, want workers", namespace)
	}
}

func TestResolveTaskNamespaceRejectsIneligibleClusters(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster rlarkv1alpha1.Cluster
	}{
		{name: "cordoned", cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Cordoned: true}}},
		{name: "draining", cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Draining: true}}},
		{name: "delisted", cluster: rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Delisted: true}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
			job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
			tt.cluster.Name = "cluster"
			tt.cluster.Spec.AgentNamespace = "workers"
			tt.cluster.Status.Phase = rlarkv1alpha1.ClusterOnline
			node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
			scheme := jobTestScheme(t)
			c := jobTestClientBuilder(scheme).WithObjects(node, &tt.cluster).Build()
			r := &Reconciler{Client: c, Scheme: scheme}

			if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err == nil {
				t.Fatal("expected dispatch rejection")
			}
			var tasks rlarkv1alpha1.TaskList
			if err := c.List(context.Background(), &tasks); err != nil {
				t.Fatal(err)
			}
			if len(tasks.Items) != 0 {
				t.Fatalf("created Tasks: %#v", tasks.Items)
			}
		})
	}
}

// TestDispatchTasksHonoursClusterDrainWhenPlacementIsUnavailable guards the drain
// against a deadlock: draining a Cluster cordons it, so no Node is eligible for new
// placement and namespace resolution fails. The Job controller must still lower the
// cluster drain stop request for the Tasks this Job already owns, otherwise the
// drain waits forever on a Task that nothing can stop.
func TestDispatchTasksHonoursClusterDrainWhenPlacementIsUnavailable(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, job.Spec.Tasks[0], "job-worker", "workers")
	task.Spec.Suspended = false
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] = "true"
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers", Cordoned: true, Draining: true},
		Status:     rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterCordoned},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(node, cluster, task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.dispatchTasks(context.Background(), job, logr.Discard())
	if err != nil {
		t.Fatalf("dispatchTasks() error = %v, want the cluster drain stop request honoured", err)
	}
	if changed {
		t.Fatal("dispatchTasks() reported a Job status change for a stop-only pass")
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended {
		t.Fatalf("cluster drain stop request was not applied: %#v", got.Spec)
	}
}

// TestClusterDrainReleaseResumesTaskDespiteStaleDesiredDigest guards the resume
// path that follows a cluster drain. The drain suspends the Task outside the
// compiled desired state, so the digest recorded before the drain still
// describes a running Task. If that digest survives the suspend, releasing the
// drain marker (uncordon + Job start) compiles the same non-suspended state,
// the comparison reports "up to date", and the Task stays suspended forever.
func TestClusterDrainReleaseResumesTaskDespiteStaleDesiredDigest(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	template.Workload.Template = validJobTaskTemplate("worker").Workload.Template
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}},
		Status: rlarkv1alpha1.JobStatus{
			Phase: rlarkv1alpha1.JobPhaseRunning,
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}},
		},
	}
	task, err := desiredTask(job, template, "job-worker", "workers", false)
	if err != nil {
		t.Fatal(err)
	}
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	task.Status.Phase = rlarkv1alpha1.TaskPhaseRunning
	if task.Annotations[DesiredDigestAnnotation] == "" {
		t.Fatal("test setup: Task has no desired digest")
	}

	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers", Cordoned: true, Draining: true},
		Status:     rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterCordoned},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(node, cluster, task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	ctx := context.Background()
	key := client.ObjectKeyFromObject(task)

	// Cluster drain: placement fails because the Cluster is cordoned, so the
	// stop request is applied outside the compiled desired state.
	clusterStop := task.Annotations
	clusterStop[rlarkv1alpha1.ClusterStoppedAnnotation] = "true"
	if err := c.Update(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := r.dispatchTasks(ctx, job, logr.Discard()); err != nil {
		t.Fatalf("dispatchTasks() error = %v, want the drain stop honoured", err)
	}
	var drained rlarkv1alpha1.Task
	if err := c.Get(ctx, key, &drained); err != nil {
		t.Fatal(err)
	}
	if !drained.Spec.Suspended {
		t.Fatal("cluster drain did not suspend the Task")
	}
	if digest := drained.Annotations[DesiredDigestAnnotation]; digest == "" {
		t.Fatal("test setup: drain suspend unexpectedly restamped the digest")
	}

	// Uncordon + Job start: the marker is released and the Cluster is online
	// again, so the Job controller has to resume the Task.
	delete(drained.Annotations, rlarkv1alpha1.ClusterStoppedAnnotation)
	if err := c.Update(ctx, &drained); err != nil {
		t.Fatal(err)
	}
	cluster.Spec.Cordoned = false
	cluster.Spec.Draining = false
	cluster.Status.Phase = rlarkv1alpha1.ClusterOnline
	if err := c.Update(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReconcileStateMachine(ctx, job); err != nil {
		t.Fatalf("ReconcileStateMachine() error = %v", err)
	}
	var resumed rlarkv1alpha1.Task
	if err := c.Get(ctx, key, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Spec.Suspended {
		t.Fatal("Task stayed suspended after the cluster drain was released")
	}
}

func TestNodeDrainStopsJobWhenPlacementIsUnavailable(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	template.Workload.Template = validJobTaskTemplate("worker").Workload.Template
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}},
		Status: rlarkv1alpha1.JobStatus{
			Phase: rlarkv1alpha1.JobPhaseRunning,
			Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker", Phase: rlarkv1alpha1.TaskPhaseRunning}},
		},
	}
	task := buildTask(job, template, "job-worker", "workers")
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] = "node"
	task.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] = "node"
	task.Spec.Suspended = true
	task.Status.Phase = rlarkv1alpha1.TaskPhaseStopped
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	node.Spec.Unschedulable = true
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == rlarkv1alpha1.ConditionTypeSchedulable {
			node.Status.Conditions[i].Status = metav1.ConditionFalse
			node.Status.Conditions[i].Reason = rlarkv1alpha1.ConditionReasonUnschedulable
		}
	}
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers"},
		Status:     rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline},
	}
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(node, cluster, task).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	changed, err := r.ReconcileStateMachine(context.Background(), job)
	if err != nil {
		t.Fatalf("ReconcileStateMachine() error = %v, want the completed node drain honoured", err)
	}
	if !changed {
		t.Fatal("ReconcileStateMachine() did not report the stopped Job status change")
	}
	if job.Spec.Suspended {
		t.Fatal("node drain changed the user's Job suspension intent")
	}
	if job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || len(job.Status.Tasks) != 1 || job.Status.Tasks[0].Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("Job status did not reflect the stopped Task: %#v", job.Status)
	}
	condition := findCondition(job.Status.Conditions, jobConditionEvicted)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != jobReasonEvicted {
		t.Fatalf("Evicted condition = %#v, want True/%s", condition, jobReasonEvicted)
	}
	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended {
		t.Fatal("Job reconciliation resumed the node-drained Task")
	}
}

func TestResolveTaskNamespaceAllowsDowngradedCluster(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker"}}}}
	cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterDowngraded}}
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(node, cluster).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	var task rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKey{Name: "job-worker", Namespace: "workers"}, &task); err != nil {
		t.Fatal(err)
	}
}

func TestResolveTaskNamespaceRecoversWhenClusterBecomesOnline(t *testing.T) {
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}, Status: rlarkv1alpha1.JobStatus{Tasks: []rlarkv1alpha1.JobTaskStatus{{Name: "worker"}}}}
	cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers", Cordoned: true}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterCordoned}}
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	scheme := jobTestScheme(t)
	c := jobTestClientBuilder(scheme).WithObjects(node, cluster).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err == nil {
		t.Fatal("expected cordoned cluster rejection")
	}
	cluster.Spec.Cordoned = false
	cluster.Status.Phase = rlarkv1alpha1.ClusterOnline
	if err := c.Update(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}
	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	var task rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKey{Name: "job-worker", Namespace: "workers"}, &task); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchResolvesNamespaceBeforeMutating(t *testing.T) {
	replicas := int32(2)
	template := placementRayTaskTemplate(t, "worker", rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "new"}})
	template.Workload.Carrier.Parameters = rayTaskTemplate(t, "worker", false, &replicas, "").Workload.Carrier.Parameters
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}, Spec: rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}}}
	task := buildTask(job, template, "job-worker", "workers")
	controller := true
	task.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}
	node := schedulableNode("node", "workers", map[string]string{"zone": "new"})
	cluster := &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: "workers"}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}}
	scheme := jobTestScheme(t)
	baseClient := jobTestClientBuilder(scheme).WithObjects(task, node, cluster).Build()
	c := &failSecondNodeListClient{Client: baseClient}
	r := &Reconciler{Client: c, Scheme: scheme}
	if _, err := r.dispatchTasks(context.Background(), job, logr.Discard()); err != nil {
		t.Fatal(err)
	}
	if c.lists != 1 {
		t.Fatalf("Node lists = %d, want 1", c.lists)
	}
}
