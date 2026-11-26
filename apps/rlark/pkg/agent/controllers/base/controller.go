package base

import (
	"fmt"
	"net/http"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// Reconciler reconciles resources.
type Reconciler interface {
	KubernetesResource() KubernetesResource
	AsPullReconciler() KubernetesReconciler
	AsKubePushReconcilers() map[KubernetesResource]KubernetesReconciler
	AsDockerPushReconcilers() map[DockerResource]DockerReconciler
	AsRawPushReconcilers() map[RawResource]RawReconciler
}

type pushWatchProvider interface {
	PushWatch(client.Object) handler.EventHandler
}

// Controller manages resources.
type Controller struct {
	ManagementClient    client.Client
	ManagementNamespace string
	AgentType           string // Kubernetes/Docker/Raw

	LocalKubeClient       client.Client
	LocalKubeCachedClient client.Client
	LocalKubeHTTP         *http.Client
	LocalKubeAPIHost      string
	LocalKubeConfig       *rest.Config
	LocalDockerClient     any // TODO
	LocalRawClient        any // TODO

	Image                       string
	PullMaxConcurrentReconciles int
	PushMaxConcurrentReconciles int

	C Reconciler
}

// SetupPullController sets the upPullController.
func (c *Controller) SetupPullController(mgr ctrl.Manager) error {
	pullReconciler := c.C.AsPullReconciler()
	if pullReconciler == nil {
		// Skip setup — no pull logic needed (e.g. Node controller)
		return nil
	}
	kubeResource := c.C.KubernetesResource()
	blder := ctrl.NewControllerManagedBy(mgr).
		For(kubeResource.Type).
		Named(kubeResource.Name + "-pull")
	if c.PullMaxConcurrentReconciles > 0 {
		blder = blder.WithOptions(controller.Options{MaxConcurrentReconciles: c.PullMaxConcurrentReconciles})
	}
	return blder.Complete(pullReconciler)
}

// SetupPushController sets the upPushController.
func (c *Controller) SetupPushController(mgr any) error {
	if mgr == nil {
		return fmt.Errorf("manager is nil")
	}

	if kubeMgr, ok := mgr.(ctrl.Manager); ok {
		if c.LocalKubeClient == nil {
			return fmt.Errorf("controller is not running in a Kubernetes environment")
		}
		for kubeResource, reconciler := range c.C.AsKubePushReconcilers() {
			blder := ctrl.NewControllerManagedBy(kubeMgr).
				For(kubeResource.Type).
				Named(kubeResource.Name + "-push")
			if workers := c.PushMaxConcurrentReconciles; workers > 0 {
				blder = blder.WithOptions(controller.Options{MaxConcurrentReconciles: workers})
			}
			if provider, ok := c.C.(pushWatchProvider); ok {
				if eventHandler := provider.PushWatch(kubeResource.Type); eventHandler != nil {
					blder = blder.Watches(kubeResource.Type, eventHandler)
				}
			}

			if err := blder.Complete(reconciler); err != nil {
				return fmt.Errorf("failed to setup push controller for %s: %w", kubeResource.Name, err)
			}
		}
		return nil
	}
	if dockerMgr, ok := mgr.(DockerControllerManager); ok {
		if c.LocalDockerClient == nil {
			return fmt.Errorf("controller is not running in a Docker environment")
		}
		for dockerResource, reconciler := range c.C.AsDockerPushReconcilers() {
			err := dockerMgr.SetupReconciler(reconciler)
			if err != nil {
				return fmt.Errorf("failed to setup push controller for %s: %w", dockerResource, err)
			}
		}
		return nil
	}
	if rawMgr, ok := mgr.(RawControllerManager); ok {
		if c.LocalRawClient == nil {
			return fmt.Errorf("controller is not running in a raw environment")
		}
		for rawResource, reconciler := range c.C.AsRawPushReconcilers() {
			err := rawMgr.SetupReconciler(reconciler)
			if err != nil {
				return fmt.Errorf("failed to setup push controller for %s: %w", rawResource, err)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown manager type: %T", mgr)
}
