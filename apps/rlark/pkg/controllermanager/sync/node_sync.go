package sync

import (
	"github.com/uptrace/bun"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// newNodeSyncHandler creates a new sync handler for Node resources.
func newNodeSyncHandler() Handler {
	return &genericSyncHandler{
		tableName:    "nodes",
		resourceType: "nodes.rlinf.io",
		isNamespaced: true, // Node is namespace-scoped
		wrapBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.NodeModel{BaseResourceModel: base}
		},
		wrapLatestBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.LatestNodeModel{BaseResourceModel: base}
		},
	}
}

// NodeReconciler reconciles Node resources.
type NodeReconciler struct {
	config Config
	*genericReconciler[*rlarkv1alpha1.Node]
}

// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/finalizers,verbs=update

// NewNodeReconciler creates a new NodeReconciler.
func NewNodeReconciler(config Config, client client.Client, db *bun.DB) *NodeReconciler {
	return &NodeReconciler{
		config: config,
		genericReconciler: &genericReconciler[*rlarkv1alpha1.Node]{
			client:  client,
			db:      db,
			handler: newNodeSyncHandler(),
			newObj:  func() *rlarkv1alpha1.Node { return &rlarkv1alpha1.Node{} },
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Node{}).
		WithOptions(r.config.ToControllerOptions()).
		Named("node-sync").
		Complete(r)
}
