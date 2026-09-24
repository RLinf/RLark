package controller

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// Reconciler reconciles resources.
type Reconciler interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	Status() client.StatusWriter
	ReconcileStateMachine(ctx context.Context, obj client.Object) (bool, error)
	IsTerminal(obj client.Object) bool
}

// ErrRequeueAfterChildCleanup asks the generic reconciler to retry after a
// child resource has had time to finish stopping or deletion.
var ErrRequeueAfterChildCleanup = errors.New("requeue after child cleanup")

const ChildCleanupRequeue = time.Second

// ReconcileWith reconciles the resource.
func ReconcileWith(
	ctx context.Context,
	req ctrl.Request,
	obj client.Object,
	resourceKind string,
	r Reconciler,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(resourceKind, req.NamespacedName)

	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if r.IsTerminal(obj) {
		return ctrl.Result{}, nil
	}

	ctx = log.WithLogger(ctx, logger)

	changed, err := r.ReconcileStateMachine(ctx, obj)
	if err != nil {
		if errors.Is(err, ErrRequeueAfterChildCleanup) {
			if changed {
				if err := r.Status().Update(ctx, obj); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: ChildCleanupRequeue}, nil
		}
		logger.Error(err, "reconcile failed")
		return ctrl.Result{}, err
	}

	if changed {
		if err := r.Status().Update(ctx, obj); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("status update conflict", "error", err.Error())
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
