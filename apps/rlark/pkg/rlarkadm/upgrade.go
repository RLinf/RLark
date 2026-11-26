package rlarkadm

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/upgrader"
)

// Upgrade upgrades the components to match the provided deploy config.
// It generates the same resource manifests as install, but updates existing resources.
// Note: This function does not provide rollback capability. If upgrade fails,
// the caller should handle recovery (e.g., by re-running with the old config).
func Upgrade(cfg *types.DeployConfig) error {
	logger := log.GetLogger()
	logger.Info("upgrading plane", "plane", cfg.Plane, "mode", cfg.EnvMode())

	ctx := context.Background()

	upgrader, err := upgrader.New(cfg)
	if err != nil {
		return err
	}

	// Phase 1: Pre-check
	if err := upgrader.PreCheck(ctx, cfg); err != nil {
		return fmt.Errorf("pre-check: %w", err)
	}

	// Phase 2: Pre-upgrade
	if err := upgrader.PreUpgrade(ctx, cfg); err != nil {
		return fmt.Errorf("pre-upgrade: %w", err)
	}

	// Phase 3: Upgrade
	if err := upgrader.Upgrade(ctx, cfg); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}

	// Phase 4: Post-upgrade
	if err := upgrader.PostUpgrade(ctx, cfg); err != nil {
		return fmt.Errorf("post-upgrade: %w", err)
	}

	logger.Info("plane upgraded successfully", "plane", cfg.Plane)

	if summary := upgrader.Summary(); summary != nil {
		summary.Print()
	}
	return nil
}

// Rollback rolls the components back to the provided deploy config.
// It is identical to Upgrade but skips the component health check in pre-check,
// because the component may be unhealthy (that is why the rollback is being performed).
func Rollback(cfg *types.DeployConfig) error {
	logger := log.GetLogger()
	logger.Info("rolling back plane", "plane", cfg.Plane, "mode", cfg.EnvMode())

	ctx := context.Background()

	upgrader, err := upgrader.New(cfg)
	if err != nil {
		return err
	}

	// Phase 1: Pre-check (skip health check for rollback)
	if err := upgrader.PreCheckForRollback(ctx, cfg); err != nil {
		return fmt.Errorf("pre-check: %w", err)
	}

	// Phase 2: Pre-upgrade
	if err := upgrader.PreUpgrade(ctx, cfg); err != nil {
		return fmt.Errorf("pre-upgrade: %w", err)
	}

	// Phase 3: Upgrade
	if err := upgrader.Upgrade(ctx, cfg); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}

	// Phase 4: Post-upgrade
	if err := upgrader.PostUpgrade(ctx, cfg); err != nil {
		return fmt.Errorf("post-upgrade: %w", err)
	}

	logger.Info("plane rolled back successfully", "plane", cfg.Plane)

	if summary := upgrader.Summary(); summary != nil {
		summary.Print()
	}
	return nil
}
