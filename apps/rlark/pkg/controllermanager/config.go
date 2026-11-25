package controllermanager

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/task"
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

	// OrphanSweep schedules the periodic backstop that reclaims children
	// whose parent is gone. It is a safety net behind parent-driven cleanup,
	// not the primary path, so its period is measured in minutes.
	OrphanSweep sweep.Options

	// ArchiveTTL 是终态资源在 kcp 保留的时长，超过后归档删除。
	// 零值使用 archive.DefaultArchiveTTL（5 分钟）。
	ArchiveTTL time.Duration

	// NodeHeartbeatTimeout is the maximum age of a node heartbeat Lease renewal.
	NodeHeartbeatTimeout time.Duration
	// NodeHeartbeatGracePeriod keeps a recently expired node connected to avoid flapping.
	NodeHeartbeatGracePeriod time.Duration

	// EnableSecretBackend 启用 Secret Backend（测试/CI 模式）。
	// 未配置 PostgreSQL 时，用 K8s Secret 存归档快照。
	EnableSecretBackend bool

	// TaskFreshness 配置 Task pullProgress/events 聚合的兜底重扫周期与分页大小。
	// Task 控制器的快路径是 Node 上报事件；Node watch 事件丢失时，
	// 该周期扫描保证聚合状态的滞后有确定上界。
	// Period 为零表示只依赖 watch 事件（不推荐，仅用于排障）。
	TaskFreshness task.ResyncOptions
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

		ControllerConcurrency:    defaultControllerConcurrencyConfig(),
		OrphanSweep:              sweep.DefaultOptions(),
		ArchiveTTL:               0, // 零值 = 使用 archive.DefaultArchiveTTL
		NodeHeartbeatTimeout:     45 * time.Second,
		NodeHeartbeatGracePeriod: 15 * time.Second,
		EnableSecretBackend:      false,
		TaskFreshness:            task.DefaultResyncOptions(),
	}
}

// SetupFlags sets the upFlags.
func (c Config) Validate() error {
	if err := c.ControllerConcurrency.Validate(); err != nil {
		return err
	}
	if c.NodeHeartbeatTimeout <= 0 {
		return fmt.Errorf("node heartbeat timeout must be positive")
	}
	if c.NodeHeartbeatGracePeriod < 0 {
		return fmt.Errorf("node heartbeat grace period must not be negative")
	}
	return nil
}

func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	c.KubeClientConfig.SetupFlags(fs)

	fs.StringVar(&c.ServerAddress, "server-address", c.ServerAddress, "The address for the RLark server to listen on (e.g., https://:8443)")
	fs.StringVar(&c.DBConfigPath, "db-config", c.DBConfigPath, "Path to database configuration file")
	c.LeaderElection.SetupFlags(fs, "")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", c.MetricsBindAddress, "The address the metric endpoint binds to.")
	fs.StringVar(&c.ProbeBindAddress, "health-probe-bind-address", c.ProbeBindAddress, "The address the probe endpoint binds to.")
	fs.DurationVar(&c.OrphanSweep.Period, "orphan-sweep-interval", c.OrphanSweep.Period, "Interval between orphan sweeps over DomainPeers and Tasks whose parent is gone.")
	fs.Int64Var(&c.OrphanSweep.PageSize, "orphan-sweep-page-size", c.OrphanSweep.PageSize, "Maximum number of children an orphan sweep lists per page.")
	fs.DurationVar(&c.OrphanSweep.StaleTTL, "orphan-stale-ttl", c.OrphanSweep.StaleTTL, "How long a child stays marked as orphaned before the sweeper deletes it.")
	fs.DurationVar(&c.ArchiveTTL, "archive-ttl", c.ArchiveTTL, "Time to keep terminal resources in kcp before archiving (e.g. 5m, 1h). Zero = use default (5m).")
	fs.DurationVar(&c.NodeHeartbeatTimeout, "node-heartbeat-timeout", c.NodeHeartbeatTimeout, "Maximum age of a node heartbeat Lease renewal before it is considered expired.")
	fs.DurationVar(&c.NodeHeartbeatGracePeriod, "node-heartbeat-grace-period", c.NodeHeartbeatGracePeriod, "Additional time to keep a recently expired node heartbeat connected to avoid flapping.")
	fs.BoolVar(&c.EnableSecretBackend, "enable-secret-backend", c.EnableSecretBackend, "Use Kubernetes Secret as persistence backend (test/CI mode, only when PostgreSQL is not configured).")
	fs.DurationVar(&c.TaskFreshness.Period, "task-freshness-resync-interval", c.TaskFreshness.Period, "Interval between fallback scans that re-enqueue Pending Tasks so a lost Node watch event cannot keep aggregated pullProgress/events stale (jittered 0.8x-1.2x). Zero disables the scan.")
	fs.Int64Var(&c.TaskFreshness.PageSize, "task-freshness-resync-page-size", c.TaskFreshness.PageSize, "Maximum number of Tasks listed per page during a Task freshness resync scan.")

	c.ControllerConcurrency.SetupFlags(fs)
}
