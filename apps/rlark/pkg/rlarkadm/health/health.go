package health

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	defaultCheckTimeout = 180 * time.Second
	checkInterval       = 3 * time.Second
)

// WaitForHealthy retries the component's HealthCheckFn until it passes or times out.
func WaitForHealthy(ctx context.Context, cfg *types.DeployConfig, comp types.Component) error {
	logger := log.GetLogger()
	if comp.HealthCheckFn == nil {
		return nil
	}

	logger.Info("waiting for component to become healthy", "name", comp.Name)

	timeout := utils.GetEnvDuration("HEALTH_CHECK_TIMEOUT", defaultCheckTimeout)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		err := comp.HealthCheckFn(ctx, cfg)
		if err == nil {
			logger.Info("component is healthy", "name", comp.Name)
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%s health check failed after %v: %w", comp.Name, timeout, err)
		}

		logger.V(1).Info("component not ready, retrying", "name", comp.Name, "err", err, "interval", checkInterval)
		<-ticker.C
	}
}

// ModeHealthCheck returns a HealthCheckFn for docker/raw modes.
// For kubernetes mode, it uses the clientset from context.
func ModeHealthCheck(comp types.Component) func(ctx context.Context, cfg *types.DeployConfig) error {
	return func(ctx context.Context, cfg *types.DeployConfig) error {
		// Try Kubernetes mode first if clientset is available
		if clientset, ok := runtime.GetClientset(ctx); ok {
			return checkK8sWorkload(ctx, clientset, comp, cfg)
		}

		// Fall back to Docker/Raw mode
		switch cfg.EnvMode() {
		case "Docker":
			return exec.Command("docker", "inspect", "--format", "{{.State.Running}}", comp.Name).Run()
		case "Raw":
			return exec.Command("systemctl", "is-active", "--quiet", comp.Name).Run()
		default:
			return nil
		}
	}
}

// checkK8sWorkload checks that a workload has fully rolled out its latest spec.
// It verifies ObservedGeneration, UpdatedReplicas, and ReadyReplicas to ensure
// old replicas are not mistaken for a successful upgrade.
func checkK8sWorkload(ctx context.Context, clientset kubernetes.Interface, comp types.Component, cfg *types.DeployConfig) error {
	name := comp.Name

	switch comp.WorkloadKind {
	case "StatefulSet":
		sts, err := clientset.AppsV1().StatefulSets(constants.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get statefulset %s: %w", name, err)
		}
		if sts.Status.ObservedGeneration < sts.Generation {
			return fmt.Errorf("statefulset %s: stale observed generation %d < %d",
				name, sts.Status.ObservedGeneration, sts.Generation)
		}
		if sts.Status.UpdatedReplicas != *sts.Spec.Replicas {
			return fmt.Errorf("statefulset %s: %d/%d replicas updated",
				name, sts.Status.UpdatedReplicas, *sts.Spec.Replicas)
		}
		if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
			return fmt.Errorf("statefulset %s: %d/%d replicas ready",
				name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
		}
		return nil
	case "DaemonSet":
		ds, err := clientset.AppsV1().DaemonSets(constants.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get daemonset %s: %w", name, err)
		}
		if ds.Status.ObservedGeneration < ds.Generation {
			return fmt.Errorf("daemonset %s: stale observed generation %d < %d",
				name, ds.Status.ObservedGeneration, ds.Generation)
		}
		if ds.Status.UpdatedNumberScheduled != ds.Status.DesiredNumberScheduled {
			return fmt.Errorf("daemonset %s: %d/%d pods updated",
				name, ds.Status.UpdatedNumberScheduled, ds.Status.DesiredNumberScheduled)
		}
		if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
			return fmt.Errorf("daemonset %s: %d/%d pods ready",
				name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
		}
		return nil
	default:
		dep, err := clientset.AppsV1().Deployments(constants.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get deployment %s: %w", name, err)
		}
		if dep.Status.ObservedGeneration < dep.Generation {
			return fmt.Errorf("deployment %s: stale observed generation %d < %d",
				name, dep.Status.ObservedGeneration, dep.Generation)
		}
		if dep.Status.UpdatedReplicas != *dep.Spec.Replicas {
			return fmt.Errorf("deployment %s: %d/%d replicas updated",
				name, dep.Status.UpdatedReplicas, *dep.Spec.Replicas)
		}
		if dep.Status.ReadyReplicas != *dep.Spec.Replicas {
			return fmt.Errorf("deployment %s: %d/%d replicas ready",
				name, dep.Status.ReadyReplicas, *dep.Spec.Replicas)
		}
		return nil
	}
}
