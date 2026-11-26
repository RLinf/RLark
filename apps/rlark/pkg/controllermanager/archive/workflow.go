package archive

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func init() {
	DefaultRegistry.Register(&WorkflowResource{})
}

// WorkflowResource archives Workflows that were stopped.
//
// The terminal set matches the Job policy on purpose: only an explicitly
// stopped object is archived. A Workflow that reached Succeeded or Failed on
// its own is kept so its outcome stays inspectable, and deleting it would
// cascade through the owner references onto Jobs that are also still being
// inspected.
type WorkflowResource struct{}

// Kind returns the resource kind name.
func (r *WorkflowResource) Kind() string {
	return "Workflow"
}

// NewObject returns an empty Workflow for the reconciler to read into.
func (r *WorkflowResource) NewObject() client.Object {
	return &rlarkv1alpha1.Workflow{}
}

// IsTerminal reports whether the Workflow reached the archivable terminal
// state.
func (r *WorkflowResource) IsTerminal(obj client.Object) bool {
	workflow, ok := obj.(*rlarkv1alpha1.Workflow)
	if !ok {
		return false
	}
	return workflow.Status.Phase == rlarkv1alpha1.WorkflowPhaseStopped
}

// GetEndTime returns the time the Workflow entered its terminal state, or nil
// when the state machine has not recorded one yet. A nil end time leaves the
// TTL with no origin to measure from, and the reconciler then waits a full TTL
// before archiving rather than deleting the object right away.
func (r *WorkflowResource) GetEndTime(obj client.Object) *time.Time {
	workflow, ok := obj.(*rlarkv1alpha1.Workflow)
	if !ok || workflow.Status.EndTime == nil {
		return nil
	}
	t := workflow.Status.EndTime.Time
	return &t
}

// SetupWithManager registers the Workflow archive controller.
func (r *WorkflowResource) SetupWithManager(mgr ctrl.Manager, reconciler *Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Workflow{}, builder.WithPredicates(terminalOrDeletingPredicate(r.IsTerminal))).
		Named("workflow-archive").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: reconciler.MaxConcurrentReconciles}).
		Complete(reconciler)
}
