package agent

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
)

// astribotMetricsBindAddress is the agent's default --metrics-bind-address: a port of its own (see constants).
var astribotMetricsBindAddress = fmt.Sprintf(":%d", constants.AgentMetricsPort)

// AstribotConfig is what the site adds to the agent's Config.
type AstribotConfig struct {
	// HealthProbeBindAddress is where /livez and /readyz are served, in every mode.
	HealthProbeBindAddress string
	// ManagementNodeLabels are labels the site declares on management Nodes.
	ManagementNodeLabels []string
	// TaskDaemonSetWorkers is the concurrency of the push controller of per-node Tasks' DaemonSets.
	TaskDaemonSetWorkers int
}

func defaultAstribotConfig() AstribotConfig {
	return AstribotConfig{
		HealthProbeBindAddress: fmt.Sprintf(":%d", constants.AgentProbePort),
	}
}

// setupFlags adds the site's flags and says what the official ones mean here.
func (c *AstribotConfig) setupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.HealthProbeBindAddress, "health-probe-bind-address", c.HealthProbeBindAddress, "The address /livez and /readyz bind to.")
	fs.IntVar(&c.TaskDaemonSetWorkers, "task-daemonset-push-controller-workers", c.TaskDaemonSetWorkers, "Override Task Runtime push concurrency for DaemonSets (0 inherits)")
	fs.StringSliceVar(&c.ManagementNodeLabels, "management-node-labels", c.ManagementNodeLabels, "Node labels, besides RLark's own, that are declared on the management Node and carried to the Kubernetes Node: a key, or a prefix ending in / or - (cluster mode)")
	fs.Lookup("metrics-bind-address").Usage = "The address the metrics and pprof endpoints bind to."
	fs.Lookup("image").Usage = "Image of the tools the cluster Agent gives Task Pods (the tools init container, the network sidecar). New Pods get it; a running workload keeps the one it has until its Task changes. Node Agents run the image of the Cluster's spec.nodeAgents.image."
	fs.Lookup("containerd-socket").Usage = "containerd socket used for image pulling (Kubernetes agent type); empty takes the socket the Node records in kubeadm.alpha.kubernetes.io/cri-socket, under the host /run mounted at /host/run"
}

// apply checks the site's settings and puts the process-wide ones in place.
func (c AstribotConfig) apply() error {
	if c.TaskDaemonSetWorkers < 0 {
		return fmt.Errorf("validate controller concurrency: task-daemonset-push controller workers must not be negative")
	}
	return node.SetManagementOwnedLabels(c.ManagementNodeLabels)
}
