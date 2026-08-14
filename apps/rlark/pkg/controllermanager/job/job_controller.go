package job

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

// Reconciler reconciles Job resources.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rlinf.io,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/status,verbs=get;update;patch

// Reconcile handles a Job reconciliation request.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return controller.ReconcileWith(ctx, req, &rlarkv1alpha1.Job{}, "job", r)
}

// IsTerminal reports whether terminal.
func (r *Reconciler) IsTerminal(obj client.Object) bool {
	job := obj.(*rlarkv1alpha1.Job)
	return job.Status.Phase == rlarkv1alpha1.JobPhaseSucceeded ||
		job.Status.Phase == rlarkv1alpha1.JobPhaseFailed
}

// ReconcileStateMachine reconciles the resource.
func (r *Reconciler) ReconcileStateMachine(ctx context.Context, obj client.Object) (bool, error) {
	return r.reconcileWithStateMachine(ctx, obj.(*rlarkv1alpha1.Job))
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Job{}).
		Owns(&rlarkv1alpha1.Task{}).
		Named("job").
		Complete(r)
}
