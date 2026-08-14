package sync

import (
	"github.com/uptrace/bun"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// newTaskSyncHandler creates a new sync handler for Task resources.
func newTaskSyncHandler() Handler {
	return &genericSyncHandler{
		tableName:    "tasks",
		resourceType: "tasks.rlinf.io",
		isNamespaced: true, // Task is namespace-scoped
		wrapBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.TaskModel{BaseResourceModel: base}
		},
		wrapLatestBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.LatestTaskModel{BaseResourceModel: base}
		},
	}
}

// TaskReconciler reconciles Task resources.
type TaskReconciler struct {
	config Config
	*genericReconciler[*rlarkv1alpha1.Task]
}

// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks/finalizers,verbs=update

// NewTaskReconciler creates a new TaskReconciler.
func NewTaskReconciler(config Config, client client.Client, db *bun.DB) *TaskReconciler {
	return &TaskReconciler{
		config: config,
		genericReconciler: &genericReconciler[*rlarkv1alpha1.Task]{
			client:  client,
			db:      db,
			handler: newTaskSyncHandler(),
			newObj:  func() *rlarkv1alpha1.Task { return &rlarkv1alpha1.Task{} },
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Task{}).
		WithOptions(r.config.ToControllerOptions()).
		Named("task-sync").
		Complete(r)
}
