package raw

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Uninstall uninstalls the components.
func (d *Installer) Uninstall(cfg *types.DeployConfig, purge bool) error {
	logger := log.GetLogger()
	for _, c := range component.ComponentsForPlane(cfg) {
		_ = exec.Command("systemctl", "stop", c.Name).Run()
		_ = exec.Command("systemctl", "disable", c.Name).Run()
		unitPath := filepath.Join(systemdDir, c.Name+".service")
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove unit %s: %w", unitPath, err)
		}
		binPath := filepath.Join(installDir, c.Name)
		if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove binary %s: %w", binPath, err)
		}
		logger.Info("service removed", "name", c.Name)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	if purge {
		if err := os.RemoveAll(installDir); err != nil {
			return fmt.Errorf("remove install dir: %w", err)
		}
		if err := os.RemoveAll(configDir); err != nil {
			return fmt.Errorf("remove config dir: %w", err)
		}
		logger.Info("purge complete")
	}

	logger.Info("plane uninstalled via systemd", "plane", cfg.Plane)
	return nil
}
