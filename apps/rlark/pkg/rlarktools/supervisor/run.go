//go:build unix

package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	rlarklog "github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	maxAuxiliaryRestarts = 3
	restartBackoff       = time.Second
	auxiliaryStableRun   = 5 * time.Minute
	shutdownGrace        = 15 * time.Second
	forcedReapGrace      = 5 * time.Second
)

type processExit struct {
	process ProcessConfig
	code    int
	runtime time.Duration
}

// ExitError preserves the primary process exit code for the supervisor CLI.
type ExitError struct {
	Process string
	Code    int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("process %q exited with code %d", e.Process, e.Code)
}

type runningProcess struct {
	config  ProcessConfig
	process *os.Process
	started time.Time
}

// Run starts each declaration as an independent OS process group. One wait4
// loop reaps both direct children and descendants reparented to this subreaper.
// Shutdown is ordered: request the primary to stop, wait for its actual exit,
// then terminate auxiliaries and start their bounded shutdown grace period.
func Run(ctx context.Context, config Config) error {
	return run(ctx, config, enableSubreaper)
}

func run(ctx context.Context, config Config, enableSubreaperFn func() error) error {
	logger := rlarklog.FromContext(ctx)
	if err := config.Validate(); err != nil {
		return err
	}
	if err := enableSubreaperFn(); err != nil {
		// PID 1 already adopts orphaned descendants. Subreaper mode only improves
		// behavior when this command is run as a non-PID-1 process, so a seccomp
		// denial must not prevent the workload from starting.
		logger.Error(err, "Child subreaper unavailable; continuing")
	}
	sigchld := make(chan os.Signal, 1)
	signal.Notify(sigchld, syscall.SIGCHLD)
	defer signal.Stop(sigchld)
	reapTicker := time.NewTicker(250 * time.Millisecond)
	defer reapTicker.Stop()
	reaperDone := make(chan struct{})
	defer close(reaperDone)
	go func() {
		for {
			select {
			case <-reaperDone:
				return
			case <-reapTicker.C:
				select {
				case sigchld <- syscall.SIGCHLD:
				default:
				}
			}
		}
	}()
	runningByName := map[string]*runningProcess{}
	runningByPID := map[int]*runningProcess{}
	restarts := map[string]int{}
	restartReady := make(chan ProcessConfig, len(config.Processes))
	reapAndLog := func() []processExit {
		exits := reap(runningByName, runningByPID)
		for _, exited := range exits {
			logger.Info("Process exited", "process", exited.process.Name, "role", exited.process.Role, "exitCode", exited.code, "runtime", exited.runtime.Round(time.Millisecond))
		}
		return exits
	}
	start := func(process ProcessConfig) error {
		cmd := exec.Command(process.Argv[0], process.Argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			logger.Error(err, "Process start failed", "process", process.Name, "role", process.Role)
			return fmt.Errorf("start process %q: %w", process.Name, err)
		}
		running := &runningProcess{config: process, process: cmd.Process, started: time.Now()}
		runningByName[process.Name] = running
		runningByPID[cmd.Process.Pid] = running
		logger.Info("Process started", "process", process.Name, "role", process.Role, "pid", cmd.Process.Pid)
		return nil
	}
	finishAuxiliaries := func() {
		if !hasRole(runningByName, executionprocess.RoleAuxiliary) {
			return
		}
		logger.Info("Auxiliary shutdown started", "gracePeriod", shutdownGrace)
		terminateRole(runningByName, executionprocess.RoleAuxiliary, syscall.SIGTERM)
		waitForRoleExit(runningByName, executionprocess.RoleAuxiliary, shutdownGrace, 25*time.Millisecond, func() {
			reapAndLog()
		})
		if hasRole(runningByName, executionprocess.RoleAuxiliary) {
			logger.Info("Auxiliary shutdown escalated", "signal", syscall.SIGKILL.String(), "remaining", countRole(runningByName, executionprocess.RoleAuxiliary))
			terminateRole(runningByName, executionprocess.RoleAuxiliary, syscall.SIGKILL)
			waitForRoleExit(runningByName, executionprocess.RoleAuxiliary, forcedReapGrace, 10*time.Millisecond, func() {
				reapAndLog()
			})
			for _, process := range runningByName {
				if process.config.Role == executionprocess.RoleAuxiliary {
					logger.Info("Process remained unreaped", "process", process.config.Name, "role", process.config.Role, "pid", process.process.Pid, "waited", forcedReapGrace)
				}
			}
		}
	}
	for _, process := range config.Processes {
		if process.Role == executionprocess.RoleAuxiliary {
			if err := start(process); err != nil {
				if !process.Required {
					logger.Info("Optional auxiliary unavailable after start failure", "process", process.Name)
					continue
				}
				finishAuxiliaries()
				return err
			}
		}
	}
	for _, process := range config.Processes {
		if process.Role == executionprocess.RolePrimary {
			if err := start(process); err != nil {
				finishAuxiliaries()
				return err
			}
		}
	}

	var terminalErr error
	stoppingPrimary := false
	externalStop := false
	stopRequested := ctx.Done()
	requestPrimaryStop := func(reason error, external bool) {
		if terminalErr == nil {
			terminalErr = reason
		}
		externalStop = externalStop || external
		if !stoppingPrimary {
			stoppingPrimary = true
			logger.Info("Primary stop requested", "external", external)
			terminateRole(runningByName, executionprocess.RolePrimary, syscall.SIGTERM)
		}
	}

	for {
		select {
		case <-stopRequested:
			requestPrimaryStop(ctx.Err(), true)
			stopRequested = nil
		case process := <-restartReady:
			if ctx.Err() != nil {
				requestPrimaryStop(ctx.Err(), true)
				stopRequested = nil
				continue
			}
			if stoppingPrimary {
				continue
			}
			logger.Info("Auxiliary restart started", "process", process.Name, "consecutiveFailures", restarts[process.Name])
			if err := start(process); err != nil {
				if process.Required {
					requestPrimaryStop(err, false)
				} else {
					logger.Info("Optional auxiliary unavailable after restart failure", "process", process.Name)
				}
			}
		case <-sigchld:
			if ctx.Err() != nil && !externalStop {
				requestPrimaryStop(ctx.Err(), true)
				stopRequested = nil
			}
			exits := reapAndLog()
			var primaryExit *processExit
			var batchedAuxiliaryErr error
			for i := range exits {
				if exits[i].process.Role == executionprocess.RolePrimary {
					primaryExit = &exits[i]
					continue
				}
				if !exits[i].process.Required {
					continue
				}
				if exits[i].code == 0 || exits[i].process.RestartPolicy != executionprocess.RestartOnFailure {
					batchedAuxiliaryErr = fmt.Errorf("required process %q exited with code %d", exits[i].process.Name, exits[i].code)
				} else if nextAuxiliaryRestartCount(restarts[exits[i].process.Name], exits[i].runtime) > maxAuxiliaryRestarts {
					batchedAuxiliaryErr = fmt.Errorf("required process %q exceeded restart limit", exits[i].process.Name)
				}
			}
			if primaryExit != nil {
				finishAuxiliaries()
				if externalStop {
					return terminalErr
				}
				if terminalErr != nil {
					return terminalErr
				}
				if batchedAuxiliaryErr != nil {
					return batchedAuxiliaryErr
				}
				if primaryExit.code == 0 {
					return nil
				}
				return &ExitError{Process: primaryExit.process.Name, Code: primaryExit.code}
			}
			for _, exited := range exits {
				if stoppingPrimary {
					continue
				}
				if exited.code == 0 || exited.process.RestartPolicy != executionprocess.RestartOnFailure {
					if exited.process.Required {
						requestPrimaryStop(fmt.Errorf("required process %q exited with code %d", exited.process.Name, exited.code), false)
					}
					continue
				}
				previousRestarts := restarts[exited.process.Name]
				restarts[exited.process.Name] = nextAuxiliaryRestartCount(previousRestarts, exited.runtime)
				if previousRestarts > 0 && restarts[exited.process.Name] == 1 {
					logger.Info("Auxiliary failure sequence reset", "process", exited.process.Name, "runtime", exited.runtime.Round(time.Millisecond))
				}
				if restarts[exited.process.Name] > maxAuxiliaryRestarts {
					logger.Info("Auxiliary restart budget exhausted", "process", exited.process.Name, "consecutiveFailures", restarts[exited.process.Name])
					if exited.process.Required {
						requestPrimaryStop(fmt.Errorf("required process %q exceeded restart limit", exited.process.Name), false)
					}
					continue
				}
				delay := time.Duration(restarts[exited.process.Name]) * restartBackoff
				logger.Info("Auxiliary restart scheduled", "process", exited.process.Name, "consecutiveFailures", restarts[exited.process.Name], "delay", delay)
				go func(process ProcessConfig) {
					timer := time.NewTimer(delay)
					defer timer.Stop()
					select {
					case <-reaperDone:
					case <-timer.C:
						select {
						case restartReady <- process:
						case <-reaperDone:
						}
					}
				}(exited.process)
			}
		}
	}
}

func reap(byName map[string]*runningProcess, byPID map[int]*runningProcess) []processExit {
	var exits []processExit
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid <= 0 || err != nil {
			return exits
		}
		running, managed := byPID[pid]
		if !managed {
			continue
		}
		delete(byPID, pid)
		delete(byName, running.config.Name)
		code := status.ExitStatus()
		if status.Signaled() {
			code = 128 + int(status.Signal())
		}
		exits = append(exits, processExit{process: running.config, code: code, runtime: time.Since(running.started)})
	}
}

// nextAuxiliaryRestartCount counts consecutive unstable failures. A process
// that stayed up for the stable-run window starts a new failure sequence, so
// occasional failures do not consume a lifetime restart budget.
func nextAuxiliaryRestartCount(previous int, runtime time.Duration) int {
	if runtime >= auxiliaryStableRun {
		return 1
	}
	return previous + 1
}

func hasRole(running map[string]*runningProcess, role executionprocess.Role) bool {
	return countRole(running, role) > 0
}

func countRole(running map[string]*runningProcess, role executionprocess.Role) int {
	count := 0
	for _, process := range running {
		if process.config.Role == role {
			count++
		}
	}
	return count
}

func waitForRoleExit(running map[string]*runningProcess, role executionprocess.Role, timeout, interval time.Duration, reapFn func()) bool {
	deadline := time.Now().Add(timeout)
	for hasRole(running, role) && time.Now().Before(deadline) {
		reapFn()
		if hasRole(running, role) {
			time.Sleep(interval)
		}
	}
	return !hasRole(running, role)
}

func terminateRole(running map[string]*runningProcess, role executionprocess.Role, signal syscall.Signal) {
	for _, process := range running {
		if process.config.Role == role {
			_ = syscall.Kill(-process.process.Pid, signal)
		}
	}
}
