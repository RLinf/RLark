package gateway

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

type stationRunningImage struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Image     string `json:"image"`
	ImageID   string `json:"imageID,omitempty"`
}

// handleStationRunningImages returns an admin-only snapshot of observed container
// images from the member cluster that owns the selected station record.
func (g *Gateway) handleStationRunningImages(c *gin.Context) {
	name, uid := c.Param("name"), c.Query("node_uid")
	if len(validation.IsDNS1123Subdomain(name)) > 0 || len(validation.IsDNS1123Subdomain(c.Query("cluster_id"))) > 0 || uid == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "valid node name, cluster_id and node_uid are required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	clusterID, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	agentID, prefixed := strings.CutPrefix(namespace, apis.RLarkAgentNamespacePrefix)
	if !prefixed || len(validation.IsDNS1123Label(agentID)) > 0 {
		respondError(c, http.StatusConflict, CodeResourceConflict, "Cluster namespace does not identify a Kubernetes agent")
		return
	}

	upstreamError := func(err error) {
		log.FromContext(ctx).Error(err, "Cannot read station running images", "cluster", clusterID, "node", name)
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "Running container images are unavailable")
	}
	clusters, err := g.stationClient.Resource(stationClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		upstreamError(err)
		return
	}
	owner, conflicted := "", false
	for _, cluster := range clusters.Items {
		if claims, _, _ := unstructured.NestedString(cluster.Object, "spec", "agentNamespace"); claims == namespace {
			conflicted = owner != "" && owner != cluster.GetName()
			owner = cluster.GetName()
			if conflicted {
				break
			}
		}
	}
	if conflicted || owner != clusterID {
		respondError(c, http.StatusConflict, CodeResourceConflict, "Cluster namespace ownership could not be verified")
		return
	}

	record, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, "Could not read site node record", err)
		return
	}
	boundUID, _, _ := unstructured.NestedString(record.Object, "spec", "nodeUID")
	recordCluster, _, _ := unstructured.NestedString(record.Object, "spec", "clusterID")
	retired, _, _ := unstructured.NestedBool(record.Object, "spec", "retired")
	if retired || boundUID != uid || recordCluster != clusterID {
		respondError(c, http.StatusConflict, CodeResourceConflict, "Site node record no longer matches this node instance")
		return
	}

	// A Pod names its node without a UID. Check the member Node on both
	// sides of the read so name reuse during the snapshot cannot mix instances.
	verifyNode := func() bool {
		var node corev1.Node
		status, err := g.readStationImageObject(ctx, agentID, "/api/v1/nodes/"+url.PathEscape(name), &node)
		switch {
		case status == http.StatusNotFound:
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "Node is absent from the member cluster")
		case err != nil:
			upstreamError(err)
		case node.Name != name || string(node.UID) != boundUID:
			respondError(c, http.StatusConflict, CodeResourceConflict, "Member node identity does not match the site node record")
		default:
			return true
		}
		return false
	}
	if !verifyNode() {
		return
	}
	query := url.Values{"fieldSelector": {fields.OneTermEqualSelector("spec.nodeName", name).String()}, "limit": {"200"}}
	images := []stationRunningImage{}
	for {
		var page corev1.PodList
		if _, err := g.readStationImageObject(ctx, agentID, "/api/v1/pods?"+query.Encode(), &page); err != nil {
			upstreamError(err)
			return
		}
		for _, pod := range page.Items {
			if pod.Spec.NodeName != name || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
				continue
			}
			for _, statuses := range [][]corev1.ContainerStatus{pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses, pod.Status.EphemeralContainerStatuses} {
				for _, status := range statuses {
					if status.State.Running != nil {
						images = append(images, stationRunningImage{pod.Namespace, pod.Name, status.Name, status.Image, status.ImageID})
					}
				}
			}
		}
		if page.Continue == "" {
			break
		}
		if page.Continue == query.Get("continue") {
			upstreamError(fmt.Errorf("member Pod list repeated its continuation token"))
			return
		}
		query.Set("continue", page.Continue)
	}
	if !verifyNode() {
		return
	}
	slices.SortFunc(images, func(a, b stationRunningImage) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Pod, b.Pod), cmp.Compare(a.Container, b.Container))
	})
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": images})
}

func (g *Gateway) readStationImageObject(ctx context.Context, agentID, path string, object any) (int, error) {
	response, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, path, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.FromContext(ctx).V(1).Info("Could not close member Kubernetes response body", "error", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, fmt.Errorf("member Kubernetes API returned HTTP %d", response.StatusCode)
	}
	return response.StatusCode, json.NewDecoder(response.Body).Decode(object)
}
