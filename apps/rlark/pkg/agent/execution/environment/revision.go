package environment

import (
	"encoding/json"
	"sort"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type bindingRevisionProjection struct {
	Kind            RequirementKind                `json:"kind"`
	RequirementName string                         `json:"requirementName"`
	Profile         ProfileReference               `json:"profile"`
	Consumers       []ResolvedConsumer             `json:"consumers,omitempty"`
	Ownership       agentruntime.ResourceOwnership `json:"ownership"`
	Lifecycle       agentruntime.ResourceLifecycle `json:"lifecycle"`
	SourceRevision  string                         `json:"sourceRevision,omitempty"`
	Algorithm       string                         `json:"algorithm"`
	ValueType       agentruntime.ObjectKind        `json:"valueType"`
}

// BindingRevision returns a canonical non-secret digest of resolved binding
// semantics. ResolvedBinding.Value is deliberately excluded because it may
// contain sensitive or order-unstable Runtime material.
func BindingRevision(resolved ResolvedEnvironment) (string, error) {
	projection := make([]bindingRevisionProjection, 0, len(resolved.Bindings))
	for _, binding := range resolved.Bindings {
		consumers := append([]ResolvedConsumer(nil), binding.Consumers...)
		sort.Slice(consumers, func(i, j int) bool {
			left, right := consumers[i], consumers[j]
			return left.TargetName+"\x00"+left.ObjectName+"\x00"+left.Role+"\x00"+left.UnitName < right.TargetName+"\x00"+right.ObjectName+"\x00"+right.Role+"\x00"+right.UnitName
		})
		projection = append(projection, bindingRevisionProjection{
			Kind: binding.Kind, RequirementName: binding.RequirementName, Profile: binding.Profile,
			Consumers: consumers, Ownership: binding.Ownership, Lifecycle: binding.Lifecycle,
			SourceRevision: binding.SourceRevision, Algorithm: binding.Algorithm, ValueType: binding.Value.Type,
		})
	}
	sort.Slice(projection, func(i, j int) bool {
		left, right := projection[i], projection[j]
		return string(left.Kind)+"\x00"+left.RequirementName+"\x00"+left.Profile.Type+"\x00"+left.Profile.Name < string(right.Kind)+"\x00"+right.RequirementName+"\x00"+right.Profile.Type+"\x00"+right.Profile.Name
	})
	data, err := json.Marshal(projection)
	if err != nil {
		return "", fault.Wrap(fault.Internal, "encode environment binding revision", err)
	}
	return string(evidence.NewDigest(data)), nil
}
