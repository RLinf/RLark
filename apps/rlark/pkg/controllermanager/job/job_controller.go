package job

import (
	"context"
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	CleanupFinalizer = "jobs.rlinf.io/task-cleanup"
	cleanupRequeue   = time.Second
)

// Reason codes for the passes this controller classifies itself, as opposed to
// the ones the shell classifies. They are a closed set because they are used as
// a metric label.
const (
	reasonCleanupFinalizerAdded = "CleanupFinalizerAdded"
	reasonNotOwned              = "NotOwned"
	reasonTaskStopPending       = "TaskStopPending"
	reasonTaskDeletePending     = "TaskDeletePending"
	reasonStoppedStatusRecorded = "StoppedStatusRecorded"
	reasonCleanupFinalizerFreed = "CleanupFinalizerFreed"
)

// Reconciler reconciles Job resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	// Shell classifies every pass, translates a retry class into a jittered
	// interval and records the result. The zero value is silent, so a test may
	// leave it unset.
	Shell controller.Shell
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
	if job.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(job, CleanupFinalizer) {
		return r.Shell.RunStateMachine(ctx, req, &rlarkv1alpha1.Job{}, "job", r)
	}
	return r.Shell.Run(ctx, "job", func(ctx context.Context) (controller.Result, error) {
		if job.DeletionTimestamp.IsZero() {
			return r.addCleanupFinalizer(ctx, job)
		}
		return r.reconcileDeletion(ctx, job)
	})
}

// addCleanupFinalizer claims the Job before any child is created, so that a
// delete request can never win a race against child creation.
//
// The pass then waits for the watch event produced by its own Update instead of
// requeueing immediately: the Job predicate admits finalizer changes, so the
// next pass is already guaranteed. Asking for an immediate requeue would only
// bypass the workqueue rate limiter.
func (r *Reconciler) addCleanupFinalizer(ctx context.Context, job *rlarkv1alpha1.Job) (controller.Result, error) {
	controllerutil.AddFinalizer(job, CleanupFinalizer)
	if err := r.Update(ctx, job); err != nil {
		return controller.Result{Reason: reasonCleanupFinalizerAdded},
			fmt.Errorf("add Job cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeWaitingForChange,
		Reason:  reasonCleanupFinalizerAdded,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
	}, nil
}

// reconcileDeletion stops and then deletes the Tasks this Job owns, and only
// then releases the finalizer.
func (r *Reconciler) reconcileDeletion(ctx context.Context, job *rlarkv1alpha1.Job) (controller.Result, error) {
	if !controllerutil.ContainsFinalizer(job, CleanupFinalizer) {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  reasonNotOwned,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}

	// Snapshot before the status is edited in place so the write below can be a
	// patch of what this pass changed rather than a full-object update.
	before := job.DeepCopy()

	stopped, err := r.stopOwnedTasks(ctx, job)
	if err != nil {
		return controller.Result{Reason: reasonTaskStopPending}, err
	}
	if !stopped {
		return inProgress(reasonTaskStopPending), nil
	}
	statusChanged := false
	if !jobTaskStatusesStopped(job) {
		statusChanged, err = r.syncTaskStatuses(ctx, job)
		if err != nil {
			return controller.Result{Reason: reasonStoppedStatusRecorded}, err
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
		if err := r.Status().Patch(ctx, job, client.MergeFrom(before)); err != nil {
			return controller.Result{Reason: reasonStoppedStatusRecorded},
				fmt.Errorf("update stopped Job status: %w", err)
		}
		// A status-only write does not pass the Job predicate, so deletion has
		// to be driven forward by an explicit retry rather than by a watch.
		return inProgress(reasonStoppedStatusRecorded), nil
	}
	pending, err := r.deleteOwnedTasks(ctx, job)
	if err != nil {
		return controller.Result{Reason: reasonTaskDeletePending}, err
	}
	if pending {
		return inProgress(reasonTaskDeletePending), nil
	}
	controllerutil.RemoveFinalizer(job, CleanupFinalizer)
	if err := r.Update(ctx, job); err != nil {
		return controller.Result{Reason: reasonCleanupFinalizerFreed},
			fmt.Errorf("remove Job cleanup finalizer: %w", err)
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonCleanupFinalizerFreed,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

// inProgress reports that cleanup advanced but has not finished, and that the
// next pass has to be self-driven because no watch event will arrive.
func inProgress(reason string) controller.Result {
	return controller.Result{
		Outcome: sharedreconcile.OutcomeCleanupInProgress,
		Reason:  reason,
		Retry: sharedreconcile.RetryDirective{
			Class: sharedreconcile.RetryInProgress,
			After: cleanupRequeue,
		},
	}
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

// cleanupTasks returns the Tasks this Job is responsible for tearing down.
//
// Ownership is decided by the shared classifier rather than by a local
// owner-reference comparison, so teardown and steady-state convergence can
// never disagree about which Tasks belong to this Job. Tasks adopted from the
// legacy naming scheme are included, which is why the names recorded in Status
// are passed in: without them a Job created before the ownership metadata
// existed would release its finalizer while its Tasks were still running.
//
// A Task whose ownership cannot be proven is an error, not a silent skip:
// deleting it could destroy another parent's child, and skipping it would
// release the finalizer while the Task is still around.
func (r *Reconciler) cleanupTasks(ctx context.Context, job *rlarkv1alpha1.Job) ([]*rlarkv1alpha1.Task, error) {
	tasks, err := r.listJobTasks(ctx, job)
	if err != nil {
		return nil, err
	}
	parent := jobParentRef(job)
	legacyNames := legacyTaskNames(job)
	owned := make([]*rlarkv1alpha1.Task, 0, len(tasks))
	for _, task := range tasks {
		decision, err := utils.ClassifyRemovedChild(task, parent, legacyNames)
		if err != nil {
			return nil, fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
		}
		if !decision.Owned() {
			continue
		}
		owned = append(owned, task)
	}
	return owned, nil
}

func (r *Reconciler) stopOwnedTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	tasks, err := r.cleanupTasks(ctx, job)
	if err != nil {
		return false, err
	}

	allStopped := true
	for _, task := range tasks {
		if !controllerutil.ContainsFinalizer(task, common.TaskAgentCleanupFinalizer) {
			if task.DeletionTimestamp.IsZero() {
				if err := client.IgnoreNotFound(r.Delete(ctx, task)); err != nil {
					return false, fmt.Errorf("delete unclaimed Task %s/%s: %w", task.Namespace, task.Name, err)
				}
			}
			continue
		}
		if task.Status.Phase != rlarkv1alpha1.TaskPhaseStopped {
			allStopped = false
		}
		if task.DeletionTimestamp.IsZero() && !task.Spec.Suspended {
			// Suspending touches exactly one Spec field, so patch it rather
			// than resubmitting the whole object and racing its owner.
			before := task.DeepCopy()
			task.Spec.Suspended = true
			if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
				return false, fmt.Errorf("stop Task %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
	}
	return allStopped, nil
}

func (r *Reconciler) deleteOwnedTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	tasks, err := r.cleanupTasks(ctx, job)
	if err != nil {
		return false, err
	}

	pending := false
	for _, task := range tasks {
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

// SetObservedGeneration implements controller.ObservedGenerationRecorder.
//
// The shell calls this only after a pass that fully processed the current Spec,
// so the field keeps its contract instead of being advanced by a state machine
// that returned early.
func (r *Reconciler) SetObservedGeneration(obj client.Object, generation int64) bool {
	job := obj.(*rlarkv1alpha1.Job)
	if job.Status.ObservedGeneration == generation {
		return false
	}
	job.Status.ObservedGeneration = generation
	return true
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &rlarkv1alpha1.Task{}, taskJobField, indexTaskByJob); err != nil {
		return fmt.Errorf("index Tasks by Job: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Job{}, builder.WithPredicates(jobSpecChangedPredicate())).
		Owns(&rlarkv1alpha1.Task{}).
		Watches(&rlarkv1alpha1.Node{}, handler.EnqueueRequestsFromMapFunc(r.jobsForSchedulingChange), builder.WithPredicates(nodeSchedulingPredicate())).
		Watches(&rlarkv1alpha1.Cluster{}, handler.EnqueueRequestsFromMapFunc(r.jobsForSchedulingChange), builder.WithPredicates(clusterSchedulingPredicate())).
		Named("job").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *Reconciler) jobsForSchedulingChange(ctx context.Context, _ client.Object) []reconcile.Request {
	var jobs rlarkv1alpha1.JobList
	if err := r.List(ctx, &jobs); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(jobs.Items))
	for i := range jobs.Items {
		if !jobAwaitingPlacement(&jobs.Items[i]) {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: jobs.Items[i].Name}})
	}
	return requests
}

// jobAwaitingPlacement reports whether Node or Cluster changes can still alter
// where this Job's Tasks are placed. Node condition updates arrive at heartbeat
// frequency, so enqueueing every Job would make reconcile work grow with Jobs ×
// Node events; Jobs that are suspended, deleting, terminal, or whose Tasks have
// all left Pending cannot change placement and are skipped. Keys already queued
// are coalesced by the controller workqueue, so the map function only has to
// bound the set it returns.
func jobAwaitingPlacement(job *rlarkv1alpha1.Job) bool {
	if !job.DeletionTimestamp.IsZero() || job.Spec.Suspended {
		return false
	}
	switch job.Status.Phase {
	case rlarkv1alpha1.JobPhaseSucceeded, rlarkv1alpha1.JobPhaseFailed, rlarkv1alpha1.JobPhaseStopped:
		return false
	}
	if len(job.Status.Tasks) == 0 {
		// A newly created Job has not recorded a Task snapshot yet, so its first
		// reconcile still has to observe Nodes.
		return true
	}
	for i := range job.Status.Tasks {
		switch job.Status.Tasks[i].Phase {
		case rlarkv1alpha1.TaskPhasePending, "":
			return true
		}
	}
	return false
}

// nodeSchedulingPredicate admits only Node changes that alter the inputs
// resolveTaskNamespace reads: the controller-written Schedulable condition,
// selector labels, deletion, and the capability snapshot.
// Comparing the whole Conditions slice would also compare Message,
// LastTransitionTime, and LastHeartbeatTime, so ordinary heartbeats and
// pressure-message updates would enqueue scheduling work without changing any
// placement decision.
func nodeSchedulingPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, oldOK := e.ObjectOld.(*rlarkv1alpha1.Node)
			newNode, newOK := e.ObjectNew.(*rlarkv1alpha1.Node)
			if !oldOK || !newOK {
				return true
			}
			// Only the controller-written Schedulable condition gates placement, so
			// changing it is the only Node condition change that can affect Jobs.
			// spec.unschedulable feeds that condition; reacting to the spec directly
			// would reconcile against a stale condition before the Node Controller
			// refreshes it.
			return nodeReadyForScheduling(oldNode.Status.Conditions) != nodeReadyForScheduling(newNode.Status.Conditions) ||
				!reflect.DeepEqual(oldNode.Status.CapabilitySnapshot, newNode.Status.CapabilitySnapshot) ||
				!reflect.DeepEqual(oldNode.Status.ObservedNodeLabels, newNode.Status.ObservedNodeLabels) ||
				!reflect.DeepEqual(oldNode.DeletionTimestamp, newNode.DeletionTimestamp)
		},
	}
}

func clusterSchedulingPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, oldOK := e.ObjectOld.(*rlarkv1alpha1.Cluster)
			newCluster, newOK := e.ObjectNew.(*rlarkv1alpha1.Cluster)
			if !oldOK || !newOK {
				return true
			}
			return oldCluster.Spec.Cordoned != newCluster.Spec.Cordoned ||
				oldCluster.Spec.Draining != newCluster.Spec.Draining ||
				oldCluster.Spec.Delisted != newCluster.Spec.Delisted ||
				oldCluster.Status.Phase != newCluster.Status.Phase ||
				!reflect.DeepEqual(oldCluster.DeletionTimestamp, newCluster.DeletionTimestamp)
		},
	}
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
