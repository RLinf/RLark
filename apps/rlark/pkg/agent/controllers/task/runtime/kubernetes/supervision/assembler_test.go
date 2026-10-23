package supervision

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestAssemblerLowersPrimaryAndAuxiliaryIntoConfigMapAndPod(t *testing.T) {
	plan, request := testPlan()
	if err := testAssembler().Assemble(context.Background(), request, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Objects) != 2 || plan.Objects[1].LogicalName != LogicalName || plan.Objects[1].Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
		t.Fatalf("objects=%#v", plan.Objects)
	}
	var workload appsv1.Deployment
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &workload); err != nil {
		t.Fatal(err)
	}
	main := workload.Spec.Template.Spec.Containers[0]
	if len(main.Command) != 1 || main.Command[0] != "/rlark-tools/rlark-tools" || len(main.Args) != 2 || main.Args[0] != "supervisor" {
		t.Fatalf("main=%#v", main)
	}
	if workload.Spec.Template.Annotations[RevisionAnnotation] == "" {
		t.Fatal("supervisor revision annotation is missing")
	}
	if len(plan.Objects[0].DependsOn) != 1 || plan.Objects[0].DependsOn[0] != LogicalName {
		t.Fatalf("dependsOn=%v", plan.Objects[0].DependsOn)
	}
	var configMap corev1.ConfigMap
	if err := json.Unmarshal(plan.Objects[1].Manifest.Object, &configMap); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Processes []struct {
			Name string `json:"name"`
		} `json:"processes"`
	}
	if err := json.Unmarshal([]byte(configMap.Data[DataKey]), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Processes) != 2 || config.Processes[0].Name != "ray-launcher" || config.Processes[1].Name != "sshd" {
		t.Fatalf("config=%#v", config)
	}
}

func TestAssemblerAddsDefaultPrimaryAndTools(t *testing.T) {
	plan, request := testPlan()
	plan.Processes = nil
	var workload appsv1.Deployment
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &workload); err != nil {
		t.Fatal(err)
	}
	workload.Spec.Template.Spec.Containers[0].VolumeMounts = nil
	plan.Objects[0].Manifest.Object, _ = json.Marshal(workload)
	if err := testAssembler().Assemble(context.Background(), request, &plan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &workload); err != nil {
		t.Fatal(err)
	}
	if len(workload.Spec.Template.Spec.InitContainers) != 1 || workload.Spec.Template.Spec.InitContainers[0].Name != "rlark-tools-init" {
		t.Fatalf("tools were not injected: %#v", workload.Spec.Template.Spec)
	}
	var configMap corev1.ConfigMap
	if err := json.Unmarshal(plan.Objects[1].Manifest.Object, &configMap); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Processes []struct {
			Name string   `json:"name"`
			Argv []string `json:"argv"`
		} `json:"processes"`
	}
	if err := json.Unmarshal([]byte(configMap.Data[DataKey]), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Processes) != 1 || config.Processes[0].Name != defaultPrimaryName || !equalStrings(config.Processes[0].Argv, []string{toolsExecutable, "idle"}) {
		t.Fatalf("default config=%#v", config)
	}
}

func TestAssemblerRejectsMultipleWorkloadUnits(t *testing.T) {
	plan, request := testPlan()
	plan.Units = append(plan.Units, plan.Units[0])
	plan.Units[1].Name = "other"
	if err := testAssembler().Assemble(context.Background(), request, &plan); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("error=%v, want failed precondition", err)
	}
}

func TestAssemblerRevisionChangesOnlyWithProcessConfig(t *testing.T) {
	first, request := testPlan()
	if err := testAssembler().Assemble(context.Background(), request, &first); err != nil {
		t.Fatal(err)
	}
	second, request := testPlan()
	second.Processes[1].Args = append(second.Processes[1].Args, "--port=2222")
	if err := testAssembler().Assemble(context.Background(), request, &second); err != nil {
		t.Fatal(err)
	}
	var firstWorkload, secondWorkload appsv1.Deployment
	if err := json.Unmarshal(first.Objects[0].Manifest.Object, &firstWorkload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Objects[0].Manifest.Object, &secondWorkload); err != nil {
		t.Fatal(err)
	}
	if firstWorkload.Spec.Template.Annotations[RevisionAnnotation] == secondWorkload.Spec.Template.Annotations[RevisionAnnotation] {
		t.Fatal("process config change did not change rollout revision")
	}
}

func testPlan() (executionplan.Plan, binding.BindRequest) {
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	target := executionplan.MutationTarget{Name: "main", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}
	workload := appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, ObjectMeta: metav1.ObjectMeta{Name: "task"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}}}}
	data, _ := json.Marshal(workload)
	processes := []executionprocess.Declaration{{Name: "ray-launcher", Target: "main", Locator: target.Locator, ContainerName: "main", Role: executionprocess.RolePrimary, Executable: "/rlark-tools/rlark-tools", Args: []string{"ray-launcher", "--role=head"}, RestartPolicy: executionprocess.RestartNever, Required: true}, {Name: "sshd", Target: "main", Locator: target.Locator, ContainerName: "main", Role: executionprocess.RoleAuxiliary, Executable: "/rlark-tools/rlark-tools", Args: []string{"sshd"}, RestartPolicy: executionprocess.RestartOnFailure, Required: false}}
	unit := executionplan.WorkloadUnit{Name: "main", ObjectName: target.ObjectName, Kind: target.Kind, Role: target.Role, Locator: target.Locator}
	plan := executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "workload", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: data}}}, Targets: []executionplan.MutationTarget{target}, Units: []executionplan.WorkloadUnit{unit}, Processes: processes}
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "uid", Generation: 2}
	request := binding.BindRequest{Environment: environment.ResolvedEnvironment{Task: task}}
	return plan, request
}

func testAssembler() Assembler {
	return Assembler{ToolsImage: "rlark:test"}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
