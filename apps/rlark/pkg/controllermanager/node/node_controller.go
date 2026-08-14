package node

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Reconciler reconciles Node resources (cluster-scoped).
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/finalizers,verbs=update

// Reconcile handles a Node reconciliation request.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)
	logger.Info("Reconciling Node")

	var node rlarkv1alpha1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		logger.Error(err, "unable to fetch Node")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info(fmt.Sprintf("Node %s agentType=%s phase=%s",
		node.Name, node.Spec.AgentType, node.Status.Phase))

	// TODO: implement reconciliation logic
	// - heartbeat / health check from agent
	// - update capacity / allocatable / used resources
	// - handle Online / Offline transitions

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Node{}).
		Named("node").
		Complete(r)
}
