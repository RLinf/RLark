package task

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// pushRuntimeReconciler converts a trusted registered workload-root change into
// a Task reconciliation hint. It never writes Runtime objects or Task Status.
type pushRuntimeReconciler struct {
	c              *Controller
	observation    kubernetesObservation
	scope          KubernetesObservationScope
	observedReader client.Reader
}

func newPushRuntimeReconciler(c *Controller, observation kubernetesObservation, scope KubernetesObservationScope, observedReader client.Reader) (*pushRuntimeReconciler, error) {
	if err := scope.validate(); err != nil {
		return nil, fmt.Errorf("construct runtime push reconciler: %w", err)
	}
	if observedReader == nil {
		return nil, fmt.Errorf("observation reader is required")
	}
	return &pushRuntimeReconciler{c: c, observation: observation, scope: scope, observedReader: observedReader}, nil
}

func (r *pushRuntimeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("runtimeRoot", req.NamespacedName, "gvk", r.observation.GVK.String())
	object := r.observation.newObject()
	if req.Namespace != r.scope.Namespace {
		return reconcile.Result{}, nil
	}
	if err := r.observedReader.Get(ctx, req.NamespacedName, object); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		logger.V(1).Info("Runtime root not found")
		return reconcile.Result{}, nil
	}
	return r.enqueueRoot(ctx, object)
}

// enqueueRoot consumes an already-read root; the watcher never writes Status.
func (r *pushRuntimeReconciler) enqueueRoot(ctx context.Context, object client.Object) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	taskName := object.GetAnnotations()[ManagementTaskNameAnnotation]
	taskNamespace := object.GetAnnotations()[ManagementTaskNamespaceAnnotation]
	if taskName == "" || taskNamespace == "" {
		logger.V(1).Info("Runtime root has no management Task identity, skipping")
		return reconcile.Result{}, nil
	}
	if !r.scope.allowsRoot(object) {
		return reconcile.Result{}, nil
	}
	var task rlarkv1alpha1.Task
	if err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &task); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		logger.V(1).Info("Management Task not found")
		return reconcile.Result{}, nil
	}
	if !r.scope.allowsTask(&task) || string(task.Spec.Runtime.Kind) != r.c.AgentType || task.Spec.Workload.Carrier.Type != r.observation.CarrierType || kubernetesobjects.OwnershipForTask(object, &task) == kubernetesobjects.OwnershipConflict {
		logger.V(1).Info("Runtime root does not own the current Task execution, skipping")
		return reconcile.Result{}, nil
	}
	root := agentruntime.ObjectRef{RuntimeID: r.scope.RuntimeID, ID: r.observation.GVK.String(), Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID()), LogicalTask: agentruntime.LogicalTaskID{Namespace: task.Namespace, Name: task.Name}}
	if err := r.c.emitRuntimeTrigger(ctx, &task, root); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
