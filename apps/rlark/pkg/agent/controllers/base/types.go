package base

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// KubernetesResource is an exported type.
type KubernetesResource struct {
	Name string
	Type client.Object
}

// KubernetesReconciler reconciles resources.
type KubernetesReconciler reconcile.TypedReconciler[reconcile.Request]

// DockerResource is an exported type.
type DockerResource struct {
	// TODO
}

// DockerRequest represents a request.
type DockerRequest struct {
	// TODO
}

// DockerReconciler reconciles resources.
type DockerReconciler reconcile.TypedReconciler[DockerRequest]

// DockerControllerManager manages resources.
type DockerControllerManager interface {
	// TODO
	SetupReconciler(reconciler DockerReconciler) error
}

// RawResource is an exported type.
type RawResource struct {
	// TODO
}

// RawRequest represents a request.
type RawRequest struct {
	// TODO
}

// RawReconciler reconciles resources.
type RawReconciler reconcile.TypedReconciler[RawRequest]

// RawControllerManager manages resources.
type RawControllerManager interface {
	// TODO
	SetupReconciler(reconciler RawReconciler) error
}
