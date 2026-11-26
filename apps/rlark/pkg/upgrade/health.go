package upgrade

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// HealthThreshold defines health check thresholds
type HealthThreshold struct {
	// Minimum ready ratio for deployments
	MinReadyRatio float64

	// Maximum allowed restart count per container
	MaxRestartCount int32

	// Maximum allowed unavailable pods
	MaxUnavailablePods int

	// Maximum consecutive check failures before declaring unhealthy
	MaxConsecutiveFailures int
}

// DefaultHealthThreshold returns default thresholds
func DefaultHealthThreshold() *HealthThreshold {
	return &HealthThreshold{
		MinReadyRatio:          0.9,
		MaxRestartCount:        5,
		MaxUnavailablePods:     0,
		MaxConsecutiveFailures: 3,
	}
}

// HealthChecker performs health checks
type HealthChecker struct {
	client    kubernetes.Interface
	namespace string
	threshold *HealthThreshold
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker(kubeconfigPath string, threshold *HealthThreshold) (*HealthChecker, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("build config: %w", err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("get in-cluster config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return &HealthChecker{
		client:    client,
		namespace: "rlark-system",
		threshold: threshold,
	}, nil
}

// Check performs all health checks
func (h *HealthChecker) Check(ctx context.Context) error {
	// Check 1: Components ready
	if err := h.checkComponentsReady(ctx); err != nil {
		return fmt.Errorf("components not ready: %w", err)
	}

	// Check 2: Pod health
	if err := h.checkPodHealth(ctx); err != nil {
		return fmt.Errorf("pod health check failed: %w", err)
	}

	// Check 3: No crash loops
	if err := h.checkNoCrashLoops(ctx); err != nil {
		return fmt.Errorf("crash loops detected: %w", err)
	}

	// Check 4: Services accessible
	if err := h.checkServicesAccessible(ctx); err != nil {
		return fmt.Errorf("services not accessible: %w", err)
	}

	return nil
}

// checkComponentsReady checks if all components are ready
func (h *HealthChecker) checkComponentsReady(ctx context.Context) error {
	components := []string{"rlark-agent", "rlark-server", "rlark-gateway", "rlark-controller-manager"}

	for _, name := range components {
		deployment, err := h.client.AppsV1().Deployments(h.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Component may not exist in all planes
				continue
			}
			return fmt.Errorf("get deployment %s: %w", name, err)
		}

		readyRatio := float64(deployment.Status.ReadyReplicas) / float64(*deployment.Spec.Replicas)
		if readyRatio < h.threshold.MinReadyRatio {
			return fmt.Errorf("deployment %s ready ratio too low: %.2f < %.2f",
				name, readyRatio, h.threshold.MinReadyRatio)
		}
	}

	return nil
}

// checkPodHealth checks pod health status
func (h *HealthChecker) checkPodHealth(ctx context.Context) error {
	pods, err := h.client.CoreV1().Pods(h.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=rlark-agent",
	})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no rlark-agent pods found")
	}

	unhealthyPods := 0
	for _, pod := range pods.Items {
		// Ignore pods that are terminating (old replica being replaced).
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			unhealthyPods++
		}
	}

	if unhealthyPods > h.threshold.MaxUnavailablePods {
		return fmt.Errorf("too many unhealthy pods: %d", unhealthyPods)
	}

	return nil
}

// checkNoCrashLoops checks for crash loops
func (h *HealthChecker) checkNoCrashLoops(ctx context.Context) error {
	pods, err := h.client.CoreV1().Pods(h.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > h.threshold.MaxRestartCount {
				return fmt.Errorf("pod %s container %s has too many restarts: %d",
					pod.Name, cs.Name, cs.RestartCount)
			}
		}
	}

	return nil
}

// checkServicesAccessible checks if services are accessible
func (h *HealthChecker) checkServicesAccessible(ctx context.Context) error {
	services := []string{"rlark-server", "rlark-gateway"}

	for _, name := range services {
		_, err := h.client.CoreV1().Services(h.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("check service %s: %w", name, err)
		}
	}

	return nil
}
