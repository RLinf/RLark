# 节点与资源管理

## 管理节点调度

管理员可通过切换节点的调度状态来控制节点是否接受新工作负载。

### 暂停调度（Pause Schedule）

- PATCH Node 资源：设置 `spec.unschedulable: true`
- 节点上已有的运行中 Worker 不受影响，也不会被自动标记为失败
- 新任务不会被调度到此节点
- 暂停调度只作用于 RLark：Agent 仅在本地 Kubernetes Node 上设置 `rlark.io/unschedulable=true` 标签，所有 RLark 工作负载模板都带有排除该标签的强制 nodeAffinity。RLark 不会写入 Kubernetes Node 的 `spec.unschedulable`，因此非 RLark 纳管工作负载的 Pod 仍会被调度到该节点
- 此操作不会改变 `AgentConnected` 或 `Ready`：处于暂停调度状态的 Node 仍可保持连接和就绪
- 节点列表会将该 Node 显示为**不可用**，调度原因为**已暂停调度**，生命周期 Phase 仍为 `Cordoned`；因为处于暂停调度状态的节点无法接收新任务，已有工作负载继续运行
- 在维护、升级或节点异常时使用
- 在 RLark 之外被 cordon 的节点会在 Node 状态的 `KubernetesCordoned=True` 中体现，并在节点列表中标记。RLark 不管理该字段，但被 cordon 的节点无法接收任何 Pod，因此 Node Controller 会同时写入 `Schedulable=False`（原因为 `KubernetesCordoned`），Job 调度器不再向该节点放置新 Task
- 迁移说明：由更早版本 RLark 执行暂停调度、同时被写过 Kubernetes Node `spec.unschedulable` 的节点会继续保持 cordon，因为 RLark 不再管理该字段。确认节点健康后，请用 `kubectl uncordon <node>` 显式恢复

## 排空（Drain）节点

排空是一次性的节点运维操作。当节点需要停机维修、重启、升级或退出服务，而节点上仍有 Task 在运行时，使用排空有序停止这些 Task。排空同时会阻止新任务放置；如果只想阻止新放置、保留已有工作负载，请使用上一节的**暂停调度（Pause Schedule）**。

### 何时使用

- 节点需要硬件维修、操作系统升级、重启或下线，继续运行工作负载不可接受
- 需要在维护前停止节点上的 RLark 工作负载，而不是直接删除节点或逐个手动停止任务
- 不要用排空做临时调度控制：仅需阻止新任务放置时应使用暂停调度（Pause Schedule），排空还会停止节点上已有的 Task

### 与暂停调度（Pause Schedule）的区别

| 操作 | 阻止新任务放置 | 停止节点上的已有 Task | 写入的期望状态 | 恢复方式 |
|------|----------------|----------------------|----------------|----------|
| 暂停调度（Pause Schedule） | 是 | 否 | `spec.unschedulable=true` | 恢复调度（Resume Schedule） |
| 排空（Drain） | 是 | 是 | `spec.unschedulable=true`，并追加一条 Drain 操作；Node Agent 随后在 Task 上写入 `spec.drainRequests` | undrain 结束排空，再用恢复调度（Resume Schedule）或 recover 恢复节点调度；被排空停止的 Task 需显式启动其 Job 才会恢复运行 |

排空不调用 Kubernetes 驱逐接口直接删除 Pod。Node Agent 识别实际调度到该节点、且归属可证明的 Task，写入一次性 `spec.drainRequests`；Task Agent 停止该 Task 的根工作负载并上报观测状态。

### 请求排空

- REST 端点：`POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/drain?namespace=<ns>`（管理员权限）
- 请求一个 `Drain` 操作，并将 `spec.unschedulable` 置为 true。若已存在未完成的 Drain，端点会复用其 operation ID，通过 `operationId` 返回，且不再追加操作；因此 UI 重试和并发重复请求会收敛到同一个操作。只有现有 Drain 全部完成后，才会创建新操作
- Gateway 将 `spec.operations` 限制为最多 32 条。达到上限后，需要创建新操作的请求返回 `409 Conflict`；复用未完成 Drain 的幂等重试仍然成功。请求新的 Drain 前，请通过 **undrain** 或 **recover** 清理已完成操作
- `spec.operations` 中的每一条都必须带非空的 `id`，且 `type` 只能是 `Drain` 或 `EnterMaintenance`。其他取值由 API Server 直接拒绝，因此格式不合法的操作根本不会进入控制器、被静默忽略
- 对于节点上仍有 Pod 的每个 Task，Agent 会在其 `spec.drainRequests` 中记录一条请求；Task Agent 停止该 Task 的根工作负载并上报 `WaitingForReschedule`。Agent 为每个 Task 最多写入 32 条 DrainRequest；达到上限后，重复记录已存在的请求仍是成功的空操作
- `spec.drainRequests` 是 Node 当前 Drain 操作的投影，而非永久的用户期望状态。当 Drain 操作通过 **undrain** 或 **recover** 从 `spec.operations` 移除后，Agent 会删除对应请求；但被排空停止的 Task 不会自动恢复运行：Node Controller 写入的停止标记会让 Task 保持 suspended，直到运维显式启动其 Job（或取消该 Task 的 suspend）。删除 Node 时会同时释放请求并清除停止标记，因此 Agent 离线或 Node 对象已删除都不会让 Task 永久停摆
- 删除 Node（`DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}`）时，会先从受影响的 Task 中释放指向该节点的 `spec.drainRequests`，再删除 Node 对象；因此即使 Node Agent 离线，直接删除 Node 也不会让 Task 永久停摆
- `spec.drainRequests` 仅由 Node Agent 写入。Job/Workflow 的 Task 模板中携带的该字段在 Job controller 创建 Task 时会被忽略，因此复制或恢复 Job 不会把历史 Drain 请求带入新的 Task
- 已完成的 Drain 会保留在 `spec.operations` 中直到运维显式清除，因此节点仍处于排空状态期间，受影响的 Task 会保持停止；该操作在保留期间保持完成，恢复调度不会把它重算为未完成并重新触发驱逐

### 观察排空进度

Node Controller 将每个 Drain 操作的有界进度写入 `status.drainOperations`：

| 字段 | 含义 |
|------|------|
| `operationID` | Drain 操作的 ID，与 `spec.operations[].id` 对应 |
| `waiting` | 已针对当前 generation 上报 `WaitingForReschedule` 的 Task 数量 |
| `blocked` | 尚未上报当前 generation 观测，或上报 `Blocked` 的 Task 数量 |
| `completed` | 该操作是否可视为完成 |

- 至少有一个 Task 收到该操作的 DrainRequest 时，仅当 `blocked=0` 才将 `completed` 置为 true；`waiting` 不计入阻塞，因为对应根工作负载已经停止运行
- 还没有 Task 收到 DrainRequest 时，Node Controller 参考 Agent 上报的 `status.activeWorkloads`：Agent 尚未上报工作负载快照时不完成；`RLarkManaged=0` 视为节点上确实没有 RLark 工作负载；`RLarkManaged>0` 说明 Agent 尚未发出请求
- `Draining` Condition 的 reason 表达整体状态：`DrainRequested`（True，等待 Task 观测）、`DrainBlocked`（True，仍有阻塞的 Task）、`DrainComplete`（False，全部完成）、`NoDrainRequest`（False，没有 Drain 操作）
- `status.phase` 在存在未完成 Drain 操作时为 `Draining`；完成后回落到 `Cordoned`，因为排空同时把 `spec.unschedulable` 置为 true
- Task 侧：`spec.drainRequests` 是 Node Agent 写入的一次性请求（含 `operationID`、`nodeUID`、`nodeName`）；`status.drainRequests` 是 Task Host 的观测。`WaitingForReschedule` 表示根工作负载已经停止、正在等待重新放置，并不表示任务已经迁移到其他节点

```bash
# Node 侧的排空进度
kubectl -n <agent-namespace> get nodes.rlinf.io <name> \
  -o jsonpath='{.status.drainOperations}'

# Draining Condition
kubectl -n <agent-namespace> get nodes.rlinf.io <name> \
  -o jsonpath='{.status.conditions[?(@.type=="Draining")]}'

# Task 侧的排空请求与观测
kubectl -n <agent-namespace> get tasks.rlinf.io <task> \
  -o jsonpath='{.spec.drainRequests}{"\n"}{.status.drainRequests}'
```

### 结束排空（undrain）

- REST 端点：`POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/undrain?namespace=<ns>`（管理员权限）
- 移除 `spec.operations` 中所有 Drain 操作，保留 `EnterMaintenance`；不会把 `spec.unschedulable` 置为 false
- 幂等：节点没有 Drain 操作时直接返回当前对象，不做修改
- 实际效果：undrain 只移除管理面的 Drain 期望状态并释放 `spec.drainRequests`，不会让被排空的 Task 自动恢复运行，也不会触发自动重调度。Node Controller 写入的 `NodeDrainStoppedAnnotation` / `NodeDrainSuspendedAnnotation` 会保留，Task 保持 suspended，直到运维显式启动其 Job（或取消该 Task 的 suspend）
- 释放请求后，节点仍保持 `spec.unschedulable=true`。需要任务回到该节点时，请继续执行 **uncordon** 或直接 **recover**，然后显式启动对应的 Job；需要任务迁移到其他节点时，先确认集群容量、nodeSelector 和污点约束，再启动 Job

### 排空后的节点恢复

#### 恢复调度（Resume Schedule）

- REST 端点：`POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/uncordon?namespace=<ns>`（管理员权限）
- 等价 kubectl：`kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge -p '{"spec":{"unschedulable":false}}'`
- 将 `spec.unschedulable` 置为 false，节点恢复接受新工作负载
- 会移除 `spec.operations` 中所有 Drain 操作（保留 `EnterMaintenance`），并释放对应的 `spec.drainRequests`；但被排空停止的 Task 不会自动恢复运行，其 `NodeDrainStoppedAnnotation` / `NodeDrainSuspendedAnnotation` 会保留，需显式启动其 Job 才会恢复
- 需要同时清除 cordon 标记和 EnterMaintenance 等运维操作时，请使用下方的 **一键恢复** 端点
- 恢复调度前请确认节点健康

#### 暂停调度后的一键恢复（Recover）

- REST 端点：`POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/recover?namespace=<ns>`（管理员权限）
- 组合恢复操作：移除 `spec.operations` 中所有 Drain 和 EnterMaintenance 操作，同时将 `spec.unschedulable` 置为 false
- 被排空停止的 Task 不会随 recover 自动恢复运行：停止标记会保留，需显式启动其 Job 才会恢复
- 幂等：如果节点已处于调度状态且没有待处理的排空操作，端点会直接返回成功而不修改对象
- 典型运维流程：暂停调度（Pause Schedule）→ 排空任务（Drain）→ 硬件/软件修复 → **一键恢复**

## 取消纳管节点

只要任一 Task 的 `status.observedNodes` 仍包含该节点，取消纳管会被拒绝并返回 `409 Conflict`，两个入口行为一致：

- `DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}?namespace=<ns>`（管理员权限）——删除管理面 Node CR
- `POST /api/v1/data-plane-nodes/{agent_id}/nodes/{node_name}/unmanage`（管理员权限）——移除数据面 Kubernetes Node 的 `rlark.io/enabled` 标签，Agent 停止上报该节点

请先排空节点，等待任务全部迁移或停止（`Draining` Condition 的 reason 达到 `DrainComplete`）后再取消纳管；没有观测到任务的节点可直接取消纳管。拒绝响应中会列出仍在该节点上的 Task 名称。此外，节点必须已无法接收新任务：Agent 仍连接的节点需先暂停调度（`spec.unschedulable=true`）；Agent 未连接的节点则无需先暂停调度，因为控制器会将其判定为 `Offline` 且 `Schedulable=False`，新任务不会被放置到该节点，从而保证失联节点仍可取消纳管。

管理后台在启用「取消纳管」操作前应用同样的两个前置条件，并以 Agent 上报的 `status.activeWorkloads.rlarkManaged` 判断节点上是否还有 RLark 工作负载。该计数不包含非 RLark 的系统 Pod，因此仅运行平台负载的节点在暂停调度后即可取消纳管。服务端基于 Task 的校验仍是最终权威：若控制台放行了操作但该节点仍被某个 Task 引用，请求依旧会返回 `409 Conflict`。

删除入口会在删除 CR 前释放受影响的 Task 中指向该节点的 `spec.drainRequests`，因此即使 Node Agent 离线、无法再 reconcile 该节点，这些 Task 也能恢复运行。作为最后兜底，控制面会为每个 Node 添加 `rlark.io/node-drain-request-cleanup` finalizer：若通过其他路径（例如 `kubectl delete`）删除节点，该 finalizer 会在删除完成前执行同样的释放逻辑，不依赖 Gateway 或 Node Agent 在线。

通过移除标签入口取消纳管时，Agent 会停止上报该节点，并在一个确认周期（约 30–36 秒；扫描间隔带最多 20% 抖动，避免同时重启的 Agent 同步发起管理面 List 请求）后删除管理面 Node CR。如果本地 Kubernetes Node 完全消失，Agent 会等待约两分钟再删除 CR，避免把同名重建的 Node 误判为已移除。只有 Agent 至少成功上报过一次的 CR 才会被删除，且 Agent 不会清除其他控制器添加的 finalizer；如果对应控制器未运行，CR 会停留在 `Terminating`，直到该控制器完成清理。

## 节点标签与管理元数据

- 管理员可为节点添加自定义标签以进行分类管理；标签以数据面为准，并供用户在任务配置中通过 nodeSelector 使用
- 管理元数据存储在 RLark Node CR 的 Annotation 上，不会被数据面覆盖；同名键同时存在时 Annotation 优先于旧版 Label
- 常用元数据：`rlark.io/node-category-{cloud,edge,robot}`（Annotation）、GPU 型号、设备型号、位置

| 键 | 可选值 | 说明 |
|----|--------|------|
| `rlark.io/node-category-{cloud,edge,robot}` | `true` | 节点类型（Node CR 注解，可同时属于多个分类） |
| `rlark.io/node-category` | `cloud`, `edge`, `robot`, `other` | 旧版逗号分隔分类值，从 Label 或 Annotation 兼容读取 |
| `rlark.io/gpu-model` | `A100`, `V100`, `T4` 等 | GPU 型号（云端节点，Node CR 注解） |
| `rlark.io/device-model` | `franka`, `ur5`, `realsense` 等 | 具身设备型号（Node CR 注解） |
| `rlark.io/city` | 任意 | 物理位置（Node CR 注解） |

## 查看节点资源

打开管理后台 → 节点，查看和管理节点：

> **截图说明：** 截图来自示例环境，资源名称和数据仅供说明，实际环境会有所不同。

![节点详情](../../images/ui/first-login-node-detail.png)

节点详情包括：
- 调度状态（可调度 / 已暂停调度），与连接和就绪状态独立
- 连接与就绪 Condition，以及 Agent 上报的活动工作负载摘要
- 管理 API 或审计集成已记录时的暂停调度操作时间和操作人
- 节点类型、接入形态、操作系统、架构
- Agent 版本
- 资源使用情况：CPU、内存、磁盘、GPU 和具身设备资源
- 磁盘使用率达到 90% 或 kubelet 上报 `DiskPressure=True` 时显示告警
- 节点上运行中的关联任务

## 场站节点

管理后台的**场站节点**页面是有界内容列（1480px，宽屏居中）。**加入节点**位于工具栏，状态页签是安静的下划线页签，显示全部、在线、离线、冲突和已移除数量。工具栏支持按节点名和 UID 搜索、按成员集群筛选、刷新数据，以及查看所选集群的近期节点操作。每个含机器人的集群单独成组，标题为集群名加一行等宽数字统计（驱动就绪、相机异常、在线）：控制面节点是紧凑行，机器人与它同组，使用相机墙 tile。机器人 tile 以画面为先：身份、状态、核验标签、期望与当前电源，以及符合当前状态的操作（启动或关闭驱动、上电/下电、按住回位）直接压在大画面上；小画面排在下方一条，以每 5 秒一帧的慢速预览刷新；电源硬件报错、急停、自检未通过或维护中的 tile 有一圈琥珀色描边。电源或驱动命令会先显示正在下发，再显示 Gateway 是否接受；期望状态立即改变，机器人在下一次上报时跟上，未跟上时可在节点注解 `rlark.io/power` 中看到原因。tile 的当前电源状态也会显示硬件报错或读不到厂商监控，而不是留空；展开详情会显示节点 `rlark.io/power` 注解，便于追查回位为什么被禁用。初始姿势是按住回位：按钮文案会提示按住，短按会说明需要按住；不可用时显示是缺上电、驱动未运行、急停按下还是电源状态硬件报错；松开后显示已停止或被拒绝的原因。选择机器人后出现现有批量电源与姿势控制。其他集群按集群分组为安静的行清单，每行图标、名称、入网状态、一个色点状态词和等宽数字字段，用发丝线分隔。所有详情默认收起，一次只开一个；详情以弹框打开，不会把 tile 或行拉高或移位。手机端 tile 单列排列。

**展开详情**在页面上方打开弹框，不再使用详情抽屉、卡内页签或行内面板；tile 和行保持原位。弹框并排展示身份、网络与入网、条件；机器人弹框额外提供电源与驱动操作和四路实时相机。按 Escape 或点关闭按钮收起，点击弹框外的暗色区域不会关闭。冲突记录同时显示记录绑定的 UID 与当前上报的 UID。节点 UID、WireGuard 公钥、隧道地址和 SSH 主机指纹等身份与连接信息可复制。重发、作废以及符合条件的**删除记录**位于面板顶部，条件不满足时就地显示原因。点击相机可放大查看画面，关闭后焦点回到该相机按钮；按 Escape 收起详情并回到展开按钮。删除记录不会移除仍在上报同名节点，该节点会在下一次上报时重新登记。**入网记录**区块列出入网码和 SSH 入网记录，以及状态、发码人、发码与过期时间、公钥和隧道地址；可重发记录、作废尚未使用的入网码、清理已兑换但未完成的入网以重新开始，或打开 SSH 入网记录。入网码本身不会保存，也不会在签发后再次显示。

## 删除节点

删除节点会把节点移出集群，并把机器恢复到可以重新入网的状态。它不是“停止调度”，也不是删除场站节点记录。

RLark 把删除记录为 `action: Remove` 的 `NodeOperation`，依次经过：检查、机器人下电、清空节点、移出隧道、重置机器、删除节点对象、确认已删除。操作由控制面负责，不依赖浏览器：

- 关闭确认抽屉或切换到其他管理页面不会停止删除；运行期间右下角的任务卡显示当前阶段。
- 节点列表会把该节点标为删除中，行内和详情显示阶段；刷新页面后会从 `NodeOperation` 列表恢复进度。
- 某个阶段失败时，操作停在 `Blocked`，`Progressing` 条件给出原因。`Blocked` 是终态：控制器不会自动重试；重试会以新的 request ID 开始一轮新操作。请求结果不确定时控制台复用同一 request ID，因此重试不会创建重复操作。
- 成员集群 API 不可达时，等待超过成员 API 超时后操作会以 `MemberClusterUnreachable` 停在 `Blocked`，不会一直停留在 `Running`。
- 节点不再上报（`NotReady`）时视为已重置，即使之前启动的重置 Pod 仍停在 `Pending`；机器重新入网时会清理保留的旧状态，删除继续执行，而不是等待一个无法运行的重置 Pod。
- 同一个节点同时只允许一个 Pending/Running 的删除操作。对同一节点的第二个请求会返回 `409 NODE_OPERATION_IN_PROGRESS`，并附带正在运行的操作名；等该操作进入 `Blocked`/`Failed` 或成功后，才能重新删除或重试。
- 控制面节点不能删除：删了整个集群就没了。删除可能会停止节点上的 RLark 任务，所属 Job 会把它们调度到别处重新启动。
- 如果机器连不上，它下次加入时会清掉旧安装；删除后 RLark 可能再也到不了这台机器，可能需要从机器侧重新入网。

居中的确认弹窗会汇总节点和集群，并在有数据时显示节点状态、类型与 IP；同时列出删除步骤和最多六个将停止的任务。若场地 Hub 最近未观测到机器，或 RLark 无法核实节点 UID、将按名称删除，会分别显示警告。预检失败时可在弹窗内重试。确认后，控制面负责的操作会独立于浏览器继续执行，并可在任务中心跟踪。删除场站节点记录是另一个操作，只在该节点已移除、处于 `Conflict`，或集群里已经没有同名 Node 时可用。

## 加入节点

在管理后台 → 场站节点 → 加入节点中选择目标集群，再选择一种方式。页面由紧凑页头和目标集群选择组成；机器工作区按 **全部 / 扫描发现 / 手动添加** 筛选。

- **扫描发现**是站点 Hub 找到的机器，可搜索、批量勾选。点**加入所选机器**会打开口令弹窗，按 NodeClass 分组，显示 Orin 入口口令是否已配置，并填写 x86 登录口令和可选的 sudo 口令。x86 留空表示继续使用已保存的机型凭据；填写会覆盖并保存。弹窗先写入机型凭据，再提交入网。这些是机型凭据，不是一次性口令，RLark 不显示口令内容。
- **手动添加**是仍在进行或已经停住的 SSH 入网记录；**打开 SSH 记录**会切到 SSH 方式并选中该记录。**添加普通电脑/GPU**打开 SSH 表单，提交后机器出现在“手动添加”里。普通 x86/GPU 不会由站点扫描产生，请使用 SSH 或入网码。
- 右侧入网操作栏独立跟踪批量、SSH 和入网码入网；宽屏时在右侧，窄屏时排在下方。SSH 重试和清理仍在 SSH 记录中；等待使用的入网码可以作废。离开页面后服务端操作继续。

## 站点扫描设置

打开**管理后台 → 场站节点 → 扫描设置**（`/admin/stations/network`），也可以从**加入节点**点击**扫描设置**进入。该页面仅管理员可用：Gateway 对读取和保存都返回 `403`，控制台显示只读提示。出现 `404` 表示站点网络尚未初始化。

页头包含网络健康项（Ready、FullyConnected、DiscoveryStale、已观测/当前配置版本）以及**返回入网**和**刷新**。工作台分为三部分：

- 左侧站点栏选择站点。编辑器显示状态、集群和扫描 Hub；**取消**和**保存设置**在编辑器页头，未修改时按钮置灰。表单可开启或关闭扫描、设置 10–3600 秒的间隔、填写 1–8 条 `/22` 到 `/32` 的额外 IPv4 网段，并单独选择管理面拨号 Hub（`from`）。
- 右侧详情显示集群和 Hub、可发现 NodeClass 的入口端口与 access/discovery 标记，以及全部出厂主机密钥指纹。窄屏时详情移到编辑器下方。
- 共用的**扫描范围与限制**折叠面板列出五条规则：只发现匹配 `NodeClass.spec.discovery.hostKeyFingerprints` 的入口设备；Hub 也会扫描直连私网；网段重叠可能产生重复 Machine；修改约 30 秒生效；没有已入网 Hub 的站点不会扫描。

工作台下方有三张卡：**最近扫描事件**、**站点发现负载**（各站点发现数量，可点击切换编辑器）、**Astribot 机器人**（在线机器人、电源、相机、正在入网和站点在线数，来自场站节点数据）。这些卡只给扫描设置页增加状态，**管理工作台不变**。

普通 x86/GPU 服务器没有 discovery 声明，不会出现在扫描列表；它们通过 SSH 入网或入网码加入。要直接扫描它们，需要单独的 direct/entry-only 准入模式。

## 通过 UI 操作

管理后台 → 节点 → 选择节点 → 切换调度状态或编辑标签。

## 通过 API 操作

```bash
# 暂停调度：暂停向该 RLark Node CR 调度新任务。必须明确 namespace，避免误操作 Kubernetes 内置 Node。
kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge \
  -p '{"spec":{"unschedulable":true}}'

# 恢复调度：恢复向该 RLark Node CR 调度新任务
kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge \
  -p '{"spec":{"unschedulable":false}}'

# 设置节点类别（写入 RLark Node CR 的 Annotation）
kubectl -n <agent-namespace> annotate nodes.rlinf.io <name> \
  rlark.io/node-category-cloud=true

# 按类别列出 RLark Node CR（Annotation 不能用于 label selector）
kubectl -n <agent-namespace> get nodes.rlinf.io -o json \
  | jq -r '.items[] | select(.metadata.annotations["rlark.io/node-category-cloud"]=="true") | .metadata.name'
```

`status.phase` 仅为运维概览。放置工作负载前，必须要求控制器写入的 `Schedulable=True` Condition 和兼容的 `status.capabilities`；Node Controller 依据 `spec.unschedulable`、`AgentConnected`、`Ready` 和 `ResourcePressure` 计算 `Schedulable`，Job Scheduler 与控制台都消费该 Condition。不得仅根据 Phase 或单个 Condition 调度。处于暂停调度状态的 Node 仍可保持连接和就绪。`Degraded=True` 用于提示资源压力或能力异常，供管理员定位；资源压力不会使 Node 失去 `Ready`，但 `ResourcePressure=True` 会使 `Schedulable=False`，该 Node 不参与新任务放置，已有工作负载继续运行。`KubernetesCordoned` 是观测而非决策：它反映 Agent 观测到的本地 Kubernetes Node `spec.unschedulable`，RLark 从不写入该字段。`KubernetesCordoned=True` 表示节点在 RLark 之外被 cordon，Kubernetes 调度器已排除该节点上的所有 Pod；此时 `Schedulable` 仍为 `True`，因为本平台自身的调度意图未变。
