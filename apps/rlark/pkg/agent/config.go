package agent

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/nodeserver"
	"github.com/rlinf/rlark/apps/rlark/pkg/server"
)

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

	LeaderElection    bool
	LeaderElectionKey string // namespace/name
	LeaderElectionID  string // unique identifier for this agent instance, usually hostname

	MetricsBindAddress string

	NodeServerConfig         nodeserver.Config
	Image                    string
	RLarkServerSSHAddress    string
	RLarkServerSSHHostKey    string
	EnableSameClusterDirect  bool
	EnableCrossClusterDirect bool
	KubeletDir               string

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
		ClientConfig:       server.DefaultClientConfig(),
		KubeClientConfig:   configs.DefaultKubernetesClientConfig(),
		AgentType:          "Kubernetes",
		Mode:               "cluster",
		LeaderElectionKey:  "default/rlark-agent",
		LeaderElectionID:   fmt.Sprintf("%s-%d", common.Hostname("node"), os.Getpid()),
		MetricsBindAddress: ":8081",
		NodeServerConfig:   nodeserver.DefaultConfig(),

		EnableSameClusterDirect:  true,
		EnableCrossClusterDirect: true,
		KubeletDir:               "",

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
	fs.BoolVar(&c.LeaderElection, "leader-election", c.LeaderElection, "enable leader election for agent")
	fs.StringVar(&c.LeaderElectionKey, "leader-election-key", c.LeaderElectionKey, "leader election key (namespace/name)")
	fs.StringVar(&c.LeaderElectionID, "leader-election-id", c.LeaderElectionID, "leader election id (unique identifier for this agent instance)")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", c.MetricsBindAddress, "The address the metric endpoint binds to.")

	fs.StringVar(&c.RLarkServerSSHAddress, "rlark-server-ssh-address", c.RLarkServerSSHAddress, "RLark server SSH address (user@host:port)")
	fs.StringVar(&c.RLarkServerSSHHostKey, "rlark-server-ssh-host-key", c.RLarkServerSSHHostKey, "RLark server SSH host key")
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
