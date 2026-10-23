package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	CarrierDeployment  = "Deployment"
	CarrierDaemonSet   = "DaemonSet"
	CarrierStatefulSet = "StatefulSet"
)

// Operations is the compatibility port used by the three current Kubernetes
// workload adapters. Its implementation remains in the Task controller while
// object operations are migrated incrementally into the Kubernetes runtime.
type Operations interface {
	PrepareTemplate(*corev1.PodTemplateSpec, *rlarkv1alpha1.Task)
	UpdateRunningTask(context.Context, *rlarkv1alpha1.Task) error
	ApplyWorkload(context.Context, *rlarkv1alpha1.Task, string) (reconcile.Result, error)
}

// Adapter bridges the legacy Task controller operations during migration.
type LegacyAdapter struct {
	kind       string
	operations Operations
}

// New constructs a compatibility Adapter for one Kubernetes workload kind.
func NewLegacyAdapter(kind string, operations Operations) *LegacyAdapter {
	return &LegacyAdapter{kind: kind, operations: operations}
}

// Prepare applies legacy Pod template mutations to task.
func (a *LegacyAdapter) Prepare(task *rlarkv1alpha1.Task) {
	if task.Spec.Workload.Template != nil {
		a.operations.PrepareTemplate(task.Spec.Workload.Template, task)
	}
}

// UpdateRunningTask delegates an update of an already-running legacy Task.
func (a *LegacyAdapter) UpdateRunningTask(ctx context.Context, task *rlarkv1alpha1.Task) error {
	return a.operations.UpdateRunningTask(ctx, task)
}

// Apply delegates legacy workload realization for the Adapter's Kubernetes kind.
func (a *LegacyAdapter) Apply(ctx context.Context, task *rlarkv1alpha1.Task) (reconcile.Result, error) {
	return a.operations.ApplyWorkload(ctx, task, a.kind)
}
