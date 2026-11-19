package raylauncher

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNoRoleRunsOnlyScriptsWithoutRay(t *testing.T) {
	t.Setenv("RLARK_RUN_SCRIPT", "run")
	old := commandRunner
	defer func() { commandRunner = old }()
	calls := make(chan []string, 4)
	commandRunner = func(_ context.Context, executable string, args ...string) error {
		calls <- append([]string{executable}, args...)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		// The Ray binary does not exist: a Task without a Ray role must not need it.
		done <- Run(ctx, Config{RayBinary: filepath.Join(t.TempDir(), "missing-ray"), BashBinary: "/bin/bash", PythonBinary: "python", Prepared: true})
	}()
	if call := <-calls; !reflect.DeepEqual(call, []string{"/bin/bash", "-c", "run"}) {
		t.Fatalf("call=%#v, want the run script", call)
	}
	select {
	case err := <-done:
		t.Fatalf("launcher exited before cancellation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("unexpected calls after the run script: %#v", <-calls)
	}
}
