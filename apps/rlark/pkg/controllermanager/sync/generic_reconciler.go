package sync

import (
	"context"
	"slices"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/persistence"
)

type genericReconciler[T client.Object] struct {
	client  client.Client
	backend persistence.Backend // 归档存储后端（PG 或 Secret）
	handler Handler
	newObj  func() T
	syncFn  func(context.Context, T) error
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

func (r *genericReconciler[T]) handleFinalizer(ctx context.Context, obj T) error {
	// 仅当 Finalizer 列表中仅有 SyncFinalizer 时才执行删除逻辑，确保 SyncFinalizer 是最后一个被移除的 Finalizer
	if len(obj.GetFinalizers()) != 1 {
		return nil
	}
	if obj.GetFinalizers()[0] != SyncFinalizer {
		return nil
	}
	obj.SetFinalizers([]string{})
	return r.client.Update(ctx, obj)
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
	obj := r.newObj()
	if err := r.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.handler != nil && !r.handler.ShouldSyncObject(obj) {
		return ctrl.Result{}, r.removeFinalizer(ctx, obj)
	}
	syncResource := r.syncResource
	if r.syncFn != nil {
		syncResource = r.syncFn
	}
	if err := syncResource(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.GetDeletionTimestamp() == nil {
		if err := r.ensureFinalizer(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.handleFinalizer(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
