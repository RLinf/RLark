package framework

import "context"

// Requirements is declared by each scenario. Providers satisfy it but must not
// infer or override it from global environment configuration.
type Requirements struct {
	Clusters []ClusterRequirement
}

type ClusterRequirement struct {
	Role         string
	WorkerNodes  int
	ControlNodes int
}

type ProvisionedEnvironment struct {
	Clusters map[string]ClusterConfig
	Metadata map[string]string
}

// Provider owns the infrastructure lifecycle. Provision and Destroy must be
// idempotent for the same run; Collect must not expose credentials.
type Provider interface {
	Provision(context.Context, *Run, Requirements) (*ProvisionedEnvironment, error)
	Collect(context.Context, *Run, *ProvisionedEnvironment) error
	Destroy(context.Context, *Run, *ProvisionedEnvironment) error
}
