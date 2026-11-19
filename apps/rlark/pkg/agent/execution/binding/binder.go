package binding

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// PlanValidator performs Runtime-specific final validation after every selected
// Environment handler has mutated the Binder-owned working Plan.
type PlanValidator interface {
	Validate(context.Context, BindRequest, *executionplan.Plan) error
}

// PlanValidatorFunc adapts a function to PlanValidator.
type PlanValidatorFunc func(context.Context, BindRequest, *executionplan.Plan) error

// Validate calls the wrapped validation function.
func (f PlanValidatorFunc) Validate(ctx context.Context, request BindRequest, plan *executionplan.Plan) error {
	return f(ctx, request, plan)
}

// WorkloadAssembler applies the Runtime's mandatory execution-unit facilities
// after Environment handlers have contributed their fragments and before unit
// lowering. It receives every declared workload unit even when no Adapter or
// Handler declared a process. It must be pure and perform no external I/O.
type WorkloadAssembler interface {
	Assemble(context.Context, BindRequest, *executionplan.Plan) error
}

// WorkloadAssemblerFunc adapts a function to WorkloadAssembler.
type WorkloadAssemblerFunc func(context.Context, BindRequest, *executionplan.Plan) error

func (f WorkloadAssemblerFunc) Assemble(ctx context.Context, request BindRequest, plan *executionplan.Plan) error {
	return f(ctx, request, plan)
}

// BindRequest supplies a compiled Plan and resolved Environment to a Binder.
type BindRequest struct {
	Runtime     agentruntime.Info
	Plan        executionplan.Plan
	Environment environment.ResolvedEnvironment
}

// PlanBinder converts a portable Plan into a Runtime-ready executable Plan. It
// must mutate only a deep copy, perform no Runtime I/O, and return no partially
// applicable result when any binding fails. It must leave ExecutableHash and
// BindingRevision empty; the Coordinator owns both after canonical ordering.
type PlanBinder interface {
	Bind(context.Context, BindRequest) (executionplan.ExecutablePlan, error)
}

// BindingPlanBinder validates targets, executes trusted handlers against a deep
// copy, validates the final Runtime-specific Plan, and returns no partial result
// on failure. It performs no management or Runtime I/O.
type BindingPlanBinder struct {
	Handlers  *environment.Registry
	Validator PlanValidator
	Materials environment.MaterialProvider
	Lowerer   agentruntime.UnitLowerer
	Workloads WorkloadAssembler
}

// Bind converts a resolved environment and portable Plan into Runtime-ready
// state. The Coordinator owns final dependency ordering and executable hashing.
func (b BindingPlanBinder) Bind(ctx context.Context, request BindRequest) (executionplan.ExecutablePlan, error) {
	if b.Handlers == nil {
		return executionplan.ExecutablePlan{}, fault.New(fault.FailedPrecondition, "environment handler registry is required")
	}
	if b.Validator == nil {
		return executionplan.ExecutablePlan{}, fault.New(fault.FailedPrecondition, "environment plan validator is required")
	}
	targets, err := environment.ValidateTargets(request.Plan.Targets, request.Plan.Objects)
	if err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	if err := validateResolvedBindings(request.Environment.Bindings, targets); err != nil {
		return executionplan.ExecutablePlan{}, err
	}

	working := request.Plan.DeepCopy()
	working.Objects, err = executionplan.ProjectUnits(working.Units, working.Objects)
	if err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	jobs, err := b.Handlers.Jobs(request.Runtime, request.Environment.Bindings, targets, &working)
	if err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	for i := range jobs {
		jobs[i].Request.Task = request.Environment.Task
	}
	if err := environment.Run(ctx, request.Environment.Task, b.Materials, jobs); err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	if b.Workloads == nil {
		return executionplan.ExecutablePlan{}, fault.New(fault.FailedPrecondition, "workload assembler is required")
	}
	if err := b.Workloads.Assemble(ctx, request, &working); err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	if b.Lowerer == nil {
		return executionplan.ExecutablePlan{}, fault.New(fault.FailedPrecondition, "runtime workload-unit lowerer is required")
	}
	for i := range working.Objects {
		lowered, err := b.Lowerer.Lower(ctx, agentruntime.UnitLoweringRequest{Runtime: request.Runtime, Object: working.Objects[i]})
		if err != nil {
			return executionplan.ExecutablePlan{}, err
		}
		working.Objects[i] = lowered
	}
	if err := b.Validator.Validate(ctx, request, &working); err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	return executionplan.ExecutablePlan{
		Digests:   executionplan.Digests{CompiledHash: working.CompiledHash},
		Objects:   working.Objects,
		Discovery: working.Discovery,
		Update:    working.Update,
	}, nil
}

func validateResolvedBindings(bindings []environment.ResolvedBinding, targets map[string]executionplan.MutationTarget) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.Kind == "" || binding.RequirementName == "" {
			return fault.New(fault.InvalidArgument, "resolved environment binding kind and requirement name are required")
		}
		key := string(binding.Kind) + "\x00" + binding.RequirementName
		if _, found := seen[key]; found {
			return fault.New(fault.InvalidArgument, "duplicate resolved environment binding")
		}
		seen[key] = struct{}{}
		if binding.Profile.Type == "" {
			return fault.New(fault.InvalidArgument, "resolved environment binding profile type is required")
		}
		if len(binding.Consumers) == 0 {
			return fault.New(fault.InvalidArgument, "resolved environment binding must contain at least one consumer")
		}
		consumerNames := make(map[string]struct{}, len(binding.Consumers))
		for _, consumer := range binding.Consumers {
			target, found := targets[consumer.TargetName]
			if !found {
				return fault.New(fault.InvalidArgument, "resolved environment binding references an unknown target")
			}
			if _, found := consumerNames[consumer.TargetName]; found {
				return fault.New(fault.InvalidArgument, "resolved environment binding contains a duplicate consumer")
			}
			consumerNames[consumer.TargetName] = struct{}{}
			if consumer.ObjectName != target.ObjectName || consumer.Role != target.Role {
				return fault.New(fault.InvalidArgument, "resolved environment consumer does not match its target")
			}
		}
	}
	return nil
}
