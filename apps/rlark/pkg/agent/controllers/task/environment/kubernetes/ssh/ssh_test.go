package ssh

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestHandlerAddsSecretMountAndDependency(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 3}
	plan, target := testPlan(t)
	resolver := Resolver{RuntimeKind: "Kubernetes"}
	resolved, err := resolver.Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sshRequirements(target.Name),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{{Handler: Handler{RuntimeKind: "Kubernetes"}}})
	if err != nil {
		t.Fatal(err)
	}
	processChecked := false
	result, err := (binding.BindingPlanBinder{Handlers: registry, Materials: staticMaterial{value: []byte("ssh-ed25519 key")}, Workloads: binding.WorkloadAssemblerFunc(func(_ context.Context, _ binding.BindRequest, plan *executionplan.Plan) error {
		if len(plan.Processes) != 1 || plan.Processes[0].Name != "sshd" || plan.Processes[0].Required {
			t.Fatalf("SSH process must be an optional auxiliary: %#v", plan.Processes)
		}
		processChecked = true
		return nil
	}), Validator: binding.PlanValidatorFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }), Lowerer: passLowerer{}}).Bind(context.Background(), binding.BindRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Plan: plan, Environment: resolved})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Objects) != 2 {
		t.Fatalf("objects=%#v", result.Objects)
	}
	secretObject, workloadObject := result.Objects[0], result.Objects[1]
	if secretObject.LogicalName != SecretLogicalName {
		secretObject, workloadObject = result.Objects[1], result.Objects[0]
	}
	if secretObject.LogicalName != SecretLogicalName || workloadObject.LogicalName != "workload" {
		t.Fatalf("objects=%#v", result.Objects)
	}
	var secret corev1.Secret
	if err := json.Unmarshal(secretObject.Manifest.Object, &secret); err != nil {
		t.Fatal(err)
	}
	if string(secret.Data[SecretDataKey]) != "ssh-ed25519 key" || secret.Name != SecretName(task) {
		t.Fatalf("secret=%#v", secret)
	}
	if len(workloadObject.DependsOn) != 1 || workloadObject.DependsOn[0] != SecretLogicalName {
		t.Fatalf("dependencies=%v", workloadObject.DependsOn)
	}
	var deployment appsv1.Deployment
	if err := json.Unmarshal(workloadObject.Manifest.Object, &deployment); err != nil {
		t.Fatal(err)
	}
	if len(deployment.Spec.Template.Spec.Volumes) != 1 || deployment.Spec.Template.Spec.Volumes[0].Secret == nil || deployment.Spec.Template.Spec.Volumes[0].Secret.SecretName != secret.Name {
		t.Fatalf("volumes=%#v", deployment.Spec.Template.Spec.Volumes)
	}
	if len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) != 1 || deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath != MountPath || !deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].ReadOnly {
		t.Fatalf("mounts=%#v", deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
	}
	if !processChecked {
		t.Fatal("process assembler was not called")
	}
}

func TestHandlerCreatesEmptySecretToDisableAuthentication(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 1}
	plan, target := testPlan(t)
	resolved, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Task:         task,
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: sshRequirements(target.Name),
		Targets:      []executionplan.MutationTarget{target},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := environment.NewRegistry([]environment.HandlerRegistration{{Handler: Handler{RuntimeKind: "Kubernetes"}}})
	result, err := (binding.BindingPlanBinder{Handlers: registry, Materials: staticMaterial{}, Workloads: binding.WorkloadAssemblerFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }), Validator: binding.PlanValidatorFunc(func(context.Context, binding.BindRequest, *executionplan.Plan) error { return nil }), Lowerer: passLowerer{}}).Bind(context.Background(), binding.BindRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}, Plan: plan, Environment: resolved})
	if err != nil {
		t.Fatal(err)
	}
	var secret corev1.Secret
	secretObject := result.Objects[0]
	if secretObject.LogicalName != SecretLogicalName {
		secretObject = result.Objects[1]
	}
	if err := json.Unmarshal(secretObject.Manifest.Object, &secret); err != nil {
		t.Fatal(err)
	}
	if value, found := secret.Data[SecretDataKey]; !found || len(value) != 0 {
		t.Fatalf("secret data=%#v", secret.Data)
	}
}

func sshRequirements(target string) executionplan.Requirements {
	return executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:      RequirementName,
		Type:      taskapi.EnvironmentTypeSSH,
		Version:   taskapi.EnvironmentVersionSSHV1,
		Consumers: []executionplan.ConsumerSelector{{Targets: []string{target}}},
	}}}
}

func TestMaterialProviderExposesOnlyMatchingTaskAuthorizedKeys(t *testing.T) {
	t.Parallel()

	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 3}
	provider := NewMaterialProvider(task, []string{"ssh-ed25519 first", "ssh-rsa second"})
	lease, err := provider.Acquire(context.Background(), environment.MaterialRequest{
		Task: task, Kind: environment.RequirementSecret, RequirementName: RequirementName,
		SourceRevision: sourceRevision(task), Name: MaterialAuthorizedKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if got := string(lease.Bytes()); got != "ssh-ed25519 first\nssh-rsa second\n" {
		t.Fatalf("authorized keys material = %q", got)
	}

	other := task
	other.ObjectUID = "other-uid"
	if _, err := provider.Acquire(context.Background(), environment.MaterialRequest{
		Task: other, Kind: environment.RequirementSecret, RequirementName: RequirementName,
		SourceRevision: sourceRevision(other), Name: MaterialAuthorizedKey,
	}); err == nil {
		t.Fatal("Acquire() allowed a different Task identity")
	}
}

func TestAuthorizedKeysValidatesSSHOwnedDeclaration(t *testing.T) {
	tests := []struct {
		name        string
		declaration coordination.LocalEnvironmentDeclaration
	}{
		{name: "unsupported version", declaration: coordination.LocalEnvironmentDeclaration{Type: taskapi.EnvironmentTypeSSH, Version: "v2", Parameters: apiextensionsv1.JSON{Raw: []byte(`{}`)}}},
		{name: "invalid parameters", declaration: coordination.LocalEnvironmentDeclaration{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: []byte(`{"authorizedKeys":{},"unknown":true}`)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AuthorizedKeys([]coordination.LocalEnvironmentDeclaration{test.declaration}); err == nil {
				t.Fatal("invalid SSH local declaration was accepted")
			}
		})
	}
}

func TestResolverRejectsUnknownSSHParameters(t *testing.T) {
	target := executionplan.MutationTarget{
		Name: "pod",
		Kind: executionplan.TargetPodTemplate,
		Locator: agentruntime.ObjectLocator{
			Path: "spec.template",
		},
	}
	requirements := sshRequirements(target.Name)
	requirements.Environments[0].Parameters = []byte(`{"port":22,"unknown":true}`)
	_, err := (Resolver{RuntimeKind: "Kubernetes"}).Resolve(context.Background(), environment.ResolveRequest{
		Runtime:      agentruntime.Info{Kind: "Kubernetes"},
		Requirements: requirements,
		Targets:      []executionplan.MutationTarget{target},
	})
	if !fault.Is(err, fault.InvalidArgument) {
		t.Fatalf("error=%v, want invalid argument", err)
	}
}

type staticMaterial struct{ value []byte }

func (p staticMaterial) Acquire(context.Context, environment.MaterialRequest) (environment.MaterialLease, error) {
	return environment.NewByteMaterial(p.value), nil
}

type passLowerer struct{}

func (passLowerer) Lower(_ context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	return request.Object, nil
}

func testPlan(t *testing.T) (executionplan.Plan, executionplan.MutationTarget) {
	t.Helper()
	deployment := appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "test"}}}}}}
	data, _ := json.Marshal(&deployment)
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	object := agentruntime.DesiredObject{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: data}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace}
	target := executionplan.MutationTarget{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "workload", Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}
	return executionplan.Plan{CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{object}, Targets: []executionplan.MutationTarget{target}}, target
}
