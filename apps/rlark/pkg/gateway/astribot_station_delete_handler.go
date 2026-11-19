package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// What the Gateway and station-controller say to each other on a retired Station an administrator wants deleted
// (station-controller's StationRemovalReconciler): who asked, and why it was not deleted.
const (
	stationDeleteRequestedAnnotation = "station.rlark.io/delete-requested-by"
	stationDeleteRefusedAnnotation   = "station.rlark.io/delete-refused"
)

// handleDeleteStation answers DELETE /api/v1/station/stations/:name?cluster_id=: a retired Station is asked to be
// deleted. station-controller, which reaches the member cluster, deletes it (with the observation of its node, and
// an audit event) once nothing stands on it, or writes on it why not. A Station that is gone already is deleted:
// the request can be made again.
func (g *Gateway) handleDeleteStation(c *gin.Context) {
	clusterID, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	resource := g.stationClient.Resource(stationRecordGVR).Namespace(namespace)
	station, err := resource.Get(ctx, c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusOK, gin.H{"deleted": true})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	if retired, _, _ := unstructured.NestedBool(station.Object, "spec", "retired"); !retired {
		c.JSON(http.StatusConflict, gin.H{"error": "only a retired Station is deleted: remove its node first"})
		return
	}
	actor := c.GetString(authSubjectKey)
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]any{stationDeleteRequestedAnnotation: actor, stationDeleteRefusedAnnotation: nil}}})
	if _, err := resource.Patch(ctx, station.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); apierrors.IsNotFound(err) {
		c.JSON(http.StatusOK, gin.H{"deleted": true})
		return
	} else if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	log.FromContext(ctx).WithName("audit").Info("station", "action", "delete", "actor", actor, "cluster", clusterID, "station", station.GetName())
	c.JSON(http.StatusAccepted, gin.H{"deleted": false})
}
