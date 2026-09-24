package node

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

const podNodeNameField = "spec.nodeName"

// Controller manages node reporting from data-plane to management cluster.
type Controller struct {
	base.Controller
}

var _ base.Reconciler = (*Controller)(nil)

// NewNodeController creates a new NodeController.
func NewNodeController(bc base.Controller) *Controller {
	nc := &Controller{
		Controller: bc,
	}
	nc.C = nc
	return nc
}

// IndexLocalFields registers cache indexes used by node reconcilers.
func IndexLocalFields(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, podNodeNameField, func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	})
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "node",
		Type: &rlarkv1alpha1.Node{},
	}
}

// AsPullReconciler is an exported method.
func (c *Controller) AsPullReconciler() base.KubernetesReconciler {
	return &pullNodeReconciler{c: c}
}

// AsKubePushReconcilers is an exported method.
func (c *Controller) AsKubePushReconcilers() map[base.KubernetesResource]base.KubernetesReconciler {
	return map[base.KubernetesResource]base.KubernetesReconciler{
		{
			Name: "node-k8snode",
			Type: &corev1.Node{},
		}: &pushNodeReconciler{c: c},
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
