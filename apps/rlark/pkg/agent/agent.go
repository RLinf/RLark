package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
}

// NewAgent creates a new Agent.
func NewAgent(config Config) *Agent {
	return &Agent{
		config: config,
	}
}

func (a *Agent) init(ctx context.Context) error {
	var err error

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

// Run runs the component.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.init(ctx); err != nil {
		return err
	}

	var eg errgroup.Group

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
