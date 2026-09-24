package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Constants used by the package.
const (
	AgentVersion      = "0.1.0"
	HeartbeatInterval = 30 * time.Second

	nodeCategoryLabel       = "rlark.io/node-category"
	nodeCategoryLabelPrefix = "rlark.io/node-category-"
	nodeCityAnnotation      = "rlark.io/city"
	nodeGPUModelAnnotation  = "rlark.io/gpu-model"
	nodeDeviceAnnotation    = "rlark.io/device-model"
	nodeLegacyModel         = "rlark.io/model"
)

func isManagementOwnedNodeLabel(key string) bool {
	return key == nodeCategoryLabel || strings.HasPrefix(key, nodeCategoryLabelPrefix)
}

func isManagementOwnedNodeAnnotation(key string) bool {
	switch key {
	case nodeCityAnnotation, nodeGPUModelAnnotation, nodeDeviceAnnotation, nodeLegacyModel:
		return true
	default:
		return false
	}
}

// mergeManagementMetadata keeps business metadata authored on the management
// plane while refreshing metadata discovered from the local Kubernetes Node.
// City and hardware models intentionally live only on the management Node CR.
func mergeManagementMetadata(management, discovered map[string]string, managementOwned func(string) bool) map[string]string {
	merged := make(map[string]string, len(discovered)+len(management))
	for key, value := range discovered {
		if !managementOwned(key) {
			merged[key] = value
		}
	}
	for key, value := range management {
		if managementOwned(key) {
			merged[key] = value
		}
	}
	return merged
}

func mergeManagementAnnotations(management, discovered map[string]string) map[string]string {
	merged := make(map[string]string, len(management)+len(discovered))
	for key, value := range management {
		if !strings.HasPrefix(key, "rlark.io/") || isManagementOwnedNodeAnnotation(key) {
			merged[key] = value
		}
	}
	for key, value := range discovered {
		if !isManagementOwnedNodeAnnotation(key) {
			merged[key] = value
		}
	}
	return merged
}

// pushNodeReconciler watches local K8s Nodes and reports their info to management Node CRs.
type pushNodeReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pushNodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)

	var k8sNode corev1.Node
	if err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: req.Name}, &k8sNode); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to get local K8s Node")
			return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
		}
		logger.Info("local K8s Node not found, waiting")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, nil
	}

	var podList corev1.PodList
	localCachedClient := r.c.LocalKubeCachedClient
	if localCachedClient == nil {
		localCachedClient = r.c.LocalKubeClient
	}
	if err := localCachedClient.List(ctx, &podList, client.MatchingFields{podNodeNameField: k8sNode.Name}); err != nil {
		logger.Error(err, "failed to list local Pods for node resource usage")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	desiredNode := r.buildRLarkNodeFromK8sNode(&k8sNode, podList.Items)
	storage, err := r.nodeStorageStatus(ctx, k8sNode.Name)
	storageCollected := err == nil && storage != nil
	if err != nil {
		logger.Error(err, "failed to get kubelet storage stats")
	} else {
		desiredNode.Status.Storage = storage
	}
	return r.updateManagementNode(ctx, logger, desiredNode, storageCollected)
}

func (r *pushNodeReconciler) buildRLarkNodeFromK8sNode(k8sNode *corev1.Node, pods []corev1.Pod) *rlarkv1alpha1.Node {
	labels := make(map[string]string)
	for k, v := range k8sNode.Labels {
		labels[k] = v
	}
	labels[rlarkv1alpha1.LabelClusterID] = r.c.ManagementNamespace
	labels[rlarkv1alpha1.LabelAgentType] = r.c.AgentType

	annotations := make(map[string]string)
	for k, v := range k8sNode.Annotations {
		if strings.HasPrefix(k, "rlark.io/") {
			annotations[k] = v
		}
	}

	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        k8sNode.Name,
			Namespace:   r.c.ManagementNamespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: rlarkv1alpha1.NodeSpec{
			AgentType:     rlarkv1alpha1.AgentType(r.c.AgentType),
			Unschedulable: k8sNode.Spec.Unschedulable,
		},
		Status: rlarkv1alpha1.NodeStatus{
			Phase:        r.getPhase(k8sNode),
			DiskPressure: diskPressure(k8sNode),
			Capacity:     k8sNode.Status.Capacity,
			Allocatable:  k8sNode.Status.Allocatable,
			Used:         requestedResourcesForNode(k8sNode.Name, pods),
			Addresses:    k8sNode.Status.Addresses,
			NodeInfo: rlarkv1alpha1.NodeInfo{
				Architecture:    k8sNode.Status.NodeInfo.Architecture,
				KernelVersion:   k8sNode.Status.NodeInfo.KernelVersion,
				OperatingSystem: k8sNode.Status.NodeInfo.OperatingSystem,
				AgentVersion:    AgentVersion,
			},
		},
	}
}

type nodeFSStats struct {
	CapacityBytes  *uint64 `json:"capacityBytes"`
	UsedBytes      *uint64 `json:"usedBytes"`
	AvailableBytes *uint64 `json:"availableBytes"`
}

type nodeStatsSummary struct {
	Node struct {
		FS      *nodeFSStats `json:"fs"`
		Runtime *struct {
			ImageFS *nodeFSStats `json:"imageFs"`
		} `json:"runtime"`
	} `json:"node"`
}

func (r *pushNodeReconciler) nodeStorageStatus(ctx context.Context, nodeName string) (*rlarkv1alpha1.NodeStorageStatus, error) {
	if r.c.LocalKubeHTTP == nil || r.c.LocalKubeAPIHost == "" {
		return nil, nil
	}
	endpoint, err := url.JoinPath(r.c.LocalKubeAPIHost, "api", "v1", "nodes", nodeName, "proxy", "stats", "summary")
	if err != nil {
		return nil, fmt.Errorf("build stats summary URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create stats summary request: %w", err)
	}
	resp, err := r.c.LocalKubeHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request stats summary: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request stats summary: %s", resp.Status)
	}
	var summary nodeStatsSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("decode stats summary: %w", err)
	}

	filesystems := []*nodeFSStats{summary.Node.FS}
	if summary.Node.Runtime != nil {
		filesystems = append(filesystems, summary.Node.Runtime.ImageFS)
	}

	storage := &rlarkv1alpha1.NodeStorageStatus{}
	seen := make(map[[2]uint64]struct{})
	collected := false
	for _, fs := range filesystems {
		if fs == nil || fs.CapacityBytes == nil || fs.AvailableBytes == nil {
			continue
		}

		// nodefs and imagefs can refer to the same underlying filesystem. In
		// that case their capacity and available bytes are identical, so only
		// count the filesystem once. A dedicated containerd image filesystem
		// is aggregated with nodefs instead.
		identity := [2]uint64{*fs.CapacityBytes, *fs.AvailableBytes}
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}

		capacity := int64(*fs.CapacityBytes)
		available := int64(*fs.AvailableBytes)
		used := capacity - available
		if fs.UsedBytes != nil {
			used = int64(*fs.UsedBytes)
		}
		storage.CapacityBytes += capacity
		storage.UsedBytes += used
		storage.AvailableBytes += available
		collected = true
	}
	if !collected {
		return nil, nil
	}
	return storage, nil
}

func diskPressure(node *corev1.Node) *bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type != corev1.NodeDiskPressure {
			continue
		}
		switch condition.Status {
		case corev1.ConditionTrue:
			pressure := true
			return &pressure
		case corev1.ConditionFalse:
			pressure := false
			return &pressure
		default:
			return nil
		}
	}
	return nil
}

// requestedResourcesForNode reports resources reserved by active Pods. Kubernetes
// does not expose actual node usage on the Node object; summed Pod requests are
// the stable, scheduler-relevant value available without a metrics-server.
func requestedResourcesForNode(nodeName string, pods []corev1.Pod) corev1.ResourceList {
	used := corev1.ResourceList{}
	for i := range pods {
		pod := &pods[i]
		if pod.Spec.NodeName != nodeName || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		podRequests := corev1.ResourceList{}
		for _, container := range pod.Spec.Containers {
			addResourceList(podRequests, container.Resources.Requests)
		}

		// Init containers run sequentially, so their contribution is the maximum
		// request for each resource rather than the sum of all init containers.
		initMaximum := corev1.ResourceList{}
		for _, container := range pod.Spec.InitContainers {
			for name, quantity := range container.Resources.Requests {
				current := initMaximum[name]
				if quantity.Cmp(current) > 0 {
					initMaximum[name] = quantity.DeepCopy()
				}
			}
		}
		for name, quantity := range initMaximum {
			current := podRequests[name]
			if quantity.Cmp(current) > 0 {
				podRequests[name] = quantity.DeepCopy()
			}
		}
		addResourceList(podRequests, pod.Spec.Overhead)
		addResourceList(used, podRequests)
	}
	return used
}

func addResourceList(target, values corev1.ResourceList) {
	for name, quantity := range values {
		current := target[name]
		current.Add(quantity)
		target[name] = current
	}
}

func (r *pushNodeReconciler) getPhase(k8sNode *corev1.Node) rlarkv1alpha1.NodePhase {
	if k8sNode == nil {
		return rlarkv1alpha1.NodeOffline
	}

	if k8sNode.Spec.Unschedulable {
		return rlarkv1alpha1.NodeOffline
	}

	for _, cond := range k8sNode.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				return rlarkv1alpha1.NodeOnline
			}
			return rlarkv1alpha1.NodeOffline
		}
	}

	return rlarkv1alpha1.NodeOffline
}

func (r *pushNodeReconciler) updateManagementNode(ctx context.Context, logger logr.Logger, desiredNode *rlarkv1alpha1.Node, storageCollected bool) (reconcile.Result, error) {
	var mgmtNode rlarkv1alpha1.Node
	err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: desiredNode.Name, Namespace: desiredNode.Namespace}, &mgmtNode)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get management Node")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	if err != nil {
		logger.Info("creating Node on management cluster")
		if err := r.c.ManagementClient.Create(ctx, desiredNode); err != nil {
			logger.Error(err, "failed to create management Node")
			return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
		}
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, nil
	}

	mgmtNode.Spec = desiredNode.Spec
	mgmtNode.Labels = mergeManagementMetadata(
		mgmtNode.Labels,
		desiredNode.Labels,
		isManagementOwnedNodeLabel,
	)
	mgmtNode.Annotations = mergeManagementAnnotations(mgmtNode.Annotations, desiredNode.Annotations)
	if err := r.c.ManagementClient.Update(ctx, &mgmtNode); err != nil {
		logger.Error(err, "failed to update management Node spec")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	// Preserve status fields owned by other node-agent components; the push
	// controller only refreshes K8s-derived Node fields (Capacity, Addresses,
	// DiskPressure condition, …) and must not clobber data that other
	// components are actively writing via the status subresource.
	//   - pullProgress: owned by the image puller (imagepull.Puller)
	//   - events: owned by the node events watcher (nodeevents.Watcher)
	desiredNode.Status.PullProgress = mgmtNode.Status.PullProgress
	desiredNode.Status.Events = mgmtNode.Status.Events
	if !storageCollected {
		// Keep the last successful nodefs/imagefs result when kubelet stats are
		// unavailable or incomplete during this reconciliation.
		desiredNode.Status.Storage = mgmtNode.Status.Storage
	}
	mgmtNode.Status = desiredNode.Status
	if err := r.c.ManagementClient.Status().Update(ctx, &mgmtNode); err != nil {
		logger.Error(err, "failed to update management Node status")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	logger.Info("management Node reported successfully")
	return reconcile.Result{RequeueAfter: HeartbeatInterval}, nil
}
