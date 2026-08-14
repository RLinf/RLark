package pod

import (
	corev1 "k8s.io/api/core/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// Controller manages pod reporting from data-plane to management cluster.
type Controller struct {
	base.Controller
}

var _ base.Reconciler = (*Controller)(nil)

// NewPodController creates a new PodController.
func NewPodController(bc base.Controller) *Controller {
	pc := &Controller{
		Controller: bc,
	}
	pc.C = pc
	return pc
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "pod",
		Type: &rlarkv1alpha1.Pod{},
	}
}

// AsPullReconciler returns nil — Pod CR is push-only, no need to create local resources from management Pod CRs.
func (c *Controller) AsPullReconciler() base.KubernetesReconciler {
	return nil
}

// AsKubePushReconcilers watches local K8s Pods and reports their info to management Pod CRs.
func (c *Controller) AsKubePushReconcilers() map[base.KubernetesResource]base.KubernetesReconciler {
	return map[base.KubernetesResource]base.KubernetesReconciler{
		base.KubernetesResource{
			Name: "pod-k8spod",
			Type: &corev1.Pod{},
		}: &pushPodReconciler{c: c},
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
