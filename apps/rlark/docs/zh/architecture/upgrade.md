# 集群升级架构

## 概述

RLark 支持跨多种部署形态（Kubernetes、Docker、裸机）对数据面集群进行批量升级。升级采用两级 CRD 设计（`UpgradeJob` 和 `ClusterUpgradeJob`）：控制面负责创建和监控升级任务，数据面 Agent 执行实际的升级操作。

## 核心概念

| 概念 | 说明 |
|------|------|
| UpgradeJob | 控制面 CRD，定义升级任务（镜像、目标集群、策略） |
| ClusterUpgradeJob | 每个目标集群对应的子任务，由数据面 Agent 执行 |
| StateStore | 集群状态存储抽象（镜像 + 配置）。实现：`ConfigMapStateStore`（K8s）、`FileStateStore`（Docker/裸机） |
| UpgradeDriver | 升级执行抽象。实现：`K8sDriver`（K8s Job）、`DockerDriver`（容器）、`RawDriver`（systemd） |
| rlark-upgrade | 升级工具镜像/二进制，执行实际升级 |

## 部署形态支持

| 形态 | StateStore | UpgradeDriver | 升级执行器 |
|------|-----------|---------------|-----------|
| Kubernetes | rlark-system 命名空间的 ConfigMap | K8sDriver 创建 batch/v1.Job | Job Pod 运行 rlark-upgrade |
| Docker | 主机文件（如 /var/lib/rlark/state.yaml） | DockerDriver 运行容器 | 容器运行 rlark-upgrade |
| 裸机 | 主机文件（如 /etc/rlark/state.yaml） | RawDriver 运行 systemd 服务 | systemd 服务运行 rlark-upgrade |

**注意**：Docker 和裸机模式尚未完全实现。架构通过 StateStore 和 UpgradeDriver 接口支持这些模式。

## 架构

```
控制面 (kcp)                        数据面集群                     升级执行器
  │                                    │                              │
  │─ UpgradeJob CRD                    │                              │
  │  (spec.image, clusterNames)        │                              │
  │                                    │                              │
  │─ ClusterUpgradeJob CRD             │                              │
  │  (per-cluster, namespaced)         │                              │
  │                                    │                              │
  │                                    │◄── Agent watch ──────────────│
  │                                    │    ClusterUpgradeJob         │
  │                                    │                              │
  │                                    │── Driver.StartUpgrade() ────►│
  │                                    │    (K8s Job / Docker / Raw)  │
  │                                    │                              │
  │                                    │                              │── Pre-check
  │                                    │                              │── Upgrade components
  │                                    │                              │── Health check
  │                                    │                              │── Rollback on failure
  │                                    │                              │── Exit (0/1/2)
  │                                    │                              │
  │                                    │── Driver.CheckStatus() ─────►│
  │                                    │    StateStore.Save()         │
  │                                    │    Update status ───────────►│
  │◄── Aggregate status ───────────────│                              │
```

## 升级流程

### 阶段 1：创建 UpgradeJob

管理员通过前端或 API 创建 `UpgradeJob`，指定：
- **image**:rlark-upgrade 镜像（如 `registry/rlark-upgrade:v0.0.10`）
- **clusterNames**：目标集群列表
- **configPatch**：部署配置覆盖（可选）
- **strategy**：并发数、失败容忍、超时、自动回滚

### 阶段 2：控制面创建 ClusterUpgradeJob

`UpgradeJob` Controller 根据 `clusterNames` 为每个集群创建 `ClusterUpgradeJob`：
- 命名空间为集群名（如 `rlark-ningxia-data`）
- 受 `strategy.concurrency` 限制并发创建数量
- `MaxConcurrentReconciles = 1`，无并发竞争问题

### 阶段 3：数据面 Agent 执行升级

数据面 Agent 监听本命名空间的 `ClusterUpgradeJob`：

1. **读取状态**：从 `StateStore` 读取当前镜像和配置（作为回滚目标）
2. **准备配置**：`configPatch` 为空则用当前配置，否则用新的配置
3. **启动升级**：调用 `UpgradeDriver.StartUpgrade()` 创建升级执行器
4. **监控**：轮询 `UpgradeDriver.CheckStatus()` 获取完成状态和 exit code

### 阶段 4：升级结果处理

根据升级执行器的 exit code 处理结果：

| Exit Code | 含义 | 处理 |
|-----------|------|------|
| 0 | 升级成功 | `StateStore.Save()` 新状态，标记 `Succeeded` |
| 2 | 升级失败，回滚成功 | 不更新状态，标记 `RolledBack` |
| 其他 | 升级失败，回滚失败 | 不更新状态，标记 `Failed` |

## StateStore 接口

`StateStore` 接口抽象集群状态的持久化方式：

```go
type StateStore interface {
    // Get 返回当前 ClusterState。
    // 不存在时返回匹配 apierrors.IsNotFound 的错误。
    Get(ctx context.Context) (*ClusterState, error)

    // Save 持久化 ClusterState。
    Save(ctx context.Context, state *ClusterState) error
}

type ClusterState struct {
    Image        string  // 当前 rlark-upgrade 镜像
    DeployConfig string  // 完整部署配置 YAML
}
```

### 实现

| 实现 | 存储 | 适用场景 |
|------|------|----------|
| ConfigMapStateStore | K8s ConfigMap `rlark-state`（rlark-system 命名空间） | Kubernetes 数据面 |
| FileStateStore | 本地文件（如 `/var/lib/rlark/state.yaml`） | Docker、裸机 |

## UpgradeDriver 接口

`UpgradeDriver` 接口抽象升级的执行方式：

```go
type UpgradeDriver interface {
    // StateStore 返回该 driver 使用的 StateStore。
    StateStore() StateStore

    // PrepareConfig 持久化本次升级的配置。
    PrepareConfig(ctx context.Context, job *ClusterUpgradeJob, config string) (configRef string, err error)

    // StartUpgrade 启动升级工作负载。
    StartUpgrade(ctx context.Context, job *ClusterUpgradeJob, rollbackImage, configRef string) (jobName string, err error)

    // CheckStatus 返回升级的当前状态。
    CheckStatus(ctx context.Context, jobName string) (JobStatus, error)
}
```

### 实现

| 实现 | 执行器 | 适用场景 |
|------|--------|----------|
| K8sDriver | `batch/v1.Job` 运行 rlark-upgrade 镜像 | Kubernetes 数据面 |
| DockerDriver | Docker 容器运行 rlark-upgrade | Docker 数据面 |
| RawDriver | systemd 服务运行 rlark-upgrade | 裸机数据面 |

## 自动回滚

当 `strategy.autoRollback` 为 true 时，升级执行器内部处理回滚：

1. 升级失败时，从 `StateStore` 读取回滚镜像和配置
2. 使用旧版本重新执行升级（即回滚）
3. 回滚成功 → exit code 2
4. 回滚失败 → exit code 1 或其他非零值

**注意**：回滚由升级执行器内部处理，不是由 Agent 创建新的回滚任务。这是因为升级过程中 Agent 自身可能不健康。

## 状态机

### ClusterUpgradeJob

```
Pending → Upgrading → Succeeded
                     ↘ Failed
                     ↘ RolledBack
```

| 状态 | 说明 |
|------|------|
| Pending | 等待执行 |
| Upgrading | 升级中 |
| Succeeded | 升级成功 |
| RolledBack | 升级失败但回滚成功 |
| Failed | 升级失败且回滚失败 |

### UpgradeJob

```
Pending → Upgrading → Succeeded
                     ↘ Failed
                     ↘ RollingBack（表示部分集群 RolledBack）
```

多集群时，整体状态取最"严重"的级别：Failed > RollingBack > Succeeded。

## 配置管理

### 状态持久化

每个数据面集群通过 `StateStore` 持久化状态：

**Kubernetes (ConfigMapStateStore)**：
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rlark-state
  namespace: rlark-system
data:
  image: "registry/rlark-upgrade:v0.0.10"
  deploy-config: |
    apiVersion: rlark.io/v1alpha1
    kind: DeployConfig
    ...
```

**Docker/裸机 (FileStateStore)**：
```yaml
# /var/lib/rlark/state.yaml
image: "registry/rlark-upgrade:v0.0.10"
deployConfig: |
  apiVersion: rlark.io/v1alpha1
  kind: DeployConfig
  ...
```

- `image`：当前 rlark-upgrade 镜像
- `deploy-config`：完整部署配置

### 配置覆盖

用户可以在 `UpgradeJob.spec.configPatch` 中提供新的配置：

```yaml
spec:
  configPatch: |
    apiVersion: rlark.io/v1alpha1
    kind: DeployConfig
    plane: data
    kubernetes:
      agent-image: registry/rlark:v0.0.67
```

- `configPatch` 非空：作为新配置使用
- `configPatch` 为空：使用 `StateStore` 中的当前配置

## 并发控制

### 控制面并发

- `MaxConcurrentReconciles = 1`：同一时间只处理一个 `UpgradeJob`
- `strategy.concurrency`：限制同时创建的 `ClusterUpgradeJob` 数量

### 数据面并发

- 每个 `ClusterUpgradeJob` 对应一个独立的升级执行器
- 单集群同时只有一个升级在进行（通过 `ClusterUpgradeJob` 状态判断）

## CRD 定义

### UpgradeJob

```yaml
apiVersion: rlinf.io/v1alpha1
kind: UpgradeJob
metadata:
  name: up-xxxxx
  annotations:
    rlark.io/description: "升级到 v0.0.67"
spec:
  image: registry/rlark-upgrade:v0.0.10
  clusterNames: ["cluster-1", "cluster-2"]
  configPatch: "..."
  strategy:
    concurrency: 2
    failureTolerate: 0
    timeoutSeconds: 600
    autoRollback: true
status:
  phase: Succeeded
  totalClusters: 2
  upgradedClusters: 2
  conditions:
    - cluster: cluster-1
      status: Succeeded
      targetImage: registry/rlark-upgrade:v0.0.10
```

### ClusterUpgradeJob

```yaml
apiVersion: rlinf.io/v1alpha1
kind: ClusterUpgradeJob
metadata:
  name: up-xxxxx-cluster-1
  namespace: cluster-1
spec:
  upgradeJobName: up-xxxxx
  image: registry/rlark-upgrade:v0.0.10
  configPatch: "..."
  timeoutSeconds: 600
  autoRollback: true
status:
  phase: Succeeded
  jobName: upgrade-up-xxxxx-cluster-1-1234567890
  startTime: "2026-11-16T08:38:32Z"
  completionTime: "2026-11-16T08:39:09Z"
```

## 升级执行器

升级执行器（Job Pod、容器或 systemd 服务）运行 `rlark-upgrade`：

1. **Pre-check**：`rlarkadm` 检查集群状态
2. **Upgrade**：`rlarkadm upgrade` 更新组件镜像
3. **Health Check**：`rlark-upgrade` 深度健康检查（连续 3 次通过）
4. **Rollback**：失败时使用 `rlarkadm rollback` 回滚

### 配置

**环境变量**：
- `TARGET_IMAGE`：目标 rlark-upgrade 镜像
- `ROLLBACK_IMAGE`：回滚镜像（从 StateStore 读取）
- `AUTO_ROLLBACK`：是否启用自动回滚

**挂载文件**：
- `deploy-config`：新的部署配置
- `rollback-config`：旧的部署配置（从 StateStore 读取）

## 前端界面

管理后台提供升级任务管理界面：

- **任务列表**：显示所有升级任务，支持排序、筛选、搜索
- **创建任务**：表单填写镜像、集群、策略、配置覆盖
- **任务详情**：显示进度、集群明细、配置
- **复制创建**：基于已有任务快速创建

## 参考

- [管理员指南 - 集群升级](../admin-guide/upgrade.md)
- [rlarkadm CLI 参考](../reference/cli.md#rlarkadm)
