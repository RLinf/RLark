// Package compatibility preserves the current Kubernetes Task API while the
// controller migrates to Runtime-neutral execution contracts. It is temporary:
// remove it after supported upgrades no longer require the legacy Task shape.
package compatibility

import (
	"errors"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// RuntimeKindKubernetes is the Runtime kind selected by Kubernetes Task specs.
const RuntimeKindKubernetes agentruntime.Kind = "Kubernetes"

// WorkloadVersion is the compatibility contract version for built-in Workloads.
const WorkloadVersion = "v1alpha1"

// ErrMissingKubernetesWorkload indicates that a Task has no workload kind.
var ErrMissingKubernetesWorkload = errors.New("task has no workload kind")

// DecodeTask maps the public Task API to the canonical runtime/workload-neutral
// execution snapshot consumed by the selected Runtime and Workload Adapter.
func DecodeTask(task *rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
	if task == nil {
		return coordination.TaskSnapshot{}, ErrMissingKubernetesWorkload
	}
	if task.Spec.Runtime.Kind != rlarkv1alpha1.RuntimeKindKubernetes {
		return coordination.TaskSnapshot{}, fmt.Errorf("task runtime kind %q is not Kubernetes", task.Spec.Runtime.Kind)
	}
	if task.Spec.Workload.Kind == "" {
		return coordination.TaskSnapshot{}, ErrMissingKubernetesWorkload
	}
	if task.Spec.Workload.Kind != taskapi.WorkloadTypeRay || task.Spec.Workload.Version != taskapi.WorkloadVersionRayV1 {
		return coordination.TaskSnapshot{}, fmt.Errorf("task workload %q/%q is unsupported", task.Spec.Workload.Kind, task.Spec.Workload.Version)
	}
	if task.Spec.Workload.Template == nil {
		return coordination.TaskSnapshot{}, fmt.Errorf("ray workload requires a Pod template")
	}
	template := *task.Spec.Workload.Template.DeepCopy()
	if len(task.Spec.Placement.NodeSelector) != 0 {
		if template.Spec.NodeSelector == nil {
			template.Spec.NodeSelector = make(map[string]string, len(task.Spec.Placement.NodeSelector))
		}
		for key, value := range task.Spec.Placement.NodeSelector {
			template.Spec.NodeSelector[key] = value
		}
	}
	requirements := executionplan.Requirements{}
	localEnvironments := make([]coordination.LocalEnvironmentDeclaration, 0, len(task.Spec.Environments))
	for _, configured := range task.Spec.Environments {
		if configured.Type == "" || configured.Version == "" {
			return coordination.TaskSnapshot{}, fmt.Errorf("decode Environment: type and version are required")
		}
		parameters := append([]byte(nil), configured.Parameters.Raw...)
		requirementParameters := parameters
		if configured.Type == taskapi.EnvironmentTypeSSH {
			requirementParameters = nil
		}
		requirements.Environments = append(requirements.Environments, executionplan.EnvironmentRequirement{
			Name: configured.Type, Type: configured.Type, Version: configured.Version, Parameters: requirementParameters,
			Consumers: []executionplan.ConsumerSelector{{Targets: []string{"workload-pod-template"}}},
		})
		localEnvironments = append(localEnvironments, coordination.LocalEnvironmentDeclaration{Type: configured.Type, Version: configured.Version, Parameters: apiextensionsv1.JSON{Raw: parameters}})
	}
	return coordination.TaskSnapshot{
		Identity: agentruntime.TaskIdentity{
			Logical: agentruntime.LogicalTaskID{
				Namespace: task.Namespace,
				Name:      task.Name,
			},
			ObjectUID:  string(task.UID),
			Generation: task.Generation,
		},
		RuntimeSelector: coordination.RuntimeSelector{ID: agentruntime.ID(task.Spec.Runtime.Name), Kind: RuntimeKindKubernetes},
		WorkloadSelector: coordination.WorkloadSelector{
			Type:    string(task.Spec.Workload.Kind),
			Version: task.Spec.Workload.Version,
		},
		CarrierSelector: coordination.CarrierSelector{Type: task.Spec.Workload.Carrier.Type, Version: task.Spec.Workload.Carrier.Version},
		Configuration: coordination.WorkloadConfiguration{
			Template:   template,
			Parameters: append([]byte(nil), task.Spec.Workload.Parameters.Raw...),
			Carrier: coordination.VersionedConfiguration{
				Type:       task.Spec.Workload.Carrier.Type,
				Version:    task.Spec.Workload.Carrier.Version,
				Parameters: append([]byte(nil), task.Spec.Workload.Carrier.Parameters.Raw...),
			},
		},
		Environment:       requirements,
		LocalEnvironments: localEnvironments,
		Deleting:          task.DeletionTimestamp != nil,
	}, nil
}
