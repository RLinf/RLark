package task

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// Constants used by the package.
const (
	ManagementTaskNameAnnotation              = "rlark.io/management-task-name"
	ManagementTaskNamespaceAnnotation         = "rlark.io/management-task-namespace"
	ManagementTaskGenerationAnnotation        = "rlark.io/management-task-generation"
	ManagementTaskAdoptedGenerationAnnotation = "rlark.io/management-task-adopted-generation"
	ManagementTaskUIDAnnotation               = "rlark.io/management-task-uid"
	ManagementTaskDomainAnnotation            = "rlark.io/management-task-domain"
	ManagementTaskUIDLabel                    = "rlark.io/management-task-uid"
	CleanupRequeueInterval                    = 2 * time.Second
	// taskReconcileLogInterval limits identical in-progress summaries while
	// preserving an occasional signal that cleanup or migration is still active.
	taskReconcileLogInterval = time.Minute
	UnsafeTaskPrivilegesEnv  = "RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES"
)

// Compatibility aliases preserve the Task package names while Host remains
// the canonical owner of claim metadata and cleanup finalization.
const (
	ManagementTaskClaimantAnnotation = host.TaskClaimantAnnotation
	ManagementTaskFinalizer          = host.TaskFinalizer
)

// pullReconciler watches management Tasks and creates workloads on local cluster.
type pullReconciler struct {
	c *Controller
}

// Reconcile delegates Task lifecycle ownership to the Runtime-neutral Host.
func (r *pullReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcileResult reconcile.Result, finalErr error) {
	if r.c.host == nil {
		return reconcile.Result{}, fmt.Errorf("task Host is not configured")
	}
	key := trigger.Key{Namespace: req.Namespace, Name: req.Name}
	hints := trigger.Set{Key: key, FullReconcileRequired: true}
	if r.c.triggers != nil {
		if pending, ok := r.c.triggers.accumulator.Take(key); ok {
			hints = pending
		}
		defer func() {
			if r.c.triggers.finish(key) {
				// Trigger-channel pressure is queue state, not a failure of the
				// completed Task reconciliation. Retry this key immediately so its
				// retained pending hints receive another wakeup opportunity. A
				// positive minimum delay uses controller-runtime's non-deprecated
				// requeue path without introducing rate-limited error backoff.
				reconcileResult.RequeueAfter = time.Nanosecond
			}
		}()
	}
	result, err := r.c.dispatch(ctx, req.NamespacedName, hints)
	if err != nil {
		// controller-runtime owns error logging. Clear only the successful-result
		// suppression state so the next recovery summary is emitted immediately.
		r.c.clearTaskReconcileLog(req.String())
		return reconcile.Result{}, err
	}
	requeueAfter := result.Retry.After
	if result.Retry.Class == host.RetryInProgress && requeueAfter <= 0 {
		requeueAfter = CleanupRequeueInterval
	}
	if r.c.shouldLogTaskReconcile(req.String(), result, time.Now()) {
		log.FromContext(ctx).Info("Task reconciliation completed",
			"task", req.NamespacedName,
			"outcome", result.Outcome,
			"retryClass", result.Retry.Class,
			"retryAfter", requeueAfter,
			"reason", result.Reason,
		)
	}
	if requeueAfter > 0 {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}
	return reconcile.Result{}, nil
}

func getWorkloadNamespace(mgmtTask *rlarkv1alpha1.Task) string {
	provider := tenant.StaticProvider{}
	namespace, err := provider.ResolveNamespace(context.Background(), tenant.IDOf(mgmtTask), "")
	if err != nil {
		return tenant.DefaultNamespace
	}
	return namespace
}
