package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/release"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm"
	rlarkadmtypes "github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
)

// The release's record in kcp (rlarkctl release apply writes it after every release it brings the site to): the
// images and the Addon versions a new cluster is made with, and the files of the release's tree it applies to a
// member cluster. A new cluster is made at the release the site is at, never as some other cluster is.
const (
	releaseFileMemberRBAC        = "member-rbac.yaml"
	releaseFileOperationRBAC     = "node-operation-member-rbac.yaml"
	releaseFileStationAgent      = "station-agent.yaml"
	releaseFileReleaseMemberRBAC = "release-member-rbac.yaml"

	stationMemberCredentialNamespace    = "station-member-credentials"
	stationOperationCredentialNamespace = "station-operation-member-credentials"
	stationMemberNamespace              = "station-system"
)

type releaseAddon struct {
	Version string            `json:"version"`
	Values  map[string]string `json:"values,omitempty"`
}

type provisionRelease struct {
	Name     string
	Registry string
	Images   map[string]string
	Addons   map[string]releaseAddon
	Files    map[string]string
}

func (g *Gateway) currentRelease(ctx context.Context) (provisionRelease, error) {
	record, err := g.rawClient.CoreV1().ConfigMaps(release.RecordNamespace).Get(ctx, release.RecordName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return provisionRelease{}, fmt.Errorf("there is no release record (%s/%s): a cluster is made at the release the site is at, which the release command records", release.RecordNamespace, release.RecordName)
	}
	if err != nil {
		return provisionRelease{}, fmt.Errorf("read the release record: %w", err)
	}
	release := provisionRelease{Name: record.Data["release"], Registry: record.Data["registry"], Files: map[string]string{}}
	if err := json.Unmarshal([]byte(record.Data["images.json"]), &release.Images); err != nil {
		return provisionRelease{}, fmt.Errorf("the release record's images.json: %w", err)
	}
	if err := json.Unmarshal([]byte(record.Data["addons.json"]), &release.Addons); err != nil {
		return provisionRelease{}, fmt.Errorf("the release record's addons.json: %w", err)
	}
	var missing []string
	for _, image := range []string{"cluster-agent", "tools", "node-agent", "station-agent", "station-wireguard"} {
		if release.Images[image] == "" {
			missing = append(missing, "image "+image)
		}
	}
	for _, file := range []string{releaseFileMemberRBAC, releaseFileOperationRBAC, releaseFileStationAgent, releaseFileReleaseMemberRBAC} {
		if release.Files[file] = record.Data[file]; release.Files[file] == "" {
			missing = append(missing, file)
		}
	}
	if release.Registry == "" {
		missing = append(missing, "registry")
	}
	if len(missing) > 0 {
		return provisionRelease{}, fmt.Errorf("the release record %s has no %s", release.Name, strings.Join(missing, ", "))
	}
	return release, nil
}

// registryOf is the registry a docker config's single entry is for.
func registryOf(dockerConfigJSON string) string {
	var config struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	_ = json.Unmarshal([]byte(dockerConfigJSON), &config)
	for registry := range config.Auths {
		return registry
	}
	return ""
}

func (r *provisionRun) agent(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	admin, err := r.needAdmin(ctx, p)
	if err != nil {
		return "", err
	}
	rel, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	agentID := provisionAgentID(p.Name)
	_, _, caPEM, err := r.g.getKCPAdminCerts()
	if err != nil {
		return "", fmt.Errorf("read the management plane's CA: %w", err)
	}
	certPEM, keyPEM, err := r.g.signCertViaServer(agentID)
	if err != nil {
		return "", fmt.Errorf("sign the agent's certificate: %w", err)
	}
	if err := r.g.storeOwnedAgentCertSecret(ctx, agentID, caPEM, []byte(certPEM), []byte(keyPEM)); err != nil {
		return "", err
	}
	pulls, err := r.g.imagePullSecretsOf(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("read the cluster's image registry credentials: %w", err)
	}
	if !slices.ContainsFunc(pulls, func(s agentImagePullSecret) bool { return registryOf(s.DockerConfigJSON) == rel.Registry }) {
		return "", fmt.Errorf("no image registry credential for %s is given to %s: include it in the credential's cluster selection", rel.Registry, agentID)
	}
	credentials := make([]rlarkadmtypes.ImagePullCredential, 0, len(pulls))
	for _, pull := range pulls {
		credentials = append(credentials, rlarkadmtypes.ImagePullCredential{Name: pull.Name, DockerConfigJSON: pull.DockerConfigJSON})
	}
	err = rlarkadm.InstallTo(ctx, admin, &rlarkadmtypes.DeployConfig{
		Plane:               rlarkadmtypes.PlaneData,
		ControlPlaneAddress: r.g.config.ServerAddress,
		Cert:                &rlarkadmtypes.CertConfig{CACert: string(caPEM), AgentCert: certPEM, AgentKey: keyPEM},
		Kubernetes: &rlarkadmtypes.KubernetesEnv{
			AgentImage: rel.Images["cluster-agent"], Image: rel.Images["tools"],
			ImagePullPolicy:       "IfNotPresent",
			AstribotKubernetesEnv: rlarkadmtypes.AstribotKubernetesEnv{ImagePullCredentials: credentials},
		},
	})
	if err != nil {
		return "", fmt.Errorf("install the cluster agent: %w", err)
	}
	// The agent reports in; its nodes are online once their node agents run, which are the Cluster's declaration.
	if err := r.g.waitCluster(ctx, p.Name, 5*time.Minute, rlarkv1alpha1.ClusterOnline, rlarkv1alpha1.ClusterDowngraded); err != nil {
		return "the agent is installed", err
	}
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster, err := r.g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, p.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		cluster.Spec.NodeAgents = &rlarkv1alpha1.ClusterNodeAgents{Enabled: true, Image: rel.Images["node-agent"]}
		metav1.SetMetaDataLabel(&cluster.ObjectMeta, clusterTypeLabel, p.Labels[clusterTypeLabel])
		_, err = r.g.kubeClient.RlinfV1alpha1().Clusters().Update(ctx, cluster, metav1.UpdateOptions{})
		return err
	}); err != nil {
		return "the agent reports in", fmt.Errorf("declare the node agents and the cluster's type: %w", err)
	}
	m, err := stationmember.New(admin)
	if err != nil {
		return "", err
	}
	nodeAgents, err := m.DaemonSetReady(ctx, "rlark-system", "rlark-agent-node", 5*time.Minute)
	if err != nil {
		return "the agent reports in", err
	}
	if err := r.g.waitCluster(ctx, p.Name, 5*time.Minute, rlarkv1alpha1.ClusterOnline); err != nil {
		return "the agent reports in; node agents " + nodeAgents, err
	}
	readback := fmt.Sprintf("Cluster %s Online (agent %s…); node agents %s", p.Name, short(strings.TrimPrefix(rel.Images["cluster-agent"][strings.LastIndex(rel.Images["cluster-agent"], "@")+1:], "sha256:")), nodeAgents)

	// The release reaches the cluster with an account of its own, which the release's tree states, at the address
	// the site's names give the control plane's name (release.ReachedAs).
	if err := release.IssueMemberCredential(ctx, m.Client, m.Dynamic, r.g.rawClient, p.Name, admin.Host, p.Status.Facts.ControlPlaneName, []byte(rel.Files[releaseFileReleaseMemberRBAC])); err != nil {
		return readback, fmt.Errorf("the release's credential: %w", err)
	}
	return readback + "; the release's credential stored (" + release.CredentialNamespace + "/" + release.CredentialName(p.Name) + ")", nil
}

// waitCluster waits until the Cluster is in one of phases.
func (g *Gateway) waitCluster(ctx context.Context, name string, within time.Duration, phases ...rlarkv1alpha1.ClusterPhase) error {
	phase := "absent"
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		cluster, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if phase = string(cluster.Status.Phase); slices.Contains(phases, cluster.Status.Phase) {
			return nil
		}
	}
	return fmt.Errorf("Cluster %s is %s after %s", name, phase, within)
}

// station registers the cluster with the site's Station: what the cluster runs for it, and how the management
// plane reaches its API. The tunnel of a node the installer made is a host service (station.rlark.io/network=host,
// which the installer gives the node), so the node runs station-agent-host.
func (r *provisionRun) station(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	admin, err := r.needAdmin(ctx, p)
	if err != nil {
		return "", err
	}
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	ca, err := os.ReadFile(r.g.config.StationAPICAFile)
	if err != nil {
		return "", fmt.Errorf("read Station API's CA: %w", err)
	}
	// The image registry credential RLark delivers into station-system, for the release's registry.
	pulls, err := r.g.imagePullSecretsOf(ctx, provisionAgentID(p.Name))
	if err != nil {
		return "", err
	}
	pull := ""
	for _, s := range pulls {
		if registryOf(s.DockerConfigJSON) == release.Registry {
			pull = s.Name
			break
		}
	}
	if pull == "" {
		return "", fmt.Errorf("no image registry credential for %s is given to the cluster", release.Registry)
	}
	m, err := stationmember.New(admin)
	if err != nil {
		return "", err
	}
	facts := p.Status.Facts
	if err := stationmember.Register(ctx, m, r.g.rawClient, stationmember.Registration{
		Cluster: p.Name, APIName: facts.ControlPlaneName,
		StationAPIURL: "https://" + r.g.stationBootstrap.TunnelAddress(), StationAPICA: ca,
		AgentImage: release.Images["station-agent"], WireguardImage: release.Images["station-wireguard"], PullSecret: pull,
		Manifests: stationmember.Manifests{
			MemberRBAC: release.Files[releaseFileMemberRBAC], OperationRBAC: release.Files[releaseFileOperationRBAC], StationAgent: release.Files[releaseFileStationAgent],
		},
	}); err != nil {
		return "", err
	}
	host, err := m.DaemonSetReady(ctx, stationmember.Namespace, "station-agent-host", 5*time.Minute)
	if err != nil {
		return "member credentials stored", err
	}
	readback := "member credentials stored; " + host
	phase := ""
	for deadline := time.Now().Add(5 * time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		enrollment, err := r.g.stationClient.Resource(networkEnrollmentGVR).Namespace(p.Name).Get(ctx, facts.NodeName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return readback, err
		}
		if enrollment != nil {
			phase, _, _ = unstructured.NestedString(enrollment.Object, "status", "phase")
		}
		network, err := r.g.provisionNetwork(ctx)
		if err != nil {
			return readback, err
		}
		if phase == "Enrolled" && network.declared(p.Name, facts.NodeName) == "" {
			return readback + "; " + facts.NodeName + " Enrolled", nil
		}
	}
	return readback, fmt.Errorf("the network enrollment of %s is %q after 5 min (station-agent-host's log says why)", facts.NodeName, phase)
}
