package job

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	controllerbase "github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

const storageTestCluster = "rlark-ksyun-gpu"

func storageViewConfigMap(t *testing.T, namespace string, view storageref.View) *corev1.ConfigMap {
	t.Helper()
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "sc-" + view.Name + "-storage", Namespace: namespace, Labels: map[string]string{storageref.ViewLabel: "true"}},
		Data:       map[string]string{storageref.ViewKey: string(data)},
	}
}

func storageTemplate(t *testing.T, storages ...string) rlarkv1alpha1.JobTaskTemplate {
	t.Helper()
	template := rayTaskTemplate(t, "head", true, nil, "")
	spec := corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
	for _, storage := range storages {
		spec.Volumes = append(spec.Volumes, corev1.Volume{Name: storage, VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{
			Driver: storageref.Driver, VolumeAttributes: map[string]string{storageref.AttributeStorage: storage},
		}}})
		spec.Containers[0].VolumeMounts = append(spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: storage, MountPath: "/mnt/" + storage, SubPath: "fleet"})
	}
	template.Workload.Template = &corev1.PodTemplateSpec{Spec: spec}
	return template
}

func storageReconciler(t *testing.T, objects ...*corev1.ConfigMap) *Reconciler {
	t.Helper()
	scheme := jobTestScheme(t)
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	builder := jobTestClientBuilder(scheme)
	for _, object := range objects {
		builder = builder.WithObjects(object)
	}
	return &Reconciler{Client: builder.Build(), Scheme: scheme}
}

func TestReconcileTaskResolvesStorageByName(t *testing.T) {
	r := storageReconciler(t,
		storageViewConfigMap(t, storageTestCluster, storageref.View{Name: "ks3", Type: storageref.TypeS3, DefaultProfile: "archive", StorageClasses: map[string]string{"archive": "sc-0123456789abcdef"}}),
		storageViewConfigMap(t, storageTestCluster, storageref.View{Name: "kpfs", Type: storageref.TypeHostPath, StorageClasses: map[string]string{"default": "sc-cccccccccccccccc", "read": "sc-cccccccccccccccc-read"}, NodeSelector: map[string]string{"rlark.io/kpfs": "true"}}),
		// The same name in another cluster does not count.
		storageViewConfigMap(t, "rlark-robot", storageref.View{Name: "nas", Type: storageref.TypeSMB, StorageClasses: map[string]string{"default": "sc-1111111111111111"}}),
	)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := storageTemplate(t, "ks3", "kpfs")

	task, err := r.reconcileTaskInNamespace(context.Background(), job, template, storageTestCluster, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	spec := task.Spec.Workload.Template.Spec
	if class := spec.Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName; class == nil || *class != "sc-0123456789abcdef" {
		t.Fatalf("ks3 volume = %+v", spec.Volumes[0].VolumeSource)
	}
	if class := spec.Volumes[1].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName; class == nil || *class != "sc-cccccccccccccccc" {
		t.Fatalf("kpfs volume = %+v", spec.Volumes[1].VolumeSource)
	}
	if spec.Affinity == nil || spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key != "rlark.io/kpfs" {
		t.Fatalf("affinity = %+v", spec.Affinity)
	}
	if !storageref.IsReference(template.Workload.Template.Spec.Volumes[0]) {
		t.Fatal("the Job's template was rewritten; only the Task gets the resolved volumes")
	}
	// The stored Task equals what the template resolves to, so a reconcile
	// does not rewrite (and restart) it.
	resolved, err := r.resolveTemplateStorage(context.Background(), template, storageTestCluster)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := desiredTask(job, resolved, task.Name, task.Namespace, false)
	if err != nil {
		t.Fatal(err)
	}
	if !taskUpToDate(task, desired) {
		t.Fatal("the stored Task differs from its resolved template")
	}
}

func TestReconcileTaskWaitsForMissingStorage(t *testing.T) {
	r := storageReconciler(t, storageViewConfigMap(t, "rlark-robot", storageref.View{Name: "nas", Type: storageref.TypeSMB, StorageClasses: map[string]string{"default": "sc-1111111111111111"}}))
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}

	_, err := r.reconcileTaskInNamespace(context.Background(), job, storageTemplate(t, "nas"), storageTestCluster, logr.Discard())
	var unavailable *storageUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err = %v, want a storage-unavailable error", err)
	}
	if want := `task "head": volume "nas": storage "nas" is not available in cluster "rlark-ksyun-gpu"`; unavailable.message != want {
		t.Fatalf("message = %q, want %q", unavailable.message, want)
	}
	var task rlarkv1alpha1.Task
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: storageTestCluster, Name: "job-head"}, &task); !apierrors.IsNotFound(err) {
		t.Fatalf("Task was created without its storage: %v", err)
	}

	if !setStorageCondition(job, err) {
		t.Fatal("condition not set")
	}
	condition := apimeta.FindStatusCondition(job.Status.Conditions, jobConditionStorageAvailable)
	if condition.Status != metav1.ConditionFalse || condition.Reason != jobReasonStorageUnavailable || condition.Message != unavailable.message {
		t.Fatalf("condition = %+v", condition)
	}
	if !setStorageCondition(job, nil) || apimeta.FindStatusCondition(job.Status.Conditions, jobConditionStorageAvailable).Status != metav1.ConditionTrue {
		t.Fatal("condition did not turn true once the storage resolves")
	}
}

func TestReconcileTaskKeepsRunningTaskWhenStorageDisappears(t *testing.T) {
	view := storageViewConfigMap(t, storageTestCluster, storageref.View{Name: "ks3", Type: storageref.TypeS3, DefaultProfile: "default", StorageClasses: map[string]string{"default": "sc-0123456789abcdef"}})
	r := storageReconciler(t, view)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid")}}
	template := storageTemplate(t, "ks3")
	task, err := r.reconcileTaskInNamespace(context.Background(), job, template, storageTestCluster, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(context.Background(), view); err != nil {
		t.Fatal(err)
	}
	_, err = r.reconcileTaskInNamespace(context.Background(), job, template, storageTestCluster, logr.Discard())
	var unavailable *storageUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err = %v, want a storage-unavailable error", err)
	}
	var kept rlarkv1alpha1.Task
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: storageTestCluster, Name: task.Name}, &kept); err != nil {
		t.Fatal(err)
	}
	if kept.ResourceVersion != task.ResourceVersion {
		t.Fatal("the running Task was changed after its storage disappeared")
	}
}

func TestReconcileRecordsMissingStorageAndRetries(t *testing.T) {
	template := storageTemplate(t, "kpfs")
	template.Placement.NodeSelector = map[string]string{"rlark.io/cluster-id": storageTestCluster}
	template.Runtime.Kind = rlarkv1alpha1.RuntimeKindKubernetes
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: types.UID("job-uid"), Finalizers: []string{CleanupFinalizer}, Generation: 1},
		Spec:       rlarkv1alpha1.JobSpec{Tasks: []rlarkv1alpha1.JobTaskTemplate{template}},
	}
	scheme := jobTestScheme(t)
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := jobTestClientBuilder(scheme).WithStatusSubresource(job).WithObjects(job,
		schedulableNode("n", storageTestCluster, template.Placement.NodeSelector),
		&rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: storageTestCluster}, Spec: rlarkv1alpha1.ClusterSpec{AgentNamespace: storageTestCluster}, Status: rlarkv1alpha1.ClusterStatus{Phase: rlarkv1alpha1.ClusterOnline}},
	).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}})
	if err != nil || result.RequeueAfter != controllerbase.WaitingRequeue {
		t.Fatalf("Reconcile() = %+v, %v; want a retry after %v", result, err, controllerbase.WaitingRequeue)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	condition := apimeta.FindStatusCondition(job.Status.Conditions, jobConditionStorageAvailable)
	if condition == nil || condition.Status != metav1.ConditionFalse ||
		condition.Message != `task "head": volume "kpfs": storage "kpfs" is not available in cluster "rlark-ksyun-gpu"` {
		t.Fatalf("StorageAvailable = %+v, want False naming the storage and the cluster", condition)
	}
	var tasks rlarkv1alpha1.TaskList
	if err := c.List(context.Background(), &tasks); err != nil || len(tasks.Items) != 0 {
		t.Fatalf("Tasks = %d, %v; want none before the storage is available", len(tasks.Items), err)
	}
}
