package environment

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
)

// PassthroughResolver accepts only an empty environment contract. It is a
// deliberate migration and test bridge while legacy Kubernetes mutations
// remain in builtin workload compilation; production Environment requirements
// must not be added without replacing this resolver in the assembled Bundle.
type PassthroughResolver struct{}

// Resolve accepts empty requirements and returns an environment with no bindings.
func (PassthroughResolver) Resolve(_ context.Context, request ResolveRequest) (ResolvedEnvironment, error) {
	if !request.Requirements.Empty() {
		return ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "passthrough environment resolver cannot resolve non-empty requirements")
	}
	return ResolvedEnvironment{Task: request.Task}, nil
}
