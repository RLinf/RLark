package controllermanager

import (
	"fmt"

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
	JobSync      int
	TaskSync     int
	WorkflowSync int
	NodeSync     int
}

func defaultControllerConcurrencyConfig() ControllerConcurrencyConfig {
	return ControllerConcurrencyConfig{
		Job:          defaultMaxConcurrentReconciles,
		Task:         defaultMaxConcurrentReconciles,
		Workflow:     defaultMaxConcurrentReconciles,
		Node:         defaultMaxConcurrentReconciles,
		Domain:       defaultMaxConcurrentReconciles,
		JobSync:      defaultMaxConcurrentReconciles,
		TaskSync:     defaultMaxConcurrentReconciles,
		WorkflowSync: defaultMaxConcurrentReconciles,
		NodeSync:     defaultMaxConcurrentReconciles,
	}
}

func (c *ControllerConcurrencyConfig) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Job, "job-controller-workers", c.Job, "Maximum concurrent Job reconciles")
	fs.IntVar(&c.Task, "task-controller-workers", c.Task, "Maximum concurrent Task reconciles")
	fs.IntVar(&c.Workflow, "workflow-controller-workers", c.Workflow, "Maximum concurrent Workflow reconciles")
	fs.IntVar(&c.Node, "node-controller-workers", c.Node, "Maximum concurrent Node reconciles")
	fs.IntVar(&c.Domain, "domain-controller-workers", c.Domain, "Maximum concurrent Domain reconciles")
	fs.IntVar(&c.JobSync, "job-sync-controller-workers", c.JobSync, "Maximum concurrent Job sync reconciles")
	fs.IntVar(&c.TaskSync, "task-sync-controller-workers", c.TaskSync, "Maximum concurrent Task sync reconciles")
	fs.IntVar(&c.WorkflowSync, "workflow-sync-controller-workers", c.WorkflowSync, "Maximum concurrent Workflow sync reconciles")
	fs.IntVar(&c.NodeSync, "node-sync-controller-workers", c.NodeSync, "Maximum concurrent Node sync reconciles")
}

func (c ControllerConcurrencyConfig) Validate() error {
	for name, workers := range map[string]int{
		"job": c.Job, "task": c.Task, "workflow": c.Workflow, "node": c.Node, "domain": c.Domain,
		"job-sync": c.JobSync, "task-sync": c.TaskSync, "workflow-sync": c.WorkflowSync, "node-sync": c.NodeSync,
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

	c.ControllerConcurrency.SetupFlags(fs)
}
