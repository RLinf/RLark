package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleSetNodeDisplayName answers PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/display-name:
// the one way a Node's spec.displayName is set, or cleared with an empty name.
// The name is unique among all Nodes' display names and names in every
// namespace; metadata.name, which Jobs, Stations and SiteNetwork key on, never
// changes. Every call is in the audit log with its actor.
func (g *Gateway) handleSetNodeDisplayName(c *gin.Context) {
	displayName, ok := validateDisplayName(c)
	if !ok {
		return
	}
	ctx, name, namespace := c.Request.Context(), c.Param("name"), c.Query("namespace")
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}
	nodes := g.accessors["nodes"]
	existing, err := nodes.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	displayNameMu.Lock()
	defer displayNameMu.Unlock()
	if displayName != "" {
		all, err := nodes.list(ctx, metav1.NamespaceAll, metav1.ListOptions{})
		if err != nil {
			respondKubeError(c, err.Error(), err)
			return
		}
		for _, other := range all.(*rlarkv1alpha1.NodeList).Items {
			if (other.Namespace != namespace || other.Name != name) && (other.Name == displayName || other.Spec.DisplayName == displayName) {
				respondError(c, http.StatusConflict, CodeResourceConflict, errDisplayNameUsed.Error()+" by "+other.Namespace+"/"+other.Name)
				return
			}
		}
	}
	// null takes the field away: the CRD refuses an empty display name.
	var value any
	if displayName != "" {
		value = displayName
	}
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"displayName": value}})
	result, err := nodes.patch(ctx, namespace, name, patch)
	log.FromContext(ctx).WithName("audit").Info("node display name", "action", "set", "actor", c.GetString(authSubjectKey), "namespace", namespace, "node", name,
		"was", existing.(*rlarkv1alpha1.Node).Spec.DisplayName, "now", displayName, "set", err == nil)
	if err != nil {
		respondCreateError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
