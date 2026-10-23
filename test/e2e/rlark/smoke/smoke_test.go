package smoke_test

import (
	"context"
	"os"
	"testing"

	shared "github.com/rlinf/rlark/test/e2e/framework"
	rlarkframework "github.com/rlinf/rlark/test/e2e/rlark/framework"
	"github.com/rlinf/rlark/test/e2e/rlark/smoke"
)

func TestRLarkSmoke(t *testing.T) {
	if os.Getenv("E2E_RUN") != "1" {
		t.Skip("run through make test-e2e-rlark")
	}
	config, err := rlarkframework.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	input, err := smoke.LoadInput(config.Shared)
	if err != nil {
		t.Fatal(err)
	}
	run, err := shared.NewRun(shared.RunConfig{Framework: config.Shared, LabelPrefix: rlarkframework.LabelPrefix}, "rlark-smoke")
	if err != nil {
		t.Fatal(err)
	}
	command := shared.ExecRunner{}
	scenario := smoke.Scenario(input, config, command)
	provider, err := config.NewProvider(scenario.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), input.Timeout)
	defer cancel()
	if err := shared.RunScenario(ctx, shared.NewScenarioRunner(config.Shared, provider), run, scenario); err != nil {
		t.Fatal(err)
	}
}
