package kubernetes

import (
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// BuildDaemonSet compiles a per-node Task spec into a DaemonSet: one Pod on
// every node its template's node selector matches. The DaemonSet controller
// replaces nothing by itself (OnDelete): the cluster Agent deletes an outdated
// Pod as the rollout policy says (package rollout), and the DaemonSet
// controller then starts the new one, so a node never runs two instances of
// the Task at once.
func BuildDaemonSet(task *rlarkv1alpha1.Task, namespace string, spec *rlarkv1alpha1.TaskWorkloadSpec, policy rlarkv1alpha1.RolloutPolicy) *appsv1.DaemonSet {
	result := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        task.Name,
			Namespace:   namespace,
			Annotations: WorkloadAnnotations(task),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: WorkloadSelector(task),
			Template: *spec.Template.DeepCopy(),
		},
	}
	rollout.Roll(result, policy)
	tenant.Inherit(task, result)
	return result
}
