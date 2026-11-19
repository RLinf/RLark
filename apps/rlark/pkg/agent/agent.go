package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/server"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// Agent is the data plane agent.
type Agent struct {
	config Config

	serverClient *server.Client

	managementConfig *rest.Config
	managementClient versioned.Interface

	localKubeConfig  *rest.Config
	localKubeClient  kubernetes.Interface
	localKubeHandler http.Handler

	localListener net.Listener
	localDialer   utils.Dial
	readiness     *readiness
	drainMu       sync.Mutex
	drain         context.CancelFunc
}

// NewAgent creates a new Agent.
func NewAgent(config Config) *Agent {
	return &Agent{
		config:    config,
		readiness: newReadiness(),
	}
}

// SetReady is the node server saying whether it serves the node's workloads.
func (a *Agent) SetReady(ready bool) {
	a.readiness.set(partNodeServer, ready)
}

func (a *Agent) startDrain() {
	a.readiness.set(partNodeServer, false)
	a.drainMu.Lock()
	drain := a.drain
	a.drainMu.Unlock()
	if drain != nil {
		drain()
	}
}

func (a *Agent) init(ctx context.Context) error {
	_ = ctx
	var err error
	if err := validateTaskRuntimeResyncPeriod(a.config.Mode, a.config.TaskRuntimeResyncPeriod); err != nil {
		return err
	}
	if err := node.SetManagementOwnedLabels(a.config.ManagementNodeLabels); err != nil {
		return err
	}
	if err := a.config.ControllerConcurrency.Validate(); err != nil {
		return fmt.Errorf("validate controller concurrency: %w", err)
	}
	if a.config.PodOrphanSweepInterval <= 0 {
		return fmt.Errorf("pod orphan sweep interval must be positive")
	}
	if a.config.PodOrphanSweepPageSize <= 0 {
		return fmt.Errorf("pod orphan sweep page size must be positive")
	}
	if a.config.PodStaleTTL <= 0 {
		return fmt.Errorf("pod stale TTL must be positive")
	}
	// Initialize server client
	a.serverClient, err = server.NewClientFromConfig(a.config.ClientConfig)
	if err != nil {
		return fmt.Errorf("create server client: %w", err)
	}

	// Initialize Management Kube API client
	a.managementConfig, err = a.config.ClientConfig.BuildRestConfig()
	if err != nil {
		return fmt.Errorf("build management API config: %w", err)
	}
	a.managementClient, err = versioned.NewForConfig(a.managementConfig)
	if err != nil {
		return fmt.Errorf("create management API client: %w", err)
	}

	switch rlarkv1alpha1.AgentType(a.config.AgentType) {
	case rlarkv1alpha1.AgentTypeKubernetes:
		// Initialize Kubernetes client
		a.localKubeConfig, err = a.config.KubeClientConfig.BuildRestConfig()
		if err != nil {
			return fmt.Errorf("build Kubernetes REST config: %w", err)
		}
		a.localKubeClient, err = kubernetes.NewForConfig(a.localKubeConfig)
		if err != nil {
			return fmt.Errorf("create Kubernetes client: %w", err)
		}

		// Initialize Kubernetes Proxy
		kubeProxy, err := server.NewKubeProxy(a.localKubeConfig)
		if err != nil {
			return fmt.Errorf("create kube proxy: %w", err)
		}
		a.localKubeHandler = kubeProxy.GetHandler()

	case rlarkv1alpha1.AgentTypeDocker:
		// TODO: Initialize Docker client

	case rlarkv1alpha1.AgentTypeRaw:
		// TODO: Initialize Raw client

	default:
		return fmt.Errorf("unknown AgentType: %s", a.config.AgentType)
	}

	// Initialize local listener and dialer
	a.localListener, a.localDialer = utils.NetPipeWithBuffer(65536)

	return nil
}

func validateTaskRuntimeResyncPeriod(mode string, period time.Duration) error {
	if mode != "node" && period < 0 {
		return fmt.Errorf("task Runtime resync period must not be negative")
	}
	return nil
}

// Run runs the component.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.init(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	a.drainMu.Lock()
	a.drain = cancel
	a.drainMu.Unlock()
	defer func() {
		a.drainMu.Lock()
		a.drain = nil
		a.drainMu.Unlock()
		cancel()
	}()

	// A component that fails stops the others, so the agent exits and its Pod restarts instead of
	// running on (tunnel and lease alive, so the Node shows Online) without that component.
	eg, ctx := errgroup.WithContext(ctx)

	// Every part this mode runs is not ready until it says so.
	if a.config.Mode == "cluster" || a.config.Mode == "both" {
		a.readiness.set(partClusterWatches, false)
	}
	if a.config.Mode == "node" || a.config.Mode == "both" {
		a.readiness.set(partNodeWatches, false)
		a.readiness.set(partNodeServer, false)
	}

	// Run tunnel for all modes
	eg.Go(func() error {
		role := ""
		if a.config.Mode == "node" {
			role = "node-agent:" + common.NodeName("-")
		}
		return a.runTunnel(ctx, role)
	})

	// Run local HTTP server for all modes
	eg.Go(func() error {
		return a.runLocalHTTPServer(ctx)
	})

	// One probe server and one metrics server, whichever parts this mode runs.
	eg.Go(func() error {
		return serve(ctx, a.config.HealthProbeBindAddress, a.probeRouter())
	})
	eg.Go(func() error {
		return serve(ctx, a.config.MetricsBindAddress, a.metricsRouter())
	})

	// Run cluster agent (controller manager) based on mode
	if a.config.Mode == "cluster" || a.config.Mode == "both" {
		eg.Go(func() error {
			return (&clusterAgent{a: a}).Run(ctx)
		})
	}

	// Run node agent based on mode
	if a.config.Mode == "node" || a.config.Mode == "both" {
		eg.Go(func() error {
			return (&nodeAgent{a: a}).Run(ctx)
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("agent run error: %w", err)
	}
	return nil
}
