package framework

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNewScenarioRunnerAppliesCleanupPolicy(t *testing.T) {
	provider := lifecycleProvider{steps: &[]string{}}
	runner := NewScenarioRunner(Config{
		CleanupTimeout:    time.Second,
		RetainEnvironment: true,
	}, provider)
	if !runner.RetainEnvironment || runner.Provider == nil || runner.CleanupContext == nil {
		t.Fatalf("runner = %#v", runner)
	}
	ctx, cancel := runner.CleanupContext()
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("cleanup context has no deadline")
	}
}

type lifecycleProvider struct {
	steps        *[]string
	provisionErr error
	collectErr   error
	destroyErr   error
}

func (p lifecycleProvider) Provision(context.Context, *Run, Requirements) (*ProvisionedEnvironment, error) {
	*p.steps = append(*p.steps, "provision")
	return &ProvisionedEnvironment{}, p.provisionErr
}

func (p lifecycleProvider) Collect(context.Context, *Run, *ProvisionedEnvironment) error {
	*p.steps = append(*p.steps, "provider-collect")
	return p.collectErr
}

func (p lifecycleProvider) Destroy(context.Context, *Run, *ProvisionedEnvironment) error {
	*p.steps = append(*p.steps, "destroy")
	return p.destroyErr
}

func TestRunScenarioOwnsLifecycleOrder(t *testing.T) {
	var steps []string
	runner := ScenarioRunner{
		Provider: lifecycleProvider{steps: &steps},
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[string]{
		Name: "ordered",
		Validate: func(context.Context, *Run) error {
			steps = append(steps, "validate")
			return nil
		},
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (string, error) {
			steps = append(steps, "prepare")
			return "environment", nil
		},
		Execute: func(context.Context, *Run, string) error {
			steps = append(steps, "execute")
			return nil
		},
		Collect: func(context.Context, *Run, string) error {
			steps = append(steps, "product-collect")
			return nil
		},
		Cleanup: func(context.Context, *Run, string) error {
			steps = append(steps, "product-cleanup")
			return nil
		},
	}
	if err := RunScenario(context.Background(), runner, &Run{}, scenario); err != nil {
		t.Fatal(err)
	}
	want := []string{"validate", "provision", "prepare", "execute", "product-collect", "provider-collect", "product-cleanup", "destroy"}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %#v, want %#v", steps, want)
	}
}

func TestRunScenarioDoesNotProvisionAfterValidationFailure(t *testing.T) {
	var steps []string
	runner := ScenarioRunner{
		Provider: lifecycleProvider{steps: &steps},
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[struct{}]{
		Name: "invalid",
		Validate: func(context.Context, *Run) error {
			steps = append(steps, "validate")
			return errors.New("invalid input")
		},
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (struct{}, error) { return struct{}{}, nil },
		Execute: func(context.Context, *Run, struct{}) error { return nil },
	}
	if err := RunScenario(context.Background(), runner, &Run{}, scenario); err == nil {
		t.Fatal("validation failure was ignored")
	}
	if !reflect.DeepEqual(steps, []string{"validate"}) {
		t.Fatalf("steps = %#v", steps)
	}
}

func TestRunScenarioCollectsAndCleansAfterExecutionFailure(t *testing.T) {
	var steps []string
	executeErr := errors.New("execute failed")
	runner := ScenarioRunner{
		Provider: lifecycleProvider{steps: &steps, collectErr: errors.New("collect failed")},
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[struct{}]{
		Name: "failure",
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (struct{}, error) {
			steps = append(steps, "prepare")
			return struct{}{}, nil
		},
		Execute: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "execute")
			return executeErr
		},
		Collect: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "product-collect")
			return nil
		},
		Cleanup: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "product-cleanup")
			return nil
		},
	}
	err := RunScenario(context.Background(), runner, &Run{}, scenario)
	if !errors.Is(err, executeErr) || !containsStep(steps, "destroy") {
		t.Fatalf("err = %v, steps = %#v", err, steps)
	}
}

func TestRunScenarioDoesNotDestroyAfterProvisionFailure(t *testing.T) {
	var steps []string
	runner := ScenarioRunner{
		Provider: lifecycleProvider{steps: &steps, provisionErr: errors.New("provision failed")},
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[struct{}]{
		Name:    "provision-failure",
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (struct{}, error) { return struct{}{}, nil },
		Execute: func(context.Context, *Run, struct{}) error { return nil },
	}
	if err := RunScenario(context.Background(), runner, &Run{}, scenario); err == nil {
		t.Fatal("provision failure was ignored")
	}
	if containsStep(steps, "destroy") {
		t.Fatalf("destroy ran without a provisioned environment: %#v", steps)
	}
}

func TestRunScenarioCollectsInfrastructureAfterPrepareFailure(t *testing.T) {
	var steps []string
	runner := ScenarioRunner{
		Provider: lifecycleProvider{steps: &steps},
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[struct{}]{
		Name: "prepare-failure",
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (struct{}, error) {
			steps = append(steps, "prepare")
			return struct{}{}, errors.New("prepare failed")
		},
		Execute: func(context.Context, *Run, struct{}) error { return nil },
	}
	if err := RunScenario(context.Background(), runner, &Run{}, scenario); err == nil {
		t.Fatal("prepare failure was ignored")
	}
	want := []string{"provision", "prepare", "provider-collect", "destroy"}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %#v, want %#v", steps, want)
	}
}

func TestRunScenarioRetainsEnvironmentButCollectsEvidence(t *testing.T) {
	var steps []string
	runner := ScenarioRunner{
		Provider:          lifecycleProvider{steps: &steps},
		RetainEnvironment: true,
		CleanupContext: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		},
	}
	scenario := Scenario[struct{}]{
		Name: "retain",
		Prepare: func(context.Context, *Run, *ProvisionedEnvironment) (struct{}, error) {
			steps = append(steps, "prepare")
			return struct{}{}, nil
		},
		Execute: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "execute")
			return nil
		},
		Collect: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "product-collect")
			return nil
		},
		Cleanup: func(context.Context, *Run, struct{}) error {
			steps = append(steps, "product-cleanup")
			return nil
		},
	}
	if err := RunScenario(context.Background(), runner, &Run{}, scenario); err != nil {
		t.Fatal(err)
	}
	want := []string{"provision", "prepare", "execute", "product-collect", "provider-collect"}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %#v, want %#v", steps, want)
	}
}

func containsStep(steps []string, expected string) bool {
	for _, step := range steps {
		if step == expected {
			return true
		}
	}
	return false
}
