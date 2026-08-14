package docker

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Uninstall uninstalls the components.
func (d *Installer) Uninstall(cfg *types.DeployConfig, purge bool) error {
	logger := log.GetLogger()
	for _, c := range component.ComponentsForPlane(cfg) {
		if err := exec.Command("docker", "rm", "-f", c.Name).Run(); err != nil {
			logger.V(1).Info("docker rm (ignored)", "name", c.Name, "err", err)
		} else {
			logger.Info("container removed", "name", c.Name)
		}
	}

	if err := os.Remove(constants.DBConfigPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove db config: %w", err)
	}
	if err := os.Remove("/etc/rlark/init-db.sql"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove init-db.sql: %w", err)
	}

	if purge {
		if err := os.RemoveAll(constants.CertDir); err != nil {
			return fmt.Errorf("remove cert dir: %w", err)
		}
		if err := os.RemoveAll("/etc/rlark"); err != nil {
			return fmt.Errorf("remove config dir: %w", err)
		}
		logger.Info("purge complete", "dir", "/etc/rlark")
	}

	logger.Info("plane uninstalled via Docker", "plane", cfg.Plane)
	return nil
}
