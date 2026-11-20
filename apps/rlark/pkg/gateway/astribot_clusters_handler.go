package gateway

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// setClusterNodeAgents replaces spec.nodeAgents. The cluster Agent then keeps (or removes) the cluster's
// node Agent DaemonSet; an omitted nodeSelector takes the CRD default rlark.io/enabled=true. The node Agents'
// image is declared here and nowhere else.
func (g *Gateway) setClusterNodeAgents(c *gin.Context) {
	var body struct {
		Enabled      *bool             `json:"enabled"`
		Image        string            `json:"image"`
		NodeSelector map[string]string `json:"nodeSelector"`
		// Rollout is how the node Agents are replaced; absent, the defaults of RolloutPolicy.
		Rollout *rlarkv1alpha1.RolloutPolicy `json:"rollout"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	// A missing enabled would silently mean "remove every node Agent".
	if body.Enabled == nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "enabled is required")
		return
	}
	if *body.Enabled && body.Image == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "image is required: the node Agents' image is declared with them")
		return
	}
	if body.Rollout != nil {
		if body.Rollout.Canary != nil && *body.Rollout.Canary < 1 {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "rollout.canary must be a positive integer")
			return
		}
		if _, err := body.Rollout.Unavailable(100); err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "rollout.maxUnavailable must be an integer or a percentage: "+err.Error())
			return
		}
	}
	if body.NodeSelector != nil && len(body.NodeSelector) == 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "nodeSelector must not be empty: omit it to select rlark.io/enabled=true")
		return
	}
	if errs := metav1validation.ValidateLabels(body.NodeSelector, field.NewPath("nodeSelector")); len(errs) > 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, errs.ToAggregate().Error())
		return
	}
	g.updateCluster(c, func(cluster *rlarkv1alpha1.Cluster) error {
		cluster.Spec.NodeAgents = &rlarkv1alpha1.ClusterNodeAgents{Enabled: *body.Enabled, Image: body.Image, NodeSelector: body.NodeSelector, Rollout: body.Rollout}
		return nil
	})
}

// setClusterDisplayName sets or, with an empty name, clears spec.displayName. The
// name is unique among the Clusters' display names and IDs, so that a name on
// screen stands for one cluster; the cluster's ID never changes.
func (g *Gateway) setClusterDisplayName(c *gin.Context) {
	displayName, ok := validateDisplayName(c)
	if !ok {
		return
	}
	var was string
	displayNameMu.Lock()
	defer displayNameMu.Unlock()
	g.updateCluster(c, func(cluster *rlarkv1alpha1.Cluster) error {
		was = cluster.Spec.DisplayName
		if displayName != "" {
			clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(c.Request.Context(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, other := range clusters.Items {
				if other.Name != cluster.Name && (other.Name == displayName || strings.TrimSpace(other.Spec.DisplayName) == displayName) {
					return fmt.Errorf("%w by %s", errDisplayNameUsed, other.Name)
				}
			}
		}
		cluster.Spec.DisplayName = displayName
		return nil
	})
	log.FromContext(c.Request.Context()).WithName("audit").Info("cluster display name", "action", "set", "actor", c.GetString(authSubjectKey),
		"cluster", c.Param("cluster_id"), "was", was, "now", displayName, "set", c.Writer.Status() == http.StatusOK)
}

// clusterTypeLabel is, on a Cluster, what kind of cluster it is: gpu (GPU 集群: training and inference machines)
// or robot (机器人集群: robots and their site's machines). It is set when the cluster is made (POST
// /cluster-provisions, the ClusterProvision carries it until the Cluster exists) or with PUT
// /clusters/{id}/cluster-type. A Node's kind stays the official rlark.io/node-category-{cloud,edge,robot}.
const clusterTypeLabel = "rlark.io/cluster-type"

var clusterTypes = []string{"gpu", "robot"}

// setClusterType sets the Cluster's type (clusterTypeLabel).
func (g *Gateway) setClusterType(c *gin.Context) {
	var request struct {
		ClusterType string `json:"clusterType"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || !slices.Contains(clusterTypes, request.ClusterType) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "clusterType must be gpu or robot")
		return
	}
	var was string
	g.updateCluster(c, func(cluster *rlarkv1alpha1.Cluster) error {
		was = cluster.Labels[clusterTypeLabel]
		metav1.SetMetaDataLabel(&cluster.ObjectMeta, clusterTypeLabel, request.ClusterType)
		return nil
	})
	log.FromContext(c.Request.Context()).WithName("audit").Info("cluster type", "action", "set", "actor", c.GetString(authSubjectKey),
		"cluster", c.Param("cluster_id"), "was", was, "now", request.ClusterType, "set", c.Writer.Status() == http.StatusOK)
}

// AstribotClusterInfo is what the site adds to ClusterInfo.
type AstribotClusterInfo struct {
	// DisplayName is spec.displayName as the administrator set it; absent when unset. The official Name is what the
	// cluster is shown as: its display name, else its ID.
	DisplayName string `json:"displayName,omitempty"`
	// NodeAgents is the declared node Agents of the cluster (spec.nodeAgents); absent when never declared.
	NodeAgents *rlarkv1alpha1.ClusterNodeAgents `json:"nodeAgents,omitempty"`
	// ClusterType is the Cluster's label rlark.io/cluster-type: gpu or robot; absent when never set.
	ClusterType string `json:"clusterType,omitempty"`
}

func setAstribotClusterInfo(info *ClusterInfo, cluster rlarkv1alpha1.Cluster) {
	info.NodeAgents = cluster.Spec.NodeAgents
	info.ClusterType = cluster.Labels[clusterTypeLabel]
	if displayName := strings.TrimSpace(cluster.Spec.DisplayName); displayName != "" {
		info.Name = displayName
		info.DisplayName = displayName
	}
}
