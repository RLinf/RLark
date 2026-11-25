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

三个证书字段都可填写内联 PEM 或已存在的文件路径。通过 `kubernetes.image` 设置非空 RLark tools/Sidecar 镜像；Kubernetes cluster 模式的 Task 装配会在启动时拒绝缺失镜像。该共享镜像字段也可作为组件镜像回退值，但仅设置 `kubernetes.agent-image` 不会提供 Task tools 镜像。安装前，请为每个需要运行节点 Agent 的 Node 添加标签：

```bash
kubectl label node <node-name> rlark.io/enabled=true
```

建议从仓库维护的示例开始：

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

多节点 Kubernetes 数据面不要在单个 Deployment 中混合 cluster 和 node 模式。`rlarkadm` 会创建正确的 Deployment、DaemonSet、证书 Secret、RBAC、Socket 挂载和容器运行时挂载。

手工监督本地进程时，`--mode=cluster --node-startup=embedded` 添加本机节点服务，不负责远程部署。提供两组角色依赖及 `--node-name`；cluster metrics 默认 `:8081`，embedded 节点健康/metrics/pprof 默认 `:8082`。两种角色共享证书派生的管理范围和本地 Kubernetes API，但保留独立的 cluster 与 `node-agent:<node-name>` tunnel role。节点启动不受 cluster 选主门控。任一角色致命错误会取消并等待整组退出（含 HTTP 关闭和节点 informer）；停止不删除 workload，也不释放 Task finalizer。embedded 健康只表示本机节点 readiness，不证明全部 cluster 控制器 Ready。不要在同机/socket 另行启动节点 Agent。旧组合模式已移除，默认仍为 external，managed 未实现。这不新增 HA/backend 身份保证，多副本仍须连接同一 Kubernetes 数据面。

Agent 支持仅启动时读取的 `--config`，以及 `check-config` 和脱敏 `print-config` 子命令。它们不接受 DeployConfig；见 [启动 schema、环境兼容与校验](../reference/configuration.md#agent-startup-file-and-validation)。非法初始配置在客户端/worker 前失败，修正后重启；MR 4.1 不支持文件热更新或 managed rollout。

正常启动在 30 秒 deadline 内检查本地 Kubernetes `/version`，node/embedded 还检查本地 Node 存在。随后启动受监督的 bootstrap tunnel，最多等待 30 秒完成 WebSocket 握手：Server 在接受升级前完成首次注册（管理 namespace、ServiceAccount 和 RBAC）。之后才在独立 30 秒 deadline 内检查管理 API：cluster 读取 Tasks，node/embedded 读取 DomainPeers/Pods（启用预拉取时还读取 Tasks）。Forbidden/NotFound 和 API 超时/不可用在期限内重试，允许注册权限传播；持续拒绝仍 fail-closed。全部角色完成 Build/Setup 后才启动业务 worker 与本地代理 HTTP 服务。embedded 在准备完成后添加节点角色 tunnel，不重复启动 bootstrap cluster tunnel。节点 informer 初次同步另有 30 秒 deadline。注册、preflight 或 Build 失败会取消并等待 bootstrap tunnel 退出，不回滚已注册的管理对象。恢复访问/配置并重启即可重新检查；这些检查不替代后续操作授权，也不证明 backend 身份。

关闭及监听启动失败时也关闭/join NodeServer 私有内存 HTTP 服务和 socket 取消 worker。关闭时，先停止私有 HTTP 连接接入、keepalive 连接和 hosts watch，再等待 workload 连接排空；并发调用 stop 是安全的。私有 dial 等待 HTTP Accept 接收，不再积累尚未被接收的缓冲连接。镜像观察器在 worker 停止后关闭进程拥有的 containerd 连接。现有连接 drain timeout 和 workload ownership 语义不变。

双向连接转发的 60 秒排空预算仅在某个复制方向结束后开始，不限制持续传输。如果连接拒绝设置读取 deadline（例如 SSH channel），到期只关闭参与转发的两个流连接，不关闭共享 SSH session 或其他 channel。如果两个方向提前结束，转发返回前会取消或等待 fallback timer 完成。这不改变 NodeServer 独立的 workload 排空预算，也不构成升级连续性保证。

Agent 内部代理 HTTP 监听也使用无缓冲 Pipe 接入，避免关闭时遗留尚未被接收的连接。已有 HTTP 关闭预算仍为三秒，优雅关闭超时后强制关闭连接；即使服务启动失败，也会等待 shutdown worker 退出。

NodeServer 私有 HTTP dial 等待接收的时间上限为 30 秒，与远端 dial 超时一致。该上限仅限制连接接入，不限制已建立的流量或 hosts watch。

SSH 服务使用的独立 `RelayStreams` helper 没有上述强制关闭兜底。流未实现读取 deadline 时，会输出包含复制方向的 verbosity-1 诊断日志；排空仍依赖可用的半关闭操作和对端完成。这条日志不提供关闭时限保证。

`rlarkadm` 为节点 Agent 分配独立的 `rlark-agent-node` ServiceAccount。本地集群 RBAC 仅允许读取当前 Node 以及列出和监听 Node Event，不会继承集群 Agent 的资源管理权限。

cluster 启动（external 或 embedded）还要求管理 `ClusterUpgradeJob` 读取权限。该依赖在 bootstrap 注册后、管理 preflight 期限内检查；持续拒绝或缺失 CRD 会阻止业务 worker 启动。node-only 不依赖此资源。升级控制器在 cluster Build 中安装，只随 manager 启动。

### Task Runtime 启动与关闭

通过管理 API 查看准入，例如 `kubectl --kubeconfig <management-kubeconfig> get tasks.rlinf.io <task-name> -n <management-namespace> -o yaml`。`status.conditions[type=ExecutionAdmitted]` 将永久性拒绝报告为 `False` / `ExecutionAdmissionFailed`，附原因说明和 observed generation。修正 Task 配置或恢复 Agent 所需能力后，相关变化和周期 resync 会重新检查准入，不走普通错误退避。准入通过将 Condition 改为 `True`，不代表 workload Ready。拒绝不改变 Phase，不销毁已有 workload/claim；删除和暂停仍按 identity 清理。完成暂停/reset 时移除准入 Condition。临时 API/transport 或状态发布失败仍是可重试错误。

装配后的 Runtime 在 startup 校验 inventory 声明/接口。正常 Task 执行在 lifecycle/workload 写入前校验编译/绑定需求；不支持的必需 Environment、unit、projection 或 profile 会 fail-closed，不认领新 Task。配置非法时不释放已有 claim。删除/暂停和 Node Drain 清理仍使用 ownership 已证明的 inventory，而不是编译当前 Spec。参见 [admission 与恢复边界](../developer-guide/runtime-provider.md#admission-conformance)。

Kubernetes cluster Agent 先构建完整的单 Runtime Task 执行组件，再安装 pull、observation 和 resync。依赖非法、观察 mapping 错误、观察权限拒绝或不确定时，manager 运行前就中止 startup。安装错误同样是致命错误：应修正配置并重启，而不是运行部分安装的 Agent。Build 的 metadata/权限 preflight 共用 30 秒 deadline，不启动 workload watch，也不写 workload。

共享管理 namespace 和 Agent Kind 的 Task 执行副本必须连接同一个 Kubernetes 数据面。不能在同一逻辑 Agent 范围下使用分别指向不同集群的 kubeconfig；backend 身份不会自动校验。Node Agent 仍是独立节点服务，不是各自认领 Task 的 owner。

关闭时，manager context 取消会停止并等待 Task observation worker 及其他 manager-owned 组件退出。Shutdown 本身不删除 Task workload，也不释放 finalizer；下一次 Agent 从管理 Task 和当前 Runtime 状态继续调谐。此次装配变化不新增 flag、Task schema 或 Docker/Raw workload 支持。

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

节点和集群视图会显示接入状态、从节点 Lease 推导的上次心跳时间，以及最近一次接入状态变化时间。Agent 每 15 秒续约一次心跳 Lease，默认 Lease 时长为 45 秒；节点 Lease 过期时，Node 会被标记为 `Offline`。重新连接的 Agent 会立即恢复续约，因此 Agent 重启完成后，Node 无需等待上一份 Lease 过期即可恢复在线。集群详情页会原样展示该集群的全部 Node，包括离线和非 Worker 节点；离线状态旁的信息按钮会提示上次在线时间。

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
| `--mode` | `cluster` | `cluster` 或 `node`；Kubernetes `rlarkadm` 使用分离工作负载，本机组合使用 `--node-startup=embedded` |
| `--agent-type` | `kubernetes` | 不区分大小写的启动 selector，映射不变的 API Kind；Docker/Raw 明确不支持 |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS 地址 |
| `--metrics-bind-address` | `:8081` | 指标监听地址 |
| `--image` | `""` | RLark tools/网络/SSH 镜像；Kubernetes cluster 模式的 Task 装配要求非空 |
| `--leader-election` | `false` | 保留的历史 flag；安装器可设置，但当前 Agent manager 未消费或执行选主门控 |
| `--task-runtime-resync-period` | `15m` | cluster 模式有界 Task Runtime 修复扫描的基础周期；实际间隔使用 80%–120% 抖动，`0` 表示禁用；cluster 模式下负值无效，node-only 模式接受这个共享 flag，但不会运行扫描器 |
| `--enable-cross-cluster-direct` | `true` | 尚未实现；跨集群流量始终走 SSH 中继 |
| `--containerd-socket` | `/run/containerd/containerd.sock` | 节点 Agent 的 Containerd Socket |

完整参数列表参见[配置项参考](../reference/configuration.md#rlark-agent)。

Runtime/Workload Coordinator 是 Deployment 和 StatefulSet Task 的生产写入者，负责执行有序 Runtime 操作。Agent 根据可信 workload-root registry 创建 Runtime push controller；这些 controller 只入队调谐 hint，通过验证的定向观测和 Task Status 更新均由 pull controller 执行。`--task-runtime-push-controller-workers` 配置每个已注册 root watch 的并发数。

Task root 和 secondary Pod watch 使用仅覆盖历史 workload namespace `rlark-system` 的独立 cache，不限制独立的集群级 Node/Pod 上报 controller。Observer 启动前通过 `SelfSubjectAccessReview` 检查 root 的 `get/list/watch`、instance profile 所需 Pod 的 `get/list/watch` 和 Deployment profile 所需 ReplicaSet 的 `get` 权限（preflight 超时 30 秒）。拒绝、不确定或 review 失败均导致启动中止，不能解释为空 inventory。凭据还必须允许创建 self-access review。启动失败时修复对应 RBAC 或 API 可用性后重启，不会启动部分 Task observer。Root 观察不需要 `/status` 写权限。经过 Task UID 校验的 root 删除会请求完整 pull 调谐；legacy metadata 缺少 UID 时依赖周期 resync。

**Worker flag 升级/回滚：**从部署参数中移除 `--task-deployment-push-controller-workers` 和 `--task-statefulset-push-controller-workers`，改为 `--task-runtime-push-controller-workers`（默认 `8`）。旧名称不是 alias，会被拒绝。若旧值不同，应明确选择共享并发预算；取较大值可保留原来的每个 root 上限，但可能提高总并发。回滚到尚不支持统一 flag 的 Agent 时，恢复其支持的两个旧 flag。不要同时运行新旧 Agent 作为 workload/Task Status writer。

Ray-on-Kubernetes Workload Adapter 支持 Deployment 和 StatefulSet carrier。

Cluster Agent 启动后会立即执行一次修复扫描，随后按配置的抖动周期重复。每轮分页扫描分配给同一 Host 或符合该 Host 条件的 management Task，预算上限为 10,000 个 Task，并产生完整调谐 hint；扫描本身不直接修改 Task 或 Runtime 对象。Management API 的临时 List 失败会记录日志并在后续轮次重试，不会停止 Agent；trigger bridge 背压会按轮汇总，但不会阻止扫描继续访问后续 Task 和分页。禁用扫描会移除 watch event 丢失后的这一恢复路径，只有在其他运维机制能够保证最终收敛时才应这样做。

升级 Agent 时，Coordinator 可以识别旧 Task controller 创建、但尚未携带 Task UID Annotation 的主工作负载。它只会接管名称相同且现有元数据与 Task 不冲突的 Deployment 或 StatefulSet；该兼容规则不会扩展到任意辅助资源。首次接管以及 workload 仍处于该 legacy adopted generation 时的后续调谐只更新 identity 和 revision metadata，保留运行中的 Pod 模板，避免升级本身触发 rollout。后续 Task generation 将 workload 建立为 canonical-owned 后，即使当前 Task generation 未变化，完整调谐也会修复 Agent 管理的 replicas 和 Pod-template 字段的带外漂移。

不要通过修改 `spec.runtime.kind` 或 `spec.runtime.name` 迁移已经 claim 的 Task。Agent 会冻结已 claim Runtime：这些字段变化时保留旧 workload，不应用该 generation 中的其他 Spec 变化，并等待恢复原值，或者等待 Task 删除后重建。已 claim 且未删除的 Task 出现解码、校验或 implementation 注册失败时，也会保留 claim 和现有 workload，而不会执行 cleanup。删除仍按已 claim Runtime 和 Task identity 执行 cleanup。升级期间，对于带 cleanup finalizer 但没有 claimant annotation 的 legacy Task，只有 Task 正在删除或者当前 Agent 能够执行时才会接管；未删除且 selector 指向其他 Agent 的 Task 会保持不动。完整 ownership 契约参见 [Runtime、Workload 与 Environment 边界](../developer-guide/runtime-provider.md)。

上层业务不应把修改已claim Kubernetes Task的`spec.workload.kind`作为常规工作流；更换carrier kind时应以新UID替换Task，或使用Job restart/replacement流程。如果直接修改仍然到达Agent，才会触发Deployment和StatefulSet之间的窄化、可恢复兼容路径：Agent先清理并确认旧carrier及旧selector下所有`DeleteOnTaskCleanup`辅助对象均已消失，其中包括Ray head Service，同时保留共享Ray ConfigMap；引用的外部PVC不属于RLark ownership。随后切换claim中冻结的Workload selector，并在后续调谐创建新carrier。ownership冲突或证据不完整会阻止迁移。该fallback可能中断workload，既不是跨Runtime迁移，也不提供事务回滚；它用于避免资源泄漏和claim metadata失真，不能替代Task replacement。

Task 删除、停止和重启会保持等待，直到确认所有该 Task 独占且标记为 `DeleteOnTaskCleanup` 的对象均已消失。这包括主 Deployment 或 StatefulSet，以及 Ray head Task 由UID标识的head Service。共享Ray启动ConfigMap会被保留；引用的外部PVC不进入cleanup inventory。快照或删除失败时会保留Task finalizer，以便后续安全恢复。

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
