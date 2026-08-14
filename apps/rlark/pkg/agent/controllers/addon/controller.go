package addon

import (
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// Constants used by the package.
const (
	AddonFinalizer = "rlark.io/agent-cleanup"
)

// Controller manages resources.
type Controller struct {
	base.Controller
}

var _ base.Reconciler = (*Controller)(nil)

// NewAddonController creates a new Controller.
func NewAddonController(bc base.Controller) *Controller {
	ac := &Controller{Controller: bc}
	ac.C = ac
	return ac
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "addon",
		Type: &rlarkv1alpha1.Addon{},
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
