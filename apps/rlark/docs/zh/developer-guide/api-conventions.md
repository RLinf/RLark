# API 约定

本页是 RLark Kubernetes 兼容管理 API、REST Gateway 与数据面 Agent 共享术语和字段契约的权威说明。

它是对[项目设计原则](design-principles.md)的补充而非替代：原则解释规则「为什么存在」，本页给出变更必须遵守的「确切名称、归属方和行为」。

!!! note "如何阅读本页"

    本页描述 `main` 上的代码当前实际保证的行为。尚未实现的规则不属于本页：设计方向不是约定，计划中的行为在落地之前不得被描述为已支持。

## 1. 术语

在代码、API 字段、提交信息与文档中，请严格按下表使用术语。`api/rlark.io/v1alpha1/groupversion_info.go` 中注册的类型清单是权威来源；注意 API Group 实际为 `rlinf.io/v1alpha1`，与目录名 `rlark.io` 并不一致。

| 术语 | 含义 | 明确边界 |
| --- | --- | --- |
| Domain | RLark 虚拟网络中的逻辑分组与寻址边界。 | 其本身不是完整的安全隔离边界。 |
| DomainPeer | Domain 路由与连接信息在数据面视角下的命名空间内视图。 | 不是恰好两个 Domain 之间的二元边。 |
| Node | 接入某个数据面集群的算力容量表示。 | 归属由拥有该 Node 的 Cluster 的 `spec.agentNamespace` 确定：Node CR 只归属其所在管理 Namespace 对应的 Cluster。`rlark.io/cluster-id` 只是兼容性查询标签，不能作为归属证明；它不是 Kubernetes `Node`。 |
| Pod | 数据面中观测到的工作负载实例在控制面的视图。 | 不是 Kubernetes `Pod` 对象本身，而是它的镜像。 |
| Job | 由一个或多个 Task 模板组成的工作负载。 | 不是 Kubernetes `Job`。 |
| Task | 由运行时相关 Spec 描述的单个工作负载单元。 | Task 是 Job 的组成单元，不是 Workflow 中的步骤。 |
| Workflow | 由 Job 模板组成的 DAG。 | 依赖关系存在于 Workflow 中的 Job 之间，绝不在同一个 Job 内的 Task 模板之间。 |
| Addon | 同步到数据面的交付单元。 | — |
| Tenant | 业务租户身份。 | 在核心模型中与 Namespace 放置解耦。 |

由上表可推出两条命名规则：

- 主语是 Domain 时不要写成「集群」；主语是 RLark 的 `Node` 或 `Pod` 时，不要写成 Kubernetes 的 node 或 pod。
- `Task` 使用统一的 `PodTemplateSpec` workload 模型。Kubernetes 原生应用该模型；未来 Runtime Provider 只转换其支持的子集，并拒绝不支持的字段。`pvcStorageMap` 与 `pvcSizeGbMap` 不属于 canonical Schema；请使用 Kubernetes 原生 volume API。

## 2. 字段所有权

每个字段恰好有一个写入方。写入方必须只 patch 自己拥有的字段，这样两个 reconciler 更新同一对象的 status 时不会互相覆盖。

### 2.1 角色

| 载体 | 角色 | 写入方 |
| --- | --- | --- |
| `spec` | 持久化的期望状态。 | 客户端（Gateway、CLI、UI）或拥有它的父控制器。 |
| `status` | 可从现实重建的观测状态。 | 仅控制器与 Agent。 |
| `status.conditions` | 关于当前调谐过程的机器可读细节。 | 对该 Condition Type 负责的控制器。 |
| Operation 资源 | 具有自身身份、不可重复执行的一次性动作。 | 客户端创建，控制器消费。 |
| Resource event | 面向人、短生命周期、可丢失的叙述。 | 任意组件；永不作为状态来源。 |
| Log | 调试细节。 | 任意组件。 |
| Audit record | 持久化的「谁做了什么」。 | 服务端入口。 |

切勿把一次性动作建模为 `spec` 布尔量（例如 `restart: true`）。`spec` 开关必须是持久化的期望状态，重启后重新调谐仍然安全。

### 2.2 控制面与数据面

`Task` 是 status 归属被拆分的资源，也是引入新拆分时应遵循的范式：

- 数据面集群 Agent 拥有 `status.phase`、`status.message` 和 `status.observedNodes`。它依据实际工作负载把这些字段收集为 `StatusIntent`——Kubernetes 运行时在 `apps/rlark/pkg/agent/controllers/task/push_deployment.go` 与 `lifecycle_kubernetes.go` 中产出 Intent——再由 `apps/rlark/pkg/agent/execution/host/host.go` 中与运行时无关的 task host 按优先级合并，并执行唯一一次 status 写入。运行时 Provider 绝不能自行 patch Task status。数据面也是 `status.observedGeneration` 的归属方，并在每次被接受的写入中推进它——包括观测值恰好未发生变化的写入，否则仅修改 Spec 会让上报的 status 悄然陈旧（见 [4.3](#43-observedgeneration)）。
- 控制面 Task 控制器拥有 `status.pullProgress` 和 `status.events`，在 `apps/rlark/pkg/controllermanager/task/task_controller.go` 中聚合。

两个写入方都使用 `client.MergeFrom(original)` 进行 status patch，从而只发送自己的字段。任一方改用整体 `Status().Update` 都会在每次调谐时抹掉对方的字段。task host 还会用它观测到的 Task UID 与 generation 对写入做围栏，因此为陈旧 Task 实例计算出的 status intent 会被丢弃而不是被应用。

其余资源的归属方是单一的：

- 控制面拥有 `Job`、`Workflow`、`Domain` 的 status。
- 数据面 Agent 只拥有 `Node` status 中由 Kubernetes 派生的容量与健康字段。控制面的 Node Reconciler 独占写入由 Agent Lease 推导的连接 `phase`、心跳时间和 `Ready` Condition；两个写入方都只能 patch 自己声明拥有的字段。
- 控制面的 Cluster Reconciler 拥有 Cluster 聚合 status 及其 `Ready` Condition。它通过列出 `Cluster.spec.agentNamespace` 中的 Node CR 推导成员关系，绝不通过选择可变的 `rlark.io/cluster-id` 标签。
- Gateway 对 `/nodes` 同样实施该归属规则，包括提供 `namespace` 查询参数时。未被认领的 Namespace 返回 `items: []` 的空列表；若两个 Cluster 声明同一 Agent Namespace，Gateway 只列出该 Namespace 一次并记录 warning 日志，避免历史重复声明导致 Node 列表不可用。Cluster 写入目前直接经由管理 API，Gateway 不注册 Cluster 的创建、更新与 Patch 路由；权威归属方必须检测并报告冲突声明。由于该列表合并多个 Namespace，其不透明的 `metadata.continue` token 只能由 Gateway 签发和消费，不能转发给 Kubernetes API。聚合 Node 列表不支持 `labelSelector` 和 `fieldSelector`；两个参数都会返回 `400 INVALID_REQUEST`。
- 数据面 Agent 拥有 `Pod` 与 `Addon` 的 status，它们是向上上报的纯观测。

### 2.3 确定性

status 写入方对相同输入必须产生逐字节相同的输出，因为每个 reconciler 都会把计算出的 status 与已存值比较，相等时跳过写入。对 Go map 不排序直接聚合会破坏这一点：比较永不成立，控制器就会每次调谐都写 status。因此 `aggregatePullProgress` 先对节点名排序，再按镜像做稳定排序，最后按固定上界截断。

## 3. 身份

Kubernetes 对象的 name 就是稳定的业务身份。不存在第二个业务 ID 字段，也不应引入：并行的标识符会让同一个对象拥有两个权威句柄，并且需要在 API Server 已经为 name 强制保证的唯一性之上，另行定义一套唯一性与冲突规则。

存在三类身份，不得互相替代：

- **业务身份**：namespaced 资源是对象的 Namespace 与 name，cluster-scoped 资源只是 name。它在对象创建时确定——由调用方提供，或由控制器为子对象派生——并在该对象的生命周期内不可变。它是各层实际用来做键的句柄：Gateway 的路径参数、存储层建索引的 `namespace` 与 `name` 列，以及派生子对象的名称。子对象的身份派生自父对象，而不是取自恰好存在的某个子对象：`utils.ChildName` 把 Task 名构造为 `<job name>-<task template name>`，因此 Task 的身份可以由 Job 的身份加模板名复现。
- **Kubernetes UID**（`metadata.uid`）精确标识一个 Kubernetes 对象实例。用它证明缓存对象或子对象仍是你观测到的那个实例。镜像 Pod 上的 `rlark.io/task-uid` 与 `rlark.io/local-pod-uid` 就用于此目的。缺失的 UID 应保持为空；存储层不得伪造一个，否则读者无法分辨真实的实例身份与编造的身份。
- **Run 身份**标识业务对象的一次执行。它既不是业务身份，也不是 UID。目前 `api/rlark.io/` 下没有任何已注册类型携带 Run 身份；如果要引入，它应当放在 annotation 中而不是放进 `spec`，因为它是关于某一次执行的观测状态，不属于用户管理的期望状态。

从前两类身份的区分出发，可以得到两条规则：

- 绝不用 name 证明对象实例身份。name 可以被删除后重建，此时 name 未变而 UID 已变，仅按 name 匹配的控制器会认领到另一个实例。
- 绝不把 UID 当作长期业务句柄。它在每次重建时都会更换，因此以 UID 为键的记录会不再描述用户仍在管理的那个业务对象。

因为 name 承载业务含义，所以它不可编辑；改名意味着另一个业务对象。面向用户的可变展示名应放在单独的展示字段中，该字段不被任何层当作键，也永远不是身份。

标签是范围元数据与选择器，不是身份。任何能写 metadata 的人都可以给对象重新打标签，因此标签可以缩小检索范围，但在证明所有权时绝不能替代 name 或 UID。

!!! note "归档与恢复——持久化后端驱动的生命周期"

    已结束的 Job 在 `ArchiveTTL`（默认 5 分钟）后被归档到持久化后端，并从 kcp 删除活动 CR。业务记录（spec + 最终状态）保留在 PostgreSQL 或 Kubernetes Secret（测试模式）中，活动 CR 被删除以释放 etcd 和 controller 内存。之后通过 `POST /jobs/:name/start` 从持久化后端恢复同名同 spec 的新 CR，新 CR 的 Kubernetes UID 会变化但业务标识（name）不变。

    统一启动接口是 `POST /jobs/:name/start`：活跃 Job（KCP 有 CR）直接 PATCH `spec.suspended=false`；归档 Job（KCP CR 已删）从后端恢复。前端调用时无需感知 Job 是否在 kcp 中。

    `PersistenceBackend` 接口抽象 PostgreSQL 和 Kubernetes Secret 两种后端，Syncer 和 Gateway 共用同一契约。新增后端类型只需实现 `Save`/`Load`/`List`/`Delete` 并在 `controller-manager.go` 和 `gateway.go` 中注册。

在认领、修改或删除子对象或外部对象之前，必须用最强证据证明所有权：UID、OwnerReference、父对象 name、模板身份和范围元数据。无法证明所有权时，什么都不做。

## 4. Phase 与 Condition

### 4.1 Phase

`Phase` 是单一、粗粒度、面向人的生命周期摘要。所有 Phase 类型都是被 API Server 校验的封闭枚举：

| 类型 | 取值 |
| --- | --- |
| `JobPhase` | `Pending`、`Running`、`Succeeded`、`Failed`、`Stopped` |
| `TaskPhase` | `Pending`、`Running`、`Succeeded`、`Failed`、`Stopped` |
| `WorkflowPhase` | `Pending`、`Running`、`Stopping`、`Stopped`、`Succeeded`、`Failed` |
| `AddonPhase` | `Pending`、`Installing`、`Ready`、`Failed`、`Upgrading` |
| `PodPhase` | `Pending`、`Running`、`Succeeded`、`Failed`、`Unknown` |
| `NodePhase` | `Online`、`Offline` |

向这些枚举新增取值属于 API 变更：必须同时提交重新生成的 CRD manifest，并确保消费方能容忍新取值。

### 4.2 Condition

Condition 承载 `Phase` 有意省略的机器可读细节。所有 Condition Type 集中声明在 `api/rlark.io/v1alpha1/conditions.go`；新增类型必须在该处声明并在本页记录，而不是在实现包里就地定义。

| 类型 | 含义 | 写入方 |
| --- | --- | --- |
| `Validated` | 控制器对资源 Spec 的校验结论。`False` 表示 Spec 非法且不会再被调谐。 | 控制面 Job、Workflow、Domain 控制器 |
| `Progressing` | 资源仍在向期望状态推进。`True` 表示仍有进行中的动作。 | 数据面 Task 控制器 |
| `Ready` | 资源当前是否可用。它是 `Phase` 的结构化补充：`Phase` 给出聚合阶段，`Ready` 给出布尔结论与机器可读的 `Reason`。 | 数据面 Addon 控制器 |

`Progressing` 目前只在已认领 Workload 的迁移路径上写出，因此未发生 Workload Kind 迁移就收敛的 Task 不会上报 `Progressing` Condition；消费者不得把它的缺失读作「未在推进」。

`Node` 声明了 `status.conditions` 但尚无写入方：节点就绪性只通过 `status.phase` 上报，因此消费者不得预期 Node 上存在 `Ready` Condition。

`Conditions` 字段的规则：

- 字段类型为 `[]metav1.Condition`，带 `+listType=map` 与 `+listMapKey=type`，因此同一 Type 至多出现一次。
- 通过 `apimeta.SetStatusCondition` 写入，以保证 `LastTransitionTime` 语义正确。
- `Reason` 与资源和场景相关，使用 CamelCase，取值稳定且可被程序消费。Reason 有意不做集中定义。
- `Validated` 的写入方必须记录两种结论。只写 `False` 会在用户修好 Spec 之后留下陈旧的拒绝结论，因此同一控制器在 Spec 校验通过时也要写 `True`。
- 非法 Spec 无法通过重试修复。因此控制器记录 `Validated=False` 后即停止，而不是返回 error——返回 error 会导致无限重入队；当出现新的 generation 时调谐自然恢复。

### 4.3 ObservedGeneration

`ObservedGeneration` 是**控制器契约，而非 API Server 保证**。它只表示「写入时该控制器已完成对这一 generation 的调谐」。消费者必须把 `observedGeneration` 落后于 `metadata.generation` 的 status 视为陈旧，且不得据此对当前 Spec 下任何结论。

因此它只存在于真正存在 Spec→Status 调谐回路的资源：`JobStatus`、`WorkflowStatus`、`TaskStatus`、`DomainStatus`、`AddonStatus`。它有意不存在于 `Node`、`Pod`、`DomainPeer`——这些资源的 status 是纯数据面观测，没有需要收敛的 Spec；在那里加该字段等于宣称一个没有控制器履行的契约。

同理，`Ready` 不填写 `ObservedGeneration`：它是观测驱动而非 Spec 驱动的。`Ready` 描述的是控制器最近一次观测到的事实，而不是哪个 Spec generation 产生了它。

写入方只在一次调谐成功返回之后推进它，绝不在开始时推进。`metav1.Condition` 另带 per-condition 的 `ObservedGeneration`，Spec 驱动的 Condition 必须填写。

## 5. 租户与 Owner 元数据

租户与创建者身份存放在 `metadata` 而非 `spec`：

- 租户：标签 `rlark.io/tenant-id`（`tenant.IDLabel`）。缺少该标签的对象在逻辑上属于 `default` 租户，以兼容租户元数据出现之前创建的对象。
- Owner：注解 `rlark.io/created-by`（`tenant.CreatedByAnnotation`）。

这两个值都由服务端依据已认证身份赋值，并覆盖客户端提交的任何内容；创建请求中伪造的标签或注解会被丢弃。派生资源从父对象继承租户与 Owner 身份。客户端过滤永远不是租户边界——每个入口点都在服务端强制校验。

租户到 Namespace 的解析模型见[用户与租户](tenancy.md)。

### 5.1 租户作用域强制

请求的租户作用域在每次请求时依据已认证主体重新解析，而不是从 Token 中读取，这样成员变更会立即生效，而不必等到 Token 过期。具有 `admin` 角色的调用方不受限制；没有任何租户归属的调用方会被以 403 `FORBIDDEN` 拒绝，而不是回退到 `default` 租户。

Workflow、Job、Task 与 Pod 属于租户作用域。Pod 虽然不由 Gateway 打戳，但仍纳入其中：Pod 由 Agent 从 Task 派生并继承其身份，因此读取或 Patch 时必须遵循同一边界。Node、Domain 与 Cluster 属于基础设施对象，从不携带租户身份，因此不在该作用域内：Node 与 Domain 的写操作本身已限定为管理员，Cluster 则根本不由 Gateway 暴露写入路由。

强制校验覆盖全部入口点，包括 Create、List、Get、Update、Patch、Delete、Job 启动接口、Job 标签与日志接口、Pod 事件、Pod 终端以及 TensorBoard 反向代理。Update、Patch、Delete 与启动接口通过读取已存储的对象判定归属，因为这类请求本身不携带租户身份。启动接口的归属来自它即将 Patch 的管理 API 对象，或者在 Job 已归档时来自它即将恢复的归档快照；恢复出来的 Job 保留快照中的租户标签，因此恢复操作不会把对象迁移到别的租户。Create 取作用域中的第一个租户，除非请求体指定了作用域本身已允许的租户。Job 标签词表接口同样受作用域约束，因此只存在于其他租户的标签 key 或 value 不会出现在词表中。

各类失败的语义有意区分：

| 情形 | 响应 |
| --- | --- |
| `?tenant=` 指定了作用域之外的租户 | 403 `FORBIDDEN` |
| 创建请求体将对象标注为作用域之外的租户 | 403 `FORBIDDEN` |
| 按名访问的对象属于其他租户 | 404 `RESOURCE_NOT_FOUND` |
| 调用方没有任何租户归属 | 403 `FORBIDDEN` |

越权的过滤条件被拒绝而非静默收窄，这样客户端才能区分「过滤被拒绝」与「过滤未命中」。按名访问的对象报告为不存在而非被禁止，因为 403 会确认该名称在调用方看不到的租户中存在。

`tenant` 承载一次授权断言，因此它不出现在 [6.3](#63) 的参数表中：作用域之外的取值会被拒绝，而不是被静默收窄。作用域内的取值会被接受；在数据库支撑的 List 上，它还会像其他一等过滤器一样把结果收窄到该租户。`admin` 调用方不受作用域限制，因此任何取值都不会被拒绝，但管理员在数据库支撑的 List 上指定租户时，结果同样会被收窄到该租户。管理 API 支撑的 List 目前不把 `tenant` 当作过滤器，因此在那里作用域内的取值不改变结果集。

对持久化存储支撑的 List，作用域会转换为标签选择器，因此 `total` 与 `hasMore` 描述的是调用方可见的行。PostgreSQL 后端在 SQL 中求值该选择器；用于测试与 CI 部署的 Secret 后端本身没有查询语言，会在计数与分页之前在内存中求值同一个选择器，因此两种后端报告的可见集合一致。标签缺失时的默认值同样由后端补齐，因为标签选择器无法表达「值属于 (...) 或标签缺失」。对管理 API 支撑的 List，作用域在取回分页之后施加；与 `tagSelector` 等在服务端窗口之后应用的过滤一样，分页窗口不再描述最终返回集。

Gateway 是强制点。直接持有管理 API 凭据的访问不在覆盖范围内；用于防止直接写 Kubernetes 篡改租户与 Owner 元数据的 Admission Webhook 尚未实现。

## 6. REST 契约

Gateway REST API 是稳定的业务视图，具备统一的错误结构、统一的分页结构，以及严格的查询参数校验。

### 6.1 错误

所有非 2xx 响应体均为：

```json
{
  "code": "RESOURCE_NOT_FOUND",
  "message": "job \"train-1\" not found",
  "reason": "NotFound",
  "requestId": "7f1c...",
  "details": { "parameter": "limit", "reason": "must be a positive integer" }
}
```

`code` 是稳定的机器可读标识；`message` 面向人且不得泄露实现细节；`reason` 在上游为 Kubernetes 错误时携带其 reason；`requestId` 回显 `X-Request-Id` 请求头，客户端未提供时使用服务端生成值；`details` 是可选的结构化上下文。

错误码集合封闭，定义在 `apps/rlark/pkg/gateway/apierror.go`。`swagger_test.go` 会双向校验 OpenAPI enum 与 Go 常量一致，因此新增错误码必须同时更新两处。

| 错误码 | 典型状态码 |
| --- | --- |
| `INVALID_QUERY_PARAMETER` | 400 |
| `INVALID_REQUEST_BODY` | 400 |
| `INVALID_REQUEST` | 400 |
| `UNAUTHORIZED` | 401 |
| `FORBIDDEN` | 403 |
| `RESOURCE_NOT_FOUND` | 404 |
| `RESOURCE_ALREADY_EXISTS` | 409 |
| `RESOURCE_CONFLICT` | 409 |
| `RESOURCE_INVALID` | 400 |
| `UPSTREAM_UNAVAILABLE` | 503 |
| `INTERNAL_ERROR` | 500 |
| `NOT_CONFIGURED` | 500 |

Kubernetes 错误是映射而非包裹：`NotFound` → 404，`AlreadyExists` 与 `Conflict` → 409，`Invalid` 与 `BadRequest` → 400，`Unauthorized` → 401，`Forbidden` → 403，`ServiceUnavailable`、`Timeout`、`TooManyRequests` → 503，`UnsupportedMediaType` → 415，其余 → 500。

### 6.2 分页

所有 list 接口都有界。`limit` 默认值即为上限 500（`maxListLimit`），因此单次请求绝不会触发无界扫描，且响应总能暴露结果被截断。

存在两条链路，信封有意不同：

- **数据库链路**返回 `items`、`total`、`limit`、`offset`、`hasMore`（`db.ListResult`）。排序按名称稳定化，以保证翻页可重复。
- **Kubernetes 链路**在归一化后的 list 之外返回 `limit`、`hasMore`、`total`，**不**返回 `offset`：Kubernetes `continue` token 保留在 `metadata` 内，因为它只对直接的 Kubernetes list 有效，不应成为归档分页契约。该链路上携带 `offset` 的请求以 400 拒绝，而非忽略——返回第一页却让调用方以为已经跳过了前面的记录，比直接拒绝该参数更糟。`total: -1` 表示总数未知；聚合 Node 列表因合并多个独立分页的 Namespace，始终使用该值。其他 Kubernetes 链路仅在服务端提供足以计算的信息时报告**页相对** `total`：续页时它由该页报告的剩余条目估计推导，因此随着调用方前进而减小。应把它读作「从当前页还能触达的条目数」，而不是稳定的集合大小。只有数据库链路返回覆盖整个集合的 `total`。

归一化始终基于深度拷贝进行。塑造响应时绝不要修改从 informer 缓存取出的对象。

### 6.3 过滤与排序

非法查询参数一律以 400 + `INVALID_QUERY_PARAMETER` 拒绝，并通过 `details.parameter` 与 `details.reason` 指出问题所在。禁止静默忽略无法解析的过滤器——那会让客户端误以为过滤已生效。

按业务身份过滤不需要单独的参数：业务身份就是 name，用 `fieldSelector` 过滤 `metadata.name` 即可表达。

| 参数 | 接受形式 |
| --- | --- |
| `limit` | 整数，大于 0，至多 500。 |
| `offset` | 整数，至少 0。仅数据库链路；Kubernetes 链路以 400 拒绝该参数。 |
| `order` | `field [asc\|desc]`；field 须匹配 `^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`，方向无法识别时直接拒绝，而不是静默按升序排序。 |
| `fieldSelector` | 逗号分隔的 `key=value` 或 `key!=value`；每个 key 须匹配与 `order` 相同的点分标识符模式。 |
| `labelSelector` | 数据库链路为逗号分隔的 `key=value` 或 `key!=value`，Kubernetes 链路为 Kubernetes label selector。key 须是合法的 qualified name。 |
| `tagSelector` | 逗号分隔的 `key=value`。在服务端窗口之后应用。 |
| `continue` | 不透明的 Kubernetes continue token。仅 Kubernetes 链路。 |

选择器路径与标签 key 来自查询串，因此在数据库链路上一律作为语句参数绑定，绝不拼入 SQL 文本。上表的模式校验在入口处执行，属于纵深防御：无意义的路径在抵达数据库之前就以 400 拒绝。

`!=` 在两条链路上都匹配字段或标签**缺失**的对象。数据库链路用 `IS DISTINCT FROM` 实现；普通的 `!=` 比较在 key 缺失时求值为 `NULL`，会恰好静默丢弃调用方想要的那批对象。

## 7. Schema 校验边界

CRD Schema 与写入路径校验的是两类不同的规则，这一划分是有意为之。

CRD Schema 只封闭**结构性**语义，即一个已存储对象必须具备、从而让所有读取方以相同方式解读它的形状：

- 封闭枚举（`+kubebuilder:validation:Enum`），例如 [4.1](#41-phase) 中的各个 `Phase` 类型；
- 列表语义（`+listType=map`、`+listMapKey=type`），例如 [4.2](#42-condition) 中的 `Conditions` 规则；
- 必填字段、类型与格式。

CRD Schema **不得**承载数量或长度配额。不要在 `api/rlark.io/` 下的类型上添加 `MaxItems`、`MinItems`、`MaxLength`、`MinLength`、`MaxProperties` 标记：诸如「一个 Job 最多 10 个标签」「标签 key 最长 10 个字符」这类上限是产品策略，而不是存储不变量。这类限制由写入路径强制：客户端提交的 `spec` 由 Gateway 校验，控制器或 Agent 聚合出的 `status` 由其写入方自行裁剪或校验。

这样划分有三个原因：

- Schema 上的上限对所有写入方一律生效，包括重建 `status` 的控制器与 Agent。当控制器合法地观测到超过上限的条目时，它将完全无法写入 status，调谐循环随之失败，而运维人员没有任何放宽该限制的手段。
- 配额属于会随产品需求变化的规则。在 Schema 中修改它是一次 CRD 变更，必须滚动到每个集群；在写入路径上修改它只是单个组件的配置或代码变更。收紧 Schema 上限还会追溯性地让已存储对象变为非法，而 API Server 只在下一次写入时才拒绝，故障点因此远离变更点。
- API Server 的拒绝返回的是 Kubernetes 校验信息，而不是 6.1 节中的 Gateway 错误信封。在写入路径上强制，才能让调用方拿到稳定的 `code`、面向人的 `message`，以及指明出错字段的 `details`。

因此，生效中的配额属于强制它的那个组件，而不属于 CRD。类型注释可以说明某字段在何处被裁剪或校验，但不得暗示 API Server 会拒绝超额对象。嵌入我方 API 的上游类型（如 `metav1.Condition`、`corev1.PodTemplateSpec`）保留上游自身声明的各种上限；本规则只约束我们自己编写的标记。

Gateway 当前生效的配额作用于客户端提交的 `JobTag` 条目，在 Create、Update、Patch 上针对 `jobs.spec`、`tasks.spec` 以及每个 `workflows.spec.jobTemplates[].spec` 校验：

| 规则 | 上限 |
| --- | --- |
| 单个 spec 的标签条目数 | 10 |
| 标签 key 的字符数 | 10 |
| 单个标签条目的 value 个数 | 10 |
| 标签 value 的字符数 | 10 |

长度按 Unicode 码点而非字节计数，因此同一个上限对中文 key 与 ASCII key 含义一致。违反配额时以 400 `INVALID_REQUEST_BODY` 拒绝，并通过 `details.field`、`details.limit`、`details.actual` 指明越界的条目。Merge Patch 会整体替换数组，因此一个 Patch 要么完全不含标签数组、不触发校验，要么完整携带它、按与 Create 相同的方式校验。对于 Schema 本身就会拒绝的畸形请求体，配额校验原样放行，而不是把它改写成一次配额违规。

## 8. 兼容性门禁

生成产物是契约的一部分，由 CI 中的 `make verify-generated` 检查：它执行 `make generate`，当已提交的 CRD manifest、deepcopy 函数、类型化客户端或 CRD 参考文档与重新生成的输出不一致时失败，并且在出现未跟踪的生成文件时同样失败。

因此对 `api/rlark.io/` 下任意类型的修改必须与以下内容一起提交：

- `api/config/crd/bases/rlinf.io_*.yaml`
- `api/kubeclients/`
- `api/rlark.io/**/zz_generated.deepcopy.go`
- `apps/rlark/docs/reference/crd.md`

OpenAPI 规范由 `swagger_test.go` 双向把关：

- spec -> route：每个文档化操作都必须在路由中真实存在，不允许悬空 `$ref`，错误码 enum 必须与 Go 常量一致。
- route -> spec：`Gateway.RegisterRoutes` 注册的每个路由都必须被 `docs/api/swagger.yaml` 描述，否则必须登记在 `routesUndocumentedInSwagger` 基线中。该基线只允许收缩：当某条登记路由已被文档化、或已从路由表中消失时测试同样失败，因此补写文档时必须删除对应条目，新增端点也无法在无文档的情况下意外上线。

`proto/embodied-runtime/` 与 `proto/execution/` 下的 gRPC 契约由 `buf` 把关。两个 tree 都引入 `proto/toolchain.mk`，它是版本钉定的唯一来源；仓库根的目标会分发到每个 tree：

- `make lint-proto` 以 `STANDARD` 规则集执行 `buf lint`。各 tree 的 `buf.yaml` 豁免了少数命名规则——两个 tree 均豁免 `PACKAGE_DIRECTORY_MATCH` 与 `SERVICE_SUFFIX`，`proto/embodied-runtime/` 另外豁免 `RPC_RESPONSE_STANDARD_NAME`——因为 package 与 service 名已嵌入到已发布的 Go/Python SDK 所使用的全限定方法名中；新增文件仍应遵循 `STANDARD`。
- `make proto-breaking` 默认针对 `branch=main` 执行 `buf breaking`（CI 用 `PROTO_BREAKING_REF=ref=<sha>` 覆盖基线）。它使用 `FILE` 类别，连源码级破坏也会拒绝，因为 `sdks/` 下的生成产物已入库并对外发布。请新增字段和 RPC，而不要重命名、改编号或删除既有定义。
- `make verify-proto-generated` 重新生成 stub，并在结果与已提交内容不一致时失败，因此 `.proto` 的改动无法脱离其 Go/Python 生成物单独上线。

生成物在自身文件头记录了生成器版本，Python stub 还会在导入时拒绝过旧的运行时，因此工具链采用钉定版本而非 `latest`。所有 tree 必须使用同一套钉定版本：用不同工具链生成的 tree 会在下一次重新生成时改写整片生成物。

| 工具 | 版本 | 钉定位置 |
| --- | --- | --- |
| `protoc` | `36.2`（文件头写作 `v7.36.2`） | `proto/toolchain.mk` |
| `protoc-gen-go` | `v1.36.12` | `proto/toolchain.mk` |
| `protoc-gen-go-grpc` | `v1.6.2` | `proto/toolchain.mk` |
| `buf` | `v1.72.0` | `proto/toolchain.mk` |
| `grpcio-tools` | `1.84.0` | `sdks/embodied-runtime-python/requirements-dev.txt` |

`make proto` 只有在找到的版本与钉定版本一致时才会执行：`protoc-check` 比对 `protoc --version`，`scripts/gen_proto.py` 比对已安装的 `grpcio-tools` 发行版本。安装方式为 `make protoc-install`（把钉定版本的 release 下载到已被 gitignore 的 `proto/bin`，该目录同时优先于 `PATH`）与 `pip install -r sdks/embodied-runtime-python/requirements-dev.txt`；Go 插件与 `buf` 会在需要时自动安装到同一目录，文件名带版本戳，因此升级钉定版本会强制重新安装。升级版本时必须在同一次改动中重新生成并提交 `sdks/*/gen/` 下的全部文件，并把 `requirements.txt` 与 `setup.py` 的运行时下界抬到新生成物所要求的版本。

数据库迁移由 `pkg/db/migrations_test.go` 把关：当迁移文件未注册自身、迁移名不唯一或不递增、或某个迁移缺少 down 函数时失败。
