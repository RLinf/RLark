package gateway

import (
	"net/http"
	"slices"
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
	// NodeUID is the UID of the Kubernetes Node the record is of: it changes
	// when the machine is reinstalled or replaced under the name.
	NodeUID string `json:"node_uid"`
	// Phase is Bound, Offline or Conflict; empty until the node's first report.
	Phase string `json:"phase"`
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

// siteNodes is, for one cluster, every unretired site node (NetworkEnrollment) with its RLark Node's labels, by name.
func (g *Gateway) siteNodes(c *gin.Context, clusterID, namespace string) ([]siteNode, error) {
	ctx := c.Request.Context()
	enrollments, err := g.stationClient.Resource(networkEnrollmentGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodes, err := g.stationClient.Resource(siteNodeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	labelsOf := map[string]map[string]string{}
	for i := range nodes.Items {
		labelsOf[nodes.Items[i].GetName()] = nodes.Items[i].GetLabels()
	}
	results := make([]siteNode, 0, len(enrollments.Items))
	for i := range enrollments.Items {
		enrollment := &enrollments.Items[i]
		if retired, _, _ := unstructured.NestedBool(enrollment.Object, "spec", "retired"); retired {
			continue
		}
		result := siteNode{ClusterID: clusterID, NodeName: enrollment.GetName(), Tunnel: "unknown", RLarkNode: "unknown", Labels: labelsOf[enrollment.GetName()]}
		result.RobotID, result.WorkstationID = result.Labels[siteNodeRobotLabel], result.Labels[siteNodeWorkstationID]
		result.NodeUID, _, _ = unstructured.NestedString(enrollment.Object, "spec", "nodeUID")
		result.Phase, _, _ = unstructured.NestedString(enrollment.Object, "status", "node", "state")
		result.LastSeenTime, _, _ = unstructured.NestedString(enrollment.Object, "status", "node", "lastSeenTime")
		result.BootTime, _, _ = unstructured.NestedString(enrollment.Object, "status", "node", "bootTime")
		result.Reboots, _, _ = unstructured.NestedInt64(enrollment.Object, "status", "node", "reboots")
		result.Online = result.Phase == "Bound"
		conditions, _, _ := unstructured.NestedSlice(enrollment.Object, "status", "node", "conditions")
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
		// A node without a key is not in the tunnel network.
		if key, _, _ := unstructured.NestedString(enrollment.Object, "spec", "publicKey"); key != "" {
			status, _, _ := unstructured.NestedMap(enrollment.Object, "status")
			text := func(key string) string { value, _ := status[key].(string); return value }
			result.Network = &siteNodeNetwork{State: text("phase"), Site: text("site"), Role: text("role"), Address: text("address"), TunnelAddress: text("tunnelAddress"), PublicKey: key, LinkSummary: text("linkSummary")}
			links, _ := status["links"].([]interface{})
			for _, raw := range links {
				link, _ := raw.(map[string]interface{})
				text := func(key string) string { value, _ := link[key].(string); return value }
				result.Network.Links = append(result.Network.Links, siteNodeLink{Hub: text("hub"), State: text("state"), Since: text("since"), LatestHandshake: text("latestHandshake")})
			}
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].NodeName < results[j].NodeName })
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
	index := slices.IndexFunc(results, func(result siteNode) bool { return result.NodeName == c.Param("name") })
	if index < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "site node not found"})
		return
	}
	c.JSON(http.StatusOK, results[index])
}
