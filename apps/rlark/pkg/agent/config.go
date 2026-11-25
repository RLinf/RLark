package agent

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/validation"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task"
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
	TaskRuntimePush int
	NodePush        int
	PodPush         int
}

func defaultControllerConcurrencyConfig() ControllerConcurrencyConfig {
	return ControllerConcurrencyConfig{
		TaskPull:        defaultControllerMaxConcurrentReconciles,
		AddonPull:       defaultControllerMaxConcurrentReconciles,
		TaskRuntimePush: defaultControllerMaxConcurrentReconciles,
		NodePush:        defaultControllerMaxConcurrentReconciles,
		PodPush:         defaultControllerMaxConcurrentReconciles,
	}
}

func (c *ControllerConcurrencyConfig) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.TaskPull, "task-pull-controller-workers", c.TaskPull, "Maximum concurrent Task pull reconciles")
	fs.IntVar(&c.AddonPull, "addon-pull-controller-workers", c.AddonPull, "Maximum concurrent Addon pull reconciles")
	fs.IntVar(&c.TaskRuntimePush, "task-runtime-push-controller-workers", c.TaskRuntimePush, "Maximum concurrent Task Runtime push reconciles per registered workload root")
	fs.IntVar(&c.NodePush, "node-push-controller-workers", c.NodePush, "Maximum concurrent Node push reconciles")
	fs.IntVar(&c.PodPush, "pod-push-controller-workers", c.PodPush, "Maximum concurrent Pod push reconciles")
}

func (c ControllerConcurrencyConfig) Validate() error {
	for name, workers := range map[string]int{
		"task-pull":         c.TaskPull,
		"addon-pull":        c.AddonPull,
		"task-runtime-push": c.TaskRuntimePush,
		"node-push":         c.NodePush,
		"pod-push":          c.PodPush,
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

	// AgentType is canonical startup input; RuntimeKind maps it to the API identity.
	AgentType string
	// Mode selects cluster or node; NodeStartup is cluster-only composition.
	Mode                   string
	NodeStartup            string
	NodeMetricsBindAddress string
	Execution              task.KubernetesExecutionConfig

	LeaderElection configs.LeaderElectionConfig

	MetricsBindAddress      string
	ControllerConcurrency   ControllerConcurrencyConfig
	PodOrphanSweepInterval  time.Duration
	PodOrphanSweepPageSize  int64
	PodStaleTTL             time.Duration
	TaskRuntimeResyncPeriod time.Duration

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
		ClientConfig:           server.DefaultClientConfig(),
		KubeClientConfig:       configs.DefaultKubernetesClientConfig(),
		AgentType:              "kubernetes",
		Mode:                   "cluster",
		NodeStartup:            "external",
		NodeMetricsBindAddress: ":8082",
		Execution:              task.KubernetesExecutionConfigFromEnvironment(""),
		LeaderElection: func() configs.LeaderElectionConfig {
			config := configs.DefaultLeaderElectionConfig()
			config.Key = "default/rlark-agent"
			config.Identity = common.Hostname("node")
			return config
		}(),
		MetricsBindAddress:      ":8081",
		ControllerConcurrency:   defaultControllerConcurrencyConfig(),
		PodOrphanSweepInterval:  5 * time.Minute,
		PodOrphanSweepPageSize:  200,
		PodStaleTTL:             15 * time.Minute,
		TaskRuntimeResyncPeriod: 15 * time.Minute,
		NodeServerConfig:        nodeserver.DefaultConfig(),

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

	fs.StringVar(&c.AgentType, "agent-type", c.AgentType, "Agent runtime: kubernetes/docker/raw (Docker/Raw execution unsupported)")
	fs.StringVar(&c.Mode, "mode", c.Mode, "Agent role: cluster/node")
	fs.StringVar(&c.NodeStartup, "node-startup", c.NodeStartup, "Cluster node startup responsibility: external/embedded (managed is not implemented)")
	fs.StringVar(&c.NodeMetricsBindAddress, "embedded-node-metrics-bind-address", c.NodeMetricsBindAddress, "Separate metrics/health address for the embedded node role")
	fs.StringVar(&c.Execution.TensorBoardImage, "tensorboard-image", c.Execution.TensorBoardImage, "Trusted TensorBoard image for Kubernetes cluster execution")
	fs.BoolVar(&c.Execution.RestartableInitContainers, "restartable-init-containers", c.Execution.RestartableInitContainers, "Kubernetes cluster supports restartable init containers")
	c.LeaderElection.SetupFlags(fs, "")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", c.MetricsBindAddress, "The address the metric endpoint binds to.")
	c.ControllerConcurrency.SetupFlags(fs)
	fs.DurationVar(&c.PodOrphanSweepInterval, "pod-orphan-sweep-interval", c.PodOrphanSweepInterval, "Interval between management Pod orphan sweeps")
	fs.Int64Var(&c.PodOrphanSweepPageSize, "pod-orphan-sweep-page-size", c.PodOrphanSweepPageSize, "Management Pods processed per orphan sweep page")
	fs.DurationVar(&c.PodStaleTTL, "pod-stale-ttl", c.PodStaleTTL, "Time a missing management Pod remains stale before deletion")
	fs.DurationVar(&c.TaskRuntimeResyncPeriod, "task-runtime-resync-period", c.TaskRuntimeResyncPeriod, "Period between cluster-mode shared management Task Runtime repair scans; 0 disables scanning (unused in node-only mode)")

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

// RuntimeKind preserves the existing API/Provider/claimant identity independently
// from canonical lower-case startup configuration. Unknown input is not mapped.
func (c Config) RuntimeKind() rlarkv1alpha1.AgentType {
	switch strings.ToLower(c.AgentType) {
	case "kubernetes":
		return rlarkv1alpha1.AgentTypeKubernetes
	case "docker":
		return rlarkv1alpha1.AgentTypeDocker
	case "raw":
		return rlarkv1alpha1.AgentTypeRaw
	default:
		return ""
	}
}

// HasNode reports whether this process owns local node services.
func (c Config) HasNode() bool {
	return c.Mode == "node" || (c.Mode == "cluster" && c.NodeStartup == "embedded")
}

// Validate performs local checks only, before clients, listeners or workers.
// Explicit option applicability is checked by the startup input loader.
func (c Config) Validate() error {
	if c.Mode != "cluster" && c.Mode != "node" {
		return fmt.Errorf("mode must be cluster or node; legacy combined role is not supported")
	}
	if c.NodeStartup != "external" && c.NodeStartup != "embedded" {
		return fmt.Errorf("node-startup must be external or embedded; managed is not implemented")
	}
	if c.Mode == "node" && c.NodeStartup != "external" {
		return fmt.Errorf("node-startup is cluster-only")
	}
	if c.RuntimeKind() == "" {
		return fmt.Errorf("unknown agent-type")
	}
	if c.RuntimeKind() != rlarkv1alpha1.AgentTypeKubernetes {
		return fmt.Errorf("selected agent-type execution is unsupported")
	}
	u, err := url.Parse(c.ClientConfig.ServerAddress)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("server-address must be an HTTPS endpoint without credentials, query or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("server-address port must be between 1 and 65535")
		}
	}
	if c.ClientConfig.ClientCertPath == "" || c.ClientConfig.ClientKeyPath == "" {
		return fmt.Errorf("client-cert and client-key are required")
	}
	if c.KubeClientConfig.QPS <= 0 || math.IsNaN(float64(c.KubeClientConfig.QPS)) || math.IsInf(float64(c.KubeClientConfig.QPS), 0) || c.KubeClientConfig.Burst <= 0 || c.KubeClientConfig.Timeout < 0 {
		return fmt.Errorf("kubernetes QPS/burst must be finite positive values and timeout non-negative")
	}
	if c.KubeClientConfig.InCluster && (c.KubeClientConfig.KubeconfigPath != "" || c.KubeClientConfig.Master != "") {
		return fmt.Errorf("in-cluster cannot be combined with kubeconfig or master")
	}
	if c.Mode == "cluster" {
		if c.LeaderElection.Enabled {
			if err := c.LeaderElection.Validate("default"); err != nil {
				return fmt.Errorf("invalid leader election configuration")
			}
			if c.LeaderElection.Identity == "" {
				return fmt.Errorf("leader election identity is required")
			}
		}
		if strings.TrimSpace(c.Image) == "" || strings.TrimSpace(c.Execution.TensorBoardImage) == "" {
			return fmt.Errorf("kubernetes cluster requires trusted image and tensorboard-image")
		}
		if err := c.ControllerConcurrency.Validate(); err != nil {
			return err
		}
		if c.PodOrphanSweepInterval <= 0 || c.PodOrphanSweepPageSize <= 0 || c.PodStaleTTL <= 0 {
			return fmt.Errorf("pod orphan sweep interval/page size and stale TTL must be positive")
		}
		if err := validateTaskRuntimeResyncPeriod(c.Mode, c.TaskRuntimeResyncPeriod); err != nil {
			return err
		}
	}
	if c.HasNode() {
		if c.NodeName == "" {
			return fmt.Errorf("node-name is required for local node services")
		}
		if len(validation.IsDNS1123Subdomain(c.NodeName)) != 0 {
			return fmt.Errorf("node-name must be a valid Kubernetes node name")
		}
		if c.NodeServerConfig.UnixSocketAddress == "" || c.NodeServerConfig.DrainTimeout <= 0 {
			return fmt.Errorf("node server socket and positive drain timeout are required")
		}
		if c.SSHMaxConnectionsPerDomain <= 0 {
			return fmt.Errorf("SSH connections per domain must be positive")
		}
		if c.ImagePullEnabled && (c.ContainerdSocket == "" || c.ContainerdNamespace == "") {
			return fmt.Errorf("image pre-pull requires containerd socket and namespace")
		}
	}
	addresses := []string{c.MetricsBindAddress}
	if c.Mode == "cluster" && c.NodeStartup == "embedded" {
		addresses = append(addresses, c.NodeMetricsBindAddress)
	}
	for _, address := range addresses {
		if address != "0" {
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("metrics bind address must be host:port or 0")
			}
			p, err := strconv.Atoi(port)
			if err != nil || p < 0 || p > 65535 {
				return fmt.Errorf("metrics port must be between 0 and 65535")
			}
		}
	}
	if len(addresses) == 2 && addresses[0] != "0" && addresses[1] != "0" {
		_, p1, _ := net.SplitHostPort(addresses[0])
		_, p2, _ := net.SplitHostPort(addresses[1])
		port1, _ := strconv.Atoi(p1)
		port2, _ := strconv.Atoi(p2)
		// Wildcard and host aliases cannot safely promise separate listeners.
		if port1 != 0 && port1 == port2 {
			return fmt.Errorf("embedded metrics listeners must use separate ports")
		}
	}
	return nil
}
