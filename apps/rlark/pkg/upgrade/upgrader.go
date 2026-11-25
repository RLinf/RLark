// Package upgrade provides the upgrade logic for rlark-upgrade.
// It wraps rlarkadm's upgrade functionality with automatic rollback.
package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Stage represents the upgrade stage
type Stage string

const (
	StagePrecheck        Stage = "precheck"
	StageUpgrade         Stage = "upgrade"
	StageDeepHealthCheck Stage = "deep-health-check"
	StageRollback        Stage = "rollback"
)

// UpgradeError represents an upgrade error with stage information
type UpgradeError struct {
	Stage   Stage
	Message string
	Err     error
}

func (e *UpgradeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Stage, e.Message)
}

// Upgrader wraps rlarkadm's upgrade functionality with enhanced health check and rollback
type Upgrader struct {
	config        *Config
	logger        logr.Logger
	healthChecker *HealthChecker

	// deployConfig is the loaded deploy config
	deployConfig *types.DeployConfig

	// rollbackConfig is the loaded rollback config
	rollbackConfig *types.DeployConfig
}

// NewUpgrader creates a new Upgrader
func NewUpgrader(config *Config) (*Upgrader, error) {
	logger := log.GetLogger()

	// Load deploy config
	deployConfig, err := rlarkadm.LoadDeployConfig(config.DeployConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load deploy config: %w", err)
	}
	if err := deployConfig.ValidateForUpgrade(); err != nil {
		return nil, fmt.Errorf("validate deploy config: %w", err)
	}

	// rlark-upgrade runs inside a Pod: the kubeconfig path in the deploy config
	// points to a host file that does not exist here. Clear it so rlarkadm
	// falls back to the in-cluster config (the Pod's ServiceAccount).
	if deployConfig.Kubernetes != nil {
		deployConfig.Kubernetes.Kubeconfig = ""
	}

	// Load rollback config
	rollbackConfig, err := rlarkadm.LoadDeployConfig(config.RollbackConfigPath)
	if err != nil {
		// Rollback config is optional, use deploy config as fallback
		logger.Info("rollback config not found, using deploy config as fallback")
		rollbackConfig = deployConfig
	} else if rollbackConfig.Kubernetes != nil {
		rollbackConfig.Kubernetes.Kubeconfig = ""
	}

	// Create health checker using the in-cluster config (no kubeconfig file)
	healthChecker, err := NewHealthChecker("", DefaultHealthThreshold())
	if err != nil {
		// Health checker is optional, don't fail if it can't be created
		logger.Info("failed to create health checker, health check will be skipped", "error", err)
		healthChecker = nil
	}

	return &Upgrader{
		config:         config,
		logger:         logger,
		healthChecker:  healthChecker,
		deployConfig:   deployConfig,
		rollbackConfig: rollbackConfig,
	}, nil
}

// Upgrade performs the upgrade with multi-stage health check
func (u *Upgrader) Upgrade(ctx context.Context) error {
	// Stage 1: Precheck (rlarkadm built-in)
	u.logger.Info("stage 1/4: precheck")
	if err := u.precheck(ctx); err != nil {
		return &UpgradeError{Stage: StagePrecheck, Message: "precheck failed", Err: err}
	}

	// Stage 2: Upgrade (rlarkadm built-in)
	u.logger.Info("stage 2/4: upgrade")
	if err := u.doUpgrade(ctx); err != nil {
		return &UpgradeError{Stage: StageUpgrade, Message: "upgrade failed", Err: err}
	}

	// Stage 3: Basic health check (rlarkadm built-in during upgrade)
	u.logger.Info("stage 3/4: basic health check (rlarkadm built-in, skipped)")

	// Stage 4: Deep health check (rlark-upgrade implemented)
	u.logger.Info("stage 4/4: deep health check")
	if err := u.deepHealthCheck(ctx); err != nil {
		return &UpgradeError{Stage: StageDeepHealthCheck, Message: "deep health check failed", Err: err}
	}

	u.logger.Info("upgrade completed successfully")
	return nil
}

// Rollback performs the rollback
func (u *Upgrader) Rollback(ctx context.Context) error {
	u.logger.Info("starting rollback")

	if err := u.doRollback(ctx); err != nil {
		return &UpgradeError{Stage: StageRollback, Message: "rollback failed", Err: err}
	}

	u.logger.Info("rollback completed successfully")
	return nil
}

// precheck performs pre-upgrade checks (rlarkadm built-in)
func (u *Upgrader) precheck(ctx context.Context) error {
	// Validation already done in NewUpgrader
	return nil
}

// doUpgrade performs the actual upgrade (rlarkadm built-in)
func (u *Upgrader) doUpgrade(ctx context.Context) error {
	return rlarkadm.Upgrade(u.deployConfig)
}

// deepHealthCheck performs deep health check with continuous monitoring.
// It returns once the cluster has been healthy for a sufficient number of
// consecutive checks, or fails after too many consecutive failures / timeout.
func (u *Upgrader) deepHealthCheck(ctx context.Context) error {
	if u.healthChecker == nil {
		u.logger.Info("health checker not available, skipping deep health check")
		return nil
	}

	// Require this many consecutive passing checks before declaring healthy.
	// With the default 10s interval this is ~30s of sustained health.
	const requiredConsecutivePasses = 3
	maxConsecutiveFailures := DefaultHealthThreshold().MaxConsecutiveFailures

	timeout := time.After(u.config.HealthCheckTimeout)
	ticker := time.NewTicker(u.config.HealthCheckInterval)
	defer ticker.Stop()

	consecutiveFailures := 0
	consecutivePasses := 0

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check cancelled: %w", ctx.Err())

		case <-timeout:
			return fmt.Errorf("health check did not reach sustained health within %v", u.config.HealthCheckTimeout)

		case <-ticker.C:
			if err := u.healthChecker.Check(ctx); err != nil {
				consecutiveFailures++
				consecutivePasses = 0
				u.logger.Info("health check failed",
					"consecutiveFailures", consecutiveFailures,
					"error", err)

				if consecutiveFailures >= maxConsecutiveFailures {
					return fmt.Errorf("too many consecutive failures: %d", consecutiveFailures)
				}
			} else {
				consecutiveFailures = 0
				consecutivePasses++
				u.logger.Info("health check passed",
					"consecutivePasses", consecutivePasses,
					"required", requiredConsecutivePasses)

				if consecutivePasses >= requiredConsecutivePasses {
					u.logger.Info("sustained health confirmed, deep health check passed")
					return nil
				}
			}
		}
	}
}

// doRollback performs the actual rollback (rlarkadm built-in)
func (u *Upgrader) doRollback(ctx context.Context) error {
	return rlarkadm.Rollback(u.rollbackConfig)
}
