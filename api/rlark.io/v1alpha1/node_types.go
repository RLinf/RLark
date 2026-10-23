package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelClusterID    = "rlark.io/cluster-id"
	LabelAgentType    = "rlark.io/agent-type"
	LabelNodeCategory = "rlark.io/node-category"
)

// NodeCategory 是节点形态分类。它只作为 LabelNodeCategory 的取值使用，
// 不是任何资源字段的类型，因此无法在 CRD Schema 层做枚举校验；
// 取值由 Gateway 在读取 Label 时解析并过滤。
type NodeCategory string

const (
	NodeCategoryCloud NodeCategory = "cloud" // 云算力节点
	NodeCategoryEdge  NodeCategory = "edge"  // 端算力节点
	NodeCategoryRobot NodeCategory = "robot" // 端真机节点
)

// NodePhase 是节点连通性阶段，由数据面 node-agent 通过心跳探测后上报。
type NodePhase string

const (
	NodeOnline  NodePhase = "Online"
	NodeOffline NodePhase = "Offline"
)

// +kubebuilder:validation:Enum=Kubernetes;Docker;Raw
type AgentType string

// 目前支持三种 Agent
const (
	AgentTypeKubernetes AgentType = "Kubernetes"
	AgentTypeDocker     AgentType = "Docker"
	AgentTypeRaw        AgentType = "Raw"
)

type NodeSpec struct {
	AgentType     AgentType `json:"agentType,omitempty"` // 节点接入的形态
	Unschedulable bool      `json:"unschedulable,omitempty"`
}

// NodeStorageStatus reports kubelet filesystem usage in bytes.
type NodeStorageStatus struct {
	CapacityBytes  int64 `json:"capacityBytes,omitempty"`
	UsedBytes      int64 `json:"usedBytes,omitempty"`
	AvailableBytes int64 `json:"availableBytes,omitempty"`
}

type NodeStatus struct {
	// Phase 是节点连通性聚合阶段，由数据面 node-agent 上报。
	// +kubebuilder:validation:Enum=Online;Offline
	Phase NodePhase `json:"phase,omitempty"`
	// Reason 是 Phase 的机器可读原因（CamelCase），与 Conditions 中
	// ConditionTypeReady 的 Reason 同源，供列表视图直接展示。
	// 该字段与 Conditions 均由数据面 node-agent 独占写入。
	Reason string `json:"reason,omitempty"`
	// Conditions 记录节点的结构化状态结论，当前仅包含 ConditionTypeReady。
	// 它由观测（而非 Spec）驱动，因此不填写 ObservedGeneration。
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// LastHeartbeatTime 是控制面 Node Reconciler 从 Agent Lease 读取到的最近一次
	// 心跳续约时间；数据面不写入本字段。
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// LastStatusChangeTime 是 Phase 最近一次发生变化的时间，与 LastHeartbeatTime
	// 同由控制面 Node Reconciler 写入，用于区分「长时间保持同一状态」与「刚刚翻转」。
	LastStatusChangeTime *metav1.Time `json:"lastStatusChangeTime,omitempty"`
	// NodeInfo 是节点基础信息，由数据面 Agent 上报；控制面不写入本字段。
	NodeInfo NodeInfo `json:"nodeInfo,omitempty"`
	// Addresses 是节点地址列表（InternalIP / Hostname 等）。
	Addresses    []corev1.NodeAddress `json:"addresses,omitempty"`
	DiskPressure *bool                `json:"diskPressure,omitempty"`
	Storage      *NodeStorageStatus   `json:"storage,omitempty"`
	Allocatable  corev1.ResourceList  `json:"allocatable,omitempty"` // 需要预留系统组件 agent
	Capacity     corev1.ResourceList  `json:"capacity,omitempty"`
	Used         corev1.ResourceList  `json:"used,omitempty"`
	// PullProgress 是节点上正在进行的镜像拉取进度，由 node-agent 上报。
	PullProgress []PullProgress `json:"pullProgress,omitempty"`
	// Events 由数据面 node-agent 上报节点相关 Kubernetes Event（如 DiskPressure
	// 等 Warning 事件及镜像拉取/调度相关事件）。控制面 Task reconciler 在 Task
	// 处于 Pending 期间聚合各节点事件到 Task.status.events，供前端展示。
	// 条目数量由 node-agent 侧裁剪（最多保留 50 条）。
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
	OperatingSystem string `json:"operatingSystem,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nodes,scope=Namespaced,shortName=rlnode
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentType`
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
