package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// UnitLowerer owns Kubernetes-specific, version/capability-aware lowering of
// Adapter-declared Pod and PodTemplate units. Capabilities are immutable values
// injected during Runtime assembly; Lower performs no discovery or external I/O.
type UnitLowerer struct {
	RestartableInitContainers bool
}

// Lower preserves restartable init-container sidecars when the selected
// Kubernetes Runtime supports them. Otherwise it converts every declared unit's
// restartable init container into an ordinary container before final validation.
func (l UnitLowerer) Lower(ctx context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	if err := ctx.Err(); err != nil {
		return agentruntime.DesiredObject{}, fault.Wrap(fault.DeadlineExceeded, "lower Kubernetes workload unit", err)
	}
	if len(request.Object.Units) == 0 {
		return request.Object, nil
	}
	result := request.Object
	for _, unit := range result.Units {
		if unit.Locator.Type != result.Kind || unit.Locator.Type != result.Manifest.Type {
			return agentruntime.DesiredObject{}, fault.New(fault.InvalidArgument, fmt.Sprintf("Kubernetes workload unit %q locator type does not match object", unit.Name))
		}
		encoded, err := lowerUnit(result.Manifest.Object, unit, l.RestartableInitContainers)
		if err != nil {
			return agentruntime.DesiredObject{}, err
		}
		result.Manifest.Object = encoded
	}
	return result, nil
}

func lowerUnit(manifest []byte, unit agentruntime.RuntimeUnit, preserveRestartableInitContainers bool) ([]byte, error) {
	if err := podunit.ValidateLocator(unit.Kind, unit.Locator); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("Kubernetes workload unit %q is unsupported", unit.Name), err)
	}
	if preserveRestartableInitContainers {
		if err := podunit.Validate(manifest, unit.Kind, unit.Locator); err != nil {
			return nil, err
		}
		return manifest, nil
	}
	return podunit.MutatePodSpec(manifest, unit.Kind, unit.Locator, lowerRestartableInitContainers)
}

func lowerRestartableInitContainers(spec *corev1.PodSpec) error {
	names := make(map[string]struct{}, len(spec.Containers)+len(spec.InitContainers))
	for _, container := range spec.Containers {
		names[container.Name] = struct{}{}
	}
	ordinaryInit := make([]corev1.Container, 0, len(spec.InitContainers))
	sidecars := make([]corev1.Container, 0, len(spec.InitContainers))
	for _, container := range spec.InitContainers {
		if container.RestartPolicy == nil || *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			ordinaryInit = append(ordinaryInit, container)
			continue
		}
		if _, exists := names[container.Name]; exists {
			return fault.New(fault.Conflict, fmt.Sprintf("restartable init container %q conflicts with an ordinary container", container.Name))
		}
		names[container.Name] = struct{}{}
		container.RestartPolicy = nil
		sidecars = append(sidecars, container)
	}
	if len(sidecars) == 0 {
		return nil
	}
	spec.InitContainers = append([]corev1.Container(nil), ordinaryInit...)
	containers := make([]corev1.Container, 0, len(spec.Containers)+len(sidecars))
	containers = append(containers, spec.Containers...)
	spec.Containers = append(containers, sidecars...)
	return nil
}

var _ agentruntime.UnitLowerer = UnitLowerer{}
