package rlarkadm

import (
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/docker"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/raw"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Installer installs RLark components.
type Installer interface {
	Install(cfg *types.DeployConfig, certBundle *cert.Bundle) error
	Summary() *types.InstallSummary
}

// LoadDeployConfig is a re-export of types.LoadDeployConfig for external callers.
func LoadDeployConfig(path string) (*types.DeployConfig, error) {
	return types.LoadDeployConfig(path)
}

// Install installs the components.
func Install(cfg *types.DeployConfig) error {
	logger := log.GetLogger()
	logger.Info("installing plane", "plane", cfg.Plane, "mode", cfg.EnvMode())

	var certBundle *cert.Bundle
	if cfg.Cert != nil {
		logger.Info("preparing certificates")
		bundle, err := cert.GenerateBundle(cfg.Cert)
		if err != nil {
			return fmt.Errorf("generate certs: %w", err)
		}
		certBundle = bundle
		logger.Info("certificates ready")
	}

	installer, err := newInstaller(cfg)
	if err != nil {
		return err
	}

	if err := installer.Install(cfg, certBundle); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	logger.Info("plane installed successfully", "plane", cfg.Plane)

	if summary := installer.Summary(); summary != nil {
		summary.Print()
	}
	return nil
}

func newInstaller(cfg *types.DeployConfig) (Installer, error) {
	switch {
	case cfg.Kubernetes != nil:
		return &kubernetes.Installer{}, nil
	case cfg.Docker != nil:
		return &docker.Installer{}, nil
	case cfg.Raw != nil:
		return &raw.Installer{}, nil
	default:
		return nil, fmt.Errorf("no deployment mode specified")
	}
}
