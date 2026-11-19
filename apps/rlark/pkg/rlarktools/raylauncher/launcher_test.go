package raylauncher

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunWorkerBuildsRayArgvWithoutShell(t *testing.T) {
	stubExecutableCheck(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	t.Setenv("RLARK_HEAD_ADDRESS", "127.0.0.1")
	t.Setenv("RLARK_RAY_PORT", strconv.Itoa(port))
	old := commandRunner
	defer func() { commandRunner = old }()
	var calls [][]string
	commandRunner = func(_ context.Context, executable string, args ...string) error {
		calls = append(calls, append([]string{executable}, args...))
		return nil
	}
	if err := Run(context.Background(), Config{Role: RoleWorker, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", Prepared: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"ray", "start", "--address=127.0.0.1:" + strconv.Itoa(port), "--temp-dir", "/tmp/ray", "--block"}
	found := false
	for _, call := range calls {
		if reflect.DeepEqual(call, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("calls=%#v, want %v", calls, want)
	}
}

func TestPrepareScriptFailureStopsBeforeRayStart(t *testing.T) {
	t.Setenv("RLARK_PREPARE_SCRIPT", "exit 9")
	old := commandRunner
	defer func() { commandRunner = old }()
	calls := 0
	commandRunner = func(_ context.Context, executable string, args ...string) error {
		calls++
		return errors.New("prepare failed")
	}
	if err := Run(context.Background(), Config{Role: RoleHead, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", SelfBinary: "/rlark-tools/rlark-tools"}); err == nil {
		t.Fatal("prepare failure was ignored")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want only prepare", calls)
	}
}

func TestNetworkWaitRunsBeforePrepare(t *testing.T) {
	t.Setenv("RLARK_HEAD_ADDRESS", "training-head-0.rlark-domain")
	t.Setenv("RLARK_PREPARE_SCRIPT", "prepare")
	oldResolver, oldRunner := hostResolver, commandRunner
	defer func() { hostResolver, commandRunner = oldResolver, oldRunner }()
	order := []string{}
	hostResolver = func(host string) ([]string, error) {
		order = append(order, "network:"+host)
		return []string{"10.0.0.1"}, nil
	}
	commandRunner = func(context.Context, string, ...string) error {
		order = append(order, "prepare")
		return nil
	}
	err := Run(context.Background(), Config{Role: RoleWorker, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", SelfBinary: "/rlark-tools/rlark-tools", WaitNetwork: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"network:training-head-0.rlark-domain", "prepare"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order=%#v, want %#v", order, want)
	}
}

func TestHeadNetworkWaitHonorsCancellation(t *testing.T) {
	old := hostResolver
	defer func() { hostResolver = old }()
	hostResolver = func(string) ([]string, error) { return nil, errors.New("not ready") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, Config{Role: RoleHead, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", SelfBinary: "/rlark-tools/rlark-tools", WaitNetwork: true})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "wait for RLark network host") {
		t.Fatalf("error=%v, want canceled network wait", err)
	}
}

func TestMissingRayCLIStopsBeforeRayCommands(t *testing.T) {
	old := commandRunner
	defer func() { commandRunner = old }()
	calls := 0
	commandRunner = func(context.Context, string, ...string) error {
		calls++
		return nil
	}
	err := Run(context.Background(), Config{Role: RoleHead, RayBinary: filepath.Join(t.TempDir(), "missing-ray"), BashBinary: "/bin/bash", PythonBinary: "python", Prepared: true})
	if err == nil || !strings.Contains(err.Error(), "ray CLI") {
		t.Fatalf("error=%v, want missing Ray CLI error", err)
	}
	if calls != 0 {
		t.Fatalf("calls=%d, want no Ray commands", calls)
	}
}

func TestPreserveExitRetainsUserScriptStatus(t *testing.T) {
	err := run(context.Background(), "/bin/sh", "-c", "exit 9")
	got := preserveExit("run script", err)
	var exitErr *ExitError
	if !errors.As(got, &exitErr) || exitErr.Code != 9 {
		t.Fatalf("error=%#v", got)
	}
}

func TestWorkerPreservesRayExitFailure(t *testing.T) {
	stubExecutableCheck(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	t.Setenv("RLARK_HEAD_ADDRESS", "127.0.0.1")
	t.Setenv("RLARK_RAY_PORT", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	old := commandRunner
	defer func() { commandRunner = old }()
	commandRunner = func(ctx context.Context, executable string, args ...string) error {
		if len(args) > 0 && args[0] == "start" {
			return run(ctx, "/bin/sh", "-c", "exit 7")
		}
		return nil
	}
	err = Run(context.Background(), Config{Role: RoleWorker, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", Prepared: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Operation != "start Ray worker" || exitErr.Code != 7 {
		t.Fatalf("error=%#v, want worker exit code 7", err)
	}
}

func TestHeadWithoutRunScriptStaysAliveUntilCancellation(t *testing.T) {
	stubExecutableCheck(t)
	t.Setenv("RLARK_NODE_IP", "10.0.0.1")
	oldRunner, oldStarter := commandRunner, headStarter
	defer func() { commandRunner, headStarter = oldRunner, oldStarter }()
	headStarter = func(context.Context, string, string, string, string) error { return nil }
	commandRunner = func(context.Context, string, ...string) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Role: RoleHead, RayBinary: "ray", BashBinary: "/bin/bash", PythonBinary: "python", Prepared: true})
	}()
	select {
	case err := <-done:
		t.Fatalf("head exited before cancellation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("head did not stop after cancellation")
	}
}

func stubExecutableCheck(t *testing.T) {
	t.Helper()
	old := executableChecker
	executableChecker = func(string) error { return nil }
	t.Cleanup(func() { executableChecker = old })
}

func TestRunPreparedPreservesExportAndWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "result")
	helper := filepath.Join(directory, "continuation.sh")
	content := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$PREPARED_VALUE\" \"$PWD\" > \"$1\"\n"
	if err := os.WriteFile(helper, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	config := Config{BashBinary: "/bin/bash", SelfBinary: helper}
	script := "export PREPARED_VALUE=ready; cd " + directory
	if err := runPrepared(context.Background(), config, script, []string{output}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ready\n"+directory+"\n" {
		t.Fatalf("result=%q", data)
	}
}
