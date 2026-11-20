package gateway

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

func (g *Gateway) listAddonCatalog(c *gin.Context) {
	items := addons.Catalog.List()
	if items == nil {
		items = []addons.ItemMeta{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "success": true})
}

func (g *Gateway) getAddonCatalog(c *gin.Context) {
	name := c.Param("name")
	addon, ok := addons.Catalog.Get(name)
	if !ok {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "addon not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": addon.Meta(), "success": true})
}
