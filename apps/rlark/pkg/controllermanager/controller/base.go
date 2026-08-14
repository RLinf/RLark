package controller

import (
	"context"
	"time"

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
		logger.Error(err, "reconcile failed")
		return ctrl.Result{}, err
	}

	if changed {
		if err := r.Status().Update(ctx, obj); err != nil {
			logger.V(1).Info("status update conflict, requeuing", "error", err.Error())
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	return ctrl.Result{}, nil
}
