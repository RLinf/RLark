// Package upgrade provides the upgrade logic for rlark-upgrade.
package upgrade

import (
	"time"
)

// Config holds the upgrade configuration
type Config struct {
	// DeployConfigPath is the path to the deploy config file
	DeployConfigPath string

	// RollbackConfigPath is the path to the rollback config file
	RollbackConfigPath string

	// AutoRollback indicates whether to automatically rollback on failure
	AutoRollback bool

	// HealthCheckTimeout is the timeout for health check
	HealthCheckTimeout time.Duration

	// HealthCheckInterval is the interval for health check
	HealthCheckInterval time.Duration

	// KubeconfigPath is the path to the kubeconfig file
	// If empty, uses in-cluster config
	KubeconfigPath string
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DeployConfigPath:    "/etc/rlark/deploy-config.yaml",
		RollbackConfigPath:  "/etc/rlark/rollback-deploy-config.yaml",
		AutoRollback:        true,
		HealthCheckTimeout:  5 * time.Minute,
		HealthCheckInterval: 10 * time.Second,
	}
}
