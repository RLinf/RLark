package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// ClusterInfo defines the aggregate information returned by the cluster APIs.
type ClusterInfo struct {
	ID                     string                     `json:"id"`
	AgentNamespace         string                     `json:"agentNamespace"`
	Name                   string                     `json:"name"`
	Type                   string                     `json:"type"`
	Region                 string                     `json:"region"`
	Location               string                     `json:"location"`
	Phase                  rlarkv1alpha1.ClusterPhase `json:"phase"`
	TotalNodes             int                        `json:"totalNodes"`
	OnlineNodes            int                        `json:"onlineNodes"`
	OfflineNodes           int                        `json:"offlineNodes"`
	CloudNodes             int                        `json:"cloudNodes"`
	EmbodiedNodes          int                        `json:"embodiedNodes"`
	Robots                 int                        `json:"robots"`
	GPUModels              []string                   `json:"gpuModels"`
	RobotModels            []string                   `json:"robotModels"`
	CPUUsage               int                        `json:"cpuUsage"`
	GPUUsage               int                        `json:"gpuUsage"`
	RobotUsage             int                        `json:"robotUsage"`
	RunningJobs            int                        `json:"runningJobs"`
	Description            string                     `json:"description"`
	AgentNamespaceConflict bool                       `json:"agentNamespaceConflict,omitempty"`
}

// ClusterDetail holds details.
type ClusterDetail struct {
	ClusterInfo
	Nodes []rlarkv1alpha1.Node `json:"nodes"`
}

// nodeCategories 返回节点所属的分类集合，支持两种 label 形式：
// 1. 枚举值：rlark.io/node-category = "cloud,edge"（逗号分隔）
// 2. 布尔位：rlark.io/node-category-cloud = "true"（前端批量编辑使用）
// 两者任一命中即返回对应分类，去重后返回。
func nodeCategories(node rlarkv1alpha1.Node) []rlarkv1alpha1.NodeCategory {
	seen := make(map[rlarkv1alpha1.NodeCategory]struct{})
	var result []rlarkv1alpha1.NodeCategory
	add := func(c rlarkv1alpha1.NodeCategory) {
		if _, ok := seen[c]; !ok {
			seen[c] = struct{}{}
			result = append(result, c)
		}
	}
	if v := node.Labels[rlarkv1alpha1.LabelNodeCategory]; v != "" {
		for _, part := range strings.Split(v, ",") {
			switch rlarkv1alpha1.NodeCategory(strings.TrimSpace(part)) {
			case rlarkv1alpha1.NodeCategoryCloud:
				add(rlarkv1alpha1.NodeCategoryCloud)
			case rlarkv1alpha1.NodeCategoryEdge:
				add(rlarkv1alpha1.NodeCategoryEdge)
			case rlarkv1alpha1.NodeCategoryRobot:
				add(rlarkv1alpha1.NodeCategoryRobot)
			}
		}
	}
	for _, c := range []rlarkv1alpha1.NodeCategory{
		rlarkv1alpha1.NodeCategoryCloud,
		rlarkv1alpha1.NodeCategoryEdge,
		rlarkv1alpha1.NodeCategoryRobot,
	} {
		if node.Labels[rlarkv1alpha1.LabelNodeCategory+"-"+string(c)] == "true" {
			add(c)
		}
	}
	return result
}

func clusterAgentNamespaceConflict(cluster rlarkv1alpha1.Cluster) bool {
	condition := apimeta.FindStatusCondition(cluster.Status.Conditions, rlarkv1alpha1.ConditionTypeValidated)
	return condition != nil && condition.Reason == "AgentNamespaceConflict"
}

func buildClusterInfo(cluster rlarkv1alpha1.Cluster, nodes []rlarkv1alpha1.Node) ClusterInfo {
	info := ClusterInfo{
		ID:                     cluster.Name,
		AgentNamespace:         cluster.Spec.AgentNamespace,
		Name:                   cluster.Name,
		Phase:                  cluster.Status.Phase,
		AgentNamespaceConflict: clusterAgentNamespaceConflict(cluster),
		GPUModels:              []string{},
		RobotModels:            []string{},
	}
	gpuModels := make(map[string]struct{})
	robotModels := make(map[string]struct{})
	runningJobs := make(map[string]struct{})

	for _, node := range nodes {
		info.TotalNodes++
		if node.Status.Phase == rlarkv1alpha1.NodeOnline {
			info.OnlineNodes++
		} else {
			info.OfflineNodes++
		}

		for _, category := range nodeCategories(node) {
			switch category {
			case rlarkv1alpha1.NodeCategoryCloud:
				info.CloudNodes++
				if model := node.Labels["rlark.io/model"]; model != "" {
					gpuModels[model] = struct{}{}
				}
			case rlarkv1alpha1.NodeCategoryEdge:
				info.EmbodiedNodes++
			case rlarkv1alpha1.NodeCategoryRobot:
				info.Robots++
				if model := node.Labels["rlark.io/model"]; model != "" {
					robotModels[model] = struct{}{}
				}
			}
		}

		if info.Region == "" {
			info.Region = node.Labels["rlark.io/region"]
		}
		if info.Location == "" {
			info.Location = node.Annotations["rlark.io/city"]
			if info.Location == "" {
				info.Location = node.Labels["rlark.io/city"]
			}
		}
		if taskName := node.Labels["rlark.io/embodied-task-name"]; taskName != "" {
			runningJobs[taskName] = struct{}{}
		} else if taskName := node.Labels["rlark.io/task-name"]; taskName != "" {
			runningJobs[taskName] = struct{}{}
		}
	}
	info.RunningJobs = len(runningJobs)

	hybrid := info.TotalNodes != info.CloudNodes+info.EmbodiedNodes+info.Robots // 有无标签的节点则显示为混合集群
	if info.CloudNodes > 0 && info.EmbodiedNodes == 0 && info.Robots == 0 && !hybrid {
		info.Type = "Cloud"
	} else if info.CloudNodes == 0 && (info.EmbodiedNodes > 0 || info.Robots > 0) && !hybrid {
		info.Type = "Embodied"
	} else {
		info.Type = "Hybrid"
	}

	for model := range gpuModels {
		info.GPUModels = append(info.GPUModels, model)
	}
	for model := range robotModels {
		info.RobotModels = append(info.RobotModels, model)
	}
	sort.Strings(info.GPUModels)
	sort.Strings(info.RobotModels)
	return info
}

func (g *Gateway) clusterNodes(c *gin.Context) ([]rlarkv1alpha1.Node, error) {
	if g.kubeClient == nil {
		return []rlarkv1alpha1.Node{}, nil
	}
	nodes, err := g.kubeClient.RlinfV1alpha1().Nodes(metav1.NamespaceAll).List(
		c.Request.Context(), metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

// listClusters handles the Cluster CR list request.
func (g *Gateway) listClusters(c *gin.Context) {
	if g.kubeClient == nil {
		c.JSON(http.StatusOK, gin.H{"data": []ClusterInfo{}, "success": true})
		return
	}

	clusterList, err := g.kubeClient.RlinfV1alpha1().Clusters().List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	nodes, err := g.clusterNodes(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	grouped := make(map[string][]rlarkv1alpha1.Node)
	for _, node := range nodes {
		grouped[node.Namespace] = append(grouped[node.Namespace], node)
	}

	clusters := make([]ClusterInfo, 0, len(clusterList.Items))
	for _, cluster := range clusterList.Items {
		if cluster.Spec.Delisted && c.GetString(authRoleKey) != "admin" {
			continue
		}
		clusters = append(clusters, buildClusterInfo(cluster, grouped[cluster.Spec.AgentNamespace]))
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].Name < clusters[j].Name })
	c.JSON(http.StatusOK, gin.H{"data": clusters, "success": true})
}

// updateCluster applies one idempotent administrator lifecycle transition.
func createMergePatch(before, after *rlarkv1alpha1.Cluster) ([]byte, error) {
	original, err := json.Marshal(before)
	if err != nil {
		return nil, fmt.Errorf("marshal original Cluster: %w", err)
	}
	modified, err := json.Marshal(after)
	if err != nil {
		return nil, fmt.Errorf("marshal modified Cluster: %w", err)
	}
	patch, err := jsonpatch.CreateMergePatch(original, modified)
	if err != nil {
		return nil, fmt.Errorf("create Cluster merge patch: %w", err)
	}
	return patch, nil
}

var (
	errClusterDelisted = errors.New("cluster is delisted")
	errClusterDraining = errors.New("cluster is draining")
)

func (g *Gateway) updateCluster(c *gin.Context, update func(*rlarkv1alpha1.Cluster) error) {
	if g.kubeClient == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
		return
	}

	clusterID := c.Param("cluster_id")
	if clusterID == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "cluster_id is required")
		return
	}
	var updated *rlarkv1alpha1.Cluster
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(c.Request.Context(), clusterID, metav1.GetOptions{})
		if err != nil {
			return err
		}
		before := cluster.DeepCopy()
		if err := update(cluster); err != nil {
			return err
		}
		patch, err := createMergePatch(before, cluster)
		if err != nil {
			return err
		}
		updated, err = g.kubeClient.RlinfV1alpha1().Clusters().Patch(c.Request.Context(), clusterID, types.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
			return
		}
		switch {
		case errors.Is(err, errClusterDelisted), errors.Is(err, errClusterDraining):
			respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
		case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		default:
			respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		}
		return
	}
	nodes, err := g.clusterNodes(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	clusterNodes := make([]rlarkv1alpha1.Node, 0)
	for _, node := range nodes {
		if node.Namespace == updated.Spec.AgentNamespace {
			clusterNodes = append(clusterNodes, node)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": buildClusterInfo(*updated, clusterNodes), "success": true})
}

func (g *Gateway) cordonCluster(c *gin.Context) {
	g.updateCluster(c, cordonClusterSpec)
}

func cordonClusterSpec(cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.Delisted {
		return fmt.Errorf("delisted cluster must be restored before it can be cordoned: %w", errClusterDelisted)
	}
	cluster.Spec.Cordoned = true
	return nil
}

func (g *Gateway) uncordonCluster(c *gin.Context) {
	g.updateCluster(c, uncordonClusterSpec)
}

func uncordonClusterSpec(cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.Delisted {
		return fmt.Errorf("delisted cluster must be restored before it can be uncordoned: %w", errClusterDelisted)
	}
	if cluster.Spec.Draining {
		if !drainComplete(cluster) {
			return fmt.Errorf("draining cluster must complete draining before it can be uncordoned: %w", errClusterDraining)
		}
		// Uncordoning a fully drained cluster cancels the drain intent as a whole.
		// Leaving Spec.Draining set would keep the cluster unschedulable and let the
		// cluster controller keep stopping Tasks, making the uncordon a silent no-op.
		cluster.Spec.Draining = false
		cluster.Spec.DrainPreviousCordoned = false
	}
	cluster.Spec.Cordoned = false
	return nil
}

func (g *Gateway) drainCluster(c *gin.Context) {
	g.updateCluster(c, drainClusterSpec)
}

func (g *Gateway) undrainCluster(c *gin.Context) {
	g.updateCluster(c, undrainClusterSpec)
}

func undrainClusterSpec(cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.Delisted {
		return fmt.Errorf("delisted cluster must be restored before its drain can be cancelled: %w", errClusterDelisted)
	}
	cluster.Spec.Draining = false
	cluster.Spec.Cordoned = cluster.Spec.DrainPreviousCordoned
	cluster.Spec.DrainPreviousCordoned = false
	return nil
}

func drainClusterSpec(cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.Delisted {
		return fmt.Errorf("delisted cluster must be restored before it can be drained: %w", errClusterDelisted)
	}
	if !cluster.Spec.Draining {
		cluster.Spec.DrainPreviousCordoned = cluster.Spec.Cordoned
	}
	cluster.Spec.Cordoned = true
	cluster.Spec.Draining = true
	return nil
}

func (g *Gateway) delistCluster(c *gin.Context) {
	g.updateCluster(c, delistClusterSpec)
}

func drainComplete(cluster *rlarkv1alpha1.Cluster) bool {
	condition := apimeta.FindStatusCondition(cluster.Status.Conditions, rlarkv1alpha1.ConditionTypeProgressing)
	return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == "DrainComplete" && condition.ObservedGeneration == cluster.Generation
}

func delistClusterSpec(cluster *rlarkv1alpha1.Cluster) error {
	if cluster.Spec.Draining && !drainComplete(cluster) {
		return fmt.Errorf("cluster drain must complete before it can be delisted: %w", errClusterDraining)
	}
	cluster.Spec.Delisted = true
	return nil
}

func (g *Gateway) deleteCluster(c *gin.Context) {
	if g.kubeClient == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
		return
	}

	clusterID := c.Param("cluster_id")
	if clusterID == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "cluster_id is required")
		return
	}
	if err := g.kubeClient.RlinfV1alpha1().Clusters().Delete(c.Request.Context(), clusterID, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (g *Gateway) restoreCluster(c *gin.Context) {
	g.updateCluster(c, func(cluster *rlarkv1alpha1.Cluster) error {
		cluster.Spec.Delisted = false
		return nil
	})
}

func (g *Gateway) getCluster(c *gin.Context) {
	if g.kubeClient == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
		return
	}

	clusterID := c.Param("cluster_id")
	if clusterID == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "cluster_id is required")
		return
	}
	cluster, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(c.Request.Context(), clusterID, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	nodes, err := g.clusterNodes(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	if cluster.Spec.Delisted && c.GetString(authRoleKey) != "admin" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
		return
	}

	clusterNodes := make([]rlarkv1alpha1.Node, 0)
	for _, node := range nodes {
		if node.Namespace == cluster.Spec.AgentNamespace {
			clusterNodes = append(clusterNodes, node)
		}
	}

	detail := ClusterDetail{
		ClusterInfo: buildClusterInfo(*cluster, clusterNodes),
		Nodes:       clusterNodes,
	}
	c.JSON(http.StatusOK, gin.H{"data": detail, "success": true})
}
