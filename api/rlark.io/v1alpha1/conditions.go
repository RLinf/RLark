package v1alpha1

// 本文件集中定义 RLark 各资源 Status.Conditions 共用的 Condition Type 常量。
//
// 约定：
//   - Conditions 使用 metav1.Condition，并以 Type 作为列表主键
//     （`+listType=map` / `+listMapKey=type`），同一 Type 在列表中至多出现一次。
//   - 写入方必须通过 apimeta.SetStatusCondition 更新，以便正确维护
//     LastTransitionTime 语义；撤回一个 Condition 必须通过
//     apimeta.RemoveStatusCondition，不得手工拼接切片。两者都应经由
//     apps/rlark/pkg/reconcile.ApplyConditionMutations 调用，使控制面与数据面
//     走同一条写入通道。
//   - Spec 驱动的 Condition 必须填写 ObservedGeneration（取写入时的
//     metadata.generation）。metav1.Condition 自带的 per-condition
//     ObservedGeneration 是控制器契约而非 API Server 保证：消费者需要据此
//     判断该 Condition 是否仍对应当前 Spec。
//   - 观测驱动的 Condition 不得填写 ObservedGeneration。填写一个并非针对该
//     Generation 计算出来的值，会让消费者误以为结论已对齐当前 Spec。
//   - 同一个 Status 可能有多个写入方。此时每个 Condition Type 必须有且只有一个
//     归属写入方，不同写入方只能写互不相交的字段集，并且都必须使用
//     client.MergeFrom（不带乐观锁）做 Status Patch，使并发写入可被确定性合并、
//     而不依赖写入先后顺序。字段归属必须在对应资源的 Status 字段注释中写明。
//   - Reason 必须是集中定义的 CamelCase 稳定短标识，供调度器、Gateway 与 UI
//     进行机读判断；Message 仅用于展示细节，不得作为业务逻辑输入。
//   - 新增 Condition Type 必须在此声明，并同步更新中英文文档，避免各实现包
//     重复定义同名字符串而语义漂移。
const (
	// ConditionTypeValidated 表示控制器对资源 Spec 的校验结论。
	// Status=False 表示 Spec 非法且不会被继续调谐，Reason 给出具体原因。
	// 该 Condition 由 Spec 驱动，写入方必须填写 ObservedGeneration。
	// 当前写入方：控制面 Job / Workflow / Domain 控制器。
	ConditionTypeValidated = "Validated"

	// ConditionTypeProgressing 表示资源正在向期望状态推进的中间过程。
	// Status=True 表示仍在推进中，Reason 给出当前推进的具体动作；
	// 推进结束（成功或失败）后写入方必须撤回该 Condition，而不是把它置为 False，
	// 以免“未在推进”与“推进失败”这两种含义混在同一个取值里。
	// 该 Condition 由 Spec 驱动，写入方必须填写 ObservedGeneration。
	// 当前写入方：控制面 Job / Workflow 控制器。
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeEvicted reports whether every Worker of a Job has been
	// stopped by an eviction (a Node or Cluster Drain) while the Job itself is
	// not suspended. It is an observation of the Task snapshot, not a verdict on
	// a specific Spec revision, so its writer must not set ObservedGeneration.
	// 该 Condition 由控制面 Job 控制器写入。
	ConditionTypeEvicted = "Evicted"

	// ConditionTypeAgentConnected reports whether the node Agent heartbeat lease is current.
	ConditionTypeAgentConnected = "AgentConnected"

	// ConditionTypeReady reports whether the node is available for its intended use.
	ConditionTypeReady = "Ready"

	// ConditionTypeRuntimeReady reports whether the selected runtime is available.
	ConditionTypeRuntimeReady = "RuntimeReady"

	// ConditionTypeNetworkReady reports whether required node networking is available.
	ConditionTypeNetworkReady = "NetworkReady"

	// ConditionTypeStorageReady reports whether required node storage is available.
	ConditionTypeStorageReady = "StorageReady"

	// ConditionTypeDeviceReady reports whether required node devices are available.
	ConditionTypeDeviceReady = "DeviceReady"

	// ConditionTypeResourcePressure reports whether the node is under scheduler-relevant pressure.
	ConditionTypeResourcePressure = "ResourcePressure"

	// ConditionTypeDraining reports whether an ordered node drain is requested or blocked.
	ConditionTypeDraining = "Draining"

	// ConditionTypeDegraded reports non-fatal resource pressure or capability failures.
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeSchedulable reports whether new workloads may be placed on the
	// Node. It is the single placement decision: the Node Controller derives it
	// from spec.unschedulable, AgentConnected, Ready, KubernetesCordoned, and
	// ResourcePressure, and every consumer (Job scheduler, Gateway, console) reads
	// it instead of re-deriving placement from the individual conditions.
	ConditionTypeSchedulable = "Schedulable"

	// ConditionTypeKubernetesCordoned reports whether the local Kubernetes Node
	// object carries spec.unschedulable=true. RLark does not manage that field, so
	// True means the node was cordoned outside RLark and the Kubernetes scheduler
	// excludes it for every Pod, not only RLark workloads. An explicit True also
	// makes Schedulable False so RLark does not place Tasks the Kubernetes
	// scheduler would refuse.
	ConditionTypeKubernetesCordoned = "KubernetesCordoned"
)

// Stable reasons for Node conditions. These identifiers are part of the machine-readable API.
const (
	ConditionReasonLeaseValid             = "LeaseValid"
	ConditionReasonLeaseExpired           = "LeaseExpired"
	ConditionReasonHeartbeatNeverReceived = "HeartbeatNeverReceived"

	ConditionReasonKubernetesNodeReady    = "KubernetesNodeReady"
	ConditionReasonKubernetesNodeNotReady = "KubernetesNodeNotReady"
	ConditionReasonKubernetesNodeUnknown  = "KubernetesNodeUnknown"
	// ConditionReasonKubernetesNodeReadyUnobserved means the Agent has not reported
	// a Kubernetes Node Ready condition. The aggregated Ready condition is False:
	// absent evidence of readiness must not be treated as ready.
	ConditionReasonKubernetesNodeReadyUnobserved = "KubernetesNodeReadyUnobserved"

	ConditionReasonRuntimeAvailable   = "RuntimeAvailable"
	ConditionReasonRuntimeUnavailable = "RuntimeUnavailable"
	ConditionReasonNetworkAvailable   = "NetworkAvailable"
	ConditionReasonNetworkUnavailable = "NetworkUnavailable"
	ConditionReasonStorageAvailable   = "StorageAvailable"
	ConditionReasonStorageUnavailable = "StorageUnavailable"
	ConditionReasonDeviceAvailable    = "DeviceAvailable"
	ConditionReasonDeviceUnavailable  = "DeviceUnavailable"

	ConditionReasonResourcePressureDetected = "ResourcePressureDetected"
	ConditionReasonNoResourcePressure       = "NoResourcePressure"
	ConditionReasonResourcePressureUnknown  = "ResourcePressureUnknown"

	ConditionReasonDrainRequested = "DrainRequested"
	ConditionReasonDrainBlocked   = "DrainBlocked"
	ConditionReasonDrainComplete  = "DrainComplete"
	ConditionReasonNoDrainRequest = "NoDrainRequest"

	// ConditionReasonSchedulable means new placement is currently allowed.
	ConditionReasonSchedulable = "Schedulable"
	// ConditionReasonUnschedulable means spec.unschedulable durably blocks new placement.
	ConditionReasonUnschedulable = "Unschedulable"
	// ConditionReasonNotSchedulable means a prerequisite condition is missing or
	// unknown, so placement cannot be allowed.
	ConditionReasonNotSchedulable = "NotSchedulable"

	// ConditionReasonKubernetesCordoned means the local Kubernetes Node is cordoned
	// (spec.unschedulable=true).
	ConditionReasonKubernetesCordoned = "KubernetesCordoned"
	// ConditionReasonKubernetesNotCordoned means the local Kubernetes Node is not cordoned.
	ConditionReasonKubernetesNotCordoned = "KubernetesNotCordoned"
	// ConditionReasonKubernetesCordonUnobserved means the Agent has not reported the
	// local Kubernetes Node's cordon state, so it is unknown.
	ConditionReasonKubernetesCordonUnobserved = "KubernetesCordonUnobserved"
)
