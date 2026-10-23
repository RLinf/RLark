package kubernetes_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	kubernetesworkload "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

func TestRayKubernetesAdaptersCompileLegacyWorkloadWithoutResourceDrift(t *testing.T) {
	for _, test := range []struct {
		name string
		kind string
		new  func() workload.Adapter
	}{
		{name: "deployment", kind: kubernetesworkload.CarrierDeployment, new: func() workload.Adapter {
			return kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace)
		}},
		{name: "daemonset", kind: kubernetesworkload.CarrierDaemonSet, new: func() workload.Adapter {
			return kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace)
		}},
		{name: "statefulset", kind: kubernetesworkload.CarrierStatefulSet, new: func() workload.Adapter {
			return kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			task := compileTask(test.kind)
			snapshot, err := compatibility.DecodeTask(task)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := test.new().Compile(context.Background(), workload.CompileRequest{Task: snapshot, Runtime: agentruntime.Info{ID: "kubernetes", Kind: compatibility.RuntimeKindKubernetes}})
			if err != nil {
				t.Fatal(err)
			}
			if plan.CompiledHash == "" || len(plan.Discovery.Roots) != 1 || plan.Discovery.Roots[0] != "workload" {
				t.Fatalf("plan metadata=%#v", plan)
			}
			if len(plan.Units) != 1 || plan.Units[0].Kind != agentruntime.UnitPodTemplate || plan.Units[0].ObjectName != "workload" {
				t.Fatalf("units=%#v", plan.Units)
			}
			primary := findObject(t, plan.Objects, "workload")
			var compiled struct {
				Spec struct {
					Template corev1.PodTemplateSpec `json:"template"`
				} `json:"spec"`
			}
			if err := json.Unmarshal(primary.Manifest.Object, &compiled); err != nil {
				t.Fatal(err)
			}
			if _, found := compiled.Spec.Template.Annotations[kubernetesobjects.ManagementTaskDomainAnnotation]; found {
				t.Fatalf("Workload Adapter still writes network domain annotation: %#v", compiled.Spec.Template.Annotations)
			}
			for _, container := range compiled.Spec.Template.Spec.InitContainers {
				if container.Name == "rlark-tools-init" || container.Name == "tensorboard" {
					t.Fatalf("Workload Adapter still injects Environment container %q", container.Name)
				}
			}
			if !plan.Environment.Empty() {
				t.Fatalf("Ray Adapter declared Runtime facilities as Environment requirements: %#v", plan.Environment)
			}
			spec := task.Spec.Workload.DeepCopy()
			spec.Template = snapshot.Configuration.Template.DeepCopy()
			kubernetesworkload.ApplyTemplateMutations(spec.Template, task)
			kubernetesworkload.ApplyWorkloadIdentity(spec.Template, task)
			spec.Template.Spec.Containers[0].Env = compiled.Spec.Template.Spec.Containers[0].Env
			var expected any
			switch test.kind {
			case kubernetesworkload.CarrierDeployment:
				expected = kubernetesworkload.BuildDeployment(task, "runtime", spec, carrierReplicas(task))
			case kubernetesworkload.CarrierDaemonSet:
				expected = kubernetesworkload.BuildDaemonSet(task, "runtime", spec)
			case kubernetesworkload.CarrierStatefulSet:
				expected = kubernetesworkload.BuildStatefulSet(task, "runtime", spec, carrierReplicas(task))
			}
			expectedJSON, _ := json.Marshal(expected)
			if !jsonEqual(primary.Manifest.Object, expectedJSON) {
				t.Fatalf("compiled workload differs from legacy output\ncompiled=%s\nlegacy=%s", primary.Manifest.Object, expectedJSON)
			}
			if len(primary.DependsOn) != 0 {
				t.Fatalf("unexpected dependencies=%v", primary.DependsOn)
			}
		})
	}
}

func TestRayKubernetesAdapterCompileIsStable(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierDeployment)
	snapshot, _ := compatibility.DecodeTask(task)
	adapter := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace)
	first, err := adapter.Compile(context.Background(), workload.CompileRequest{Task: snapshot, Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Compile(context.Background(), workload.CompileRequest{Task: snapshot, Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes}})
	if err != nil {
		t.Fatal(err)
	}
	if first.CompiledHash != second.CompiledHash || !reflect.DeepEqual(first.Objects, second.Objects) {
		t.Fatal("same Task produced a different plan and could trigger an unnecessary rollout")
	}
}

func TestRayKubernetesAdapterPreservesGenericEphemeralVolume(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierDeployment)
	task.Spec.Workload.Template.Spec.Volumes = []corev1.Volume{{
		Name: "scratch",
		VolumeSource: corev1.VolumeSource{Ephemeral: &corev1.EphemeralVolumeSource{
			VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			},
		}},
	}}
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := kubernetesworkload.NewAdapter(
		workload.TypeID(taskapi.WorkloadTypeRay),
		namespace,
	).Compile(context.Background(), workload.CompileRequest{
		Task:    snapshot,
		Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes},
	})
	if err != nil {
		t.Fatal(err)
	}
	primary := findObject(t, plan.Objects, "workload")
	var deployment appsv1.Deployment
	if err := json.Unmarshal(primary.Manifest.Object, &deployment); err != nil {
		t.Fatal(err)
	}
	volumes := deployment.Spec.Template.Spec.Volumes
	if len(volumes) != 1 || volumes[0].Ephemeral == nil || volumes[0].Ephemeral.VolumeClaimTemplate == nil {
		t.Fatalf("generic ephemeral volume was not preserved: %#v", volumes)
	}
	for _, object := range plan.Objects {
		if object.Kind.Kind == "PersistentVolumeClaim" {
			t.Fatalf("generic ephemeral volume generated a standalone PVC: %#v", object)
		}
	}
}

func TestRayKubernetesAdapterRejectsInvalidCarrierParameters(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		parameters string
	}{
		{
			name:       "deployment unknown field",
			kind:       kubernetesworkload.CarrierDeployment,
			parameters: `{"replicas":1,"unknown":true}`,
		},
		{
			name:       "daemonset replicas",
			kind:       kubernetesworkload.CarrierDaemonSet,
			parameters: `{"replicas":1}`,
		},
		{
			name:       "statefulset unknown field",
			kind:       kubernetesworkload.CarrierStatefulSet,
			parameters: `{"replicas":1,"unknown":true}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := compileTask(test.kind)
			task.Spec.Workload.Carrier.Parameters = apiextensionsv1.JSON{
				Raw: []byte(test.parameters),
			}
			snapshot, err := compatibility.DecodeTask(task)
			if err != nil {
				t.Fatal(err)
			}
			adapter := kubernetesworkload.NewAdapter(
				workload.TypeID(taskapi.WorkloadTypeRay),
				namespace,
			)
			_, err = adapter.Compile(context.Background(), workload.CompileRequest{
				Task: snapshot,
				Runtime: agentruntime.Info{
					Kind: compatibility.RuntimeKindKubernetes,
				},
			})
			if err == nil {
				t.Fatal("invalid carrier parameters were accepted")
			}
		})
	}
}

func TestRayKubernetesAdapterMarksRayResourcesOwned(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierDeployment)
	setRayRole(t, task, taskapi.RayRoleHead)
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Compile(context.Background(), workload.CompileRequest{Task: snapshot, Runtime: agentruntime.Info{ID: "kubernetes", Kind: compatibility.RuntimeKindKubernetes}})
	if err != nil {
		t.Fatal(err)
	}
	service := findObject(t, plan.Objects, "ray-service")
	if service.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup || service.Ownership.Scope != agentruntime.IdentityTaskIncarnation || service.Ownership.OwnerKey != "task-uid" || service.Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		t.Fatalf("Ray Service ownership/lifecycle=%#v/%q", service.Ownership, service.Lifecycle)
	}
	if service.UpdateMode != agentruntime.UpdateReplace {
		t.Fatalf("Ray Service update mode=%q, want Replace", service.UpdateMode)
	}
	primary := findObject(t, plan.Objects, "workload")
	if primary.Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		t.Fatalf("primary workload ownership=%#v, want explicit adoption policy", primary.Ownership)
	}
	if primary.ReconcileProfile.Kind != "WorkloadProjection" || service.ReconcileProfile.Kind != "ServiceProjection" {
		t.Fatalf("reconcile profiles: workload=%#v service=%#v", primary.ReconcileProfile, service.ReconcileProfile)
	}
	for _, object := range plan.Objects {
		if object.Kind.Kind == "PersistentVolumeClaim" {
			t.Fatalf("Ray Adapter generated a compatibility PVC: %#v", object)
		}
	}
}

func TestRayKubernetesAdapterConsumesHeadTaskName(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierStatefulSet)
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{
		Role:          taskapi.RayRoleWorker,
		HeadTaskName:  "training-head",
		TotalNodes:    3,
		NodeRankStart: 1,
		Prepare:       &taskapi.InlineScript{Inline: "prepare"},
		Run:           &taskapi.InlineScript{Inline: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Parameters.Raw = parameters

	plan, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Compile(context.Background(), workload.CompileRequest{
		Task:    mustSnapshot(t, task),
		Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes},
	})
	if err != nil {
		t.Fatal(err)
	}
	object := findObject(t, plan.Objects, "workload")
	var statefulSet appsv1.StatefulSet
	if err := json.Unmarshal(object.Manifest.Object, &statefulSet); err != nil {
		t.Fatal(err)
	}
	environment := map[string]corev1.EnvVar{}
	for _, variable := range statefulSet.Spec.Template.Spec.Containers[0].Env {
		environment[variable.Name] = variable
	}
	for name, want := range map[string]string{
		"RLARK_HEAD_ADDRESS":    "training-head-ray-head",
		"RLARK_RAY_PORT":        "6379",
		"RLARK_PREPARE_SCRIPT":  "prepare",
		"RLARK_RUN_SCRIPT":      "run",
		"RLARK_NODE_RANK_START": "1",
	} {
		if got := environment[name].Value; got != want {
			t.Errorf("%s=%q, want %q", name, got, want)
		}
	}
	if got := environment["POD_NAME"].ValueFrom; got == nil || got.FieldRef == nil || got.FieldRef.FieldPath != "metadata.name" {
		t.Errorf("POD_NAME=%#v, want metadata.name fieldRef", environment["POD_NAME"])
	}
}

func TestRayKubernetesAdapterUsesNetworkDomainForHeadDiscovery(t *testing.T) {
	task := compileTask(kubernetesworkload.CarrierStatefulSet)
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{Role: taskapi.RayRoleWorker, HeadTaskName: "training-head"})
	if err != nil {
		t.Fatal(err)
	}
	network, err := json.Marshal(taskapi.NetworkEnvironmentParametersV1{Domain: "training"})
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Parameters.Raw = parameters
	task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: taskapi.EnvironmentTypeNetwork, Version: taskapi.EnvironmentVersionNetworkV1, Parameters: apiextensionsv1.JSON{Raw: network}}}

	plan, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Compile(context.Background(), workload.CompileRequest{Task: mustSnapshot(t, task), Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes}})
	if err != nil {
		t.Fatal(err)
	}
	object := findObject(t, plan.Objects, "workload")
	var statefulSet appsv1.StatefulSet
	if err := json.Unmarshal(object.Manifest.Object, &statefulSet); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{}
	for _, variable := range statefulSet.Spec.Template.Spec.Containers[0].Env {
		environment[variable.Name] = variable.Value
	}
	if got := environment["RLARK_HEAD_ADDRESS"]; got != "training-head-0.rlark-domain" {
		t.Fatalf("RLARK_HEAD_ADDRESS=%q", got)
	}
	if len(plan.Processes) != 1 || !reflect.DeepEqual(plan.Processes[0].Args, []string{"ray-launcher", "--role=worker", "--wait-network"}) {
		t.Fatalf("Ray process args=%#v", plan.Processes)
	}
}

func TestRayKubernetesAdapterInventoryDeclaresCompleteIdentityResourceSet(t *testing.T) {
	adapter := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace)
	task := compileTask(kubernetesworkload.CarrierDeployment)
	inventory, err := adapter.Inventory(context.Background(), workload.InventoryRequest{
		Task: agentruntime.TaskIdentity{
			Logical: agentruntime.LogicalTaskID{
				Namespace: task.Namespace,
				Name:      task.Name,
			},
			ObjectUID: string(task.UID),
		},
		Runtime: agentruntime.Info{Kind: compatibility.RuntimeKindKubernetes},
		Carrier: coordination.CarrierSelector{
			Type:    task.Spec.Workload.Carrier.Type,
			Version: task.Spec.Workload.Carrier.Version,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 4 {
		t.Fatalf("candidates=%#v, want supervisor ConfigMap, SSH Secret, workload, and head Service", inventory.Candidates)
	}
	configCandidate, secretCandidate, workloadCandidate, serviceCandidate := inventory.Candidates[0], inventory.Candidates[1], inventory.Candidates[2], inventory.Candidates[3]
	if configCandidate.Kind != (agentruntime.ObjectKind{Version: "v1", Kind: "ConfigMap"}) || configCandidate.Namespace != "runtime" || configCandidate.Name == "" || configCandidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup || configCandidate.Ownership.Scope != agentruntime.IdentityTaskIncarnation || configCandidate.Ownership.OwnerKey != "task-uid" {
		t.Fatalf("supervisor ConfigMap candidate=%#v", configCandidate)
	}
	if secretCandidate.Kind != (agentruntime.ObjectKind{Version: "v1", Kind: "Secret"}) || secretCandidate.Namespace != "runtime" || secretCandidate.Name == "" || secretCandidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup || secretCandidate.Ownership.Scope != agentruntime.IdentityTaskIncarnation || secretCandidate.Ownership.OwnerKey != "task-uid" {
		t.Fatalf("SSH Secret candidate=%#v", secretCandidate)
	}
	if workloadCandidate.Kind != (agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}) || workloadCandidate.Namespace != "runtime" || workloadCandidate.Name != "task" || len(workloadCandidate.Labels) != 0 || workloadCandidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup || workloadCandidate.Ownership.Scope != agentruntime.IdentityTaskIncarnation || workloadCandidate.Ownership.OwnerKey != "task-uid" || workloadCandidate.Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		t.Fatalf("workload candidate=%#v", workloadCandidate)
	}
	if serviceCandidate.Kind != (agentruntime.ObjectKind{Version: "v1", Kind: "Service"}) || serviceCandidate.Namespace != "runtime" || serviceCandidate.Name != "task-ray-head" || len(serviceCandidate.Labels) != 0 || serviceCandidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup || serviceCandidate.Ownership.Scope != agentruntime.IdentityTaskIncarnation || serviceCandidate.Ownership.OwnerKey != "task-uid" || serviceCandidate.Ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		t.Fatalf("Service candidate=%#v", serviceCandidate)
	}
}

func TestRayHeadServiceIdentityIgnoresTaskGeneration(t *testing.T) {
	first := compileTask(kubernetesworkload.CarrierDeployment)
	setRayRole(t, first, taskapi.RayRoleHead)
	second := first.DeepCopy()
	second.Generation++
	firstService := kubernetesworkload.BuildRayHeadService("runtime", first)
	secondService := kubernetesworkload.BuildRayHeadService("runtime", second)
	if !reflect.DeepEqual(firstService, secondService) {
		t.Fatalf("Task generation changed Ray Service:\nfirst=%#v\nsecond=%#v", firstService, secondService)
	}
	if _, found := firstService.Annotations[kubernetesobjects.ManagementTaskGenerationAnnotation]; found {
		t.Fatalf("Ray Service carries generation annotation: %#v", firstService.Annotations)
	}
}

func TestRayKubernetesAdaptersObserveLegacyPhases(t *testing.T) {
	one := int32(1)
	deploymentObject := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}}
	manifest, _ := json.Marshal(deploymentObject)
	request := workload.ObserveRequest{
		Task: mustSnapshot(t, compileTask(kubernetesworkload.CarrierDeployment)),
		Objects: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{{Manifest: agentruntime.TypedManifest{
			Type:   agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Object: manifest,
		}}}},
	}
	observation, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Observe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Phase != workload.Phase(rlarkv1alpha1.TaskPhaseRunning) {
		t.Fatalf("phase=%q", observation.Phase)
	}
	request.Instances = agentruntime.InstanceSnapshot{Instances: []agentruntime.Instance{{Name: "pod", Reason: "CrashLoopBackOff", Message: "back-off"}}}
	observation, err = kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Observe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Phase != workload.Phase(rlarkv1alpha1.TaskPhaseFailed) || observation.Message != "pod pod container main: back-off" {
		t.Fatalf("observation=%#v", observation)
	}
}

func TestRayKubernetesAdapterObserveUsesProjectedContainerNameAndFailedScheduling(t *testing.T) {
	one := int32(1)
	object := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2}}
	manifest, _ := json.Marshal(object)
	base := workload.ObserveRequest{
		Task: mustSnapshot(t, compileTask(kubernetesworkload.CarrierDeployment)),
		Objects: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{{Manifest: agentruntime.TypedManifest{
			Type:   agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Object: manifest,
		}}}},
	}
	base.Instances = agentruntime.InstanceSnapshot{Instances: []agentruntime.Instance{{Name: "pod", Conditions: []agentruntime.InstanceCondition{{Scope: "instance", Reason: "FailedScheduling", Message: "no nodes"}}}}}
	observation, err := kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Observe(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Phase != workload.Phase(rlarkv1alpha1.TaskPhasePending) || observation.Message != "FailedScheduling" {
		t.Fatalf("scheduling observation=%#v", observation)
	}
	base.Instances.Instances[0].Conditions = append(base.Instances.Instances[0].Conditions, agentruntime.InstanceCondition{Scope: "container", Subject: "trainer", Reason: "OOMKilled"})
	observation, err = kubernetesworkload.NewAdapter(workload.TypeID(taskapi.WorkloadTypeRay), namespace).Observe(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Phase != workload.Phase(rlarkv1alpha1.TaskPhaseFailed) || observation.Message != "pod pod container trainer: OOMKilled" {
		t.Fatalf("container observation=%#v", observation)
	}
}

func TestObserveStatusMatrixMatchesLegacyReducers(t *testing.T) {
	one, zero := int32(1), int32(0)
	tests := []struct {
		name    string
		kind    string
		object  any
		phase   rlarkv1alpha1.TaskPhase
		message string
	}{
		{name: "deployment stopped", kind: kubernetesworkload.CarrierDeployment, object: &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &zero}}, phase: rlarkv1alpha1.TaskPhaseStopped},
		{name: "deployment unobserved", kind: kubernetesworkload.CarrierDeployment, object: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1}}, phase: rlarkv1alpha1.TaskPhasePending},
		{name: "deployment deadline", kind: kubernetesworkload.CarrierDeployment, object: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: "deadline"}}}}, phase: rlarkv1alpha1.TaskPhaseFailed, message: "deadline"},
		{name: "deployment replica failure", kind: kubernetesworkload.CarrierDeployment, object: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue, Message: "replica"}}}}, phase: rlarkv1alpha1.TaskPhaseFailed, message: "replica"},
		{name: "daemonset partial", kind: kubernetesworkload.CarrierDaemonSet, object: &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 1}}, phase: rlarkv1alpha1.TaskPhasePending},
		{name: "daemonset ready", kind: kubernetesworkload.CarrierDaemonSet, object: &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberReady: 2, NumberAvailable: 2}}, phase: rlarkv1alpha1.TaskPhaseRunning},
		{name: "statefulset stopped", kind: kubernetesworkload.CarrierStatefulSet, object: &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &zero}}, phase: rlarkv1alpha1.TaskPhaseStopped},
		{name: "statefulset revision pending", kind: kubernetesworkload.CarrierStatefulSet, object: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.StatefulSetSpec{Replicas: &one}, Status: appsv1.StatefulSetStatus{ObservedGeneration: 2, UpdatedReplicas: 1, ReadyReplicas: 1, CurrentReplicas: 1, CurrentRevision: "old", UpdateRevision: "new"}}, phase: rlarkv1alpha1.TaskPhasePending},
		{name: "statefulset ready", kind: kubernetesworkload.CarrierStatefulSet, object: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.StatefulSetSpec{Replicas: &one}, Status: appsv1.StatefulSetStatus{ObservedGeneration: 2, UpdatedReplicas: 1, ReadyReplicas: 1, CurrentReplicas: 1, CurrentRevision: "new", UpdateRevision: "new"}}, phase: rlarkv1alpha1.TaskPhaseRunning},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := json.Marshal(test.object)
			if err != nil {
				t.Fatal(err)
			}
			phase, message, err := kubernetesworkload.ObserveObject(test.kind, manifest, agentruntime.InstanceSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if phase != test.phase || message != test.message {
				t.Fatalf("got (%q,%q), want (%q,%q)", phase, message, test.phase, test.message)
			}
		})
	}
}

func compileTask(kind string) *rlarkv1alpha1.Task {
	parameters, _ := json.Marshal(taskapi.RayExecutionParametersV1{Role: taskapi.RayRoleWorker})
	replicas := int32(2)
	carrierParameters, _ := json.Marshal(taskapi.KubernetesReplicatedCarrierParametersV1{Replicas: &replicas})
	carrierType := taskapi.CarrierTypeKubernetesDeployment
	if kind == kubernetesworkload.CarrierDaemonSet {
		carrierType = taskapi.CarrierTypeKubernetesDaemonSet
		carrierParameters = []byte(`{}`)
	}
	if kind == kubernetesworkload.CarrierStatefulSet {
		carrierType = taskapi.CarrierTypeKubernetesStatefulSet
	}
	return &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "management", UID: types.UID("task-uid"), Generation: 3}, Spec: rlarkv1alpha1.TaskSpec{
		Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"zone": "edge"}},
		Runtime:   rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes},
		Workload: rlarkv1alpha1.TaskWorkloadSpec{
			Kind:       taskapi.WorkloadTypeRay,
			Version:    taskapi.WorkloadVersionRayV1,
			Parameters: apiextensionsv1.JSON{Raw: parameters},
			Carrier: rlarkv1alpha1.TaskWorkloadCarrier{
				Type:       carrierType,
				Version:    taskapi.CarrierVersionKubernetesV1,
				Parameters: apiextensionsv1.JSON{Raw: carrierParameters},
			},
			Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Image: "example:v1"}},
				Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "existing-data"},
				}}},
			}},
		},
	}}
}

func carrierReplicas(task *rlarkv1alpha1.Task) *int32 {
	parameters, _ := taskapi.DecodeParametersStrict[taskapi.KubernetesReplicatedCarrierParametersV1](task.Spec.Workload.Carrier.Parameters)
	return parameters.Replicas
}

func mustSnapshot(t *testing.T, task *rlarkv1alpha1.Task) coordination.TaskSnapshot {
	t.Helper()
	snapshot, err := compatibility.DecodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func setRayRole(t *testing.T, task *rlarkv1alpha1.Task, role taskapi.RayRole) {
	t.Helper()
	parameters, err := json.Marshal(taskapi.RayExecutionParametersV1{Role: role})
	if err != nil {
		t.Fatal(err)
	}
	task.Spec.Workload.Parameters.Raw = parameters
}

func namespace(agentruntime.TaskIdentity) string { return "runtime" }
func findObject(t *testing.T, objects []agentruntime.DesiredObject, name string) agentruntime.DesiredObject {
	t.Helper()
	for _, object := range objects {
		if object.LogicalName == name {
			return object
		}
	}
	t.Fatalf("object %q not found", name)
	return agentruntime.DesiredObject{}
}
func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
