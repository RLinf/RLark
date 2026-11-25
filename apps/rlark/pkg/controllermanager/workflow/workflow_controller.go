package workflow

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	CleanupFinalizer = "workflows.rlinf.io/job-cleanup"
	cleanupRequeue   = time.Second
)

// Reason codes for the passes this controller classifies itself. Closed set:
// they are used as a metric label.
const (
	reasonCleanupFinalizerAdded = "CleanupFinalizerAdded"
	reasonNotOwned              = "NotOwned"
	reasonJobDeletePending      = "JobDeletePending"
	reasonCleanupFinalizerFreed = "CleanupFinalizerFreed"
)

// Reconciler reconciles Workflow resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	// Shell classifies every pass, translates a retry class into a jittered
	// interval and records the result. The zero value is silent, so a test may
	// leave it unset.
	Shell controller.Shell
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
	if wf.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(wf, CleanupFinalizer) {
		return r.Shell.RunStateMachine(ctx, req, &rlarkv1alpha1.Workflow{}, "workflow", r)
	}
	return r.Shell.Run(ctx, "workflow", func(ctx context.Context) (controller.Result, error) {
		if wf.DeletionTimestamp.IsZero() {
			return r.addCleanupFinalizer(ctx, wf)
		}
		return r.reconcileDeletion(ctx, wf)
	})
}

// addCleanupFinalizer claims the Workflow before any Job is created.
//
// The next pass is driven by the watch event produced by this Update rather
// than by an immediate requeue, which would only bypass the rate limiter.
func (r *Reconciler) addCleanupFinalizer(ctx context.Context, wf *rlarkv1alpha1.Workflow) (controller.Result, error) {
	controllerutil.AddFinalizer(wf, CleanupFinalizer)
	if err := r.Update(ctx, wf); err != nil {
		return controller.Result{Reason: reasonCleanupFinalizerAdded},
			fmt.Errorf("add Workflow cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeWaitingForChange,
		Reason:  reasonCleanupFinalizerAdded,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
	}, nil
}

func (r *Reconciler) reconcileDeletion(ctx context.Context, wf *rlarkv1alpha1.Workflow) (controller.Result, error) {
	if !controllerutil.ContainsFinalizer(wf, CleanupFinalizer) {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  reasonNotOwned,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	pending, err := r.deleteOwnedJobs(ctx, wf)
	if err != nil {
		return controller.Result{Reason: reasonJobDeletePending}, err
	}
	if pending {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeCleanupInProgress,
			Reason:  reasonJobDeletePending,
			Retry: sharedreconcile.RetryDirective{
				Class: sharedreconcile.RetryInProgress,
				After: cleanupRequeue,
			},
		}, nil
	}
	controllerutil.RemoveFinalizer(wf, CleanupFinalizer)
	if err := r.Update(ctx, wf); err != nil {
		return controller.Result{Reason: reasonCleanupFinalizerFreed},
			fmt.Errorf("remove Workflow cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonCleanupFinalizerFreed,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

func (r *Reconciler) deleteOwnedJobs(ctx context.Context, wf *rlarkv1alpha1.Workflow) (bool, error) {
	jobs, err := r.listWorkflowJobs(ctx, wf)
	if err != nil {
		return false, err
	}

	parent := workflowParentRef(wf)
	legacyNames := legacyJobNames(wf)
	pending := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		decision, err := utils.ClassifyRemovedChild(job, parent, legacyNames)
		if err != nil {
			return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
		}
		if !decision.Owned() {
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

// SetObservedGeneration implements controller.ObservedGenerationRecorder.
//
// The shell calls this only after a pass that fully processed the current Spec,
// so consumers can still tell that Status does not yet describe the current
// Spec when a pass ended early.
func (r *Reconciler) SetObservedGeneration(obj client.Object, generation int64) bool {
	wf := obj.(*rlarkv1alpha1.Workflow)
	if wf.Status.ObservedGeneration == generation {
		return false
	}
	wf.Status.ObservedGeneration = generation
	return true
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &rlarkv1alpha1.Job{},
		jobWorkflowField, indexJobByWorkflow); err != nil {
		return fmt.Errorf("index Jobs by Workflow: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Workflow{}).
		Owns(&rlarkv1alpha1.Job{}).
		Named("workflow").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
