package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TaskRole 描述 Task 在业务编排中的角色，用于前端分组与展示。
// 该字段当前不在 CRD 层做枚举校验：它可能承载上层编排框架的自定义角色名，
// 收窄取值会阻止合法的既有数据写入。请勿据此推断取值的封闭集合。
type TaskRole string

// TaskPlacementSpec describes scheduling constraints for the workload.
type TaskPlacementSpec struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// RuntimeKind 表示负责执行 Task 工作负载的 Runtime 类型。
// CRD 有意不限制该字符串的枚举值，使 Docker、Raw 等 Runtime 可以在不修改
// Task Schema 的情况下扩展。Agent 必须在产生任何外部副作用前，通过已注册的
// Provider 和能力校验拒绝未知或尚未支持的 Runtime 类型。
type RuntimeKind string

// TaskRuntimeSpec selects the runtime implementation and instance that lowers
// the common Pod template. Name is a stable runtime instance identifier; it is
// not a display name or proof of ownership.
type TaskRuntimeSpec struct {
	Kind RuntimeKind `json:"kind"`
	Name string      `json:"name,omitempty"`
}

// WorkloadKind identifies the workload semantics independently of its Runtime
// carrier. The CRD deliberately keeps this identifier open; registered
// Adapters must reject unsupported kinds before producing external effects.
type WorkloadKind string

// TaskWorkloadCarrier selects the Runtime-specific carrier used to realize a
// workload. Type and Version identify a carrier contract; Parameters remain
// opaque until the selected Adapter or Runtime implementation strictly decodes
// them. Carrier identifiers are open strings, not CRD enums.
type TaskWorkloadCarrier struct {
	Type       string               `json:"type"`
	Version    string               `json:"version"`
	Parameters apiextensionsv1.JSON `json:"parameters,omitempty"`
}

// TaskEnvironment 表示 Task 请求的一种可扩展环境能力。
// Type 是带所有者作用域的稳定类型标识，Version 表示该类型的参数版本。
// Parameters 必须由对应的已注册 Handler 严格解码和校验；未知类型、未知版本或
// 非法参数必须在产生任何外部副作用前被拒绝。
type TaskEnvironment struct {
	Type       string               `json:"type"`
	Version    string               `json:"version"`
	Parameters apiextensionsv1.JSON `json:"parameters,omitempty"`
}

// TaskWorkloadSpec is the common workload declaration for every runtime.
// Kubernetes applies the Pod template natively. Future Docker and Raw providers
// lower the subset they support and reject unsupported fields before producing
// external effects.
type TaskWorkloadSpec struct {
	Kind       WorkloadKind            `json:"kind,omitempty"`
	Version    string                  `json:"version"`
	Parameters apiextensionsv1.JSON    `json:"parameters,omitempty"`
	Carrier    TaskWorkloadCarrier     `json:"carrier"`
	Template   *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// TaskPhase 是单个 Task 的生命周期阶段，由数据面 Agent 观测本地工作负载后上报。
// 取值集合与 JobPhase 对齐，Task 没有独立的 Stopping 中间态。
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Stopped
type TaskPhase string

const (
	TaskPhasePending   TaskPhase = "Pending"
	TaskPhaseRunning   TaskPhase = "Running"
	TaskPhaseSucceeded TaskPhase = "Succeeded"
	TaskPhaseFailed    TaskPhase = "Failed"
	TaskPhaseStopped   TaskPhase = "Stopped"
)

type TaskSpec struct {
	Role           TaskRole          `json:"role"`
	DownstreamName string            `json:"downstreamName,omitempty"`
	Suspended      bool              `json:"suspended,omitempty"`
	Placement      TaskPlacementSpec `json:"placement"`
	Runtime        TaskRuntimeSpec   `json:"runtime"`
	Workload       TaskWorkloadSpec  `json:"workload"`
	Environments   []TaskEnvironment `json:"environments,omitempty"`
	Tags           []JobTag          `json:"tags,omitempty"`
}

type TaskStatus struct {
	Phase TaskPhase `json:"phase,omitempty"`
	// ObservedGeneration 是数据面 Agent 最近一次完成本地工作负载调谐时所观测到的
	// metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取
	// phase / message / observedNodes 之前，应比较本字段与 metadata.generation
	// 以识别过期状态。
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ObservedNodes 是当前承载该 Task 工作负载的节点名，由数据面 Agent 排序去重后上报。
	ObservedNodes []string `json:"observedNodes,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	StartTime        *metav1.Time       `json:"startTime,omitempty"`
	CompletionTime   *metav1.Time       `json:"completionTime,omitempty"`
	Message          string             `json:"message,omitempty"`
	RetryCount       int32              `json:"retryCount,omitempty"`
	TensorBoardProxy string             `json:"tensorBoardProxy,omitempty"` // 由 gateway 计算填入，指向对应 Pod 6006 端口的代理相对地址
	// PullProgress 由数据面节点上的 image pull monitor 上报，仅在 Pod 处于
	// ContainerCreating（尚未 Running）期间由 cluster-agent 聚合写入。Pod 进入
	// Running 后被清空。供前端展示镜像拉取进度/速度。
	PullProgress []PullProgress `json:"pullProgress,omitempty"`
	// Events 在 Task 处于 Pending 期间由控制面 Task reconciler 从各节点
	// Node.status.events 聚合而来，包含 DiskPressure 等 Warning 事件及镜像
	// 拉取/调度相关事件。Task 离开 Pending 后被清空。
	Events []NodeEvent `json:"events,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=tasks,scope=Namespaced,shortName=task
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=`.spec.runtime.kind`
// +kubebuilder:printcolumn:name="Workload",type=string,JSONPath=`.spec.workload.kind`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TaskSpec   `json:"spec,omitempty"`
	Status            TaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}
