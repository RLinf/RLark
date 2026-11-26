// Package upgrader provides the interface for upgrading RLark components.
package upgrader

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	k8supgrade "github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/upgrader/kubernetes"
)

// Upgrader defines the interface for upgrading RLark components.
type Upgrader interface {
	// PreCheck performs pre-upgrade checks.
	PreCheck(ctx context.Context, cfg *types.DeployConfig) error

	// PreCheckForRollback performs pre-upgrade checks for a rollback,
	// skipping the component health check because the component may be
	// unhealthy (that is why the rollback is being performed).
	PreCheckForRollback(ctx context.Context, cfg *types.DeployConfig) error

	// PreUpgrade performs pre-upgrade preparation.
	PreUpgrade(ctx context.Context, cfg *types.DeployConfig) error

	// Upgrade upgrades the components to match the provided deploy config.
	Upgrade(ctx context.Context, cfg *types.DeployConfig) error

	// PostUpgrade performs post-upgrade tasks.
	PostUpgrade(ctx context.Context, cfg *types.DeployConfig) error

	// Summary returns the upgrade summary.
	Summary() *types.UpgradeSummary
}

// New creates a new Upgrader based on the deploy config.
func New(cfg *types.DeployConfig) (Upgrader, error) {
	switch {
	case cfg.Kubernetes != nil:
		return k8supgrade.New(cfg)
	case cfg.Docker != nil:
		return newDockerUpgrader(cfg)
	case cfg.Raw != nil:
		return newRawUpgrader(cfg)
	default:
		return nil, fmt.Errorf("no deployment mode specified")
	}
}

// dockerUpgrader is a placeholder for Docker upgrader.
type dockerUpgrader struct{}

func newDockerUpgrader(cfg *types.DeployConfig) (Upgrader, error) {
	return &dockerUpgrader{}, nil
}

func (u *dockerUpgrader) PreCheck(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("docker upgrade not implemented yet")
}

func (u *dockerUpgrader) PreCheckForRollback(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("docker upgrade not implemented yet")
}

func (u *dockerUpgrader) PreUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("docker upgrade not implemented yet")
}

func (u *dockerUpgrader) Upgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("docker upgrade not implemented yet")
}

func (u *dockerUpgrader) PostUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("docker upgrade not implemented yet")
}

func (u *dockerUpgrader) Summary() *types.UpgradeSummary {
	return nil
}

// rawUpgrader is a placeholder for Raw Binary upgrader.
type rawUpgrader struct{}

func newRawUpgrader(cfg *types.DeployConfig) (Upgrader, error) {
	return &rawUpgrader{}, nil
}

func (u *rawUpgrader) PreCheck(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("raw upgrade not implemented yet")
}

func (u *rawUpgrader) PreCheckForRollback(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("raw upgrade not implemented yet")
}

func (u *rawUpgrader) PreUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("raw upgrade not implemented yet")
}

func (u *rawUpgrader) Upgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("raw upgrade not implemented yet")
}

func (u *rawUpgrader) PostUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return fmt.Errorf("raw upgrade not implemented yet")
}

func (u *rawUpgrader) Summary() *types.UpgradeSummary {
	return nil
}
