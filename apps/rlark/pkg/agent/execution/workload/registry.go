package workload

import (
	"errors"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
)

var (
	// ErrAdapterNotFound indicates that no Adapter matches a WorkloadSelector.
	ErrAdapterNotFound = errors.New("workload adapter not found")
	// ErrAmbiguousAdapter indicates duplicate registrations for a WorkloadSelector.
	ErrAmbiguousAdapter = errors.New("ambiguous workload adapter")
)

// Candidate binds a versioned Workload Type to a selectable value.
type Candidate[T any] struct {
	Type    TypeID
	Version string
	Value   T
}

// Select returns the unique candidate matching the selector's Type and Version.
func Select[T any](selector coordination.WorkloadSelector, candidates []Candidate[T]) (T, error) {
	matches := make([]Candidate[T], 0, 1)
	for _, candidate := range candidates {
		if string(candidate.Type) != selector.Type || candidate.Version != selector.Version {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 1 {
		return matches[0].Value, nil
	}
	var zero T
	if len(matches) == 0 {
		return zero, ErrAdapterNotFound
	}
	return zero, fmt.Errorf("%w: matched %d adapters", ErrAmbiguousAdapter, len(matches))
}
