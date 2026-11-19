package v1alpha1

// 本文件集中定义 RLark 各资源 Status.Conditions 共用的 Condition Type 常量。
//
// 约定：
//   - Conditions 使用 metav1.Condition，并以 Type 作为列表主键
//     （`+listType=map` / `+listMapKey=type`），同一 Type 在列表中至多出现一次。
//   - 写入方必须通过 apimeta.SetStatusCondition 更新，以便正确维护
//     LastTransitionTime 语义。
//   - Spec 驱动的 Condition 必须填写 ObservedGeneration（取写入时的
//     metadata.generation）。metav1.Condition 自带的 per-condition
//     ObservedGeneration 是控制器契约而非 API Server 保证：消费者需要据此
//     判断该 Condition 是否仍对应当前 Spec。
//   - Reason 为资源与场景特有，不在此集中定义；它必须是
//     CamelCase、取值稳定、可被程序消费的短标识。
//   - 新增 Condition Type 必须在此声明，并同步更新中英文文档，避免各实现包
//     重复定义同名字符串而语义漂移。
const (
	// ConditionTypeValidated 表示控制器对资源 Spec 的校验结论。
	// Status=False 表示 Spec 非法且不会被继续调谐，Reason 给出具体原因。
	// 该 Condition 由 Spec 驱动，写入方必须填写 ObservedGeneration。
	// 当前写入方：控制面 Job / Workflow / Domain 控制器。
	ConditionTypeValidated = "Validated"

	// ConditionTypeProgressing 表示资源正在向期望状态推进的中间过程。
	// Status=True 表示仍在推进中，Reason 给出当前推进的具体动作。
	// 当前写入方：数据面 Task 控制器（工作负载类型迁移）。
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeReady 表示资源当前是否处于可用状态，是 Phase 的结构化补充：
	// Phase 给出聚合阶段，Ready 给出“是否可用”这一布尔结论及其机器可读原因。
	// 该 Condition 由观测驱动（而非 Spec 驱动），因此不保证填写 ObservedGeneration。
	// 当前写入方：数据面 Node 控制器、数据面 Addon 控制器。
	ConditionTypeReady = "Ready"
)
