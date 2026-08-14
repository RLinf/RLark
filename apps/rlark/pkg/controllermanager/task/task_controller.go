package task

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Reconciler reconciles Task resources.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/finalizers,verbs=update

// Reconcile handles a Task reconciliation request.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName)
	logger.Info("Reconciling Task")

	var task rlarkv1alpha1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		logger.Error(err, "unable to fetch Task")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info(fmt.Sprintf("Task %s/%s agentType=%s phase=%s",
		task.Namespace, task.Name, task.Spec.AgentType, task.Status.Phase))

	// TODO: implement reconciliation logic per agent type:
	// - AgentTypeKubernetes: reconcile Deployment/DaemonSet/StatefulSet/CloneSet
	// - AgentTypeDocker: manage Docker containers
	// - AgentTypeRaw: run raw artifacts
	// - track node assignment via status.ObservedNodes

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Task{}).
		Named("task").
		Complete(r)
}
