package rlarkctl

import (
	"context"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/server"
)

// Variables used by the package.
var (
	ClientConfig     server.ClientConfig            = server.DefaultClientConfig()
	KubeClientConfig configs.KubernetesClientConfig = configs.DefaultKubernetesClientConfig()
)

// SetupPersistentFlags sets the upPersistentFlags.
func SetupPersistentFlags(fs *pflag.FlagSet) {
	ClientConfig.SetupFlags(fs)
	KubeClientConfig.SetupFlags(fs)
}

// NewClient creates a new Client.
func NewClient(ctx context.Context) (*server.Client, error) {
	if ClientConfig.ServerAddress != "" && ClientConfig.ClientCertPath != "" && ClientConfig.ClientKeyPath != "" {
		return server.NewClientFromConfig(ClientConfig)
	}
	return server.NewClientFromKubernetes(ctx, ClientConfig.ServerAddress, KubeClientConfig)
}
