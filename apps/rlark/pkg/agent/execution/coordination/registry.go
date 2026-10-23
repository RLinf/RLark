package coordination

import (
	"errors"
	"fmt"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

var (
	// ErrRuntimeNotFound indicates that no Provider matches a RuntimeSelector.
	ErrRuntimeNotFound = errors.New("runtime not found")
	// ErrAmbiguousRuntime indicates that a RuntimeSelector matches multiple Providers.
	ErrAmbiguousRuntime = errors.New("ambiguous runtime selector")
)

// RuntimeCandidate is one Provider identity considered during Runtime selection.
type RuntimeCandidate struct {
	Info agentruntime.Info
}

// SelectRuntime returns the unique candidate matching all non-empty selector fields.
func SelectRuntime(selector RuntimeSelector, candidates []RuntimeCandidate) (RuntimeCandidate, error) {
	matches := make([]RuntimeCandidate, 0, 1)
	for _, candidate := range candidates {
		if selector.ID != "" && candidate.Info.ID != selector.ID {
			continue
		}
		if selector.Kind != "" && candidate.Info.Kind != selector.Kind {
			continue
		}
		matches = append(matches, candidate)
	}
	switch len(matches) {
	case 0:
		return RuntimeCandidate{}, ErrRuntimeNotFound
	case 1:
		return matches[0], nil
	default:
		return RuntimeCandidate{}, fmt.Errorf("%w: matched %d runtimes", ErrAmbiguousRuntime, len(matches))
	}
}
