# 节点与资源管理

## 管理节点调度

管理员可通过切换节点的调度状态来控制节点是否接受新工作负载。

### 停止调度（Cordon）

- PATCH Node 资源：设置 `spec.unschedulable: true`
- 节点上已有的运行中 Worker 不受影响
- 新任务不会被调度到此节点
- 在维护、升级或节点异常时使用

### 恢复调度（Uncordon）

- PATCH Node 资源：设置 `spec.unschedulable: false`
- 节点恢复接受新工作负载
- 恢复调度前请确认节点健康

## 节点标签

- 管理员可为节点添加自定义标签以进行分类管理
- 用户在任务配置中通过 nodeSelector 使用标签
- 常用标签：`rlark.io/node-category`（cloud/edge/robot）、GPU 型号、位置

| 标签 | 可选值 | 说明 |
|------|--------|------|
| `rlark.io/node-category` | `cloud`, `edge`, `robot`, `other` | 节点类型 |
| `rlark.io/gpu-model` | `A100`, `V100`, `T4` 等 | GPU 型号（云端节点） |
| `rlark.io/device-model` | `franka`, `ur5`, `realsense` 等 | 具身设备型号 |
| `rlark.io/city` | 任意 | 物理位置（注解） |

## 查看节点资源

打开管理后台 → 节点，查看和管理节点：

> **截图说明：** 截图来自示例环境，资源名称和数据仅供说明，实际环境会有所不同。

![节点详情](../../images/ui/first-login-node-detail.png)

节点详情包括：
- 调度状态（可调度 / 已停止调度）
- 节点类型、接入形态、操作系统、架构
- Agent 版本
- 资源使用情况：CPU、内存、磁盘、GPU 和具身设备资源
- 磁盘使用率达到 90% 或 kubelet 上报 `DiskPressure=True` 时显示告警
- 节点上运行中的关联任务

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

确认弹窗会列出将停止的任务以及受影响的场站节点记录。删除场站节点记录是另一个操作，只在该节点已移除、处于 `Conflict`，或集群里已经没有同名 Node 时可用。

## 通过 UI 操作

管理后台 → 节点 → 选择节点 → 切换调度状态或编辑标签。

## 通过 API 操作

```bash
# 停止调度
kubectl patch node <name> --type merge -p '{"spec":{"unschedulable":true}}'

# 恢复调度
kubectl patch node <name> --type merge -p '{"spec":{"unschedulable":false}}'

# 按类别列出节点
kubectl get nodes -l rlark.io/node-category=cloud
```