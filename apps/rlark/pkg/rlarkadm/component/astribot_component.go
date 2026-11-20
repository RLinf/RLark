package component

import (
	"slices"

	"github.com/rlinf/rlark/apps/rlark/astribot/identity"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// The site's components are the official ones (component.go, as upstream has it) with these differences:
// no node agent DaemonSet (the node agents are the Cluster's declaration, spec.nodeAgents, which the cluster
// agent keeps as its DaemonSet); the cluster agent on its own ports with a readiness probe; and each
// control-plane component reaching kcp with its own credential instead of kcp's administrator kubeconfig.
func init() {
	components = slices.DeleteFunc(components, func(c types.Component) bool { return c.Name == constants.ComponentAgentNode })
	for i := range components {
		c := &components[i]
		switch c.Name {
		case constants.ComponentAgent:
			c.Port, c.MetricsPort = constants.AgentProbePort, constants.AgentMetricsPort
			// Ready once the agent's first watches of the management plane and of its cluster have synced.
			c.ProbeFn = func(cfg *types.DeployConfig) (*corev1.Probe, *corev1.Probe) {
				return nil, &corev1.Probe{
					ProbeHandler:     corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/readyz", Port: intstr.FromInt32(constants.AgentProbePort)}},
					PeriodSeconds:    5,
					TimeoutSeconds:   3,
					FailureThreshold: 3,
				}
			}
		case constants.ComponentGateway, constants.ComponentControllerManager, constants.ComponentServer:
			name, args, volumes := c.Name, c.ArgsFn, c.VolumeFn
			c.ArgsFn = func(cfg *types.DeployConfig) []string {
				args := args(cfg)
				if i := slices.Index(args, constants.KCPKubeconfigPath); i >= 0 {
					args[i] = constants.KCPCredentialDir + "/" + identity.KubeconfigKey
				}
				return args
			}
			c.VolumeFn = func(cfg *types.DeployConfig) ([]corev1.Volume, []corev1.VolumeMount) {
				vols, mounts := volumes(cfg)
				if i := slices.IndexFunc(vols, func(v corev1.Volume) bool { return v.Name == "kcp-kubeconfig" }); i >= 0 {
					vols[i], mounts[i] = kcpCredentialVolume(name)
				}
				return vols, mounts
			}
		}
	}
}

// kcpCredentialVolume mounts the component's own kcp credential (rlarkadm identity), whole and not by
// subPath, so a rotated token reaches the running Pod.
func kcpCredentialVolume(name string) (corev1.Volume, corev1.VolumeMount) {
	secret := identity.RLarkSecretName(identity.Component{Name: name})
	return corev1.Volume{
			Name:         "kcp-credential",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}},
		},
		corev1.VolumeMount{Name: "kcp-credential", MountPath: constants.KCPCredentialDir, ReadOnly: true}
}

// credentialPullSecrets names, for each credential RLark gave the cluster, the installer's copy, which pulls the
// agent's first image, and the delivered one, which follows the credential.
func credentialPullSecrets(cfg *types.DeployConfig) []corev1.LocalObjectReference {
	var secrets []corev1.LocalObjectReference
	for _, credential := range cfg.Kubernetes.ImagePullCredentials {
		secrets = append(secrets, corev1.LocalObjectReference{Name: types.InstallCredentialName(credential.Name)}, corev1.LocalObjectReference{Name: credential.Name})
	}
	return secrets
}

// placeClusterAgent makes the cluster Agent's Deployment the site's. The cluster Agent rewrites what it renders
// from its own flags (the node Agent DaemonSet, Task sidecars); an old and a new one side by side during a rolling
// update would flip those back and forth, so it is recreated. It reaches the management plane the way its node
// does: in the node's network, by the names the site gives the node (its /etc/hosts, which a host-network Pod is
// started with), through the node's tunnel. It goes to the control plane where the cluster has one in sight, the
// machine of the site that is always there, and is the first node a new cluster has.
func placeClusterAgent(dep *appsv1.Deployment, c *types.Component) {
	if c.Name != constants.ComponentAgent {
		return
	}
	dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	spec := &dep.Spec.Template.Spec
	spec.HostNetwork, spec.DNSPolicy = true, corev1.DNSClusterFirstWithHostNet
	spec.Tolerations = []corev1.Toleration{{Key: constants.ControlPlaneNodeRole, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
		Weight: 100, Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: constants.ControlPlaneNodeRole, Operator: corev1.NodeSelectorOpExists}}},
	}}}}
}
