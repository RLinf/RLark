package controller

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// Observation is the observed-state payload carried by a control-plane Result.
//
// It is empty because a control-plane state machine edits the typed Status of
// the object it already holds, instead of reporting an observation gathered
// from somewhere else. The data-plane Host, which does report such an
// observation, instantiates the same shared Result with a real payload type.
type Observation = struct{}

// Result is the control-plane instantiation of the shared reconcile result, so
// that the control plane and the data plane classify a pass against one set of
// Outcome and RetryClass values rather than two parallel vocabularies.
type Result = sharedreconcile.Result[Observation]

// Reason codes attached to a classified pass by the shell itself. They are a
// closed set: the shell never derives a reason from an error string or an
// object name, because these values are used as a metric label.
const (
	// ReasonSynced is the steady state: the pass finished and the next one
	// should be driven by a watch event.
	ReasonSynced = "Synced"
	// ReasonObjectGone means the object disappeared between the event and the
	// Get. There is nothing left to reconcile and nothing to report.
	ReasonObjectGone = "ObjectGone"
	// ReasonTerminal means the state machine declared the object finished and
	// declined to look at it.
	ReasonTerminal = "Terminal"
	// ReasonChildCleanupInProgress means the pass asked to come back because a
	// child it had stopped is not gone yet.
	ReasonChildCleanupInProgress = "ChildCleanupInProgress"
	// ReasonReconcileFailed is the catch-all for a state machine error.
	ReasonReconcileFailed = "ReconcileFailed"
	// ReasonStatusWriteFailed means the decision was made but could not be
	// persisted.
	ReasonStatusWriteFailed = "StatusWriteFailed"
	// ReasonStatusWriteConflict distinguishes the expected, benign case of
	// another writer having won a race from a genuine write failure.
	ReasonStatusWriteConflict = "StatusWriteConflict"
)

// Reconciler reconciles resources.
type Reconciler interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	Status() client.StatusWriter
	ReconcileStateMachine(ctx context.Context, obj client.Object) (bool, error)
	IsTerminal(obj client.Object) bool
}

// ObservedGenerationRecorder is implemented by a Reconciler whose resource
// carries Status.ObservedGeneration.
//
// The field is advanced by the shell rather than by each state machine, because
// its contract is "the Spec at this generation has been fully processed". Only
// the shell knows whether the pass actually completed: a state machine that
// returns an error, or that asks to come back for child cleanup, has not
// finished with the current Spec and must not claim it has. Returning true
// means the object was modified and needs to be persisted.
type ObservedGenerationRecorder interface {
	SetObservedGeneration(obj client.Object, generation int64) bool
}

// ErrRequeueAfterChildCleanup asks the generic reconciler to retry after a
// child resource has had time to finish stopping or deletion.
//
// It stays a sentinel error, rather than becoming a Result value, because it is
// returned from deep inside the Job and Workflow state machines, where the
// intermediate frames only forward errors. The shell is responsible for turning
// it back into an explicit classification.
var ErrRequeueAfterChildCleanup = errors.New("requeue after child cleanup")

const ChildCleanupRequeue = time.Second

// Shell is the single entry point every control-plane controller funnels its
// reconcile pass through. It owns the three concerns that were previously
// duplicated (or simply missing) per controller: turning a retry class into a
// concrete jittered interval, recording the pass, and emitting events.
//
// The zero value is usable and silent: no metrics, no events, platform default
// intervals without jitter. Production wiring goes through NewShell so that
// jitter is always on.
type Shell struct {
	Backoff  sharedreconcile.Backoff
	Metrics  *Metrics
	Recorder record.EventRecorder
}

// NewShell returns a Shell with the platform default backoff, including jitter.
func NewShell(metrics *Metrics, recorder record.EventRecorder) Shell {
	return Shell{
		Backoff:  sharedreconcile.DefaultBackoff(),
		Metrics:  metrics,
		Recorder: recorder,
	}
}

// Run executes one reconcile pass and translates its classification into a
// controller-runtime result.
//
// An error is still returned to controller-runtime unchanged, so that a failing
// pass keeps the workqueue's exponential rate limiting instead of being
// re-driven at a fixed interval. The Result returned alongside an error is used
// only to classify the failure for metrics.
func (s Shell) Run(
	ctx context.Context,
	resourceKind string,
	pass func(ctx context.Context) (Result, error),
) (ctrl.Result, error) {
	started := time.Now()
	result, err := pass(ctx)
	s.Metrics.observe(resourceKind, result, err, time.Since(started))
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: s.Backoff.Interval(result.Retry)}, nil
}

// RunStateMachine runs a Get / state-machine / status-write pass.
func (s Shell) RunStateMachine(
	ctx context.Context,
	req ctrl.Request,
	obj client.Object,
	resourceKind string,
	r Reconciler,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(resourceKind, req.NamespacedName)
	ctx = log.WithLogger(ctx, logger)

	return s.Run(ctx, resourceKind, func(ctx context.Context) (Result, error) {
		if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return Result{
					Outcome: sharedreconcile.OutcomeDropped,
					Reason:  ReasonObjectGone,
					Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
				}, nil
			}
			return Result{Reason: ReasonReconcileFailed}, err
		}

		if r.IsTerminal(obj) {
			return Result{
				Outcome: sharedreconcile.OutcomeSucceeded,
				Reason:  ReasonTerminal,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}

		// Snapshot before the state machine edits the object in place, so the
		// status write can be a patch of exactly what this pass changed rather
		// than a full-object update that would clobber fields owned by other
		// writers.
		before := obj.DeepCopyObject().(client.Object)

		changed, err := r.ReconcileStateMachine(ctx, obj)
		awaitingCleanup := errors.Is(err, ErrRequeueAfterChildCleanup)
		if err != nil && !awaitingCleanup {
			logger.Error(err, "reconcile failed")
			s.Warnf(obj, ReasonReconcileFailed, "Reconcile failed: %v", err)
			return Result{Reason: ReasonReconcileFailed}, err
		}

		if !awaitingCleanup {
			if recorder, ok := r.(ObservedGenerationRecorder); ok {
				if recorder.SetObservedGeneration(obj, obj.GetGeneration()) {
					changed = true
				}
			}
		}

		if changed {
			if writeErr := r.Status().Patch(ctx, obj, client.MergeFrom(before)); writeErr != nil {
				if apierrors.IsConflict(writeErr) {
					logger.V(1).Info("status patch conflict", "error", writeErr.Error())
					return Result{Reason: ReasonStatusWriteConflict}, writeErr
				}
				s.Warnf(obj, ReasonStatusWriteFailed, "Status write failed: %v", writeErr)
				return Result{Reason: ReasonStatusWriteFailed}, writeErr
			}
		}

		if awaitingCleanup {
			return Result{
				Outcome: sharedreconcile.OutcomeCleanupInProgress,
				Reason:  ReasonChildCleanupInProgress,
				Retry: sharedreconcile.RetryDirective{
					Class: sharedreconcile.RetryInProgress,
					After: ChildCleanupRequeue,
				},
			}, nil
		}

		return Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  ReasonSynced,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	})
}

// Warnf emits a Warning event. It is a no-op without a recorder so that a
// controller under unit test does not need one.
func (s Shell) Warnf(obj runtime.Object, reason, format string, args ...any) {
	s.eventf(obj, corev1.EventTypeWarning, reason, format, args...)
}

// Eventf emits a Normal event.
func (s Shell) Eventf(obj runtime.Object, reason, format string, args ...any) {
	s.eventf(obj, corev1.EventTypeNormal, reason, format, args...)
}

func (s Shell) eventf(obj runtime.Object, eventType, reason, format string, args ...any) {
	if s.Recorder == nil || obj == nil {
		return
	}
	s.Recorder.Eventf(obj, eventType, reason, format, args...)
}
