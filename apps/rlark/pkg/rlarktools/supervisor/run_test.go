//go:build unix

package supervisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	rlarklog "github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func TestRunReturnsPrimaryExitAndStopsAuxiliary(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{{Name: "primary", Role: executionprocess.RolePrimary, Argv: []string{"/bin/sh", "-c", "exit 7"}, RestartPolicy: executionprocess.RestartNever, Required: true}, {Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: []string{"/bin/sh", "-c", "while true; do sleep 1; done"}, RestartPolicy: executionprocess.RestartOnFailure, Required: true}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var exitErr *ExitError
	if err := Run(ctx, config); !errors.As(err, &exitErr) || exitErr.Code != 7 {
		t.Fatalf("primary exit error=%v", err)
	}
}

func TestRunHonorsCancellation(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{{Name: "primary", Role: executionprocess.RolePrimary, Argv: []string{"/bin/sh", "-c", "while true; do sleep 1; done"}, RestartPolicy: executionprocess.RestartNever, Required: true}}}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := Run(ctx, config); err == nil {
		t.Fatal("cancellation was not returned")
	}
}

func TestCancellationWaitsForPrimaryExitBeforeStoppingAuxiliary(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	directory := t.TempDir()
	primaryTerm := filepath.Join(directory, "primary-term")
	auxiliaryTerm := filepath.Join(directory, "auxiliary-term")
	releasePrimary := filepath.Join(directory, "release-primary")
	primaryReady := filepath.Join(directory, "primary-ready")
	auxiliaryReady := filepath.Join(directory, "auxiliary-ready")
	helper := supervisorHelperCommand(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{
		{Name: "primary", Role: executionprocess.RolePrimary, Argv: append(helper, "primary", primaryTerm, releasePrimary, primaryReady), RestartPolicy: executionprocess.RestartNever, Required: true},
		{Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: append(helper, "auxiliary", auxiliaryTerm, auxiliaryReady), RestartPolicy: executionprocess.RestartOnFailure, Required: true},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()
	waitForFile(t, primaryReady, time.Second)
	waitForFile(t, auxiliaryReady, time.Second)
	cancel()
	waitForFile(t, primaryTerm, time.Second)
	if _, err := os.Stat(auxiliaryTerm); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("auxiliary stopped before primary exited: %v", err)
	}
	if err := os.WriteFile(releasePrimary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, auxiliaryTerm, time.Second)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not finish after ordered shutdown")
	}
}

func TestRequiredAuxiliaryFailureWaitsForPrimaryExit(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	directory := t.TempDir()
	primaryTerm := filepath.Join(directory, "primary-term")
	releasePrimary := filepath.Join(directory, "release-primary")
	primaryReady := filepath.Join(directory, "primary-ready")
	helper := supervisorHelperCommand(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{
		{Name: "primary", Role: executionprocess.RolePrimary, Argv: append(helper, "primary", primaryTerm, releasePrimary, primaryReady), RestartPolicy: executionprocess.RestartNever, Required: true},
		{Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: append(helper, "exit-after-file", primaryReady, "0"), RestartPolicy: executionprocess.RestartOnFailure, Required: true},
	}}
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), config) }()
	waitForFile(t, primaryTerm, time.Second)
	select {
	case err := <-done:
		t.Fatalf("supervisor returned before primary exited: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.WriteFile(releasePrimary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "required process \"aux\" exited") {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not finish after primary exit")
	}
}

func TestOptionalAuxiliaryInitialStartFailureDoesNotFailPrimary(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{
		{Name: "primary", Role: executionprocess.RolePrimary, Argv: []string{"/bin/sh", "-c", "exit 0"}, RestartPolicy: executionprocess.RestartNever, Required: true},
		{Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: []string{filepath.Join(t.TempDir(), "missing-auxiliary")}, RestartPolicy: executionprocess.RestartOnFailure, Required: false},
	}}
	if err := Run(context.Background(), config); err != nil {
		t.Fatalf("optional auxiliary start failure failed primary: %v", err)
	}
}

func TestOptionalAuxiliaryRestartStartFailureDoesNotStopPrimary(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	directory := t.TempDir()
	primaryReady := filepath.Join(directory, "primary-ready")
	primaryTerm := filepath.Join(directory, "primary-term")
	releasePrimary := filepath.Join(directory, "release-primary")
	auxiliaryStarts := filepath.Join(directory, "auxiliary-starts")
	auxiliary := filepath.Join(directory, "auxiliary")
	helper := supervisorHelperCommand(t)
	content := "#!/bin/sh\nprintf 'start\\n' >> \"$1\"\nrm -- \"$0\"\nexit 1\n"
	if err := os.WriteFile(auxiliary, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{
		{Name: "primary", Role: executionprocess.RolePrimary, Argv: append(helper, "primary", primaryTerm, releasePrimary, primaryReady), RestartPolicy: executionprocess.RestartNever, Required: true},
		{Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: []string{auxiliary, auxiliaryStarts}, RestartPolicy: executionprocess.RestartOnFailure, Required: false},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()
	waitForFile(t, primaryReady, time.Second)
	waitForFile(t, auxiliaryStarts, time.Second)
	time.Sleep(restartBackoff + 100*time.Millisecond)
	if _, err := os.Stat(primaryTerm); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("optional auxiliary restart failure stopped primary: %v", err)
	}
	cancel()
	waitForFile(t, primaryTerm, time.Second)
	if err := os.WriteFile(releasePrimary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after cancellation")
	}
}

func TestCancellationDuringAuxiliaryBackoffDoesNotRestartAuxiliary(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	directory := t.TempDir()
	primaryReady := filepath.Join(directory, "primary-ready")
	primaryTerm := filepath.Join(directory, "primary-term")
	releasePrimary := filepath.Join(directory, "release-primary")
	auxiliaryStarts := filepath.Join(directory, "auxiliary-starts")
	helper := supervisorHelperCommand(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{
		{Name: "primary", Role: executionprocess.RolePrimary, Argv: append(helper, "primary", primaryTerm, releasePrimary, primaryReady), RestartPolicy: executionprocess.RestartNever, Required: true},
		{Name: "aux", Role: executionprocess.RoleAuxiliary, Argv: append(helper, "record-and-exit", auxiliaryStarts, "1"), RestartPolicy: executionprocess.RestartOnFailure, Required: true},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()
	waitForFile(t, primaryReady, time.Second)
	waitForFile(t, auxiliaryStarts, time.Second)
	cancel()
	waitForFile(t, primaryTerm, time.Second)
	if err := os.WriteFile(releasePrimary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after backoff cancellation")
	}
	data, err := os.ReadFile(auxiliaryStarts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "start") != 1 {
		t.Fatalf("auxiliary restarted during shutdown: %q", data)
	}
}

func TestNextAuxiliaryRestartCount(t *testing.T) {
	tests := []struct {
		name     string
		previous int
		runtime  time.Duration
		want     int
	}{
		{name: "first unstable failure", runtime: time.Second, want: 1},
		{name: "consecutive unstable failure", previous: 2, runtime: auxiliaryStableRun - time.Nanosecond, want: 3},
		{name: "stable run resets sequence", previous: maxAuxiliaryRestarts, runtime: auxiliaryStableRun, want: 1},
		{name: "long stable run resets sequence", previous: maxAuxiliaryRestarts, runtime: auxiliaryStableRun + time.Hour, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextAuxiliaryRestartCount(test.previous, test.runtime); got != test.want {
				t.Fatalf("nextAuxiliaryRestartCount(%d, %s)=%d, want %d", test.previous, test.runtime, got, test.want)
			}
		})
	}
}

func TestRunContinuesWhenSubreaperIsUnavailable(t *testing.T) {
	requireIsolatedSupervisorTest(t)
	config := Config{APIVersion: APIVersion, Processes: []ProcessConfig{{
		Name:          "primary",
		Role:          executionprocess.RolePrimary,
		Argv:          []string{"/bin/sh", "-c", "exit 0"},
		RestartPolicy: executionprocess.RestartNever,
		Required:      true,
	}}}
	want := errors.New("operation denied")
	ctx := rlarklog.WithLogger(context.Background(), testr.New(t))
	if err := run(ctx, config, func() error { return want }); err != nil {
		t.Fatalf("run returned subreaper setup error: %v", err)
	}
}

func TestWaitForRoleExitIsBoundedWhenProcessCannotBeReaped(t *testing.T) {
	running := map[string]*runningProcess{
		"aux": {config: ProcessConfig{Name: "aux", Role: executionprocess.RoleAuxiliary}},
	}
	started := time.Now()
	if waitForRoleExit(running, executionprocess.RoleAuxiliary, 20*time.Millisecond, time.Millisecond, func() {}) {
		t.Fatal("unreaped auxiliary was reported as exited")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("bounded wait took %s", elapsed)
	}
}

func TestSupervisorHelperProcess(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	args := os.Args[separator+1:]
	if len(args) == 0 {
		os.Exit(2)
	}
	touch := func(path string) {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			os.Exit(3)
		}
	}
	wait := func(path string) {
		for {
			if _, err := os.Stat(path); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	switch args[0] {
	case "primary":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		touch(args[3])
		<-signals
		touch(args[1])
		wait(args[2])
		os.Exit(0)
	case "auxiliary":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		touch(args[2])
		<-signals
		touch(args[1])
		os.Exit(0)
	case "exit-after-file":
		wait(args[1])
		code, _ := strconv.Atoi(args[2])
		os.Exit(code)
	case "record-and-exit":
		file, err := os.OpenFile(args[1], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(3)
		}
		_, _ = file.WriteString("start\n")
		_ = file.Close()
		code, _ := strconv.Atoi(args[2])
		os.Exit(code)
	default:
		os.Exit(2)
	}
}

func supervisorHelperCommand(t *testing.T) []string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{executable, "-test.run=^TestSupervisorHelperProcess$", "--"}
}

func requireIsolatedSupervisorTest(t *testing.T) {
	t.Helper()
	if raceEnabled {
		t.Skip("process-wide wait4 ownership cannot be isolated under the race test wrapper")
	}
	if os.Getenv("RLARK_SUPERVISOR_ISOLATED_TEST") == "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), "RLARK_SUPERVISOR_ISOLATED_TEST=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("isolated supervisor test failed: %v", err)
	}
	t.SkipNow()
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %q was not created", path)
}
