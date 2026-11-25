// rlark-upgrade is a fully autonomous upgrade tool.
// It performs upgrade with automatic health check and rollback.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/upgrade"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	// Exit codes
	ExitSuccess           = 0
	ExitUpgradeFailed     = 1
	ExitRolledBack        = 2
	ExitRollbackFailed    = 3
	ExitPrecheckFailed    = 4
	ExitHealthCheckFailed = 5
)

func main() {
	// Setup signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize logger
	log.InitLogger("info")
	logger := log.GetLogger()

	logger.Info("starting rlark-upgrade")

	// Load configuration
	config := &upgrade.Config{
		DeployConfigPath:    utils.GetEnv("DEPLOY_CONFIG", "/etc/rlark/deploy-config.yaml"),
		RollbackConfigPath:  utils.GetEnv("ROLLBACK_CONFIG", "/etc/rlark/rollback-deploy-config.yaml"),
		AutoRollback:        utils.GetEnvBool("AUTO_ROLLBACK", true),
		HealthCheckTimeout:  utils.GetEnvDuration("HEALTH_CHECK_TIMEOUT", 5*time.Minute),
		HealthCheckInterval: utils.GetEnvDuration("HEALTH_CHECK_INTERVAL", 10*time.Second),
	}

	if config.DeployConfigPath == "" {
		logger.Error(fmt.Errorf("DEPLOY_CONFIG is required"), "invalid config")
		os.Exit(ExitPrecheckFailed)
	}

	logger.Info("loaded config",
		"deployConfig", config.DeployConfigPath,
		"rollbackConfig", config.RollbackConfigPath,
		"autoRollback", config.AutoRollback,
	)

	// Create upgrader
	upgrader, err := upgrade.NewUpgrader(config)
	if err != nil {
		logger.Error(err, "create upgrader")
		os.Exit(ExitPrecheckFailed)
	}

	// Perform upgrade (includes multi-stage health check)
	err = upgrader.Upgrade(ctx)
	if err == nil {
		logger.Info("upgrade completed successfully")
		os.Exit(ExitSuccess)
	}

	logger.Error(err, "upgrade failed")

	// Handle rollback
	if !config.AutoRollback {
		logger.Info("auto-rollback disabled, exiting")
		os.Exit(ExitUpgradeFailed)
	}

	logger.Info("starting automatic rollback")

	rollbackErr := upgrader.Rollback(ctx)
	if rollbackErr != nil {
		logger.Error(rollbackErr, "rollback failed")
		os.Exit(ExitRollbackFailed)
	}

	logger.Info("rollback completed successfully")
	os.Exit(ExitRolledBack)
}
