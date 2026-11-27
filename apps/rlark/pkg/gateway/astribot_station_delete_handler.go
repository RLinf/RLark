package gateway

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// handleDeleteStation answers DELETE /api/v1/station/stations/:name?cluster_id=: the site node's record (its
// NetworkEnrollment) is deleted when its node was removed (retired), when another Node reports under the name
// (Conflict), or when the node does not report (is not Bound) and RLark has no Node of the name. It is deleted as it
// was read: one that changed meanwhile answers 409. A record that is gone already is deleted: the request can be
// made again.
func (g *Gateway) handleDeleteStation(c *gin.Context) {
	clusterID, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	ctx, name := c.Request.Context(), c.Param("name")
	resource := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace)
	record, err := resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.Status(http.StatusOK)
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	retired, _, _ := unstructured.NestedBool(record.Object, "spec", "retired")
	state, _, _ := unstructured.NestedString(record.Object, "status", "node", "state")
	if !retired && state != "Conflict" {
		_, err := g.stationClient.Resource(siteNodeGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
			return
		}
		if err == nil || state == "Bound" {
			c.JSON(http.StatusConflict, gin.H{"error": "the node is in the cluster: remove it first"})
			return
		}
	}
	uid, version := record.GetUID(), record.GetResourceVersion()
	err = resource.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}})
	if apierrors.IsConflict(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "the record changed while it was being deleted: look at it again"})
		return
	}
	if err != nil && !apierrors.IsNotFound(err) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	log.FromContext(ctx).WithName("audit").Info("site node", "action", "delete", "actor", c.GetString(authSubjectKey), "cluster", clusterID, "node", name)
	c.Status(http.StatusOK)
}
