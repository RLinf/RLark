package framework

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type ClusterConfig struct {
	Kubeconfig string
	Context    string
}

type Cluster struct {
	Name       string
	RESTConfig *rest.Config
	Kubernetes kubernetes.Interface
}

// NewClusters initializes arbitrary named Kubernetes endpoints. It has no
// knowledge of product roles or deployment topology.
func NewClusters(configs map[string]ClusterConfig, userAgent string) (map[string]*Cluster, error) {
	clusters := make(map[string]*Cluster, len(configs))
	for name, config := range configs {
		cluster, err := NewCluster(name, config, userAgent)
		if err != nil {
			return nil, err
		}
		clusters[name] = cluster
	}
	return clusters, nil
}

func NewCluster(name string, config ClusterConfig, userAgent string) (*Cluster, error) {
	if name == "" {
		return nil, fmt.Errorf("cluster name must not be empty")
	}
	if config.Kubeconfig == "" {
		return nil, fmt.Errorf("cluster %q kubeconfig must not be empty", name)
	}
	if _, err := os.Stat(config.Kubeconfig); err != nil {
		return nil, fmt.Errorf("access cluster %q kubeconfig: %w", name, err)
	}
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: config.Context}
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load cluster %q kubeconfig: %w", name, err)
	}
	restConfig.UserAgent = userAgent
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create cluster %q Kubernetes client: %w", name, err)
	}
	return &Cluster{Name: name, RESTConfig: restConfig, Kubernetes: client}, nil
}
