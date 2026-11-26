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

	// JWTTokenTTL is the lifetime of a console login: the access token's exp
	// and its cookie's expiry. Nothing renews a token; a login lasts this long.
	JWTTokenTTL time.Duration

	// SubmissionVerificationKeyPath enables verification of Astribot IAM submitter claims.
	SubmissionVerificationKeyPath string

	// EnableSecretBackend 启用 Secret Backend（测试/CI 模式）。
	// 未配置 PostgreSQL 时，用 K8s Secret 存归档快照。
	EnableSecretBackend bool

	// SyncResources 指定哪些资源走持久化 Backend 查询（PG 或 Secret）。
	// 默认已同步的资源：jobs, tasks, workflows, nodes。
	// 未列出的资源（如 pods, domains, addons）走 KCP 查询。
	SyncResources []string

	AstribotConfig
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Address:          ":8080",
		ServerAddress:    "https://rlark-server.rlark-system.svc:8443",
		JWTTokenTTL:      8 * time.Hour,
		KubeClientConfig: configs.DefaultKubernetesClientConfig(),
		SyncResources:    []string{"jobs", "tasks", "workflows", "nodes"},
	}
}

// SetupFlags registers command-line flags for the configuration.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Address, "addr", c.Address, "The address the API gateway binds to.")
	fs.StringVar(&c.DBConfigPath, "db-config", c.DBConfigPath, "The file path to the database configuration (e.g., YAML or JSON).")
	fs.StringVar(&c.ServerAddress, "server-address", c.ServerAddress, "The address of the rlark-server for certificate signing.")
	fs.DurationVar(&c.JWTTokenTTL, "jwt-token-ttl", c.JWTTokenTTL, "How long a console login lasts: the access token's exp and its cookie's expiry (for example 8h, 168h).")
	fs.StringVar(&c.SubmissionVerificationKeyPath, "submission-verification-key", c.SubmissionVerificationKeyPath, "Optional Ed25519 public key PEM for Astribot IAM workload submission assertions.")
	fs.BoolVar(&c.EnableSecretBackend, "enable-secret-backend", c.EnableSecretBackend, "Use Kubernetes Secret as persistence backend (test/CI mode, only when PostgreSQL is not configured).")
	fs.StringSliceVar(&c.SyncResources, "sync-resources", c.SyncResources, "Resources that query from persistence backend (jobs, tasks, workflows, nodes). Others query from KCP.")

	c.KubeClientConfig.SetupFlags(fs)
	c.AstribotConfig.setupFlags(fs)
}
