package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

var stationRebindGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "stationrebinds"}

const maxStationRebindRequestBytes = 16 * 1024

type createStationRebindRequest struct {
	ClusterID              string `json:"cluster_id" binding:"required,max=128"`
	RequestID              string `json:"request_id" binding:"required,max=128"`
	StationName            string `json:"station_name" binding:"required,max=253"`
	StationUID             string `json:"station_uid" binding:"required,max=128"`
	ExpectedBindingVersion int64  `json:"expected_binding_version" binding:"required,gt=0"`
	TargetNodeName         string `json:"target_node_name" binding:"required,max=253"`
	TargetNodeUID          string `json:"target_node_uid" binding:"required,max=128"`
	TargetSystemUUID       string `json:"target_system_uuid,omitempty" binding:"omitempty,max=128"`
	Reason                 string `json:"reason" binding:"required,max=1024"`
}

func (g *Gateway) handleCreateStationRebind(c *gin.Context) {
	var request createStationRebindRequest
	if !decodeOneJSON(c, maxStationRebindRequestBytes, &request, false) {
		return
	}
	if err := binding.Validator.ValidateStruct(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Station rebind request"})
		return
	}
	actor := c.GetString(authSubjectKey)
	if strings.TrimSpace(request.ClusterID) != request.ClusterID || strings.TrimSpace(request.RequestID) != request.RequestID || strings.TrimSpace(request.Reason) == "" ||
		strings.TrimSpace(request.StationName) != request.StationName || strings.TrimSpace(request.TargetNodeName) != request.TargetNodeName ||
		strings.TrimSpace(request.StationUID) != request.StationUID || strings.TrimSpace(request.TargetNodeUID) != request.TargetNodeUID ||
		strings.TrimSpace(request.TargetSystemUUID) != request.TargetSystemUUID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Station rebind fields must not contain surrounding whitespace"})
		return
	}
	if problems := validation.IsDNS1123Subdomain(request.TargetNodeName); len(problems) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_node_name must be a DNS subdomain"})
		return
	}
	if problems := validation.IsDNS1123Subdomain(request.StationName); len(problems) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "station_name must be a DNS subdomain"})
		return
	}
	namespace, err := g.stationNamespace(c, request.ClusterID)
	if err != nil {
		return
	}
	name := stationRebindOperationName(request.ClusterID, request.RequestID)
	spec := map[string]interface{}{
		"requestID":              request.RequestID,
		"stationRef":             map[string]interface{}{"name": request.StationName, "uid": request.StationUID},
		"expectedBindingVersion": request.ExpectedBindingVersion,
		"targetNodeName":         request.TargetNodeName,
		"targetNodeUID":          request.TargetNodeUID,
		"reason":                 request.Reason,
		"requestedBy":            actor,
	}
	if request.TargetSystemUUID != "" {
		spec["targetSystemUUID"] = request.TargetSystemUUID
	}
	resource := g.stationClient.Resource(stationRebindGVR).Namespace(namespace)
	if existing, getErr := resource.Get(c.Request.Context(), name, metav1.GetOptions{}); getErr == nil {
		g.respondStationRebindReplay(c, existing, spec)
		return
	} else if !apierrors.IsNotFound(getErr) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station rebind lookup failed"})
		return
	}
	station, err := g.stationClient.Resource(stationRecordGVR).Namespace(namespace).Get(c.Request.Context(), request.StationName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Station not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station lookup failed"})
		return
	}
	if string(station.GetUID()) != request.StationUID {
		c.JSON(http.StatusConflict, gin.H{"error": "Station object UID changed"})
		return
	}
	retired, found, nestedErr := unstructured.NestedBool(station.Object, "spec", "retired")
	if nestedErr != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Station state is invalid"})
		return
	}
	if found && retired {
		c.JSON(http.StatusConflict, gin.H{"error": "retired Stations cannot be rebound"})
		return
	}
	version, found, nestedErr := unstructured.NestedInt64(station.Object, "spec", "binding", "version")
	if nestedErr != nil || !found {
		c.JSON(http.StatusConflict, gin.H{"error": "Station binding state is invalid"})
		return
	}
	if version != request.ExpectedBindingVersion {
		c.JSON(http.StatusConflict, gin.H{"error": "Station binding version changed"})
		return
	}
	operation := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1",
		"kind":       "StationRebind",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       spec,
	}}
	created, err := resource.Create(c.Request.Context(), operation, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := resource.Get(c.Request.Context(), name, metav1.GetOptions{})
		if getErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Station rebind result is unknown"})
			return
		}
		g.respondStationRebindReplay(c, existing, spec)
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station rebind could not be persisted"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operation": created.Object, "created": true})
}

func (g *Gateway) respondStationRebindReplay(c *gin.Context, existing *unstructured.Unstructured, requestedSpec map[string]interface{}) {
	existingSpec, found, err := unstructured.NestedMap(existing.Object, "spec")
	if err != nil || !found || !apiequality.Semantic.DeepEqual(existingSpec, requestedSpec) {
		c.JSON(http.StatusConflict, gin.H{"error": "request_id already identifies a different Station rebind"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operation": existing.Object, "created": false})
}

func (g *Gateway) handleGetStationRebind(c *gin.Context) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	operation, err := g.stationClient.Resource(stationRebindGVR).Namespace(namespace).Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Station rebind operation not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station rebind query failed"})
		return
	}
	c.JSON(http.StatusOK, operation.Object)
}

func stationRebindOperationName(clusterID, requestID string) string {
	digest := sha256.Sum256([]byte(clusterID + "\x00" + requestID))
	return "rebind-" + hex.EncodeToString(digest[:16])
}
