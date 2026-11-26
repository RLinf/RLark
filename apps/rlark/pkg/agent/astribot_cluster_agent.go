package agent

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
	"github.com/rlinf/rlark/apps/rlark/astribot/resident"
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
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

// astribotManagementOptions makes the management manager's options the site's. The agent serves its metrics and
// probes itself, the same in every mode, so the manager serves none. And the agent may read only its own Cluster
// (named as its namespace); a list of every Cluster would be refused.
func astribotManagementOptions(clusterID string, options ctrl.Options) ctrl.Options {
	options.Metrics = metricsserver.Options{BindAddress: "0"}
	options.Cache.ByObject = map[client.Object]cache.ByObject{
		&rlarkv1alpha1.Cluster{}: {Field: fields.OneTermEqualSelector("metadata.name", clusterID)},
	}
	return options
}

// prepareAstribot installs site controllers and prepares supervised manager startup.
//
// The agent is ready once the first watches of both sides have synced. The
// watches the controllers hold are asked for here, before the managers
// start, so that "every informer has synced" cannot be true while a
// controller has yet to ask for its own. A watch that does not sync in time
// stops its manager, and with it the agent.
func (c *clusterAgent) prepareAstribot(ctx context.Context, mm, localManager ctrl.Manager, lm interface {
	Start(ctx context.Context) error
	Add(manager.Runnable) error
}, mclient, local client.Client, clusterID, agentType string) (func(context.Context) error, error) {
	if err := c.setupAstribotControllers(mm, localManager, lm, mclient, local, clusterID, agentType); err != nil {
		return nil, err
	}
	for _, object := range []client.Object{&rlarkv1alpha1.Cluster{}, &rlarkv1alpha1.Task{}, &rlarkv1alpha1.Node{}, &rlarkv1alpha1.Pod{}, &rlarkv1alpha1.Addon{}} {
		if _, err := mm.GetCache().GetInformer(ctx, object, cache.BlockUntilSynced(false)); err != nil {
			return nil, fmt.Errorf("watch %T of the management plane: %w", object, err)
		}
	}
	for _, object := range []client.Object{&corev1.Node{}, &corev1.Pod{}, &appsv1.DaemonSet{}} {
		if _, err := localManager.GetCache().GetInformer(ctx, object, cache.BlockUntilSynced(false)); err != nil {
			return nil, fmt.Errorf("watch %T of the cluster: %w", object, err)
		}
	}
	return func(ctx context.Context) error {
		return startTogether(ctx, mm.Start, lm.Start, func(ctx context.Context) error {
			if mm.GetCache().WaitForCacheSync(ctx) && localManager.GetCache().WaitForCacheSync(ctx) {
				log.FromContext(ctx).WithName("clusterAgent").Info("the first watches of the management plane and of the cluster have synced; the agent is ready")
				c.a.ready.set(partClusterWatches, true)
			}
			<-ctx.Done()
			return nil
		})
	}, nil
}
