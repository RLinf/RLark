package kind

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	shared "github.com/rlinf/rlark/test/e2e/framework"
)

const ProviderName = "kind"

var invalidClusterCharacter = regexp.MustCompile(`[^a-z0-9-]+`)

type Config struct {
	Binary    string
	NodeImage string
	ExtraArgs []string
	Command   shared.CommandRunner
	RetryWait time.Duration
}

type Provider struct {
	config Config
}

func New(config Config) *Provider {
	if config.Binary == "" {
		config.Binary = "kind"
	}
	if config.Command == nil {
		config.Command = shared.ExecRunner{}
	}
	if config.RetryWait <= 0 {
		config.RetryWait = time.Second
	}
	return &Provider{config: config}
}

func (p *Provider) Provision(ctx context.Context, run *shared.Run, requirements shared.Requirements) (*shared.ProvisionedEnvironment, error) {
	if run == nil {
		return nil, fmt.Errorf("run must not be nil")
	}
	if len(requirements.Clusters) == 0 {
		return nil, fmt.Errorf("scenario must require at least one cluster")
	}
	environment := &shared.ProvisionedEnvironment{
		Clusters: make(map[string]shared.ClusterConfig, len(requirements.Clusters)),
		Metadata: map[string]string{"provider": ProviderName},
	}
	created := make([]string, 0, len(requirements.Clusters))
	for _, requirement := range requirements.Clusters {
		if err := validateRequirement(requirement); err != nil {
			_ = p.destroyClusters(ctx, created)
			return nil, err
		}
		clusterName := clusterName(run.ID, requirement.Role)
		kubeconfig := filepath.Join(run.ArtifactsDir, "kubeconfigs", requirement.Role+".yaml")
		if err := os.MkdirAll(filepath.Dir(kubeconfig), 0o750); err != nil {
			_ = p.destroyClusters(ctx, created)
			return nil, fmt.Errorf("create kubeconfig directory: %w", err)
		}
		kindConfig := filepath.Join(run.ArtifactsDir, "kind", requirement.Role+".yaml")
		if err := writeKindConfig(kindConfig, requirement); err != nil {
			_ = p.destroyClusters(ctx, created)
			return nil, err
		}
		args := []string{"create", "cluster", "--name", clusterName, "--config", kindConfig, "--kubeconfig", kubeconfig, "--wait", "5m"}
		if p.config.NodeImage != "" {
			args = append(args, "--image", p.config.NodeImage)
		}
		args = append(args, p.config.ExtraArgs...)
		if _, err := p.config.Command.Run(ctx, p.config.Binary, args...); err != nil {
			_ = p.destroyClusters(ctx, created)
			return nil, fmt.Errorf("create Kind cluster for role %q: %w", requirement.Role, err)
		}
		created = append(created, clusterName)
		environment.Clusters[requirement.Role] = shared.ClusterConfig{Kubeconfig: kubeconfig}
		environment.Metadata["cluster."+requirement.Role] = clusterName
	}
	return environment, nil
}

func (p *Provider) Collect(ctx context.Context, run *shared.Run, environment *shared.ProvisionedEnvironment) error {
	if run == nil || environment == nil {
		return fmt.Errorf("run and environment must not be nil")
	}
	destination := filepath.Join(run.ArtifactsDir, "kind")
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return fmt.Errorf("create Kind artifact directory: %w", err)
	}
	var errors []string
	for role := range environment.Clusters {
		name := environment.Metadata["cluster."+role]
		if name == "" {
			errors = append(errors, fmt.Sprintf("cluster name missing for role %q", role))
			continue
		}
		if _, err := p.config.Command.Run(ctx, p.config.Binary, "export", "logs", filepath.Join(destination, role), "--name", name); err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("collect Kind logs: %s", strings.Join(errors, "; "))
	}
	return nil
}

func (p *Provider) Destroy(ctx context.Context, _ *shared.Run, environment *shared.ProvisionedEnvironment) error {
	if environment == nil {
		return nil
	}
	names := make([]string, 0, len(environment.Clusters))
	for role := range environment.Clusters {
		if name := environment.Metadata["cluster."+role]; name != "" {
			names = append(names, name)
		}
	}
	return p.destroyClusters(ctx, names)
}

func (p *Provider) destroyClusters(ctx context.Context, names []string) error {
	var errors []string
	for i := len(names) - 1; i >= 0; i-- {
		if err := p.destroyCluster(ctx, names[i]); err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("delete Kind clusters: %s", strings.Join(errors, "; "))
	}
	return nil
}

func (p *Provider) destroyCluster(ctx context.Context, name string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := p.config.Command.Run(ctx, p.config.Binary, "delete", "cluster", "--name", name); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == 3 {
			break
		}
		timer := time.NewTimer(p.config.RetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("delete Kind cluster %q: %w", name, errors.Join(lastErr, ctx.Err()))
		case <-timer.C:
		}
	}
	return fmt.Errorf("delete Kind cluster %q after 3 attempts: %w", name, lastErr)
}

func validateRequirement(requirement shared.ClusterRequirement) error {
	if requirement.Role == "" {
		return fmt.Errorf("cluster role must not be empty")
	}
	if requirement.ControlNodes < 1 {
		return fmt.Errorf("cluster role %q requires at least one control-plane node", requirement.Role)
	}
	if requirement.WorkerNodes < 0 {
		return fmt.Errorf("cluster role %q has a negative worker count", requirement.Role)
	}
	return nil
}

func clusterName(runID, role string) string {
	name := strings.ToLower("e2e-" + runID + "-" + role)
	name = invalidClusterCharacter.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

func writeKindConfig(path string, requirement shared.ClusterRequirement) error {
	var config strings.Builder
	config.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n")
	for range requirement.ControlNodes {
		config.WriteString("- role: control-plane\n")
	}
	for range requirement.WorkerNodes {
		config.WriteString("- role: worker\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create Kind config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(config.String()), 0o600); err != nil {
		return fmt.Errorf("write Kind config: %w", err)
	}
	return nil
}
