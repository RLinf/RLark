package coordination

import (
	"errors"
	"testing"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestSelectRuntimeRequiresUniqueMatch(t *testing.T) {
	candidates := []RuntimeCandidate{
		{Info: agentruntime.Info{ID: "cluster-a", Kind: "Kubernetes"}},
		{Info: agentruntime.Info{ID: "cluster-b", Kind: "Kubernetes"}},
	}

	selected, err := SelectRuntime(RuntimeSelector{ID: "cluster-a", Kind: "Kubernetes"}, candidates)
	if err != nil || selected.Info.ID != "cluster-a" {
		t.Fatalf("SelectRuntime() = %#v, %v", selected, err)
	}

	_, err = SelectRuntime(RuntimeSelector{Kind: "Kubernetes"}, candidates)
	if !errors.Is(err, ErrAmbiguousRuntime) {
		t.Fatalf("SelectRuntime() error = %v, want ambiguous runtime", err)
	}

	_, err = SelectRuntime(RuntimeSelector{ID: "missing"}, candidates)
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("SelectRuntime() error = %v, want runtime not found", err)
	}
}
