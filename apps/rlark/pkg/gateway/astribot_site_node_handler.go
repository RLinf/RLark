package gateway

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	siteNodeGVR           = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "nodes"}
	networkEnrollmentGVR  = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "networkenrollments"}
	siteNodeRobotLabel    = "rlark.io/robot-id"
	siteNodeWorkstationID = "rlark.io/workstation-id"
)

// siteNode is the read-only view a scheduler of work (a Fleet Run) needs of
// one machine: who it is, whether it is there, and the one address the other
// machines reach it at. It carries nothing about what runs on the machine.
type siteNode struct {
	ClusterID string `json:"cluster_id"`
	NodeName  string `json:"node_name"`
	// SiteNodeID is the Station object's name: stable across reinstalls.
	SiteNodeID string `json:"site_node_id"`
	Sequence   int64  `json:"sequence"`
	// BindingVersion increases when an administrator rebinds the identity to
	// a reinstalled machine; a consumer that cached it knows to look again.
	BindingVersion int64  `json:"binding_version"`
	Phase          string `json:"phase"`
	// Online is the heartbeat alone: the node's agent reported within the
	// last 90 s. The three fields after it are separate facts; a consumer
	// that needs the node reachable checks Tunnel as well.
	Online bool `json:"online"`
	// Tunnel is "verified" when the node reaches every peer that is itself
	// online, "failed" when it does not, "unknown" when it reports no probes.
	Tunnel string `json:"tunnel"`
	// RLarkNode is "ready" when the RLark Node of the same name is Ready.
	RLarkNode    string `json:"rlark_node"`
	LastSeenTime string `json:"last_seen_time,omitempty"`
	// BootTime changes when the machine restarts, also when the restart was
	// too short to show as offline; Reboots counts the changes seen.
	BootTime      string            `json:"boot_time,omitempty"`
	Reboots       int64             `json:"reboots"`
	Labels        map[string]string `json:"labels,omitempty"`
	Network       *siteNodeNetwork  `json:"network,omitempty"`
	RobotID       string            `json:"robot_id,omitempty"`
	WorkstationID string            `json:"workstation_id,omitempty"`
}

type siteNodeNetwork struct {
	State string `json:"state"`
	Site  string `json:"site,omitempty"`
	Role  string `json:"role,omitempty"`
	// Address is the one address every other member reaches the node at.
	Address       string `json:"address,omitempty"`
	TunnelAddress string `json:"tunnel_address,omitempty"`
	// PublicKey is the WireGuard public key the node enrolled with; its
	// private key never leaves the machine.
	PublicKey string `json:"public_key,omitempty"`
	// Links is, for a leaf, what each hub it dials sees of it, and
	// LinkSummary the line to show for them: empty while all are connected.
	Links       []siteNodeLink `json:"links,omitempty"`
	LinkSummary string         `json:"link_summary,omitempty"`
}

type siteNodeLink struct {
	Hub string `json:"hub"`
	// State is Connected, Unanswered (the node's requests arrive at the hub
	// and its answers do not get back), Silent (nothing arrives) or Unknown.
	State           string `json:"state"`
	Since           string `json:"since"`
	LatestHandshake string `json:"latest_handshake,omitempty"`
}

// siteNodes joins, for one cluster, every unretired identity with its network
// enrollment and its RLark Node.
func (g *Gateway) siteNodes(c *gin.Context, clusterID, namespace string) ([]siteNode, error) {
	ctx := c.Request.Context()
	stations, err := g.stationClient.Resource(stationRecordGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	enrollments, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodes, err := g.stationClient.Resource(siteNodeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	enrollmentOf, nodeOf := map[string]*unstructured.Unstructured{}, map[string]*unstructured.Unstructured{}
	for i := range enrollments.Items {
		enrollmentOf[enrollments.Items[i].GetName()] = &enrollments.Items[i]
	}
	for i := range nodes.Items {
		nodeOf[nodes.Items[i].GetName()] = &nodes.Items[i]
	}
	results := make([]siteNode, 0, len(stations.Items))
	for i := range stations.Items {
		station := &stations.Items[i]
		if retired, _, _ := unstructured.NestedBool(station.Object, "spec", "retired"); retired {
			continue
		}
		result := siteNode{ClusterID: clusterID, SiteNodeID: station.GetName(), Tunnel: "unknown", RLarkNode: "unknown"}
		result.NodeName, _, _ = unstructured.NestedString(station.Object, "spec", "binding", "nodeName")
		result.Sequence, _, _ = unstructured.NestedInt64(station.Object, "spec", "sequence")
		result.BindingVersion, _, _ = unstructured.NestedInt64(station.Object, "spec", "binding", "version")
		result.Phase, _, _ = unstructured.NestedString(station.Object, "status", "phase")
		result.LastSeenTime, _, _ = unstructured.NestedString(station.Object, "status", "lastSeenTime")
		result.BootTime, _, _ = unstructured.NestedString(station.Object, "status", "bootTime")
		result.Reboots, _, _ = unstructured.NestedInt64(station.Object, "status", "reboots")
		result.Online = result.Phase == "Bound"
		conditions, _, _ := unstructured.NestedSlice(station.Object, "status", "conditions")
		for _, raw := range conditions {
			condition, _ := raw.(map[string]interface{})
			status, _ := condition["status"].(string)
			switch condition["type"] {
			case "NetworkVerified":
				result.Tunnel = map[string]string{"True": "verified", "False": "failed", "Unknown": "unknown"}[status]
			case "RLarkLinked":
				result.RLarkNode = map[string]string{"True": "ready", "False": "not-ready", "Unknown": "unknown"}[status]
			}
		}
		// An answer for an earlier binding describes a machine that is gone.
		if enrollment := enrollmentOf[result.NodeName]; enrollment != nil {
			if version, _, _ := unstructured.NestedInt64(enrollment.Object, "spec", "bindingVersion"); version == result.BindingVersion {
				status, _, _ := unstructured.NestedMap(enrollment.Object, "status")
				text := func(key string) string { value, _ := status[key].(string); return value }
				result.Network = &siteNodeNetwork{State: text("phase"), Site: text("site"), Role: text("role"), Address: text("address"), TunnelAddress: text("tunnelAddress"), LinkSummary: text("linkSummary")}
				result.Network.PublicKey, _, _ = unstructured.NestedString(enrollment.Object, "spec", "publicKey")
				links, _ := status["links"].([]interface{})
				for _, raw := range links {
					link, _ := raw.(map[string]interface{})
					text := func(key string) string { value, _ := link[key].(string); return value }
					result.Network.Links = append(result.Network.Links, siteNodeLink{Hub: text("hub"), State: text("state"), Since: text("since"), LatestHandshake: text("latestHandshake")})
				}
			}
		}
		if node := nodeOf[result.NodeName]; node != nil {
			result.Labels = node.GetLabels()
			result.RobotID, result.WorkstationID = result.Labels[siteNodeRobotLabel], result.Labels[siteNodeWorkstationID]
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Sequence < results[j].Sequence })
	return results, nil
}

// siteNodesOfRequest resolves cluster_id and lists that cluster's site nodes.
func (g *Gateway) siteNodesOfRequest(c *gin.Context) ([]siteNode, bool) {
	clusterID, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return nil, false
	}
	results, err := g.siteNodes(c, clusterID, namespace)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Station API request failed"})
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return results, true
}

// handleListSiteNodes answers GET /api/v1/site-nodes?cluster_id=&label=key=value:
// every site node of a cluster in one call, optionally those carrying a label.
func (g *Gateway) handleListSiteNodes(c *gin.Context) {
	results, ok := g.siteNodesOfRequest(c)
	if !ok {
		return
	}
	if key, value, found := strings.Cut(c.Query("label"), "="); found {
		kept := results[:0]
		for _, result := range results {
			if result.Labels[key] == value {
				kept = append(kept, result)
			}
		}
		results = kept
	}
	c.JSON(http.StatusOK, gin.H{"items": results})
}

// handleGetSiteNode answers GET /api/v1/site-nodes/:name?cluster_id=. It is a
// lookup by Node name: callers keep the name in their own configuration and
// read everything else here.
func (g *Gateway) handleGetSiteNode(c *gin.Context) {
	results, ok := g.siteNodesOfRequest(c)
	if !ok {
		return
	}
	var found *siteNode
	for i := range results {
		if results[i].NodeName != c.Param("name") {
			continue
		}
		if found != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "more than one identity is bound to this node"})
			return
		}
		found = &results[i]
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site node not found"})
		return
	}
	c.JSON(http.StatusOK, found)
}
