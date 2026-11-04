# 集群与节点

使用本指南在创建 Job 前查找数据面集群，并确认其中有合适的 Worker 资源。

## 任务一：查找可用集群

1. 在业务平台打开**集群**。
2. 按集群名称或 ID 搜索，或使用列表提供的元数据筛选。
3. 检查集群状态、在线率和 Worker 数量。
4. 打开计划使用的集群。

Cluster 是集群作用域的 RLark 资源。`spec.agentNamespace` 是其 Node CR 成员关系的权威边界：集群详情与节点数只包含该命名空间中的 Node CR。`rlark.io/cluster-id` 仅用于兼容查询，不能证明成员归属。其 cluster Agent 每 15 秒续约一次控制面的 `heartbeat` Lease，Lease 在 45 秒后过期。每个已注册 Worker 还需要由 `rlark-agent-node` 维护自身的 `node-heartbeat-<node-name>` Lease；节点 Lease 缺失或过期时，仅该 Worker 会离线。

列表的**状态**列和状态筛选使用以下集群阶段：

- **Online（在线）**：集群心跳有效，且所有已注册 Worker 在线。
- **Offline（离线）**：心跳 Lease 缺失或已过期。
- **Downgraded（服务降级）**：集群心跳有效，但至少一个已注册 Worker 离线。
- **Cordoned（停用调度）**：持续的 Cluster 意图禁止调度；该操作不会停止已有 Task。
- **Draining（排空中）**：管理员已请求有序排空。RLark 会将已处于 Succeeded 或 Failed 终态的 Task 视为已排空，并为其余 Task 记录排空停止请求，随后等待每个 Task 针对当前 generation 上报停止状态。拥有 Job 的 Task 由 Job 控制器下发暂停；独立 Task 由 Cluster 控制器直接暂停。检查会从 1 秒到 1 分钟指数退避。排空超过 30 分钟仍未完成时，会保持请求状态，并通过 `Progressing=True`、`Reason=DrainTimedOut` Condition 列出阻塞的 Task 以便恢复处理；排空完成本身不会更改持续的排空或停用调度意图。出现当前 generation 的 `Progressing=False`、`Reason=DrainComplete` 后，可使用下架隐藏集群，使用 Undrain 恢复开始排空前保存的停用调度状态，或使用故障恢复取消排空意图并恢复调度。取消排空会清除停止请求，并且只恢复由排空暂停的独立 Task；排空前就已暂停的 Task 保持自身的期望状态。
- **Delisted（已下架）**：管理员已将集群及其节点从用户控制台隐藏。对非管理员，节点页面和 Node 列表 API 不返回该集群的节点；重新上架后节点会再次可见。管理员仍可查看集群及其节点，以便执行运维或上架恢复。

列表还显示 Worker 总数、在线数和离线数，并按 `在线 Worker / Worker 总数` 计算在线率。数量和在线率反映 Worker 心跳观测结果，本身不会改变集群阶段或调度资格。

管理员可使用**故障维修（Cordon）**暂停新任务调度；当集群处于 Cordoned 状态或其排空已完成时，使用**故障恢复（Uncordon）**恢复新任务调度；使用**驱逐任务（Drain）**仅停止所选集群中的 RLark Task，等待排空完成后使用**下架（Delist）**停止向用户展示集群，使用**上架（Restore）**恢复可见性且不改变此前的调度意图。下架与由心跳缺席导致的 **Offline（离线）**状态不同。仍处于排空状态的集群，只有当前 generation 的 Condition 为 `Progressing=False`、`Reason=DrainComplete` 后才能下架或执行故障恢复；排空未完成时，这些操作会被拒绝。对已完成排空的集群执行故障恢复时，还会清除 `spec.draining` 和保存的停用调度状态，使集群重新可被调度，而不是继续被排空意图阻塞。故障恢复不会修复 Agent 连接或节点健康状态；清除暂停调度意图后，集群状态会根据已观测的心跳和节点状态重新计算。已下架集群需先上架后才能执行故障恢复。Job 的结果仍由已观测的 Task 状态聚合得出。

创建 Job 时，控制台展示 **Online（在线）** 和 **Downgraded（服务降级）** 集群。Job 控制器在解析 Task 的节点选择器时也会在服务端强制执行该规则：仅接受在线且可调度的 Node，且其所属的唯一 Cluster 必须同时满足 `spec.cordoned`、`spec.draining`、`spec.delisted` 均为 false，并且阶段为 **Online（在线）** 或 **Downgraded（服务降级）**。Cordoned、Draining、Offline 和 Delisted 集群均不会作为候选，通过 API 创建的 Job 同样受此约束。Downgraded 集群只会将任务调度至其中在线且可调度的 Worker；离线 Worker 仍不具备调度资格。Gateway 会拒绝在 Cluster 处于 Cordoned、Draining 或 Delisted 期间向其 agent namespace 创建新的独立 Task，但允许更新和 patch 已有 Task 以进行运维恢复。Cordon 不会停止已有 Task；已有 RLark Task 仍由 Drain 负责停止。

> **截图说明：** 截图来自示例环境，资源名称和数据仅供说明，实际环境会有所不同。

![集群列表](../../images/ui/first-login-cluster-list.png)

## 任务二：检查集群容量

1. 在集群详情中查看 Worker 总数、离线 Worker 和运行中的 Job。
2. 对比云算力、端算力和真机资源分组。
3. 筛选或搜索 Worker 列表，查找所需硬件。

![集群详情](../../images/ui/first-login-cluster-detail.png)

## 任务三：检查 Worker

1. 从集群详情选择 Worker，或打开**节点**后搜索。
2. 确认 Worker 在线且可调度。
3. 查看接入形态、操作系统、架构和 Agent 版本。
4. 查看 CPU、内存、GPU 容量及已申请资源。页面用量来自 Kubernetes requests 汇总，不是实时硬件利用率。
5. 检查已放置在该节点上的 Job 和 Worker。

使用节点页面的**刷新**按钮可原位更新当前筛选条件下的全部节点。请求处理中会保留当前行并显示半透明局部遮罩和居中的旋转加载指示，页面其余部分的位置不会变化。

![节点详情](../../images/ui/first-login-node-detail.png)

!!! note "节点分类"
业务平台使用 RLark 分类标签组织云算力、端算力和真机 Worker。旧版分类值及明确上报受支持资源的节点仍可见；Kubernetes 控制面节点不进入业务平台 Worker 视图。

## 完成结果

此时应已选定一个集群，并在需要时得到可匹配在线、可调度且容量充足 Worker 的节点选择条件。接下来可参照[提交和管理 Job](jobs.md)。

## API 等效操作

查询 Cluster 摘要和 Node CR。字段和过滤参数参见 [API 参考](../api/reference.md)。
