package podmanager

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ---------------------------------------------------------------------------
// Current-pod discovery helpers
// ---------------------------------------------------------------------------
//
// The device-plugin often needs to inherit attributes of its own pod and
// apply them to the controller pods it creates:
//
//   - OwnerReferences — so the controller pod is garbage-collected with the
//     device-plugin pod.
//   - Tolerations — so the controller pod schedules onto the same tainted
//     nodes as the device-plugin.
//   - Image — so the controller pod's init container reuses the
//     device-plugin image (which ships the controller/CLI binaries),
//     avoiding a separate init-image configuration.
//   - NodeName — so the controller pod is pinned to the same node as the
//     device-plugin.
//
// To avoid a separate API call per attribute, fetch the current pod once
// with CurrentPod (or CurrentPodFor) and extract each attribute with the
// corresponding FromPod helper. The Discover* functions remain as
// single-attribute convenience wrappers.

// currentPodFromEnv reads the POD_NAME and POD_NAMESPACE environment
// variables (set by the downward API) and returns them, or an error if
// either is unset.
func currentPodFromEnv() (string, string, error) {
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName == "" || podNamespace == "" {
		return "", "", fmt.Errorf(
			"POD_NAME and POD_NAMESPACE env vars must be set (via the downward API)")
	}
	return podName, podNamespace, nil
}

// CurrentPod returns the pod identified by the POD_NAME and POD_NAMESPACE
// environment variables (typically set by the Kubernetes downward API).
// Use it to fetch the pod once and extract multiple attributes via the
// FromPod helpers below, avoiding a separate API call per attribute.
//
// POD_NAME and POD_NAMESPACE must be set via the downward API, e.g.:
//
//	env:
//	  - name: POD_NAME
//	    valueFrom:
//	      fieldRef:
//	        fieldPath: metadata.name
//	  - name: POD_NAMESPACE
//	    valueFrom:
//	      fieldRef:
//	        fieldPath: metadata.namespace
func CurrentPod(ctx context.Context, clientset kubernetes.Interface) (*corev1.Pod, error) {
	podName, podNamespace, err := currentPodFromEnv()
	if err != nil {
		return nil, err
	}
	return CurrentPodFor(ctx, clientset, podName, podNamespace)
}

// CurrentPodFor is like CurrentPod but takes the pod name and namespace
// explicitly instead of reading them from environment variables.
func CurrentPodFor(ctx context.Context, clientset kubernetes.Interface, podName, podNamespace string) (*corev1.Pod, error) {
	pod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get current pod %s/%s: %w", podNamespace, podName, err)
	}
	return pod, nil
}

// ---------------------------------------------------------------------------
// From-pod attribute extractors
// ---------------------------------------------------------------------------

// OwnerRefFromPod returns an OwnerReference that points at the given pod
// itself. Created pods are therefore owned by (and garbage-collected with)
// the device-plugin pod, regardless of whether that pod is itself owned by
// a higher-level controller such as a DaemonSet.
func OwnerRefFromPod(pod *corev1.Pod) metav1.OwnerReference {
	controller := true
	return metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       pod.Name,
		UID:        pod.UID,
		Controller: &controller,
	}
}

// TolerationsFromPod returns the tolerations of the given pod.
func TolerationsFromPod(pod *corev1.Pod) []corev1.Toleration {
	return pod.Spec.Tolerations
}

// ImageFromPod returns the image of the first container of the given pod.
// It returns an error if the pod has no containers.
func ImageFromPod(pod *corev1.Pod) (string, error) {
	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("pod %s/%s has no containers", pod.Namespace, pod.Name)
	}
	return pod.Spec.Containers[0].Image, nil
}

// NodeNameFromPod returns the name of the node the given pod is scheduled
// on (pod.spec.nodeName). It returns an error if the pod is not yet
// scheduled to a node.
func NodeNameFromPod(pod *corev1.Pod) (string, error) {
	if pod.Spec.NodeName == "" {
		return "", fmt.Errorf("pod %s/%s is not yet scheduled to a node", pod.Namespace, pod.Name)
	}
	return pod.Spec.NodeName, nil
}

// ---------------------------------------------------------------------------
// Convenience wrappers — single attribute via a single API call
// ---------------------------------------------------------------------------
//
// Each Discover* function fetches the current pod and extracts one
// attribute. When multiple attributes are needed, prefer CurrentPod + the
// FromPod helpers to avoid redundant API calls.

// DiscoverOwnerRef returns an OwnerReference pointing at the current pod
// itself. See OwnerRefFromPod for the semantics.
func DiscoverOwnerRef(ctx context.Context, clientset kubernetes.Interface) (metav1.OwnerReference, error) {
	pod, err := CurrentPod(ctx, clientset)
	if err != nil {
		return metav1.OwnerReference{}, err
	}
	return OwnerRefFromPod(pod), nil
}

// DiscoverOwnerRefForPod is like DiscoverOwnerRef but takes the pod name
// and namespace explicitly. See OwnerRefFromPod for the semantics.
func DiscoverOwnerRefForPod(ctx context.Context, clientset kubernetes.Interface, podName, podNamespace string) (metav1.OwnerReference, error) {
	pod, err := CurrentPodFor(ctx, clientset, podName, podNamespace)
	if err != nil {
		return metav1.OwnerReference{}, err
	}
	return OwnerRefFromPod(pod), nil
}

// DiscoverTolerations returns the tolerations of the current pod.
func DiscoverTolerations(ctx context.Context, clientset kubernetes.Interface) ([]corev1.Toleration, error) {
	pod, err := CurrentPod(ctx, clientset)
	if err != nil {
		return nil, err
	}
	return TolerationsFromPod(pod), nil
}

// DiscoverTolerationsForPod is like DiscoverTolerations but takes the pod
// name and namespace explicitly.
func DiscoverTolerationsForPod(ctx context.Context, clientset kubernetes.Interface, podName, podNamespace string) ([]corev1.Toleration, error) {
	pod, err := CurrentPodFor(ctx, clientset, podName, podNamespace)
	if err != nil {
		return nil, err
	}
	return TolerationsFromPod(pod), nil
}

// DiscoverImage returns the image of the first container of the current
// pod.
func DiscoverImage(ctx context.Context, clientset kubernetes.Interface) (string, error) {
	pod, err := CurrentPod(ctx, clientset)
	if err != nil {
		return "", err
	}
	return ImageFromPod(pod)
}

// DiscoverImageForPod is like DiscoverImage but takes the pod name and
// namespace explicitly.
func DiscoverImageForPod(ctx context.Context, clientset kubernetes.Interface, podName, podNamespace string) (string, error) {
	pod, err := CurrentPodFor(ctx, clientset, podName, podNamespace)
	if err != nil {
		return "", err
	}
	return ImageFromPod(pod)
}

// DiscoverNodeName returns the name of the node the current pod is
// scheduled on.
func DiscoverNodeName(ctx context.Context, clientset kubernetes.Interface) (string, error) {
	pod, err := CurrentPod(ctx, clientset)
	if err != nil {
		return "", err
	}
	return NodeNameFromPod(pod)
}

// DiscoverNodeNameForPod is like DiscoverNodeName but takes the pod name
// and namespace explicitly.
func DiscoverNodeNameForPod(ctx context.Context, clientset kubernetes.Interface, podName, podNamespace string) (string, error) {
	pod, err := CurrentPodFor(ctx, clientset, podName, podNamespace)
	if err != nil {
		return "", err
	}
	return NodeNameFromPod(pod)
}
