package sharedmemory

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestHandlerMountsSharedMemory(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlan(t)

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"512Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := bind(t, plan, resolved, task)
	assertSharedMemoryMount(t, result, "512Mi")
}

func TestHandlerMountsSharedMemoryToAllContainers(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "test"},
						{Name: "sidecar", Image: "sidecar"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	plan := executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"256Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := bind(t, plan, resolved, task)
	var bound appsv1.Deployment
	if err := json.Unmarshal(result.Objects[0].Manifest.Object, &bound); err != nil {
		t.Fatal(err)
	}

	if len(bound.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("volumes=%#v", bound.Spec.Template.Spec.Volumes)
	}
	volume := bound.Spec.Template.Spec.Volumes[0]
	if volume.Name != VolumeName || volume.EmptyDir == nil {
		t.Fatalf("volume=%#v", volume)
	}
	if volume.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Fatalf("medium=%v, want Memory", volume.EmptyDir.Medium)
	}
	if volume.EmptyDir.SizeLimit == nil || volume.EmptyDir.SizeLimit.String() != "256Mi" {
		t.Fatalf("sizeLimit=%v", volume.EmptyDir.SizeLimit)
	}

	if len(bound.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("containers=%#v", bound.Spec.Template.Spec.Containers)
	}
	for _, container := range bound.Spec.Template.Spec.Containers {
		found := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == VolumeName && mount.MountPath == MountPath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("container %q missing shared memory mount: %#v", container.Name, container.VolumeMounts)
		}
	}
}

func TestResolverRejectsInvalidSharedMemorySize(t *testing.T) {
	tests := []struct {
		name       string
		parameters string
	}{
		{name: "missing size", parameters: `{}`},
		{name: "zero size", parameters: `{"size":"0"}`},
		{name: "negative size", parameters: `{"size":"-100Mi"}`},
		{name: "invalid quantity", parameters: `{"size":"not-a-quantity"}`},
		{name: "unknown field", parameters: `{"size":"100Mi","unknown":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := executionplan.MutationTarget{
				Name:    "pod",
				Kind:    executionplan.TargetPodTemplate,
				Locator: agentruntime.ObjectLocator{Path: "spec.template"},
			}
			_, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
				Runtime:      agentruntime.Info{Kind: "Kubernetes"},
				Requirements: sharedMemoryRequirements(target.Name, test.parameters),
				Targets:      []executionplan.MutationTarget{target},
			})
			if !fault.Is(err, fault.InvalidArgument) {
				t.Fatalf("error=%v, want invalid argument", err)
			}
		})
	}
}

func TestResolverRejectsMismatchedType(t *testing.T) {
	target := executionplan.MutationTarget{
		Name:    "pod",
		Kind:    executionplan.TargetPodTemplate,
		Locator: agentruntime.ObjectLocator{Path: "spec.template"},
	}
	_, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Runtime: agentruntime.Info{Kind: "Kubernetes"},
		Requirements: executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
			Name:       RequirementName,
			Type:       "rlinf.io/not-shared-memory",
			Version:    taskapi.EnvironmentVersionSharedMemoryV1,
			Parameters: []byte(`{"size":"100Mi"}`),
		}}},
		Targets: []executionplan.MutationTarget{target},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func TestApplyDetectsVolumeNameConflict(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlanWithConflictingVolume(t, VolumeName)

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"128Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = bindExpectingConflict(t, plan, resolved, task)
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestApplyDetectsMountPathConflict(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlanWithConflictingMountPath(t, MountPath, "other-volume")

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"128Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = bindExpectingConflict(t, plan, resolved, task)
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestApplyIsIdempotentForEquivalentExistingMount(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlanWithEquivalentSharedMemory(t, "64Mi")

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"64Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := bind(t, plan, resolved, task)
	assertSharedMemoryMount(t, result, "64Mi")
}

func TestApplyDetectsReadOnlyMountConflict(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlanWithReadOnlySharedMemoryMount(t)

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"64Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = bindExpectingConflict(t, plan, resolved, task)
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestApplyDetectsSubPathMountConflict(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlanWithSubPathSharedMemoryMount(t)

	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sharedMemoryRequirements(target.Name, `{"size":"64Mi"}`),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = bindExpectingConflict(t, plan, resolved, task)
	if !fault.Is(err, fault.Conflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestResolverRejectsMultipleDeclarations(t *testing.T) {
	target := executionplan.MutationTarget{
		Name:    "pod",
		Kind:    executionplan.TargetPodTemplate,
		Locator: agentruntime.ObjectLocator{Path: "spec.template"},
	}
	requirements := executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{
		{
			Name:       RequirementName,
			Type:       taskapi.EnvironmentTypeSharedMemory,
			Version:    taskapi.EnvironmentVersionSharedMemoryV1,
			Parameters: []byte(`{"size":"100Mi"}`),
		},
		{
			Name:       RequirementName,
			Type:       taskapi.EnvironmentTypeSharedMemory,
			Version:    taskapi.EnvironmentVersionSharedMemoryV1,
			Parameters: []byte(`{"size":"200Mi"}`),
		},
	}}
	_, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: requirements,
		Targets:      []executionplan.MutationTarget{target},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

func bind(t *testing.T, plan executionplan.Plan, resolved environment.ResolvedEnvironment, task agentruntime.TaskIdentity) executionplan.ExecutablePlan {
	t.Helper()
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{{Handler: Handler{RuntimeKind: "Kubernetes"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (binding.BindingPlanBinder{
		Handlers:  registry,
		Workloads: binding.WorkloadAssemblerFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }),
		Validator: binding.PlanValidatorFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }),
		Lowerer:   passLowerer{},
	}).Bind(context.Background(), binding.BindRequest{
		Runtime:     agentruntime.Info{Kind: "Kubernetes"},
		Plan:        plan,
		Environment: resolved,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func bindExpectingConflict(t *testing.T, plan executionplan.Plan, resolved environment.ResolvedEnvironment, task agentruntime.TaskIdentity) (executionplan.ExecutablePlan, error) {
	t.Helper()
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{{Handler: Handler{RuntimeKind: "Kubernetes"}}})
	if err != nil {
		t.Fatal(err)
	}
	return (binding.BindingPlanBinder{
		Handlers:  registry,
		Workloads: binding.WorkloadAssemblerFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }),
		Validator: binding.PlanValidatorFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }),
		Lowerer:   passLowerer{},
	}).Bind(context.Background(), binding.BindRequest{
		Runtime:     agentruntime.Info{Kind: "Kubernetes"},
		Plan:        plan,
		Environment: resolved,
	})
}

func assertSharedMemoryMount(t *testing.T, result executionplan.ExecutablePlan, expectedSize string) {
	t.Helper()
	if len(result.Objects) != 1 {
		t.Fatalf("objects=%#v", result.Objects)
	}
	var deployment appsv1.Deployment
	if err := json.Unmarshal(result.Objects[0].Manifest.Object, &deployment); err != nil {
		t.Fatal(err)
	}
	if len(deployment.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("volumes=%#v", deployment.Spec.Template.Spec.Volumes)
	}
	volume := deployment.Spec.Template.Spec.Volumes[0]
	if volume.Name != VolumeName || volume.EmptyDir == nil {
		t.Fatalf("volume=%#v", volume)
	}
	if volume.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Fatalf("medium=%v, want Memory", volume.EmptyDir.Medium)
	}
	if volume.EmptyDir.SizeLimit == nil || volume.EmptyDir.SizeLimit.String() != expectedSize {
		t.Fatalf("sizeLimit=%v, want %s", volume.EmptyDir.SizeLimit, expectedSize)
	}
	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("containers=%#v", deployment.Spec.Template.Spec.Containers)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].Name != VolumeName || container.VolumeMounts[0].MountPath != MountPath {
		t.Fatalf("mounts=%#v", container.VolumeMounts)
	}
}

func sharedMemoryRequirements(target string, parameters string) executionplan.Requirements {
	return executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:       RequirementName,
		Type:       taskapi.EnvironmentTypeSharedMemory,
		Version:    taskapi.EnvironmentVersionSharedMemoryV1,
		Parameters: json.RawMessage(parameters),
		Consumers:  []executionplan.ConsumerSelector{{Targets: []string{target}}},
	}}}
}

type passLowerer struct{}

func (passLowerer) Lower(_ context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	return request.Object, nil
}

func testPlan(t *testing.T) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "test"}}},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func testPlanWithConflictingVolume(t *testing.T, volumeName string) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "test"}},
					Volumes: []corev1.Volume{
						{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func testPlanWithConflictingMountPath(t *testing.T, mountPath string, volumeName string) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "test",
						VolumeMounts: []corev1.VolumeMount{
							{Name: volumeName, MountPath: mountPath},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func testPlanWithEquivalentSharedMemory(t *testing.T, size string) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	sizeLimit := resourceQuantity(t, size)
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "test",
						VolumeMounts: []corev1.VolumeMount{
							{Name: VolumeName, MountPath: MountPath},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: VolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium:    corev1.StorageMediumMemory,
									SizeLimit: &sizeLimit,
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func testPlanWithReadOnlySharedMemoryMount(t *testing.T) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	sizeLimit := resourceQuantity(t, "64Mi")
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "test",
						VolumeMounts: []corev1.VolumeMount{
							{Name: VolumeName, MountPath: MountPath, ReadOnly: true},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: VolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium:    corev1.StorageMediumMemory,
									SizeLimit: &sizeLimit,
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func testPlanWithSubPathSharedMemoryMount(t *testing.T) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	sizeLimit := resourceQuantity(t, "64Mi")
	deployment := appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "test",
						VolumeMounts: []corev1.VolumeMount{
							{Name: VolumeName, MountPath: MountPath, SubPath: "subdir"},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: VolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium:    corev1.StorageMediumMemory,
									SizeLimit: &sizeLimit,
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{
		LogicalName: "workload", Kind: kind,
		Manifest:  agentruntime.TypedManifest{Type: kind, Object: data},
		Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"},
		Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
	}
	target := executionplan.MutationTarget{
		Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload",
		Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"},
	}
	return executionplan.Plan{
		CompiledHash: "compiled",
		Objects:      []agentruntime.DesiredObject{object},
		Targets:      []executionplan.MutationTarget{target},
	}, target
}

func resourceQuantity(t *testing.T, s string) (q resource.Quantity) {
	t.Helper()
	q, err := resource.ParseQuantity(s)
	if err != nil {
		t.Fatalf("parse quantity %q: %v", s, err)
	}
	return q
}
