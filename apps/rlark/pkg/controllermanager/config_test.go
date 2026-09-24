package controllermanager

import (
	"testing"

	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
)

func TestDefaultControllerConcurrency(t *testing.T) {
	config := DefaultConfig()
	got := config.ControllerConcurrency
	if got.Job != 8 || got.Task != 8 || got.Workflow != 8 || got.Node != 8 || got.Domain != 8 ||
		got.JobSync != 8 || got.TaskSync != 8 || got.WorkflowSync != 8 || got.NodeSync != 8 {
		t.Fatalf("default controller concurrency = %+v, want all 8", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("default controller concurrency is invalid: %v", err)
	}
}

func TestControllerConcurrencyFlags(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{
		"--job-controller-workers=1",
		"--task-controller-workers=2",
		"--workflow-controller-workers=3",
		"--node-controller-workers=4",
		"--domain-controller-workers=5",
		"--job-sync-controller-workers=6",
		"--task-sync-controller-workers=7",
		"--workflow-sync-controller-workers=8",
		"--node-sync-controller-workers=9",
	}); err != nil {
		t.Fatal(err)
	}
	want := ControllerConcurrencyConfig{
		Job: 1, Task: 2, Workflow: 3, Node: 4, Domain: 5,
		JobSync: 6, TaskSync: 7, WorkflowSync: 8, NodeSync: 9,
	}
	if config.ControllerConcurrency != want {
		t.Fatalf("controller concurrency = %+v, want %+v", config.ControllerConcurrency, want)
	}
}

func TestControllerConcurrencyValidation(t *testing.T) {
	config := defaultControllerConcurrencyConfig()
	config.Domain = 0
	if err := config.Validate(); err == nil {
		t.Fatal("expected validation error for zero Domain workers")
	}
}

func TestLeaderElectionFlags(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{"--leader-election=false", "--leader-election-key=custom-lock"}); err != nil {
		t.Fatal(err)
	}
	if config.LeaderElection.Enabled || config.LeaderElection.Key != "custom-lock" {
		t.Fatalf("leader election config = %+v", config.LeaderElection)
	}
}

func TestManagerOptionsDisabledLeaderElectionAllowsEmptyKey(t *testing.T) {
	config := DefaultConfig()
	config.LeaderElection.Enabled = false
	config.LeaderElection.Key = ""
	options, err := managerOptions(config, &rest.Config{Host: "https://127.0.0.1"})
	if err != nil {
		t.Fatalf("managerOptions() error = %v", err)
	}
	if options.LeaderElection {
		t.Fatal("leader election unexpectedly enabled")
	}
}

func TestManagerOptionsUsesLeaderElectionIdentity(t *testing.T) {
	config := DefaultConfig()
	config.KubeClientConfig.Namespace = "rlark-system"
	config.LeaderElection.Identity = "controller-1"
	options, err := managerOptions(config, &rest.Config{Host: "https://127.0.0.1"})
	if err != nil {
		t.Fatalf("managerOptions() error = %v", err)
	}
	if options.LeaderElectionResourceLockInterface == nil {
		t.Fatal("custom leader election lock was not configured")
	}
	if options.LeaderElectionResourceLockInterface.Identity() != "controller-1" {
		t.Fatalf("lock identity = %q, want controller-1", options.LeaderElectionResourceLockInterface.Identity())
	}
	if options.LeaderElectionResourceLockInterface.Describe() != "rlark-system/rlark-controller-manager" {
		t.Fatalf("lock = %q", options.LeaderElectionResourceLockInterface.Describe())
	}
}
