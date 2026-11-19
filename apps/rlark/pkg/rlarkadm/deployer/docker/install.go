package docker

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Installer installs RLark components.
type Installer struct {
	summary *types.InstallSummary
}

// Install installs the components.
func (d *Installer) Install(cfg *types.DeployConfig, certBundle *cert.Bundle) error {
	logger := log.GetLogger()
	certDir := ""
	if certBundle != nil {
		certDir = constants.CertDir
		if err := certBundle.WriteToDir(certDir); err != nil {
			return err
		}
	}

	dbDir := ""
	if cfg.DB != nil {
		dbDir = "/etc/rlark"
		if err := writeDBConfigFile(cfg.DB); err != nil {
			return err
		}
		if err := writeInitDBScript(); err != nil {
			return err
		}
	}

	for _, c := range component.ComponentsForPlane(cfg) {
		// 如果组件已存在且健康，跳过部署
		if c.HealthCheckFn != nil && c.HealthCheckFn(cfg) == nil {
			logger.Info("component already healthy, skipping", "name", c.Name)
			continue
		}

		var env map[string]string
		if c.EnvFn != nil {
			env = c.EnvFn(cfg)
		}
		args := c.ArgsFn(cfg)
		var volMounts [][2]string
		if c.Name == constants.ComponentPostgresql {
			volMounts = append(volMounts, [2]string{"/etc/rlark/init-db.sql", constants.PostgresqlInitDir + "/init-db.sql"})
		}
		if err := dockerRun(c.Name, c.ImageFn(cfg), c.Port, certDir, dbDir, env, args, volMounts); err != nil {
			return err
		}
		logger.Info("container started", "name", c.Name, "port", c.Port)
		if err := health.WaitForHealthy(cfg, c); err != nil {
			return err
		}
	}

	logger.Info("plane deployed via Docker", "plane", cfg.Plane)

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

func dockerRun(name, image string, port int32, certDir, dbDir string, env map[string]string, args []string, volMounts [][2]string) error {
	logger := log.GetLogger()
	if err := exec.Command("docker", "rm", "-f", name).Run(); err != nil {
		logger.V(1).Info("removed existing container", "name", name, "err", err)
	}
	dockerArgs := []string{
		"run", "-d", "--name", name,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:%d", port, port),
	}
	if certDir != "" {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:%s:ro", certDir, constants.CertDir))
	}
	if dbDir != "" {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s/%s:%s:ro", dbDir, "db.yaml", constants.DBConfigPath))
	}
	for _, m := range volMounts {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:%s:ro", m[0], m[1]))
	}
	for k, v := range env {
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker run %s: %w", name, err)
	}
	return nil
}

func writeDBConfigFile(cfg *types.DBConfig) error {
	if err := os.MkdirAll("/etc/rlark", 0755); err != nil {
		return fmt.Errorf("create db config dir: %w", err)
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

func writeInitDBScript() error {
	if err := os.WriteFile("/etc/rlark/init-db.sql", []byte(constants.InitDBSQL), 0644); err != nil {
		return fmt.Errorf("write init-db.sql: %w", err)
	}
	return nil
}
