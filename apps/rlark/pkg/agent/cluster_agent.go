package agent

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/addon"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/clusterupgradejob"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/delivery"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/pod"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// clusterAgent manages cluster-level operations (controller manager).
type clusterAgent struct {
	a     *Agent
	start func(context.Context) error
}

// Build installs the complete cluster role without starting managers or watches.
func (c *clusterAgent) Build(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("clusterAgent")
	ctrl.SetLogger(logger)

	agentType := string(c.a.config.RuntimeKind())
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
		Image:               c.a.config.Image,
	}

	var lm interface {
		Start(ctx context.Context) error
		Add(manager.Runnable) error
	}
	var localManager ctrl.Manager

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
		transport, err := rest.TransportFor(c.a.localKubeConfig)
		if err != nil {
			return fmt.Errorf("create local Kubernetes HTTP transport: %w", err)
		}
		bc.LocalKubeClient = lclient
		bc.LocalKubeCachedClient = m.GetClient()
		bc.LocalKubeHTTP = &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		}
		bc.LocalKubeAPIHost = c.a.localKubeConfig.Host
		bc.LocalKubeConfig = c.a.localKubeConfig
		lm = m
		localManager = m

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
	taskAssembly, err := task.BuildKubernetesTaskAssembly(ctx, task.KubernetesTaskAssemblyInput{
		ManagementClient: mclient, ManagementNamespace: clusterID,
		LocalClient: bc.LocalKubeClient, LocalCachedClient: bc.LocalKubeCachedClient,
		LocalHTTP: bc.LocalKubeHTTP, LocalAPIHost: bc.LocalKubeAPIHost, LocalConfig: bc.LocalKubeConfig,
		Mapper: localManager.GetRESTMapper(), Execution: c.a.config.Execution,
		PullWorkers: c.a.config.ControllerConcurrency.TaskPull, PushWorkers: c.a.config.ControllerConcurrency.TaskRuntimePush,
		ResyncPeriod: c.a.config.TaskRuntimeResyncPeriod,
	})
	if err != nil {
		return fmt.Errorf("build Task execution assembly: %w", err)
	}
	if err := taskAssembly.Setup(mm, localManager); err != nil {
		return fmt.Errorf("install Task execution assembly: %w", err)
	}

	// Setup Node controllers
	nodeController := bc
	nodeController.PushMaxConcurrentReconciles = c.a.config.ControllerConcurrency.NodePush
	nc := node.NewNodeController(nodeController)
	if err := lm.Add(node.NewOfflineSweeper(nc, 0)); err != nil {
		return fmt.Errorf("setup node offline sweeper: %w", err)
	}
	if err := node.IndexLocalFields(ctx, localManager); err != nil {
		return fmt.Errorf("index local fields for node controller: %w", err)
	}
	if err := nc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup node pull controller: %w", err)
	}
	if err := nc.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup node push controller: %w", err)
	}

	// Setup Pod controllers (push-only: reports local K8s Pods to management Pod CRs)
	podController := bc
	podController.PushMaxConcurrentReconciles = c.a.config.ControllerConcurrency.PodPush
	pc := pod.NewPodController(podController)
	if err := lm.Add(pod.NewOrphanSweeper(pc, c.a.config.PodOrphanSweepInterval, c.a.config.PodOrphanSweepPageSize, c.a.config.PodStaleTTL)); err != nil {
		return fmt.Errorf("setup pod orphan sweeper: %w", err)
	}
	if err := pc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup pod pull controller: %w", err)
	}
	if err := pc.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup pod push controller: %w", err)
	}

	// Setup Addon controllers (pull-only: watches management Addon CRs and deploys to local cluster)
	addonController := bc
	addonController.PullMaxConcurrentReconciles = c.a.config.ControllerConcurrency.AddonPull
	ac := addon.NewAddonController(addonController)
	if err := ac.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup addon pull controller: %w", err)
	}
	if err := ac.SetupPushController(lm); err != nil {
		return fmt.Errorf("setup addon push controller: %w", err)
	}

	// Setup ClusterUpgradeJob controller (pull-only: watches management ClusterUpgradeJob CRs and triggers upgrade on local cluster)
	clusterUpgradeJobController := bc
	clusterUpgradeJobController.PullMaxConcurrentReconciles = 1
	cujc := clusterupgradejob.NewClusterUpgradeJobController(clusterUpgradeJobController)
	if err := cujc.SetupPullController(mm); err != nil {
		return fmt.Errorf("setup cluster upgrade job pull controller: %w", err)
	}

	deliveryReconciler, err := delivery.New(c.a.localKubeConfig, mclient, bc.LocalKubeClient, clusterID)
	if err != nil {
		return fmt.Errorf("create delivery controller: %w", err)
	}
	if err := deliveryReconciler.Setup(mm, localManager); err != nil {
		return fmt.Errorf("setup delivery controller: %w", err)
	}

	c.start = func(ctx context.Context) error {
		eg, runCtx := errgroup.WithContext(ctx)
		eg.Go(func() error { return mm.Start(runCtx) })
		eg.Go(func() error { return lm.Start(runCtx) })
		return eg.Wait()
	}
	return nil
}

// Run starts an already built cluster role.
func (c *clusterAgent) Run(ctx context.Context) error {
	if c.start == nil {
		return fmt.Errorf("cluster role must be built before starting")
	}
	return c.start(ctx)
}
