package grpc

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestPlanRoundTripPreservesExecutionDeclarations(t *testing.T) {
	locator := agentruntime.ObjectLocator{Type: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Path: ".spec.template"}
	plan := executionplan.Plan{
		Environment: executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{Name: "network", Type: "rlinf.io/network", Version: "v1", Parameters: []byte(`{"domain":"domain"}`), Consumers: []executionplan.ConsumerSelector{{Targets: []string{"pod"}}}}}},
		Targets:     []executionplan.MutationTarget{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "worker", Locator: locator}},
		Units:       []executionplan.WorkloadUnit{{Name: "pod", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "worker", Locator: locator}},
		Processes:   []executionprocess.Declaration{{Name: "primary", Target: "pod", Locator: locator, ContainerName: "main", Role: executionprocess.RolePrimary, Executable: "/rlark-tools/rlark-tools", Args: []string{"ray-launcher"}, RestartPolicy: executionprocess.RestartNever, Required: true}},
	}
	wire, err := toWirePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fromWirePlan(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Environment, plan.Environment) || !reflect.DeepEqual(got.Targets, plan.Targets) || !reflect.DeepEqual(got.Units, plan.Units) || !reflect.DeepEqual(got.Processes, plan.Processes) {
		t.Fatalf("round trip plan=%#v", got)
	}
}

func TestWirePlanAcceptsEmptyEnvironment(t *testing.T) {
	wire, err := toWirePlan(executionplan.Plan{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fromWirePlan(wire); err != nil {
		t.Fatal(err)
	}
}

func TestTaskRequirementsTransportParametersAsJSON(t *testing.T) {
	task := coordination.TaskSnapshot{
		Identity: agentruntime.TaskIdentity{
			Logical: agentruntime.LogicalTaskID{
				Namespace: "management",
				Name:      "task",
			},
		},
		Environment: executionplan.Requirements{
			Environments: []executionplan.EnvironmentRequirement{{
				Name:       "network",
				Type:       "rlinf.io/network",
				Version:    "v1",
				Parameters: []byte(`{"domain":"domain-a"}`),
			}},
		},
	}
	wire, err := toWireTask(task)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire.EnvironmentRequirements.Value, []byte(`"parameters":{"domain":"domain-a"}`)) {
		t.Fatalf("wire requirements did not preserve JSON parameters: %s", wire.EnvironmentRequirements.Value)
	}
	roundTrip, err := fromWireTask(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(roundTrip.Environment.Environments[0].Parameters); got != `{"domain":"domain-a"}` {
		t.Fatalf("parameters = %s", got)
	}
}

func TestExecutablePlanRoundTripPreservesBindingRevisionAndRuntimeUnits(t *testing.T) {
	locator := agentruntime.ObjectLocator{Type: agentruntime.ObjectKind{Version: "v1", Kind: "PodTemplate"}, Path: ".spec.template"}
	plan := executionplan.ExecutablePlan{
		Digests: executionplan.Digests{CompiledHash: "sha256:compiled", ExecutableHash: "sha256:executable", BindingRevision: "sha256:binding"},
		Objects: []agentruntime.DesiredObject{{
			LogicalName: "workload", Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Manifest:  agentruntime.TypedManifest{Type: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Object: []byte(`{"kind":"Deployment"}`)},
			Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityLogicalTask, OwnerKey: "management/task"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace,
			Units: []agentruntime.RuntimeUnit{{Name: "pod", Kind: agentruntime.UnitPodTemplate, Role: "worker", Locator: locator}},
		}},
	}
	wire, err := toWireExecutable(plan)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fromWireExecutable(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digests != plan.Digests || !reflect.DeepEqual(got.Objects[0].Units, plan.Objects[0].Units) {
		t.Fatalf("executable round trip=%#v", got)
	}
}
