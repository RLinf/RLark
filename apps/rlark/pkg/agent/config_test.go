package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestDefaultControllerConcurrency(t *testing.T) {
	got := DefaultConfig().ControllerConcurrency
	if got.TaskPull != 8 || got.AddonPull != 8 || got.TaskDeployment != 8 ||
		got.TaskDaemonSet != 8 || got.TaskStatefulSet != 8 || got.NodePush != 8 || got.PodPush != 8 {
		t.Fatalf("default controller concurrency = %+v, want all 8", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("default controller concurrency is invalid: %v", err)
	}
}

func TestDefaultPodOrphanSweep(t *testing.T) {
	config := DefaultConfig()
	if config.PodOrphanSweepInterval != 5*time.Minute || config.PodOrphanSweepPageSize != 200 || config.PodStaleTTL != 15*time.Minute {
		t.Fatalf("default Pod orphan sweep = %s/%d/%s", config.PodOrphanSweepInterval, config.PodOrphanSweepPageSize, config.PodStaleTTL)
	}
	if config.TaskRuntimeResyncPeriod != 15*time.Minute {
		t.Fatalf("default Task Runtime resync period = %s", config.TaskRuntimeResyncPeriod)
	}
}

func TestTaskRuntimeResyncFlag(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{"--task-runtime-resync-period=0"}); err != nil {
		t.Fatal(err)
	}
	if config.TaskRuntimeResyncPeriod != 0 {
		t.Fatalf("Task Runtime resync period = %s", config.TaskRuntimeResyncPeriod)
	}
}

func TestTaskRuntimeResyncFlagDocumentsNodeOnlyScope(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	flag := fs.Lookup("task-runtime-resync-period")
	if flag == nil || !strings.Contains(flag.Usage, "unused in node-only mode") {
		t.Fatalf("Task Runtime resync flag usage = %q", flag.Usage)
	}
}

func TestTaskRuntimeResyncValidationAppliesOnlyToClusterModes(t *testing.T) {
	if err := validateTaskRuntimeResyncPeriod("cluster", -time.Second); err == nil {
		t.Fatal("cluster mode accepted a negative Task Runtime resync period")
	}
	if err := validateTaskRuntimeResyncPeriod("both", -time.Second); err == nil {
		t.Fatal("both mode accepted a negative Task Runtime resync period")
	}
	if err := validateTaskRuntimeResyncPeriod("node", -time.Second); err != nil {
		t.Fatalf("node-only mode rejected an unused Task Runtime resync period: %v", err)
	}
}

func TestControllerConcurrencyFlags(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{
		"--task-pull-controller-workers=1",
		"--addon-pull-controller-workers=2",
		"--task-deployment-push-controller-workers=3",
		"--task-daemonset-push-controller-workers=4",
		"--task-statefulset-push-controller-workers=5",
		"--node-push-controller-workers=6",
		"--pod-push-controller-workers=7",
	}); err != nil {
		t.Fatal(err)
	}
	want := ControllerConcurrencyConfig{
		TaskPull: 1, AddonPull: 2, TaskDeployment: 3, TaskDaemonSet: 4,
		TaskStatefulSet: 5, NodePush: 6, PodPush: 7,
	}
	if config.ControllerConcurrency != want {
		t.Fatalf("controller concurrency = %+v, want %+v", config.ControllerConcurrency, want)
	}
}

func TestControllerConcurrencyValidation(t *testing.T) {
	config := defaultControllerConcurrencyConfig()
	config.PodPush = 0
	if err := config.Validate(); err == nil {
		t.Fatal("expected validation error for zero Pod push workers")
	}
}

func TestLeaderElectionFlags(t *testing.T) {
	config := DefaultConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.SetupFlags(fs)
	if err := fs.Parse([]string{
		"--leader-election=true",
		"--leader-election-key=rlark-system/agent",
		"--leader-election-id=agent-1",
	}); err != nil {
		t.Fatal(err)
	}
	if !config.LeaderElection.Enabled || config.LeaderElection.Key != "rlark-system/agent" || config.LeaderElection.Identity != "agent-1" {
		t.Fatalf("leader election config = %+v", config.LeaderElection)
	}
}
