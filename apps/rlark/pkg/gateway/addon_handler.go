package gateway

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": addon, "success": true})
}

func (g *Gateway) installClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	ctx := c.Request.Context()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req installAddonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AddonName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "addonName is required"})
		return
	}

	addonCR := &rlarkv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.AddonName,
			Namespace: clusterID,
		},
		Spec: rlarkv1alpha1.AddonSpec{
			AddonName: req.AddonName,
			Version:   req.Version,
			Values:    req.Values,
		},
	}

	result, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Create(ctx, addonCR, metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req installAddonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if req.Version != "" {
		existing.Spec.Version = req.Version
	}
	if req.Values != nil {
		existing.Spec.Values = req.Values
	}

	result, err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "success": true})
}

func (g *Gateway) deleteClusterAddon(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	name := c.Param("name")
	ctx := c.Request.Context()

	if err := g.kubeClient.RlinfV1alpha1().Addons(clusterID).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
