// Package clusterupgradejob provides the data plane controller for ClusterUpgradeJob.
// It watches ClusterUpgradeJob CRs and creates upgrade Jobs on the local cluster.
package clusterupgradejob

import (
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// Controller manages resources.
type Controller struct {
	base.Controller
}

var _ base.Reconciler = (*Controller)(nil)

// NewClusterUpgradeJobController creates a new Controller.
func NewClusterUpgradeJobController(bc base.Controller) *Controller {
	uc := &Controller{Controller: bc}
	uc.C = uc
	return uc
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "clusterupgradejob",
		Type: &rlarkv1alpha1.ClusterUpgradeJob{},
	}
}

// AsPullReconciler is an exported method.
func (c *Controller) AsPullReconciler() base.KubernetesReconciler {
	return &pullReconciler{c: c}
}

// AsKubePushReconcilers is an exported method.
func (c *Controller) AsKubePushReconcilers() map[base.KubernetesResource]base.KubernetesReconciler {
	return nil
}

// AsDockerPushReconcilers is an exported method.
func (c *Controller) AsDockerPushReconcilers() map[base.DockerResource]base.DockerReconciler {
	return nil
}

// AsRawPushReconcilers is an exported method.
func (c *Controller) AsRawPushReconcilers() map[base.RawResource]base.RawReconciler {
	return nil
}
