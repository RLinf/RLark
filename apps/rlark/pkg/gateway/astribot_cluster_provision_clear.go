package gateway

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/release"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
)

// clearClusterProvisionRequest takes away a making that stopped, or what the making of a cluster that was deleted
// left. Password logs in to its machine to put it back as it was; MachineGone says the machine is not there to be
// put back (reinstalled, returned), and only RLark's own records go.
type clearClusterProvisionRequest struct {
	Password    string `json:"password"`
	User        string `json:"user"`
	MachineGone bool   `json:"machine_gone"`
}

// clearMachineScript undoes on the machine what the stages did there: the control plane this installer made (which
// refuses while other nodes are in its cluster, and keeps a last snapshot), and the tunnel. An installer still
// running there is one whose stage was cut off (the Gateway restarted): no making runs now, so it is stopped first;
// this script and the shells it runs under hold the installer's name in their own command lines and are left out.
// The machine keeps its tunnel key and the installer: a making that starts over finds the same machine.
const clearMachineScript = `set -eu
B=` + provisionBootstrapDir + `
mine=" "; p=$$; while [ "$p" -gt 1 ]; do mine="$mine$p "; p=$(awk '{print $4}' /proc/$p/stat); done
for p in $(pgrep -f 'control-plane\.sh|station-bootstrap control-plane' || true); do
  case "$mine" in *" $p "*) ;; *) kill "$p" 2>/dev/null || true ;; esac
done
pkill -x kubeadm || true
if [ -x $B/station-bootstrap ] && [ -d /var/lib/rlark/control-plane ]; then (cd $B && ./station-bootstrap control-plane undo); fi
systemctl disable --now rlark-station-gateway >/dev/null 2>&1 || true
rm -f /etc/systemd/system/rlark-station-gateway.service /etc/rlark/station/bootstrap.json
rm -rf /var/lib/station/wg
ip link del st-wg0 2>/dev/null || true
systemctl daemon-reload
echo cleared=yes
`

// handleClearClusterProvision answers POST /api/v1/cluster-provisions/:name/clear: a making that stopped half way,
// or the record of one whose cluster was deleted, is taken away so that the site, the cluster's name and the machine
// can be used again. Until then they are held: the SiteNetwork has the site, the control plane's name and the hub by
// its key, and a new cluster of the name is refused. It answers what it did, a line a step.
func (g *Gateway) handleClearClusterProvision(c *gin.Context) {
	var req clearClusterProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	ctx := c.Request.Context()
	provisions := g.kubeClient.RlinfV1alpha1().ClusterProvisions()
	p, err := provisions.Get(ctx, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get cluster provision: "+err.Error(), err)
		return
	}
	_, err = g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, p.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		respondKubeError(c, err.Error(), err)
		return
	}
	listed := err == nil
	reached := slices.ContainsFunc(p.Status.Stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool {
		return s.Name == stageBundle && s.State != rlarkv1alpha1.ClusterProvisionStagePending
	})
	switch {
	case p.Status.Phase == rlarkv1alpha1.ClusterProvisionSucceeded && listed:
		respondError(c, http.StatusConflict, CodeResourceConflict, "cluster "+p.Name+" is made and listed: delete the cluster first; clearing then takes away what its making declared")
		return
	case reached && req.Password == "" && !req.MachineGone:
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "the making reached the machine: give its SSH password so that the machine is put back too, or machine_gone when it is not there any more")
		return
	case !g.provisions.start(p.Name):
		respondError(c, http.StatusConflict, CodeResourceConflict, "the making of "+p.Name+" is running")
		return
	}
	defer g.provisions.end(p.Name)
	password := []byte(req.Password)
	defer clear(password)
	if user := strings.TrimSpace(req.User); user != "" {
		p.Spec.Machine.User = user
	}
	steps, err := g.clearProvision(ctx, p, password, listed)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error(), "steps": steps})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "steps": steps})
}

// clearProvision takes away, in the order that leaves nothing pointing at what is gone: the machine's control plane
// and tunnel, the Cluster a making that did not finish listed, the credentials the management plane reaches the
// cluster with, the SiteNetwork's declarations of the making, and the record itself.
func (g *Gateway) clearProvision(ctx context.Context, p *rlarkv1alpha1.ClusterProvision, password []byte, listed bool) ([]string, error) {
	var steps []string
	facts := p.Status.Facts
	if len(password) > 0 {
		machine, _, err := g.provisionDial(ctx, p.Spec.Machine, password, p.Status.HostKey)
		if err != nil {
			return steps, fmt.Errorf("log in to %s@%s: %w", p.Spec.Machine.User, p.Spec.Machine.Address, err)
		}
		out, err := machine.Run(ctx, clearMachineScript, nil, nil)
		_ = machine.Close()
		if err != nil {
			return steps, fmt.Errorf("put the machine back: %w", err)
		}
		steps = append(steps, "机器 "+p.Spec.Machine.Address+"：撤掉这次建的控制面和隧道（"+lastLine(out, "control plane")+"）；隧道密钥和安装包留在机器上")
	} else {
		steps = append(steps, "机器 "+p.Spec.Machine.Address+" 没动（没给密码或已不在）：它要再用，先在机器上撤掉旧控制面")
	}
	if listed {
		if err := g.kubeClient.RlinfV1alpha1().Clusters().Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return steps, fmt.Errorf("delete the Cluster the making listed: %w", err)
		}
		steps = append(steps, "删掉没建完就登记了的集群 "+p.Name)
	}
	secrets := map[string]string{
		stationMemberCredentialNamespace:    stationmember.SecretName(p.Name),
		stationOperationCredentialNamespace: stationmember.SecretName(p.Name),
		release.CredentialNamespace:         release.CredentialName(p.Name),
		g.managementNamespace():             "rlark-agent-cert-" + provisionAgentID(p.Name),
	}
	for namespace, name := range secrets {
		if err := g.rawClient.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return steps, fmt.Errorf("delete the credential %s/%s: %w", namespace, name, err)
		}
	}
	steps = append(steps, "删掉管理面到这个集群的凭据（Station、节点操作、发布、agent 证书）")
	// What Station keeps of the cluster's nodes: a cluster made again under the name has nodes of the same names
	// with other UIDs, which the records of the earlier ones would hold in Conflict, and admissions hold keys and
	// tunnel addresses.
	records, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(p.Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return steps, fmt.Errorf("list the cluster's site nodes: %w", err)
	}
	for i := range records.Items {
		if err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(p.Name).Delete(ctx, records.Items[i].GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return steps, fmt.Errorf("delete the site node %s: %w", records.Items[i].GetName(), err)
		}
	}
	admissions, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace(p.Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return steps, fmt.Errorf("list the cluster's node admissions: %w", err)
	}
	for i := range admissions.Items {
		if err := g.stationClient.Resource(nodeAdmissionGVR).Namespace(p.Name).Delete(ctx, admissions.Items[i].GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return steps, fmt.Errorf("delete the node admission %s: %w", admissions.Items[i].GetName(), err)
		}
	}
	steps = append(steps, fmt.Sprintf("删掉这个集群的 %d 条场站节点记录和它的入网许可（公钥和隧道地址放出）", len(records.Items)))
	controlPlaneName := "cp." + p.Name + ".rlark.internal"
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		network, err := g.provisionNetwork(ctx)
		if err != nil {
			return err
		}
		object := network.object
		var names, sites, nodes []interface{}
		for _, entry := range network.list("spec", "names") {
			if entry["name"] != controlPlaneName {
				names = append(names, entry)
			}
		}
		// The site goes with its only cluster; a site that has others keeps them.
		for _, entry := range network.list("spec", "sites") {
			clusters, _, _ := unstructured.NestedStringSlice(entry, "clusters")
			if !slices.Contains(clusters, p.Name) {
				sites = append(sites, entry)
				continue
			}
			if clusters = slices.DeleteFunc(clusters, func(cluster string) bool { return cluster == p.Name }); len(clusters) > 0 {
				_ = unstructured.SetNestedStringSlice(entry, clusters, "clusters")
				sites = append(sites, entry)
			}
		}
		for _, entry := range network.list("spec", "nodes") {
			if entry["clusterID"] != p.Name {
				nodes = append(nodes, entry)
			}
		}
		for path, value := range map[string][]interface{}{"names": names, "sites": sites, "nodes": nodes} {
			if err := unstructured.SetNestedSlice(object.Object, value, "spec", path); err != nil {
				return err
			}
		}
		_, err = g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, object, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return steps, fmt.Errorf("take the making's declarations out of the SiteNetwork: %w", err)
	}
	steps = append(steps, fmt.Sprintf("SiteNetwork：去掉场地 %s、名字 %s、%s 的节点声明（隧道地址 %s 放出）", p.Spec.Site, controlPlaneName, p.Name, facts.TunnelAddress))
	if err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return steps, fmt.Errorf("delete the record of the making: %w", err)
	}
	return append(steps, "删掉这次新建的记录：场地、集群名和机器可以重新用"), nil
}
