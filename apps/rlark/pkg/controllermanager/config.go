package controllermanager

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

const defaultMaxConcurrentReconciles = 8

// ControllerConcurrencyConfig configures worker concurrency for each core controller.
type ControllerConcurrencyConfig struct {
	Job          int
	Task         int
	Workflow     int
	Node         int
	Domain       int
	Cluster      int
	JobSync      int
	TaskSync     int
	WorkflowSync int
	NodeSync     int
	Archive      int
}

func defaultControllerConcurrencyConfig() ControllerConcurrencyConfig {
	return ControllerConcurrencyConfig{
		Job:          defaultMaxConcurrentReconciles,
		Task:         defaultMaxConcurrentReconciles,
		Workflow:     defaultMaxConcurrentReconciles,
		Node:         defaultMaxConcurrentReconciles,
		Domain:       defaultMaxConcurrentReconciles,
		Cluster:      defaultMaxConcurrentReconciles,
		JobSync:      defaultMaxConcurrentReconciles,
		TaskSync:     defaultMaxConcurrentReconciles,
		WorkflowSync: defaultMaxConcurrentReconciles,
		NodeSync:     defaultMaxConcurrentReconciles,
		Archive:      defaultMaxConcurrentReconciles,
	}
}

func (c *ControllerConcurrencyConfig) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Job, "job-controller-workers", c.Job, "Maximum concurrent Job reconciles")
	fs.IntVar(&c.Task, "task-controller-workers", c.Task, "Maximum concurrent Task reconciles")
	fs.IntVar(&c.Workflow, "workflow-controller-workers", c.Workflow, "Maximum concurrent Workflow reconciles")
	fs.IntVar(&c.Node, "node-controller-workers", c.Node, "Maximum concurrent Node reconciles")
	fs.IntVar(&c.Domain, "domain-controller-workers", c.Domain, "Maximum concurrent Domain reconciles")
	fs.IntVar(&c.Cluster, "cluster-controller-workers", c.Cluster, "Maximum concurrent Cluster reconciles")
	fs.IntVar(&c.JobSync, "job-sync-controller-workers", c.JobSync, "Maximum concurrent Job sync reconciles")
	fs.IntVar(&c.TaskSync, "task-sync-controller-workers", c.TaskSync, "Maximum concurrent Task sync reconciles")
	fs.IntVar(&c.WorkflowSync, "workflow-sync-controller-workers", c.WorkflowSync, "Maximum concurrent Workflow sync reconciles")
	fs.IntVar(&c.NodeSync, "node-sync-controller-workers", c.NodeSync, "Maximum concurrent Node sync reconciles")
	fs.IntVar(&c.Archive, "archive-controller-workers", c.Archive, "Maximum concurrent Archive reconciles")
}

func (c ControllerConcurrencyConfig) Validate() error {
	for name, workers := range map[string]int{
		"job": c.Job, "task": c.Task, "workflow": c.Workflow, "node": c.Node, "domain": c.Domain, "cluster": c.Cluster,
		"job-sync": c.JobSync, "task-sync": c.TaskSync, "workflow-sync": c.WorkflowSync, "node-sync": c.NodeSync,
		"archive": c.Archive,
	} {
		if workers <= 0 {
			return fmt.Errorf("%s controller workers must be positive", name)
		}
	}
	return nil
}

// Config holds configuration options.
type Config struct {
	// Kubernetes client configuration.
	KubeClientConfig configs.KubernetesClientConfig

	// Server Address
	ServerAddress string

	// DBConfigPath is the file path to the database configuration (e.g., YAML or JSON).
	DBConfigPath string

	LeaderElection configs.LeaderElectionConfig

	MetricsBindAddress string
	ProbeBindAddress   string

	ControllerConcurrency ControllerConcurrencyConfig

	// ArchiveTTL 是终态资源在 kcp 保留的时长，超过后归档删除。
	// 零值使用 archive.DefaultArchiveTTL（5 分钟）。
	ArchiveTTL time.Duration

	// EnableSecretBackend 启用 Secret Backend（测试/CI 模式）。
	// 未配置 PostgreSQL 时，用 K8s Secret 存归档快照。
	EnableSecretBackend bool
}

// DefaultConfig returns the default config.
func DefaultConfig() Config {
	return Config{
		KubeClientConfig: configs.DefaultKubernetesClientConfig(),
		ServerAddress:    "https://rlark-server.rlark-system.svc:8443",
		DBConfigPath:     "",

		LeaderElection: func() configs.LeaderElectionConfig {
			config := configs.DefaultLeaderElectionConfig()
			config.Enabled = true
			config.Key = "rlark-controller-manager"
			return config
		}(),

		MetricsBindAddress: ":8080",
		ProbeBindAddress:   ":8081",

		ControllerConcurrency: defaultControllerConcurrencyConfig(),
		ArchiveTTL:            0, // 零值 = 使用 archive.DefaultArchiveTTL
		EnableSecretBackend:   false,
	}
}

// SetupFlags sets the upFlags.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	c.KubeClientConfig.SetupFlags(fs)

	fs.StringVar(&c.ServerAddress, "server-address", c.ServerAddress, "The address for the RLark server to listen on (e.g., https://:8443)")
	fs.StringVar(&c.DBConfigPath, "db-config", c.DBConfigPath, "Path to database configuration file")
	c.LeaderElection.SetupFlags(fs, "")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", c.MetricsBindAddress, "The address the metric endpoint binds to.")
	fs.StringVar(&c.ProbeBindAddress, "health-probe-bind-address", c.ProbeBindAddress, "The address the probe endpoint binds to.")
	fs.DurationVar(&c.ArchiveTTL, "archive-ttl", c.ArchiveTTL, "Time to keep terminal resources in kcp before archiving (e.g. 5m, 1h). Zero = use default (5m).")
	fs.BoolVar(&c.EnableSecretBackend, "enable-secret-backend", c.EnableSecretBackend, "Use Kubernetes Secret as persistence backend (test/CI mode, only when PostgreSQL is not configured).")

	c.ControllerConcurrency.SetupFlags(fs)
}
