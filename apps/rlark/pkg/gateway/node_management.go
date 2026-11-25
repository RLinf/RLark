package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// enabledLabel is the Kubernetes Node label that marks a node as managed by RLark.
const enabledLabel = "rlark.io/enabled"

// dataPlaneNodeSummary represents a Kubernetes Node from a data-plane cluster.
type dataPlaneNodeSummary struct {
	Name      string               `json:"name"`
	Labels    map[string]string    `json:"labels,omitempty"`
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`
	Ready     bool                 `json:"ready"`
	Managed   bool                 `json:"managed"` // true if labels[enabledLabel] == "true"
}

// clusterDataPlaneNodes holds the result of listing nodes from one cluster.
type clusterDataPlaneNodes struct {
	Cluster   string                 `json:"cluster"`
	Namespace string                 `json:"namespace"`
	Error     string                 `json:"error,omitempty"`
	Nodes     []dataPlaneNodeSummary `json:"nodes,omitempty"`
}

// listDataPlaneNodes lists Kubernetes Nodes from all visible data-plane clusters
// through the agent proxy. It returns both managed and unmanaged nodes so that
// the admin UI can display and manage unmanaged nodes.
func (g *Gateway) listDataPlaneNodes(c *gin.Context) {
	ctx := c.Request.Context()

	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}

	// Filter to clusters visible to the caller (skip delisted for non-admin;
	// admin sees everything for management purposes).
	isAdmin := c.GetString(authRoleKey) == "admin"
	visibleClusters := visibleClusterNamespaces(clusters.Items, isAdmin)

	results := make([]clusterDataPlaneNodes, 0, len(visibleClusters))
	for _, cluster := range visibleClusters {
		agentNamespace := cluster.Spec.AgentNamespace
		agentID := strings.TrimPrefix(agentNamespace, "rlark-")
		if agentID == agentNamespace || agentID == "" {
			// Not a valid agent namespace; skip.
			continue
		}
		result := clusterDataPlaneNodes{
			Cluster:   cluster.Name,
			Namespace: agentNamespace,
		}
		nodes, err := g.fetchDataPlaneNodes(ctx, agentID)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Nodes = nodes
		}
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{"clusters": results})
}

// fetchDataPlaneNodes GET /api/v1/nodes from one data-plane cluster.
func (g *Gateway) fetchDataPlaneNodes(ctx context.Context, agentID string) ([]dataPlaneNodeSummary, error) {
	kubePath := "/api/v1/nodes"
	resp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, kubePath, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var nodeList corev1.NodeList
	if err := json.Unmarshal(body, &nodeList); err != nil {
		return nil, fmt.Errorf("decode node list: %w", err)
	}

	summary := make([]dataPlaneNodeSummary, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ready = cond.Status == corev1.ConditionTrue
				break
			}
		}
		managed := node.Labels[enabledLabel] == "true"
		summary = append(summary, dataPlaneNodeSummary{
			Name:      node.Name,
			Labels:    node.Labels,
			Addresses: node.Status.Addresses,
			Ready:     ready,
			Managed:   managed,
		})
	}
	return summary, nil
}

// manageDataPlaneNode adds rlark.io/enabled=true to a Kubernetes Node in a data-plane cluster.
// agent_id identifies the cluster agent; node_name is the Kubernetes Node name.
func (g *Gateway) manageDataPlaneNode(c *gin.Context) {
	g.setDataPlaneNodeLabel(c, true)
}

// unmanageDataPlaneNode removes rlark.io/enabled label from a Kubernetes Node in a data-plane cluster.
func (g *Gateway) unmanageDataPlaneNode(c *gin.Context) {
	g.setDataPlaneNodeLabel(c, false)
}

// setDataPlaneNodeLabel is the shared implementation for manage/unmanage.
// enable=true adds the label via strategic merge patch;
// enable=false removes it via JSON merge patch where null is the standard delete marker (RFC 7396).
func (g *Gateway) setDataPlaneNodeLabel(c *gin.Context, enable bool) {
	ctx := c.Request.Context()

	agentID := c.Param("agent_id")
	nodeName := c.Param("node_name")
	if agentID == "" || nodeName == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "agent_id and node_name are required")
		return
	}

	if !enable {
		// 取消纳管前置校验:管理面 Node CR 与 K8s Node 同名,位于 agent 命名空间。
		if g.kubeClient == nil {
			respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client not configured")
			return
		}
		namespace := "rlark-" + agentID

		// 校验 1: 节点必须已 cordon,避免取消纳管过程中有新任务被调度上来。
		if err := ensureNodeCordoned(ctx, g.kubeClient, namespace, nodeName); err != nil {
			if errors.Is(err, errNodeNotCordoned) {
				rejectNodeUnmanageNotCordoned(c, namespace, nodeName)
				return
			}
			respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("check node cordon state: %v", err))
			return
		}

		// 校验 2: 节点上仍观测到任务时拒绝取消纳管,避免工作负载失去状态上报而被遗弃;
		// 运维需先排空节点并等待任务全部迁离。
		tasks, err := tasksObservedOnNode(ctx, g.kubeClient, namespace, nodeName)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("list tasks on node: %v", err))
			return
		}
		if len(tasks) > 0 {
			rejectNodeUnmanage(c, namespace, nodeName, tasks)
			return
		}
	}

	var (
		patchBody   []byte
		contentType string
		err         error
	)
	if enable {
		// Strategic merge patch safely adds/overwrites the single label without
		// touching other metadata fields.
		patchBody, err = json.Marshal(map[string]any{
			"metadata": map[string]any{
				"labels": map[string]string{
					enabledLabel: "true",
				},
			},
		})
		contentType = string(types.StrategicMergePatchType)
	} else {
		// JSON merge patch (RFC 7396) uses null as the canonical delete marker;
		// this is unambiguous for Kubernetes map fields unlike strategic merge patch.
		patchBody, err = json.Marshal(map[string]any{
			"metadata": map[string]any{
				"labels": map[string]any{
					enabledLabel: nil,
				},
			},
		})
		contentType = string(types.MergePatchType)
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("build patch: %v", err))
		return
	}

	kubePath := fmt.Sprintf("/api/v1/nodes/%s", nodeName)
	headers := map[string]string{
		"Content-Type": contentType,
	}
	resp, err := g.proxyKubeRequestWithHeaders(ctx, http.MethodPatch, agentID, kubePath, patchBody, headers)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("proxy request: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable,
			fmt.Sprintf("agent returned HTTP %d: %s", resp.StatusCode, string(body)))
		return
	}

	// Return a minimal confirmation.
	c.JSON(http.StatusOK, gin.H{
		"agentId":  agentID,
		"nodeName": nodeName,
		"enabled":  enable,
	})
}

// listDataPlaneNodesForCluster lists nodes for a single data-plane cluster.
func (g *Gateway) listDataPlaneNodesForCluster(c *gin.Context) {
	ctx := c.Request.Context()
	agentID := c.Param("agent_id")
	if agentID == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "agent_id is required")
		return
	}

	nodes, err := g.fetchDataPlaneNodes(ctx, agentID)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cluster": agentID,
		"nodes":   nodes,
	})
}

// nodeLabelsPatchBody is the request body for PATCH /nodes/:name/patch-labels.
// Callers supply a strategic merge patch of label keys to add/update or null
// markers to delete. All label keys are forwarded verbatim to the data-plane
// Kubernetes Node, including rlark.io/* system labels.
type nodeLabelsPatchBody struct {
	Labels map[string]any `json:"labels"`
}

// patchNodeLabelsRelay receives a management Node labels patch, resolves the
// owning data-plane agent, and relays the patch directly to the local
// Kubernetes Node through the Server proxy. This is a transparent relay: the
// Gateway does not author labels — it forwards the caller's patch verbatim to
// the agent, which applies it to its own local K8s Node.
//
// This replaces the old round-trip where the management CR's metadata.labels
// were written by the data plane and then echoed back as the UI source of truth.
// Labels now flow directly from operator → Gateway → agent → K8s Node; the
// management CR only carries RLark-owned system labels.
func (g *Gateway) patchNodeLabelsRelay(c *gin.Context) {
	if g.kubeClient == nil || g.serverTransport == nil {
		respondError(c, http.StatusNotFound, CodeInvalidRequest, "kube client or server transport not configured")
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name is required")
		return
	}
	if namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace query parameter is required")
		return
	}
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}

	// Parse body early; malformed JSON is a client error before we do any IO.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	var body nodeLabelsPatchBody
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
			return
		}
	}

	// Reject empty patches; there is nothing to relay.
	if len(body.Labels) == 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "labels map must not be empty")
		return
	}

	// Resolve management Node → Cluster → agentID. The management Node's
	// namespace IS the Cluster name; Cluster.Spec.AgentNamespace carries the
	// agent's lease namespace (format: "rlark-{agentID}").
	ctx := c.Request.Context()
	cluster, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, fmt.Sprintf("resolve cluster %q: %v", namespace, err.Error()), err)
		return
	}
	agentNamespace := cluster.Spec.AgentNamespace
	agentID := strings.TrimPrefix(agentNamespace, "rlark-")
	if agentID == agentNamespace || agentID == "" {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			fmt.Sprintf("cluster %q agentNamespace %q has invalid format (expected rlark-<agentID>)", namespace, agentNamespace))
		return
	}

	// Build the strategic merge patch body that the data-plane K8s API accepts.
	// We wrap the caller's labels dict into a standard metadata.labels patch.
	patchBody, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"labels": body.Labels,
		},
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("build patch: %v", err))
		return
	}

	kubePath := fmt.Sprintf("/api/v1/nodes/%s", name)
	headers := map[string]string{
		"Content-Type": string(types.StrategicMergePatchType),
	}
	resp, err := g.proxyKubeRequestWithHeaders(ctx, http.MethodPatch, agentID, kubePath, patchBody, headers)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("relay to agent %s: %v", agentID, err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		// Surface agent's response verbatim so the caller sees K8s validation
		// errors (invalid label keys, too-long values, etc.).
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	// Relay succeeded. Return a minimal confirmation — the caller doesn't need
	// the full K8s Node object back, and the next UI refresh will pick up the
	// change through observedNodeLabels.
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"agentId":  agentID,
		"nodeName": name,
	})
}
