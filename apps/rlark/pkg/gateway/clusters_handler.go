package gateway

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// ClusterInfo defines the aggregate information returned by the cluster APIs.
type ClusterInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Region        string   `json:"region"`
	Location      string   `json:"location"`
	Phase         string   `json:"phase"`
	TotalNodes    int      `json:"totalNodes"`
	OnlineNodes   int      `json:"onlineNodes"`
	OfflineNodes  int      `json:"offlineNodes"`
	CloudNodes    int      `json:"cloudNodes"`
	EmbodiedNodes int      `json:"embodiedNodes"`
	Robots        int      `json:"robots"`
	GPUModels     []string `json:"gpuModels"`
	RobotModels   []string `json:"robotModels"`
	CPUUsage      int      `json:"cpuUsage"`
	GPUUsage      int      `json:"gpuUsage"`
	RobotUsage    int      `json:"robotUsage"`
	RunningJobs   int      `json:"runningJobs"`
	Description   string   `json:"description"`
}

// ClusterDetail holds details.
type ClusterDetail struct {
	ClusterInfo
	Nodes []rlarkv1alpha1.Node `json:"nodes"`
}

func buildClusterInfo(clusterID string, nodes []rlarkv1alpha1.Node) ClusterInfo {
	info := ClusterInfo{
		ID:          clusterID,
		Name:        clusterID,
		GPUModels:   []string{},
		RobotModels: []string{},
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

		switch node.Labels[rlarkv1alpha1.LabelNodeCategory] {
		case string(rlarkv1alpha1.NodeCategoryCloud):
			info.CloudNodes++
			if model := node.Labels["rlark.io/model"]; model != "" {
				gpuModels[model] = struct{}{}
			}
		case string(rlarkv1alpha1.NodeCategoryEdge):
			info.EmbodiedNodes++
		case string(rlarkv1alpha1.NodeCategoryRobot):
			info.Robots++
			if model := node.Labels["rlark.io/model"]; model != "" {
				robotModels[model] = struct{}{}
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

	switch {
	case info.TotalNodes == 0 || info.OnlineNodes == 0:
		info.Phase = "Offline"
	case info.OnlineNodes == info.TotalNodes:
		info.Phase = "Online"
	default:
		info.Phase = "Degraded"
	}
	if info.CloudNodes > 0 && info.EmbodiedNodes == 0 && info.Robots == 0 {
		info.Type = "Cloud"
	} else if info.CloudNodes == 0 && (info.EmbodiedNodes > 0 || info.Robots > 0) {
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

// listClusters handles the aggregated cluster list request.
func (g *Gateway) listClusters(c *gin.Context) {
	nodes, err := g.clusterNodes(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	grouped := make(map[string][]rlarkv1alpha1.Node)
	for _, node := range nodes {
		clusterID := node.Labels[rlarkv1alpha1.LabelClusterID]
		if clusterID != "" {
			grouped[clusterID] = append(grouped[clusterID], node)
		}
	}

	clusters := make([]ClusterInfo, 0, len(grouped))
	for clusterID, clusterNodes := range grouped {
		clusters = append(clusters, buildClusterInfo(clusterID, clusterNodes))
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].Name < clusters[j].Name })
	c.JSON(http.StatusOK, gin.H{"data": clusters, "success": true})
}

// getCluster handles a single aggregated cluster detail request.
func (g *Gateway) getCluster(c *gin.Context) {
	clusterID := c.Param("cluster_id")
	nodes, err := g.clusterNodes(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	clusterNodes := make([]rlarkv1alpha1.Node, 0)
	for _, node := range nodes {
		if node.Labels[rlarkv1alpha1.LabelClusterID] == clusterID {
			clusterNodes = append(clusterNodes, node)
		}
	}
	if len(clusterNodes) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "cluster not found"})
		return
	}

	detail := ClusterDetail{
		ClusterInfo: buildClusterInfo(clusterID, clusterNodes),
		Nodes:       clusterNodes,
	}
	c.JSON(http.StatusOK, gin.H{"data": detail, "success": true})
}
