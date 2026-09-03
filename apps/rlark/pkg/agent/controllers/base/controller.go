package base

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// managementTaskNameAnnotation marks K8s workloads (and their pods) that back a
// management Task. It mirrors task.ManagementTaskNameAnnotation; duplicated here
// to avoid an import cycle (the task package imports base).
const managementTaskNameAnnotation = "rlark.io/management-task-name"

// podOwningKind returns the Kubernetes Kind of pod-owning workloads
// (Deployment/StatefulSet/DaemonSet) for the given push-reconciler object type,
// or "" if the object is not a pod-owning workload.
func podOwningKind(obj client.Object) string {
	switch obj.(type) {
	case *appsv1.Deployment:
		return "Deployment"
	case *appsv1.StatefulSet:
		return "StatefulSet"
	case *appsv1.DaemonSet:
		return "DaemonSet"
	}
	return ""
}

// enqueueOwningWorkload maps a Pod change to the reconcile request of the
// workload that owns it (Deployment/StatefulSet/DaemonSet). StatefulSet and
// DaemonSet pods reference their owning workload directly via OwnerReferences;
// Deployment pods are owned by a ReplicaSet which is in turn owned by the
// Deployment, so the ReplicaSet is resolved to find the Deployment.
//
// Pod container status transitions (e.g. a container entering CrashLoopBackOff)
// are not reflected in the workload's own status fields (restart counts and
// waiting reasons live on the Pod, not on the StatefulSet/Deployment status).
// Without watching Pods, a workload whose status has stabilized — e.g. a
// StatefulSet whose readyReplicas never reached desired because its pod crashed
// on the first start — would never be re-reconciled, so the failure stays
// hidden and the Task status is never updated.
func enqueueOwningWorkload(localClient client.Client, wantKind string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		return podOwnerRequests(ctx, localClient, pod, wantKind)
	})
}

// podOwnerRequests resolves the workload (of kind wantKind) that owns pod and
// returns the reconcile request for it. StatefulSet/DaemonSet pods reference
// their owning workload directly; Deployment pods are owned by a ReplicaSet
// which is resolved to find the owning Deployment.
func podOwnerRequests(ctx context.Context, localClient client.Client, pod *corev1.Pod, wantKind string) []reconcile.Request {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return nil
	}
	// Direct ownership (StatefulSet/DaemonSet pods).
	if owner.Kind == wantKind {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}}}
	}
	// Indirect ownership: Pod -> ReplicaSet -> Deployment.
	if owner.Kind == "ReplicaSet" && wantKind == "Deployment" {
		var rs appsv1.ReplicaSet
		if err := localClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}, &rs); err != nil {
			return nil
		}
		if depOwner := metav1.GetControllerOf(&rs); depOwner != nil && depOwner.Kind == wantKind {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: depOwner.Name, Namespace: rs.Namespace}}}
		}
	}
	return nil
}

// hasManagementTaskAnnotation is a predicate that only lets through K8s objects
// carrying the rlark management-task annotation. It restricts the Pod watch to
// rlark task pods so non-rlark pods in the cluster don't trigger (potentially
// costly) reconciliation or ReplicaSet lookups.
func hasManagementTaskAnnotation() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetAnnotations()[managementTaskNameAnnotation] != ""
	})
}

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

	Image string

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
			blder := ctrl.NewControllerManagedBy(kubeMgr).
				For(kubeResource.Type).
				Named(kubeResource.Name + "-push")

			// For pod-owning workloads (Deployment/StatefulSet/DaemonSet),
			// also watch Pods so that container status changes — most
			// importantly a container entering CrashLoopBackOff — re-trigger
			// reconciliation. The workload's own status does not reflect
			// per-container waiting/restart state, so without this a pod that
			// crashes after the workload status stabilized is never detected
			// and the Task keeps reporting a stale phase.
			if kind := podOwningKind(kubeResource.Type); kind != "" {
				blder = blder.Watches(
					&corev1.Pod{},
					enqueueOwningWorkload(c.LocalKubeClient, kind),
					builder.WithPredicates(hasManagementTaskAnnotation()),
				)
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
