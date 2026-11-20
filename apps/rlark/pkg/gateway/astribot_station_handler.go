package gateway

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var stationClusterGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "clusters"}

// pageOptions is a list request's page: limit (1 to 500, else the default) and continue. On a bad limit it has
// answered.
func pageOptions(c *gin.Context, limit int64) (metav1.ListOptions, bool) {
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 500"})
			return metav1.ListOptions{}, false
		}
		limit = parsed
	}
	return metav1.ListOptions{Limit: limit, Continue: c.Query("continue")}, true
}

// clusterNamespace reads the required cluster_id of the request and its Cluster's namespace. On failure it has
// answered.
func (g *Gateway) clusterNamespace(c *gin.Context) (clusterID, namespace string, ok bool) {
	clusterID = c.Query("cluster_id")
	if clusterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return "", "", false
	}
	namespace, err := g.stationNamespace(c, clusterID)
	return clusterID, namespace, err == nil
}

// handleListStations answers GET /api/v1/station/stations: the site nodes (NetworkEnrollments) of one cluster, or of
// all without cluster_id, each without its network document.
func (g *Gateway) handleListStations(c *gin.Context) {
	options, ok := pageOptions(c, 200)
	if !ok {
		return
	}
	resource := g.stationClient.Resource(networkEnrollmentGVR)
	var records *unstructured.UnstructuredList
	var err error
	if clusterID := c.Query("cluster_id"); clusterID != "" {
		namespace, err := g.stationNamespace(c, clusterID)
		if err != nil {
			return
		}
		records, err = resource.Namespace(namespace).List(c.Request.Context(), options)
	} else {
		records, err = resource.List(c.Request.Context(), options)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	for i := range records.Items {
		unstructured.RemoveNestedField(records.Items[i].Object, "status", "config")
	}
	c.JSON(http.StatusOK, gin.H{"items": records.Items, "continue": records.GetContinue()})
}

// handleGetStation answers GET /api/v1/station/stations/:name?cluster_id=: one site node, without its network
// document.
func (g *Gateway) handleGetStation(c *gin.Context) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	record, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace).Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "site node not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return
	}
	unstructured.RemoveNestedField(record.Object, "status", "config")
	c.JSON(http.StatusOK, record.Object)
}

func (g *Gateway) stationNamespace(c *gin.Context, clusterID string) (string, error) {
	clusters, err := g.stationClient.Resource(stationClusterGVR).List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cluster API request failed"})
		return "", err
	}
	var namespace string
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if cluster.GetName() != clusterID {
			continue
		}
		value, found, nestedErr := unstructured.NestedString(cluster.Object, "spec", "agentNamespace")
		if nestedErr != nil || !found || value == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "Cluster namespace mapping is invalid"})
			return "", apierrors.NewBadRequest("invalid Cluster namespace mapping")
		}
		namespace = value
	}
	if namespace == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return "", apierrors.NewNotFound(stationClusterGVR.GroupResource(), clusterID)
	}
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if cluster.GetName() == clusterID {
			continue
		}
		value, found, nestedErr := unstructured.NestedString(cluster.Object, "spec", "agentNamespace")
		if nestedErr == nil && found && value == namespace {
			c.JSON(http.StatusConflict, gin.H{"error": "Cluster namespace mapping is ambiguous"})
			return "", apierrors.NewConflict(stationClusterGVR.GroupResource(), clusterID, nil)
		}
	}
	return namespace, nil
}
