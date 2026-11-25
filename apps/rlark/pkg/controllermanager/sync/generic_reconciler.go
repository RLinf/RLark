package sync

import (
	"context"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/persistence"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// Reason codes this reconciler attaches to a classified pass. Closed set: they
// are used as a metric label.
const (
	reasonSyncNotRequired   = "SyncNotRequired"
	reasonSnapshotPersisted = "SnapshotPersisted"
	reasonPersistFailed     = "SnapshotPersistFailed"
	reasonFinalizerWrite    = "FinalizerWriteFailed"
	reasonFinalizerReleased = "SyncFinalizerReleased"
)

type genericReconciler[T client.Object] struct {
	client  client.Client
	backend persistence.Backend // 归档存储后端（PG 或 Secret）
	handler Handler
	newObj  func() T
	syncFn  func(context.Context, T) error
	// kind labels the recorded pass. Empty means the pass is still recorded,
	// just under an empty controller label, so a test may leave it unset.
	kind string
	// shell classifies the pass and records it. The zero value is silent.
	shell controller.Shell
}

func (r *genericReconciler[T]) saveToBackend(ctx context.Context, obj T) error {
	m, err := r.handler.ToPersistedLastestModelObject(obj)
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	return r.backend.Save(ctx, m)
}

func (r *genericReconciler[T]) syncResource(ctx context.Context, obj T) error {
	return r.saveToBackend(ctx, obj)
}

// handleFinalizer releases SyncFinalizer once this controller's own cleanup
// obligation is discharged. That obligation is persisting the snapshot, which
// the caller has already done before reaching here.
//
// A finalizer must decide on its own presence and its own completion only.
// Waiting for a position in the finalizer list couples unrelated controllers:
// with jobs.rlinf.io/task-cleanup, workflows.rlinf.io/job-cleanup,
// rlark.io/agent-cleanup and domains.rlinf.io/peer-cleanup all able to coexist
// on one object, "am I the last one left" turns deletion into a function of
// array order and makes two such waiters deadlock each other forever.
func (r *genericReconciler[T]) handleFinalizer(ctx context.Context, obj T) error {
	return r.removeFinalizer(ctx, obj)
}

func (r *genericReconciler[T]) ensureFinalizer(ctx context.Context, obj T) error {
	if slices.Contains(obj.GetFinalizers(), SyncFinalizer) {
		return nil
	}
	obj.SetFinalizers(append(obj.GetFinalizers(), SyncFinalizer))
	return r.client.Update(ctx, obj)
}

func (r *genericReconciler[T]) removeFinalizer(ctx context.Context, obj T) error {
	if !slices.Contains(obj.GetFinalizers(), SyncFinalizer) {
		return nil
	}
	finalizers := slices.Clone(obj.GetFinalizers())
	obj.SetFinalizers(slices.DeleteFunc(finalizers, func(finalizer string) bool {
		return finalizer == SyncFinalizer
	}))
	return r.client.Update(ctx, obj)
}

// Reconcile reconciles the resource.
func (r *genericReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.shell.Run(ctx, r.kind, func(ctx context.Context) (controller.Result, error) {
		return r.reconcile(ctx, req)
	})
}

func (r *genericReconciler[T]) reconcile(ctx context.Context, req ctrl.Request) (controller.Result, error) {
	obj := r.newObj()
	if err := r.client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return controller.Result{
				Outcome: sharedreconcile.OutcomeDropped,
				Reason:  controller.ReasonObjectGone,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}
	if r.handler != nil && !r.handler.ShouldSyncObject(obj) {
		if err := r.removeFinalizer(ctx, obj); err != nil {
			return finalizerFailure(err), err
		}
		return controller.Result{
			Outcome: sharedreconcile.OutcomeDropped,
			Reason:  reasonSyncNotRequired,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}
	syncResource := r.syncResource
	if r.syncFn != nil {
		syncResource = r.syncFn
	}
	if err := syncResource(ctx, obj); err != nil {
		// The backend is the only external effect here and it is retried
		// through the workqueue, so the finalizer must stay until the snapshot
		// is actually persisted.
		return controller.Result{Reason: reasonPersistFailed}, err
	}
	if obj.GetDeletionTimestamp() == nil {
		if err := r.ensureFinalizer(ctx, obj); err != nil {
			return finalizerFailure(err), err
		}
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  reasonSnapshotPersisted,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange},
		}, nil
	}

	if err := r.handleFinalizer(ctx, obj); err != nil {
		return finalizerFailure(err), err
	}
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  reasonFinalizerReleased,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
	}, nil
}

// finalizerFailure classifies a failed finalizer write. A conflict is the
// expected outcome of racing another writer on the same metadata and is kept
// distinct from a genuine write failure.
func finalizerFailure(err error) controller.Result {
	if apierrors.IsConflict(err) {
		return controller.Result{Reason: controller.ReasonStatusWriteConflict}
	}
	return controller.Result{Reason: reasonFinalizerWrite}
}
