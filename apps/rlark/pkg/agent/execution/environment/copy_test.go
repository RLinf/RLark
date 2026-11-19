package environment

import (
	"testing"

	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
)

func TestRequirementsDeepCopyHasNoMutableAliases(t *testing.T) {
	requirements := executionplan.Requirements{Environments: []executionplan.EnvironmentRequirement{{
		Name:       "network",
		Type:       "rlinf.io/network",
		Version:    "v1",
		Parameters: []byte(`{"domain":"domain"}`),
		Consumers: []executionplan.ConsumerSelector{{
			Targets:     []string{"pod"},
			ObjectNames: []string{"workload"},
			Roles:       []string{"worker"},
			UnitNames:   []string{"main"},
		}},
	}}}

	copy := requirements.DeepCopy()
	copy.Environments[0].Parameters[0] = '['
	copy.Environments[0].Consumers[0].Targets[0] = "changed"
	copy.Environments[0].Consumers[0].ObjectNames[0] = "changed"
	copy.Environments[0].Consumers[0].Roles[0] = "changed"
	copy.Environments[0].Consumers[0].UnitNames[0] = "changed"

	consumer := requirements.Environments[0].Consumers[0]
	if string(requirements.Environments[0].Parameters) != `{"domain":"domain"}` || consumer.Targets[0] != "pod" || consumer.ObjectNames[0] != "workload" || consumer.Roles[0] != "worker" || consumer.UnitNames[0] != "main" {
		t.Fatal("executionplan.Requirements.DeepCopy retained a mutable alias")
	}
}
