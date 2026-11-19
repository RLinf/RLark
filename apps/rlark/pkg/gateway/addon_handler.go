package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

type installAddonRequest struct {
	AddonName string            `json:"addonName"`
	Version   string            `json:"version,omitempty"`
	Values    map[string]string `json:"values,omitempty"`
}

func (g *Gateway) listClusterAddons(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	ctx := c.Request.Context()

	addons, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).List(ctx, metav1.ListOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": addons.Items, "success": true})
}

func (g *Gateway) listInstalledAddons(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Query("cluster")

	namespace := clusterID
	if clusterID == "" {
		namespace = metav1.NamespaceAll
	}

	addons, err := g.kubeClient.RlinfV1alpha1().Addons(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	type installedAddon struct {
		rlarkv1alpha1.Addon
		ClusterID string `json:"clusterId"`
	}

	result := make([]installedAddon, 0, len(addons.Items))
	for _, a := range addons.Items {
		result = append(result, installedAddon{
			Addon:     a,
			ClusterID: a.Namespace,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result, "success": true, "total": len(result)})
}

func (g *Gateway) getClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	name := c.Param("name")
	ctx := c.Request.Context()

	addon, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": addon, "success": true})
}

func (g *Gateway) installClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	ctx := c.Request.Context()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}

	var req installAddonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}

	if req.AddonName == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "addonName is required")
		return
	}

	meta, status, err := g.publishAddonPackage(ctx, clusterID, req.AddonName, req.Version)
	if err != nil {
		respondError(c, status, codeForAddonPackage(status), err.Error())
		return
	}
	if missing := addons.MissingRequired(meta, req.Values); len(missing) > 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "required parameters are missing: "+strings.Join(missing, ", "))
		return
	}

	// The Addon is pinned to the version it was installed at: a later catalog changes nothing for it.
	addonCR := &rlarkv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.AddonName,
			Namespace: clusterID,
		},
		Spec: rlarkv1alpha1.AddonSpec{
			AddonName: req.AddonName,
			Version:   meta.Version,
			Values:    req.Values,
		},
	}

	result, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Create(ctx, addonCR, metav1.CreateOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result, "success": true})
}

func (g *Gateway) updateClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	name := c.Param("name")
	ctx := c.Request.Context()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}

	var req installAddonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}

	existing, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, err.Error())
		return
	}

	// A request without a version keeps the Addon's own; one without values keeps its values. Either way the
	// version's Package is published (again, for an Addon installed before Packages were).
	if req.Version != "" {
		existing.Spec.Version = req.Version
	}
	if req.Values != nil {
		existing.Spec.Values = req.Values
	}
	meta, status, err := g.publishAddonPackage(ctx, clusterID, existing.Spec.AddonName, existing.Spec.Version)
	if err != nil {
		respondError(c, status, codeForAddonPackage(status), err.Error())
		return
	}
	existing.Spec.Version = meta.Version
	if missing := addons.MissingRequired(meta, existing.Spec.Values); len(missing) > 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "required parameters are missing: "+strings.Join(missing, ", "))
		return
	}

	result, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "success": true})
}

func (g *Gateway) deleteClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	name := c.Param("name")
	ctx := c.Request.Context()

	if err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	// The Packages published for it go with it; installing it again publishes anew.
	if err := g.rawClient.CoreV1().ConfigMaps(clusterID).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: addons.LabelPackage + "=" + name}); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
