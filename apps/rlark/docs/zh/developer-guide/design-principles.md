# 项目设计原则

本文定义开源 RLark 项目的目标设计方向，是贡献者和编码 Agent 在设计、实现与评审时应遵守的规范。现有代码只能说明项目历史，不能自动成为新代码的先例：仓库中同时存在成熟实现、兼容路径、原型以及早于本规范形成的旧模式。

当现有代码与本文不一致时：

1. 保留外部确实需要的兼容行为；
2. 不继续扩大旧模式；
3. 让新代码向本文原则收敛；
4. 无法在同一变更中完成迁移时，记录有意保留的例外。

## 1. 基于状态设计，而不是依赖事件历史

RLark 是分布式控制系统。Watch 通知、HTTP 请求和连接事件可能重复、合并、延迟、乱序或丢失。

- 事件只负责触发检查，不是事实来源。
- 作出决定前重新读取资源和依赖的当前状态。
- 不依赖某个 Controller 一定先于另一个执行。
- 进程重启后能够根据已有状态恢复协调。
- 正确性不能依赖事件可靠投递时，增加周期观察或修复机制。

Reconcile 通常应遵循以下结构：

```text
读取当前资源
  → 处理删除
  → 验证期望状态
  → 观察依赖和子资源
  → 计算差异
  → 幂等执行外部操作
  → Patch Status 和 Condition
  → 完成、重试或延迟 Requeue
```

## 2. 外部操作必须幂等且可恢复

创建、更新、删除、持久化和 Provider 操作都必须能够安全重复。外部操作成功但结果尚未记录时发生崩溃，重试不能产生重复资源或重复历史。

- 在需要时使用稳定的 Resource、Run 和 Operation ID。
- 创建前先观察；结果不确定时再次观察。
- 当目标状态就是不存在时，`NotFound` 应视为清理成功。
- Delete 请求被接受不等于清理已经完成。
- 依赖不可用时使用有上限并带抖动的退避，避免请求风暴。
- 区分无效输入、不支持能力、冲突、依赖不可用、操作进行中、外部失败和内部不变量错误。

永久错误应写入稳定 Condition，并等待相关输入变化。冲突应重新读取并重新计算，不能重复提交基于旧对象的更新。

## 3. 分离期望、观测、操作和历史

每种表达只承担一种职责：

| 表达 | 职责 |
| --- | --- |
| Spec | 用户或上层 Controller 拥有的长期期望 |
| Status | Controller 对当前 Generation 的观测 |
| Condition | 当前状态及可机读原因 |
| Operation | Start、Restart、Restore、Purge 等一次性动作 |
| Resource Event | 对用户有意义的状态变化 |
| Log | 组件诊断信息 |
| Audit Event | 谁对什么执行了什么操作以及结果 |

- 不把 Controller 运行结果写入 Spec。
- 真实概念是一次性操作时，不用长期切换的 Spec 开关代替 Operation。
- Phase 用于生命周期概览，Condition 用于详细、可机读的状态。
- Condition 使用稳定 Type 和 Reason、可读 Message，并在适用 Generation 语义时携带 `ObservedGeneration`。
- 不把无限增长的历史、日志、遥测流和大体量数据放入 Status。

`ObservedGeneration` 是 Controller 契约，不是 Kubernetes API Server 自动提供的强一致性保障。对于由 Spec 驱动的 Condition，Controller 只有在观察到当前 `metadata.generation` 并生成对应结果后，才能将 `ObservedGeneration` 设置为该 Generation。消费方发现二者不一致时，应将这类 Condition 视为旧状态。该行为需要通过 Controller 实现和测试保障；Schema 只保证字段存在，不能证明字段值真实正确。

是否按 Generation 判断新鲜度，应根据 Condition 的语义决定。`Validated`、`Progressing`、`Scheduled` 和工作负载就绪等 Condition 通常由 Spec 驱动，应遵循上述规则。Agent 连接或基础设施资源压力等主要描述外部事实的 Condition，在 Spec 变化后可能仍然有效；使用 `metav1.Condition` 表达时，仍应记录写入时所观察到的 Generation，但消费方不能假设所有外部健康事实都会因为 Generation 不一致而自动失效。

当布尔值或开关描述的是持续期望状态时，放在 Spec 中是合适的。例如 `enabled: false`、`suspended: true` 或 `unschedulable: true`，表示 Controller 应持续维持该状态，并在重启或发生漂移后重新收敛。表达这种语义时，优先使用 `suspended`、`stopped` 等状态式命名，而不是容易被理解为命令的 `stop`。

如果开关变化的真实含义是“执行一次动作”，则不适合用持久 Spec 开关表达。例如 `restart: true`、`retry: true`、`purge: true` 或 `rotateCredential: true` 在重试和重启后都有歧义：由谁将它恢复为 `false`、操作是否已经完成、如何再次请求同一种操作都不明确。此类行为应建模为拥有稳定身份、目标、状态和结果的 Operation。

可以用一个问题判断：Controller 重启后再次看到相同值，是否应该继续维持这个状态？如果是，它属于 Spec；如果重复执行会产生错误，而该值只是在表达一次动作边沿，则应使用 Operation。`stop` 这样的词可能表达两种模型，因此必须明确语义：“保持停止，直到期望被修改”是期望状态；“停止本次 Run 一次”是 Operation。

优先使用 Status Patch、Merge Patch 或 Server-Side Apply，避免完整对象 Update。每个 Controller 只修改自己拥有的字段。

## 4. 使用稳定身份并证明所有权

必须区分：

- 稳定业务 Resource ID 标识长期业务对象；
- Run 或 Operation ID 标识一次执行；
- Kubernetes UID 标识一个 Kubernetes 对象实例；
- 显示名或 Kubernetes 名称不能替代上述身份。

认领、修改或删除子资源与外部对象前，必须使用最强的可用证据证明所有权，例如 UID、OwnerReference、稳定父资源身份、模板身份和作用域元数据。

- 名称和 Label 可用于寻找候选对象，不能单独证明所有权。
- 权威元数据矛盾时停止并报告冲突。
- 不静默接管未知对象。
- 派生资源必须继承父资源的稳定 Tenant 和 Owner 身份。
- 当可信父资源唯一地分配 Namespace 时，Namespace 可以是权威的作用域元数据。Cluster 与 Node 的归属以 `Cluster.spec.agentNamespace` 为准：只能聚合该 Namespace 中的 Node CR。`rlark.io/cluster-id` 只是兼容性查询标签，绝不能作为归属证明。若两个父资源声明同一权威作用域，必须拒绝或显式报告冲突。

## 5. 将删除设计为可恢复工作流

删除是状态协调过程，不是单次 API 调用。

- 每个 Finalizer 只有一个明确责任方和职责。
- 不依赖 Finalizer 数组顺序。
- Finalizer 操作在重启或部分完成后可安全重复。
- 完成清理前确认子资源和外部资源确实已经消失。
- 通过 Condition 和有界基数指标暴露阻塞原因。
- 为卡住的清理提供可审计的人工修复路径。

工作负载清理、持久化、凭据或外部资源清理等职责应分开。需要持久化的数据必须在活动资源删除前保存完成。强制删除只能尽力补偿，不能宣称完全无损。

## 6. 保守演进数据契约

采用以下迁移策略：

```text
识别旧格式
  → 根据权威状态证明其含义
  → 在边界归一化或安全补全
  → 新写入只使用规范格式
  → 歧义数据保持不动并报告
```

- 兼容读取旧格式，新写入向唯一表示收敛。
- 归一化 API 响应前先 DeepCopy informer 或 lister 对象，绝不修改缓存对象。
- 读取时归一化已经足够时，避免大范围历史回填。
- 废弃字段必须有迁移说明和测试，不能长期保留两个同等权威字段。
- API 稳定前补齐默认值、Schema 校验、转换规则和兼容测试。
- 非法 Selector 和未知枚举应明确拒绝，不能静默忽略。

公开枚举、路由、Runtime 或 Driver 就是在提供契约。不要把尚未实现的能力作为稳定 API 发布；真正的实验能力应清晰标记，或在语义成熟前保持非公开。

## 7. 优先使用原生和强类型契约

当 Kubernetes 已能表达需求时，复用其惯例与类型，例如 OwnerReference、Finalizer、Condition、Status Subresource、Lease、`PodTemplateSpec`、Resource Quantity、Merge Patch 和 Server-Side Apply。

- 优先使用强类型 API 和明确判别字段，避免 `any` 和无约束 Map。
- 可机械验证的约束应进入 CRD、protobuf 或请求 Schema，不能只写在注释或 UI 校验中。
- 数量与长度配额不要写进 CRD Schema。枚举、列表语义、必填字段、类型和格式属于结构性约束，应当留在 Schema 中；而“最多 N 项”“最多 N 个字符”属于产品策略，由 Gateway 或写入方控制器在写入路径上校验。参见 [API 约定](api-conventions.md) 第 7 节「Schema 校验边界」。
- 区分合法零值与“未配置”。
- 生成代码只通过源定义和生成工具更新，不直接手改。
- 尚无实现和一致性测试的 Runtime 变体不进入稳定 Schema。

## 8. 明确 API 与持久化边界

Kubernetes API 是声明接口和活动协调接口；REST API 是稳定业务视图和操作边界；持久化是长期业务数据与查询的实现，不是公共契约；Provider 协议是独立版本化边界。

- 不把数据库行或保存的 CR Raw JSON 暴露为长期公共 API。
- 稳定业务身份与 Kubernetes 运行元数据分离。
- Kubernetes 资源用于活动协调，不用于保存无限增长的业务历史。
- 资源 API 承诺支持直接 Kubernetes 操作时，正确性不能依赖所有请求都经过 Gateway。
- REST API 使用一致的错误、过滤、排序和分页语义。
- 使用稳定的机器可读错误码和 Request ID，不泄露敏感实现细节。
- 跨组件长时间操作使用可观察的异步 Operation 表达。

## 9. 在可信边界强制执行租户与安全规则

Tenant 是稳定业务作用域，不只是任意 Label，也不必等同于 Kubernetes Namespace。部署 Namespace 可以根据 Tenant 和集群上下文解析，但不能定义 Tenant 身份。RLark 不干涉上层产品或具体部署将 Tenant 与 Namespace 定义为一对一绑定；这里只是不把这种绑定固化为核心模型的不变量。此类策略应通过显式配置或 Resolver 实现，使核心模型仍能兼容一对多、按集群分别映射以及未来变化的部署规则。

- 核心资源只有一个主要 Owner Tenant；共享使用显式 Binding。
- Tenant 和 Owner 元数据由可信服务端或 Admission 路径设置或验证。
- 派生资源继承 Tenant，不能由下游 Controller 改变。
- Authentication、Authorization 和 Admission/Policy 职责分离。
- 授权以 Subject、Action、Resource、Scope、Context 表达，默认拒绝。
- List 过滤不能替代 Get、Logs、Exec、Proxy、Terminal 或 SSH 时的再次授权。
- 客户端 Header、Label、Annotation、名称、ID、路径或隧道来源都不能直接作为授权证据。
- Gateway 与直接 Kubernetes API 路径应执行等价隔离。
- 审计敏感操作的允许与拒绝，但不记录 Secret。

## 10. 显式建模生命周期与能力

不能把独立维度折叠到一个 Phase。例如 Node 的连接、就绪、可调度、资源压力和生命周期是不同概念。

- 基础设施资源是长期资源；暂时离线不等于删除。
- 高频存活信号使用 Lease 类机制，避免不断重写大型 Status。
- 使用服务端时间、可配置超时和迟滞判断连接状态。
- 显式声明 Runtime、Workload、Network、Storage、Operation 和 Device 能力。
- 不根据宽泛 Runtime 类型推断所有能力。
- 可行时在下发前检查任务需求与可用能力。

当兼容性依赖版本时，能力声明应包含可用状态和版本。

## 11. 显式管理依赖和组件生命周期

业务逻辑放在可测试 Package 中；命令入口主要负责装配和运行组件。

- 外部 Client 通过构造函数和基于真实替换点的小接口注入。
- 阻塞或远程操作接收 `context.Context`，遵守取消和 Deadline。
- 启动后台工作前验证配置。
- 后台 Worker 的 Start 和 Stop 行为必须显式。
- 避免包级全局变量、隐藏式 Service Locator、无类型占位依赖和构造函数隐式启动 Goroutine。
- 不为暗示未来后端支持而创建没有真实实现的推测性抽象。

长驻 Manager 应能发现漂移并自愈，清晰定义并发和锁所有权，并能使用 Fake Client 或注入时钟进行测试。

## 12. 统一配置与错误处理

长驻组件应逐步统一为：

```go
DefaultConfig()
SetupFlags(...)
Validate()
```

- 默认值保守且可预测。
- 校验数值范围和跨字段关系。
- 除非组件明确支持热更新，启动后配置保持不可变。
- 环境变量解释集中在配置边界。

Go 中使用 `%w` 包装错误并补充操作上下文，通过 `errors.Is`、`errors.As` 和 `errors.Join` 保留类型语义。不能无说明地忽略错误。HTTP 和 RPC 边界应统一映射领域错误，不能向用户暴露任意内部错误字符串。

## 13. 每类可观测信号各司其职

- **Condition** 回答资源当前处于什么状态。
- **Resource Event** 解释有意义的状态变化。
- **Log** 提供诊断细节。
- **Metric** 展示聚合健康、性能和容量。
- **Audit Event** 记录安全相关操作和结果。
- **Trace** 串联选定的跨组件操作。

跨组件传播有用的 Request、Correlation、Tenant、Resource、Run、Task Run 和 Operation 上下文。常规指标 Label 不使用无界 Resource ID、用户名或错误文本。重复日志和 Event 应限频。可观测系统故障不能阻塞核心 Reconcile。

Token、私钥、Secret 值和敏感环境变量不得出现在日志、Event、Metric、Trace 或返回给用户的错误中。

Liveness 与 Readiness 必须分离：Liveness 判断进程是否需要重启，Readiness 判断组件是否能安全接收工作。

## 14. 保持 UI 强类型和分层

- 业务请求统一经过共享 Transport 和领域 API 层。
- 在边界把 Wire DTO 转换为强类型页面或领域模型，不让 `any` 扩散到组件。
- 将解析、归一化、校验和状态转换提取为纯函数并编写聚焦测试。
- 页面增长后拆分 Data Hook、Form Model 和展示组件。
- 集中管理刷新行为，适当在页面隐藏时暂停，取消过期请求，防止旧响应覆盖新状态。
- Mock 安装在 Transport 边界，不在业务组件中散布 Mock 分支。
- UI 与 API 语义保持一致，但安全授权和保护系统的校验绝不能依赖 UI。

## 15. 测试故障与恢复语义

Controller 和分布式组件的测试不能只覆盖成功路径，应包括：

- 重复 Reconcile 和通知丢失；
- 旧版本更新冲突；
- 重启后从观测状态恢复；
- 外部操作部分成功；
- 依赖故障和恢复；
- 延迟删除和资源已经不存在；
- 所有权冲突和有歧义的旧对象；
- API 响应归一化时缓存保持不变；
- 取消、超时和并发停止；
- 租户与授权拒绝路径；
- 配置漂移和自愈。

优先使用 Fake Client 和小型注入接口编写确定性单元测试；行为跨越真实 API 边界时，再增加集成测试或一致性测试。

## 16. 贡献与兼容门禁

所有用户可见变更都需要测试以及中英文文档。公共函数和类型应有有效注释。生产代码使用结构化日志。遵循仓库 Lint、Conventional Commits 和 Signed-off-by 要求。

稳定契约变更应包含相应检查：

- CRD Schema 与旧对象兼容性；
- 生成 Client 和 Manifest；
- protobuf 兼容性；
- OpenAPI 与路由一致性；
- 数据库向前迁移；
- Provider 一致性；
- REST 与 UI 行为一致性。

## Agent 设计检查清单

开始实现前，Agent 应回答：

1. 权威的期望状态和观测状态分别是什么？
2. 每个字段、子资源和 Finalizer 由谁拥有？
3. 稳定业务身份、运行身份和 Kubernetes 实例身份分别是什么？
4. 事件重复、丢失或组件重启后处理时会发生什么？
5. 外部操作成功但响应丢失时会发生什么？
6. 如何读取旧数据，什么证据能保证认领没有歧义？
7. 每个入口需要哪些 Tenant 和授权检查？
8. 哪些错误是永久、冲突、进行中或可重试的？
9. 使用哪个 Condition、Event、Metric、Log 或 Audit 解释结果？
10. 哪些故障与恢复测试能够证明系统最终收敛？

如果这些问题没有明确答案，设计还不适合进入实现阶段。
