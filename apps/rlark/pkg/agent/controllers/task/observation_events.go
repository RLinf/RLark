package task

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

// runtimeObservationRequest retains only bounded identity, never a cached
// object or Status. Delete identity survives tombstones and queue retries.
type runtimeObservationRequest struct {
	Key             types.NamespacedName
	UID             types.UID
	Deleted         bool
	Pod             bool
	TaskName        string
	TaskUID         types.UID
	OwnerName       string
	OwnerUID        types.UID
	OwnerKind       string
	OwnerAPIVersion string
}

func runtimeEventHandler(scope KubernetesObservationScope, pod bool) handler.TypedEventHandler[client.Object, runtimeObservationRequest] {
	add := func(obj client.Object, deleted bool, q workqueue.TypedRateLimitingInterface[runtimeObservationRequest]) {
		if obj == nil || obj.GetNamespace() != scope.Namespace || obj.GetName() == "" || obj.GetUID() == "" {
			return
		}
		// Preserve the legacy Pod watch filter before enqueueing or resolving
		// owners. This is a cost filter, not proof of Task ownership.
		if pod && obj.GetAnnotations()[ManagementTaskNameAnnotation] == "" {
			return
		}
		req := runtimeObservationRequest{Key: client.ObjectKeyFromObject(obj), UID: obj.GetUID(), Deleted: deleted, Pod: pod}
		if deleted {
			if pod {
				owner := metav1.GetControllerOf(obj)
				if owner == nil || owner.UID == "" {
					return
				}
				req.OwnerName, req.OwnerUID, req.OwnerKind, req.OwnerAPIVersion = owner.Name, owner.UID, owner.Kind, owner.APIVersion
			} else {
				annotations := obj.GetAnnotations()
				if annotations[ManagementTaskNamespaceAnnotation] != scope.ManagementNamespace || len(validation.IsDNS1123Subdomain(annotations[ManagementTaskNameAnnotation])) != 0 || annotations[ManagementTaskUIDAnnotation] == "" {
					return
				}
				if id := annotations[kubernetesruntime.RuntimeIDAnnotation]; id != "" && id != string(scope.RuntimeID) {
					return
				}
				if annotations[kubernetesruntime.RuntimeIDAnnotation] == "" && scope.RuntimeID != kubernetesruntime.DefaultID {
					return
				}
				if kind := annotations[kubernetesruntime.RuntimeKindAnnotation]; kind != "" && kind != string(kubernetesruntime.Kind) {
					return
				}
				req.TaskName, req.TaskUID = annotations[ManagementTaskNameAnnotation], types.UID(annotations[ManagementTaskUIDAnnotation])
			}
		}
		q.Add(req)
	}
	return handler.TypedFuncs[client.Object, runtimeObservationRequest]{
		CreateFunc: func(_ context.Context, e event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[runtimeObservationRequest]) {
			add(e.Object, false, q)
		},
		UpdateFunc: func(_ context.Context, e event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[runtimeObservationRequest]) {
			add(e.ObjectOld, false, q)
			add(e.ObjectNew, false, q)
		},
		DeleteFunc: func(_ context.Context, e event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[runtimeObservationRequest]) {
			add(e.Object, true, q)
		},
		GenericFunc: func(_ context.Context, e event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[runtimeObservationRequest]) {
			add(e.Object, false, q)
		},
	}
}

func runtimeRootHandler(scope KubernetesObservationScope) handler.TypedEventHandler[client.Object, runtimeObservationRequest] {
	return runtimeEventHandler(scope, false)
}
func runtimePodHandler(scope KubernetesObservationScope) handler.TypedEventHandler[client.Object, runtimeObservationRequest] {
	return runtimeEventHandler(scope, true)
}

type runtimeObservationReconciler struct {
	push    pushRuntimeReconciler
	profile *kubernetesruntime.ObservationProfile
}

func (r *runtimeObservationReconciler) Reconcile(ctx context.Context, req runtimeObservationRequest) (reconcile.Result, error) {
	if req.Key.Namespace != r.push.scope.Namespace {
		return reconcile.Result{}, nil
	}
	if req.Pod {
		return r.reconcilePod(ctx, req)
	}
	if !req.Deleted {
		return r.push.Reconcile(ctx, reconcile.Request{NamespacedName: req.Key})
	}
	if req.TaskUID == "" || req.TaskName == "" || req.UID == "" {
		return reconcile.Result{}, nil
	}
	// An old deletion hint must not be interpreted as evidence that a replacement
	// root is missing. Root reads and Task identity are revalidated on every retry.
	root := r.push.observation.newObject()
	// Authoritative absence/replacement confirmation must bypass the eventually
	// consistent watch cache. Ordinary hint reads do not use this direct path.
	if err := r.push.c.LocalKubeClient.Get(ctx, req.Key, root); err == nil {
		return reconcile.Result{}, nil
	} else if client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}
	var task rlarkv1alpha1.Task
	if err := r.push.c.ManagementClient.Get(ctx, client.ObjectKey{Namespace: r.push.scope.ManagementNamespace, Name: req.TaskName}, &task); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if task.UID != req.TaskUID || !r.push.scope.allowsTask(&task) || task.Spec.Workload.Carrier.Type != r.push.observation.CarrierType {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, r.push.c.triggers.emit(trigger.Trigger{
		Key: trigger.Key{Namespace: task.Namespace, Name: task.Name}, Kind: trigger.RuntimeState,
		RuntimeID: r.push.scope.RuntimeID, ObjectUID: string(task.UID), Generation: task.Generation,
		// No Root: forces pull reconciliation instead of using deleted evidence.
	})
}

func (r *runtimeObservationReconciler) reconcilePod(ctx context.Context, req runtimeObservationRequest) (reconcile.Result, error) {
	if r.profile == nil {
		return reconcile.Result{}, nil
	}
	var pod corev1.Pod
	if req.Deleted {
		pod.ObjectMeta = metav1.ObjectMeta{Namespace: req.Key.Namespace, Name: req.Key.Name, UID: req.UID, OwnerReferences: []metav1.OwnerReference{{Name: req.OwnerName, UID: req.OwnerUID, Kind: req.OwnerKind, APIVersion: req.OwnerAPIVersion, Controller: boolPointer(true)}}}
	} else {
		if err := r.push.observedReader.Get(ctx, req.Key, &pod); err != nil {
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
		// Metadata may have changed since enqueueing. Do not resolve owners
		// for a Pod that is no longer marked as a management Task instance.
		if pod.UID != req.UID || pod.Annotations[ManagementTaskNameAnnotation] == "" {
			return reconcile.Result{}, nil
		}
	}
	root, err := r.profile.OwningRoot(ctx, r.push.observedReader, r.push.c.LocalKubeClient, &pod, r.push.observation.newObject())
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve instance owner: %w", err)
	}
	if root == nil {
		return reconcile.Result{}, nil
	}
	return r.push.enqueueRoot(ctx, root)
}

func boolPointer(value bool) *bool { return &value }

func (s KubernetesObservationScope) allowsTask(task *rlarkv1alpha1.Task) bool {
	if task.Namespace != s.ManagementNamespace || string(task.Spec.Runtime.Kind) != string(kubernetesruntime.Kind) {
		return false
	}
	if task.Spec.Runtime.Name == "" {
		return s.RuntimeID == kubernetesruntime.DefaultID
	}
	return task.Spec.Runtime.Name == string(s.RuntimeID)
}

func (s KubernetesObservationScope) allowsRoot(root client.Object) bool {
	if root.GetNamespace() != s.Namespace || root.GetAnnotations()[ManagementTaskNamespaceAnnotation] != s.ManagementNamespace {
		return false
	}
	annotations := root.GetAnnotations()
	if id := annotations[kubernetesruntime.RuntimeIDAnnotation]; id != "" && id != string(s.RuntimeID) {
		return false
	}
	if kind := annotations[kubernetesruntime.RuntimeKindAnnotation]; kind != "" && kind != string(kubernetesruntime.Kind) {
		return false
	}
	// Only the historical single default Runtime may read missing Runtime IDs.
	return annotations[kubernetesruntime.RuntimeIDAnnotation] != "" || s.RuntimeID == kubernetesruntime.DefaultID
}
