package task

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	kubernetesenvironment "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/network"
	kubernetesssh "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/ssh"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/tensorboard"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes/supervision"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

const (
	tensorBoardImageEnv     = "RLARK_TENSORBOARD_IMAGE"
	defaultTensorBoardImage = "rlinf/tensorboard:2.15.1-rc1"
)

// workloadImplementationRegistry is the shared immutable registry view needed
// by both Host admission and Coordinator compilation. A single instance keeps
// exact Workload x Runtime selection consistent across those boundaries.
type workloadImplementationRegistry interface {
	host.ImplementationRegistry
	coordinator.WorkloadImplementationSelector
}

// NewKubernetesExecutionCoordinator constructs the production Coordinator and
// its immutable implementation registry for built-in Kubernetes Workloads.
func NewKubernetesExecutionCoordinator(kubeClient client.Client, image string) (*coordinator.Coordinator, error) {
	if kubeClient == nil {
		return nil, fmt.Errorf("local Kubernetes client is required")
	}
	// StaticProvider currently places every realizable Task in the historical
	// data-plane namespace. TODO(environment): resolve per-tenant placement as an
	// explicit environment binding when a persistent tenant provider is added.
	namespace := func(agentruntime.TaskIdentity) string { return tenant.DefaultNamespace }
	provider := kubernetesruntime.New(kubeClient, kubeClient.Scheme(), namespace)
	runtimeInfo, err := provider.Info(context.Background())
	if err != nil {
		return nil, fmt.Errorf("inspect Kubernetes Runtime Provider: %w", err)
	}
	workloads := raykubernetes.Registrations(namespace, kubernetesruntime.Kind)
	implementations, err := newHostImplementationRegistry(workloads)
	if err != nil {
		return nil, fmt.Errorf("initialize Workload implementation registry: %w", err)
	}
	networkProfile := network.Profile{Image: image}
	tensorBoardProfile := tensorboard.Profile{Image: configuredTensorBoardImage()}
	handlers, err := environment.NewRegistry([]environment.HandlerRegistration{
		{Handler: network.Handler{RuntimeKind: kubernetesruntime.Kind}},
		{Handler: kubernetesssh.Handler{RuntimeKind: kubernetesruntime.Kind}},
		{Handler: tensorboard.Handler{RuntimeKind: kubernetesruntime.Kind}},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize Environment handler registry: %w", err)
	}
	resolver, err := kubernetesenvironment.NewResolver([]kubernetesenvironment.ResolverRegistration{
		{Type: taskapi.EnvironmentTypeNetwork, Resolver: network.Resolver{RuntimeKind: kubernetesruntime.Kind, Profile: networkProfile}},
		{Type: taskapi.EnvironmentTypeSSH, Resolver: kubernetesssh.Resolver{RuntimeKind: kubernetesruntime.Kind}},
		{Type: taskapi.EnvironmentTypeTensorBoard, Resolver: tensorboard.Resolver{RuntimeKind: kubernetesruntime.Kind, Profile: tensorBoardProfile}},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize Environment resolver registry: %w", err)
	}
	baseBinder := binding.BindingPlanBinder{Handlers: handlers, Validator: binding.DesiredPlanValidator{
		RuntimeKind: kubernetesruntime.Kind,
		Objects:     provider,
	}, Workloads: supervision.Assembler{ToolsImage: image}, Lowerer: kubernetesruntime.UnitLowerer{RestartableInitContainers: os.Getenv("KUBERNETES_SUPPORT_SIDECAR_FEATURE") != "false"}}
	return &coordinator.Coordinator{
		Runtimes:  []coordinator.RuntimeRegistration{{Info: runtimeInfo, Provider: provider, Comparator: kubernetesruntime.ObjectComparator{Profiles: kubernetesruntime.BuiltinProfileRegistry()}}},
		Workloads: workloads, Implementations: implementations,
		Resolver: resolver,
		Binder:   baseBinder,
		BinderForTask: func(snapshot coordination.TaskSnapshot) (binding.PlanBinder, error) {
			keys, err := kubernetesssh.AuthorizedKeys(snapshot.LocalEnvironments)
			if err != nil {
				return nil, err
			}
			binder := baseBinder
			binder.Materials = kubernetesssh.NewMaterialProvider(snapshot.Identity, keys)
			return binder, nil
		},
	}, nil
}

func configuredTensorBoardImage() string {
	// TODO(environment): Replace this transitional environment variable with a
	// validated, typed Agent configuration injected into production assembly.
	if image := strings.TrimSpace(os.Getenv(tensorBoardImageEnv)); image != "" {
		return image
	}
	return defaultTensorBoardImage
}

// NewKubernetesHost constructs the production Task Host from an already
// validated Coordinator registry. It does not install dependencies on, or
// otherwise mutate, the Controller or Coordinator.
func NewKubernetesHost(controller *Controller, execution *coordinator.Coordinator) (*host.Host, error) {
	if controller == nil || controller.ManagementClient == nil {
		return nil, fmt.Errorf("task controller and management client are required")
	}
	if execution == nil {
		return nil, fmt.Errorf("task execution coordinator is required")
	}
	implementations, ok := execution.Implementations.(host.ImplementationRegistry)
	if !ok {
		return nil, fmt.Errorf("task execution coordinator requires the shared Host implementation registry")
	}
	selector := coordination.RuntimeSelector{ID: kubernetesruntime.DefaultID, Kind: kubernetesruntime.Kind}
	bundle := host.Bundle{
		RuntimeSelector: selector,
		Decoder:         host.TaskDecoderFunc(compatibility.DecodeTask),
		Executor:        host.CoordinatorExecutor{Coordinator: execution, Selector: selector},
		Implementations: implementations,
		Lifecycle:       kubernetesLifecycle{controller: controller},
		AfterExecution:  controller.recordVerifiedExecution,
	}
	status := host.KubernetesStatusWriter{Client: controller.ManagementClient}
	result := &host.Host{Tasks: controller.ManagementClient, Bundle: bundle, Eligibility: kubernetesEligibility{agentType: controller.AgentType}, Status: status, Claimant: controller.ManagementNamespace + "/" + controller.AgentType}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// newHostImplementationRegistry translates Coordinator registrations into the
// shared immutable registry used by both Coordinator and Host. Construction
// validates that each Adapter advertises the exact registered implementation.
func newHostImplementationRegistry(workloads []coordinator.WorkloadRegistration) (workloadImplementationRegistry, error) {
	registrations := make([]host.WorkloadRegistration, 0, len(workloads))
	for _, registration := range workloads {
		registrations = append(registrations, host.WorkloadRegistration{Key: host.ImplementationKey{WorkloadType: registration.Type, WorkloadVersion: registration.Version, RuntimeKind: registration.RuntimeKind}, Adapter: registration.Adapter})
	}
	implementations, err := host.NewImplementationRegistry(context.Background(), registrations)
	if err != nil {
		return nil, err
	}
	registry, ok := implementations.(workloadImplementationRegistry)
	if !ok {
		return nil, fmt.Errorf("workload implementation registry cannot be used by Coordinator")
	}
	return registry, nil
}
