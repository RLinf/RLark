package sync

import (
	"github.com/uptrace/bun"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// newWorkflowSyncHandler creates a new sync handler for Workflow resources.
func newWorkflowSyncHandler() Handler {
	return &genericSyncHandler{
		tableName:    "workflows",
		resourceType: "workflows.rlinf.io",
		isNamespaced: false, // Workflow is cluster-scoped
		wrapBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.WorkflowModel{BaseResourceModel: base}
		},
		wrapLatestBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.LatestWorkflowModel{BaseResourceModel: base}
		},
	}
}

// WorkflowReconciler reconciles Workflow resources.
type WorkflowReconciler struct {
	config Config
	*genericReconciler[*rlarkv1alpha1.Workflow]
}

// +kubebuilder:rbac:groups=rlinf.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=workflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=workflows/finalizers,verbs=update

// NewWorkflowReconciler creates a new WorkflowReconciler.
func NewWorkflowReconciler(config Config, client client.Client, db *bun.DB) *WorkflowReconciler {
	return &WorkflowReconciler{
		config: config,
		genericReconciler: &genericReconciler[*rlarkv1alpha1.Workflow]{
			client:  client,
			db:      db,
			handler: newWorkflowSyncHandler(),
			newObj:  func() *rlarkv1alpha1.Workflow { return &rlarkv1alpha1.Workflow{} },
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Workflow{}).
		WithOptions(r.config.ToControllerOptions()).
		Named("workflow-sync").
		Complete(r)
}
