package gateway

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

// Config holds the configuration for the API gateway.
type Config struct {
	// Address is the address the API gateway binds to.
	Address string

	// KubeClientConfig is the Kubernetes client configuration.
	KubeClientConfig configs.KubernetesClientConfig

	// DBConfigPath is the file path to the database configuration (e.g., YAML or JSON).
	DBConfigPath string

	// ServerAddress is the address of the rlark-server for certificate signing.
	ServerAddress string

	// JWTTokenTTL is the lifetime of a UI access token.
	JWTTokenTTL time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Address:          ":8080",
		ServerAddress:    "https://rlark-server.rlark-system.svc:8443",
		JWTTokenTTL:      8 * time.Hour,
		KubeClientConfig: configs.DefaultKubernetesClientConfig(),
	}
}

// SetupFlags registers command-line flags for the configuration.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Address, "addr", c.Address, "The address the API gateway binds to.")
	fs.StringVar(&c.DBConfigPath, "db-config", c.DBConfigPath, "The file path to the database configuration (e.g., YAML or JSON).")
	fs.StringVar(&c.ServerAddress, "server-address", c.ServerAddress, "The address of the rlark-server for certificate signing.")
	fs.DurationVar(&c.JWTTokenTTL, "jwt-token-ttl", c.JWTTokenTTL, "The lifetime of UI JWT access tokens.")

	c.KubeClientConfig.SetupFlags(fs)
}
