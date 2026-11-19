package base

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// hasManagementTaskAnnotation is a predicate that only lets through K8s objects
// carrying the rlark management-task annotation. It restricts the Pod watch to
// rlark task pods so non-rlark pods in the cluster don't trigger (potentially
// costly) reconciliation or ReplicaSet lookups.
// podStartFailures are the Pod Event reasons that keep a Pod from starting
// without changing it.
var podStartFailures = map[string]bool{"FailedMount": true, "FailedAttachVolume": true, "FailedScheduling": true}

func isPodStartFailure() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		return ok && event.Type == corev1.EventTypeWarning && event.InvolvedObject.Kind == "Pod" && podStartFailures[event.Reason]
	})
}

// enqueueEventPodWorkload maps a Pod's Event to the workload of kind wantKind
// that owns the Pod, if the Pod is a Task's.
func enqueueEventPodWorkload(localClient client.Client, wantKind string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return nil
		}
		var pod corev1.Pod
		if err := localClient.Get(ctx, types.NamespacedName{Namespace: event.InvolvedObject.Namespace, Name: event.InvolvedObject.Name}, &pod); err != nil ||
			pod.UID != event.InvolvedObject.UID || pod.Annotations[managementTaskNameAnnotation] == "" {
			return nil
		}
		return podOwnerRequests(ctx, localClient, &pod, wantKind)
	})
}
