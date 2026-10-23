package kubernetes_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
)

type operations struct {
	prepared bool
	applied  string
}

func (o *operations) PrepareTemplate(*corev1.PodTemplateSpec, *rlarkv1alpha1.Task) { o.prepared = true }
func (*operations) UpdateRunningTask(context.Context, *rlarkv1alpha1.Task) error   { return nil }
func (o *operations) ApplyWorkload(_ context.Context, _ *rlarkv1alpha1.Task, kind string) (reconcile.Result, error) {
	o.applied = kind
	return reconcile.Result{}, nil
}

type adapter interface {
	Prepare(*rlarkv1alpha1.Task)
	Apply(context.Context, *rlarkv1alpha1.Task) (reconcile.Result, error)
}

func TestBuiltinAdaptersKeepTheirOwnWorkloadKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
		new  func(*operations) adapter
	}{
		{name: "deployment", kind: raykubernetes.CarrierDeployment, new: func(o *operations) adapter {
			return raykubernetes.NewLegacyAdapter(raykubernetes.CarrierDeployment, o)
		}},
		{name: "daemonset", kind: raykubernetes.CarrierDaemonSet, new: func(o *operations) adapter {
			return raykubernetes.NewLegacyAdapter(raykubernetes.CarrierDaemonSet, o)
		}},
		{name: "statefulset", kind: raykubernetes.CarrierStatefulSet, new: func(o *operations) adapter {
			return raykubernetes.NewLegacyAdapter(raykubernetes.CarrierStatefulSet, o)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &operations{}
			workload := tt.new(o)
			task := &rlarkv1alpha1.Task{Spec: rlarkv1alpha1.TaskSpec{Workload: rlarkv1alpha1.TaskWorkloadSpec{Template: &corev1.PodTemplateSpec{}}}}
			workload.Prepare(task)
			if _, err := workload.Apply(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if !o.prepared || o.applied != tt.kind {
				t.Fatalf("prepared=%v applied=%q, want %q", o.prepared, o.applied, tt.kind)
			}
		})
	}
}
