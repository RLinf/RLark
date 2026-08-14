package sync

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/uptrace/bun"
)

type genericReconciler[T client.Object] struct {
	client  client.Client
	db      *bun.DB
	handler Handler
	newObj  func() T
}

func (r *genericReconciler[T]) saveToDatabase(ctx context.Context, m db.ResourceModel) error {
	_, err := r.db.NewInsert().
		Model(m).
		On("CONFLICT (id) DO UPDATE").
		Set("namespace = EXCLUDED.namespace").
		Set("name = EXCLUDED.name").
		Set("uid = EXCLUDED.uid").
		Set("raw = EXCLUDED.raw").
		Set("deleted_at = EXCLUDED.deleted_at").
		Exec(ctx)
	return err
}

func (r *genericReconciler[T]) syncResource(ctx context.Context, obj T) error {
	m, err := r.handler.ToPersistedModelObject(obj)
	if err != nil {
		return fmt.Errorf("convert into persisted model: %w", err)
	}
	if m != nil {
		if err := r.saveToDatabase(ctx, m); err != nil {
			return fmt.Errorf("save history into database: %w", err)
		}
	}

	latestM, err := r.handler.ToPersistedLastestModelObject(obj)
	if err != nil {
		return fmt.Errorf("convert into persisted latest model: %w", err)
	}
	if latestM != nil {
		if err := r.saveToDatabase(ctx, latestM); err != nil {
			return fmt.Errorf("save latest model into database: %w", err)
		}
	}

	return nil
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

// Reconcile reconciles the resource.
func (r *genericReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.newObj()
	if err := r.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if err := r.syncResource(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.GetDeletionTimestamp() != nil {
		if err := r.handleFinalizer(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
