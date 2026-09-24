package types

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"go.yaml.in/yaml/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
)

// Plane identifies a deployment plane.
type Plane string

// Constants used by the package.
const (
	PlaneControl Plane = "control"
	PlaneData    Plane = "data"
)

// StorageType represents a storage type.
type StorageType string

// Constants used by the package.
const (
	StorageEmptyDir StorageType = "emptyDir"
	StorageHostPath StorageType = "hostPath"
	StoragePVC      StorageType = "pvc"
)

// DeployConfig holds configuration options.
type DeployConfig struct {
	APIVersion          string `json:"apiVersion,omitempty" yaml:"apiVersion"`
	Kind                string `json:"kind,omitempty" yaml:"kind"`
	Plane               Plane  `json:"plane,omitempty" yaml:"plane"`
	ControlPlaneAddress string `json:"controlPlaneAddress,omitempty" yaml:"control-plane-address,omitempty"`
	// SSHAddress is the control-plane Server SSH address used for cross-cluster
	// networking, in the form "user@host:port" (e.g. "client@rlark-server:2222").
	// When empty it is auto-derived from ControlPlaneAddress.
	SSHAddress            string         `json:"sshAddress,omitempty" yaml:"ssh-address,omitempty"`
	DB                    *DBConfig      `json:"db,omitempty" yaml:"db,omitempty"`
	Kubernetes            *KubernetesEnv `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
	Docker                *DockerEnv     `json:"docker,omitempty" yaml:"docker,omitempty"`
	Raw                   *RawEnv        `json:"raw,omitempty" yaml:"raw,omitempty"`
	Cert                  *CertConfig    `json:"cert,omitempty" yaml:"cert,omitempty"`
	InsecureSkipTLSVerify bool           `json:"insecureSkipTlsVerify,omitempty" yaml:"insecure-skip-tls-verify,omitempty"`
}

// KubernetesEnv holds environment configuration.
type KubernetesEnv struct {
	ManagementAPI          string `json:"managementApi,omitempty" yaml:"management-api,omitempty"`
	Kubeconfig             string `json:"kubeconfig,omitempty" yaml:"kubeconfig,omitempty"`
	GatewayImage           string `json:"gatewayImage,omitempty" yaml:"gateway-image"`
	ControllerManagerImage string `json:"controllerManagerImage,omitempty" yaml:"controller-manager-image"`
	ServerImage            string `json:"serverImage,omitempty" yaml:"server-image"`
	AgentImage             string `json:"agentImage,omitempty" yaml:"agent-image"`
	Image                  string `json:"image,omitempty" yaml:"image,omitempty"`
	KCPImage               string `json:"kcpImage,omitempty" yaml:"kcp-image,omitempty"`
	EtcdImage              string `json:"etcdImage,omitempty" yaml:"etcd-image,omitempty"`
	PostgresqlImage        string `json:"postgresqlImage,omitempty" yaml:"postgresql-image,omitempty"`
	UIImage                string `json:"uiImage,omitempty" yaml:"ui-image,omitempty"`
	// ImagePullPolicy is the pull policy applied to all control/data plane
	// component containers. One of Always, IfNotPresent, Never. Defaults to
	// Always when empty.
	ImagePullPolicy  string           `json:"imagePullPolicy,omitempty" yaml:"image-pull-policy,omitempty"`
	ImagePullSecrets []string         `json:"imagePullSecrets,omitempty" yaml:"image-pull-secrets,omitempty"`
	Replicas         int32            `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Storage          *StorageConfig   `json:"storage,omitempty" yaml:"storage,omitempty"`
	KCP              *ComponentConfig `json:"kcp,omitempty" yaml:"kcp,omitempty"`
	Etcd             *EtcdConfig      `json:"etcd,omitempty" yaml:"etcd,omitempty"`
	Postgresql       *ComponentConfig `json:"postgresql,omitempty" yaml:"postgresql,omitempty"`
	// ContainerdSocket is the host path to the containerd socket used by the
	// node-agent for image pre-pull progress monitoring. Defaults to
	// /run/containerd/containerd.sock when empty. Set this for non-standard
	// runtimes such as k3s (/run/k3s/containerd/containerd.sock).
	ContainerdSocket string `json:"containerdSocket,omitempty" yaml:"containerd-socket,omitempty"`
}

// ComponentConfig holds configuration options.
type ComponentConfig struct {
	Replicas int32          `yaml:"replicas,omitempty"`
	Storage  *StorageConfig `yaml:"storage,omitempty"`
}

// EtcdConfig holds configuration options.
type EtcdConfig struct {
	Address  string         `yaml:"address,omitempty"` // 外部 etcd 地址，如 https://etcd.example.com:2379
	Replicas int32          `yaml:"replicas,omitempty"`
	Storage  *StorageConfig `yaml:"storage,omitempty"`
}

// StorageConfig holds configuration options.
type StorageConfig struct {
	Type         StorageType       `yaml:"type"`
	HostPath     string            `yaml:"host-path,omitempty"`
	StorageClass string            `yaml:"storage-class,omitempty"`
	Size         string            `yaml:"size,omitempty"`
	NodeSelector map[string]string `yaml:"node-selector,omitempty"`
}

// DockerEnv holds environment configuration.
type DockerEnv struct {
	GatewayImage           string `yaml:"gateway-image"`
	ControllerManagerImage string `yaml:"controller-manager-image"`
	ServerImage            string `yaml:"server-image"`
	AgentImage             string `yaml:"agent-image"`
	Image                  string `yaml:"image,omitempty"`
	KCPImage               string `yaml:"kcp-image,omitempty"`
	EtcdImage              string `yaml:"etcd-image,omitempty"`
	PostgresqlImage        string `yaml:"postgresql-image,omitempty"`
	UIImage                string `yaml:"ui-image,omitempty"`
	// ImagePullPolicy is the pull policy applied to all component containers.
	// One of Always, IfNotPresent, Never. Defaults to Always when empty.
	ImagePullPolicy string `yaml:"image-pull-policy,omitempty"`
}

// RawEnv holds environment configuration.
type RawEnv struct {
	GatewayArtifact           string `yaml:"gateway-artifact"`
	ControllerManagerArtifact string `yaml:"controller-manager-artifact"`
	ServerArtifact            string `yaml:"server-artifact"`
	AgentArtifact             string `yaml:"agent-artifact"`
	NetworkSidecarArtifact    string `yaml:"network-sidecar-artifact,omitempty"`
	KCPArtifact               string `yaml:"kcp-artifact,omitempty"`
	EtcdArtifact              string `yaml:"etcd-artifact,omitempty"`
	PostgresqlArtifact        string `yaml:"postgresql-artifact,omitempty"`
}

// CertConfig holds configuration options.
type CertConfig struct {
	CACert    string `yaml:"ca-cert,omitempty"`
	AgentCert string `yaml:"agent-cert,omitempty"`
	AgentKey  string `yaml:"agent-key,omitempty"`
}

// DBConfig holds configuration options.
type DBConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// Component describes a deployable component.
type Component struct {
	Name            string
	Port            int32
	Plane           Plane
	NeedsService    bool
	Headless        bool
	ParallelPodMgmt bool
	WorkloadKind    string
	ServiceAccount  string
	RBACRules       []rbacv1.PolicyRule
	RBACRulesFn     func(cfg *DeployConfig) []rbacv1.PolicyRule
	Dependencies    []string
	MetricsPort     int32
	EnabledFn       func(cfg *DeployConfig) bool
	ImageFn         func(cfg *DeployConfig) string
	ArtifactFn      func(cfg *DeployConfig) string
	CommandFn       func(cfg *DeployConfig) []string
	ArgsFn          func(cfg *DeployConfig) []string
	EnvFn           func(cfg *DeployConfig) map[string]string
	ExtraPortsFn    func(cfg *DeployConfig) []corev1.ContainerPort
	ExtraSvcPortsFn func(cfg *DeployConfig) []corev1.ServicePort
	K8sEnvFn        func(cfg *DeployConfig) []corev1.EnvVar
	VolumeFn        func(cfg *DeployConfig) ([]corev1.Volume, []corev1.VolumeMount)
	VolumeClaimFn   func(cfg *DeployConfig) []corev1.PersistentVolumeClaim
	ProbeFn         func(cfg *DeployConfig) (*corev1.Probe, *corev1.Probe)
	HealthCheckFn   func(cfg *DeployConfig) error
	PostDeployFn    func(cfg *DeployConfig) error
}

// GetName returns the name.
func (c *Component) GetName() string {
	return c.Name
}

// GetDependencies returns the dependencies.
func (c *Component) GetDependencies() []string {
	return c.Dependencies
}

// LoadDeployConfig loads the deployConfig.
func LoadDeployConfig(path string) (*DeployConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read deploy config: %w", err)
	}
	var cfg DeployConfig
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal deploy config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate validates the configuration.
func (c *DeployConfig) Validate() error {
	if c.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}

	if c.Kind != "DeployConfig" {
		return fmt.Errorf("kind must be DeployConfig, got %q", c.Kind)
	}

	if c.Plane != PlaneControl && c.Plane != PlaneData {
		return fmt.Errorf("plane must be %q or %q", PlaneControl, PlaneData)
	}

	modes := 0
	if c.Kubernetes != nil {
		modes++
	}

	if c.Docker != nil {
		modes++
	}

	if c.Raw != nil {
		modes++
	}

	if modes != 1 {
		return fmt.Errorf("exactly one of kubernetes/docker/raw must be specified")
	}

	var pullPolicy string
	if c.Kubernetes != nil {
		pullPolicy = c.Kubernetes.ImagePullPolicy
		if c.Kubernetes.KCP != nil && c.Kubernetes.KCP.Replicas > 1 {
			return fmt.Errorf("kubernetes.kcp.replicas must be 1")
		}
		switch c.Kubernetes.ManagementAPI {
		case "", "kcp", "kubernetes":
		default:
			return fmt.Errorf("kubernetes.management-api must be %q or %q, got %q", "kcp", "kubernetes", c.Kubernetes.ManagementAPI)
		}
		if c.Kubernetes.ManagementAPI == "kubernetes" && c.Plane != PlaneControl {
			return fmt.Errorf("kubernetes.management-api %q is only supported for the control plane", "kubernetes")
		}
	} else if c.Docker != nil {
		pullPolicy = c.Docker.ImagePullPolicy
	}
	switch corev1.PullPolicy(pullPolicy) {
	case "", corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever:
	default:
		return fmt.Errorf("image-pull-policy must be one of Always, IfNotPresent, Never, got %q", pullPolicy)
	}

	if c.Plane == PlaneData {
		if c.Cert == nil {
			return fmt.Errorf("cert is required for data plane")
		}
		if c.Cert.CACert == "" {
			return fmt.Errorf("cert.ca-cert is required for data plane")
		}
		if c.Cert.AgentCert == "" {
			return fmt.Errorf("cert.agent-cert is required for data plane")
		}
		if c.Cert.AgentKey == "" {
			return fmt.Errorf("cert.agent-key is required for data plane")
		}

		if c.ControlPlaneAddress == "" {
			return fmt.Errorf("control-plane-address is required for data plane")
		}
	}

	return nil
}

// UsesKubernetesManagementAPI reports whether control-plane state is stored in
// the target Kubernetes API instead of a separately deployed kcp instance.
func (c *DeployConfig) UsesKubernetesManagementAPI() bool {
	return c.Kubernetes != nil && c.Kubernetes.ManagementAPI == "kubernetes"
}

// EnvMode returns the environment mode.
func (c *DeployConfig) EnvMode() string {
	if c.Kubernetes != nil {
		return "Kubernetes"
	}
	if c.Docker != nil {
		return "Docker"
	}
	return "Raw"
}

// defaultSSHUser is the user embedded in the auto-derived Server SSH address.
const defaultSSHUser = "client"

// SSHServerAddress resolves the control-plane Server SSH address in the form
// "user@host:port". When SSHAddress is explicitly set it is returned as-is;
// otherwise it is derived from ControlPlaneAddress by extracting its host and
// combining it with the default SSH user and Server SSH port. An empty string
// is returned when neither is available.
func (c *DeployConfig) SSHServerAddress() string {
	if c.SSHAddress != "" {
		return c.SSHAddress
	}
	host := hostFromAddress(c.ControlPlaneAddress)
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s:%d", defaultSSHUser, host, constants.ServerSSHPort)
}

// hostFromAddress extracts the bare host from a control-plane address that may
// be a full URL (e.g. "https://rlark-server:8443"), a "host:port" pair, or a
// bare host. Any scheme and port are stripped.
func hostFromAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	// Full URL with scheme, e.g. https://host:8443.
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	// host:port without scheme.
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	// Bare host.
	return addr
}

// ImagePullPolicy resolves the configured image pull policy for component
// containers, defaulting to corev1.PullAlways when unset. Only the Kubernetes
// and Docker environment modes carry the setting; other modes return the
// default.
func (c *DeployConfig) ImagePullPolicy() corev1.PullPolicy {
	var raw string
	switch {
	case c.Kubernetes != nil:
		raw = c.Kubernetes.ImagePullPolicy
	case c.Docker != nil:
		raw = c.Docker.ImagePullPolicy
	}
	switch corev1.PullPolicy(raw) {
	case corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever:
		return corev1.PullPolicy(raw)
	default:
		return corev1.PullAlways
	}
}
