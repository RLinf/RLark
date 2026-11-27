# Astribot 上游 RLark 升级核对

本次核对范围为 `9ad4a21..d06745e`。功能变更来自 `8c3a72d`，`d06745e` 修正发布流程并补充 v0.1.0 变更日志。Astribot 分支包含 `d06745e`，也包含该区间新增的全部 211 个路径。以下分别记录分支集成和部署环境验收。

| 领域 | 分支集成 | 环境验收 |
| --- | --- | --- |
| 数据面批量升级 | 共用升级页面、Gateway API、两个 CRD、Controller Manager 协调器、Agent 执行器均已保留。Astribot 管理面身份已补齐 Gateway 访问升级任务，以及 Controller Manager 协调和上报升级任务所需权限。数据面 Agent 在目标集群命名空间的管理面角色已包含子资源和状态权限。 | 使用前先应用 CRD 和 RBAC。使用审核通过的 `rlark-upgrade` 镜像在可丢弃的 Kubernetes 测试集群验证并发、超时、失败阈值和回滚。扩展至 Astribot 管理的集群前，核对 Agent 参数、挂载、凭据和拓扑。 |
| 节点生命周期 | 节点管理保留心跳、可调度状态、暂停调度、排空进度、取消排空、恢复和取消纳管。排空及恢复调度的确认框明确了被排空的任务需要显式启动对应 Job。Astribot 的场站节点移除仍走独立流程。 | 在空闲测试节点验证排空完成、恢复调度后旧任务仍停止、显式启动 Job 后任务恢复。另行验证场站节点移除。 |
| 调度和诊断 | 创建任务保留按集群、节点、容量和可调度状态选点，以及 `/dev/shm` 配置。Worker 保留调度、存储和拉取镜像等待原因。Kubernetes 所有权链校验和状态投影也覆盖 Astribot 的 DaemonSet 和逐节点任务实例。 | 用 Kubernetes 任务验证选点、Worker 等待与共享内存挂载，核对 Pod 和 Task 状态与页面显示。验证 DaemonSet 所有者 UID 不匹配和 Pod 替换时不会带入旧状态，且逐节点实例能更新和清除。 |
| Agent、故障恢复与部署 | 启动配置校验、依赖和权限检查、周期状态修复、所有权校验、Helm Chart 和发布流程均已保留。控制面 Chart 内的 CRD 和权威 Schema 一致。 | 在测试集群验证 Agent 重启自愈、磁盘压力恢复和控制器启动。在隔离环境渲染并安装两个 Chart，核对官方部署路径。Astribot 继续使用自己的部署流程。 |

Astribot 标准发布在 `crds-rbac` 阶段应用 CRD 和生成的管理面角色。Astribot 发布镜像清单及官方 v0.1.0 已发布镜像列表均未包含 `rlark-upgrade`。批量升级验收前，使用仓库的 `docker-upgrade-push` 构建并发布审核通过的镜像。官方升级 Job 会根据 `DeployConfig` 重建 cluster Deployment 和 node DaemonSet。带自定义挂载、启动参数或 embedded 拓扑的 Astribot 部署继续使用各自的更新流程，直到验证过升级和回滚能保留所需配置。上游 Kubernetes 工作负载路径仍处于预览阶段，Docker 和裸机任务执行尚未实现。

参考文档

- [上游集群升级](../admin-guide/upgrade.md)
- [节点管理](../admin-guide/nodes.md)
- [任务](../user-guide/jobs.md)
- [Agent](../admin-guide/agent.md)
