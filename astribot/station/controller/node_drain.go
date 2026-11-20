package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

var ErrNodeIdentityChanged = errors.New("node identity changed")
var ErrNodeDrainBlocked = errors.New("node drain blocked")

// DrainWorker performs one retryable drain pass against an exact Node instance.
// It never bypasses a disruption budget or deletes a Pod directly.
func DrainWorker(ctx context.Context, client kubernetes.Interface, name string, uid types.UID) (bool, error) {
	node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("read node: %w", err)
	}
	if node.UID != uid || node.DeletionTimestamp != nil {
		return false, ErrNodeIdentityChanged
	}
	if _, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; controlPlane {
		return false, fmt.Errorf("%w: control-plane node", ErrNodeDrainBlocked)
	}
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		if _, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return false, fmt.Errorf("cordon node: %w", err)
		}
	}
	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
	if err != nil {
		return false, fmt.Errorf("list node pods: %w", err)
	}
	// Check every candidate before evicting any of them so an obvious blocker
	// cannot leave the node partially drained.
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName != name || isDaemonOrMirrorPod(pod) || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		// Local emptyDir data goes with the node being removed.
		if pod.UID == "" || !hasController(pod) {
			return false, fmt.Errorf("%w: pod %s/%s has no verified controller or UID", ErrNodeDrainBlocked, pod.Namespace, pod.Name)
		}
	}
	pending := false
	for i := range pods.Items {
		pod := &pods.Items[i]
		// Fake clients do not enforce field selectors; this also defends against
		// an unexpected API response before requesting an eviction.
		if pod.Spec.NodeName != name || isDaemonOrMirrorPod(pod) || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		pending = true
		if pod.DeletionTimestamp != nil {
			continue
		}
		current, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("recheck node before eviction: %w", err)
		}
		if current.UID != uid || current.DeletionTimestamp != nil {
			return false, ErrNodeIdentityChanged
		}
		eviction := &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace}, DeleteOptions: &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pod.UID}}}
		if err := client.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
	return !pending, nil
}

func hasController(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller && owner.UID != "" {
			return true
		}
	}
	return false
}

func isDaemonOrMirrorPod(pod *corev1.Pod) bool {
	if _, mirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; mirror {
		return true
	}
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" && owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}

// ForceDrainWorker empties a worker whose kubelet has stopped reporting: no
// kubelet will ever confirm an eviction there, so its pods are deleted at
// once and their controllers start them elsewhere. Pods without a controller
// and their local data go with the machine. DaemonSet pods stay until the Node
// is deleted, when Kubernetes collects them.
func ForceDrainWorker(ctx context.Context, client kubernetes.Interface, name string, uid types.UID) (bool, error) {
	node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("read node: %w", err)
	}
	if node.UID != uid || node.DeletionTimestamp != nil {
		return false, ErrNodeIdentityChanged
	}
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		if _, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return false, fmt.Errorf("cordon node: %w", err)
		}
	}
	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
	if err != nil {
		return false, fmt.Errorf("list node pods: %w", err)
	}
	now := int64(0)
	pending := false
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName != name || isDaemonOrMirrorPod(pod) {
			continue
		}
		pending = true
		err := client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &now, Preconditions: &metav1.Preconditions{UID: &pod.UID}})
		if err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
	return !pending, nil
}
