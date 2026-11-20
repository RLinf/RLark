package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// createClusterProvisionRequest is what "new cluster" asks for: the new site, its first machine and how to log in
// to it. The password is used for this making only and kept nowhere.
type createClusterProvisionRequest struct {
	Site string `json:"site"`
	// Type is the new cluster's type, gpu or robot (the Cluster's label rlark.io/cluster-type).
	Type string `json:"type"`
	// Cluster is the new cluster's name; empty, rlark-<site>-<type>.
	Cluster  string `json:"cluster"`
	Address  string `json:"address"`
	Port     int32  `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	// TunnelEndpoint is the ip:port other sites dial for the machine's tunnel; empty, the address at port 51820.
	TunnelEndpoint string `json:"tunnel_endpoint"`
}

// retryClusterProvisionRequest goes on from the stage a making stopped at. User, when given, is the account to log in
// to the machine with from now on (the one given first was refused).
type retryClusterProvisionRequest struct {
	Password string `json:"password"`
	User     string `json:"user"`
}

func (g *Gateway) handleListClusterProvisions(c *gin.Context) {
	list, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, "list cluster provisions: "+err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list.Items})
}

func (g *Gateway) handleGetClusterProvision(c *gin.Context) {
	p, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get cluster provision: "+err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (g *Gateway) handleCreateClusterProvision(c *gin.Context) {
	var req createClusterProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	req.Site, req.Cluster, req.Address, req.User = strings.TrimSpace(req.Site), strings.TrimSpace(req.Cluster), strings.TrimSpace(req.Address), strings.TrimSpace(req.User)
	if req.Cluster == "" {
		req.Cluster = apis.RLarkAgentNamespacePrefix + req.Site + "-" + req.Type
	}
	if req.Port == 0 {
		req.Port = 22
	}
	req.TunnelEndpoint = strings.TrimSpace(req.TunnelEndpoint)
	if req.TunnelEndpoint == "" {
		req.TunnelEndpoint = net.JoinHostPort(req.Address, strconv.Itoa(provisionTunnelPort))
	}
	switch {
	case !slices.Contains(clusterTypes, req.Type):
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "type must be gpu or robot")
		return
	case len(validation.IsDNS1123Label(req.Site)) > 0:
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "site must be lower-case letters, digits and -")
		return
	case !strings.HasPrefix(req.Cluster, apis.RLarkAgentNamespacePrefix) || len(validation.IsDNS1123Label(req.Cluster)) > 0:
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "cluster must start with "+apis.RLarkAgentNamespacePrefix+" and be lower-case letters, digits and -")
		return
	case req.Address == "" || strings.ContainsAny(req.Address, " /:@") && net.ParseIP(req.Address) == nil:
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "address must be the machine's IP address or host name")
		return
	case req.User == "" || req.Password == "":
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "user and password are required")
		return
	case req.Port < 1 || req.Port > 65535:
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "port must be 1-65535")
		return
	case !validTunnelEndpoint(req.TunnelEndpoint):
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "tunnel_endpoint must be an IPv4 ip:port (the address is a host name: give it)")
		return
	}
	ctx := c.Request.Context()
	// What an earlier making of the name left holds the site, the name and the machine until it is cleared.
	if left, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(ctx, req.Cluster, metav1.GetOptions{}); err == nil {
		respondError(c, http.StatusConflict, CodeResourceAlreadyExists, fmt.Sprintf("an earlier making of %s is left (%s, at stage %s): retry it, or clear it (POST /api/v1/cluster-provisions/%s/clear) and start over", req.Cluster, left.Status.Phase, left.Status.Stage, req.Cluster))
		return
	} else if !apierrors.IsNotFound(err) {
		respondKubeError(c, err.Error(), err)
		return
	}
	if _, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, req.Cluster, metav1.GetOptions{}); err == nil {
		respondError(c, http.StatusConflict, CodeResourceAlreadyExists, "a cluster "+req.Cluster+" already exists")
		return
	} else if !apierrors.IsNotFound(err) {
		respondKubeError(c, err.Error(), err)
		return
	}
	network, err := g.provisionNetwork(ctx)
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeNotConfigured, err.Error())
		return
	}
	if network.hasSite(req.Site) || network.siteOf(req.Cluster) != "" {
		respondError(c, http.StatusConflict, CodeResourceAlreadyExists, "the SiteNetwork already has site "+req.Site+" or cluster "+req.Cluster)
		return
	}
	if _, err := g.currentRelease(ctx); err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeNotConfigured, err.Error())
		return
	}
	client := g.kubeClient.RlinfV1alpha1().ClusterProvisions()
	p, err := client.Create(ctx, &rlarkv1alpha1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{Name: req.Cluster, Labels: map[string]string{clusterTypeLabel: req.Type}},
		Spec: rlarkv1alpha1.ClusterProvisionSpec{
			Site:        req.Site,
			Machine:     rlarkv1alpha1.ClusterProvisionMachine{Address: req.Address, Port: req.Port, User: req.User, TunnelEndpoint: req.TunnelEndpoint},
			RequestedBy: c.GetString(authSubjectKey),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		respondKubeError(c, "create cluster provision: "+err.Error(), err)
		return
	}
	p.Status = newProvisionStatus()
	if p, err = client.UpdateStatus(ctx, p, metav1.UpdateOptions{}); err != nil {
		respondKubeError(c, "write the cluster provision's status: "+err.Error(), err)
		return
	}
	g.startProvision(ctx, p.Name, req.Password, provisionStages[0])
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

func (g *Gateway) handleRetryClusterProvision(c *gin.Context) {
	var req retryClusterProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	ctx := c.Request.Context()
	p, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(ctx, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "get cluster provision: "+err.Error(), err)
		return
	}
	if p.Status.Phase != rlarkv1alpha1.ClusterProvisionFailed {
		respondError(c, http.StatusConflict, CodeResourceConflict, "the making of "+p.Name+" is "+string(p.Status.Phase)+": only a stopped one is retried")
		return
	}
	if provisionNeedsMachine(p.Status.Stage) && req.Password == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "stage "+p.Status.Stage+" logs in to the machine: its SSH password is required")
		return
	}
	if user := strings.TrimSpace(req.User); user != "" && user != p.Spec.Machine.User {
		p.Spec.Machine.User = user
		if _, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Update(ctx, p, metav1.UpdateOptions{}); err != nil {
			respondKubeError(c, "change the machine's account: "+err.Error(), err)
			return
		}
	}
	if !g.startProvision(ctx, p.Name, req.Password, p.Status.Stage) {
		respondError(c, http.StatusConflict, CodeResourceConflict, "the making of "+p.Name+" is running")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": p})
}

// startProvision runs the making in the background, from stage on; false when it already runs.
func (g *Gateway) startProvision(ctx context.Context, name, password, stage string) bool {
	if !g.provisions.start(name) {
		return false
	}
	run := &provisionRun{g: g, name: name}
	if password != "" {
		run.password = []byte(password)
	}
	go run.run(context.WithoutCancel(ctx), stage)
	return true
}
