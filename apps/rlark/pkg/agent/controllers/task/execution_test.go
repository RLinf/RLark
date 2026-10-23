package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	compatibility "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	kubernetesenvironment "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes"
	kubernetesssh "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/ssh"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func claimedExecutionTask(phase rlarkv1alpha1.TaskPhase) *rlarkv1alpha1.Task {
	parameters, _ := json.Marshal(taskapi.RayExecutionParametersV1{Role: taskapi.RayRoleWorker})
	carrierParameters, _ := json.Marshal(taskapi.KubernetesReplicatedCarrierParametersV1{})
	return &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "task",
			Namespace:  "default",
			UID:        "uid",
			Generation: 3,
			Finalizers: []string{ManagementTaskFinalizer},
			Annotations: map[string]string{
				ManagementTaskClaimantAnnotation:          "cluster/Kubernetes",
				host.TaskClaimedWorkloadTypeAnnotation:    taskapi.WorkloadTypeRay,
				host.TaskClaimedWorkloadVersionAnnotation: taskapi.WorkloadVersionRayV1,
				host.TaskClaimedCarrierTypeAnnotation:     taskapi.CarrierTypeKubernetesDeployment,
				host.TaskClaimedCarrierVersionAnnotation:  taskapi.CarrierVersionKubernetesV1,
			},
		},
		Spec: rlarkv1alpha1.TaskSpec{
			Placement: rlarkv1alpha1.TaskPlacementSpec{},
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{
				Kind: rlarkv1alpha1.RuntimeKindKubernetes,
			},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Kind:       taskapi.WorkloadTypeRay,
				Version:    taskapi.WorkloadVersionRayV1,
				Parameters: apiextensionsv1.JSON{Raw: parameters},
				Template:   &corev1.PodTemplateSpec{},
				Carrier: rlarkv1alpha1.TaskWorkloadCarrier{
					Type:       taskapi.CarrierTypeKubernetesDeployment,
					Version:    taskapi.CarrierVersionKubernetesV1,
					Parameters: apiextensionsv1.JSON{Raw: carrierParameters},
				},
			},
		},
		Status: rlarkv1alpha1.TaskStatus{
			Phase: phase,
		},
	}
}

func TestKubernetesExecutionCoordinatorEvaluatesAllBuiltinWorkloads(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	carriers := []struct {
		name   string
		typeID string
	}{
		{name: "Deployment", typeID: taskapi.CarrierTypeKubernetesDeployment},
		{name: "DaemonSet", typeID: taskapi.CarrierTypeKubernetesDaemonSet},
		{name: "StatefulSet", typeID: taskapi.CarrierTypeKubernetesStatefulSet},
	}
	for _, carrier := range carriers {
		t.Run(carrier.name, func(t *testing.T) {
			task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
			task.Generation = 3
			task.Spec.Workload.Carrier.Type = carrier.typeID
			task.Annotations[host.TaskClaimedCarrierTypeAnnotation] = carrier.typeID
			if carrier.typeID == taskapi.CarrierTypeKubernetesDaemonSet {
				task.Spec.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: []byte(`{}`)}
			}
			task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
			snapshot, err := compatibility.DecodeTask(task)
			if err != nil {
				t.Fatal(err)
			}
			result, err := execution.Evaluate(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Operations) == 0 || result.Plan.Digests.ExecutableHash == "" {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestKubernetesExecutionCoordinatorRegistersProviderCapabilities(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	execution, err := NewKubernetesExecutionCoordinator(fake.NewClientBuilder().WithScheme(scheme).Build(), "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Runtimes) != 1 {
		t.Fatalf("runtimes=%#v, want one Kubernetes Runtime", execution.Runtimes)
	}
	capabilities := execution.Runtimes[0].Info.Capabilities
	for _, required := range []agentruntime.Capability{agentruntime.CapabilityIncarnationAdoption, agentruntime.CapabilityReconcileProfiles} {
		if !slices.Contains(capabilities, required) {
			t.Fatalf("Runtime capabilities=%v, missing %q", capabilities, required)
		}
	}
}

func TestKubernetesExecutionCoordinatorAssemblesEnvironmentBinder(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	execution, err := NewKubernetesExecutionCoordinator(fake.NewClientBuilder().WithScheme(scheme).Build(), "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := execution.Resolver.(kubernetesenvironment.Resolver); !ok {
		t.Fatalf("Resolver=%T, want composed Kubernetes Environment Resolver", execution.Resolver)
	}
	if _, ok := execution.Binder.(binding.BindingPlanBinder); !ok {
		t.Fatalf("Binder=%T, want BindingPlanBinder", execution.Binder)
	}
}

func TestKubernetesExecutionCoordinatorBindsSSHEnvironment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	execution, err := NewKubernetesExecutionCoordinator(fake.NewClientBuilder().WithScheme(scheme).Build(), "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"authorizedKeys":["ssh-ed25519 public-key"]}`)}}}
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	result, err := execution.Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var secret corev1.Secret
	var workload appsv1.Deployment
	foundSecret, foundWorkload := false, false
	for _, object := range result.Plan.Objects {
		if object.LogicalName == kubernetesssh.SecretLogicalName {
			if err := json.Unmarshal(object.Manifest.Object, &secret); err != nil {
				t.Fatal(err)
			}
			foundSecret = true
		}
		if object.Kind.Group == appsv1.GroupName && object.Kind.Kind == "Deployment" {
			if err := json.Unmarshal(object.Manifest.Object, &workload); err != nil {
				t.Fatal(err)
			}
			foundWorkload = true
		}
	}
	if !foundSecret || string(secret.Data[kubernetesssh.SecretDataKey]) != "ssh-ed25519 public-key\n" || !foundWorkload {
		t.Fatalf("SSH binding missing: secret=%t data=%q workload=%t", foundSecret, secret.Data[kubernetesssh.SecretDataKey], foundWorkload)
	}
	encoded, _ := json.Marshal(workload.Spec.Template.Spec)
	if !strings.Contains(string(encoded), kubernetesssh.VolumeName) || !strings.Contains(string(encoded), "supervisor") {
		t.Fatalf("SSH workload projection missing: %s", encoded)
	}
}

func TestKubernetesExecutionCoordinatorRejectsUnknownEnvironment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	execution, err := NewKubernetesExecutionCoordinator(fake.NewClientBuilder().WithScheme(scheme).Build(), "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: "example.io/unknown", Version: "v1", Parameters: apiextensionsv1.JSON{Raw: []byte(`{}`)}}}
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.Evaluate(context.Background(), snapshot); err == nil {
		t.Fatal("unknown Environment was accepted by the Resolver")
	}
}

func TestConfiguredTensorBoardImage(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(tensorBoardImageEnv, "")
		if got := configuredTensorBoardImage(); got != defaultTensorBoardImage {
			t.Fatalf("image=%q, want default %q", got, defaultTensorBoardImage)
		}
	})
	t.Run("configured", func(t *testing.T) {
		t.Setenv(tensorBoardImageEnv, "registry.example/tensorboard:v1")
		if got := configuredTensorBoardImage(); got != "registry.example/tensorboard:v1" {
			t.Fatalf("image=%q, want configured image", got)
		}
	})
	t.Run("blank falls back", func(t *testing.T) {
		t.Setenv(tensorBoardImageEnv, "  ")
		if got := configuredTensorBoardImage(); got != defaultTensorBoardImage {
			t.Fatalf("image=%q, want default %q", got, defaultTensorBoardImage)
		}
	})
}

func TestKubernetesExecutionCoordinatorUsesConfiguredTensorBoardImage(t *testing.T) {
	t.Setenv(tensorBoardImageEnv, "registry.example/tensorboard:v1")
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	execution, err := NewKubernetesExecutionCoordinator(fake.NewClientBuilder().WithScheme(scheme).Build(), "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: taskapi.EnvironmentTypeTensorBoard, Version: taskapi.EnvironmentVersionTensorBoardV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"logDir":"/logs"}`)}}}
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	result, err := execution.Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var workload appsv1.Deployment
	found := false
	for _, object := range result.Plan.Objects {
		if object.Kind.Group == appsv1.GroupName && object.Kind.Kind == "Deployment" {
			if err := json.Unmarshal(object.Manifest.Object, &workload); err != nil {
				t.Fatal(err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("workload Deployment was not found in executable plan")
	}
	for _, container := range workload.Spec.Template.Spec.InitContainers {
		if container.Name == "tensorboard" {
			if container.Image != "registry.example/tensorboard:v1" {
				t.Fatalf("TensorBoard image=%q, want configured image", container.Image)
			}
			return
		}
	}
	t.Fatal("configured TensorBoard sidecar was not bound")
}

func TestNewKubernetesHostDoesNotInstallOrMutateDependencies(t *testing.T) {
	scheme := executionScheme(t)
	management := fake.NewClientBuilder().WithScheme(scheme).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	implementations := execution.Implementations
	controller := NewTaskController(base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local})

	taskHost, err := NewKubernetesHost(controller, execution)
	if err != nil {
		t.Fatal(err)
	}
	if taskHost == nil {
		t.Fatal("NewKubernetesHost returned nil Host")
	}
	if controller.host != nil || controller.status != nil || controller.execution != nil {
		t.Fatalf("constructor installed dependencies: host=%v status=%v execution=%v", controller.host, controller.status, controller.execution)
	}
	if execution.Implementations != implementations {
		t.Fatal("constructor replaced the Coordinator implementation registry")
	}

	controller.WithExecutionCoordinator(execution).WithHost(taskHost)
	if controller.host != taskHost || controller.status != taskHost.Status || controller.execution != execution {
		t.Fatal("explicit controller installation did not retain constructed dependencies")
	}
}

func TestTaskReconcileLogSuppressesOnlyRepeatedInProgressResults(t *testing.T) {
	controller := &Controller{}
	now := time.Now()
	inProgress := host.Result{Outcome: host.OutcomeCleanupInProgress, Retry: host.RetryDirective{Class: host.RetryInProgress}, Reason: "TaskDeletionCleanup"}
	if !controller.shouldLogTaskReconcile("default/task", inProgress, now) {
		t.Fatal("first in-progress result was suppressed")
	}
	if controller.shouldLogTaskReconcile("default/task", inProgress, now.Add(CleanupRequeueInterval)) {
		t.Fatal("repeated in-progress result was not suppressed")
	}
	controller.clearTaskReconcileLog("default/task")
	if !controller.shouldLogTaskReconcile("default/task", inProgress, now.Add(CleanupRequeueInterval)) {
		t.Fatal("first result after an error recovery was suppressed")
	}
	changed := inProgress
	changed.Reason = "TaskSuspended"
	if !controller.shouldLogTaskReconcile("default/task", changed, now.Add(2*CleanupRequeueInterval)) {
		t.Fatal("changed in-progress reason was suppressed")
	}
	if !controller.shouldLogTaskReconcile("default/task", changed, now.Add(taskReconcileLogInterval+2*CleanupRequeueInterval)) {
		t.Fatal("periodic in-progress summary was suppressed")
	}
	succeeded := host.Result{Outcome: host.OutcomeSucceeded, Retry: host.RetryDirective{Class: host.RetryNone}, Reason: "Reconciled"}
	if !controller.shouldLogTaskReconcile("default/task", succeeded, now) {
		t.Fatal("first non-in-progress result was suppressed")
	}
	if !controller.shouldLogTaskReconcile("default/task", succeeded, now) {
		t.Fatal("repeated non-in-progress result was suppressed")
	}
	controller.logMu.Lock()
	defer controller.logMu.Unlock()
	if _, found := controller.logState["default/task"]; found {
		t.Fatal("terminal result did not clear suppression state")
	}
}

func TestKubernetesHostDoesNotClaimUnregisteredWorkloadCombination(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Finalizers = nil
	task.Annotations = nil
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	implementations, err := newHostImplementationRegistry(execution.Workloads[1:])
	if err != nil {
		t.Fatal(err)
	}
	execution.Implementations = implementations
	if execution.Implementations != implementations {
		t.Fatal("failed to install test implementation registry")
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
	if _, err := (&pullReconciler{c: controller}).Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), request.NamespacedName, &current); err != nil {
		t.Fatal(err)
	}
	if containsString(current.Finalizers, ManagementTaskFinalizer) || current.Annotations[ManagementTaskClaimantAnnotation] != "" {
		t.Fatalf("unregistered combination was claimed: %#v", current.ObjectMeta)
	}
	if local.creates != 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("unregistered combination touched Runtime: create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
	}
}

func TestProductionCoordinatorIsSoleWorkloadWriter(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Generation = 3
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	reconciler := &pullReconciler{c: controller}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if local.creates == 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("writes create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
	}
	creates := local.creates
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if local.creates != creates || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("same generation wrote again: create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
	}
}

func TestPullReconcileAppliesTemplateMutationsOnce(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleHead
	task.Spec.Workload.Carrier.Type = taskapi.CarrierTypeKubernetesStatefulSet
	task.Annotations[host.TaskClaimedCarrierTypeAnnotation] = taskapi.CarrierTypeKubernetesStatefulSet
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local, Image: "rlark:test"}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var workload appsv1.StatefulSet
	if err := local.Get(context.Background(), client.ObjectKey{Namespace: tenant.DefaultNamespace, Name: task.Name}, &workload); err != nil {
		t.Fatal(err)
	}
	volumes, mounts, initContainers := map[string]int{}, map[string]int{}, map[string]int{}
	for _, volume := range workload.Spec.Template.Spec.Volumes {
		volumes[volume.Name]++
	}
	for _, mount := range workload.Spec.Template.Spec.Containers[0].VolumeMounts {
		mounts[mount.MountPath]++
	}
	for _, container := range workload.Spec.Template.Spec.InitContainers {
		initContainers[container.Name]++
	}
	for name, count := range volumes {
		if count != 1 {
			t.Fatalf("volume %q occurs %d times", name, count)
		}
	}
	for path, count := range mounts {
		if count != 1 {
			t.Fatalf("mount path %q occurs %d times", path, count)
		}
	}
	for name, count := range initContainers {
		if count != 1 {
			t.Fatalf("init container %q occurs %d times", name, count)
		}
	}
}

func TestFirstPullReconcileInjectsMatchingImagePullSecrets(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Finalizers = nil
	task.Annotations = nil
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "registry.example.com/project/image:v1"}}
	credential := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: tenant.DefaultNamespace, Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.example.com"}}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("opaque")}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(credential).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local, Image: "rlark:test"}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var workload appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKey{Namespace: tenant.DefaultNamespace, Name: task.Name}, &workload); err != nil {
		t.Fatal(err)
	}
	want := []corev1.LocalObjectReference{{Name: "registry"}}
	if !reflect.DeepEqual(workload.Spec.Template.Spec.ImagePullSecrets, want) {
		t.Fatalf("imagePullSecrets=%v, want %v", workload.Spec.Template.Spec.ImagePullSecrets, want)
	}
	var claimed rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &claimed); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(claimed.Finalizers, ManagementTaskFinalizer) || claimed.Annotations[ManagementTaskClaimantAnnotation] != "cluster/Kubernetes" {
		t.Fatalf("Task was not claimed: annotations=%v finalizers=%v", claimed.Annotations, claimed.Finalizers)
	}
}

func TestRunningTaskGenerationUpdatesDeployment(t *testing.T) {
	scheme := executionScheme(t)
	one, two := int32(1), int32(2)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	carrierParameters, _ := json.Marshal(taskapi.KubernetesReplicatedCarrierParametersV1{Replicas: &two})
	task.Spec.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: carrierParameters}
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v2"}}
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", Annotations: map[string]string{
			ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace,
			ManagementTaskUIDAnnotation: string(task.UID), ManagementTaskGenerationAnnotation: "2",
		}},
		Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}},
	}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var got appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != two || got.Spec.Template.Spec.Containers[0].Image != "example:v2" {
		t.Fatalf("deployment was not updated: replicas=%v image=%q", got.Spec.Replicas, got.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestClaimedTaskRuntimeSelectorChangeDoesNotCleanupKubernetes(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Spec.Runtime.Kind = rlarkv1alpha1.RuntimeKind("Docker")
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v2"}}
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	if local.creates != 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("selector change touched Runtime: create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
	}
	var workload appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &workload); err != nil {
		t.Fatal(err)
	}
	if got := workload.Spec.Template.Spec.Containers[0].Image; got != "example:v1" {
		t.Fatalf("selector violation partially applied another Spec change: image=%q", got)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(current.Status.Message, "Runtime selector changed") {
		t.Fatalf("status message=%q", current.Status.Message)
	}
}

func TestClaimedTaskRuntimeSelectorRecoveryAppliesFrozenSpec(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Spec.Runtime.Kind = rlarkv1alpha1.RuntimeKind("Docker")
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v2"}}
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example:v1"}}}}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if local.updates != 0 {
		t.Fatalf("selector violation updated workload %d times", local.updates)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), request.NamespacedName, &current); err != nil {
		t.Fatal(err)
	}
	current.Spec.Runtime.Kind = rlarkv1alpha1.RuntimeKindKubernetes
	if err := management.Update(context.Background(), &current); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if local.updates != 1 {
		t.Fatalf("restored selector updates=%d, want 1", local.updates)
	}
	var workload appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &workload); err != nil {
		t.Fatal(err)
	}
	if got := workload.Spec.Template.Spec.Containers[0].Image; got != "example:v2" {
		t.Fatalf("restored selector did not apply desired image: %q", got)
	}
}

func TestClaimedWorkloadSelectorChangePreservesExistingRoot(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Annotations[host.TaskClaimedWorkloadVersionAnnotation] = "v0"
	task.Annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleHead
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "old", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}}
	service := raykubernetes.BuildRayHeadService(tenant.DefaultNamespace, task)
	service.UID = "service-uid"
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &delayedDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(old, service).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err != nil {
		t.Fatal(err)
	}
	if local.deletes != 0 || result.RequeueAfter != 0 {
		t.Fatalf("deletes=%d result=%#v", local.deletes, result)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(old), old); err != nil {
		t.Fatalf("retained workload was removed: %v", err)
	}
}

func TestClaimedWorkloadSelectorChangeDoesNotSwitchClaimOrCreateRoot(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Annotations[host.TaskClaimedWorkloadVersionAnnotation] = "v0"
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "old", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(old).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}

	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil || result.RequeueAfter != 0 || local.deletes != 0 || local.creates != 0 {
		t.Fatalf("first result=%#v error=%v writes=%#v", result, err, local)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Annotations[host.TaskClaimedWorkloadTypeAnnotation] != taskapi.WorkloadTypeRay {
		t.Fatalf("claimed selector=%v", current.Annotations)
	}
	if current.Annotations[host.TaskClaimedWorkloadVersionAnnotation] != "v0" {
		t.Fatalf("claimed selector=%v", current.Annotations)
	}

	var deployment appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(old), &deployment); err != nil {
		t.Fatalf("existing root was removed: %v", err)
	}
}

func TestClaimedWorkloadSelectorChangeHasNoRuntimeSideEffects(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Annotations[host.TaskClaimedWorkloadVersionAnnotation] = "v0"
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "old", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}}
	events := &operationEvents{}
	management := &statusEventClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build(), events: events}
	local := &deleteEventClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(old).Build(), events: events}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.values) != 0 {
		t.Fatalf("unexpected operations=%v", events.values)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("result=%#v", result)
	}
}

func TestClaimedWorkloadSelectorChangeDoesNotAttemptDelete(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Annotations[host.TaskClaimedWorkloadVersionAnnotation] = "v0"
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "old", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &failingDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(old).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("result=%#v", result)
	}
	var existing appsv1.Deployment
	if getErr := local.Get(context.Background(), client.ObjectKeyFromObject(old), &existing); getErr != nil {
		t.Fatalf("existing root was removed: %v", getErr)
	}
}

func TestReconcileFailsWithoutTaskHost(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	controller := NewTaskController(base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).Build()})
	_, err := (&pullReconciler{c: controller}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err == nil || !strings.Contains(err.Error(), "task Host is not configured") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateExecutionDependenciesRejectsPartialAssembly(t *testing.T) {
	scheme := executionScheme(t)
	management := fake.NewClientBuilder().WithScheme(scheme).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*Controller)
		want string
	}{
		{name: "host", edit: func(c *Controller) { c.host = nil }, want: "task Host is not configured"},
		{name: "coordinator", edit: func(c *Controller) { c.execution = nil }, want: "task execution coordinator is not configured"},
		{name: "status writer", edit: func(c *Controller) { c.status = nil }, want: "task Status writer is not configured"},
		{name: "management client", edit: func(c *Controller) { c.ManagementClient = nil }, want: "task management client is not configured"},
		{name: "local client", edit: func(c *Controller) { c.LocalKubeClient = nil }, want: "task local Kubernetes client is not configured"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
			test.edit(controller)
			if err := controller.validateExecutionDependencies(); err == nil || err.Error() != test.want {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestFastDispatchWithMissingCoordinatorReturnsConfigurationError(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	controller.execution = nil
	key := client.ObjectKeyFromObject(task)
	_, err = controller.dispatch(context.Background(), key, trigger.Set{
		Key:        trigger.Key{Namespace: key.Namespace, Name: key.Name},
		Kinds:      map[trigger.Kind]struct{}{trigger.RuntimeState: {}},
		Roots:      []agentruntime.ObjectRef{{ID: "apps/v1, Kind=Deployment", Namespace: tenant.DefaultNamespace, Name: task.Name, UID: "root-uid"}},
		ObjectUID:  string(task.UID),
		Generation: task.Generation,
	})
	if err == nil || err.Error() != "task execution coordinator is not configured" {
		t.Fatalf("error=%v, want coordinator configuration error", err)
	}
}

func TestCoordinatorAdoptsOriginMainRunningWorkloadsWithoutTemplateDrift(t *testing.T) {
	for _, test := range []struct {
		name string
		kind string
		old  func(corev1.PodTemplateSpec) client.Object
	}{
		{name: "deployment", kind: raykubernetes.CarrierDeployment, old: func(template corev1.PodTemplateSpec) client.Object {
			one := int32(1)
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "task",
					Namespace: tenant.DefaultNamespace,
					UID:       "workload-uid",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &one,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}},
					Template: template,
				},
			}
		}},
		{name: "daemonset", kind: raykubernetes.CarrierDaemonSet, old: func(template corev1.PodTemplateSpec) client.Object {
			return &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "task",
					Namespace: tenant.DefaultNamespace,
					UID:       "workload-uid",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}},
					Template: template,
				},
			}
		}},
		{name: "statefulset", kind: raykubernetes.CarrierStatefulSet, old: func(template corev1.PodTemplateSpec) client.Object {
			one := int32(1)
			return &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "task",
					Namespace: tenant.DefaultNamespace,
					UID:       "workload-uid",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas:            &one,
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}},
					Template:            template,
					PodManagementPolicy: appsv1.ParallelPodManagement,
				},
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheme := executionScheme(t)
			task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
			carrierType := taskapi.CarrierTypeKubernetesDeployment
			if test.kind == raykubernetes.CarrierDaemonSet {
				carrierType = taskapi.CarrierTypeKubernetesDaemonSet
			}
			if test.kind == raykubernetes.CarrierStatefulSet {
				carrierType = taskapi.CarrierTypeKubernetesStatefulSet
			}
			task.Spec.Workload.Carrier.Type = carrierType
			task.Annotations[host.TaskClaimedCarrierTypeAnnotation] = carrierType
			if test.kind != raykubernetes.CarrierDaemonSet {
				one := int32(1)
				parameters, _ := json.Marshal(taskapi.KubernetesReplicatedCarrierParametersV1{Replicas: &one})
				task.Spec.Workload.Carrier.Parameters = apiextensionsv1.JSON{Raw: parameters}
			}
			task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}
			// This fixture is the shape produced before Coordinator: name/selector
			// ownership, no workload-level management annotations or UID selector.
			oldTemplate := task.Spec.Workload.Template.DeepCopy()
			workloadObject := test.old(*oldTemplate)
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
			local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(workloadObject).Build()}
			execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
			if err != nil {
				t.Fatal(err)
			}
			reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
			request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
			if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if local.creates != 1 || local.deletes != 0 || local.updates != 1 {
				t.Fatalf("adoption writes create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
			}
			gotTemplate := readWorkloadTemplate(t, local, test.kind)
			if !reflect.DeepEqual(gotTemplate.Spec, oldTemplate.Spec) || !reflect.DeepEqual(gotTemplate.Labels, oldTemplate.Labels) {
				t.Fatalf("running template drifted\nold=%#v\ngot=%#v", oldTemplate, gotTemplate)
			}
			if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if local.updates != 1 || local.creates != 1 || local.deletes != 0 {
				t.Fatalf("second reconcile wrote create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
			}
		})
	}
}

func TestDeletingOriginMainWorkloadKeepsFinalizerUntilObjectDisappears(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	delete(task.Annotations, host.TaskClaimedWorkloadTypeAnnotation)
	delete(task.Annotations, host.TaskClaimedWorkloadVersionAnnotation)
	now := metav1.Now()
	task.DeletionTimestamp = &now
	task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: *task.Spec.Workload.Template}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &delayedDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if local.deletes != 1 || result.RequeueAfter != CleanupRequeueInterval {
		t.Fatalf("deletes=%d result=%#v", local.deletes, result)
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !containsString(got.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("finalizer removed while legacy workload remains: %v", got.Finalizers)
	}
}

func TestDeletingOriginMainWorkloadKeepsFinalizerOnDeleteFailure(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	delete(task.Annotations, host.TaskClaimedWorkloadTypeAnnotation)
	delete(task.Annotations, host.TaskClaimedWorkloadVersionAnnotation)
	now := metav1.Now()
	task.DeletionTimestamp = &now
	task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: *task.Spec.Workload.Template}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &failingDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	_, err = reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err == nil {
		t.Fatal("delete failure was ignored")
	}
	var got rlarkv1alpha1.Task
	if getErr := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); getErr != nil {
		t.Fatal(getErr)
	}
	if !containsString(got.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("finalizer removed after delete failure: %v", got.Finalizers)
	}
}

func TestDeletingWorkloadRecoversWhenDeleteResponseIsLost(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	now := metav1.Now()
	task.DeletionTimestamp = &now
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
	workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &lostDeleteResponseClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
	if _, err := reconciler.Reconcile(context.Background(), request); err == nil {
		t.Fatal("lost delete response was not reported")
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), request.NamespacedName, &current); err != nil {
		t.Fatal(err)
	}
	if !containsString(current.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("lost response released finalizer: %v", current.Finalizers)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	current = rlarkv1alpha1.Task{}
	err = management.Get(context.Background(), request.NamespacedName, &current)
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	if err == nil && containsString(current.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("recovery retained finalizer: %v", current.Finalizers)
	}
	if local.deletes != 1 {
		t.Fatalf("delete calls=%d, want 1", local.deletes)
	}
}

func TestStopWaitsForOriginMainWorkloadDeletion(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*rlarkv1alpha1.Task)
	}{
		{name: "stop", prepare: func(task *rlarkv1alpha1.Task) { task.Spec.Suspended = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheme := executionScheme(t)
			task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
			test.prepare(task)
			task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}
			legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: *task.Spec.Workload.Template}}
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
			local := &delayedDeleteClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
			execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
			if err != nil {
				t.Fatal(err)
			}
			reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
			if err != nil {
				t.Fatal(err)
			}
			if local.deletes != 1 || result.RequeueAfter != CleanupRequeueInterval {
				t.Fatalf("deletes=%d result=%#v", local.deletes, result)
			}
			var got rlarkv1alpha1.Task
			if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
				t.Fatal(err)
			}
			if test.name == "stop" && got.Status.Phase == rlarkv1alpha1.TaskPhaseStopped {
				t.Fatal("stop completed before workload disappeared")
			}
		})
	}
}

func TestKubernetesLifecycleResetsStaleStoppedStatus(t *testing.T) {
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseStopped)
	decision, err := (kubernetesLifecycle{}).BeforeReconcile(context.Background(), host.LifecycleInput{External: task})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != host.LifecycleWait || decision.Reason != "StoppedStatusReset" || decision.Status == nil || decision.Status.Phase != string(rlarkv1alpha1.TaskPhasePending) {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestAgentIgnoresJobRestartAnnotation(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Annotations["rlark.io/restarted-at"] = "job-operation"
	task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: *task.Spec.Workload.Template}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	if local.deletes != 0 {
		t.Fatalf("Job restart annotation caused %d Agent deletes", local.deletes)
	}
}

func TestStopCleanupLeavesExternalPVCAndSharedRayConfig(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	task.Spec.Suspended = true
	task.Annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleWorker
	task.Spec.Workload.Template = &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}, Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-task-data"}}}}}}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: *task.Spec.Workload.Template}}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-task-data",
			Namespace: tenant.DefaultNamespace,
			UID:       "pvc-uid",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}
	shared := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ray-worker-init", Namespace: tenant.DefaultNamespace, UID: "shared-uid"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy, pvc, shared).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	for _, object := range []client.Object{&corev1.PersistentVolumeClaim{}, &corev1.ConfigMap{}} {
		name := pvc.Name
		if _, ok := object.(*corev1.ConfigMap); ok {
			name = shared.Name
		}
		if err := local.Get(context.Background(), client.ObjectKey{Namespace: tenant.DefaultNamespace, Name: name}, object); err != nil {
			t.Fatalf("untouched %T %q: %v", object, name, err)
		}
	}
	if local.deletes != 1 {
		t.Fatalf("deletes=%d, want only workload", local.deletes)
	}
}

func TestUnrealizableClaimedTaskPreservesLegacyWorkloadUntilDeletion(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	// A claimed Task can become unrealizable after an API downgrade, partial
	// migration, or temporarily incompatible Agent upgrade. That is not deletion
	// authorization, so the workload and claim must remain intact.
	task.Spec.Workload = rlarkv1alpha1.TaskWorkloadSpec{}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": task.Name}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": task.Name}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if !containsString(got.Finalizers, ManagementTaskFinalizer) || got.Annotations[ManagementTaskClaimantAnnotation] == "" {
		t.Fatalf("unrealizable Task claim changed: %#v", got.ObjectMeta)
	}
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(legacy), &appsv1.Deployment{}); err != nil {
		t.Fatalf("preserved legacy workload: %v", err)
	}
	if local.deletes != 0 {
		t.Fatalf("deletes=%d, want none before deletion", local.deletes)
	}

	// Deletion remains Spec-independent and cleans the claimed workload by the
	// recorded Runtime and Task UID without managing referenced external storage.
	if err := management.Delete(context.Background(), &got); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	got = rlarkv1alpha1.Task{}
	err = management.Get(context.Background(), client.ObjectKeyFromObject(task), &got)
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	if err == nil && containsString(got.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("finalizer remains after deletion cleanup: %v", got.Finalizers)
	}
	if local.deletes != 1 {
		t.Fatalf("deletes=%d, want legacy workload only during deletion", local.deletes)
	}
}

func TestKubernetesHostLeavesOtherAgentLegacyFinalizerUntouched(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseRunning)
	delete(task.Annotations, ManagementTaskClaimantAnnotation)
	task.Spec.Runtime.Kind = rlarkv1alpha1.RuntimeKind("Docker")
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "legacy-uid"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	reconciler := &pullReconciler{c: newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	if local.creates != 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("other Agent legacy Task touched Runtime: create=%d update=%d delete=%d", local.creates, local.updates, local.deletes)
	}
	var current rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !containsString(current.Finalizers, ManagementTaskFinalizer) || current.Annotations[ManagementTaskClaimantAnnotation] != "" || !reflect.DeepEqual(current.Status, task.Status) {
		t.Fatalf("other Agent legacy Task changed: metadata=%#v status=%#v", current.ObjectMeta, current.Status)
	}
	var currentLegacy appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(legacy), &currentLegacy); err != nil {
		t.Fatalf("legacy workload changed or deleted: %v", err)
	}
}

func TestPushEmitsRuntimeTriggerWithoutRuntimeOrStatusEffects(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	one := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", Generation: 3, Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID)}}, Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}}, Status: appsv1.DeploymentStatus{ObservedGeneration: 3, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &operationCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	if _, err := (&pushDeploymentReconciler{c: controller}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(deployment)}); err != nil {
		t.Fatal(err)
	}
	if local.lists != 0 {
		t.Fatalf("push list calls=%d, want no Runtime observation before pull dispatch", local.lists)
	}
	if local.creates != 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("push wrote local resources: %#v", local)
	}
	key := trigger.Key{Namespace: task.Namespace, Name: task.Name}
	set, ok := controller.triggers.accumulator.Take(key)
	if !ok || !set.Has(trigger.RuntimeState) || len(set.Roots) != 1 || set.Roots[0].UID != string(deployment.UID) {
		t.Fatalf("runtime trigger=%#v ok=%v", set, ok)
	}
	controller.triggers.accumulator.Done(key)
}

func TestRuntimeTriggerUsesVerifiedFastObservationAfterFullReconcile(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "origin-main:v1"}}
	one := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "workload-uid", ResourceVersion: "1", Generation: 3, Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID), ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", task.Generation)}}, Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "origin-main:v1"}}}}}, Status: appsv1.DeploymentStatus{ObservedGeneration: 3, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &operationCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	pull := &pullReconciler{c: controller}
	key := client.ObjectKeyFromObject(task)
	if _, err := pull.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	var root appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(deployment), &root); err != nil {
		t.Fatal(err)
	}
	if _, err := kubernetesruntime.ReadExecutionRevision(&root); err != nil {
		t.Fatalf("full reconcile did not establish root revision: %v annotations=%v", err, root.Annotations)
	}
	local.lists, local.creates, local.updates, local.deletes = 0, 0, 0, 0
	if _, err := (&pushDeploymentReconciler{c: controller}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(deployment)}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-controller.triggers.events:
	case <-time.After(time.Second):
		t.Fatal("runtime trigger was not emitted")
	}
	if _, err := pull.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if local.creates != 0 || local.updates != 0 || local.deletes != 0 {
		t.Fatalf("fast observation performed Runtime writes: %#v", local)
	}
	if local.lists < 1 || local.lists > 2 {
		t.Fatalf("fast observation list calls=%d, want targeted Pod/Event lists only", local.lists)
	}
}

func TestPullReconcileRequeuesTriggerWakeupPressureWithoutError(t *testing.T) {
	scheme := executionScheme(t)
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "origin-main:v1"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := &operationCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	execution, err := NewKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	controller.triggers.events = make(chan event.GenericEvent, 1)
	key := trigger.Key{Namespace: task.Namespace, Name: task.Name}
	if err := controller.triggers.emit(trigger.Trigger{Key: key, Kind: trigger.Periodic, ObjectUID: string(task.UID), Generation: task.Generation}); err != nil {
		t.Fatal(err)
	}
	<-controller.triggers.events
	controller.host.Bundle.Observer = emitTriggerObserver{emit: func() {
		if err := controller.triggers.emit(trigger.Trigger{Key: key, Kind: trigger.Periodic, ObjectUID: string(task.UID), Generation: task.Generation}); err != nil {
			t.Errorf("emit during reconcile: %v", err)
		}
		controller.triggers.events <- eventForTest("occupied")
	}}

	result, err := (&pullReconciler{c: controller}).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)})
	if err != nil {
		t.Fatalf("successful reconcile reported trigger pressure as an error: %v", err)
	}
	if result.RequeueAfter != time.Nanosecond {
		t.Fatalf("result=%#v, want immediate requeue", result)
	}
	set, ok := controller.triggers.accumulator.Take(key)
	if !ok || !set.Has(trigger.Periodic) || !set.FullReconcileRequired {
		t.Fatalf("retained trigger=%#v ok=%v", set, ok)
	}
}

type emitTriggerObserver struct {
	emit func()
}

func (o emitTriggerObserver) Start(ctx context.Context, _ observability.Reconcile) (context.Context, observability.Scope) {
	return ctx, &emitTriggerScope{emit: o.emit}
}

type emitTriggerScope struct {
	emit func()
	once sync.Once
}

func (s *emitTriggerScope) Event(observability.Event) { s.once.Do(s.emit) }
func (*emitTriggerScope) End(observability.Result)    {}

func executionScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
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
	return scheme
}

func newHostedTestController(t *testing.T, baseController base.Controller, execution *coordinator.Coordinator) *Controller {
	t.Helper()
	controller := NewTaskController(baseController).WithExecutionCoordinator(execution)
	taskHost, err := NewKubernetesHost(controller, execution)
	if err != nil {
		t.Fatal(err)
	}
	return controller.WithHost(taskHost)
}

func readWorkloadTemplate(t *testing.T, kube client.Client, kind string) corev1.PodTemplateSpec {
	t.Helper()
	key := client.ObjectKey{Namespace: tenant.DefaultNamespace, Name: "task"}
	switch kind {
	case raykubernetes.CarrierDeployment:
		var object appsv1.Deployment
		if err := kube.Get(context.Background(), key, &object); err != nil {
			t.Fatal(err)
		}
		return object.Spec.Template
	case raykubernetes.CarrierDaemonSet:
		var object appsv1.DaemonSet
		if err := kube.Get(context.Background(), key, &object); err != nil {
			t.Fatal(err)
		}
		return object.Spec.Template
	default:
		var object appsv1.StatefulSet
		if err := kube.Get(context.Background(), key, &object); err != nil {
			t.Fatal(err)
		}
		return object.Spec.Template
	}
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

type delayedDeleteClient struct {
	client.Client
	deletes int
}

type failingDeleteClient struct{ client.Client }

type lostDeleteResponseClient struct {
	client.Client
	deletes int
}

func (c *failingDeleteClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return errors.New("delete failed")
}

func (c *lostDeleteResponseClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	c.deletes++
	if err := c.Client.Delete(ctx, object, options...); err != nil {
		return err
	}
	return errors.New("delete response lost")
}

func (c *delayedDeleteClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	c.deletes++
	return nil
}

type executionCountingClient struct {
	client.Client
	creates int
	updates int
	deletes int
}

type operationCountingClient struct {
	client.Client
	lists, creates, updates, deletes int
}

type operationEvents struct{ values []string }

type statusEventClient struct {
	client.Client
	events *operationEvents
}

type statusEventWriter struct {
	client.SubResourceWriter
	events *operationEvents
}

func (c *statusEventClient) Status() client.SubResourceWriter {
	return &statusEventWriter{SubResourceWriter: c.Client.Status(), events: c.events}
}

func (w *statusEventWriter) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.SubResourcePatchOption) error {
	w.events.values = append(w.events.values, "status")
	return w.SubResourceWriter.Patch(ctx, object, patch, options...)
}

type deleteEventClient struct {
	client.Client
	events *operationEvents
}

func (c *deleteEventClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	c.events.values = append(c.events.values, "delete")
	return c.Client.Delete(ctx, object, options...)
}

func (c *operationCountingClient) List(ctx context.Context, list client.ObjectList, options ...client.ListOption) error {
	c.lists++
	return c.Client.List(ctx, list, options...)
}
func (c *operationCountingClient) Create(ctx context.Context, object client.Object, options ...client.CreateOption) error {
	c.creates++
	return c.Client.Create(ctx, object, options...)
}
func (c *operationCountingClient) Update(ctx context.Context, object client.Object, options ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, object, options...)
}
func (c *operationCountingClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	c.deletes++
	return c.Client.Delete(ctx, object, options...)
}

func (c *executionCountingClient) Create(ctx context.Context, object client.Object, options ...client.CreateOption) error {
	c.creates++
	return c.Client.Create(ctx, object, options...)
}

func (c *executionCountingClient) Update(ctx context.Context, object client.Object, options ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, object, options...)
}

func (c *executionCountingClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	c.deletes++
	return c.Client.Delete(ctx, object, options...)
}
