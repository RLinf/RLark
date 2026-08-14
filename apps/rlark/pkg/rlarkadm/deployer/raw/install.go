package raw

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

const (
	installDir = "/opt/rlark"
	configDir  = "/etc/rlark"
	systemdDir = "/etc/systemd/system"
)

// Installer installs RLark components.
type Installer struct {
	summary *types.InstallSummary
}

// Install installs the components.
func (d *Installer) Install(cfg *types.DeployConfig, certBundle *cert.Bundle) error {
	logger := log.GetLogger()
	certDir := filepath.Join(configDir, "certs")
	if certBundle != nil {
		if err := certBundle.WriteToDir(certDir); err != nil {
			return err
		}
	}

	for _, dir := range []string{installDir, configDir, systemdDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	if cfg.DB != nil {
		if err := writeDBConfigFile(cfg.DB); err != nil {
			return err
		}
	}

	comps := component.ComponentsForPlane(cfg)
	binPaths := make([]string, 0, len(comps))

	for _, c := range comps {
		// 如果组件已存在且健康，跳过部署
		if c.HealthCheckFn != nil && c.HealthCheckFn(cfg) == nil {
			logger.Info("component already healthy, skipping", "name", c.Name)
			continue
		}

		binPath, err := downloadArtifact(c.ArtifactFn(cfg), c.Name)
		if err != nil {
			return err
		}
		args := c.ArgsFn(cfg)
		if err := writeSystemdUnit(c.Name, binPath, args); err != nil {
			return err
		}
		binPaths = append(binPaths, c.Name)
		logger.Info("service deployed", "name", c.Name, "port", c.Port)
	}

	if err := systemctlReloadAndEnable(binPaths...); err != nil {
		return err
	}

	for _, c := range comps {
		if err := health.WaitForHealthy(cfg, c); err != nil {
			return err
		}
	}

	logger.Info("plane deployed via systemd", "plane", cfg.Plane)

	d.summary = d.buildSummary(cfg)
	return nil
}

// Summary is an exported method.
func (d *Installer) Summary() *types.InstallSummary {
	return d.summary
}

func (d *Installer) buildSummary(cfg *types.DeployConfig) *types.InstallSummary {
	summary := &types.InstallSummary{
		Plane: string(cfg.Plane),
		Mode:  cfg.EnvMode(),
	}
	for _, c := range component.ComponentsForPlane(cfg) {
		healthy := false
		if c.HealthCheckFn != nil {
			healthy = c.HealthCheckFn(cfg) == nil
		}
		summary.Components = append(summary.Components, types.ComponentStatus{
			Name:    c.Name,
			Healthy: healthy,
			Port:    c.Port,
			Address: fmt.Sprintf("http://localhost:%d", c.Port),
		})
	}
	if cfg.Plane == types.PlaneData {
		summary.ControlPlaneAddress = cfg.ControlPlaneAddress
	}
	return summary
}

func downloadArtifact(url, binName string) (string, error) {
	logger := log.GetLogger()
	dest := filepath.Join(installDir, binName)
	logger.Info("downloading binary", "name", binName, "url", url)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", binName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", binName, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", dest, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}
	if err := os.Chmod(dest, 0755); err != nil {
		return "", fmt.Errorf("chmod %s: %w", dest, err)
	}
	return dest, nil
}

func writeSystemdUnit(name, binPath string, args []string) error {
	execStart := binPath
	if len(args) > 0 {
		execStart += " " + strings.Join(args, " ")
	}

	unit := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, name, execStart)

	path := filepath.Join(systemdDir, name+".service")
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return fmt.Errorf("write systemd unit %s: %w", path, err)
	}
	return nil
}

func systemctlReloadAndEnable(units ...string) error {
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	for _, u := range units {
		if err := exec.Command("systemctl", "enable", "--now", u).Run(); err != nil {
			return fmt.Errorf("systemctl enable --now %s: %w", u, err)
		}
	}
	return nil
}

func writeDBConfigFile(cfg *types.DBConfig) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	yamlData, err := component.DBConfigYAML(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(constants.DBConfigPath, yamlData, 0644); err != nil {
		return fmt.Errorf("write db config file: %w", err)
	}
	return nil
}
