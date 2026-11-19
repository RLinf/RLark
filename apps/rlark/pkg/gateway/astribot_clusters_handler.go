package gateway

import (
	"fmt"
	"net/http"
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
