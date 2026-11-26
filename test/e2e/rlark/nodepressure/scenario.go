package nodepressure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	shared "github.com/rlinf/rlark/test/e2e/framework"
	rlarkframework "github.com/rlinf/rlark/test/e2e/rlark/framework"
)

const (
	clusterRole = "control"

	managementNamespace         = "rlark-system"
	controllerManagerDeployment = "rlark-controller-manager"

	// The Node controller treats a Lease named node-heartbeat-<node-name> as the
	// authoritative connectivity signal for a Node.
	nodeHeartbeatLeasePrefix = "node-heartbeat-"
)

type Input struct {
	RLarkImage string
	UIImage    string
	RLarkadm   string
	Timeout    time.Duration
}

type Environment struct {
	*rlarkframework.Environment
	Input Input
}

func LoadInput(config shared.Config) (Input, error) {
	input := Input{
		RLarkImage: os.Getenv("RLARK_NODE_PRESSURE_IMAGE"),
		UIImage:    os.Getenv("RLARK_NODE_PRESSURE_UI_IMAGE"),
		RLarkadm:   shared.EnvOrDefault("RLARK_NODE_PRESSURE_RLARKADM", filepath.Join(config.RepoRoot, "apps", "rlark", "bin", "rlarkadm")),
	}
	timeout, err := shared.DurationEnv("RLARK_NODE_PRESSURE_TIMEOUT", 20*time.Minute)
	if err != nil {
		return Input{}, err
	}
	input.Timeout = timeout
	if input.RLarkImage == "" || input.UIImage == "" {
		return Input{}, fmt.Errorf("RLARK_NODE_PRESSURE_IMAGE and RLARK_NODE_PRESSURE_UI_IMAGE are required")
	}
	if !immutableImage(input.RLarkImage) || !immutableImage(input.UIImage) {
		return Input{}, fmt.Errorf("node pressure images must use a digest or an immutable non-latest tag")
	}
	return input, nil
}

// Scenario validates how the control plane derives Node conditions from the
// agent-reported disk pressure signal. It injects the management Node status and
// heartbeat Lease directly, so it runs without a data plane Agent and without
// creating real disk pressure.
func Scenario(input Input, config rlarkframework.Config, command shared.CommandRunner) shared.Scenario[*Environment] {
	requirements := shared.Requirements{Clusters: []shared.ClusterRequirement{
		{Role: clusterRole, ControlNodes: 1, WorkerNodes: 1},
	}}
	return shared.Scenario[*Environment]{
		Name:         "rlark-node-pressure",
		Requirements: requirements,
		Validate: func(ctx context.Context, _ *shared.Run) error {
			if info, err := os.Stat(input.RLarkadm); err != nil {
				return fmt.Errorf("access rlarkadm %q: %w", input.RLarkadm, err)
			} else if info.Mode()&0o111 == 0 {
				return fmt.Errorf("rlarkadm %q is not executable", input.RLarkadm)
			}
			for _, image := range []string{input.RLarkImage, input.UIImage} {
				if _, err := command.Run(ctx, "docker", "image", "inspect", image); err != nil {
					return fmt.Errorf("required local image %q is unavailable: %w", image, err)
				}
			}
			return nil
		},
		Prepare: func(ctx context.Context, run *shared.Run, provisioned *shared.ProvisionedEnvironment) (*Environment, error) {
			base, err := rlarkframework.NewEnvironment(config, requirements, provisioned)
			if err != nil {
				return nil, err
			}
			if _, err := base.Cluster(clusterRole); err != nil {
				return nil, err
			}
			clusterName := provisioned.Metadata["cluster."+clusterRole]
			for _, image := range []string{input.RLarkImage, input.UIImage} {
				if _, err := command.Run(ctx, config.Kind.Binary, "load", "docker-image", image, "--name", clusterName); err != nil {
					return nil, fmt.Errorf("load image %q into Kind: %w", image, err)
				}
			}
			deployConfig := filepath.Join(run.ArtifactsDir, "deploy-control-plane.yaml")
			content := fmt.Sprintf(`apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  management-api: kubernetes
  kubeconfig: %q
  gateway-image: %q
  controller-manager-image: %q
  server-image: %q
  ui-image: %q
  image-pull-policy: Never
  storage:
    type: emptyDir
`, provisioned.Clusters[clusterRole].Kubeconfig, input.RLarkImage, input.RLarkImage, input.RLarkImage, input.UIImage)
			if err := os.WriteFile(deployConfig, []byte(content), 0o600); err != nil {
				return nil, fmt.Errorf("write deploy config: %w", err)
			}
			if _, err := command.Run(ctx, input.RLarkadm, "install", "-f", deployConfig); err != nil {
				return nil, fmt.Errorf("install RLark control plane: %w", err)
			}
			if err := base.EnableRLarkClient(clusterRole); err != nil {
				return nil, err
			}
			return &Environment{Environment: base, Input: input}, nil
		},
		Execute: func(ctx context.Context, run *shared.Run, environment *Environment) error {
			cluster, err := environment.Cluster(clusterRole)
			if err != nil {
				return err
			}
			if err := waitForControllerManager(ctx, cluster); err != nil {
				return err
			}
			namespace := run.Name("nodes")
			nodeName := nodeFixtureName(run)
			leaseName := nodeHeartbeatLeasePrefix + nodeName
			if _, err := cluster.Kubernetes.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: run.Labels},
			}, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create fixture namespace %q: %w", namespace, err)
			}
			nodeClient := environment.RLark.RlinfV1alpha1().Nodes(namespace)
			if _, err := nodeClient.Create(ctx, &rlarkv1alpha1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName, Namespace: namespace, Labels: run.Labels},
			}, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create node %q: %w", nodeName, err)
			}
			// The Agent normally reports these fields. Injecting them directly keeps
			// the scenario free of a data plane deployment and real disk usage.
			if _, err := nodeClient.Patch(ctx, nodeName, types.MergePatchType,
				[]byte(`{"status":{"kubernetesReady":true,"diskPressure":true}}`),
				metav1.PatchOptions{}, "status"); err != nil {
				return fmt.Errorf("inject disk pressure into node %q: %w", nodeName, err)
			}
			if _, err := cluster.Kubernetes.CoordinationV1().Leases(namespace).Create(ctx, heartbeatLease(namespace, leaseName, run.Labels), metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create node heartbeat lease %q: %w", leaseName, err)
			}

			deadlineCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return shared.Poll(deadlineCtx, 2*time.Second, func(ctx context.Context) (bool, string, error) {
				// Renew the Lease so the Node stays connected while the controller
				// catches up; the default grace window is shorter than the deadline.
				if err := renewHeartbeatLease(ctx, cluster, namespace, leaseName); err != nil {
					return false, "", err
				}
				node, err := nodeClient.Get(ctx, nodeName, metav1.GetOptions{})
				if err != nil {
					return false, "", fmt.Errorf("get node %q: %w", nodeName, err)
				}
				return matchesDiskPressureDerivation(node)
			})
		},
		Collect: func(ctx context.Context, run *shared.Run, environment *Environment) error {
			namespace := run.Name("nodes")
			node, err := environment.RLark.RlinfV1alpha1().Nodes(namespace).Get(ctx, nodeFixtureName(run), metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("get node for evidence: %w", err)
			}
			var summary strings.Builder
			fmt.Fprintf(&summary, "phase: %s\n", node.Status.Phase)
			if node.Status.DiskPressure != nil {
				fmt.Fprintf(&summary, "diskPressure: %t\n", *node.Status.DiskPressure)
			}
			for _, condition := range node.Status.Conditions {
				fmt.Fprintf(&summary, "%s: %s (%s)\n", condition.Type, condition.Status, condition.Reason)
			}
			return os.WriteFile(filepath.Join(run.ArtifactsDir, "node-pressure.txt"), []byte(summary.String()), 0o600)
		},
		Cleanup: func(ctx context.Context, run *shared.Run, _ *Environment) error {
			deployConfig := filepath.Join(run.ArtifactsDir, "deploy-control-plane.yaml")
			if _, err := command.Run(ctx, input.RLarkadm, "uninstall", "-f", deployConfig, "--purge", "--yes"); err != nil {
				return fmt.Errorf("uninstall RLark control plane: %w", err)
			}
			return nil
		},
	}
}

func nodeFixtureName(run *shared.Run) string {
	return "node-" + run.ID
}

func heartbeatLease(namespace, name string, labels map[string]string) *coordinationv1.Lease {
	duration := int32(45)
	holder := "e2e-node-pressure"
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			LeaseDurationSeconds: &duration,
			RenewTime:            &metav1.MicroTime{Time: time.Now().UTC()},
		},
	}
}

func renewHeartbeatLease(ctx context.Context, cluster *shared.Cluster, namespace, name string) error {
	renewTime := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")
	_, err := cluster.Kubernetes.CoordinationV1().Leases(namespace).Patch(ctx, name, types.MergePatchType,
		[]byte(fmt.Sprintf(`{"spec":{"renewTime":%q}}`, renewTime)), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("renew node heartbeat lease %q: %w", name, err)
	}
	return nil
}

func waitForControllerManager(ctx context.Context, cluster *shared.Cluster) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return shared.Poll(deadlineCtx, 2*time.Second, func(ctx context.Context) (bool, string, error) {
		deployment, err := cluster.Kubernetes.AppsV1().Deployments(managementNamespace).Get(ctx, controllerManagerDeployment, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, "controller manager deployment not found", nil
		}
		if err != nil {
			return false, "", fmt.Errorf("get controller manager deployment: %w", err)
		}
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if deployment.Status.ObservedGeneration < deployment.Generation || deployment.Status.AvailableReplicas < desired {
			return false, fmt.Sprintf("controller manager has %d/%d available replicas", deployment.Status.AvailableReplicas, desired), nil
		}
		return true, "controller manager is available", nil
	})
}

func matchesDiskPressureDerivation(node *rlarkv1alpha1.Node) (bool, string, error) {
	if node.Status.DiskPressure == nil || !*node.Status.DiskPressure {
		return false, "node status does not report diskPressure=true", nil
	}
	expected := []struct {
		conditionType string
		status        metav1.ConditionStatus
		reason        string
	}{
		{rlarkv1alpha1.ConditionTypeAgentConnected, metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonLeaseValid},
		{rlarkv1alpha1.ConditionTypeResourcePressure, metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonResourcePressureDetected},
		{rlarkv1alpha1.ConditionTypeStorageReady, metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonStorageUnavailable},
		{rlarkv1alpha1.ConditionTypeDegraded, metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonResourcePressureDetected},
		{rlarkv1alpha1.ConditionTypeReady, metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesNodeReady},
	}
	for _, want := range expected {
		condition := apimeta.FindStatusCondition(node.Status.Conditions, want.conditionType)
		if condition == nil {
			return false, fmt.Sprintf("condition %s is missing: %s", want.conditionType, describeNode(node)), nil
		}
		if condition.Status != want.status || condition.Reason != want.reason {
			return false, fmt.Sprintf("%s=%s/%s, want %s/%s: %s", want.conditionType, condition.Status, condition.Reason, want.status, want.reason, describeNode(node)), nil
		}
	}
	if node.Status.Phase != rlarkv1alpha1.NodeDegraded {
		return false, fmt.Sprintf("phase=%s, want %s: %s", node.Status.Phase, rlarkv1alpha1.NodeDegraded, describeNode(node)), nil
	}
	return true, "node derives Degraded from disk pressure while remaining Ready", nil
}

func describeNode(node *rlarkv1alpha1.Node) string {
	parts := []string{fmt.Sprintf("phase=%s", node.Status.Phase)}
	for _, condition := range node.Status.Conditions {
		parts = append(parts, fmt.Sprintf("%s=%s/%s", condition.Type, condition.Status, condition.Reason))
	}
	return strings.Join(parts, ", ")
}

func immutableImage(image string) bool {
	if strings.Contains(image, "@sha256:") {
		return true
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	return lastColon > lastSlash && image[lastColon+1:] != "latest"
}
