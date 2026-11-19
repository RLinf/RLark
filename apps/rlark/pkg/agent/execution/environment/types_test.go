package environment

import (
	"testing"

	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
)

func TestConsumerSelectorContainsOnlyStaticTargets(t *testing.T) {
	selector := executionplan.ConsumerSelector{
		Targets:     []string{"workload-pod-template"},
		ObjectNames: []string{"workload"},
		Roles:       []string{"worker"},
		UnitNames:   []string{"main"},
	}
	if selector.Targets[0] != "workload-pod-template" || selector.ObjectNames[0] != "workload" || selector.Roles[0] != "worker" || selector.UnitNames[0] != "main" {
		t.Fatalf("unexpected selector: %#v", selector)
	}
}
