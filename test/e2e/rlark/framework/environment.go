package rlarkframework

import (
	"fmt"

	rlarkclient "github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	shared "github.com/rlinf/rlark/test/e2e/framework"
)

type Environment struct {
	Config       Config
	Requirements shared.Requirements
	Provisioned  *shared.ProvisionedEnvironment
	Clusters     map[string]*shared.Cluster
	RLark        rlarkclient.Interface
}

// NewEnvironment initializes Kubernetes clients only. Product clients are
// enabled explicitly by the scenario according to its declared requirements.
func NewEnvironment(config Config, requirements shared.Requirements, provisioned *shared.ProvisionedEnvironment) (*Environment, error) {
	if provisioned == nil {
		return nil, fmt.Errorf("provisioned environment must not be nil")
	}
	clusters, err := shared.NewClusters(provisioned.Clusters, "rlark-e2e/1.0")
	if err != nil {
		return nil, err
	}
	return &Environment{Config: config, Requirements: requirements, Provisioned: provisioned, Clusters: clusters}, nil
}

func (e *Environment) EnableRLarkClient(role string) error {
	cluster, err := e.Cluster(role)
	if err != nil {
		return err
	}
	client, err := rlarkclient.NewForConfig(cluster.RESTConfig)
	if err != nil {
		return fmt.Errorf("create RLark client for cluster role %q: %w", role, err)
	}
	e.RLark = client
	return nil
}

func (e *Environment) Cluster(role string) (*shared.Cluster, error) {
	requested := false
	for _, requirement := range e.Requirements.Clusters {
		if requirement.Role == role {
			requested = true
			break
		}
	}
	if !requested {
		return nil, fmt.Errorf("cluster role %q was not requested by the scenario", role)
	}
	cluster, ok := e.Clusters[role]
	if !ok {
		return nil, fmt.Errorf("cluster role %q has no initialized client", role)
	}
	return cluster, nil
}
