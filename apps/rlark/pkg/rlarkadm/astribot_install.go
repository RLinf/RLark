package rlarkadm

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/deployer/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// InstallTo installs a data plane's components into the Kubernetes cluster
// restConfig reaches, without a kubeconfig file: the Gateway installs the agent
// of a cluster it made this way.
func InstallTo(ctx context.Context, restConfig *rest.Config, cfg *types.DeployConfig) error {
	bundle, err := cert.GenerateBundle(cfg.Cert)
	if err != nil {
		return fmt.Errorf("generate certs: %w", err)
	}
	return (&kubernetes.Installer{}).InstallTo(ctx, restConfig, cfg, bundle)
}
