package binding

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// DesiredObjectValidator validates one Runtime declaration without effects.
type DesiredObjectValidator interface {
	ValidateDesired(agentruntime.DesiredObject) error
}

// DesiredPlanValidator adapts Runtime declaration validation to final Plan
// validation while keeping Runtime packages independent of binding contracts.
type DesiredPlanValidator struct {
	RuntimeKind agentruntime.Kind
	Objects     DesiredObjectValidator
}

// Validate checks every desired object before the Coordinator observes or
// mutates Runtime state.
func (v DesiredPlanValidator) Validate(_ context.Context, request BindRequest, plan *executionplan.Plan) error {
	if v.Objects == nil {
		return fault.New(fault.FailedPrecondition, "desired object validator is required")
	}
	if v.RuntimeKind == "" || request.Runtime.Kind != v.RuntimeKind {
		return fault.New(fault.FailedPrecondition, "plan validator does not support the selected Runtime kind")
	}
	if plan == nil {
		return fault.New(fault.Internal, "plan validator received no Plan")
	}
	if err := agentruntime.ValidateObjects(plan.Objects); err != nil {
		return fault.Wrap(fault.InvalidArgument, "invalid desired object graph", err)
	}
	for _, object := range plan.Objects {
		if err := v.Objects.ValidateDesired(object); err != nil {
			return err
		}
	}
	return nil
}

var _ PlanValidator = DesiredPlanValidator{}
