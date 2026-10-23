package environment

import (
	"context"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestPassthroughResolverAcceptsOnlyEmptyRequirements(t *testing.T) {
	task := agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "cluster", Name: "task"}}
	resolved, err := (PassthroughResolver{}).Resolve(context.Background(), ResolveRequest{Task: task})
	if err != nil || resolved.Task != task || len(resolved.Bindings) != 0 {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	_, err = (PassthroughResolver{}).Resolve(context.Background(), ResolveRequest{
		Task: task,
		Requirements: executionplan.Requirements{
			Environments: []executionplan.EnvironmentRequirement{{
				Name:    "network",
				Type:    "rlinf.io/network",
				Version: "v1",
			}},
		},
	})
	if !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("error=%v", err)
	}
}
