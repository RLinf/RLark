package task

import (
	appsv1 "k8s.io/api/apps/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// Controller manages resources.
type Controller struct {
	base.Controller
}

var _ base.Reconciler = (*Controller)(nil)

// NewTaskController creates a new Controller.
func NewTaskController(bc base.Controller) *Controller {
	tc := &Controller{
		Controller: bc,
	}
	tc.C = tc
	return tc
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "task",
		Type: &rlarkv1alpha1.Task{},
	}
}

// AsPullReconciler is an exported method.
func (c *Controller) AsPullReconciler() base.KubernetesReconciler {
	return &pullReconciler{c: c}
}

// AsKubePushReconcilers is an exported method.
func (c *Controller) AsKubePushReconcilers() map[base.KubernetesResource]base.KubernetesReconciler {
	return map[base.KubernetesResource]base.KubernetesReconciler{
		base.KubernetesResource{
			Name: "task-deployment",
			Type: &appsv1.Deployment{},
		}: &pushDeploymentReconciler{c: c},
		base.KubernetesResource{
			Name: "task-daemonset",
			Type: &appsv1.DaemonSet{},
		}: &pushDaemonSetReconciler{c: c},
		base.KubernetesResource{
			Name: "task-statefulset",
			Type: &appsv1.StatefulSet{},
		}: &pushStatefulSetReconciler{c: c},
	}
}

// AsDockerPushReconcilers is an exported method.
func (c *Controller) AsDockerPushReconcilers() map[base.DockerResource]base.DockerReconciler {
	return nil
}

// AsRawPushReconcilers is an exported method.
func (c *Controller) AsRawPushReconcilers() map[base.RawResource]base.RawReconciler {
	return nil
}
