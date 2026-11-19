package pod

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

// Controller manages pod reporting from data-plane to management cluster.
type Controller struct {
	base.Controller
	deleteMu   sync.Mutex
	deleteUIDs map[types.NamespacedName][]types.UID
}

var _ base.Reconciler = (*Controller)(nil)

// NewPodController creates a new PodController.
func NewPodController(bc base.Controller) *Controller {
	pc := &Controller{
		Controller: bc,
		deleteUIDs: map[types.NamespacedName][]types.UID{},
	}
	pc.C = pc
	return pc
}

func (c *Controller) deleteEventHandler() handler.EventHandler {
	return handler.Funcs{DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		if e.Object == nil || e.Object.GetUID() == "" {
			return
		}
		key := types.NamespacedName{Name: e.Object.GetName(), Namespace: e.Object.GetNamespace()}
		c.deleteMu.Lock()
		c.deleteUIDs[key] = append(c.deleteUIDs[key], e.Object.GetUID())
		c.deleteMu.Unlock()
		q.Add(reconcile.Request{NamespacedName: key})
	}}
}

func (c *Controller) pendingDeleteUIDs(key types.NamespacedName) []types.UID {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	return append([]types.UID(nil), c.deleteUIDs[key]...)
}

func (c *Controller) acknowledgeDeleteUID(key types.NamespacedName, uid types.UID) {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	uids := c.deleteUIDs[key]
	for i, queuedUID := range uids {
		if queuedUID != uid {
			continue
		}
		c.deleteUIDs[key] = append(uids[:i], uids[i+1:]...)
		if len(c.deleteUIDs[key]) == 0 {
			delete(c.deleteUIDs, key)
		}
		return
	}
}

// PushWatch captures the UID carried by delete events before the cache loses it.
func (c *Controller) PushWatch(obj client.Object) handler.EventHandler {
	if _, ok := obj.(*corev1.Pod); !ok {
		return nil
	}
	return c.deleteEventHandler()
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
