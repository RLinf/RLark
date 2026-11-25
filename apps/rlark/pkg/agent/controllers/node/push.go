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
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
)

// Constants used by the package.
const (
	AgentVersion      = "0.1.0"
	HeartbeatInterval = 30 * time.Second

	nodeCityAnnotation     = "rlark.io/city"
	nodeGPUModelAnnotation = "rlark.io/gpu-model"
	nodeDeviceAnnotation   = "rlark.io/device-model"
	nodeLegacyModel        = "rlark.io/model"
	nodeEnabledLabel       = "rlark.io/enabled"
	nodeUnschedulableLabel = "rlark.io/unschedulable"
)

// isManagementOwnedNodeAnnotation reports whether a Node annotation is authored
// on the management plane. Categories live here so an administrator decision is
// not overwritten by annotations found on the data-plane Kubernetes Node.
func isManagementOwnedNodeAnnotation(key string) bool {
	switch key {
	case nodeCityAnnotation, nodeGPUModelAnnotation, nodeDeviceAnnotation, nodeLegacyModel,
		rlarkv1alpha1.LabelNodeCategory,
		rlarkv1alpha1.LabelNodeCategory + "-" + string(rlarkv1alpha1.NodeCategoryCloud),
		rlarkv1alpha1.LabelNodeCategory + "-" + string(rlarkv1alpha1.NodeCategoryEdge),
		rlarkv1alpha1.LabelNodeCategory + "-" + string(rlarkv1alpha1.NodeCategoryRobot):
		return true
	default:
		return false
	}
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

	if k8sNode.Labels[nodeEnabledLabel] != "true" {
		logger.Info("skipping unmanaged local Kubernetes Node")
		return reconcile.Result{}, nil
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
	// ObservedNodeLabels is the full label set the agent reads from the local
	// Kubernetes Node. This is the authoritative source of truth for what
	// labels are actually on the data-plane node. metadata.labels on the
	// management Node CR only receives RLark system labels from this
	// reconciler; Gateway label edits are relayed directly to the data-plane
	// Kubernetes Node and are not written here.
	observedLabels := make(map[string]string, len(k8sNode.Labels)+1)
	systemLabels := make(map[string]string)
	for k, v := range k8sNode.Labels {
		observedLabels[k] = v
		if strings.HasPrefix(k, "rlark.io/") {
			// rlark.io/ 系统标签仍同步到 metadata.labels，因为 controller-manager
			// 和其他控制面组件通过 label selector 观察节点状态（如 enabled）。
			systemLabels[k] = v
		}
	}
	systemLabels[rlarkv1alpha1.LabelAgentType] = r.c.AgentType

	annotations := make(map[string]string)
	for k, v := range k8sNode.Annotations {
		if strings.HasPrefix(k, "rlark.io/") {
			annotations[k] = v
		}
	}

	cordoned := k8sNode.Spec.Unschedulable
	requested := requestedResourcesForNode(k8sNode.Name, pods)
	return &rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        k8sNode.Name,
			Namespace:   r.c.ManagementNamespace,
			Labels:      systemLabels,
			Annotations: annotations,
		},
		Status: rlarkv1alpha1.NodeStatus{
			KubernetesNodeUID:  k8sNode.UID,
			KubernetesReady:    kubernetesReady(k8sNode),
			KubernetesCordoned: &cordoned,
			DiskPressure:       diskPressure(k8sNode),
			Capacity:           k8sNode.Status.Capacity,
			Allocatable:        k8sNode.Status.Allocatable,
			Used:               requested,
			CapabilitySnapshot: capabilitySnapshot(k8sNode, r.c.AgentType, requested),
			Addresses:          k8sNode.Status.Addresses,
			ObservedNodeLabels: observedLabels,
			NodeInfo: rlarkv1alpha1.NodeInfo{
				Architecture:    k8sNode.Status.NodeInfo.Architecture,
				KernelVersion:   k8sNode.Status.NodeInfo.KernelVersion,
				OperatingSystem: k8sNode.Status.NodeInfo.OperatingSystem,
				AgentVersion:    AgentVersion,
			},
			ActiveWorkloads: activeWorkloadSummary(pods),
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

func kubernetesReady(node *corev1.Node) *bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			ready := condition.Status == corev1.ConditionTrue
			return &ready
		}
	}
	return nil
}

func activeWorkloadSummary(pods []corev1.Pod) rlarkv1alpha1.NodeWorkloadSummary {
	summary := rlarkv1alpha1.NodeWorkloadSummary{}
	for i := range pods {
		switch pods[i].Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			continue
		case corev1.PodPending:
			summary.Pending++
		case corev1.PodRunning:
			summary.Running++
		default:
			summary.Stopping++
		}
		summary.Total++
		if isRLarkManagedPod(&pods[i]) {
			summary.RLarkManaged++
		}
	}
	return summary
}

// isRLarkManagedPod returns true when the Pod carries the RLark
// task-management annotations that identify it as belonging to an RLark Task.
func isRLarkManagedPod(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	_, hasName := pod.Annotations[kubernetesobjects.ManagementTaskNameAnnotation]
	_, hasUID := pod.Annotations[kubernetesobjects.ManagementTaskUIDAnnotation]
	return hasName && hasUID
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
// does not expose actual node utilization on the Node object; summed Pod requests
// are the stable, scheduler-relevant allocation value. High-frequency utilization
// belongs in a monitoring system and must not rewrite Node status.
func capabilitySnapshot(node *corev1.Node, agentType string, requested corev1.ResourceList) rlarkv1alpha1.NodeCapabilitySnapshot {
	return rlarkv1alpha1.NodeCapabilitySnapshot{
		Runtime: rlarkv1alpha1.NodeRuntimeCapability{
			Type:    rlarkv1alpha1.AgentType(agentType),
			Version: node.Status.NodeInfo.ContainerRuntimeVersion,
			// The Job scheduler compares this list with Task.spec.workload.kind, so
			// advertise the task workload contracts this Agent executes rather than
			// the Kubernetes carrier kind.
			WorkloadKinds: []string{taskapi.WorkloadTypeRay},
		},
		Network: rlarkv1alpha1.NodeNetworkCapability{
			HostNetwork: true,
		},
		Devices: nodeDeviceCapabilities(node.Status.Capacity, node.Status.Allocatable, requested),
	}
}

func nodeDeviceCapabilities(capacity, allocatable, requested corev1.ResourceList) []rlarkv1alpha1.NodeDeviceCapability {
	devices := make([]rlarkv1alpha1.NodeDeviceCapability, 0)
	for name, total := range capacity {
		if !isExtendedResource(name) {
			continue
		}
		available := allocatable[name]
		available.Sub(requested[name])
		if available.Sign() < 0 {
			available.Set(0)
		}
		devices = append(devices, rlarkv1alpha1.NodeDeviceCapability{
			Category:  string(name),
			Count:     int32(total.Value()),
			Available: int32(available.Value()),
		})
	}
	return devices
}

func isExtendedResource(name corev1.ResourceName) bool {
	return strings.Contains(string(name), "/") && !strings.HasPrefix(string(name), "kubernetes.io/")
}

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

	// Annotations still merge (only rlark.io/ system annotations flow from
	// k8s Node; management-owned annotations stay untouched). Labels: only
	// rlark.io/ system labels are pushed from data plane to metadata.labels.
	// Non-system labels (team.example/*, kubernetes.io/*) are observed through
	// status.ObservedNodeLabels instead. Gateway label edits are relayed
	// directly to the data-plane Kubernetes Node, so they are not written here.
	mgmtNode.Annotations = mergeManagementAnnotations(mgmtNode.Annotations, desiredNode.Annotations)
	mgmtNode.Labels = mergeSystemLabels(mgmtNode.Labels, desiredNode.Labels)
	if err := r.c.ManagementClient.Update(ctx, &mgmtNode); err != nil {
		logger.Error(err, "failed to update management Node spec")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	beforeStatus := mgmtNode.DeepCopy()
	mgmtNode.Status.KubernetesNodeUID = desiredNode.Status.KubernetesNodeUID
	// LastHeartbeatTime is deliberately not written here. Liveness is decided
	// by the control plane from the heartbeat Lease, read through a single
	// clock; a timestamp each agent stamps with its own clock would compete
	// with that decision and make skew look like liveness.
	mgmtNode.Status.KubernetesReady = desiredNode.Status.KubernetesReady
	mgmtNode.Status.KubernetesCordoned = desiredNode.Status.KubernetesCordoned
	mgmtNode.Status.NodeInfo = desiredNode.Status.NodeInfo
	mgmtNode.Status.Addresses = desiredNode.Status.Addresses
	mgmtNode.Status.DiskPressure = desiredNode.Status.DiskPressure
	mgmtNode.Status.Capacity = desiredNode.Status.Capacity
	mgmtNode.Status.Allocatable = desiredNode.Status.Allocatable
	mgmtNode.Status.Used = desiredNode.Status.Used
	mgmtNode.Status.CapabilitySnapshot = desiredNode.Status.CapabilitySnapshot
	mgmtNode.Status.ObservedNodeLabels = desiredNode.Status.ObservedNodeLabels
	mgmtNode.Status.ActiveWorkloads = desiredNode.Status.ActiveWorkloads
	if storageCollected {
		mgmtNode.Status.Storage = desiredNode.Status.Storage
	}
	if err := r.c.ManagementClient.Status().Patch(ctx, &mgmtNode, client.MergeFrom(beforeStatus)); err != nil {
		logger.Error(err, "patch management Node status")
		return reconcile.Result{RequeueAfter: HeartbeatInterval}, err
	}

	logger.Info("management Node reported successfully")
	return reconcile.Result{RequeueAfter: HeartbeatInterval}, nil
}

// mergeSystemLabels pushes rlark.io/ system labels from the data plane to the
// management CR while preserving any other labels already present on
// metadata.labels. RLark does not treat those preserved labels as the
// data-plane label set; status.ObservedNodeLabels is authoritative.
func mergeSystemLabels(managed, observed map[string]string) map[string]string {
	result := make(map[string]string, len(managed)+len(observed))
	for k, v := range managed {
		result[k] = v
	}
	for k, v := range observed {
		if strings.HasPrefix(k, "rlark.io/") {
			result[k] = v
		}
	}
	return result
}
