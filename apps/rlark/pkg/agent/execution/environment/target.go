package environment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// ValidateTargets validates Adapter-declared mutation targets against the
// desired objects they authorize and returns a name-indexed copy. Runtime-
// specific locator validation remains the selected Binder's responsibility.
func ValidateTargets(targets []executionplan.MutationTarget, objects []agentruntime.DesiredObject) (map[string]executionplan.MutationTarget, error) {
	objectsByName := make(map[string]agentruntime.DesiredObject, len(objects))
	for _, object := range objects {
		if object.LogicalName == "" {
			return nil, fault.New(fault.InvalidArgument, "environment target object has an empty logical name")
		}
		if _, found := objectsByName[object.LogicalName]; found {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment targets reference duplicate object logical name %q", object.LogicalName))
		}
		objectsByName[object.LogicalName] = object
	}

	result := make(map[string]executionplan.MutationTarget, len(targets))
	for _, target := range targets {
		if strings.TrimSpace(target.Name) == "" {
			return nil, fault.New(fault.InvalidArgument, "environment target name is required")
		}
		if _, found := result[target.Name]; found {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate environment target %q", target.Name))
		}
		object, found := objectsByName[target.ObjectName]
		if !found {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment target %q references missing object %q", target.Name, target.ObjectName))
		}
		if target.Kind != executionplan.TargetPodTemplate && target.Kind != executionplan.TargetPod {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment target %q has unsupported kind %q", target.Name, target.Kind))
		}
		if target.Locator.Type != object.Kind || target.Locator.Type != object.Manifest.Type {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment target %q locator type does not match object %q", target.Name, target.ObjectName))
		}
		if strings.TrimSpace(target.Locator.Path) == "" {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment target %q locator path is required", target.Name))
		}
		result[target.Name] = target
	}
	return result, nil
}

// ResolveConsumers reduces exact target selectors to canonical concrete
// consumers. Compatibility selectors are rejected here until an Adapter-owned
// normalization path can prove that they select exactly one target.
func ResolveConsumers(selectors []executionplan.ConsumerSelector, targets map[string]executionplan.MutationTarget) ([]ResolvedConsumer, error) {
	resolved := make(map[string]ResolvedConsumer)
	for _, selector := range selectors {
		if len(selector.ObjectNames) != 0 || len(selector.Roles) != 0 || len(selector.UnitNames) != 0 {
			return nil, fault.New(fault.InvalidArgument, "legacy environment consumer selectors must be normalized to exact targets")
		}
		if len(selector.Targets) == 0 {
			return nil, fault.New(fault.InvalidArgument, "environment consumer selector must name at least one target")
		}
		for _, name := range selector.Targets {
			target, found := targets[name]
			if !found {
				return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("environment consumer references unknown target %q", name))
			}
			resolved[name] = ResolvedConsumer{TargetName: name, ObjectName: target.ObjectName, Role: target.Role}
		}
	}

	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)
	consumers := make([]ResolvedConsumer, 0, len(names))
	for _, name := range names {
		consumers = append(consumers, resolved[name])
	}
	return consumers, nil
}
