package agent

import (
	"cmp"
	"fmt"
	"net/url"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
	"github.com/rlinf/rlark/apps/rlark/astribot/resident"
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
)

// setupAstribotControllers sets up our controllers of the cluster agent: the node Agents, the resident
// PriorityClass and the rollout of RLark's DaemonSets.
func (c *clusterAgent) setupAstribotControllers(mm, localManager ctrl.Manager, lm interface{ Add(manager.Runnable) error }, mclient, local client.Client, clusterID, agentType string) error {
	// Node Agents connect as this agent does; without its own SSH address they take the Server's
	// SSH port on the host of --server-address, as rlarkadm renders it.
	serverURL, err := url.Parse(c.a.config.ClientConfig.ServerAddress)
	if err != nil {
		return fmt.Errorf("parse server address: %w", err)
	}
	sshAddress := cmp.Or(c.a.config.RLarkServerSSHAddress, fmt.Sprintf("client@%s:%d", serverURL.Hostname(), constants.ServerSSHPort))
	nodeAgents := &nodeagent.Reconciler{
		Management:  mclient,
		ClusterName: clusterID,
		Local:       local,
		Namespace:   os.Getenv("POD_NAMESPACE"),
		PodName:     os.Getenv("POD_NAME"),
		Connection: nodeagent.Connection{
			AgentType:             agentType,
			ServerAddress:         c.a.config.ClientConfig.ServerAddress,
			ServerHostname:        c.a.config.ClientConfig.ServerHostname,
			CACert:                c.a.config.ClientConfig.CAPath,
			ClientCert:            c.a.config.ClientConfig.ClientCertPath,
			ClientKey:             c.a.config.ClientConfig.ClientKeyPath,
			InsecureSkipTLSVerify: c.a.config.ClientConfig.InsecureSkipTLSVerify,
			SSHAddress:            sshAddress,
			SSHHostKey:            c.a.config.RLarkServerSSHHostKey,
		},
	}
	if err := nodeAgents.Setup(mm, localManager); err != nil {
		return fmt.Errorf("setup node Agent controller: %w", err)
	}

	// The PriorityClass per-node resident workloads name, and the rollout of every DaemonSet RLark places.
	if err := lm.Add(&resident.PriorityClass{Local: local}); err != nil {
		return fmt.Errorf("setup resident PriorityClass: %w", err)
	}
	if err := (&rollout.Updater{Local: localManager.GetClient()}).Setup(localManager); err != nil {
		return fmt.Errorf("setup DaemonSet rollout controller: %w", err)
	}
	return nil
}
