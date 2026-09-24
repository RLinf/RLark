package job

import (
	"context"
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

const (
	CleanupFinalizer = "jobs.rlinf.io/task-cleanup"
	cleanupRequeue   = time.Second
)

// Reconciler reconciles Job resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=rlinf.io,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/status,verbs=get;update;patch

// Reconcile handles a Job reconciliation request.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	job := &rlarkv1alpha1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if job.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(job, CleanupFinalizer) {
			controllerutil.AddFinalizer(job, CleanupFinalizer)
			if err := r.Update(ctx, job); err != nil {
				return ctrl.Result{}, fmt.Errorf("add Job cleanup finalizer: %w", err)
			}
			return ctrl.Result{RequeueAfter: time.Nanosecond}, nil
		}
		return controller.ReconcileWith(ctx, req, &rlarkv1alpha1.Job{}, "job", r)
	}
	if !controllerutil.ContainsFinalizer(job, CleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	stopped, err := r.stopOwnedTasks(ctx, job)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !stopped {
		return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
	}
	statusChanged := false
	if !jobTaskStatusesStopped(job) {
		statusChanged, err = r.syncTaskStatuses(ctx, job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	terminal := job.Status.Phase == rlarkv1alpha1.JobPhaseSucceeded ||
		job.Status.Phase == rlarkv1alpha1.JobPhaseFailed
	if job.Status.Phase != rlarkv1alpha1.JobPhaseStopped && !terminal {
		job.Status.Phase = rlarkv1alpha1.JobPhaseStopped
		statusChanged = true
	}
	if job.Status.EndTime == nil && !terminal {
		now := metav1.Now()
		job.Status.EndTime = &now
		statusChanged = true
	}
	if statusChanged {
		if err := r.Status().Update(ctx, job); err != nil {
			return ctrl.Result{}, fmt.Errorf("update stopped Job status: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Nanosecond}, nil
	}
	pending, err := r.deleteOwnedTasks(ctx, job)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
	}
	controllerutil.RemoveFinalizer(job, CleanupFinalizer)
	if err := r.Update(ctx, job); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove Job cleanup finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func jobTaskStatusesStopped(job *rlarkv1alpha1.Job) bool {
	if len(job.Status.Tasks) != len(job.Spec.Tasks) {
		return false
	}
	for _, task := range job.Status.Tasks {
		if task.Phase != rlarkv1alpha1.TaskPhaseStopped {
			return false
		}
	}
	return true
}

func (r *Reconciler) stopOwnedTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks); err != nil {
		return false, fmt.Errorf("list Tasks owned by Job %s: %w", job.Name, err)
	}

	allStopped := true
	for i := range tasks.Items {
		task := &tasks.Items[i]
		owner := metav1.GetControllerOf(task)
		if owner == nil || owner.UID != job.UID || owner.Kind != "Job" || owner.APIVersion != rlarkv1alpha1.GroupVersion.String() {
			continue
		}
		if task.Status.Phase != rlarkv1alpha1.TaskPhaseStopped {
			allStopped = false
		}
		if task.DeletionTimestamp.IsZero() && task.Annotations[StoppedAnnotation] != "true" {
			if task.Annotations == nil {
				task.Annotations = map[string]string{}
			}
			task.Annotations[StoppedAnnotation] = "true"
			if task.Spec.Kubernetes != nil && task.Spec.Kubernetes.Workload != nil {
				task.Spec.Kubernetes.Workload.Replicas = ptr.To(int32(0))
			}
			if err := r.Update(ctx, task); err != nil {
				return false, fmt.Errorf("stop Task %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
	}
	return allStopped, nil
}

func (r *Reconciler) deleteOwnedTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks); err != nil {
		return false, fmt.Errorf("list Tasks owned by Job %s: %w", job.Name, err)
	}

	pending := false
	for i := range tasks.Items {
		task := &tasks.Items[i]
		owner := metav1.GetControllerOf(task)
		if owner == nil || owner.UID != job.UID || owner.Kind != "Job" || owner.APIVersion != rlarkv1alpha1.GroupVersion.String() {
			continue
		}
		pending = true
		if task.DeletionTimestamp.IsZero() {
			if err := client.IgnoreNotFound(r.Delete(ctx, task)); err != nil {
				return false, fmt.Errorf("delete Task %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
	}
	return pending, nil
}

// IsTerminal reports whether terminal.
func (r *Reconciler) IsTerminal(client.Object) bool {
	return false
}

// ReconcileStateMachine reconciles the resource.
func (r *Reconciler) ReconcileStateMachine(ctx context.Context, obj client.Object) (bool, error) {
	return r.reconcileWithStateMachine(ctx, obj.(*rlarkv1alpha1.Job))
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Job{}, builder.WithPredicates(jobSpecChangedPredicate())).
		Owns(&rlarkv1alpha1.Task{}).
		Named("job").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}

func jobSpecChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, oldOK := e.ObjectOld.(*rlarkv1alpha1.Job)
			newJob, newOK := e.ObjectNew.(*rlarkv1alpha1.Job)
			return !oldOK || !newOK || oldJob.Generation != newJob.Generation ||
				!reflect.DeepEqual(oldJob.Annotations, newJob.Annotations) ||
				!reflect.DeepEqual(oldJob.Finalizers, newJob.Finalizers) ||
				!reflect.DeepEqual(oldJob.DeletionTimestamp, newJob.DeletionTimestamp)
		},
	}
}
