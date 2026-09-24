package workflow

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

const (
	CleanupFinalizer = "workflows.rlinf.io/job-cleanup"
	cleanupRequeue   = time.Second
)

// Reconciler reconciles Workflow resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=rlinf.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=workflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=workflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/status,verbs=get;update;patch

// Reconcile handles a Workflow reconciliation request.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	wf := &rlarkv1alpha1.Workflow{}
	if err := r.Get(ctx, req.NamespacedName, wf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if wf.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(wf, CleanupFinalizer) {
			controllerutil.AddFinalizer(wf, CleanupFinalizer)
			if err := r.Update(ctx, wf); err != nil {
				return ctrl.Result{}, fmt.Errorf("add Workflow cleanup finalizer: %w", err)
			}
			return ctrl.Result{RequeueAfter: time.Nanosecond}, nil
		}
		return controller.ReconcileWith(ctx, req, &rlarkv1alpha1.Workflow{}, "workflow", r)
	}
	if !controllerutil.ContainsFinalizer(wf, CleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	pending, err := r.deleteOwnedJobs(ctx, wf)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
	}
	controllerutil.RemoveFinalizer(wf, CleanupFinalizer)
	if err := r.Update(ctx, wf); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove Workflow cleanup finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteOwnedJobs(ctx context.Context, wf *rlarkv1alpha1.Workflow) (bool, error) {
	var jobs rlarkv1alpha1.JobList
	if err := r.List(ctx, &jobs); err != nil {
		return false, fmt.Errorf("list Jobs owned by Workflow %s: %w", wf.Name, err)
	}

	pending := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		owner := metav1.GetControllerOf(job)
		if owner == nil || owner.UID != wf.UID || owner.Kind != "Workflow" || owner.APIVersion != rlarkv1alpha1.GroupVersion.String() {
			continue
		}
		pending = true
		if job.DeletionTimestamp.IsZero() {
			if err := client.IgnoreNotFound(r.Delete(ctx, job)); err != nil {
				return false, fmt.Errorf("delete Job %s: %w", job.Name, err)
			}
		}
	}
	return pending, nil
}

// IsTerminal reports whether the generic controller may stop reconciling.
// Completed Workflows still need child pruning and status normalization.
func (r *Reconciler) IsTerminal(client.Object) bool { return false }

// ReconcileStateMachine reconciles the resource.
func (r *Reconciler) ReconcileStateMachine(ctx context.Context, obj client.Object) (bool, error) {
	return r.reconcileWithStateMachine(ctx, obj.(*rlarkv1alpha1.Workflow))
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Workflow{}).
		Owns(&rlarkv1alpha1.Job{}).
		Named("workflow").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
