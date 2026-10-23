package framework

import (
	"context"
	"errors"
	"fmt"
)

type Scenario[E any] struct {
	Name         string
	Requirements Requirements
	Validate     func(context.Context, *Run) error
	Prepare      func(context.Context, *Run, *ProvisionedEnvironment) (E, error)
	Execute      func(context.Context, *Run, E) error
	Collect      func(context.Context, *Run, E) error
	Cleanup      func(context.Context, *Run, E) error
}

type ScenarioRunner struct {
	Provider          Provider
	RetainEnvironment bool
	CleanupContext    func() (context.Context, context.CancelFunc)
}

func NewScenarioRunner(config Config, provider Provider) ScenarioRunner {
	return ScenarioRunner{
		Provider:          provider,
		RetainEnvironment: config.RetainEnvironment,
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), config.CleanupTimeout)
		},
	}
}

// RunScenario owns the infrastructure and product lifecycle. Collection and
// cleanup use a fresh bounded context so cancellation of the test body does not
// prevent evidence capture or resource removal.
func RunScenario[E any](ctx context.Context, runner ScenarioRunner, run *Run, scenario Scenario[E]) (err error) {
	if runner.Provider == nil {
		return fmt.Errorf("scenario provider must not be nil")
	}
	if run == nil {
		return fmt.Errorf("scenario run must not be nil")
	}
	if scenario.Prepare == nil || scenario.Execute == nil {
		return fmt.Errorf("scenario %q must define prepare and execute", scenario.Name)
	}
	if runner.CleanupContext == nil {
		return fmt.Errorf("scenario cleanup context factory must not be nil")
	}
	if scenario.Validate != nil {
		if validateErr := scenario.Validate(ctx, run); validateErr != nil {
			return fmt.Errorf("validate scenario %q: %w", scenario.Name, validateErr)
		}
	}

	provisioned, provisionErr := runner.Provider.Provision(ctx, run, scenario.Requirements)
	if provisionErr != nil {
		return fmt.Errorf("provision scenario %q: %w", scenario.Name, provisionErr)
	}

	cleanupCtx, cancelCleanup := runner.CleanupContext()
	defer cancelCleanup()
	var environment E
	prepared := false
	defer func() {
		if prepared && scenario.Collect != nil {
			err = errors.Join(err, wrapPhase("collect product", scenario.Collect(cleanupCtx, run, environment)))
		}
		err = errors.Join(err, wrapPhase("collect infrastructure", runner.Provider.Collect(cleanupCtx, run, provisioned)))
		if !runner.RetainEnvironment && prepared && scenario.Cleanup != nil {
			err = errors.Join(err, wrapPhase("cleanup product", scenario.Cleanup(cleanupCtx, run, environment)))
		}
		if !runner.RetainEnvironment {
			err = errors.Join(err, wrapPhase("destroy infrastructure", runner.Provider.Destroy(cleanupCtx, run, provisioned)))
		}
	}()

	var prepareErr error
	environment, prepareErr = scenario.Prepare(ctx, run, provisioned)
	if prepareErr != nil {
		return fmt.Errorf("prepare scenario %q: %w", scenario.Name, prepareErr)
	}
	prepared = true
	if executeErr := scenario.Execute(ctx, run, environment); executeErr != nil {
		return fmt.Errorf("execute scenario %q: %w", scenario.Name, executeErr)
	}
	return nil
}

func wrapPhase(phase string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", phase, err)
}
