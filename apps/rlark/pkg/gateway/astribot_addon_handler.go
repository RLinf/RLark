package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

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
