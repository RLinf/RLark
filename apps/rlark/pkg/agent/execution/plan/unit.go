package plan

import (
	"fmt"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// ProjectUnits validates Adapter declarations and attaches an object-relative,
// read-only projection to each desired object for Runtime consumption.
func ProjectUnits(units []WorkloadUnit, objects []agentruntime.DesiredObject) ([]agentruntime.DesiredObject, error) {
	result := make([]agentruntime.DesiredObject, len(objects))
	byName := make(map[string]int, len(objects))
	for i := range objects {
		result[i] = objects[i]
		result[i].Units = append([]agentruntime.RuntimeUnit(nil), objects[i].Units...)
		if objects[i].LogicalName == "" {
			return nil, fault.New(fault.InvalidArgument, "workload unit object has an empty logical name")
		}
		if _, exists := byName[objects[i].LogicalName]; exists {
			return nil, fault.New(fault.InvalidArgument, "workload units reference duplicate object logical names")
		}
		byName[objects[i].LogicalName] = i
	}
	seen := make(map[string]struct{}, len(units))
	for _, unit := range units {
		if strings.TrimSpace(unit.Name) == "" {
			return nil, fault.New(fault.InvalidArgument, "workload unit name is required")
		}
		if _, exists := seen[unit.Name]; exists {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate workload unit %q", unit.Name))
		}
		seen[unit.Name] = struct{}{}
		index, exists := byName[unit.ObjectName]
		if !exists {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("workload unit %q references missing object %q", unit.Name, unit.ObjectName))
		}
		object := result[index]
		if unit.Locator.Type != object.Kind || unit.Locator.Type != object.Manifest.Type {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("workload unit %q locator type does not match object %q", unit.Name, unit.ObjectName))
		}
		switch unit.Kind {
		case agentruntime.UnitPodTemplate:
			if strings.TrimSpace(unit.Locator.Path) == "" {
				return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("PodTemplate workload unit %q requires a static locator path", unit.Name))
			}
		case agentruntime.UnitPod:
			if strings.TrimSpace(unit.Locator.Path) == "" {
				return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("Pod workload unit %q requires a static locator path", unit.Name))
			}
		default:
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("workload unit %q has unsupported kind %q", unit.Name, unit.Kind))
		}
		result[index].Units = append(result[index].Units, agentruntime.RuntimeUnit{Name: unit.Name, Kind: unit.Kind, Role: unit.Role, Locator: unit.Locator})
	}
	return result, nil
}
