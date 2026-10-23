package smoke

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	shared "github.com/rlinf/rlark/test/e2e/framework"
	rlarkframework "github.com/rlinf/rlark/test/e2e/rlark/framework"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const clusterRole = "smoke"

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
		RLarkImage: os.Getenv("RLARK_SMOKE_IMAGE"),
		UIImage:    os.Getenv("RLARK_SMOKE_UI_IMAGE"),
		RLarkadm:   shared.EnvOrDefault("RLARK_SMOKE_RLARKADM", filepath.Join(config.RepoRoot, "apps", "rlark", "bin", "rlarkadm")),
	}
	timeout, err := shared.DurationEnv("RLARK_SMOKE_TIMEOUT", 20*time.Minute)
	if err != nil {
		return Input{}, err
	}
	input.Timeout = timeout
	if input.RLarkImage == "" || input.UIImage == "" {
		return Input{}, fmt.Errorf("RLARK_SMOKE_IMAGE and RLARK_SMOKE_UI_IMAGE are required")
	}
	if !immutableImage(input.RLarkImage) || !immutableImage(input.UIImage) {
		return Input{}, fmt.Errorf("smoke images must use a digest or an immutable non-latest tag")
	}
	return input, nil
}

func Scenario(input Input, config rlarkframework.Config, command shared.CommandRunner) shared.Scenario[*Environment] {
	requirements := shared.Requirements{Clusters: []shared.ClusterRequirement{
		{Role: clusterRole, ControlNodes: 1, WorkerNodes: 1},
	}}
	return shared.Scenario[*Environment]{
		Name:         "rlark-smoke",
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
			return &Environment{Environment: base, Input: input}, nil
		},
		Execute: func(ctx context.Context, _ *shared.Run, environment *Environment) error {
			cluster, err := environment.Cluster(clusterRole)
			if err != nil {
				return err
			}
			deadlineCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			if err := shared.Poll(deadlineCtx, 2*time.Second, func(ctx context.Context) (bool, string, error) {
				crds, err := cluster.Kubernetes.Discovery().ServerResourcesForGroupVersion("rlinf.io/v1alpha1")
				if err != nil {
					return false, err.Error(), nil
				}
				return len(crds.APIResources) > 0, fmt.Sprintf("discovered %d RLark resources", len(crds.APIResources)), nil
			}); err != nil {
				return err
			}
			return shared.Poll(deadlineCtx, 2*time.Second, func(ctx context.Context) (bool, string, error) {
				deployments, err := cluster.Kubernetes.AppsV1().Deployments("rlark-system").List(ctx, metav1.ListOptions{})
				if err != nil {
					return false, "", fmt.Errorf("list RLark deployments: %w", err)
				}
				if len(deployments.Items) == 0 {
					return false, "RLark installation created no deployments", nil
				}
				for _, deployment := range deployments.Items {
					desired := int32(1)
					if deployment.Spec.Replicas != nil {
						desired = *deployment.Spec.Replicas
					}
					if deployment.Status.ObservedGeneration < deployment.Generation || deployment.Status.AvailableReplicas < desired {
						return false, fmt.Sprintf("deployment %s has %d/%d available replicas", deployment.Name, deployment.Status.AvailableReplicas, desired), nil
					}
				}
				return true, fmt.Sprintf("all %d RLark deployments are available", len(deployments.Items)), nil
			})
		},
		Collect: func(ctx context.Context, run *shared.Run, environment *Environment) error {
			cluster, err := environment.Cluster(clusterRole)
			if err != nil {
				return err
			}
			pods, err := cluster.Kubernetes.CoreV1().Pods("rlark-system").List(ctx, metav1.ListOptions{})
			if err != nil {
				return fmt.Errorf("list RLark pods: %w", err)
			}
			var summary strings.Builder
			for _, pod := range pods.Items {
				fmt.Fprintf(&summary, "%s\t%s\n", pod.Name, pod.Status.Phase)
			}
			return os.WriteFile(filepath.Join(run.ArtifactsDir, "rlark-pods.txt"), []byte(summary.String()), 0o600)
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

func immutableImage(image string) bool {
	if strings.Contains(image, "@sha256:") {
		return true
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	return lastColon > lastSlash && image[lastColon+1:] != "latest"
}
