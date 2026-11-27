package component

import (
	"slices"
	"strings"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestAgentWorkloadsForDeclaredNodeAgents(t *testing.T) {
	cfg := &types.DeployConfig{Plane: types.PlaneData, Kubernetes: &types.KubernetesEnv{}}
	components := ComponentsForPlane(cfg)
	// The node agents are the Cluster's declaration, which the cluster agent makes into its DaemonSet.
	if len(components) != 1 || components[0].Name != constants.ComponentAgent {
		t.Fatalf("data plane components = %v, want the cluster agent alone", components)
	}
	if got := Deployment(cfg, &components[0]).Spec.Strategy.Type; got != appsv1.RecreateDeploymentStrategyType {
		t.Errorf("cluster agent strategy = %q: two cluster agents at once would rewrite the node agent DaemonSet back and forth", got)
	}
}

// Agents keep off the ports controllers default to: a cluster agent in the host's network shares
// it with other controllers (the site's Station controller probes on 8081).
func TestAgentsProbeTheirOwnPort(t *testing.T) {
	cfg := &types.DeployConfig{Plane: types.PlaneData, Kubernetes: &types.KubernetesEnv{}}
	for _, c := range ComponentsForPlane(cfg) {
		if c.Name != constants.ComponentAgent {
			continue
		}
		_, readiness := c.ProbeFn(cfg)
		if got := readiness.HTTPGet.Port.IntValue(); got != 10382 {
			t.Errorf("%s readiness probe port = %d, want 10382", c.Name, got)
		}
	}
}

// Each control-plane component reaches kcp as itself: it mounts the Secret of its own identity whole
// (a rotated token reaches the Pod; a subPath mount would freeze it) and never kcp's administrator
// credential, which kcp writes anew at every start.
func TestControlPlaneComponentsUseTheirOwnKCPIdentity(t *testing.T) {
	cfg := &types.DeployConfig{Plane: types.PlaneControl, Kubernetes: &types.KubernetesEnv{}}
	seen := 0
	for _, c := range ComponentsForPlane(cfg) {
		if c.Name != constants.ComponentServer && c.Name != constants.ComponentControllerManager && c.Name != constants.ComponentGateway {
			continue
		}
		seen++
		spec := Deployment(cfg, &c).Spec.Template.Spec
		container := spec.Containers[0]
		args := strings.Join(container.Args, " ")
		if !strings.Contains(args, "--kubeconfig /etc/rlark/kcp/kubeconfig") {
			t.Errorf("%s args = %s", c.Name, args)
		}
		var secret string
		for _, v := range spec.Volumes {
			if v.Name == "kcp-credential" {
				secret = v.Secret.SecretName
			}
			if v.ConfigMap != nil && v.ConfigMap.Name == "rlark-kcp-kubeconfig" {
				t.Errorf("%s mounts kcp's administrator credential", c.Name)
			}
		}
		if secret != c.Name+"-kcp" {
			t.Errorf("%s mounts the kcp credential of %q", c.Name, secret)
		}
		for _, m := range container.VolumeMounts {
			if m.Name == "kcp-credential" && (m.MountPath != "/etc/rlark/kcp" || m.SubPath != "") {
				t.Errorf("%s mount = %+v", c.Name, m)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("saw %d control-plane components", seen)
	}
}

// A cluster Agent rlarkadm installs starts by itself on a new cluster, whose one node is its control plane: it is
// let onto that node, prefers it, and finds the management plane by the names the site gives the node.
func TestClusterAgentStartsOnANewClustersControlPlane(t *testing.T) {
	cfg := &types.DeployConfig{Plane: types.PlaneData, Kubernetes: &types.KubernetesEnv{}}
	for _, c := range ComponentsForPlane(cfg) {
		if c.Name != constants.ComponentAgent {
			continue
		}
		spec := Deployment(cfg, &c).Spec.Template.Spec
		if !spec.HostNetwork || spec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
			t.Errorf("host network %v, DNS policy %q: the agent resolves the site's names as its node does", spec.HostNetwork, spec.DNSPolicy)
		}
		tolerated := slices.ContainsFunc(spec.Tolerations, func(toleration corev1.Toleration) bool {
			return toleration.Key == "node-role.kubernetes.io/control-plane" && toleration.Operator == corev1.TolerationOpExists && toleration.Effect == corev1.TaintEffectNoSchedule
		})
		if !tolerated {
			t.Errorf("tolerations %+v: a new cluster's only node is its tainted control plane", spec.Tolerations)
		}
		preferred := spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(preferred) != 1 || preferred[0].Preference.MatchExpressions[0].Key != "node-role.kubernetes.io/control-plane" || len(spec.NodeSelector) != 0 {
			t.Errorf("placement %+v %v: the control plane is preferred, not required", preferred, spec.NodeSelector)
		}
		if len(spec.HostAliases) != 0 {
			t.Errorf("host aliases %+v: the site's names are the node's", spec.HostAliases)
		}
	}
}
