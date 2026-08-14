package rlarkadm

import (
	"fmt"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/docker"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/raw"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Uninstaller uninstalls RLark components.
type Uninstaller interface {
	Uninstall(cfg *types.DeployConfig, purge bool) error
}

// Uninstall uninstalls the components.
func Uninstall(cfg *types.DeployConfig, purge bool, skipConfirm bool) error {
	logger := log.GetLogger()

	uninstaller, err := newUninstaller(cfg)
	if err != nil {
		return err
	}

	summary := buildUninstallSummary(cfg, purge)
	fmt.Print(summary)
	if !skipConfirm {
		fmt.Print("\nDo you want to continue? [y/N]: ")
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if answer != "y" && answer != "Y" && answer != "yes" {
			logger.Info("uninstall cancelled")
			return nil
		}
	}

	logger.Info("uninstalling plane", "plane", cfg.Plane, "mode", cfg.EnvMode(), "purge", purge)

	if err := uninstaller.Uninstall(cfg, purge); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	logger.Info("plane uninstalled successfully", "plane", cfg.Plane)
	return nil
}

func buildUninstallSummary(cfg *types.DeployConfig, purge bool) string {
	var b strings.Builder
	comps := component.ComponentsForPlane(cfg)

	fmt.Fprintf(&b, "\nYou are about to uninstall the %s plane (%s mode).\n", cfg.Plane, cfg.EnvMode())
	fmt.Fprintf(&b, "Namespace: %s\n\n", constants.Namespace)
	fmt.Fprintf(&b, "The following resources will be deleted:\n")

	for _, c := range comps {
		fmt.Fprintf(&b, "  - %s (%s", c.Name, c.WorkloadKind)
		if c.WorkloadKind == "" {
			fmt.Fprintf(&b, "Deployment")
		}
		fmt.Fprintf(&b, ")")
		if c.NeedsService {
			fmt.Fprintf(&b, " + Service")
		}
		if len(c.RBACRules) > 0 {
			fmt.Fprintf(&b, " + RBAC")
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "  - ConfigMaps: rlark-db-config, rlark-postgres-init, rlark-kcp-kubeconfig\n")
	fmt.Fprintf(&b, "  - Secrets: rlark-tls, rlark-agent-cert\n")

	if purge {
		fmt.Fprintf(&b, "\n  --purge: Namespace %s will also be deleted!\n", constants.Namespace)
	}

	return b.String()
}

func newUninstaller(cfg *types.DeployConfig) (Uninstaller, error) {
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
