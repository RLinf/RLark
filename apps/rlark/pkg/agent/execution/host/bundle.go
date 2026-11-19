package host

import (
	"context"
	"errors"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// Validate rejects incomplete Bundle assembly before controllers start.
func (b Bundle) Validate() error {
	if b.RuntimeSelector.ID == "" || b.RuntimeSelector.Kind == "" {
		return errors.New("bundle runtime ID and kind are required")
	}
	if b.Decoder == nil {
		return errors.New("bundle task decoder is required")
	}
	if b.Executor == nil {
		return errors.New("bundle executor is required")
	}
	if b.Implementations == nil {
		return errors.New("bundle implementation registry is required")
	}
	if b.Lifecycle == nil {
		return errors.New("bundle lifecycle is required")
	}
	return nil
}

// CoordinatorExecutor adapts the production Coordinator to the narrow Host
// boundary while freezing selection to one Runtime ID and kind.
type CoordinatorExecutor struct {
	Coordinator *coordinator.Coordinator
	Selector    coordination.RuntimeSelector
}

// Reconcile delegates a current immutable snapshot to the Coordinator.
func (e CoordinatorExecutor) Reconcile(ctx context.Context, task coordination.TaskSnapshot) (coordinator.ReconcileResult, error) {
	if e.Coordinator == nil {
		return coordinator.ReconcileResult{}, errors.New("coordinator executor is not configured")
	}
	task.RuntimeSelector = e.Selector
	return e.Coordinator.Reconcile(ctx, task)
}

// CleanupTask performs identity-only cleanup for the Bundle Runtime.
func (e CoordinatorExecutor) CleanupTask(ctx context.Context, task agentruntime.TaskIdentity, workloadSelector coordination.WorkloadSelector, carrierSelector coordination.CarrierSelector) (coordinator.CleanupResult, error) {
	if e.Coordinator == nil {
		return coordinator.CleanupResult{}, errors.New("coordinator executor is not configured")
	}
	return e.Coordinator.CleanupTask(ctx, task, e.Selector, workloadSelector, carrierSelector)
}

// ObserveRoot performs targeted observation for the Bundle Runtime.
func (e CoordinatorExecutor) ObserveRoot(ctx context.Context, task coordination.TaskSnapshot, root agentruntime.ObjectRef) (coordinator.ObserveResult, error) {
	if e.Coordinator == nil {
		return coordinator.ObserveResult{}, errors.New("coordinator executor is not configured")
	}
	task.RuntimeSelector = e.Selector
	return e.Coordinator.ObserveRoot(ctx, task, root)
}
