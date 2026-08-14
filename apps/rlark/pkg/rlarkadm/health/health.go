package health

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	checkTimeout  = 180 * time.Second
	checkInterval = 3 * time.Second
)

// WaitForHealthy retries the component's HealthCheckFn until it passes or times out.
func WaitForHealthy(cfg *types.DeployConfig, comp types.Component) error {
	logger := log.GetLogger()
	if comp.HealthCheckFn == nil {
		return nil
	}

	logger.Info("waiting for component to become healthy", "name", comp.Name)

	deadline := time.Now().Add(checkTimeout)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		err := comp.HealthCheckFn(cfg)
		if err == nil {
			logger.Info("component is healthy", "name", comp.Name)
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%s health check failed after %v: %w", comp.Name, checkTimeout, err)
		}

		logger.V(1).Info("component not ready, retrying", "name", comp.Name, "err", err, "interval", checkInterval)
		<-ticker.C
	}
}

// ModeHealthCheck returns a HealthCheckFn for docker/raw modes.
// For kubernetes mode, the deployer overrides HealthCheckFn with K8sWorkloadHealthCheck.
func ModeHealthCheck(comp types.Component) func(cfg *types.DeployConfig) error {
	return func(cfg *types.DeployConfig) error {
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

// K8sWorkloadHealthCheck returns a HealthCheckFn that checks workload readiness
// based on the component's WorkloadKind (Deployment, StatefulSet, or DaemonSet).
func K8sWorkloadHealthCheck(clientset *kubernetes.Clientset, comp types.Component) func(cfg *types.DeployConfig) error {
	name := comp.Name
	switch comp.WorkloadKind {
	case "StatefulSet":
		return func(cfg *types.DeployConfig) error {
			sts, err := clientset.AppsV1().StatefulSets(constants.Namespace).Get(
				context.Background(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get statefulset %s: %w", name, err)
			}
			if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
				return fmt.Errorf("statefulset %s: %d/%d replicas ready",
					name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
			}
			return nil
		}
	case "DaemonSet":
		return func(cfg *types.DeployConfig) error {
			ds, err := clientset.AppsV1().DaemonSets(constants.Namespace).Get(
				context.Background(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get daemonset %s: %w", name, err)
			}
			if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
				return fmt.Errorf("daemonset %s: %d/%d pods ready",
					name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
			}
			return nil
		}
	default:
		return func(cfg *types.DeployConfig) error {
			dep, err := clientset.AppsV1().Deployments(constants.Namespace).Get(
				context.Background(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get deployment %s: %w", name, err)
			}
			if dep.Status.ReadyReplicas != *dep.Spec.Replicas {
				return fmt.Errorf("deployment %s: %d/%d replicas ready",
					name, dep.Status.ReadyReplicas, *dep.Spec.Replicas)
			}
			return nil
		}
	}
}
