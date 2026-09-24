package agent

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/nodeserver"
	"github.com/rlinf/rlark/apps/rlark/pkg/server"
)

const defaultControllerMaxConcurrentReconciles = 8

// ControllerConcurrencyConfig configures worker concurrency for Agent controllers.
type ControllerConcurrencyConfig struct {
	TaskPull        int
	AddonPull       int
	TaskDeployment  int
	TaskDaemonSet   int
	TaskStatefulSet int
	NodePush        int
	PodPush         int
}

func defaultControllerConcurrencyConfig() ControllerConcurrencyConfig {
	return ControllerConcurrencyConfig{
		TaskPull:        defaultControllerMaxConcurrentReconciles,
		AddonPull:       defaultControllerMaxConcurrentReconciles,
		TaskDeployment:  defaultControllerMaxConcurrentReconciles,
		TaskDaemonSet:   defaultControllerMaxConcurrentReconciles,
		TaskStatefulSet: defaultControllerMaxConcurrentReconciles,
		NodePush:        defaultControllerMaxConcurrentReconciles,
		PodPush:         defaultControllerMaxConcurrentReconciles,
	}
}

func (c *ControllerConcurrencyConfig) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.TaskPull, "task-pull-controller-workers", c.TaskPull, "Maximum concurrent Task pull reconciles")
	fs.IntVar(&c.AddonPull, "addon-pull-controller-workers", c.AddonPull, "Maximum concurrent Addon pull reconciles")
	fs.IntVar(&c.TaskDeployment, "task-deployment-push-controller-workers", c.TaskDeployment, "Maximum concurrent Task Deployment push reconciles")
	fs.IntVar(&c.TaskDaemonSet, "task-daemonset-push-controller-workers", c.TaskDaemonSet, "Maximum concurrent Task DaemonSet push reconciles")
	fs.IntVar(&c.TaskStatefulSet, "task-statefulset-push-controller-workers", c.TaskStatefulSet, "Maximum concurrent Task StatefulSet push reconciles")
	fs.IntVar(&c.NodePush, "node-push-controller-workers", c.NodePush, "Maximum concurrent Node push reconciles")
	fs.IntVar(&c.PodPush, "pod-push-controller-workers", c.PodPush, "Maximum concurrent Pod push reconciles")
}

func (c ControllerConcurrencyConfig) Validate() error {
	for name, workers := range map[string]int{
		"task-pull":             c.TaskPull,
		"addon-pull":            c.AddonPull,
		"task-deployment-push":  c.TaskDeployment,
		"task-daemonset-push":   c.TaskDaemonSet,
		"task-statefulset-push": c.TaskStatefulSet,
		"node-push":             c.NodePush,
		"pod-push":              c.PodPush,
	} {
		if workers <= 0 {
			return fmt.Errorf("%s controller workers must be positive", name)
		}
	}
	return nil
}

// Config holds configuration options.
type Config struct {
	ClientConfig     server.ClientConfig
	KubeClientConfig configs.KubernetesClientConfig

	// AgentType is the type of agent (Kubernetes/Docker/Raw)
	AgentType string
	// Agent mode: cluster/node/both
	// In a Kubernetes cluster, use Deployment for cluster mode and DaemonSet for node mode.
	// For single-node, use both mode to start both cluster and node functionality.
	Mode string

	LeaderElection configs.LeaderElectionConfig

	MetricsBindAddress     string
	ControllerConcurrency  ControllerConcurrencyConfig
	PodOrphanSweepInterval time.Duration
	PodOrphanSweepPageSize int64
	PodStaleTTL            time.Duration

	NodeServerConfig           nodeserver.Config
	Image                      string
	RLarkServerSSHAddress      string
	RLarkServerSSHHostKey      string
	SSHMaxConnectionsPerDomain int
	EnableSameClusterDirect    bool
	EnableCrossClusterDirect   bool
	KubeletDir                 string

	// Image pre-pull (node-agent): pre-pull task images into the node's
	// container runtime (containerd for Kubernetes, docker for Docker).
	ImagePullEnabled    bool
	ContainerdSocket    string
	ContainerdNamespace string
	NodeName            string
}

// DefaultConfig returns the default config.
func DefaultConfig() Config {
	return Config{
		ClientConfig:     server.DefaultClientConfig(),
		KubeClientConfig: configs.DefaultKubernetesClientConfig(),
		AgentType:        "Kubernetes",
		Mode:             "cluster",
		LeaderElection: func() configs.LeaderElectionConfig {
			config := configs.DefaultLeaderElectionConfig()
			config.Key = "default/rlark-agent"
			config.Identity = common.Hostname("node")
			return config
		}(),
		MetricsBindAddress:     ":8081",
		ControllerConcurrency:  defaultControllerConcurrencyConfig(),
		PodOrphanSweepInterval: 5 * time.Minute,
		PodOrphanSweepPageSize: 200,
		PodStaleTTL:            15 * time.Minute,
		NodeServerConfig:       nodeserver.DefaultConfig(),

		EnableSameClusterDirect:    true,
		EnableCrossClusterDirect:   true,
		SSHMaxConnectionsPerDomain: 4,
		KubeletDir:                 "",

		ImagePullEnabled:    true,
		ContainerdSocket:    "/run/containerd/containerd.sock",
		ContainerdNamespace: "k8s.io",
		NodeName:            os.Getenv("NODE_NAME"),
	}
}

// SetupFlags sets the upFlags.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	c.ClientConfig.SetupFlags(fs)
	c.KubeClientConfig.SetupFlags(fs)
	c.NodeServerConfig.SetupFlags(fs)

	fs.StringVar(&c.AgentType, "agent-type", c.AgentType, "agent type: Kubernetes/Docker/Raw")
	fs.StringVar(&c.Mode, "mode", c.Mode, "agent mode: cluster/node/both")
	c.LeaderElection.SetupFlags(fs, "")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", c.MetricsBindAddress, "The address the metric endpoint binds to.")
	c.ControllerConcurrency.SetupFlags(fs)
	fs.DurationVar(&c.PodOrphanSweepInterval, "pod-orphan-sweep-interval", c.PodOrphanSweepInterval, "Interval between management Pod orphan sweeps")
	fs.Int64Var(&c.PodOrphanSweepPageSize, "pod-orphan-sweep-page-size", c.PodOrphanSweepPageSize, "Management Pods processed per orphan sweep page")
	fs.DurationVar(&c.PodStaleTTL, "pod-stale-ttl", c.PodStaleTTL, "Time a missing management Pod remains stale before deletion")

	fs.StringVar(&c.RLarkServerSSHAddress, "rlark-server-ssh-address", c.RLarkServerSSHAddress, "RLark server SSH address (user@host:port)")
	fs.StringVar(&c.RLarkServerSSHHostKey, "rlark-server-ssh-host-key", c.RLarkServerSSHHostKey, "RLark server SSH host key")
	fs.IntVar(&c.SSHMaxConnectionsPerDomain, "ssh-max-connections-per-domain", c.SSHMaxConnectionsPerDomain, "Maximum adaptive SSH connections per domain")
	fs.StringVar(&c.Image, "image", c.Image, "RLark container image (used for network sidecar, SSH server, etc.)")

	fs.BoolVar(&c.EnableSameClusterDirect, "enable-same-cluster-direct", c.EnableSameClusterDirect, "Enable direct access to pods in the same cluster")
	fs.BoolVar(&c.EnableCrossClusterDirect, "enable-cross-cluster-direct", c.EnableCrossClusterDirect, "Enable direct access to pods in different clusters")
	fs.StringVar(&c.KubeletDir, "kubelet-dir", c.KubeletDir, "Kubelet directory(optional, used for reading pod UID from kube-api-access projected volume)")

	// Image pre-pull (node-agent).
	fs.BoolVar(&c.ImagePullEnabled, "image-pull-enabled", c.ImagePullEnabled, "Pre-pull task images into the node container runtime (containerd/docker) when a task is dispatched")
	fs.StringVar(&c.ContainerdSocket, "containerd-socket", c.ContainerdSocket, "containerd socket address used by ctr for image pulling (Kubernetes agent type)")
	fs.StringVar(&c.ContainerdNamespace, "containerd-namespace", c.ContainerdNamespace, "containerd namespace used by ctr for image pulling (Kubernetes agent type, default k8s.io)")
	fs.StringVar(&c.NodeName, "node-name", c.NodeName, "Name of the local node, used for Kubernetes node-selector matching (defaults to $NODE_NAME)")
}
