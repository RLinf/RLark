package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	LabelClusterID    = "rlark.io/cluster-id"
	LabelAgentType    = "rlark.io/agent-type"
	LabelNodeCategory = "rlark.io/node-category"
)

// NodeCategory 是节点形态分类。管理面把分类写在 Node CR 的 Annotation 上
// （LabelNodeCategory 及其 "rlark.io/node-category-<category>" 布尔形式），
// 旧版本的 Label 形式仅作兼容读取。它不是任何资源字段的类型，因此无法在
// CRD Schema 层做枚举校验；取值由 Gateway 读取时解析并过滤。
type NodeCategory string

const (
	NodeCategoryCloud NodeCategory = "cloud" // 云算力节点
	NodeCategoryEdge  NodeCategory = "edge"  // 端算力节点
	NodeCategoryRobot NodeCategory = "robot" // 端真机节点
)

// NodePhase 是由 Node Controller 计算的节点生命周期摘要。它不表示连接、健康和
// 可调度性的单一真相；使用 Conditions、Spec.Unschedulable 和 Agent 观测字段分别判断。
// +kubebuilder:validation:Enum=Registering;Online;Degraded;Offline;Cordoned;Draining
// +kubebuilder:validation:Optional
type NodePhase string

const (
	NodeRegistering NodePhase = "Registering"
	NodeOnline      NodePhase = "Online"
	NodeDegraded    NodePhase = "Degraded"
	NodeOffline     NodePhase = "Offline"
	NodeCordoned    NodePhase = "Cordoned"
	NodeDraining    NodePhase = "Draining"
	// NodePending and NodeReady are retained as compatibility aliases. New
	// consumers should use Conditions instead of treating Phase as a scheduler
	// decision input.
	NodePending = NodeRegistering
	NodeReady   = NodeOnline
	// NodeMaintenance is a legacy alias.
	NodeMaintenance = NodeCordoned
)

// +kubebuilder:validation:Enum=Kubernetes;Docker;Raw
type AgentType string

const (
	AgentTypeKubernetes AgentType = "Kubernetes"
	AgentTypeDocker     AgentType = "Docker"
	AgentTypeRaw        AgentType = "Raw"
)

// NodeOperationType is the one-shot administrative action requested for a Node.
// The enum is closed so that an unsupported value is rejected by the API server
// instead of being silently skipped by every consumer. Each listed value has
// exactly one consuming owner: Drain is projected onto Tasks by the Node Agent,
// EnterMaintenance is realized by the control-plane Node reconciler.
// +kubebuilder:validation:Enum=Drain;EnterMaintenance
type NodeOperationType string

const (
	NodeOperationDrain            NodeOperationType = "Drain"
	NodeOperationEnterMaintenance NodeOperationType = "EnterMaintenance"
)

// NodeOperationRequest identifies one administrative operation. Administrators
// create a new ID for each new action; controllers must treat an already-observed
// ID idempotently rather than inferring a new action from a boolean toggle.
type NodeOperationRequest struct {
	// ID is a stable operation identifier supplied by the requester. It is
	// required and must be non-empty: an operation without an identity cannot be
	// tracked, reported, or retried idempotently, so it is rejected at the API
	// boundary rather than dropped by each consumer.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`
	// Type selects the requested one-shot operation.
	Type NodeOperationType `json:"type"`
}

// NodeIdentity is immutable management-owned identity for a registered Node.
type NodeIdentity struct {
	// ID is the stable RLark business identity of the Node.
	ID string `json:"id,omitempty"`
	// Domain is the owning Domain identifier.
	Domain string `json:"domain,omitempty"`
	// Tenant is the owning tenant identifier, independent of Kubernetes namespace placement.
	Tenant string `json:"tenant,omitempty"`
	// Owner is the management principal or controller that owns the Node.
	Owner string `json:"owner,omitempty"`
}

// NodeRuntimeReference selects the runtime that realizes workloads on the Node.
type NodeRuntimeReference struct {
	// Type identifies the configured runtime implementation.
	Type AgentType `json:"type,omitempty"`
	// Name identifies the runtime instance or provider configuration.
	Name string `json:"name,omitempty"`
}

// NodeSpec contains only management-owned durable desired state. Agents must not
// write this section; administrative APIs own Unschedulable and Operations.
type NodeSpec struct {
	// AgentType is a reserved compatibility field for the Node access form.
	//
	// Deprecated: use Runtime.Type for new integrations. This field must remain in
	// the API and generated CRD while non-Kubernetes runtime implementations are
	// incomplete, so existing resources and clients retain their declared intent.
	AgentType AgentType `json:"agentType,omitempty"`
	// Identity is set by the management API or control plane when the Node is registered.
	Identity NodeIdentity `json:"identity,omitempty"`
	// Runtime is the management-selected runtime reference for this Node.
	Runtime NodeRuntimeReference `json:"runtime,omitempty"`
	// Unschedulable durably blocks placement of new workloads without stopping existing ones.
	Unschedulable bool `json:"unschedulable,omitempty"`
	// Operations contains uniquely identified one-shot maintenance or drain requests.
	// +listType=map
	// +listMapKey=id
	Operations []NodeOperationRequest `json:"operations,omitempty"`
}

// NodeStorageStatus reports a bounded kubelet filesystem snapshot in bytes.
// It is monitoring data, not a scheduler capacity input.
type NodeStorageStatus struct {
	CapacityBytes  int64 `json:"capacityBytes,omitempty"`
	UsedBytes      int64 `json:"usedBytes,omitempty"`
	AvailableBytes int64 `json:"availableBytes,omitempty"`
}

// NodeRuntimeCapability describes the runtime and workload kinds the Agent can run.
type NodeRuntimeCapability struct {
	// Type identifies the runtime implementation.
	Type AgentType `json:"type,omitempty"`
	// Version is the runtime implementation version.
	Version string `json:"version,omitempty"`
	// WorkloadKinds lists the workload kinds supported by this runtime.
	WorkloadKinds []string `json:"workloadKinds,omitempty"`
}

// NodeOperationsCapability is reserved for operations whose support is observed locally.
type NodeOperationsCapability struct {
	Logs     bool `json:"logs,omitempty"`
	Exec     bool `json:"exec,omitempty"`
	Terminal bool `json:"terminal,omitempty"`
}

// NodeNetworkCapability describes Node-local networking features supported by the runtime.
type NodeNetworkCapability struct {
	CrossCluster    bool `json:"crossCluster,omitempty"`
	HostNetwork     bool `json:"hostNetwork,omitempty"`
	ServiceExposure bool `json:"serviceExposure,omitempty"`
}

// NodeStorageCapability is reserved for storage support observed locally on the Node.
type NodeStorageCapability struct {
	VolumeTypes    []string `json:"volumeTypes,omitempty"`
	StorageClasses []string `json:"storageClasses,omitempty"`
}

// NodeDeviceCapability describes Kubernetes extended-resource inventory and scheduling availability.
type NodeDeviceCapability struct {
	// Category is the Kubernetes extended resource name, for example "nvidia.com/gpu".
	Category string `json:"category"`
	// Count is the capacity reported by the Kubernetes Node.
	Count int32 `json:"count,omitempty"`
	// Available is allocatable capacity less than non-terminal Pod resource requests.
	Available int32 `json:"available,omitempty"`
}

// NodeCapabilitySnapshot is the Agent-observed, Node-local capability declaration.
// It contains only facts derived from the Kubernetes Node and bound Pod requests;
// transient health continues to be reported through readiness conditions.
type NodeCapabilitySnapshot struct {
	Runtime    NodeRuntimeCapability    `json:"runtime,omitempty"`
	Operations NodeOperationsCapability `json:"operations,omitempty"`
	Network    NodeNetworkCapability    `json:"network,omitempty"`
	Storage    NodeStorageCapability    `json:"storage,omitempty"`
	// +listType=map
	// +listMapKey=category
	Devices []NodeDeviceCapability `json:"devices,omitempty"`
}

// NodeWorkloadSummary is a bounded aggregate of active workloads observed by the Agent.
type NodeWorkloadSummary struct {
	Total    int32 `json:"total,omitempty"`
	Pending  int32 `json:"pending,omitempty"`
	Running  int32 `json:"running,omitempty"`
	Stopping int32 `json:"stopping,omitempty"`
	// RLarkManaged is the count of Pods on this Node that are managed by RLark
	// (identified by the RLark task-management annotations). Agent populates it
	// so that controllers can distinguish "Agent hasn't scanned the node yet"
	// from "the node genuinely has zero RLark-managed workloads", which matters
	// for deciding whether a Node Drain operation can complete.
	RLarkManaged int32 `json:"rlarkManaged,omitempty"`
}

// NodeDrainOperationStatus is the bounded Node Controller summary of one drain operation.
type NodeDrainOperationStatus struct {
	OperationID string `json:"operationID"`
	Waiting     int32  `json:"waiting,omitempty"`
	Blocked     int32  `json:"blocked,omitempty"`
	Completed   bool   `json:"completed,omitempty"`
}

// NodeStatus separates Agent-observed facts from Node Controller-derived lifecycle
// conclusions. Job and scheduler components consume this status read-only.
//
// NodeStatus has two writers, and each one writes a disjoint set of fields:
//
//   - The control-plane Node Controller owns Phase, Conditions, DrainOperations,
//     LastHeartbeatTime and LastStatusChangeTime. They are its conclusions about
//     the Agent heartbeat Lease and the facts reported by the data plane.
//   - The data-plane node-agent owns NodeInfo, Addresses, DiskPressure, Storage,
//     Capacity, Allocatable, Used, CapabilitySnapshot, ActiveWorkloads,
//     ObservedNodeLabels, PullProgress and Events. They are Node-local observations.
//
// Both writers Patch the status with client.MergeFrom (no optimistic lock), so
// concurrent writes merge deterministically and do not depend on write order.
// That only holds while the ownership above is respected: a new field must be
// assigned to exactly one writer and must never be written by both.
type NodeStatus struct {
	// Phase is a concise lifecycle summary calculated only by the Node Controller.
	Phase NodePhase `json:"phase,omitempty"`
	// Conditions are calculated only by the Node Controller from desired state and Agent facts.
	// They are observation-driven and therefore carry no per-condition
	// ObservedGeneration; use the status-level ObservedGeneration to tell whether
	// they already reflect the latest Spec.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// LastHeartbeatTime is the Agent heartbeat renewal the Node Controller read
	// from the coordination Lease. It is republished through the control-plane
	// clock so that it stays comparable across Nodes, and it only ever moves
	// forward. Agents must not write it.
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// LastStatusChangeTime is the latest time an Agent-observed or controller-derived state changed.
	LastStatusChangeTime *metav1.Time `json:"lastStatusChangeTime,omitempty"`
	// KubernetesNodeUID is the Agent-observed UID of the local Kubernetes Node object.
	// It identifies one Node incarnation and is used to correlate Node Drain requests.
	KubernetesNodeUID types.UID `json:"kubernetesNodeUID,omitempty"`
	// KubernetesReady is the raw Kubernetes Node Ready observation reported by the Agent.
	KubernetesReady *bool `json:"kubernetesReady,omitempty"`
	// KubernetesCordoned is the raw observation of the local Kubernetes Node's
	// spec.unschedulable reported by the Agent. RLark does not manage that field:
	// a cordoned Kubernetes Node conceals placement from every Pod, not only RLark
	// workloads, so the Agent reports the observation and the Node Controller
	// derives the KubernetesCordoned condition from it without changing RLark's own
	// placement decision. Absence means the Agent has not reported the value.
	KubernetesCordoned *bool `json:"kubernetesCordoned,omitempty"`
	// NodeInfo is Agent-reported operating system, architecture, Agent, and runtime version information.
	NodeInfo NodeInfo `json:"nodeInfo,omitempty"`
	// Addresses are Agent-observed node addresses.
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`
	// DiskPressure is the raw kubelet observation used by the Node Controller to derive ResourcePressure.
	DiskPressure *bool `json:"diskPressure,omitempty"`
	// Storage is a bounded monitoring snapshot and must not be used for placement or capacity decisions.
	Storage *NodeStorageStatus `json:"storage,omitempty"`
	// Allocatable is the Kubernetes Node's scheduler-declared capacity after system reservations.
	// Schedulers use this value, together with Used, for placement decisions.
	Allocatable corev1.ResourceList `json:"allocatable,omitempty"`
	// Capacity is the Kubernetes Node's physical resource inventory. It is an informational upper bound,
	// not the schedulable capacity.
	Capacity corev1.ResourceList `json:"capacity,omitempty"`
	// Used is the sum of non-terminal Pod resource requests already allocated for scheduling.
	// It is not actual runtime utilization.
	Used corev1.ResourceList `json:"used,omitempty"`
	// CapabilitySnapshot is the Agent-observed structured capability declaration.
	// Transient capability availability is reported by readiness Conditions.
	CapabilitySnapshot NodeCapabilitySnapshot `json:"capabilitySnapshot,omitempty"`
	// ActiveWorkloads is the Agent-observed bounded summary of current workloads.
	ActiveWorkloads NodeWorkloadSummary `json:"activeWorkloads,omitempty"`
	// ObservedNodeLabels is the Agent-observed label dictionary read from the
	// local Kubernetes Node object. It is the user-facing source of truth for
	// what labels are actually present on the data-plane Node. RLark currently
	// writes only RLark-owned system labels (rlark.io/*) to metadata.labels on
	// the management Node CR; other labels found there are not the data-plane
	// label set. Frontends and operator consoles should display
	// ObservedNodeLabels, not metadata.labels.
	ObservedNodeLabels map[string]string `json:"observedNodeLabels,omitempty"`
	// DrainOperations is the Node Controller's bounded summary of requested drain operations.
	// +listType=map
	// +listMapKey=operationID
	DrainOperations []NodeDrainOperationStatus `json:"drainOperations,omitempty"`
	// PullProgress is node-agent-reported progress for in-flight image pulls.
	PullProgress []PullProgress `json:"pullProgress,omitempty"`
	// Events is a bounded node-agent-reported diagnostic event summary.
	Events []NodeEvent `json:"events,omitempty"`
}

// PullProgress captures the progress of an in-flight image pull on a node.
type PullProgress struct {
	Image      string  `json:"image"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Speed      float64 `json:"speed"`
	Status     string  `json:"status"`
	Message    string  `json:"message,omitempty"`
}

// NodeEvent represents a Kubernetes Event observed on a node that is relevant
// for surfacing to operators (e.g. DiskPressure warnings, FailedScheduling,
// image pull failures). The node-agent collects Warning events plus a small
// set of Normal scheduling/pulling events and writes them to Node.status.events.
type NodeEvent struct {
	Type       string      `json:"type"`   // Warning / Normal
	Reason     string      `json:"reason"` // DiskPressure, FailedScheduling, Pulling, etc.
	Message    string      `json:"message,omitempty"`
	LastTime   metav1.Time `json:"lastTime,omitempty"` // 最近一次发生时间
	Count      int32       `json:"count,omitempty"`
	Source     string      `json:"source,omitempty"`     // 事件来源组件，如 kubelet
	ObjectKind string      `json:"objectKind,omitempty"` // 涉及对象类型：Node / Pod
	ObjectName string      `json:"objectName,omitempty"` // 涉及对象名称
}

type NodeInfo struct {
	Architecture    string `json:"architecture,omitempty"`
	KernelVersion   string `json:"kernelVersion,omitempty"`
	AgentVersion    string `json:"agentVersion,omitempty"`
	RuntimeVersion  string `json:"runtimeVersion,omitempty"`
	OperatingSystem string `json:"operatingSystem,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nodes,scope=Namespaced,shortName=rlnode
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.spec.runtime.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Unschedulable",type=boolean,JSONPath=`.spec.unschedulable`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}
