package coordinator

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// ValidateCapabilities checks the mandatory inventory contract at assembly and
// request boundaries. Optional features are checked only where they are used;
// unknown optional advertisements do not grant any authority.
func (c *Coordinator) ValidateCapabilities(ctx context.Context, runtimeSelector coordination.RuntimeSelector, workloadSelector coordination.WorkloadSelector) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r, err := c.selectRuntime(runtimeSelector)
	if err != nil {
		return err
	}
	a, err := c.selectWorkload(workloadSelector, r.Info.Kind)
	if err != nil {
		return err
	}
	info, err := a.Info(ctx)
	if err != nil {
		return err
	}
	if !hasCapability(info.Capabilities, agentruntime.CapabilityWorkloadInventory) {
		return fault.New(fault.FailedPrecondition, "selected Workload does not advertise identity-based inventory")
	}
	if _, ok := a.(workload.InventoryAdapter); !ok {
		return fault.New(fault.FailedPrecondition, "selected Workload does not implement identity-based inventory")
	}
	if !hasCapability(r.Info.Capabilities, agentruntime.CapabilityCandidateInventory) {
		return fault.New(fault.FailedPrecondition, "selected Runtime does not advertise Adapter-directed inventory")
	}
	if _, ok := r.Provider.(agentruntime.InventoryProvider); !ok {
		return fault.New(fault.FailedPrecondition, "selected Runtime does not implement Adapter-directed inventory")
	}
	return nil
}

func validateDesiredAdoptionCapabilities(objects []agentruntime.DesiredObject, adapterCapabilities, runtimeCapabilities []agentruntime.Capability) error {
	for _, object := range objects {
		if object.Ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption {
			return validateIncarnationAdoptionCapabilities([]agentruntime.ObjectCandidate{{Ownership: object.Ownership}}, adapterCapabilities, runtimeCapabilities)
		}
	}
	return nil
}

func validateOwnedFieldCapabilities(objects []agentruntime.DesiredObject, adapterCapabilities, runtimeCapabilities []agentruntime.Capability) error {
	for _, object := range objects {
		if len(object.OwnedFields) == 0 && object.Ownership.Scope != agentruntime.IdentitySharedResource {
			continue
		}
		if !hasCapability(adapterCapabilities, agentruntime.CapabilityOwnedFieldProjection) || !hasCapability(runtimeCapabilities, agentruntime.CapabilityOwnedFieldProjection) {
			return fault.New(fault.FailedPrecondition, "owned-field projection requires Workload and Runtime capabilities")
		}
		if object.Ownership.Scope == agentruntime.IdentitySharedResource && len(object.OwnedFields) == 0 {
			return fault.New(fault.FailedPrecondition, "shared resource requires an explicit owned-field projection")
		}
	}
	return nil
}
