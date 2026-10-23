package archive

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func init() {
	DefaultRegistry.Register(&JobResource{})
}

// JobResource 实现 Job 的归档接口。
type JobResource struct{}

// Kind 返回资源类型名。
func (r *JobResource) Kind() string {
	return "Job"
}

// NewObject 返回一个新的空 Job 对象。
func (r *JobResource) NewObject() client.Object {
	return &rlarkv1alpha1.Job{}
}

// IsTerminal 判断 Job 是否处于终态（仅 Stopped 需要归档）。
func (r *JobResource) IsTerminal(obj client.Object) bool {
	job, ok := obj.(*rlarkv1alpha1.Job)
	if !ok {
		return false
	}
	return job.Status.Phase == rlarkv1alpha1.JobPhaseStopped
}

// GetEndTime 获取 Job 的终态结束时间。
func (r *JobResource) GetEndTime(obj client.Object) *time.Time {
	job, ok := obj.(*rlarkv1alpha1.Job)
	if !ok || job.Status.EndTime == nil {
		return nil
	}
	t := job.Status.EndTime.Time
	return &t
}

// SetupWithManager 注册 Job 归档 controller。
func (r *JobResource) SetupWithManager(mgr ctrl.Manager, reconciler *Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Job{}, builder.WithPredicates(terminalOrDeletingPredicate(r.IsTerminal))).
		Named("job-archive").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: reconciler.MaxConcurrentReconciles}).
		Complete(reconciler)
}
