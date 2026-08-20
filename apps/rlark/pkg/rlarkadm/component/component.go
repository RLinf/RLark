package component

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"go.yaml.in/yaml/v2"
)

var commonEnvs = []corev1.EnvVar{
	{
		Name: "POD_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
		},
	},
	{
		Name: "POD_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
		},
	},
	{
		Name: "NODE_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"},
		},
	},
	{
		Name: "POD_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
		},
	},
	{
		Name:  "DISABLE_HTTP2",
		Value: "true",
	},
}

// Component describes a deployable component.
type Component = types.Component

func commonArgs() []string {
	return []string{"--kubeconfig", constants.KCPKubeconfigPath}
}

func dbConfigVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{{
			Name: "db-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "rlark-db-config"},
				},
			},
		}},
		[]corev1.VolumeMount{{
			Name:      "db-config",
			MountPath: constants.DBConfigPath,
			SubPath:   "db.yaml",
			ReadOnly:  true,
		}}
}

func certVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{{
			Name: "cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "rlark-agent-cert"},
			},
		}},
		[]corev1.VolumeMount{{
			Name:      "cert",
			MountPath: constants.CertDir,
			ReadOnly:  true,
		}}
}

func kcpDataVolume(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
	storage := resolveComponentStorage(cfg, constants.ComponentKCP)
	switch storage.Type {
	case "", types.StorageEmptyDir:
		return []corev1.Volume{{
				Name: "kcp-data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}},
			[]corev1.VolumeMount{{
				Name:      "kcp-data",
				MountPath: constants.KCPEtcdDataDir,
			}}
	case types.StorageHostPath:
		return []corev1.Volume{{
				Name: "kcp-data",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: storage.HostPath,
						Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
					},
				},
			}},
			[]corev1.VolumeMount{{
				Name:      "kcp-data",
				MountPath: constants.KCPEtcdDataDir,
			}}
	default:
		return nil, []corev1.VolumeMount{{
			Name:      "kcp-data",
			MountPath: constants.KCPEtcdDataDir,
		}}
	}
}

func kcpVolumeClaim(cfg *types.DeployConfig) []corev1.PersistentVolumeClaim {
	storage := resolveComponentStorage(cfg, constants.ComponentKCP)
	if storage.Type != types.StoragePVC {
		return nil
	}
	size := storage.Size
	if size == "" {
		size = "10Gi"
	}
	return []corev1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{Name: "kcp-data"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: stringPtr(storage.StorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: mustParseQuantity(size)},
			},
		},
	}}
}

func resolveEtcdConfig(cfg *types.DeployConfig) *types.EtcdConfig {
	if cfg.Kubernetes == nil || cfg.Kubernetes.Etcd == nil {
		return &types.EtcdConfig{}
	}
	return cfg.Kubernetes.Etcd
}

func resolveEtcdReplicas(cfg *types.DeployConfig) int32 {
	if cfg.Kubernetes == nil {
		return 1
	}
	ec := resolveEtcdConfig(cfg)
	if ec.Replicas != 0 {
		return ec.Replicas
	}
	if cfg.Kubernetes.Replicas != 0 {
		return cfg.Kubernetes.Replicas
	}
	return 1
}

func resolveEtcdStorage(cfg *types.DeployConfig) *types.StorageConfig {
	if cfg.Kubernetes == nil {
		return &types.StorageConfig{}
	}
	ec := resolveEtcdConfig(cfg)
	if ec.Storage != nil {
		return ec.Storage
	}
	if cfg.Kubernetes.Storage != nil {
		return cfg.Kubernetes.Storage
	}
	return &types.StorageConfig{}
}

func etcdConfigured(cfg *types.DeployConfig) bool {
	return cfg.Kubernetes != nil && cfg.Kubernetes.Etcd != nil
}

func etcdAddress(cfg *types.DeployConfig) string {
	if !etcdConfigured(cfg) {
		return ""
	}
	return cfg.Kubernetes.Etcd.Address
}

func etcdEnabled(cfg *types.DeployConfig) bool {
	return etcdConfigured(cfg) && etcdAddress(cfg) == ""
}

func etcdReplicas(cfg *types.DeployConfig) int32 {
	return resolveEtcdReplicas(cfg)
}

func etcdInitialClusterDNS(cfg *types.DeployConfig) string {
	replicas := etcdReplicas(cfg)
	var members []string
	for i := int32(0); i < replicas; i++ {
		name := fmt.Sprintf("%s-%d", constants.ComponentEtcd, i)
		peerURL := fmt.Sprintf("http://%s.%s.%s.svc:%d",
			name, constants.ComponentEtcd, constants.Namespace, constants.EtcdPeerPort)
		members = append(members, fmt.Sprintf("%s=%s", name, peerURL))
	}
	return joinStrings(members, ",")
}

func etcdDataVolume(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
	storage := resolveEtcdStorage(cfg)
	switch storage.Type {
	case "", types.StorageEmptyDir:
		return []corev1.Volume{{
				Name: "etcd-data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}},
			[]corev1.VolumeMount{{
				Name:      "etcd-data",
				MountPath: constants.EtcdDataDir,
			}}
	case types.StorageHostPath:
		return []corev1.Volume{{
				Name: "etcd-data",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: storage.HostPath,
						Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
					},
				},
			}},
			[]corev1.VolumeMount{{
				Name:      "etcd-data",
				MountPath: constants.EtcdDataDir,
			}}
	default:
		return nil, []corev1.VolumeMount{{
			Name:      "etcd-data",
			MountPath: constants.EtcdDataDir,
		}}
	}
}

func etcdVolumeClaim(cfg *types.DeployConfig) []corev1.PersistentVolumeClaim {
	storage := resolveEtcdStorage(cfg)
	if storage.Type != types.StoragePVC {
		return nil
	}
	size := storage.Size
	if size == "" {
		size = "8Gi"
	}
	return []corev1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{Name: "etcd-data"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: stringPtr(storage.StorageClass),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: mustParseQuantity(size)},
			},
		},
	}}
}

func resolveComponentConfig(cfg *types.DeployConfig, name string) *types.ComponentConfig {
	if cfg.Kubernetes == nil {
		return &types.ComponentConfig{}
	}
	switch name {
	case constants.ComponentKCP:
		if cfg.Kubernetes.KCP != nil {
			return cfg.Kubernetes.KCP
		}
	case constants.ComponentPostgresql:
		if cfg.Kubernetes.Postgresql != nil {
			return cfg.Kubernetes.Postgresql
		}
	case constants.ComponentEtcd:
		if cfg.Kubernetes.Etcd != nil {
			return &types.ComponentConfig{
				Replicas: cfg.Kubernetes.Etcd.Replicas,
				Storage:  cfg.Kubernetes.Etcd.Storage,
			}
		}
	}
	return &types.ComponentConfig{}
}

func resolveComponentStorage(cfg *types.DeployConfig, name string) *types.StorageConfig {
	if cfg.Kubernetes == nil {
		return &types.StorageConfig{}
	}
	cc := resolveComponentConfig(cfg, name)
	if cc.Storage != nil {
		return cc.Storage
	}
	if cfg.Kubernetes.Storage != nil {
		return cfg.Kubernetes.Storage
	}
	return &types.StorageConfig{}
}

func resolveComponentReplicas(cfg *types.DeployConfig, name string) int32 {
	if cfg.Kubernetes == nil {
		return 1
	}
	cc := resolveComponentConfig(cfg, name)
	if cc.Replicas != 0 {
		return cc.Replicas
	}
	if cfg.Kubernetes.Replicas != 0 {
		return cfg.Kubernetes.Replicas
	}
	return 1
}

func leaderElectFlag(cfg *types.DeployConfig, name string) string {
	if resolveComponentReplicas(cfg, name) > 1 {
		return "true"
	}
	return "false"
}

func resolveComponentNodeSelector(cfg *types.DeployConfig, name string) map[string]string {
	storage := resolveComponentStorage(cfg, name)
	return storage.NodeSelector
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.MustParse("10Gi")
	}
	return q
}

func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for _, s := range items[1:] {
		result += sep + s
	}
	return result
}

func kubeconfigVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{{
			Name: "kcp-kubeconfig",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "rlark-kcp-kubeconfig"},
				},
			},
		}},
		[]corev1.VolumeMount{{
			Name:      "kcp-kubeconfig",
			MountPath: constants.KCPKubeconfigPath,
			SubPath:   "admin.kubeconfig",
			ReadOnly:  true,
		}}
}

func postgresqlDataVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{{
			Name: "pg-data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}},
		[]corev1.VolumeMount{{
			Name:      "pg-data",
			MountPath: constants.PostgresqlDataDir,
		}}
}

func postgresqlInitVolume() ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{{
			Name: "pg-init",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "rlark-postgres-init"},
				},
			},
		}},
		[]corev1.VolumeMount{{
			Name:      "pg-init",
			MountPath: constants.PostgresqlInitDir,
			ReadOnly:  true,
		}}
}

// 全局部署配置，后续新部署的组件往上添加，也可通过配置文件控制.
var components = []types.Component{
	{
		Name: constants.ComponentGateway, Port: 8090, Plane: types.PlaneControl, NeedsService: true,
		Dependencies:  []string{constants.ComponentKCP, constants.ComponentServer},
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentGateway}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.GatewayImage }, func(d *types.DockerEnv) string { return d.GatewayImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.GatewayArtifact },
		CommandFn: func(cfg *types.DeployConfig) []string {
			return []string{"/usr/local/bin/gateway"}
		},
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{"--addr=:8090"}
			if cfg.DB != nil {
				args = append(args, "--db-config="+constants.DBConfigPath)
			}
			return append(args, commonArgs()...)
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			vols, mounts := kubeconfigVolume()
			if cfg.DB != nil {
				dv, dm := dbConfigVolume()
				vols = append(vols, dv...)
				mounts = append(mounts, dm...)
			}
			return vols, mounts
		},
	},
	{
		Name: constants.ComponentControllerManager, Port: 8081, Plane: types.PlaneControl,
		MetricsPort:   8080,
		Dependencies:  []string{constants.ComponentKCP},
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentControllerManager}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.ControllerManagerImage }, func(d *types.DockerEnv) string { return d.ControllerManagerImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.ControllerManagerArtifact },
		CommandFn: func(cfg *types.DeployConfig) []string {
			return []string{"/usr/local/bin/controller-manager"}
		},
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{
				"--metrics-bind-address=:8080",
				"--health-probe-bind-address=:8081",
				"--leader-elect=" + leaderElectFlag(cfg, constants.ComponentControllerManager),
			}
			if cfg.DB != nil {
				args = append(args, "--db-config="+constants.DBConfigPath)
			}
			return append(args, commonArgs()...)
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			vols, mounts := kubeconfigVolume()
			if cfg.DB != nil {
				dv, dm := dbConfigVolume()
				vols = append(vols, dv...)
				mounts = append(mounts, dm...)
			}
			return vols, mounts
		},
	},
	{
		Name: constants.ComponentServer, Port: 8443, Plane: types.PlaneControl, NeedsService: true,
		MetricsPort:   8888,
		Dependencies:  []string{constants.ComponentKCP},
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentServer}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.ServerImage }, func(d *types.DockerEnv) string { return d.ServerImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.ServerArtifact },
		CommandFn: func(cfg *types.DeployConfig) []string {
			return []string{"/usr/local/bin/server"}
		},
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{
				"--https-port=8443",
				"--unsafe-http-port=8888",
				"--ssh-port=2222",
				"--auto-sign-tls-ca-cert",
				"--tls-domains=localhost,rlark-server,rlark-server." + constants.Namespace + ",rlark-server." + constants.Namespace + ".svc",
			}
			if cfg.DB != nil {
				args = append(args, "--db-config="+constants.DBConfigPath)
			}
			return append(args, commonArgs()...)
		},
		ExtraSvcPortsFn: func(cfg *types.DeployConfig) []corev1.ServicePort {
			return []corev1.ServicePort{
				{
					Name:       "ssh",
					Port:       constants.ServerSSHPort,
					TargetPort: intstr.FromInt32(constants.ServerSSHPort),
				},
			}
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			vols, mounts := kubeconfigVolume()
			if cfg.DB != nil {
				dv, dm := dbConfigVolume()
				vols = append(vols, dv...)
				mounts = append(mounts, dm...)
			}
			return vols, mounts
		},
	},
	{
		Name: constants.ComponentAgent, Port: 8081, Plane: types.PlaneData,
		MetricsPort:    8081,
		HealthCheckFn:  health.ModeHealthCheck(types.Component{Name: constants.ComponentAgent}),
		ServiceAccount: "rlark-agent",
		RBACRules: []rbacv1.PolicyRule{
			{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
		},
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.AgentImage }, func(d *types.DockerEnv) string { return d.AgentImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.AgentArtifact },
		CommandFn: func(cfg *types.DeployConfig) []string {
			return []string{"/usr/local/bin/agent"}
		},
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{
				"--server-address=" + cfg.ControlPlaneAddress,
				"--agent-type=" + cfg.EnvMode(),
				"--client-cert=" + constants.CertDir + "/tls.crt",
				"--client-key=" + constants.CertDir + "/tls.key",
				"--ca-cert=" + constants.CertDir + "/ca.crt",
				"--leader-election=" + leaderElectFlag(cfg, constants.ComponentAgent),
				"--mode=cluster",
			}

			if img := rlarkImage(cfg); img != "" {
				args = append(args, "--image="+img)
			}

			if cfg.Kubernetes != nil {
				args = append(args, "--in-cluster")
			}

			if cfg.InsecureSkipTLSVerify {
				args = append(args, "--insecure-skip-tls-verify")
			}

			return args
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			return certVolume()
		},
	},
	{
		Name: constants.ComponentAgentNode, Port: 8081, Plane: types.PlaneData, WorkloadKind: "DaemonSet",
		HealthCheckFn:  health.ModeHealthCheck(types.Component{Name: constants.ComponentAgentNode}),
		ServiceAccount: "rlark-agent",
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.AgentImage }, func(d *types.DockerEnv) string { return d.AgentImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.AgentArtifact },
		CommandFn: func(cfg *types.DeployConfig) []string {
			return []string{"/usr/local/bin/agent"}
		},
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{
				"--server-address=" + cfg.ControlPlaneAddress,
				"--agent-type=" + cfg.EnvMode(),
				"--client-cert=" + constants.CertDir + "/tls.crt",
				"--client-key=" + constants.CertDir + "/tls.key",
				"--ca-cert=" + constants.CertDir + "/ca.crt",
				"--leader-election=false",
				"--mode=node",
				"--rlark-server-ssh-address=client@" + cfg.ControlPlaneAddress + ":" + strconv.Itoa(constants.ServerSSHPort),
				// Enable node-level image pre-pulling (containerd/docker). The
				// node-agent mounts the container runtime socket below.
				"--image-pull-enabled=true",
			}

			if cfg.Kubernetes != nil {
				args = append(args, "--in-cluster")
				// Override the containerd socket path for non-standard runtimes
				// such as k3s (/run/k3s/containerd/containerd.sock).
				if cfg.Kubernetes.ContainerdSocket != "" {
					args = append(args, "--containerd-socket="+cfg.Kubernetes.ContainerdSocket)
				}
			}

			if cfg.InsecureSkipTLSVerify {
				args = append(args, "--insecure-skip-tls-verify")
			}

			return args
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			vols, mounts := certVolume()
			vols = append(vols, corev1.Volume{
				Name: "nodeserver-socket",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/run/rlark",
						Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
					},
				},
			})
			mounts = append(mounts, corev1.VolumeMount{
				Name:      "nodeserver-socket",
				MountPath: "/var/run/rlark",
			})
			// Mount the host containerd socket so the node-agent can invoke
			// `ctr` to pre-pull images for Kubernetes/containerd nodes.
			// When a custom socket path is configured (e.g. k3s uses
			// /run/k3s/containerd/containerd.sock), mount its parent
			// directory instead of the default /run/containerd.
			socketHostDir := "/run/containerd"
			if cfg.Kubernetes != nil && cfg.Kubernetes.ContainerdSocket != "" {
				socketHostDir = filepath.Dir(cfg.Kubernetes.ContainerdSocket)
			}
			vols = append(vols, corev1.Volume{
				Name: "containerd-socket",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: socketHostDir,
						Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
					},
				},
			})
			mounts = append(mounts, corev1.VolumeMount{
				Name:      "containerd-socket",
				MountPath: socketHostDir,
			})
			return vols, mounts
		},
	},
	{
		Name: constants.ComponentEtcd, Port: constants.EtcdClientPort, Plane: types.PlaneControl,
		NeedsService:    true,
		Headless:        true,
		ParallelPodMgmt: true,
		WorkloadKind:    "StatefulSet",
		MetricsPort:     constants.EtcdMetricsPort,
		EnabledFn:       func(cfg *types.DeployConfig) bool { return etcdEnabled(cfg) },
		HealthCheckFn:   health.ModeHealthCheck(types.Component{Name: constants.ComponentEtcd}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.EtcdImage }, func(d *types.DockerEnv) string { return d.EtcdImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.EtcdArtifact },
		CommandFn:  func(cfg *types.DeployConfig) []string { return []string{"etcd"} },
		K8sEnvFn: func(cfg *types.DeployConfig) []corev1.EnvVar {
			return commonEnvs
		},
		ExtraPortsFn: func(cfg *types.DeployConfig) []corev1.ContainerPort {
			return []corev1.ContainerPort{{ContainerPort: constants.EtcdPeerPort}}
		},
		ExtraSvcPortsFn: func(cfg *types.DeployConfig) []corev1.ServicePort {
			return []corev1.ServicePort{
				{
					Name:       "peer",
					Port:       constants.EtcdPeerPort,
					TargetPort: intstr.FromInt32(constants.EtcdPeerPort),
				},
			}
		},
		ArgsFn: func(cfg *types.DeployConfig) []string {
			peerPort := strconv.Itoa(constants.EtcdPeerPort)
			clientPort := strconv.Itoa(constants.EtcdClientPort)
			metricsPort := strconv.Itoa(constants.EtcdMetricsPort)

			args := []string{
				"--name=$(POD_NAME)",
				"--listen-peer-urls=http://0.0.0.0:" + peerPort,
				"--listen-client-urls=http://0.0.0.0:" + clientPort,
				"--initial-cluster-token=" + constants.EtcdClusterToken,
				"--initial-cluster-state=new",
				"--listen-metrics-urls=http://0.0.0.0:" + metricsPort,
				"--auto-compaction-mode=periodic",
				"--auto-compaction-retention=1h",
				"--data-dir=" + constants.EtcdDataDir,
			}

			if etcdReplicas(cfg) == 1 {
				args = append(args,
					"--initial-advertise-peer-urls=http://$(POD_IP):"+peerPort,
					"--advertise-client-urls=http://$(POD_IP):"+clientPort,
					"--initial-cluster=$(POD_NAME)=http://$(POD_IP):"+peerPort,
				)
			} else {
				dnsHost := "$(POD_NAME)." + constants.ComponentEtcd + "." + constants.Namespace + ".svc"
				args = append(args,
					"--initial-advertise-peer-urls=http://"+dnsHost+":"+peerPort,
					"--advertise-client-urls=http://"+dnsHost+":"+clientPort,
					"--initial-cluster="+etcdInitialClusterDNS(cfg),
				)
			}

			return args
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			return etcdDataVolume(cfg)
		},
		VolumeClaimFn: func(cfg *types.DeployConfig) []corev1.PersistentVolumeClaim {
			return etcdVolumeClaim(cfg)
		},
		ProbeFn: func(cfg *types.DeployConfig) (*corev1.Probe, *corev1.Probe) {
			liveness := &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/livez",
						Port: intstr.FromInt32(constants.EtcdMetricsPort),
					},
				},
				InitialDelaySeconds: 15,
				PeriodSeconds:       10,
				TimeoutSeconds:      5,
				FailureThreshold:    3,
			}
			readiness := &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/readyz",
						Port: intstr.FromInt32(constants.EtcdMetricsPort),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				TimeoutSeconds:      5,
				SuccessThreshold:    1,
				FailureThreshold:    30,
			}
			return liveness, readiness
		},
	},
	{
		Name: constants.ComponentKCP, Port: 6443, Plane: types.PlaneControl, NeedsService: true,
		MetricsPort:   8080,
		Dependencies:  []string{constants.ComponentEtcd},
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentKCP}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.KCPImage }, func(d *types.DockerEnv) string { return d.KCPImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.KCPArtifact },
		ArgsFn: func(cfg *types.DeployConfig) []string {
			args := []string{"start"}
			if addr := etcdAddress(cfg); addr != "" {
				args = append(args, "--etcd-servers="+addr)
			} else if etcdEnabled(cfg) {
				args = append(args, fmt.Sprintf("--etcd-servers=http://%s.%s.svc:%d",
					constants.ComponentEtcd, constants.Namespace, constants.EtcdClientPort))
			}
			return args
		},
		EnvFn: func(cfg *types.DeployConfig) map[string]string {
			return map[string]string{"KCP_LOG_LEVEL": "2"}
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			if etcdConfigured(cfg) {
				return nil, nil
			}
			return kcpDataVolume(cfg)
		},
		VolumeClaimFn: func(cfg *types.DeployConfig) []corev1.PersistentVolumeClaim {
			if etcdConfigured(cfg) {
				return nil
			}
			return kcpVolumeClaim(cfg)
		},
		ProbeFn: func(cfg *types.DeployConfig) (*corev1.Probe, *corev1.Probe) {
			liveness := &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/livez",
						Port:   intstr.FromInt32(6443),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				InitialDelaySeconds: 45,
				PeriodSeconds:       10,
				TimeoutSeconds:      10,
				FailureThreshold:    6,
				SuccessThreshold:    1,
			}
			readiness := &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/readyz",
						Port:   intstr.FromInt32(6443),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				FailureThreshold: 6,
			}
			return liveness, readiness
		},
	},
	{
		Name: constants.ComponentPostgresql, Port: 5432, Plane: types.PlaneControl, NeedsService: true,
		EnabledFn:     func(cfg *types.DeployConfig) bool { return cfg.DB != nil },
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentPostgresql}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.PostgresqlImage }, func(d *types.DockerEnv) string { return d.PostgresqlImage })
		},
		ArtifactFn: func(cfg *types.DeployConfig) string { return cfg.Raw.PostgresqlArtifact },
		ArgsFn:     func(cfg *types.DeployConfig) []string { return []string{} },
		EnvFn: func(cfg *types.DeployConfig) map[string]string {
			return map[string]string{
				"POSTGRES_USER":     cfg.DB.User,
				"POSTGRES_PASSWORD": cfg.DB.Password,
				"POSTGRES_DB":       cfg.DB.Database,
			}
		},
		VolumeFn: func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
			dv, dm := postgresqlDataVolume()
			iv, im := postgresqlInitVolume()
			return append(dv, iv...), append(dm, im...)
		},
	},
	{
		Name: constants.ComponentUI, Port: 80, Plane: types.PlaneControl, NeedsService: true,
		Dependencies:  []string{constants.ComponentGateway},
		HealthCheckFn: health.ModeHealthCheck(types.Component{Name: constants.ComponentUI}),
		ImageFn: func(cfg *types.DeployConfig) string {
			return imageByMode(cfg, func(k *types.KubernetesEnv) string { return k.UIImage }, func(d *types.DockerEnv) string { return d.UIImage })
		},
		ArgsFn: func(cfg *types.DeployConfig) []string { return []string{} },
	},
}

func imageByMode(cfg *types.DeployConfig, k8sFn func(*types.KubernetesEnv) string, dockerFn func(*types.DockerEnv) string) string {
	switch cfg.EnvMode() {
	case "Kubernetes":
		if cfg.Kubernetes != nil {
			return k8sFn(cfg.Kubernetes)
		}
	case "Docker":
		if cfg.Docker != nil {
			return dockerFn(cfg.Docker)
		}
	}
	return ""
}

func rlarkImage(cfg *types.DeployConfig) string {
	switch cfg.EnvMode() {
	case "Kubernetes":
		if cfg.Kubernetes != nil {
			return cfg.Kubernetes.Image
		}
	case "Docker":
		if cfg.Docker != nil {
			return cfg.Docker.Image
		}
	}
	return ""
}

// ComponentsForPlane returns the components for the given plane.
func ComponentsForPlane(cfg *types.DeployConfig) []types.Component {
	logger := log.GetLogger()
	var result []types.Component
	var topos []utils.Topo
	for i := range components {
		c := &components[i]
		if c.Plane != cfg.Plane {
			continue
		}
		if c.EnabledFn != nil && !c.EnabledFn(cfg) {
			continue
		}
		result = append(result, *c)
		topos = append(topos, c)
	}

	sorted, err := utils.TopologicalSort(topos)
	if err != nil {
		logger.Error(nil, "topological sort failed, using original order", "err", err)
		return result
	}

	var sortedComps []types.Component
	for _, t := range sorted {
		sortedComps = append(sortedComps, *t.(*types.Component))
	}
	return sortedComps
}

// Deployment returns a Deployment for the component.
func Deployment(cfg *types.DeployConfig, c *types.Component) *appsv1.Deployment {
	labels := map[string]string{"app": c.Name}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: constants.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utils.Ptr(resolveComponentReplicas(cfg, c.Name)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccountName(c),
					Containers: []corev1.Container{{
						Name:            c.Name,
						Image:           c.ImageFn(cfg),
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{{
							ContainerPort: c.Port,
						}},
						Args: c.ArgsFn(cfg),
					}},
				},
			},
		},
	}

	if c.CommandFn != nil {
		dep.Spec.Template.Spec.Containers[0].Command = c.CommandFn(cfg)
	}

	if c.K8sEnvFn != nil {
		dep.Spec.Template.Spec.Containers[0].Env = append(dep.Spec.Template.Spec.Containers[0].Env, c.K8sEnvFn(cfg)...)
	}

	if c.EnvFn != nil {
		envMap := c.EnvFn(cfg)
		var envs []corev1.EnvVar
		for k, v := range envMap {
			envs = append(envs, corev1.EnvVar{Name: k, Value: v})
		}
		dep.Spec.Template.Spec.Containers[0].Env = envs
	}

	if c.VolumeFn != nil {
		vols, mounts := c.VolumeFn(cfg)
		dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes, vols...)
		dep.Spec.Template.Spec.Containers[0].VolumeMounts = append(dep.Spec.Template.Spec.Containers[0].VolumeMounts, mounts...)
	}

	if c.VolumeClaimFn != nil {
		claims := c.VolumeClaimFn(cfg)
		for _, claim := range claims {
			vol := corev1.Volume{
				Name: claim.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claim.Name,
					},
				},
			}
			dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes, vol)
		}

		if len(claims) > 0 {
			uid := int64(0)
			dep.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsUser: &uid,
				FSGroup:   &uid,
			}
		}
	}

	if c.ProbeFn != nil {
		liveness, readiness := c.ProbeFn(cfg)
		if liveness != nil {
			dep.Spec.Template.Spec.Containers[0].LivenessProbe = liveness
		}
		if readiness != nil {
			dep.Spec.Template.Spec.Containers[0].ReadinessProbe = readiness
		}
	}

	return dep
}

// DaemonSet returns a DaemonSet for the component.
func DaemonSet(cfg *types.DeployConfig, c *types.Component) *appsv1.DaemonSet {
	labels := map[string]string{"app": c.Name}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: constants.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccountName(c),
					Containers: []corev1.Container{{
						Name:            c.Name,
						Image:           c.ImageFn(cfg),
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{{
							ContainerPort: c.Port,
						}},
						Args: c.ArgsFn(cfg),
						SecurityContext: &corev1.SecurityContext{
							Privileged: utils.Ptr(true),
						},
					}},
					HostPID: true,
				},
			},
		},
	}

	if c.CommandFn != nil {
		ds.Spec.Template.Spec.Containers[0].Command = c.CommandFn(cfg)
	}

	if c.K8sEnvFn != nil {
		ds.Spec.Template.Spec.Containers[0].Env = append(ds.Spec.Template.Spec.Containers[0].Env, c.K8sEnvFn(cfg)...)
	}

	if c.EnvFn != nil {
		envMap := c.EnvFn(cfg)
		var envs []corev1.EnvVar
		for k, v := range envMap {
			envs = append(envs, corev1.EnvVar{Name: k, Value: v})
		}
		ds.Spec.Template.Spec.Containers[0].Env = envs
	}

	// K8sEnvFn injects downward-API / FieldRef env vars (e.g. NODE_NAME for the
	// node-agent image-pull feature). Mirrors the StatefulSet behavior.
	if c.K8sEnvFn != nil {
		ds.Spec.Template.Spec.Containers[0].Env = append(ds.Spec.Template.Spec.Containers[0].Env, c.K8sEnvFn(cfg)...)
	}

	if c.VolumeFn != nil {
		vols, mounts := c.VolumeFn(cfg)
		ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, vols...)
		ds.Spec.Template.Spec.Containers[0].VolumeMounts = append(ds.Spec.Template.Spec.Containers[0].VolumeMounts, mounts...)
	}

	return ds
}

// StatefulSet returns a StatefulSet for the component.
func StatefulSet(cfg *types.DeployConfig, c *types.Component) *appsv1.StatefulSet {
	labels := map[string]string{"app": c.Name}

	ports := []corev1.ContainerPort{{
		ContainerPort: c.Port,
	}}

	if c.MetricsPort != 0 && c.MetricsPort != c.Port {
		ports = append(ports, corev1.ContainerPort{ContainerPort: c.MetricsPort})
	}

	if c.ExtraPortsFn != nil {
		ports = append(ports, c.ExtraPortsFn(cfg)...)
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: constants.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    utils.Ptr(resolveComponentReplicas(cfg, c.Name)),
			ServiceName: c.Name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccountName(c),
					Containers: []corev1.Container{{
						Name:            c.Name,
						Image:           c.ImageFn(cfg),
						ImagePullPolicy: corev1.PullAlways,
						Ports:           ports,
						Args:            c.ArgsFn(cfg),
					}},
				},
			},
		},
	}

	if c.CommandFn != nil {
		sts.Spec.Template.Spec.Containers[0].Command = c.CommandFn(cfg)
	}

	if c.ParallelPodMgmt {
		sts.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
	}

	if c.K8sEnvFn != nil {
		sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, c.K8sEnvFn(cfg)...)
	}

	if c.EnvFn != nil {
		envMap := c.EnvFn(cfg)
		var envs []corev1.EnvVar
		for k, v := range envMap {
			envs = append(envs, corev1.EnvVar{Name: k, Value: v})
		}
		sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, envs...)
	}

	if c.VolumeFn != nil {
		vols, mounts := c.VolumeFn(cfg)
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, vols...)
		sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, mounts...)
	}

	if c.ProbeFn != nil {
		liveness, readiness := c.ProbeFn(cfg)
		if liveness != nil {
			sts.Spec.Template.Spec.Containers[0].LivenessProbe = liveness
		}
		if readiness != nil {
			sts.Spec.Template.Spec.Containers[0].ReadinessProbe = readiness
		}
	}

	if c.VolumeClaimFn != nil {
		sts.Spec.VolumeClaimTemplates = c.VolumeClaimFn(cfg)
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			uid := int64(0)
			sts.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsUser: &uid,
				FSGroup:   &uid,
			}
		}
	}

	nodeSelector := resolveComponentNodeSelector(cfg, c.Name)
	if nodeSelector != nil {
		sts.Spec.Template.Spec.NodeSelector = nodeSelector
	}

	return sts
}

// ServiceAccountName returns the service account name.
func ServiceAccountName(c *types.Component) string {
	if c.ServiceAccount != "" {
		return c.ServiceAccount
	}
	return "default"
}

// RBAC returns RBAC resources for the component.
func RBAC(c *types.Component) (*corev1.ServiceAccount, *rbacv1.ClusterRole, *rbacv1.ClusterRoleBinding) {
	if len(c.RBACRules) == 0 {
		return nil, nil, nil
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: c.ServiceAccount, Namespace: constants.Namespace},
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: c.ServiceAccount},
		Rules:      c.RBACRules,
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: c.ServiceAccount},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: c.ServiceAccount, Namespace: constants.Namespace}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: c.ServiceAccount, APIGroup: "rbac.authorization.k8s.io"},
	}
	return sa, cr, crb
}

// Service returns a Service for the component.
func Service(cfg *types.DeployConfig, c *types.Component) *corev1.Service {
	if !c.NeedsService {
		return nil
	}
	selector := map[string]string{"app": c.Name}
	labels := map[string]string{"app": c.Name}

	if c.MetricsPort != 0 {
		labels[constants.PrometheusScrapeLabelKey] = constants.PrometheusScrapeLabelVal
		labels[constants.PrometheusPortLabelKey] = fmt.Sprintf("%d", c.MetricsPort)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: constants.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				Name:       "client",
				Port:       c.Port,
				TargetPort: intstr.FromInt32(c.Port),
			}},
		},
	}

	if c.Headless {
		svc.Spec.ClusterIP = "None"
		svc.Spec.PublishNotReadyAddresses = true
	}

	if c.ExtraSvcPortsFn != nil {
		svc.Spec.Ports = append(svc.Spec.Ports, c.ExtraSvcPortsFn(cfg)...)
	}

	if c.MetricsPort != 0 && c.MetricsPort != c.Port {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "metrics",
			Port:       c.MetricsPort,
			TargetPort: intstr.FromInt32(c.MetricsPort),
		})
	}

	return svc
}

// DBConfigYAML returns the database config as YAML.
func DBConfigYAML(cfg *types.DBConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal db config: %w", err)
	}
	return data, nil
}
