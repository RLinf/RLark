package task

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

type countingClient struct {
	client.Client
	updates       int
	statusPatches int
}

type failingWriteClient struct {
	client.Client
	createErr error
	updateErr error
}

func (c *failingWriteClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createErr != nil {
		return c.createErr
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *failingWriteClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, obj, opts...)
}

func (c *countingClient) Status() client.StatusWriter {
	return &countingTaskStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type countingTaskStatusWriter struct {
	client.SubResourceWriter
	parent *countingClient
}

func (w *countingTaskStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	w.parent.statusPatches++
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}

func TestAppendSSHPublicKeyToRunningPodsRequiresKubeConfig(t *testing.T) {
	r := &pullReconciler{c: NewTaskController(base.Controller{
		LocalKubeClient: fake.NewClientBuilder().Build(),
	})}
	err := r.appendSSHPublicKeyToRunningPods(context.Background(), &rlarkv1alpha1.Task{}, "default", nil)
	if err == nil {
		t.Fatal("appendSSHPublicKeyToRunningPods() error = nil, want local Kubernetes config error")
	}
}

func TestAppendSSHPublicKeyToPodRequiresValidConfig(t *testing.T) {
	err := appendSSHPublicKeyToPod(context.Background(), &rest.Config{Host: "://invalid"}, "default", "pod", "ssh-ed25519 AAAA")
	if err == nil {
		t.Fatal("appendSSHPublicKeyToPod() error = nil, want Kubernetes client error")
	}
}

func TestTaskPullConcurrency(t *testing.T) {
	if got := NewTaskController(base.Controller{PullMaxConcurrentReconciles: 4}).PullMaxConcurrentReconciles; got != 4 {
		t.Fatalf("configured Task pull concurrency = %d, want 4", got)
	}
}

func TestEnsureRayResourcesSkipsUnchangedConfigMapUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "worker", Annotations: map[string]string{rlarkv1alpha1.RayRoleAnnotation: "worker"},
	}}
	cm := buildRayConfigMap("rlark-system", "worker")
	wrapped := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()}
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: wrapped})}

	if err := r.ensureRayResources(context.Background(), mgmtTask, nil); err != nil {
		t.Fatal(err)
	}
	if wrapped.updates != 0 {
		t.Fatalf("unchanged Ray ConfigMap caused %d updates, want 0", wrapped.updates)
	}

	cm.Data = map[string]string{"changed": "true"}
	if err := wrapped.Client.Update(context.Background(), cm); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureRayResources(context.Background(), mgmtTask, nil); err != nil {
		t.Fatal(err)
	}
	if wrapped.updates != 1 {
		t.Fatalf("changed Ray ConfigMap caused %d updates, want 1", wrapped.updates)
	}
}

func TestEnsureRayResourcesReusesServiceFromPreviousTask(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "head", Namespace: "default", UID: "new-uid",
		Annotations: map[string]string{rlarkv1alpha1.RayRoleAnnotation: rlarkv1alpha1.RayRoleHead},
	}}
	service := buildRayHeadService("rlark-system", task.Name)
	service.Annotations = map[string]string{ManagementTaskUIDAnnotation: "old-uid"}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(service).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}

	if err := r.ensureRayResources(context.Background(), task, nil); err != nil {
		t.Fatalf("reuse Ray Service: %v", err)
	}
}

func TestEnsureImagePullSecretsUsesAllMatchingLocalCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	credential := func(name, registry string, labeled bool) *corev1.Secret {
		labels := map[string]string{}
		if labeled {
			labels[common.ImageRegistryCredentialLabel] = "true"
		}
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "rlark-system", Labels: labels,
			Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: registry},
		}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("config")}}
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		credential("ir-z", "registry.example.com", true),
		credential("ir-a", "registry.example.com", true),
		credential("ir-team", "registry.example.com/team", true),
		credential("unrelated", "registry.example.com", false),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "malformed", Namespace: "rlark-system", Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.example.com"}}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "wrong-namespace", Namespace: "other", Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.example.com"}}},
	).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{
		LocalKubeClient: local,
	})}
	template := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers:       []corev1.Container{{Image: "registry.example.com/team/app:v1"}},
		InitContainers:   []corev1.Container{{Image: "registry.example.com/init:v1"}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "user-secret"}, {Name: "ir-z"}},
	}}
	if err := r.ensureImagePullSecrets(context.Background(), &template, "rlark-system"); err != nil {
		t.Fatal(err)
	}
	want := []corev1.LocalObjectReference{{Name: "user-secret"}, {Name: "ir-z"}, {Name: "ir-a"}, {Name: "ir-team"}}
	if !reflect.DeepEqual(template.Spec.ImagePullSecrets, want) {
		t.Fatalf("imagePullSecrets = %#v, want %#v", template.Spec.ImagePullSecrets, want)
	}

	var secrets corev1.SecretList
	if err := local.List(context.Background(), &secrets); err != nil {
		t.Fatal(err)
	}
	if len(secrets.Items) != 6 {
		t.Fatalf("local secret count = %d, want 6", len(secrets.Items))
	}
}

func TestReconcileAddsFinalizerAndCreatesStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 3},
		Spec: rlarkv1alpha1.TaskSpec{
			AgentType: rlarkv1alpha1.AgentTypeKubernetes,
			Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{
				Kind: rlarkv1alpha1.KubernetesWorkloadStatefulSet,
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "example/image:latest"}},
				}},
			}},
		},
	}
	managementClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{
		ManagementNamespace: "cluster-1",
		AgentType:           string(rlarkv1alpha1.AgentTypeKubernetes),
		ManagementClient:    managementClient,
		LocalKubeClient:     localClient,
	})}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "task", Namespace: "default"}}); err != nil {
		t.Fatal(err)
	}

	var updatedTask rlarkv1alpha1.Task
	if err := managementClient.Get(context.Background(), client.ObjectKeyFromObject(task), &updatedTask); err != nil {
		t.Fatal(err)
	}
	if len(updatedTask.Finalizers) != 1 || updatedTask.Finalizers[0] != ManagementTaskFinalizer {
		t.Fatalf("Task finalizers = %v, want %q", updatedTask.Finalizers, ManagementTaskFinalizer)
	}
	if got := updatedTask.Annotations[ManagementTaskClaimantAnnotation]; got != "cluster-1/Kubernetes" {
		t.Fatalf("Task claimant = %q", got)
	}

	var sts appsv1.StatefulSet
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, &sts); err != nil {
		t.Fatalf("StatefulSet was not created in the first reconcile: %v", err)
	}
	if got := sts.Annotations[ManagementTaskGenerationAnnotation]; got != "3" {
		t.Fatalf("StatefulSet Task generation annotation = %q, want 3", got)
	}
}

func TestCreateOrUpdateWorkloadUsesTaskGeneration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	existing := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "rlark-system",
		Annotations: map[string]string{
			ManagementTaskGenerationAnnotation: "2",
			ManagementTaskNameAnnotation:       "task",
			ManagementTaskNamespaceAnnotation:  "default",
			ManagementTaskUIDAnnotation:        "uid",
		},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}
	mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "default", UID: "uid", Generation: 2, ResourceVersion: "different",
	}}
	updates := 0
	applyUpdate := func(client.Object, client.Object) { updates++ }
	newWorkload := func() *appsv1.StatefulSet {
		return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system"}}
	}

	if _, err := r.createOrUpdateWorkload(context.Background(), mgmtTask, "StatefulSet", &appsv1.StatefulSet{}, newWorkload(), applyUpdate); err != nil {
		t.Fatal(err)
	}
	if updates != 0 {
		t.Fatalf("same Task generation caused %d workload updates, want 0", updates)
	}

	mgmtTask.Generation = 3
	if _, err := r.createOrUpdateWorkload(context.Background(), mgmtTask, "StatefulSet", &appsv1.StatefulSet{}, newWorkload(), applyUpdate); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("changed Task generation caused %d workload updates, want 1", updates)
	}
}

func TestWorkloadKindMigrationResetsStatusAndPreservesOtherFields(t *testing.T) {
	for _, phase := range []rlarkv1alpha1.TaskPhase{rlarkv1alpha1.TaskPhaseRunning, rlarkv1alpha1.TaskPhaseFailed, rlarkv1alpha1.TaskPhaseStopped} {
		t.Run(string(phase), func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = rlarkv1alpha1.AddToScheme(scheme)
			task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 4}, Status: rlarkv1alpha1.TaskStatus{
				Phase: phase, ObservedNodes: []string{"old-node"}, Message: "keep", RetryCount: 3,
				Conditions: []metav1.Condition{{Type: "Other", Status: metav1.ConditionTrue, Reason: "Keep"}},
			}}
			old := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{
				ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid",
			}}}
			management := &countingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Task{}).WithObjects(task).Build()}
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(old).Build()
			r := &pullReconciler{c: NewTaskController(base.Controller{ManagementClient: management, LocalKubeClient: local})}

			pending, err := r.deleteOtherWorkloadKinds(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, "Deployment", task)
			if err != nil || !pending {
				t.Fatalf("deleteOtherWorkloadKinds() = %v, %v", pending, err)
			}
			var got rlarkv1alpha1.Task
			if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
				t.Fatal(err)
			}
			if got.Status.Phase != rlarkv1alpha1.TaskPhasePending || len(got.Status.ObservedNodes) != 0 || got.Status.Message != "keep" || got.Status.RetryCount != 3 || len(got.Status.Conditions) != 2 {
				t.Fatalf("migration status = %#v", got.Status)
			}
			condition := got.Status.Conditions[1]
			if condition.Type != WorkloadProgressingCondition || condition.Status != metav1.ConditionTrue || condition.Reason != WorkloadKindMigrationReason {
				t.Fatalf("migration condition = %#v", condition)
			}
			if management.statusPatches != 1 {
				t.Fatalf("status patches = %d, want 1", management.statusPatches)
			}
			if _, err := r.deleteOtherWorkloadKinds(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, "Deployment", &got); err != nil {
				t.Fatal(err)
			}
			if management.statusPatches != 1 {
				t.Fatalf("unchanged migration status patched again: %d", management.statusPatches)
			}
		})
	}
}

func TestTargetWorkloadClearsMigrationCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 2}, Status: rlarkv1alpha1.TaskStatus{
		Phase: rlarkv1alpha1.TaskPhasePending, Message: "keep", Conditions: []metav1.Condition{{Type: WorkloadProgressingCondition, Status: metav1.ConditionTrue, Reason: WorkloadKindMigrationReason}},
	}}
	target := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{
		ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid", ManagementTaskGenerationAnnotation: "2",
	}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Task{}).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(target).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{ManagementClient: management, LocalKubeClient: local})}
	if _, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, target.DeepCopy(), func(client.Object, client.Object) {}); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Message != "keep" || !reflect.DeepEqual(got.Status.Conditions, []metav1.Condition(nil)) {
		t.Fatalf("target status = %#v", got.Status)
	}
	if got.Status.Phase != rlarkv1alpha1.TaskPhasePending || len(got.Status.ObservedNodes) != 0 {
		t.Fatalf("phase and observed nodes should remain reset until push observes the target: %#v", got.Status)
	}
}

func TestTargetWorkloadCreateClearsMigrationConditionAfterSuccess(t *testing.T) {
	task, management, local, r := migrationTestReconciler(t, nil)
	if _, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) {}); err != nil {
		t.Fatal(err)
	}
	assertMigrationCondition(t, management, task, false)
	var target appsv1.Deployment
	if err := local.Get(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, &target); err != nil {
		t.Fatalf("target was not created: %v", err)
	}
}

func TestTargetWorkloadFailuresRetainMigrationCondition(t *testing.T) {
	t.Run("conflict", func(t *testing.T) {
		conflict := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskUIDAnnotation: "other"}}}
		task, management, _, r := migrationTestReconciler(t, conflict)
		if _, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) {}); err == nil {
			t.Fatal("expected ownership conflict")
		}
		assertMigrationCondition(t, management, task, true)
	})

	t.Run("update", func(t *testing.T) {
		existing := buildDeployment(&rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 1}}, &rlarkv1alpha1.KubernetesWorkloadSpec{})
		task, management, local, r := migrationTestReconciler(t, existing)
		r.c.LocalKubeClient = &failingWriteClient{Client: local, updateErr: fmt.Errorf("update failed")}
		if _, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(existing, desired client.Object) {
			existing.(*appsv1.Deployment).Spec = desired.(*appsv1.Deployment).Spec
		}); err == nil {
			t.Fatal("expected update failure")
		}
		assertMigrationCondition(t, management, task, true)
	})

	t.Run("create", func(t *testing.T) {
		task, management, local, r := migrationTestReconciler(t, nil)
		r.c.LocalKubeClient = &failingWriteClient{Client: local, createErr: fmt.Errorf("create failed")}
		if _, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) {}); err == nil {
			t.Fatal("expected create failure")
		}
		assertMigrationCondition(t, management, task, true)
	})
}

func migrationTestReconciler(t *testing.T, workload client.Object) (*rlarkv1alpha1.Task, client.Client, client.Client, *pullReconciler) {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 2}, Status: rlarkv1alpha1.TaskStatus{
		Phase:      rlarkv1alpha1.TaskPhasePending,
		Conditions: []metav1.Condition{{Type: WorkloadProgressingCondition, Status: metav1.ConditionTrue, Reason: WorkloadKindMigrationReason}},
	}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Task{}).WithObjects(task).Build()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if workload != nil {
		builder = builder.WithObjects(workload)
	}
	local := builder.Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{ManagementClient: management, LocalKubeClient: local})}
	return task, management, local, r
}

func assertMigrationCondition(t *testing.T, management client.Client, task *rlarkv1alpha1.Task, want bool) {
	t.Helper()
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, condition := range got.Status.Conditions {
		found = found || condition.Type == WorkloadProgressingCondition && condition.Reason == WorkloadKindMigrationReason
	}
	if found != want {
		t.Fatalf("migration condition present = %v, want %v: %#v", found, want, got.Status)
	}
}

func TestCleanupWorkloadWaitsForStatefulSetBeforeDeletingPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "rlark-system", Finalizers: []string{"test/finalizer"},
		Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid"},
	}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "data", Namespace: "rlark-system", Finalizers: []string{"test/finalizer"},
		Labels:      map[string]string{PVCTaskLabel: "task"},
		Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid"},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, pvc).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}

	pending, err := r.cleanupWorkload(context.Background(), &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("cleanup should remain pending while StatefulSet exists")
	}

	var remainingSTS appsv1.StatefulSet
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, &remainingSTS); err != nil {
		t.Fatalf("StatefulSet should still exist until its finalizer is cleared: %v", err)
	}
	var remainingPVC corev1.PersistentVolumeClaim
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "data", Namespace: "rlark-system"}, &remainingPVC); err != nil {
		t.Fatalf("PVC should not be deleted before the StatefulSet and its Pods are gone: %v", err)
	}
	if !remainingPVC.DeletionTimestamp.IsZero() {
		t.Fatal("PVC deletion started before StatefulSet foreground deletion completed")
	}
	if remainingSTS.DeletionTimestamp.IsZero() {
		t.Fatal("StatefulSet deletion was not requested")
	}
}

func TestCleanupWorkloadDeletesPVCAfterWorkloadIsGone(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "data", Namespace: "rlark-system", Finalizers: []string{"test/finalizer"},
		Labels:      map[string]string{PVCTaskLabel: "task"},
		Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid"},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}

	pending, err := r.cleanupWorkload(context.Background(), &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("cleanup should remain pending while PVC exists")
	}
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "data", Namespace: "rlark-system"}, pvc); err != nil {
		t.Fatal(err)
	}
	if pvc.DeletionTimestamp.IsZero() {
		t.Fatal("PVC deletion was not requested after workloads disappeared")
	}
}

func TestEnsurePVCsUsesConfiguredSize(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}
	mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default"}}
	//nolint:staticcheck // Support for deprecated PVC storage class mapping
	workload := &rlarkv1alpha1.KubernetesWorkloadSpec{
		PvcSizeGbMap: map[string]int32{"data": 20},
		Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name: "data",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "data",
			}},
		}}}},
	}

	if err := r.ensurePVCs(context.Background(), mgmtTask, workload); err != nil {
		t.Fatal(err)
	}
	var pvc corev1.PersistentVolumeClaim
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "data", Namespace: "rlark-system"}, &pvc); err != nil {
		t.Fatal(err)
	}
	if got := pvc.Spec.Resources.Requests.Storage().String(); got != "20Gi" {
		t.Fatalf("PVC storage request = %q, want 20Gi", got)
	}
}

func TestEnsurePVCsReusesClaimFromPreviousTask(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "data", Namespace: "rlark-system",
		Annotations: map[string]string{ManagementTaskUIDAnnotation: "old-uid"},
	}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "new-uid"}}
	workload := &rlarkv1alpha1.KubernetesWorkloadSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
		Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}},
	}}}}}

	if err := r.ensurePVCs(context.Background(), task, workload); err != nil {
		t.Fatalf("reuse PVC: %v", err)
	}
}

func TestEnsurePVCsChecksImmutableStorageClassBeforeAdoption(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	oldClass := "old"
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "rlark-system", Labels: map[string]string{PVCTaskLabel: "task"}, Annotations: map[string]string{PVCOwnerAnnotation: "task", PVCOwnerTaskAnnotation: "task"}}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &oldClass}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}
	//nolint:staticcheck // Support for deprecated PVC storage class mapping
	workload := &rlarkv1alpha1.KubernetesWorkloadSpec{
		PvcStorageMap: map[string]string{"data": "new"},
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}},
				},
			},
		},
	}
	if err := r.ensurePVCs(context.Background(), task, workload); err == nil {
		t.Fatal("expected immutable storage class conflict")
	}
	var got corev1.PersistentVolumeClaim
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(pvc), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.StorageClassName == nil || *got.Spec.StorageClassName != oldClass {
		t.Fatalf("PVC was modified before conflict: annotations=%v storageClass=%v", got.Annotations, got.Spec.StorageClassName)
	}
}

func TestRestartCleanupRequiredAndAnnotationPropagation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	oldSTS := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "rlark-system",
		Annotations: map[string]string{RestartedAtAnnotation: "old"},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(oldSTS).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}
	mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "default", ResourceVersion: "2",
		Annotations: map[string]string{RestartedAtAnnotation: "new"},
	}}

	required, err := r.restartCleanupRequired(context.Background(), mgmtTask, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("new restart annotation should require cleanup")
	}
	built := buildStatefulSet(mgmtTask, &rlarkv1alpha1.KubernetesWorkloadSpec{})
	if got := built.Annotations[RestartedAtAnnotation]; got != "new" {
		t.Fatalf("StatefulSet restart annotation = %q, want new", got)
	}

	oldSTS.Annotations[RestartedAtAnnotation] = "new"
	if err := localClient.Update(context.Background(), oldSTS); err != nil {
		t.Fatal(err)
	}
	required, err = r.restartCleanupRequired(context.Background(), mgmtTask, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if required {
		t.Fatal("matching restart annotation should make rebuild idempotent")
	}
}

func TestApplyRLarkToolsAlwaysInjectsWithoutSSHKey(t *testing.T) {
	template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}}

	applyRLarkTools(template, "rlinf/rlark:test")

	if len(template.Spec.InitContainers) != 1 {
		t.Fatalf("init containers = %d, want 1", len(template.Spec.InitContainers))
	}
	init := template.Spec.InitContainers[0]
	if init.Name != rlarkToolsInitContainerName {
		t.Fatalf("init container name = %q, want %q", init.Name, rlarkToolsInitContainerName)
	}
	if len(init.Command) != 3 || init.Command[1] != rlarkToolsBinPath || init.Command[2] != rlarkToolsBinDst {
		t.Fatalf("init container command = %v", init.Command)
	}
	if len(template.Spec.Containers[0].VolumeMounts) != 1 || template.Spec.Containers[0].VolumeMounts[0].MountPath != rlarkToolsDstDir {
		t.Fatalf("main container volume mounts = %v", template.Spec.Containers[0].VolumeMounts)
	}
}

func TestApplyTemplateMutationsUnsafeTaskPrivileges(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		privileged  bool
		hostNetwork bool
	}{
		{name: "disabled by default"},
		{name: "only exact true enables", env: "TRUE"},
		{name: "enabled", env: "true", privileged: true, hostNetwork: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(UnsafeTaskPrivilegesEnv, tt.env)
			template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers:     []corev1.Container{{Name: "main"}},
				InitContainers: []corev1.Container{{Name: "init"}},
			}}
			mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task"}}

			applyTemplateMutations(template, mgmtTask, "")

			if template.Spec.HostNetwork != tt.hostNetwork {
				t.Fatalf("hostNetwork = %v, want %v", template.Spec.HostNetwork, tt.hostNetwork)
			}
			for _, container := range append(template.Spec.Containers, template.Spec.InitContainers...) {
				got := container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged
				if got != tt.privileged {
					t.Fatalf("container %q privileged = %v, want %v", container.Name, got, tt.privileged)
				}
			}
		})
	}
}
