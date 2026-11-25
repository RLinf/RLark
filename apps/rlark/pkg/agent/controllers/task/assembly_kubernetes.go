package task

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// KubernetesTaskAssemblyInput supplies explicit, borrowed dependencies for one
// Task execution runtime. Node services and manager lifecycle remain Agent-owned.
type KubernetesTaskAssemblyInput struct {
	ManagementClient    client.Client
	ManagementNamespace string
	LocalClient         client.Client
	LocalCachedClient   client.Client
	LocalHTTP           *http.Client
	LocalAPIHost        string
	LocalConfig         *rest.Config
	Mapper              meta.RESTMapper
	Execution           KubernetesExecutionConfig
	PullWorkers         int
	PushWorkers         int
	ResyncPeriod        time.Duration
}

// KubernetesTaskAssembly owns complete Task wiring, but does not start managers.
// Its dependencies are frozen after Build; queue/evidence state remains mutable.
type KubernetesTaskAssembly struct {
	controller *Controller
	observers  *observationRunner
	resync     *TaskResync
	setupMu    sync.Mutex
	setupTried bool
}

// BuildKubernetesTaskAssembly validates and builds the complete default
// Kubernetes Task execution path. Metadata/RBAC preflight may perform bounded
// I/O, but Build never registers runnables, starts watches, or writes workloads,
// Task claims, or Status. An error returns no usable partial assembly.
func BuildKubernetesTaskAssembly(ctx context.Context, input KubernetesTaskAssemblyInput) (*KubernetesTaskAssembly, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build Kubernetes Task assembly: %w", err)
	}
	scope := KubernetesObservationScope{RuntimeID: kubernetesruntime.DefaultID, Namespace: tenant.DefaultNamespace, ManagementNamespace: input.ManagementNamespace}
	if err := scope.validate(); err != nil {
		return nil, fmt.Errorf("validate Task assembly scope: %w", err)
	}
	if input.ManagementClient == nil || input.LocalClient == nil || input.LocalConfig == nil || input.Mapper == nil {
		return nil, fmt.Errorf("task assembly requires management/local clients, local config, and REST mapper")
	}
	if input.LocalClient.Scheme() == nil {
		return nil, fmt.Errorf("task assembly requires a local Kubernetes scheme")
	}
	if input.PullWorkers < 0 || input.PushWorkers < 0 || input.ResyncPeriod < 0 {
		return nil, fmt.Errorf("task assembly workers and resync period must not be negative")
	}
	if strings.TrimSpace(input.Execution.Image) == "" {
		return nil, fmt.Errorf("task assembly requires a trusted RLark image")
	}
	if strings.TrimSpace(input.Execution.TensorBoardImage) == "" {
		return nil, fmt.Errorf("task assembly requires a trusted TensorBoard image")
	}
	// Snapshot mutable configuration; clients, Scheme, and mapper are borrowed
	// and must not be reconfigured while the Agent is running.
	localConfig := rest.CopyConfig(input.LocalConfig)
	c := NewTaskController(base.Controller{
		ManagementClient: input.ManagementClient, ManagementNamespace: scope.ManagementNamespace,
		AgentType: string(kubernetesruntime.Kind), Image: input.Execution.Image,
		LocalKubeClient: input.LocalClient, LocalKubeCachedClient: input.LocalCachedClient,
		LocalKubeHTTP: input.LocalHTTP, LocalKubeAPIHost: input.LocalAPIHost, LocalKubeConfig: localConfig,
		PullMaxConcurrentReconciles: input.PullWorkers, PushMaxConcurrentReconciles: input.PushWorkers,
	})
	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	execution, err := newKubernetesExecutionCoordinator(buildCtx, input.LocalClient, input.Execution)
	if err != nil {
		return nil, fmt.Errorf("build Task execution coordinator: %w", err)
	}
	c.WithExecutionCoordinator(execution)
	for _, registration := range execution.Workloads {
		if err := execution.ValidateCapabilities(buildCtx, coordination.RuntimeSelector{ID: kubernetesruntime.DefaultID, Kind: kubernetesruntime.Kind}, coordination.WorkloadSelector{Type: string(registration.Type), Version: registration.Version}); err != nil {
			return nil, fmt.Errorf("validate Task assembly capabilities: %w", err)
		}
	}
	observations, err := KubernetesExecutionObservations(execution)
	if err != nil {
		return nil, fmt.Errorf("load Task observation contracts: %w", err)
	}
	if err := c.WithKubernetesObservations(input.LocalClient.Scheme(), input.Mapper, observations); err != nil {
		return nil, fmt.Errorf("build Task observation registry: %w", err)
	}
	taskHost, err := NewKubernetesHost(c, execution)
	if err != nil {
		return nil, fmt.Errorf("build Task Host: %w", err)
	}
	c.WithHost(taskHost)
	if err := c.validateExecutionDependencies(); err != nil {
		return nil, fmt.Errorf("validate Task execution dependencies: %w", err)
	}
	observers, err := c.buildKubernetesObservationRunner(buildCtx, input.Mapper, scope, kubernetesruntime.BuiltinObservationProfiles())
	if err != nil {
		return nil, fmt.Errorf("build Task observation sources: %w", err)
	}
	if err := buildCtx.Err(); err != nil {
		return nil, fmt.Errorf("build Kubernetes Task assembly: %w", err)
	}
	c.observationScope = &scope
	return &KubernetesTaskAssembly{
		controller: c, observers: observers,
		resync: NewTaskResync(input.ManagementClient, scope.ManagementNamespace, input.ResyncPeriod, c),
	}, nil
}

// Setup installs the complete assembly once, before either manager starts.
// Managers cannot undo arbitrary Add/setup calls: any error is fatal to Agent
// startup, and the assembly and managers must be discarded, never started/reused.
// Manager contexts own Start/cancel/join; this assembly has no separate Stop.
func (a *KubernetesTaskAssembly) Setup(managementManager, localManager ctrl.Manager) error {
	if a == nil {
		return fmt.Errorf("task assembly is required")
	}
	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	if a.setupTried {
		return fmt.Errorf("task assembly Setup was already attempted; restart the Agent to retry")
	}
	a.setupTried = true
	if managementManager == nil || localManager == nil || a.controller == nil || a.observers == nil || a.resync == nil {
		return fmt.Errorf("complete Task assembly and management/local managers are required")
	}
	if err := a.controller.SetupPullController(managementManager); err != nil {
		return fmt.Errorf("install Task pull controller: %w", err)
	}
	if err := managementManager.Add(a.resync); err != nil {
		return fmt.Errorf("install Task resync: %w", err)
	}
	if err := localManager.Add(a.observers); err != nil {
		return fmt.Errorf("install Task observation sources: %w", err)
	}
	return nil
}
