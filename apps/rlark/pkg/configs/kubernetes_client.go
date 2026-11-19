package configs

import (
	"cmp"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesClientConfig holds configuration options.
type KubernetesClientConfig struct {
	// KubeconfigPath is the path to the kubeconfig file for connecting to the Kubernetes cluster.
	KubeconfigPath string

	// Master is the address of the Kubernetes API server. If not specified, the client will use the default in-cluster configuration or kubeconfig file configuration.
	Master string

	// InCluster indicates whether to use in-cluster configuration.
	InCluster bool

	// Namespace is the Kubernetes namespace to configure the client for. If empty, the client will use the default namespace.
	Namespace string

	// QPS is the maximum number of queries per second allowed for the Kubernetes client.
	QPS float32

	// Burst is the maximum burst for throttle when connecting to the Kubernetes cluster.
	Burst int

	// Timeout is the timeout for Kubernetes client requests (optional).
	Timeout time.Duration
}

// DefaultKubernetesClientConfig returns the default kubernetesClientConfig.
func DefaultKubernetesClientConfig() KubernetesClientConfig {
	return KubernetesClientConfig{
		KubeconfigPath: os.Getenv("KUBECONFIG"),
		QPS:            5000,
		Burst:          8000,
	}
}

// SetupFlags sets the upFlags.
func (c *KubernetesClientConfig) SetupFlags(fs *pflag.FlagSet) {
	// Kubernetes client flags
	fs.StringVar(&c.KubeconfigPath, "kubeconfig", c.KubeconfigPath, "Path to kubeconfig file (if not using in-cluster config)")
	fs.StringVar(&c.Master, "master", c.Master, "The address of the Kubernetes API server (overrides kubeconfig)")
	fs.BoolVar(&c.InCluster, "in-cluster", c.InCluster, "Use in-cluster Kubernetes configuration")
	fs.StringVar(&c.Namespace, "kube-namespace", c.Namespace, "Kubernetes namespace to configure the client for")
	fs.Float32Var(&c.QPS, "kube-qps", c.QPS, "Kubernetes client QPS")
	fs.IntVar(&c.Burst, "kube-burst", c.Burst, "Kubernetes client burst")
	fs.DurationVar(&c.Timeout, "kube-timeout", c.Timeout, "Kubernetes client request timeout")
}

// DefaultNamespace returns the default namespace.
func (c KubernetesClientConfig) DefaultNamespace() string {
	return cmp.Or(c.Namespace, "default")
}

// BuildRestConfig builds the restConfig.
func (c KubernetesClientConfig) BuildRestConfig() (*rest.Config, error) {
	var restConfig *rest.Config
	var err error
	if c.InCluster {
		restConfig, err = rest.InClusterConfig()
	} else {
		restConfig, err = clientcmd.BuildConfigFromFlags(c.Master, c.KubeconfigPath)
	}
	if err != nil {
		return nil, err
	}
	restConfig.QPS = c.QPS
	restConfig.Burst = c.Burst
	restConfig.Timeout = c.Timeout
	return restConfig, nil
}
