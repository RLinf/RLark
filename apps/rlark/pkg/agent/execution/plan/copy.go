package plan

import (
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// DeepCopy returns a Plan with no mutable aliases to the receiver. Binders must
// mutate only this working copy so a failed binding cannot corrupt the
// Adapter's compiled intent. Nested Environment data is copied by its owning
// package.
func (plan Plan) DeepCopy() Plan {
	result := plan
	result.Objects = make([]agentruntime.DesiredObject, len(plan.Objects))
	for i := range plan.Objects {
		result.Objects[i] = plan.Objects[i]
		result.Objects[i].Manifest.Object = append([]byte(nil), plan.Objects[i].Manifest.Object...)
		result.Objects[i].DependsOn = append([]string(nil), plan.Objects[i].DependsOn...)
		result.Objects[i].OwnedFields = append([]string(nil), plan.Objects[i].OwnedFields...)
		result.Objects[i].Units = append([]agentruntime.RuntimeUnit(nil), plan.Objects[i].Units...)
	}
	result.Targets = append([]MutationTarget(nil), plan.Targets...)
	result.Units = append([]WorkloadUnit(nil), plan.Units...)
	result.Processes = append([]executionprocess.Declaration(nil), plan.Processes...)
	for i := range result.Processes {
		result.Processes[i].Args = append([]string(nil), plan.Processes[i].Args...)
	}
	result.Environment = plan.Environment.DeepCopy()
	result.Discovery.Roots = append([]string(nil), plan.Discovery.Roots...)
	if plan.Discovery.Options != nil {
		options := *plan.Discovery.Options
		options.Value = append([]byte(nil), plan.Discovery.Options.Value...)
		result.Discovery.Options = &options
	}
	return result
}

// DeepCopy returns Requirements with no mutable aliases to the receiver.
func (requirements Requirements) DeepCopy() Requirements {
	result := requirements
	result.Environments = append([]EnvironmentRequirement(nil), requirements.Environments...)
	for i := range result.Environments {
		result.Environments[i].Parameters = append([]byte(nil), requirements.Environments[i].Parameters...)
		result.Environments[i].Consumers = deepCopyConsumers(requirements.Environments[i].Consumers)
	}
	return result
}

func deepCopyConsumers(consumers []ConsumerSelector) []ConsumerSelector {
	result := append([]ConsumerSelector(nil), consumers...)
	for i := range result {
		result[i].Targets = append([]string(nil), consumers[i].Targets...)
		result[i].ObjectNames = append([]string(nil), consumers[i].ObjectNames...)
		result[i].Roles = append([]string(nil), consumers[i].Roles...)
		result[i].UnitNames = append([]string(nil), consumers[i].UnitNames...)
	}
	return result
}
