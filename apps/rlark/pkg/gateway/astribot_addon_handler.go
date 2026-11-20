package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

// publishAddonPackageFor is what installing and updating an Addon add to the official handlers: the Package of the
// version is published (again, for an Addon installed before Packages were), *version becomes the published one, so
// the Addon is pinned to it and a later catalog changes nothing for it, and the values must give every required
// parameter. It answers the request and returns false when any of that fails.
func (g *Gateway) publishAddonPackageFor(c *gin.Context, clusterID, name string, version *string, values map[string]string) bool {
	meta, status, err := g.publishAddonPackage(c.Request.Context(), clusterID, name, *version)
	if err != nil {
		respondError(c, status, codeForAddonPackage(status), err.Error())
		return false
	}
	*version = meta.Version
	if missing := addons.MissingRequired(meta, values); len(missing) > 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "required parameters are missing: "+strings.Join(missing, ", "))
		return false
	}
	return true
}

// deleteAddonPackages deletes the Packages published for a deleted Addon; installing it again publishes anew.
func (g *Gateway) deleteAddonPackages(c *gin.Context, clusterID, name string) bool {
	if err := g.rawClient.CoreV1().ConfigMaps(clusterID).DeleteCollection(c.Request.Context(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: addons.LabelPackage + "=" + name}); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return false
	}
	return true
}

// publishAddonPackage publishes the Package (addons.Publish) and returns, on failure, the HTTP status to answer with.
func (g *Gateway) publishAddonPackage(ctx context.Context, clusterID, name, version string) (addons.ItemMeta, int, error) {
	meta, err := addons.Publish(ctx, g.rawClient.CoreV1().ConfigMaps(clusterID), clusterID, name, version)
	switch {
	case errors.Is(err, addons.ErrNotInCatalog):
		return meta, http.StatusNotFound, err
	case errors.Is(err, addons.ErrPublishedOtherwise):
		return meta, http.StatusConflict, err
	case err != nil:
		return meta, http.StatusInternalServerError, err
	}
	return meta, 0, nil
}

func codeForAddonPackage(status int) string {
	switch status {
	case http.StatusNotFound:
		return CodeResourceNotFound
	case http.StatusConflict:
		return CodeResourceConflict
	default:
		return CodeInternalError
	}
}
