# Agent 安装与升级

## 安装

Agent 运行在每个数据面集群中。`rlarkadm` 创建的 Kubernetes 部署包括：

- `rlark-agent` Deployment，以 `--mode=cluster` 运行，负责集群级同步。
- `rlark-agent-node` DaemonSet，以 `--mode=node` 运行，负责节点网络和镜像预拉取。它只会调度到带有 `rlark.io/enabled=true` 标签的 Node，且仅这些 Node 会作为活动节点上报到管理面。RLark 创建的 Kubernetes Task 工作负载也强制要求该标签，因此用户提供的节点选择器不能将 Task 调度到其他恰好匹配的 Kubernetes Node。后续移除该标签或本地 Node 消失时，cluster Agent 会保留管理面的 Node 记录并将其标记为 `Offline`。

### 前置条件

- 具有 `kubectl` 访问权限的 Kubernetes 集群。
- 能够出方向访问 Server HTTPS/WSS 端口，通常为 8443。
- 为该集群签发的 CA 证书、Agent 证书和 Agent 私钥。
- Server TLS 证书包含实际使用的主机名或 IP。

### 使用 rlarkadm 安装（推荐）

项目中没有 `rlarkagent` 命令。把管理控制台或 Gateway API 返回的证书值写入 `DeployConfig`，然后执行 `rlarkadm install`：

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://rlark.example.com:8443
cert:
  ca-cert: /path/to/ca-cert.pem
  agent-cert: /path/to/agent-cert.pem
  agent-key: /path/to/agent-key.pem
kubernetes:
  kubeconfig: ~/.kube/config
  agent-image: rlark:latest
  image: rlark:latest
  # containerd-socket: /run/k3s/containerd/containerd.sock
```

三个证书字段都可填写内联 PEM 或已存在的文件路径。`kubernetes.image` 可选；设置后启用网络 Sidecar 和 SSH 支持。安装前，请为每个需要运行节点 Agent 的 Node 添加标签：

```bash
kubectl label node <node-name> rlark.io/enabled=true
```

建议从仓库维护的示例开始：

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

多节点 Kubernetes 数据面不要在单个 Deployment 中混合 cluster 和 node 模式。`rlarkadm` 会创建正确的 Deployment、DaemonSet、证书 Secret、RBAC、Socket 挂载和容器运行时挂载。

`rlarkadm` 为节点 Agent 分配独立的 `rlark-agent-node` ServiceAccount。本地集群 RBAC 仅允许读取当前 Node 以及列出和监听 Node Event，不会继承集群 Agent 的资源管理权限。

### 验证

```bash
kubectl get deployment/rlark-agent daemonset/rlark-agent-node -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system

kubectl logs -n rlark-system deployment/rlark-agent --tail=100
kubectl logs -n rlark-system daemonset/rlark-agent-node --tail=100

# 通过 UI 到 Gateway 的代理验证注册资源。
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80
curl --fail http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes
```

8443 是 Server 隧道和代理流量端口，不是 Gateway REST 端点。Agent 在 `:8081` 暴露指标，但没有专用 HTTP 健康路由，因此应检查 rollout 状态、日志和资源注册。

### 接入状态与心跳

Server 会在已注册集群的命名空间中维护集群 Agent 的 `heartbeat` Lease，用于集群连通性。每个节点 Agent 会在同一命名空间维护专属的 `node-heartbeat-<node-name>` Lease。Node controller 仅以该节点专属 Lease 作为节点状态的权威来源：续约有效时标记为在线，Lease 超时后以 `AgentLeaseExpired` 原因标记为离线。

已注册 Node 缺少专属 Lease 时，会以 `NodeAgentLeaseMissing` 原因标记为 `Offline`；集群 Agent 心跳不能将此类 Node 标记为在线。因此，即使 cluster Agent 仍保持连接，Node 也必须运行 `rlark-agent-node` 并保持有效隧道才会显示在线。

节点和集群视图会显示接入状态、从节点 Lease 推导的上次心跳时间，以及最近一次接入状态变化时间。Agent 每 15 秒续约一次心跳 Lease，默认 Lease 时长为 45 秒；节点 Lease 过期时，Node 会被标记为 `Offline`。集群详情页会原样展示该集群的全部 Node，包括离线和非 Worker 节点；离线状态旁的信息按钮会提示上次在线时间。

### 验证清单

| 检查项 | 预期结果 |
|--------|----------|
| 集群 Agent | `rlark-agent` Deployment 可用 |
| 节点 Agent | `rlark-agent-node` 的期望和就绪数量一致 |
| 连接 | 日志显示成功连接 Server，且没有反复出现 TLS 错误 |
| 集群状态 | 管理后台显示在线 |
| 节点注册 | Worker 节点显示预期标签和容量 |
| Task 创建 | 测试 Job 能创建并运行 Task 工作负载 |
| 网络 | 配置 `kubernetes.image` 后，网络 Sidecar 和 SSH 正常 |

## 升级

查看[发布说明](../reference/changelog.md)，备份清单和证书，并先在非关键集群测试。两个 Agent 工作负载应与控制面使用同一发行版本：

```bash
kubectl set image deployment/rlark-agent agent=<new-image> -n rlark-system
kubectl set image daemonset/rlark-agent-node agent=<new-image> -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system
```

也可以修改部署配置中的 `agent-image`，再执行 `rlarkadm install -f deploy-data-plane.yaml`。项目中没有 `rlarkagent upgrade` 命令。

### Cluster 资源迁移

控制面和 Agent 升级到包含 `Cluster` CRD 的版本后，原有集群的 cluster Agent 在下一次成功连接 Server 时，Server 会创建一个集群作用域的 `Cluster` 对象。对象名称和 `spec.agentNamespace` 均保留已有的 `rlark-<cluster-id>` 值。`spec.agentNamespace` 是该 Agent 心跳 Lease 与 Node CR 的权威管理位置；Controller 仅聚合此命名空间中的 Node，不再通过可变标签筛选归属。在 Agent 注册期间，Server 会为 `spec.agentNamespace` 中的 Node 统一更新兼容性查询用的 `rlark.io/cluster-id` 标签，以保留旧集群 ID；不会重命名或复制 Node CR 与心跳 Lease。

迁移具备幂等性：Agent 重连不会覆盖已存在 `Cluster` 的 Spec，Agent 命名空间中已有的 Node 会继续归属该 Cluster。Agent 只上报 Kubernetes 派生的节点容量和健康字段；控制面通过 node Agent Lease 独占写入 Node 连通性 Phase、心跳时间和 `Ready` Condition。应先升级控制面，再升级数据面 Agent；待所有旧集群 Agent 重新连接后，使用以下命令确认迁移完成：

```bash
kubectl get clusters.rlinf.io
kubectl get cluster <cluster-id> -o jsonpath='{.spec.agentNamespace}{"\n"}'
```

不会猜测或合成尚未重连的旧集群。请保留其 Agent 凭据和配置，恢复连通性，并等待下一次成功连接 Server 后再将迁移视为完成。

## 配置参考

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--mode` | `cluster` | `cluster`、`node` 或 `both`；Kubernetes `rlarkadm` 使用分离的 cluster/node 工作负载 |
| `--agent-type` | `Kubernetes` | Agent 类型 selector；只有 Kubernetes 具备 workload 执行路径，Docker 和 Raw 仍是骨架，不能执行 Task |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS 地址 |
| `--metrics-bind-address` | `:8081` | 指标监听地址 |
| `--image` | `""` | 网络 Sidecar 和 SSH 镜像 |
| `--leader-election` | `false` | Leader Election；需要多副本时 `rlarkadm` 为集群 Agent 启用 |
| `--task-runtime-resync-period` | `15m` | cluster 模式有界 Task Runtime 修复扫描的基础周期；实际间隔使用 80%–120% 抖动，`0` 表示禁用；`cluster`/`both` 模式下负值无效，node-only 模式接受这个共享 flag，但不会运行扫描器 |
| `--enable-cross-cluster-direct` | `true` | 允许跨集群 Pod 直连路由 |
| `--containerd-socket` | `/run/containerd/containerd.sock` | 节点 Agent 的 Containerd Socket |

完整参数列表参见[配置项参考](../reference/configuration.md#rlark-agent)。

Runtime/Workload Coordinator 现在是 Deployment、DaemonSet 和 StatefulSet Task 的生产写入者，负责执行有序 Runtime 操作。Runtime push controller 只入队调谐 hint；通过验证的定向观测和 Task Status 更新均由 pull controller 执行。

Cluster Agent 启动后会立即执行一次修复扫描，随后按配置的抖动周期重复。每轮分页扫描分配给同一 Host 或符合该 Host 条件的 management Task，预算上限为 10,000 个 Task，并产生完整调谐 hint；扫描本身不直接修改 Task 或 Runtime 对象。Management API 的临时 List 失败会记录日志并在后续轮次重试，不会停止 Agent；trigger bridge 背压会按轮汇总，但不会阻止扫描继续访问后续 Task 和分页。禁用扫描会移除 watch event 丢失后的这一恢复路径，只有在其他运维机制能够保证最终收敛时才应这样做。

升级 Agent 时，Coordinator 可以识别旧 Task controller 创建、但尚未携带 Task UID Annotation 的主工作负载。它只会接管名称相同且现有元数据与 Task 不冲突的 Deployment、DaemonSet 或 StatefulSet；该兼容规则不会扩展到任意辅助资源。首次接管以及 workload 仍处于该 legacy adopted generation 时的后续调谐只更新 identity 和 revision metadata，保留运行中的 Pod 模板，避免升级本身触发 rollout。后续 Task generation 将 workload 建立为 canonical-owned 后，即使当前 Task generation 未变化，完整调谐也会修复 Agent 管理的 replicas 和 Pod-template 字段的带外漂移。

不要通过修改 `spec.runtime.kind` 或 `spec.runtime.name` 迁移已经 claim 的 Task。Agent 会冻结已 claim Runtime：这些字段变化时保留旧 workload，不应用该 generation 中的其他 Spec 变化，并等待恢复原值，或者等待 Task 删除后重建。已 claim 且未删除的 Task 出现解码、校验或 implementation 注册失败时，也会保留 claim 和现有 workload，而不会执行 cleanup。删除仍按已 claim Runtime 和 Task identity 执行 cleanup。升级期间，对于带 cleanup finalizer 但没有 claimant annotation 的 legacy Task，只有 Task 正在删除或者当前 Agent 能够执行时才会接管；未删除且 selector 指向其他 Agent 的 Task 会保持不动。完整 ownership 契约参见 [Runtime、Workload 与 Environment 边界](../developer-guide/runtime-provider.md)。

上层业务不应把修改已claim Kubernetes Task的`spec.workload.kind`作为常规工作流；更换carrier kind时应以新UID替换Task，或使用Job restart/replacement流程。如果直接修改仍然到达Agent，才会触发Deployment、DaemonSet和StatefulSet之间的窄化、可恢复兼容路径：Agent先清理并确认旧carrier及旧selector下所有`DeleteOnTaskCleanup`辅助对象均已消失，其中包括Ray head Service，同时保留共享Ray ConfigMap；引用的外部PVC不属于RLark ownership。随后切换claim中冻结的Workload selector，并在后续调谐创建新carrier。ownership冲突或证据不完整会阻止迁移。该fallback可能中断workload，既不是跨Runtime迁移，也不提供事务回滚；它用于避免资源泄漏和claim metadata失真，不能替代Task replacement。

Task 删除、停止和重启会保持等待，直到确认所有该 Task 独占且标记为 `DeleteOnTaskCleanup` 的对象均已消失。这包括主 Deployment、DaemonSet 或 StatefulSet，以及 Ray head Task 由UID标识的head Service。共享Ray启动ConfigMap会被保留；引用的外部PVC不进入cleanup inventory。快照或删除失败时会保留Task finalizer，以便后续安全恢复。

当RLark拥有的selector、port、Service type、`publishNotReadyAddresses`或标准metadata发生漂移时，Ray head Service使用Replace语义。Agent会按UID校验并删除当前Service，确认对象消失后在后续reconcile中重建；这可能短暂中断访问，也可能分配不同的Service地址或端口。Kubernetes物化的默认值和分配字段，包括ClusterIP、IP family字段、NodePort、默认protocol和target port，以及无关metadata，不属于漂移投影，不会单独触发Replace。

删除 Job 时，不带 Agent cleanup finalizer 的 owned Task 从未被 claim，因此没有 Agent-created Runtime effect 需要停止。Job controller 会直接删除该 Task，而不是等待其 Status 变为 `Stopped`；因此 unsupported Pending Task 可以随 Job 一起消失。携带 Agent cleanup finalizer 的 Task 仍遵循正常的 stop 和 Agent cleanup 协议。

Cleanup 使用此前已认领的Task身份和Runtime对象快照，不要求当前Workload Spec仍可解码。因此，已认领的Task即使变为不支持的形态或丢失Kubernetes Workload Spec，仍可删除能够证明归属的主工作负载和Ray head Service并释放finalizer，同时保留共享Ray ConfigMap；引用的外部PVC保持不变。工作负载状态事件使用针对变更工作负载及其Pod的定向观测，不再执行全量受管资源扫描。

每次成功的 Task pull reconciliation 都会输出一条结构化 `Task reconciliation completed` 摘要，包含 `task`、`outcome`、`retryClass`、实际生效的 `retryAfter`，以及 `RuntimeSelectorChanged`、`WorkloadSelectorChanged`、`CarrierSelectorChanged`、`ClaimedByOtherHost` 等稳定 `reason`。对于 outcome、retry class 和 reason 均未变化的两秒一次 in-progress 调谐，摘要最多每分钟输出一次；reason 或 outcome 变化时立即输出。失败的调谐会将错误返回 controller-runtime，不再输出这条摘要，避免重复错误日志。

### 旧版 PVC 升级提示

Agent 不再根据 Ray Task 声明创建、收养、校验、盘点或删除 PVC。旧版 Agent 已创建的 claim 在升级时保持不变；移除旧 metadata 或删除 claim 前应先盘点：

```bash
# 携带旧 Task 标签的 legacy 兼容 PVC：
kubectl get pvc -n rlark-system -l rlark.io/task
kubectl get pvc -n rlark-system -l rlark.io/task=<task-name> \
  -o custom-columns=NAME:.metadata.name,TASK:.metadata.labels.rlark\.io/task,OWNER:.metadata.annotations.rlark\.io/pvc-owner-task,STATUS:.status.phase,CAPACITY:.status.capacity.storage

# 手工盘点时同时展示仅有旧 annotation 的 PVC：
kubectl get pvc -n rlark-system \
  -o custom-columns=NAME:.metadata.name,TASK_LABEL:.metadata.labels.rlark\.io/task,OWNER_ANNOTATION:.metadata.annotations.rlark\.io/pvc-owner-task,STATUS:.status.phase,CAPACITY:.status.capacity.storage
```

带label selector的命令无法找到仅携带`rlark.io/pvc-owner-task`的旧PVC，这类claim需要通过namespace盘点命令检查。新的`persistentVolumeClaim.claimName` volume仅用于引用，要求claim已存在于workload namespace。RLark不会根据旧label或annotation推断ownership。删除legacy或外部管理的PVC前，必须检查所有workload引用及绑定PV的reclaim policy。由`ephemeral.volumeClaimTemplate`创建的Pod级PVC仍由Kubernetes管理并随Pod删除。

共享Ray bootstrap ConfigMap同样会跨Task cleanup保留，其固定名称由RLark保留。Agent会保留无关metadata，但当`data`、`binaryData`或Ray脚本身份annotation与内置脚本不一致时进行修复。不要编辑这些脚本字段，也不要让其他controller管理它们。Ray角色ownership metadata冲突仍会返回Conflict，需要运维人员调查。
