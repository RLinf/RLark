package gateway

import (
	"context"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
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
	ReadyNodes             int                        `json:"readyNodes"`
	DegradedNodes          int                        `json:"degradedNodes"`
	DisconnectedNodes      int                        `json:"disconnectedNodes"`
	UnschedulableNodes     int                        `json:"unschedulableNodes"`
	DominantFailureReason  string                     `json:"dominantFailureReason,omitempty"`
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

// nodeCategories 返回节点所属的分类集合。分类由管理面维护在 Node CR 的
// Annotation 上，支持两种形式：
// 1. 枚举值：rlark.io/node-category = "cloud,edge"（逗号分隔）
// 2. 布尔位：rlark.io/node-category-cloud = "true"（前端管理界面使用）
// 旧版本写入的 Label 形式继续作为兼容回退读取；Annotation 存在时优先生效。
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
	value := func(key string) string {
		if v, ok := node.Annotations[key]; ok {
			return v
		}
		return node.Labels[key]
	}
	if v := value(rlarkv1alpha1.LabelNodeCategory); v != "" {
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
		if value(rlarkv1alpha1.LabelNodeCategory+"-"+string(c)) == "true" {
			add(c)
		}
	}
	return result
}

// nodeMetadataValue prefers the administrator annotation on the management Node
// CR and falls back to the legacy label form.
func nodeMetadataValue(node rlarkv1alpha1.Node, key string) string {
	if v := node.Annotations[key]; v != "" {
		return v
	}
	return node.Labels[key]
}

func clusterAgentNamespaceConflict(cluster rlarkv1alpha1.Cluster) bool {
	condition := apimeta.FindStatusCondition(cluster.Status.Conditions, rlarkv1alpha1.ConditionTypeValidated)
	return condition != nil && condition.Reason == "AgentNamespaceConflict"
}

func buildClusterInfo(cluster rlarkv1alpha1.Cluster, nodes []rlarkv1alpha1.Node, runningJobs int) ClusterInfo {
	info := ClusterInfo{
		ID:                     cluster.Name,
		AgentNamespace:         cluster.Spec.AgentNamespace,
		Name:                   cluster.Name,
		Phase:                  cluster.Status.Phase,
		TotalNodes:             int(cluster.Status.TotalNodes),
		OnlineNodes:            int(cluster.Status.OnlineNodes),
		ReadyNodes:             int(cluster.Status.ReadyNodes),
		DegradedNodes:          int(cluster.Status.DegradedNodes),
		DisconnectedNodes:      int(cluster.Status.DisconnectedNodes),
		UnschedulableNodes:     int(cluster.Status.UnschedulableNodes),
		DominantFailureReason:  cluster.Status.DominantFailureReason,
		AgentNamespaceConflict: clusterAgentNamespaceConflict(cluster),
		RunningJobs:            runningJobs,
		GPUModels:              []string{},
		RobotModels:            []string{},
	}
	gpuModels := make(map[string]struct{})
	robotModels := make(map[string]struct{})

	for _, node := range nodes {
		for _, category := range nodeCategories(node) {
			switch category {
			case rlarkv1alpha1.NodeCategoryCloud:
				info.CloudNodes++
				if model := nodeMetadataValue(node, "rlark.io/gpu-model"); model != "" {
					gpuModels[model] = struct{}{}
				} else if model := nodeMetadataValue(node, "rlark.io/model"); model != "" {
					gpuModels[model] = struct{}{}
				}
			case rlarkv1alpha1.NodeCategoryEdge:
				info.EmbodiedNodes++
			case rlarkv1alpha1.NodeCategoryRobot:
				info.Robots++
				if model := nodeMetadataValue(node, "rlark.io/device-model"); model != "" {
					robotModels[model] = struct{}{}
				} else if model := nodeMetadataValue(node, "rlark.io/model"); model != "" {
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
	}
	info.OfflineNodes = info.DisconnectedNodes
	categorizedNodes := info.CloudNodes + info.EmbodiedNodes + info.Robots
	categoryTotal := info.TotalNodes
	if categoryTotal == 0 {
		categoryTotal = categorizedNodes
	}

	hybrid := categorizedNodes != 0 && categoryTotal != categorizedNodes // 有无标签的节点则显示为混合集群
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

// clusterRunningJobs counts the RLark Tasks still active in each Agent namespace.
// Tasks are placed into the namespace of their target Cluster (the Node's own
// namespace, i.e. cluster.spec.agentNamespace), so that namespace is the
// cluster-scoped source of truth for running work. The data-plane Agent never
// writes a task label onto the Node CR, so counting Node labels always yielded
// zero for Kubernetes clusters; counting the Task phase instead matches the
// drain/delist gating, which is defined over non-terminal, non-stopped Tasks.
func (g *Gateway) clusterRunningJobs(c *gin.Context) (map[string]int, error) {
	if g.kubeClient == nil {
		return map[string]int{}, nil
	}
	tasks, err := g.kubeClient.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(
		c.Request.Context(), metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}
	running := make(map[string]int)
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !task.DeletionTimestamp.IsZero() {
			continue
		}
		switch task.Status.Phase {
		case rlarkv1alpha1.TaskPhasePending, rlarkv1alpha1.TaskPhaseRunning:
			running[task.Namespace]++
		}
	}
	return running, nil
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
	runningJobs, err := g.clusterRunningJobs(c)
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
		clusters = append(clusters, buildClusterInfo(cluster, grouped[cluster.Spec.AgentNamespace], runningJobs[cluster.Spec.AgentNamespace]))
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
	g.updateClusterWithCleanup(c, update, nil)
}

// updateClusterWithCleanup applies update and then, when cleanup is non-nil,
// runs it after the Cluster patch succeeds but before responding. Callers use
// cleanup for side effects that must accompany the spec change, such as
// releasing Tasks a cancelled drain stopped.
func (g *Gateway) updateClusterWithCleanup(c *gin.Context, update func(*rlarkv1alpha1.Cluster) error, cleanup func(context.Context, *rlarkv1alpha1.Cluster) error) {
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
	if cleanup != nil {
		if err := cleanup(c.Request.Context(), updated); err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
			return
		}
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
	runningJobs, err := g.clusterRunningJobs(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": buildClusterInfo(*updated, clusterNodes, runningJobs[updated.Spec.AgentNamespace]), "success": true})
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
		// The drain stop markers are deliberately left on the Tasks: uncordon only
		// restores scheduling, so evicted Tasks stay Stopped until the operator
		// starts their Job or unsuspends the Task.
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
	g.updateClusterWithCleanup(c, undrainClusterSpec, func(ctx context.Context, cluster *rlarkv1alpha1.Cluster) error {
		return clearClusterDrainStops(ctx, g.kubeClient, cluster.Spec.AgentNamespace)
	})
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

const taskJobLabel = "rlinf.io/job"

// clusterDrainStopped reports whether a cluster drain stopped this Task. The two
// markers are written independently: job-owned Tasks rely on the Job controller
// honouring ClusterStoppedAnnotation, while standalone Tasks are also suspended
// by the drain, which records that ownership in ClusterDrainSuspendedAnnotation.
func clusterDrainStopped(task *rlarkv1alpha1.Task) bool {
	return task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] == "true" ||
		task.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] == "true"
}

// clearClusterDrainStop removes the cluster drain markers from task. Only a
// suspension the drain wrote itself is resumed; a Task the user had already
// suspended keeps its own desired state.
func clearClusterDrainStop(ctx context.Context, client versioned.Interface, task *rlarkv1alpha1.Task) error {
	if !task.DeletionTimestamp.IsZero() || !clusterDrainStopped(task) {
		return nil
	}
	drainSuspended := task.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] == "true"
	annotations := map[string]any{rlarkv1alpha1.ClusterStoppedAnnotation: nil}
	if drainSuspended {
		annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] = nil
	}
	patch := map[string]any{"metadata": map[string]any{"annotations": annotations}}
	if drainSuspended {
		patch["spec"] = map[string]any{"suspended": false}
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("build cluster drain marker patch for Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	if _, err := client.RlinfV1alpha1().Tasks(task.Namespace).Patch(ctx, task.Name, types.MergePatchType, body, metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("clear cluster drain marker from Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	return nil
}

// clearClusterDrainStops releases every Task in the agent namespace that a
// cancelled drain stopped.
func clearClusterDrainStops(ctx context.Context, client versioned.Interface, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("cluster agent namespace is empty")
	}
	tasks, err := client.RlinfV1alpha1().Tasks(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Tasks in agent namespace: %w", err)
	}
	for i := range tasks.Items {
		if err := clearClusterDrainStop(ctx, client, &tasks.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

func clusterDrainStoppedTasksForJob(ctx context.Context, client versioned.Interface, job *rlarkv1alpha1.Job) ([]rlarkv1alpha1.Task, error) {
	tasks, err := client.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{taskJobLabel: job.Name}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list Tasks for Job %q: %w", job.Name, err)
	}
	stopped := make([]rlarkv1alpha1.Task, 0)
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !metav1.IsControlledBy(task, job) || !clusterDrainStopped(task) {
			continue
		}
		stopped = append(stopped, *task)
	}
	return stopped, nil
}

// jobHasClusterDrainStops reports whether any Task owned by job still carries a
// cluster drain marker.
func (g *Gateway) jobHasClusterDrainStops(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	if g.kubeClient == nil {
		return false, nil
	}
	tasks, err := clusterDrainStoppedTasksForJob(ctx, g.kubeClient, job)
	if err != nil {
		return false, err
	}
	return len(tasks) > 0, nil
}

// clearClusterDrainStopsForJob releases the Tasks owned by job that a cluster
// drain stopped. Starting the Job is the explicit resume action for a drain, so
// the markers must be gone before the Job controller can dispatch Tasks again.
func (g *Gateway) clearClusterDrainStopsForJob(ctx context.Context, job *rlarkv1alpha1.Job) error {
	if g.kubeClient == nil {
		return nil
	}
	tasks, err := clusterDrainStoppedTasksForJob(ctx, g.kubeClient, job)
	if err != nil {
		return err
	}
	for i := range tasks {
		if err := clearClusterDrainStop(ctx, g.kubeClient, &tasks[i]); err != nil {
			return err
		}
	}
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
	runningJobs, err := g.clusterRunningJobs(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	detail := ClusterDetail{
		ClusterInfo: buildClusterInfo(*cluster, clusterNodes, runningJobs[cluster.Spec.AgentNamespace]),
		Nodes:       clusterNodes,
	}
	c.JSON(http.StatusOK, gin.H{"data": detail, "success": true})
}
