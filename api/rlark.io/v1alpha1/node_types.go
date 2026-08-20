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

type NodeCategory string

const (
	NodeCategoryCloud NodeCategory = "cloud" // 云算力节点
	NodeCategoryEdge  NodeCategory = "edge"  // 端算力节点
	NodeCategoryRobot NodeCategory = "robot" // 端真机节点
)

type NodePhase string

const (
	NodeOnline  NodePhase = "Online"
	NodeOffline NodePhase = "Offline"
)

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

type NodeStatus struct {
	Phase        NodePhase            `json:"phase,omitempty"`
	Reason       string               `json:"reason,omitempty"`
	NodeInfo     NodeInfo             `json:"nodeInfo,omitempty"`
	Addresses    []corev1.NodeAddress `json:"addresses,omitempty"`
	DiskPressure *bool                `json:"diskPressure,omitempty"`
	Allocatable  corev1.ResourceList  `json:"allocatable,omitempty"` // 需要预留系统组件 agent
	Capacity     corev1.ResourceList  `json:"capacity,omitempty"`
	Used         corev1.ResourceList  `json:"used,omitempty"`
	PullProgress []PullProgress       `json:"pullProgress,omitempty"`
	// Events 由数据面 node-agent 上报节点相关 Kubernetes Event（如 DiskPressure
	// 等 Warning 事件及镜像拉取/调度相关事件）。控制面 Task reconciler 在 Task
	// 处于 Pending 期间聚合各节点事件到 Task.status.events，供前端展示。
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
