package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var nodeOperationGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "nodeoperations"}

const maxNodeOperationRequestBytes = 64 << 10
const nodeOperationPowerOffAnnotation = "station.rlark.io/power-off"

// nodeOperationCreateRequest asks for a node to be removed. Joins are not
// asked for here: station-controller makes them for the machines an
// administrator admits, and a machine joined by SSH runs its code's line.
type nodeOperationCreateRequest struct {
	ClusterID string `json:"cluster_id"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	NodeName  string `json:"node_name"`
	// NodeUID is the Node instance to remove; empty removes whatever is under the name.
	NodeUID string `json:"node_uid,omitempty"`
}

func nodeOperationName(clusterID, requestID string) string {
	digest := sha256.Sum256([]byte(clusterID + "\x00" + requestID))
	return "node-op-" + hex.EncodeToString(digest[:16])
}

func (g *Gateway) handleCreateNodeOperation(c *gin.Context) {
	var request nodeOperationCreateRequest
	if !decodeOneJSON(c, maxNodeOperationRequestBytes, &request, false) {
		return
	}
	if err := validateNodeOperationCreate(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actor := c.GetString(authSubjectKey)
	namespace, err := g.stationNamespace(c, request.ClusterID)
	if err != nil {
		return
	}
	name := nodeOperationName(request.ClusterID, request.RequestID)
	resource := g.stationClient.Resource(nodeOperationGVR).Namespace(namespace)
	// One removal at a time per node: a second operation while one is still
	// pending or running would power, drain, reset, and delete concurrently.
	// A terminal failure may be retried with a new request ID.
	operations, listErr := resource.List(c.Request.Context(), metav1.ListOptions{})
	if listErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "node operations could not be listed"})
		return
	}
	for index := range operations.Items {
		existing := &operations.Items[index]
		if existing.GetName() == name {
			continue
		}
		action, _, _ := unstructured.NestedString(existing.Object, "spec", "action")
		nodeName, _, _ := unstructured.NestedString(existing.Object, "spec", "nodeName")
		if action != "Remove" || nodeName != request.NodeName {
			continue
		}
		phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
		if phase == "Succeeded" || phase == "Failed" || phase == "Blocked" {
			continue
		}
		c.JSON(http.StatusConflict, gin.H{
			"code":  "NODE_OPERATION_IN_PROGRESS",
			"error": fmt.Sprintf("node %s already has a removal in progress (%s); wait for it to finish or retry after it stops", request.NodeName, existing.GetName()),
		})
		return
	}
	// A removal logs in nowhere: station-controller resets the node from inside its cluster.
	// Without a UID it is of whatever is under the name: the Node its cluster
	// has then, or what RLark keeps of a name that has none (a join that never finished).
	spec := map[string]any{"requestID": request.RequestID, "action": request.Action, "clusterID": request.ClusterID, "nodeName": request.NodeName, "requestedBy": actor}
	if request.NodeUID != "" {
		spec["nodeUID"] = request.NodeUID
	}
	want := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeOperation", "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": spec}}
	created, err := resource.Create(c.Request.Context(), want, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, readErr := resource.Get(c.Request.Context(), name, metav1.GetOptions{})
		if readErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "node operation lookup failed"})
			return
		}
		if !apiequality.Semantic.DeepEqual(existing.Object["spec"], any(spec)) {
			c.JSON(http.StatusConflict, gin.H{"error": "request ID is already used for a different operation"})
			return
		}
		c.JSON(http.StatusAccepted, safeNodeOperation(existing))
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "node operation could not be created"})
		return
	}
	// The record of the node's SSH join goes with the node.
	if err := g.kubeClient.RlinfV1alpha1().NodeJoins().Delete(c.Request.Context(), nodeJoinName(request.ClusterID, request.NodeName), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		log.FromContext(c.Request.Context()).Error(err, "delete the node's join record", "cluster", request.ClusterID, "node", request.NodeName)
	}
	c.Header("Location", "/api/v1/station/node-operations/"+name+"?cluster_id="+request.ClusterID)
	c.JSON(http.StatusAccepted, safeNodeOperation(created))
}

func validateNodeOperationCreate(r nodeOperationCreateRequest) error {
	for _, value := range []string{r.ClusterID, r.RequestID, r.Action, r.NodeName, r.NodeUID} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("node operation fields must not contain surrounding whitespace")
		}
	}
	if len(validation.IsDNS1123Subdomain(r.ClusterID)) != 0 || len(validation.IsDNS1123Subdomain(r.RequestID)) != 0 {
		return fmt.Errorf("cluster_id and request_id must be DNS subdomains")
	}
	if r.Action != "Remove" {
		return fmt.Errorf("unsupported node operation action: only Remove is asked for here")
	}
	if len(validation.IsDNS1123Subdomain(r.NodeName)) != 0 {
		return fmt.Errorf("node_name must be a DNS subdomain")
	}
	return nil
}

func safeNodeOperation(op *unstructured.Unstructured) map[string]any {
	metadata := map[string]any{"name": op.GetName(), "namespace": op.GetNamespace(), "uid": op.GetUID(), "creationTimestamp": op.GetCreationTimestamp()}
	// What a removal's PowerOff stage found (station-controller, RobotPowerOff).
	if note := op.GetAnnotations()[nodeOperationPowerOffAnnotation]; note != "" {
		metadata["annotations"] = map[string]string{nodeOperationPowerOffAnnotation: note}
	}
	return map[string]any{"apiVersion": op.GetAPIVersion(), "kind": op.GetKind(), "metadata": metadata, "spec": op.Object["spec"], "status": op.Object["status"]}
}

func (g *Gateway) handleGetNodeOperation(c *gin.Context) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	op, err := g.stationClient.Resource(nodeOperationGVR).Namespace(namespace).Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "node operation not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "node operation lookup failed"})
		return
	}
	c.JSON(http.StatusOK, safeNodeOperation(op))
}

func (g *Gateway) handleListNodeOperations(c *gin.Context) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	options, ok := pageOptions(c, 100)
	if !ok {
		return
	}
	items, err := g.stationClient.Resource(nodeOperationGVR).Namespace(namespace).List(c.Request.Context(), options)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "node operations lookup failed"})
		return
	}
	out := make([]map[string]any, 0, len(items.Items))
	for i := range items.Items {
		out = append(out, safeNodeOperation(&items.Items[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "continue": items.GetContinue()})
}
