package base

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler reconciles resources.
type Reconciler interface {
	KubernetesResource() KubernetesResource
	AsPullReconciler() KubernetesReconciler
	AsKubePushReconcilers() map[KubernetesResource]KubernetesReconciler
	AsDockerPushReconcilers() map[DockerResource]DockerReconciler
	AsRawPushReconcilers() map[RawResource]RawReconciler
}

// Controller manages resources.
type Controller struct {
	ManagementClient    client.Client
	ManagementNamespace string
	AgentType           string // Kubernetes/Docker/Raw

	LocalKubeClient   client.Client
	LocalDockerClient any // TODO
	LocalRawClient    any // TODO

	NetworkSidecarImage string

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
	return ctrl.NewControllerManagedBy(mgr).
		For(kubeResource.Type).
		Named(kubeResource.Name + "-pull").
		Complete(pullReconciler)
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
			err := ctrl.NewControllerManagedBy(kubeMgr).
				For(kubeResource.Type).
				Named(kubeResource.Name + "-push").
				Complete(reconciler)
			if err != nil {
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
