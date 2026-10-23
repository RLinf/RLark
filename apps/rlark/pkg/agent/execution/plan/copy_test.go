package plan

import (
	"testing"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestPlanDeepCopyHasNoMutableAliases(t *testing.T) {
	options := agentruntime.TypedOptions{Value: []byte(`{"pods":true}`)}
	plan := Plan{
		Objects: []agentruntime.DesiredObject{{
			LogicalName: "workload", Manifest: agentruntime.TypedManifest{Object: []byte(`{"metadata":{"name":"task"}}`)},
			DependsOn: []string{"config"}, OwnedFields: []string{"data"}, Units: []agentruntime.RuntimeUnit{{Name: "existing"}},
		}},
		Targets: []MutationTarget{{Name: "pod", ObjectName: "workload"}},
		Units:   []WorkloadUnit{{Name: "pod", ObjectName: "workload"}},
		Environment: Requirements{Environments: []EnvironmentRequirement{{
			Name:       "config",
			Type:       "example.io/config",
			Version:    "v1",
			Parameters: []byte(`{"key":"value"}`),
			Consumers:  []ConsumerSelector{{Targets: []string{"pod"}}},
		}}},
		Discovery: InstanceDiscovery{Roots: []string{"workload"}, Options: &options},
	}

	copy := plan.DeepCopy()
	copy.Objects[0].Manifest.Object[0] = '['
	copy.Objects[0].DependsOn[0] = "changed"
	copy.Objects[0].OwnedFields[0] = "changed"
	copy.Targets[0].Name = "changed"
	copy.Units[0].Name = "changed"
	copy.Objects[0].Units[0].Name = "changed"
	copy.Environment.Environments[0].Parameters[0] = '['
	copy.Environment.Environments[0].Consumers[0].Targets[0] = "changed"
	copy.Discovery.Roots[0] = "changed"
	copy.Discovery.Options.Value[0] = '['

	if string(plan.Objects[0].Manifest.Object) != `{"metadata":{"name":"task"}}` || plan.Objects[0].DependsOn[0] != "config" || plan.Objects[0].OwnedFields[0] != "data" ||
		plan.Targets[0].Name != "pod" || plan.Units[0].Name != "pod" || plan.Objects[0].Units[0].Name != "existing" || string(plan.Environment.Environments[0].Parameters) != `{"key":"value"}` || plan.Environment.Environments[0].Consumers[0].Targets[0] != "pod" ||
		plan.Discovery.Roots[0] != "workload" || string(plan.Discovery.Options.Value) != `{"pods":true}` {
		t.Fatal("Plan.DeepCopy retained a mutable alias")
	}
}
