package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
	ready         atomic.Bool
	drainMu       sync.Mutex
	drain         context.CancelFunc
}

// NewAgent creates a new Agent.
func NewAgent(config Config) *Agent {
	return &Agent{
		config: config,
	}
}

func (a *Agent) SetReady(ready bool) {
	a.ready.Store(ready)
}

func (a *Agent) startDrain() {
	a.ready.Store(false)
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
	a.config.AgentType = strings.ToLower(a.config.AgentType)
	a.config.Execution.Image = a.config.Image
	if err := a.config.Validate(); err != nil {
		return fmt.Errorf("validate agent config: %w", err)
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

	switch a.config.RuntimeKind() {
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
	// Successful private dials must reach HTTP Accept; queued buffered peers
	// are not tracked or closed by http.Server during shutdown.
	a.localListener, a.localDialer = utils.NetPipe()

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
	defer func() { _ = a.localListener.Close() }() // Process-owned in-memory listener; no persistent state is removed.
	if err := a.localPreflight(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	a.drainMu.Lock()
	a.drain = cancel
	a.drainMu.Unlock()
	defer func() {
		a.ready.Store(false)
		a.drainMu.Lock()
		a.drain = nil
		a.drainMu.Unlock()
		cancel()
	}()

	tunnels := tunnelRoles(a.config)
	err := runWithRegistration(ctx, func(ctx context.Context, registered func()) error {
		return a.runRegistrationTunnel(ctx, tunnels[0], registered)
	}, func(ctx context.Context) ([]func(context.Context) error, error) {
		if err := a.managementPreflight(ctx); err != nil {
			return nil, err
		}
		roles, err := buildRoles(ctx, a.config, &clusterAgent{a: a}, &nodeAgent{a: a})
		if err != nil {
			return nil, err
		}
		components := append([]func(context.Context) error{a.runLocalHTTPServer}, roles...)
		for _, role := range tunnels[1:] {
			components = append(components, func(ctx context.Context) error { return a.runTunnel(ctx, role) })
		}
		return components, nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("agent run error: %w", err)
	}
	return nil
}
