package workload

import (
	"testing"

	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestExecutablePlanContainsOnlyBoundRuntimeState(t *testing.T) {
	plan := executionplan.ExecutablePlan{
		Objects: []agentruntime.DesiredObject{{LogicalName: "workload"}},
		Discovery: executionplan.InstanceDiscovery{
			Roots: []string{"workload"},
		},
	}
	if len(plan.Objects) != 1 || plan.Discovery.Roots[0] != "workload" {
		t.Fatalf("unexpected executable plan: %#v", plan)
	}
}
