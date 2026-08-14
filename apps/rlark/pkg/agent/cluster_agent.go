package agent

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/addon"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/pod"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// clusterAgent manages cluster-level operations (controller manager).
type clusterAgent struct {
	a *Agent
}

// Run runs the component.
func (c *clusterAgent) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("clusterAgent")
	ctrl.SetLogger(logger)

	agentType := c.a.config.AgentType
	clusterID := c.a.config.ClientConfig.ServerNamespace
	logger.Info(fmt.Sprintf("agentType=%s, clusterID=%s", agentType, clusterID))
	if clusterID == "" {
		return fmt.Errorf("cluster ID is empty")
	}

	mm, err := ctrl.NewManager(c.a.managementConfig, ctrl.Options{
		Scheme:  controllers.MgmtScheme,
		Metrics: metricsserver.Options{BindAddress: c.a.config.MetricsBindAddress},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				clusterID: {},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create management manager: %w", err)
	}
	mclient, err := client.New(c.a.managementConfig, client.Options{Scheme: controllers.MgmtScheme})
	if err != nil {
		return fmt.Errorf("create management direct client: %w", err)
	}

	bc := base.Controller{
		ManagementClient:    mclient,
		ManagementNamespace: clusterID,
		AgentType:           agentType,
		NetworkSidecarImage: c.a.config.NetworkSidecarImage,
	}

	var lm interface {
		Start(ctx context.Context) error
	}

	switch rlarkv1alpha1.AgentType(agentType) {
	case rlarkv1alpha1.AgentTypeKubernetes:
		if c.a.localKubeConfig == nil {
			return fmt.Errorf("local kube config is required for kubernetes agent")
		}
		m, err := ctrl.NewManager(c.a.localKubeConfig, ctrl.Options{
			Scheme:  controllers.MgmtScheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		if err != nil {
			return fmt.Errorf("create local manager: %w", err)
		}
		lclient, err := client.New(c.a.localKubeConfig, client.Options{Scheme: controllers.MgmtScheme})
		if err != nil {
			return fmt.Errorf("create local direct client: %w", err)
		}
		bc.LocalKubeClient = lclient
		lm = m

	case rlarkv1alpha1.AgentTypeDocker:
		// TODO: initialize Docker controller manager
		return fmt.Errorf("docker agent is not implemented yet")

	case rlarkv1alpha1.AgentTypeRaw:
		// TODO: initialize Raw controller manager
		return fmt.Errorf("raw agent is not implemented yet")

	default:
		return fmt.Errorf("unknown AgentType: %s", agentType)
	}

	// Setup Task controllers
	tc := task.NewTaskController(bc)
	if err := tc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup task pull controller: %w", err)
	}
	if err := tc.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup task push controller: %w", err)
	}

	// Setup Node controllers
	nc := node.NewNodeController(bc)
	if err := nc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup node pull controller: %w", err)
	}
	if err := nc.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup node push controller: %w", err)
	}

	// Setup Pod controllers (push-only: reports local K8s Pods to management Pod CRs)
	pc := pod.NewPodController(bc)
	if err := pc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup pod pull controller: %w", err)
	}
	if err := pc.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup pod push controller: %w", err)
	}

	// Setup Addon controllers (pull-only: watches management Addon CRs and deploys to local cluster)
	ac := addon.NewAddonController(bc)
	if err := ac.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup addon pull controller: %w", err)
	}
	if err := ac.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup addon push controller: %w", err)
	}

	var eg errgroup.Group
	eg.Go(func() error { return mm.Start(ctx) })
	eg.Go(func() error { return lm.Start(ctx) })
	return eg.Wait()
}
