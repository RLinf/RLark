package kind

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	shared "github.com/rlinf/rlark/test/e2e/framework"
)

type recordingRunner struct {
	commands []string
	failAt   int
	failures int
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if r.failures > 0 {
		r.failures--
		return nil, errors.New("injected failure")
	}
	if r.failAt > 0 && len(r.commands) == r.failAt {
		return nil, fmt.Errorf("injected failure")
	}
	return nil, nil
}

func TestProviderRetriesTransientDestroyFailure(t *testing.T) {
	runner := &recordingRunner{failures: 1}
	provider := New(Config{Command: runner, RetryWait: time.Nanosecond})
	environment := &shared.ProvisionedEnvironment{
		Clusters: map[string]shared.ClusterConfig{"smoke": {}},
		Metadata: map[string]string{"cluster.smoke": "owned-cluster"},
	}

	if err := provider.Destroy(context.Background(), testRun(t), environment); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v, want two delete attempts", runner.commands)
	}
}

func TestProviderCreatesAndDestroysScenarioRequirements(t *testing.T) {
	runner := &recordingRunner{}
	provider := New(Config{Command: runner, NodeImage: "kindest/node:immutable"})
	run := testRun(t)
	requirements := shared.Requirements{Clusters: []shared.ClusterRequirement{
		{Role: "hub", ControlNodes: 1},
		{Role: "worker-east", ControlNodes: 1, WorkerNodes: 2},
	}}

	environment, err := provider.Provision(context.Background(), run, requirements)
	if err != nil {
		t.Fatal(err)
	}
	if len(environment.Clusters) != 2 || environment.Metadata["provider"] != ProviderName {
		t.Fatalf("environment = %#v", environment)
	}
	if err := provider.Destroy(context.Background(), run, environment); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 4 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	if !strings.Contains(runner.commands[0], "create cluster") ||
		!strings.Contains(runner.commands[0], "--image kindest/node:immutable") ||
		!strings.Contains(runner.commands[3], "delete cluster") {
		t.Fatalf("commands = %#v", runner.commands)
	}
}

func TestProviderRollsBackCreatedClustersAfterFailure(t *testing.T) {
	runner := &recordingRunner{failAt: 2}
	provider := New(Config{Command: runner})
	run := testRun(t)
	requirements := shared.Requirements{Clusters: []shared.ClusterRequirement{
		{Role: "first", ControlNodes: 1},
		{Role: "second", ControlNodes: 1},
	}}

	if _, err := provider.Provision(context.Background(), run, requirements); err == nil {
		t.Fatal("injected create failure was ignored")
	}
	if len(runner.commands) != 3 || !strings.Contains(runner.commands[2], "delete cluster") {
		t.Fatalf("rollback commands = %#v", runner.commands)
	}
}

func testRun(t *testing.T) *shared.Run {
	t.Helper()
	return &shared.Run{ID: "run-123", ArtifactsDir: t.TempDir()}
}
