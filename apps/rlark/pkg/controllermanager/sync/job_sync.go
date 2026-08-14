package sync

import (
	"github.com/uptrace/bun"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// newJobSyncHandler creates a new sync handler for Job resources.
func newJobSyncHandler() Handler {
	return &genericSyncHandler{
		tableName:    "jobs",
		resourceType: "jobs.rlinf.io",
		isNamespaced: false, // Job is cluster-scoped
		wrapBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.JobModel{BaseResourceModel: base}
		},
		wrapLatestBaseModel: func(base db.BaseResourceModel) db.ResourceModel {
			return &db.LatestJobModel{BaseResourceModel: base}
		},
	}
}

// JobReconciler reconciles Job resources.
type JobReconciler struct {
	config Config
	*genericReconciler[*rlarkv1alpha1.Job]
}

// +kubebuilder:rbac:groups=rlinf.io,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=jobs/finalizers,verbs=update

// NewJobReconciler creates a new JobReconciler.
func NewJobReconciler(config Config, client client.Client, db *bun.DB) *JobReconciler {
	return &JobReconciler{
		config: config,
		genericReconciler: &genericReconciler[*rlarkv1alpha1.Job]{
			client:  client,
			db:      db,
			handler: newJobSyncHandler(),
			newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Job{}).
		WithOptions(r.config.ToControllerOptions()).
		Named("job-sync").
		Complete(r)
}
