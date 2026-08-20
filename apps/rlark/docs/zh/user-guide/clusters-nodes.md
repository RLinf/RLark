# 集群与节点

使用本指南在创建 Job 前查找数据面集群，并确认其中有合适的 Worker 资源。

## 任务一：查找可用集群

1. 在业务平台打开**集群**。
2. 按集群名称或 ID 搜索，或使用列表提供的元数据筛选。
3. 检查在线状态、在线率和 Worker 数量。
4. 打开计划使用的集群。

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

![节点详情](../../images/ui/first-login-node-detail.png)

!!! note "节点分类"
    业务平台使用 RLark 分类标签组织云算力、端算力和真机 Worker。旧版分类值及明确上报受支持资源的节点仍可见；Kubernetes 控制面节点不进入业务平台 Worker 视图。

## 完成结果

此时应已选定一个集群，并在需要时得到可匹配在线、可调度且容量充足 Worker 的节点选择条件。接下来可参照[提交和管理 Job](jobs.md)。

## API 等效操作

查询 Cluster 摘要和 Node CR。字段和过滤参数参见 [API 参考](../api/reference.md)。
