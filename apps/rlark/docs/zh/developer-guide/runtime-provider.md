# Runtime、Workload 与 Environment 边界

本文描述 RLark 管理面 Task 生命周期、Workload 语义、Environment 需求和 Runtime 对象操作之间的内部执行边界。它是实现说明，不是公开 API，也不能作为某种 Runtime 已受支持的依据。

## 兼容范围

Kubernetes 是唯一受支持的 workload 运行时。Docker 和 Raw 字段只是 API 骨架，不能执行 workload。该执行边界属于内部实现，不定义 CRD、生成客户端、REST 接口、CLI 参数、配置、Status Schema、Finalizer、Annotation key 或生成的 Kubernetes 对象结构。

Kubernetes 兼容契约包括固定 workload namespace、legacy workload 收养、claimant identity 格式、原地模板修改、SSH key 注入、跨 Domain 网络 sidecar 注入和清理顺序。Task 被 claim 时会冻结 workload 与 carrier selector；已 claim Task 修改任一 selector 都会在不产生 Runtime effect 的情况下被拒绝，调用方必须删除并以新 UID 重建 Task。存储采用原生 PodTemplate 语义：`persistentVolumeClaim.claimName` 引用已有外部 PVC，而 `ephemeral.volumeClaimTemplate` 创建的 PVC 由 Kubernetes 随 Pod 管理。

Host 将已 claim Runtime 视为不可变。已 claim Task 的 `spec.runtime.kind` 或 `spec.runtime.name` 发生变化时，Host 保持已 claim Runtime 不变，不触碰新 selector 指向的 Runtime，报告 `WaitingForChange`，并要求恢复原 selector，或者删除并重建 Task。同一个非法 generation 中的其他 Spec 变化也不会被部分应用。Task 删除时按已 claim Runtime 和 Task identity 执行 cleanup。该规则避免一次意外 selector 修改被解释为隐式、破坏性的 Runtime 迁移。

只有 Task 正在删除，或者当前 Agent 能够执行该 Task 时，Host 才会接管没有 claimant annotation 的 legacy cleanup finalizer。对于未删除且 selector 指向其他 Agent 的 Task，Host 保持对象不动：不补 claimant metadata、不写 Status、不调用 Executor，也不清理 Runtime。这样可以防止模糊的 legacy metadata 被解释为跨 Agent cleanup 授权。

## 实现布局

Runtime-neutral 合同与编排统一位于 `pkg/agent/execution`：

- `plan` 负责 portable compiled Plan、显式 Environment mutation target 和独立 workload-unit 声明；
- `workload` 负责 Workload Adapter 合同与选择；
- `environment` 负责 requirement、resolved binding、可信 Handler 注册、material lease 和标准化的非秘密 binding revision；
- `binding` 负责 unit 投影、在私有 Plan 副本上应用 Environment Handler、调用 Runtime lowering 和最终校验；
- `runtime` 负责 Runtime object、instance、candidate inventory、profile selection 和 unit-lowering 合同；
- `coordination` 与 `coordination/coordinator` 负责 Runtime-neutral 选择、diff、排序和执行编排；
- `host` 负责管理面 Task claim、cleanup finalizer、生命周期分派，以及数据面 Agent 拥有的 Task Status 字段；
- `runtime/transport/grpc` 与 `workload/transport/grpc` 子目录负责在内部接口和版本化 execution 协议之间进行适配。

具体 Task API compatibility 与 Kubernetes 实现归属 `pkg/agent/controllers/task`，不属于中立 framework：

```text
controllers/task/compatibility/kubernetes/
├── task.go             # 当前 Task API decoder
└── kubernetesobjects/  # 旧 metadata、ownership 和 profile ID/projection

controllers/task/workload/
└── ray/kubernetes/   # 一个面向 Ray、支持三种 carrier kind 的 Adapter

controllers/task/runtime/kubernetes/              # Provider、profile executor/registry、lowering
  supervision/                                    # 强制 tools 安装与进程监督
controllers/task/environment/kubernetes/          # Kubernetes Environment 组合 Resolver
  network/                                        # 跨 Domain Network Resolver/Handler
  ssh/                                            # workload SSH Secret Resolver/Handler 与本地 material source
  tensorboard/                                    # TensorBoard Resolver/Handler
```

生产 Task pull reconciler 是 Runtime-neutral Host 的薄适配层。Kubernetes 生产装配当前
会构造一个 Coordinator，其中包含不可变 implementation registry 和一个 Kubernetes
Runtime registration：

```text
Task Host
  -> 精确的 (Workload Type, Workload Version, Runtime Kind) implementation
  -> Coordinator Executor
  -> Runtime Provider
```

Workload 与 Runtime 概念保持正交，但不能任意组合。只有显式注册的三元组才能
被选择。Host 与 Coordinator 共享同一个不可变 implementation registry。重复
注册、Adapter metadata 与注册键不一致，或 Adapter 未声明对应 Runtime kind 时，
Agent 会在启动装配阶段拒绝该注册。Kubernetes 当前只注册一个
`(rlinf.io/ray, v1, Kubernetes)` implementation。Ray Adapter 独立解码
`workload.carrier`，支持版本为 `v1` 的 `rlinf.io/kubernetes-deployment`、
`rlinf.io/kubernetes-daemonset` 和 `rlinf.io/kubernetes-statefulset` carrier。
它们不是通用 Kubernetes 原生 workload 实现。

未 claim Task 会先完成解码和精确 Registry 检查，之后才能写入任何 claim metadata。
无效输入或未注册组合保持未 claim，并且不会写 Runtime。已 claim 且未删除的 Task 如果
无法解码、未通过校验或找不到已注册 implementation，Host 会保留 claim 和现有 Runtime
对象，等待实现支持或 Spec 恢复；implementation 可用性不能被解释为销毁运行中 workload
的授权。Task 删除和显式 Lifecycle cleanup 会选择 claim 中冻结的 Runtime 与 Workload
implementation，并在不依赖当前 Workload Spec 的情况下按 Task identity 执行 cleanup，
因此在 API 降级或只完成部分迁移的更新后仍可恢复这些清理流程。完整的 per-Runtime
Bundle/Factory 仍属于后续装配工作，本文不会把它描述为已经存在。

Deployment、DaemonSet 和 StatefulSet 是单个 Ray Adapter 注册内的 carrier 选择，
不是 Workload 选择入口。

该 compatibility tree 有意归 Task controller 所有，不属于 Runtime-neutral
`execution` framework。它只用于保留当前 Kubernetes Task API 的解码、证明旧 Agent
所创建对象的 ownership、历史 carrier ownership 恢复，以及升级期间仍需要的
Kubernetes reconcile profile ID 和 projection helper。Profile executor 及其不可变 registry
归 Runtime 所有。该 compatibility tree 是临时迁移债务，不是稳定扩展面。当受支持的
升级路径不再需要读取或收养老对象，并且公开 Task API 不再要求这些兼容 carrier selector
时，应删除整个 tree。新功能不得仅为方便而继续向这里增加合同。

Coordinator 通过 Runtime registration 注入的 comparator 获得 Runtime-specific 对象
比较能力。Kubernetes comparator 会把每个版本化 reconcile profile 委托给同一个
Runtime-owned 不可变 executor registry；desired-object validation 和 Provider Apply 也共用
该 registry。Coordinator 本身既不知道 Kubernetes resource kind，也不导入该 compatibility tree。

这次内部语义纠正不改变 API/CRD 定义、注册 key、生成的 Kubernetes 对象名称、manifest
或生命周期行为。未来真正通用的 Kubernetes Workload API 需要单独设计公开合同和
capability；不能把当前 carrier discriminator 当作已支持此能力。

## 内部职责

Runtime-neutral Task Host 负责：

- 读取和筛选管理面 Task；
- claimant annotation 与清理 finalizer；
- 已 claim 且未删除的 Task 无法解码、未通过校验或找不到已注册 implementation 时，
  保留现有 Runtime 对象并等待恢复原契约；
- 为 Task 删除和显式 Lifecycle cleanup 执行基于身份的清理；
- 比较 claim 中冻结的 Workload 与 carrier selector 和当前解码出的 selector，并在 Runtime-specific Lifecycle 前拒绝原地 selector 变更；
- 分派 Runtime-specific Lifecycle 决策，包括 Task stop 与 deletion 状态转换；
- 冻结 claim 中的 Runtime、Workload 与 carrier selector，并在 claim 前校验精确的 Workload
  type、version 和 Runtime kind 组合；Coordinator 使用同一个 Registry 选择并执行
  Workload Adapter；
- patch 数据面 Agent 所拥有的 Task phase、message、observed nodes 和兼容
  conditions。pull reconciliation 与 Kubernetes 定向观测共用该 Status 边界。

控制面的 Task controller 独立只拥有聚合后的 `pullProgress` 和 `events` 字段；它不会
写入数据面 Host 拥有的 phase、message、observed nodes 或兼容 conditions。

Finalizer ownership 只覆盖 Host 自己的条目，不表示拥有整个
`metadata.finalizers` 列表。claim 只追加 `rlark.io/agent-cleanup`；release
先校验并只删除这个精确条目，其他 controller 拥有的 finalizer 必须保留。同一个
patch 还会校验 Task `resourceVersion`，因此列表发生重排或并发修改时会重新读取后
重试，而不会按过期索引删除条目。

对于冻结Workload selector引入之前创建、且尚未删除的legacy claim，只有两个selector
字段同时缺失，并且当前Task能够解码到唯一已注册implementation时，Host才会回填selector
annotation。该乐观锁 claim patch 会保留 claimant、finalizer 及所有非 Host 拥有的
metadata，随后继续正常调谐。selector只存在一部分时含义不明确，必须保持Blocked，绝不
根据当前Spec补全。Task删除期间则继续使用更窄的fallback：完全缺失时仅为identity cleanup
解码selector，不把恢复值持久化。

claim 会冻结 Runtime 范围。已 claim Task 不能通过修改 `runtime.kind` 或 `runtime.name`
迁移到另一 Runtime；Agent 不会触碰新 Runtime，而是等待恢复原值，或者等待
用户删除并重建 Task。已 claim 且未删除的 Task 出现解码、校验或 implementation 注册
失败时，同样采用这一非破坏性规则。删除时仍按已 claim Runtime 和身份
执行 cleanup。只有完整 Runtime
快照确认所有独占 cleanup 对象已经消失后，Host 才会释放 cleanup finalizer 和
claim annotation。delete 请求被接受不等于 cleanup 完成；如果 delete 响应丢失，
下一轮 reconciliation 会重新读取 Runtime 状态并安全确认对象已经消失，而不依赖上次
响应。如果管理面 Task 已经 NotFound，Host 会丢弃该 reconcile 请求，绝不执行仅按名称
cleanup，因为此时已无法证明 Task UID 和 claimed identity。

内置 Ray-on-Kubernetes Workload Adapter 负责渲染、期望对象生命周期声明、更新兼容性和语义观察。Kubernetes Lifecycle 计算 Task stop 决策，Host 负责分派这些决策；`Coordinator.CleanupTask` 直接根据 Adapter inventory 执行基于身份的 Task deletion cleanup，不要求当前 Workload 仍可编译。Job restart 不属于 Agent lifecycle：Job controller 会替换 Task 对象，Agent 只处理其普通删除流程。该兼容实现当前包括：

- 判断当前 Kubernetes Task 结构是否可执行；
- inventory 声明的 Kubernetes cleanup 生命周期与顺序；
- Workload 自有模板修改和 workload identity，在 Workload 编译期间、渲染期望 Kubernetes 对象之前仅执行一次；跨 Domain Network 注入改由 Environment binding 完成；
- 镜像拉取 Secret、PVC 与 Ray 资源；
- 声明式 workload SSH requirement；Secret 交付和 Pod template mount 由 Environment 层负责；
- 渲染选中的 Deployment、DaemonSet 或 StatefulSet workload，再由 Coordinator 和 Runtime Provider 应用。

旧 `Operations` Adapter 仅作为有测试覆盖的兼容端口保留；生产装配使用 Ray-on-Kubernetes Adapter，不会调用其中的 `ApplyWorkload` 方法。

Task 的 domain 非空时，内置 Adapter 会生成一个 `CrossDomain` Network requirement，并只授权主 carrier 上一个精确 PodTemplate target。生产 Kubernetes Resolver 从 Agent 配置选择包含共享 Agent image 的不可变可信 profile。Network Handler 只修改 Binder 拥有的 Plan 副本，始终以 restartable init container 形式注入 `rlark-network-sidecar`，同时注入 `/var/run/rlark` NodeServer socket volume/mount 以及所需的 privileged/`NET_ADMIN` 配置。等价声明保持幂等；container 或 volume 同名但不等价时会阻止执行。缺少 image 时也会阻止 Task，而不会静默创建缺少所请求跨 Domain 网络的 workload。DomainPeer 路由和 NodeServer SSH relay 凭据仍属于数据面状态，绝不进入 Task binding 或 binding revision。

RLark tools 与进程监督是强制的 execution-unit 能力，而不是 Task Environment 声明。Workload Adapter 不需要请求它们。Environment binding 完成后，所选 Runtime 的 workload assembler 会接收每个已声明的 `WorkloadUnit`，安装可信 tools 二进制，并把进程声明降低为该 Runtime 的 supervisor 机制。Kubernetes 实现在 `runtime/kubernetes/supervision` 中同时拥有这两个操作；它们是同一项强制 assembly facility，而不是可分别选择的功能。该实现注入既有的 `rlark-tools-init` 复制操作、EmptyDir 和 main-container mount。无法提供此合同的 Runtime implementation 必须 fail-closed，不能静默运行不受监督的 workload。Kubernetes 当前每个 Plan 只支持一个 workload unit；在实现稳定的 per-unit supervisor 对象身份之前，多 unit 会被明确拒绝。

对于已识别的 Ray head 与 worker Task，Adapter 恰好声明一个 required primary `ProcessDeclaration`，用于运行 `rlark-tools ray-launcher`；它不声明 tools 或 supervisor requirement。SSH Handler 再贡献一个 optional auxiliary `ProcessDeclaration`，用于运行 `rlark-tools sshd`；其端口由 Adapter 通过 typed option 选择（head 为 22，worker 为 2222），Handler 不自行推断 Ray role。如果某个 workload unit 没有任何贡献者声明 primary，workload assembler 会补充 portable 默认 primary `/rlark-tools/rlark-tools idle`；它只等待终止信号，不依赖 workload image 中的 shell 或 `sleep` 实现。所有 Environment handler 完成后，Kubernetes workload assembler 会校验声明，创建归当前 Task incarnation 所有的 supervisor ConfigMap，并把 main container 入口替换为 `rlark-tools supervisor`。配置内容摘要写入 Pod template，因此进程配置变化会触发 rollout。Secret 内容不进入该配置或摘要，所以单独轮换 key 不会有意触发 workload rollout。

`rlark-tools supervisor` 是仅支持 Unix 的运行时。它作为 PID 1 运行，并将 primary 与 auxiliary declaration 启动为独立 process group。在 Linux 上，它会尝试启用 child subreaper；若启用失败（例如 seccomp 拒绝 `PR_SET_CHILD_SUBREAPER`），只记录日志而不阻止启动，因为 PID namespace 中的 PID 1 已会收养孤儿后代进程。supervisor 会回收被收养的后代进程，并向各 process group 转发终止信号；不支持 Windows。required auxiliary 是必须常驻的服务：任何退出（包括退出码为零）都表示服务失败。配置 `RestartOnFailure` 时，非零退出最多获得三次连续恢复重启，并使用线性退避。单次运行达到五分钟会重置连续失败序列，因此长生命周期中的偶发失败不会耗尽终身预算；退出码为零仍属于不可重启的终止性失败。关闭过程按顺序执行：Kubernetes 请求终止或 required auxiliary 进入终止性失败时，supervisor 先只向 primary 发送 `SIGTERM`，等待 primary 实际退出；该等待没有 supervisor 内部 deadline，因此 primary 始终不退出时由 Kubernetes 提供最终边界。primary 无论自行退出还是收到请求后退出，从其实际退出时刻起，supervisor 才向剩余 auxiliary 发送 `SIGTERM` 并启动最长 15 秒的 auxiliary 关闭期，必要时再发送 `SIGKILL`。发送 `SIGKILL` 后最多再等待五秒回收，随后逐一记录仍未回收的 auxiliary，而不会让 PID 1 永久阻塞。生命周期日志使用 RLark 的结构化 `logr`/zap 管线，公开进程名称、角色、PID、退出码、运行时长、重启计数、关闭转换和强杀升级，但绝不包含 argv、环境变量或 Secret 内容。supervisor CLI 会保留 primary 的非零退出码；required auxiliary 和 supervisor 内部失败使用工具的通用失败码。primary 退出决定正常 workload 完成；required auxiliary 耗尽连续重启预算仍会使 supervisor 失败。Network 与 TensorBoard 不属于 process declaration，继续使用各自现有的 Kubernetes 生命周期。`ray-launcher` 不拥有网络配置。Ray Adapter 发现 canonical Network Environment 时，会启用内置且支持 context 取消的 DNS 就绪屏障，并在用户 prepare script 前执行；不再使用外部 `WAIT_NETWORK_SCRIPT` hook。用户 prepare/run script 仍是显式 shell 兼容边界。Ray 主生命周期由 Go 实现；prepare 状态传递 trampoline 与 Ray SDK 集群检查从 package 文件 embed 进二进制，不通过 ConfigMap 或挂载脚本交付。

Process declaration 是 portable Plan 合同，会被深拷贝并计入 Adapter compiled hash。Workload Adapter 协议会将它与 Workload unit、精确 mutation target 和 Environment requirement 一起传输。Runtime Provider 只接收最终装配后的 manifest 与只读 Runtime-unit projection，不解释 Environment requirement 或 process 语义。

在可选的用户 prepare-script 边界之后、执行任何 Ray stop/start 命令之前，`ray-launcher` 会确认配置的 Ray CLI 可解析且可执行。CLI 缺失或不可执行会使 primary 失败。Head 上通过 embed 交付的 Ray SDK 集群检查为保持兼容仍是非致命检查，但随后的 `ray status` 失败属于致命错误。Worker 上无论 `ray start --block` 启动失败，还是启动后 Ray 以非零状态退出，都会使 primary 失败；普通进程退出时，Ray 退出码会经 launcher 和 supervisor 保留下来。

Workload 语义由 `spec.workload.kind` 和 `spec.workload.version` 选择，任意 JSON `spec.workload.parameters` 必须由选中的 Adapter 严格解码。`spec.workload.carrier` 独立包含 Runtime-specific carrier 的 `type`、`version` 和任意 JSON `parameters`。Deployment 与 StatefulSet carrier 只接受 `{replicas}`；DaemonSet carrier 只接受空对象。Environment 使用相同的 type/version/parameters envelope 组成有序列表，并且允许重复；是否要求唯一由选中的 Environment resolver 按自身合同决定。未知类型、版本、非法参数或 Runtime 不支持的能力都必须在 Provider effect 前失败。

Task decoder 会通用映射每个 Environment envelope，不枚举内置类型：它只检查必需的公共字段，保留顺序和重复声明，深拷贝参数，并把特定类型的唯一性、version 与参数校验交给选中的 Runtime resolver。Kubernetes Environment resolver 通过不可变的 type-to-resolver registry 安装；空 type、nil resolver 或重复 type 注册会在构造时失败。组合式 resolver 按类型首次出现顺序分组，并把同一类型的全部声明按原始顺序一次性交给对应 resolver。Network、SSH 与 TensorBoard 当前会自行拒绝多个声明；未来的 Environment 合同可以注册接受并解析多个声明的 resolver。因此未知类型可以完成 decode，但会在 Provider observation 或 effect 前的 resolve 阶段 fail closed。Portable execution plan 保留同样的 Environment envelope，不再把 requirement 拆分成 network、secret、tool、service、storage、device 或 config 字段。每个 plan requirement 包含 `name`、开放字符串 `type` 与 `version`、复制后的 `parameters` 以及 consumer selectors。SSH authorized public keys 只保留在本地 execution declaration 中，不进入 portable requirement parameters 或 workload transport；SSH Handler 通过 material boundary 获取它们。

Domain 是虚拟网络配置，不是 Task placement 或 Runtime identity。`Task.spec.placement` 只包含调度约束。独立 Task 通过且仅通过一个版本为 `v1`、parameters 为 `{domain}` 的 `rlinf.io/network` environment 请求跨 Domain 网络；重复的 Network 声明会在 Provider effect 前被拒绝。Job controller 保留 `Job.spec.domain` 作为上层便利字段，并把它 lower 成该 Task environment。Kubernetes Network handler 负责最终的 `rlark.io/management-task-domain` PodTemplate annotation 和 sidecar mutation；Job controller 不直接写 Kubernetes-specific workload metadata。Host claim 不再持久化或比较 Domain。

对于识别为 Ray head 或 worker 且请求 SSH Environment 的 Task，该 Environment 会转换为面向其精确 PodTemplate target 的独立 workload SSH Secret requirement。Environment 的 `authorizedKeys` 数组属于公钥认证配置：material provider 从深拷贝后的本地 Task snapshot 中读取它，而 portable Plan parameters、远程 workload transport、resolved binding、Status、日志、错误、revision 和 operation identity 都不会包含它。没有匹配 Ray workload 与 SSH Environment 的 Task 不会获得自动 workload SSH 生命周期。`HandlerRequest.Task` 显式提供生成对象所需的 Task identity，Handler 和 Provider 无需从名称或 label 推断 ownership。

SSH Handler 声明一个归当前 Task incarnation 独占的 opaque Secret，让 workload 依赖它，并把其中的 `authorized_key` item 以只读方式挂载到 main container 的 `/var/run/rlark/ssh/authorized_key`。该目录挂载刻意不使用 `subPath`，因此 kubelet 可以通过原子切换 volume projection 更新文件。Secret 先于 workload 创建；Task cleanup 时先删除 workload，再删除 Secret。Secret 名称由 logical Task name 和 Task UID digest 派生；Kubernetes profile 只有在证明对象具有当前 incarnation 的标准 ownership 后才允许原地更新 data。同名 foreign 或 ownership 不明确的 Secret 绝不会被收养、更新或删除。

空 `authorized_key` 文件表示关闭 workload 公钥认证；配置该 Environment 时，内置 Ray workload 仍会尝试启动 SSH auxiliary。`rlark-tools sshd` 每次认证都会重新打开配置文件，因此首次迁移 rollout 完成后，轮换或清空被引用的key material通常只更新Secret。SSH 可用性不属于 Task 成功条件：optional auxiliary 首次启动失败只记录日志并忽略；正常退出不会重启；非零退出会获得 supervisor 的有限次数重启。重启启动失败或耗尽尝试也只记录日志并忽略，primary 与 Task 会在没有 SSH 的情况下继续运行。消费端不得把文件复制到持久位置、缓存解析后的 `..data` 路径或长期持有打开的文件描述符。更新受 kubelet Secret projection 收敛延迟影响；该合同不承诺瞬时撤销。旧 `RLARK_SSH_PUBLIC_KEY` 环境变量 fallback 和对运行中 Pod 执行 `exec` 追加 key 的路径不再使用。NodeServer 与 DomainPeer 的 SSH relay credential 是独立的数据面机制，不受本改动影响。

Task 请求 `rlinf.io/tensorboard/v1` 且 `parameters.logDir` 非空时，Environment Handler 会生成单独的辅助服务 requirement。这是一项有意的行为变化。旧 TensorBoard mutation 只从 Ray 初始化流程调用，因此受已识别的 Ray head 或 worker role 门控。新的 Environment requirement 只取决于显式、非空的 `logDir`，并适用于任意内置 Kubernetes carrier。TensorBoard Handler 从 `RLARK_TENSORBOARD_IMAGE` 选择可信 image；该变量未设置或为空时回退到历史值 `rlinf/tensorboard:2.15.1-rc1`。在经过校验的 typed Agent 配置接管该值前，环境变量只是过渡装配方式；Task 和 Adapter 不能选择任意镜像。

若 main container 已有 volume mount 覆盖所请求的绝对目录，则 Handler 复用该 mount；否则创建历史 EmptyDir，并把 TensorBoard 声明为 restartable init sidecar。等价声明保持幂等；名称、mount path 冲突或相对 TensorBoard 目录都会在 Provider 观测或 effect 前 fail-closed。虽然 Kubernetes Resolver 会组合三者，Tools、TensorBoard 和 Network 仍分别使用独立 Handler 与 profile。

旧版 `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES=true` mutation 在 Workload 编译期间、Environment binding 之前执行。因此它只作用于 Adapter 产出的 Pod template 中当时已有的 container 和 init container，不会统一提升后续由 Environment Handler 注入的 container 权限。该顺序不会移除 Environment 功能所需的权限：可信 Network profile 会为 `rlark-network-sidecar` 显式声明 privileged/`NET_ADMIN`，Tools 和 TensorBoard 则使用各自 Handler 声明的 security context。后续 Environment Handler 不得把该 unsafe 开关视为隐式请求特权的授权来源。

Environment handler 完成后，Kubernetes Runtime lowerer 会对 Adapter 声明的每个 `Pod` 或 `PodTemplate` unit 执行最终 capability 适配。生产装配只读取一次 `KUBERNETES_SUPPORT_SIDECAR_FEATURE`。只要该值不严格等于 `false`，就保留 restartable init container；值为 `false` 时，会把所有 `restartPolicy` 为 `Always` 的 init container 按声明顺序移动到已有普通 container 之后，并移除仅属于 init container 的 restart policy，普通 init container 保持不变。若名称与已有普通 container 冲突，或 unit locator 不受支持，则在 Provider effect 前 fail-closed。该规则统一作用于 Workload 编译和 Environment binding 产生的 sidecar，两类 producer 都不再自行解释 Runtime capability。

对于已 claim 且未删除的 Task，Host 将 Workload selector 与 carrier selector 都视为
不可变。Workload selector 变化时报告 `WorkloadSelectorChanged`；carrier selector 变化时
报告 `CarrierSelectorChanged`。两条路径都不会删除现有 carrier、创建替代对象或改写冻结的
claim。调用方必须恢复原 selector，或者删除并以新 UID 重建 Task。Task 删除和暂停仍使用
冻结的 selector 与 identity inventory，因此即使当前 Spec 非法或已经变化，cleanup 仍可恢复。

Kubernetes Runtime 边界负责通用对象与实例操作，不应吸收 Ray 或 RLark Task 专属编排语义；稳定的旧对象 metadata 和 ownership 规则单独放在 compatibility 边界供双方使用。

这些 Go 接口是仓库内部实现契约，其范围小于服务化 Runtime Provider 契约，不能被视为稳定公开 SDK 或网络协议。

## 远程边界

需要允许外部实现的组件定义版本化远程协议：

- `WorkloadAdapter`：Workload 元数据与 Configuration Schema、声明式 Compile、语义 Observe 和基于身份的 Inventory；
- `RuntimeProvider`：Runtime 信息、对象 Get/List/Apply/Delete、Adapter 定向候选快照、旧 claim Workload 恢复和完整实例快照；

Workload Adapter 可以是内置实现或远程实现。两者实现相同的内部 `workload.Adapter` 契约，只返回声明结果，不能访问 Runtime Provider 或直接写 Task Status。这是 VolcanoJob、PyTorchJob 等外部 Job 类型的主要扩展入口。

Environment Resolver/Binder、Coordinator、Compatibility Mapper 和 Instance Projection 保持进程内调用，使平台 Policy、最终准入和外部 API 修改仍由 RLark Core 控制。首版 Runtime 协议不包含 Logs、Exec 和 Watch；只有真实远程实现需要时，才将其增加为可选服务。

内部 Environment 链路将声明与 effect 分开：

```text
Adapter.Compile
  -> desired object + portable requirement + 显式 MutationTarget
Resolver.Resolve
  -> 已校验的具体 consumer + 可信的非秘密 profile metadata
PlanBinder.Bind
  -> 基于 deep copy 生成 Runtime-ready ExecutablePlan
Coordinator
  -> binding revision + inventory + diff
Runtime Provider
  -> 只针对最终 ExecutablePlan 执行观测与有序 effect
```

`MutationTarget` 是 mutation 位置的 allow-list 条目，不是通用 JSON 修改 API。它以逻辑身份引用一个 Plan object，声明封闭的 target kind，并携带由所选 Runtime Binder 解释和校验的静态路径。Requirement 应引用精确 target name。旧 object/role/unit selector 只作为兼容输入，binding handler 运行前必须将其收敛为具体 target。远程 Workload Adapter protobuf 会把 Target 与 Environment requirement 一起传输。缺少精确 target 授权的 requirement 会形成不完整且不安全的 Plan，因此转换会原子地保留两类合同，而不会传输 requirement 却静默丢弃 target。

Mutation target、Workload unit 和最终 Runtime-unit projection 共用一个中立的 `runtime.UnitKind` 枚举。当前封闭值为 `PodTemplate` 和完整 `Pod`，但类型名没有限定为 Pod，未来 Runtime 合同可以增加其他显式支持的 unit shape，而无需再建立平行枚举。共用 kind 只描述结构，不会让 Workload unit 自动获得 Environment mutation 授权。如果 typed desired object 自身具有对应语义，包括某些 Runtime-specific resource 本身就表示 PodTemplate，两种当前 kind 都可以使用整个对象 locator `.`；如果结构内嵌在其他对象中，则使用 `spec.template` 等静态字段 locator。中立的 `execution/runtime/unit` accessor 只在 `.` 或点分隔且已存在的 object field 上读写 opaque JSON；它拒绝数组与 JSONPath operator，也不解释 Runtime 或 unit 语义。`Pod` target 绝不表示 Deployment、DaemonSet、StatefulSet 或其他 controller 后续创建的 Pod。仅声明 path 不能证明任意 JSON 具有 Pod 语义；所选 Runtime lowerer、Handler 和 final validator 必须校验定位值的 typed structure。

Kubernetes Network Handler 和 Runtime lowerer 使用该 accessor 后，再把定位值解码为完整 Pod 或 PodTemplate。它们不再根据外层 workload GVK 分支，因此 Adapter 可以在任意 Kubernetes workload object 或 CRD 的有效静态字段路径上声明已授权 unit。Network Handler 同时拥有跨 Domain sidecar/socket mutation 和 Pod metadata domain annotation；Workload Adapter 继续写入 Task name、namespace、UID 和 workload identity metadata，但不再写入 network-owned domain annotation。路径缺失、Pod 结构错误、type 不匹配、名称冲突或 unit kind 不受支持时，都会在 Provider effect 前 fail-closed。该通用性只适用于显式声明的 unit 和 mutation target，不允许扫描 manifest，也不授权修改 controller 后续创建的 running Pod。

Kubernetes reconcile profile 通过 Runtime-owned 的不可变 registry 执行，并由 desired-object validation、Provider Apply 与 Coordinator comparison 共用。每个 executor 只负责一个精确且带版本的 profile ID，并拥有其 validation、comparison 与 update preparation；重复注册和未知的非空 profile 都会 fail-closed。这样可以删除中心 profile switch，而不会把 profile 变成 untyped policy language。builtin workload compatibility 仍刻意保持窄化，历史 carrier adoption、recovery、discovery 与 cleanup 则继续和通用路径分离。

Kubernetes Provider 根据声明的 GVK 解析对象。对于注入 Scheme 已注册的 GVK，它保留 typed object；对于其他可发现 GVK，则使用 Kubernetes unstructured object 与 list，因此通用 exact-object CRUD 和 Adapter-directed inventory 不再受 builtin carrier kind 限制。更新前仍必须注册 reconcile profile，并由其验证 object contract；dynamic decoding 不表示任意 CRD 自动支持 builtin workload profile。

`WorkloadUnit` 与 mutation 授权相互独立。Adapter 把每个 Pod-like unit 声明为 `PodTemplate` 或直接 `Pod`，同时给出所属逻辑对象和 typed static locator。binding 会先校验和投影这些声明，运行 Environment handler，再调用所选 Runtime 拥有的无 effect `runtime.UnitLowerer`。中立接口只接收单个 `runtime.DesiredObject`，不接收 Workload Plan；Kubernetes 实现归属 `controllers/task/runtime/kubernetes`。lowerer 可以根据可信 Runtime version 或 capability 改造最终已声明 unit，但不得执行 Runtime I/O、不得依赖 Workload 或 Environment 包，并且输出仍受 final Plan validation 约束。最终 `DesiredObject` 会保留只读、对象相对的 `RuntimeUnit` metadata，使本地 Runtime Provider 无需扫描 manifest、也无需获得 Environment target 权限即可识别 Pod 语义。Provider 仍只应用最终 manifest，不能把 unit 当成隐藏 mutation 授权。当前远程 Workload Adapter 和 Runtime Provider protobuf 尚不承载 unit；两个 transport 对非空 unit 都会明确拒绝，而不是静默丢弃。Wire 支持需要使用 pinned protobuf toolchain 做 additive、capability-negotiated 协议升级。

Resolver 和 Binder 必须在 Coordinator 读取 Provider 快照或执行 Provider effect 前完整成功。Binder 只能修改 Plan 的 deep copy，不执行 management 或 Runtime I/O，失败时不能返回可部分 Apply 的 Plan。因此 Provider 只接收最终 Runtime-ready object，不推断 network、SSH 或其他 Environment 业务语义。

共享的便携执行数据由中立的 `execution/plan` 包拥有：编译后的 `Plan`、其中的 Environment requirement 和 mutation-target 声明、最终 `ExecutablePlan` 以及 deep-copy 行为。调用方直接引用这些定义，不再通过 Workload 或 Environment alias 间接引用。`execution` tree 只保留 Runtime-neutral 合同、编排、校验和 transport，不提供生产 Runtime、Workload 或 Environment 实现。Task controller 持有的实现位于 `controllers/task/workload/ray/kubernetes`、`controllers/task/runtime/kubernetes` 和 `controllers/task/environment/kubernetes`；这些实现可以依赖 framework，framework 不得反向依赖它们。不要向根 `execution` 目录堆放共享数据类型。

Environment handler 不采用固定的业务 phase 顺序。可信本地生产装配会为 handler 注册稳定 ID，并声明强制 `Requires` 和可选 `Before`/`After` 依赖。强制依赖的 handler 必须由同一次请求中的显式 binding 选中；框架不会制造输入含糊的空 binding。可选顺序引用只在双方均参与时生效。Registry 会拒绝重复 ID/selection key 和依赖环，在构造时复制顺序 metadata，并以 handler ID 作为稳定 tie-breaker，保证注册顺序或后续 handler mutation 不会改变执行结果。Task 和 Adapter payload 不能指定 handler 或修改依赖图。顺序也不授予“后写覆盖”权限：可信 handler 必须在 mutation 前声明标准化且不含秘密的 semantic write intent；等价写保持幂等，同一个 slot 的不同值会在任何 handler Apply 前以 `Conflict` 使整次 Bind 失败。具体 Binder 在 mutation 后仍必须执行最终 Runtime schema validation；intent 声明不能证明最终 manifest 有效。这个校验接缝保持单向依赖：`binding.DesiredPlanValidator` 先统一调用 `runtime.ValidateObjects` 校验通用 logical identity、ownership、reference 和 DAG 不变量，再逐对象调用所选 Runtime 实现的窄接口、无 effect 的 `DesiredObjectValidator`。Binding 负责理解 `BindRequest` 和 Plan；Runtime 包不得导入 binding 合同或重复图校验。

这是增量落地的内部合同。独立的 `execution/binding` 包负责 `BindingPlanBinder`、其 request/interface、强制 Runtime workload-assembler hook 和 final Plan-validation adaptation；`plan` 继续作为数据合同包，`workload` 负责 Adapter 语义。Kubernetes 生产装配现已使用该 Binder、Kubernetes final Plan validator、强制 tools/supervisor assembly，以及组合了独立可信 Network、workload SSH 和 TensorBoard handler 的 Resolver。内置 Ray-on-Kubernetes Adapter 会生成一个精确 PodTemplate target 和 unit，以及自身的 primary process declaration，但不再自行执行这些 mutation。尚未支持的 Storage、Device、非内置 Secret 或 Config requirement 继续在 Provider 观测或 effect 前 fail-closed。

协议定义位于 `proto/execution`，生成的 Go binding 位于 `sdks/execution-go/gen`。在仓库根目录执行：

```bash
make proto-execution
```

生成依赖固定版本的工具链：请先执行 `make protoc-install`，详见 [Protobuf 产物与固定版本工具链](local-development.md#protobuf)。

Transport adapter 位于 `execution/runtime/transport/grpc` 和 `execution/workload/transport/grpc`。客户端分别实现内部 `runtime.Provider` 和 `workload.Adapter` 接口；服务端将内部实现通过生成 SDK 暴露。Workload Adapter 会接收规范化的 typed `WorkloadConfiguration`：公共 Pod 模板、不透明 workload parameters，以及一个携带不透明参数的版本化 carrier envelope。Workload identity 由 `WorkloadSelector` 承载，carrier identity 还会冻结到 `CarrierSelector`；transport conversion 要求 selector 与 configuration envelope 一致。Adapter 独立负责严格校验 workload 和 carrier 参数，并生成一个完整 Plan。现有 protobuf `TaskSnapshot.configuration` 仍是 JSON `TypedValue`，因此本次 JSON 结构变化无需修改 protobuf schema。Environment 通过各自有序的 envelope 单独解析；系统不再提供通用 extension 列表、Runtime-native 配置逃生口或 compile 后 object patch 阶段。protobuf 消息不会成为内部领域模型。协议 major version 2 会传输完整 Plan 合同，包括 Environment requirement、mutation target、Workload 与 Runtime unit、process declaration 和 binding revision；同时通过 capability 广告 Workload inventory、Runtime candidate inventory、owned-field projection、旧 claim Workload 恢复、有证据的 Task-incarnation 收养和 typed reconcile profile。当前 Coordinator 会在使用收养声明和 reconcile profile 时校验对应 capability。较早的 inventory、projection 和 claimed-recovery 标志会被传输和广告，但这些路径尚未统一通过 capability negotiation 完成组合选择；Core 同时依赖所需 Go interface，并在执行时校验声明。消费者不能把每个已广告标志理解为所有组合都已经完成协商。两类 transport adapter 都已通过内存 gRPC 往返测试，但默认 Task 控制器配置尚未选择远程实现。

传输适配器会将稳定的 execution fault 映射为 gRPC status code，并在客户端还原，以支持通过 `errors.Is` 分类处理。`execution/transport/grpc` 中的可选 interceptor 为其他 execution 服务提供相同的映射。当前协议不包含请求关联和分布式 tracing；只有端到端 tracing 真正实现时才会增加 request ID。

Runtime 的 `ListCandidateObjects`、旧版 `ListManaged` 和 `ListInstances` 都是原子的完整快照操作。Provider 无法生成完整结果集时必须返回错误；不支持部分成功快照，也不得使用部分结果执行清理或判断资源缺失。

选定的 Workload Adapter 根据冻结的 Task 和 implementation identity 生成有界 inventory。候选项按创建顺序声明。identity cleanup 以 `LifecycleDeleteOnTaskCleanup` 作为删除授权，每轮删除当前仍存在的最后一个精确名称 cleanup candidate，即按逆序清理；它不维护 Kubernetes kind allow-list。Ray-on-Kubernetes Adapter 先声明 SSH Secret，再声明其 workload 消费方，使 workload 对象先于 Secret 消失。它不声明 PVC candidate：引用的外部 PVC 不属于 RLark ownership，generic ephemeral PVC 则随 Pod 生命周期管理。Kubernetes Provider 只执行 Adapter 声明，不计算 Ray 名称，也不推断 Ray role。候选名称和标签只能定位对象，不能证明 ownership；Provider 返回对象前仍会校验标准 Task identity，并在适用处校验对象 UID。任一查询失败都会使完整快照原子失败。实例观测会对 workload root 执行 UID 校验，只列出该 workload 选中的 Pod，并使用 `involvedObject.uid` field selector 查询 Pod Event。

每个 desired Kubernetes object 都会显式选择 typed reconcile profile，而不是让 Provider 根据 manifest kind 推断更新策略。当前 Ray Adapter 为 Deployment、DaemonSet 和 StatefulSet carrier 选择 workload projection，为 head Service 选择 Service projection，为 bootstrap ConfigMap 选择 shared-field projection。

Runtime-owned 不可变 registry 会分派显式选择的版本化 executor，并验证 typed manifest 是否与该 profile 匹配；profile 缺失、未知、重复或不匹配都会被拒绝。每个 executor 为一个有界 Kubernetes 机制同时实现 validation、comparison 和 update preparation，例如比较 Service owned projection。选择哪种机制表达 Workload 契约仍由 Adapter 负责。

在安排 Service Replace 前，Coordinator 通过该 registry 只比较 Service profile 拥有的字段。API Server 物化和分配的 ClusterIP、ClusterIPs、IP family、NodePort、默认 protocol 和 target port、流量策略、session affinity，以及无关 metadata 都不会产生 Replace 漂移。内置 registry 仍刻意保持窄化，其 workload executor 不是通用 CRD update policy。

Kubernetes Provider 在比较期望 Deployment、DaemonSet 或 StatefulSet 的 Pod template 与 API Server 返回的对象之前，会在期望 template 未指定相应字段时保守地保留常见的 Pod 和 container 具现化默认值。当前兼容集合覆盖 Pod restart policy、DNS policy、scheduler name、termination grace period、空 Pod security context，以及 container image-pull 和 termination-message 默认值。这样既不会覆盖显式期望值，也能避免常见的重复更新，但并未复现随 Kubernetes 版本变化的全部嵌套 container、probe、volume 或 ephemeral-container 默认规则。当前 Provider 将完整 Pod template 视为 Agent 管理字段；mutating admission webhook 或其他 controller 注入该 template 的字段不属于当前支持的 ownership 边界，可能被覆盖后再次注入。在实现明确的 Agent-owned 字段投影或 server-side apply ownership 契约前，集群不应对 RLark 管理的 workload template 应用此类变更。

Ray bootstrap ConfigMap 使用 RLark 保留固定名称，并跨 Task cleanup 保留。Adapter 在每个 desired object 中显式投影其 `data`、`binaryData` 和 Ray 脚本身份 annotation 的 ownership；Provider 只更新该投影，不识别 Ray 名称。脚本内容一致时保持零写，发生漂移时原地修复；其他 label 和 annotation 会保留。没有身份 annotation 的旧固定名称 ConfigMap 可以规范化，但 Ray role metadata 冲突时会保持不动并返回 Conflict。shared object 缺少 projection 或 projection 不受支持时会被拒绝，不会退化为整对象更新。运维人员不得把这些保留名称用于无关 ConfigMap，也不得让其他 manager 写入脚本字段。

PodTemplate 中的 `persistentVolumeClaim` volume 仅用于引用，必须填写 workload namespace 中已有 PVC 的名称。Ray Adapter 和 Kubernetes Provider 不创建、收养、校验、修改、盘点或删除该 PVC。Generic ephemeral volume 仍保留在 Pod template 中，其 PVC 由 Kubernetes 按 Pod 生命周期创建和删除。

Ray head Service遵循主workload策略，而不是保留型共享资源策略。它属于单个Task incarnation，携带标准Task UID metadata，并声明`Replace`更新语义。有冲突的受管Service会使用UID fencing删除，并在后续reconcile中重新创建，因此API Server分配的地址或端口可能变化，也可能发生短暂中断。其稳定identity不包含Task generation和adopted-generation annotation，因此仅generation变化不会替换Service。Task-incarnation ownership默认要求标准的当前Task UID metadata。Ray Adapter仅为主workload和head Service显式允许有证据的收养：兼容的旧workload继续使用已有的窄化workload证明。为兼容升级，当同一完整inventory快照中恰好存在一个已证明的workload root时，Provider也会返回没有标准Task UID的精确名称、非workload辅助候选；这样即使旧head Service没有标准annotation和owner reference，`Replace`仍可将其删除。标签选择型候选、foreign标准UID、指向未证明对象的controller，以及root缺失或存在歧义都会阻止该路径。删除仍使用UID fencing。该策略不适用于引用的外部 PVC 或 shared ConfigMap；外部 PVC 不由 RLark 管理，shared ConfigMap 则保持其投影语义。

Execution 错误保留 `InvalidArgument`、`NotFound`、`Conflict`、`PermissionDenied`、`Unavailable` 等 typed fault code；本地包装错误同时保留底层 cause，供 `errors.Is`/`errors.As` 和诊断使用。gRPC transport 会将这些类别映射为 status code，并在客户端恢复 execution fault 类别。

生产 Coordinator 提供三个相互独立的路径：

- `Reconcile` 编译并绑定期望计划，向选定 Adapter 请求 identity inventory，读取完整候选快照，计算 diff，执行有序操作并观测结果。
- `CleanupTask` 选择已 claim Runtime 和 claim annotation 中冻结的 Workload implementation，向 Adapter 请求 identity inventory，并在 Provider 完成 ownership 证明后使用各精确候选声明的 lifecycle。它绝不重新编译当前 Spec，因此即使该 Spec 无法解码，Task 删除仍可恢复。对于冻结 Workload selector 引入之前写入的 claim，如果删除中 Task 的两个 selector 字段都缺失，Host可以仅从仍受finalizer保护的Task解码并校验Adapter selector注册；后续cleanup仍只使用identity inventory和Provider ownership/UID校验，恢复出的selector不会写回Task。这个窄化升级fallback使Job restart即使是新Agent第一次观察旧Task，也能清理该Task。selector只存在一部分、Task无法解码或registration不可用时会阻止删除。在这个删除态fallback之外，旧selector恢复仍要求唯一、ownership已证明且execution revision完整的workload root；证据歧义或冲突会阻止删除。精确候选只要声明为 `DeleteOnTaskCleanup`，就会与 Kubernetes kind 无关地按 inventory 逆序并使用对象 UID fencing 删除；共享 Ray 脚本 ConfigMap 继续保持 `Retain`，外部 PVC 引用不会进入 inventory。
- `ObserveRoot` 获取一个带 UID 校验的工作负载 root，只列出它的实例和相关事件，再调用选定 Adapter 的 `Observe`。它不会执行 compile、resolve、bind、`ListManaged`、diff 或 Runtime 写入。Runtime push reconciler 只产生 hint；仅当快速观测 gate 验证当前 Task、claim、ownership、UID、Runtime、Workload 和 revision evidence 全部一致时，pull controller 才会对高频 root 变化使用该路径。

Cleanup 从不依赖进程内缓存。定向观测会重新读取当前 Task 和 Runtime root，但其优化 gate 还要求当前 Agent 进程内由成功完整调谐建立的 revision evidence。该缓存不具权威性：进程重启、缓存缺失、desired-state trigger、periodic trigger、旧 metadata 或证据冲突都会降级为完整调谐。

Host 将每次 reconciliation 归类为一个互斥结果：`Succeeded`、
`WaitingForChange`、`Retrying`、`CleanupInProgress`、`Blocked` 或 `Dropped`。
重试行为使用 `None`、`OnChange`、`Conflict`、`Transient` 或 `InProgress` typed
分类，不解析错误字符串。Status intent 携带 Task UID 和 generation，防止旧观测
更新已重建或更新后的 Task incarnation。

这些 outcome 和 retry class 是 Host 层的调度词汇，不是完整贯通
Provider、Coordinator 与 Host 的端到端 retry 协议。Kubernetes 生产路径实际使用
success、waiting、retrying、cleanup in progress 和 dropped；`Blocked` 当前仅用于少量
claim 后稳定契约错误及防御性 lifecycle 分支。预计广泛使用 `Blocked` 的 canonical
ownership 和策略冲突，延期到后续 metadata 与 ResourcePolicy 工作。已定义的 Provider
和 Coordinator fault code 会继续保留，但不保证每种 fault 都映射为独立的 Host
retry class。

精确 Workload type、version 与 Runtime kind 注册不存在时，execution fault 仍为
`NotFound`。注册项存在但缺少可用 Provider 或 Adapter 属于无效装配，仍为
`FailedPrecondition`。

Host 按 management Task 的 namespace 和 name 串行 reconciliation，同时允许不同
Task 并发执行。Runtime-specific 兼容 lifecycle 通过 opaque lifecycle input 接收
外部 Task；通用 Host 不解析 Kubernetes workload 字段。Lifecycle 只能返回决策和
status intent，不得直接 patch Task。Status intent 区分“未提供字段”和“显式空值”，
因此只更新 condition 时不会清空 phase、message 或 observed nodes。同一轮
reconciliation 产生的 intent 会先做 UID/generation fencing，再按优先级合并，并在
lifecycle 允许时统一写入。

Kubernetes 执行路径在有界的进程内 accumulator 中合并 desired-state、Runtime-state 和 periodic hint。饱和会显式报告，不会静默驱逐其他 Task；watch replay 与周期 resync 用于恢复丢失的 hint。Runtime push controller 不再直接观测 root 或写 Task Status。Verified fast observation 会有意绕过 Host keyed lock 和 Lifecycle hook：它只能读取 Runtime 状态，并执行一次带 UID/generation fence 的 workload Status 写入。若某个 effect 要求每轮调谐都执行，则不能只把它加入 Lifecycle hook；还必须扩展或禁用 fast-path gate。

共享的 cluster-Agent resync 启动后立即执行首轮，通过分页扫描分配给同一 Host 或符合该 Host 条件的 management Task，每轮预算上限为 10,000 个 Task。扫描器与调谐共享 Host claim 和 eligibility 规则，而不是重复实现：有效 claim 必须包含 Agent finalizer；已 claim Task 即使 selector 漂移或进入删除流程，仍属于其 owning Host。后续扫描使用 `--task-runtime-resync-period` 的 80%–120% 抖动间隔；默认值为 `15m`，`0` 表示禁用，负值会在启动时被拒绝。Management API 的临时 List 失败会记录日志并在下一周期重试，不会终止 Agent；trigger bridge 背压按轮汇总，也不会阻止后续 Task 和分页被访问。扫描只产生 periodic 完整调谐 hint，不把进程内扫描进度当作期望状态。

对于已经证明 ownership 的主 Deployment、DaemonSet 或 StatefulSet，正常 Apply 会在 root 顶层 annotation 写入 canonical Runtime 身份（ID/kind）、Workload 身份（type/version）、compiled/executable digest 和不含秘密的 binding revision。现有 Task name、namespace、UID、generation 与 domain annotation 仍是 ownership 和 lifecycle 兼容契约。Job restart annotation 不会复制到 Runtime 对象，也不会由 Agent 解释。metadata 只写在 root 对象顶层，不写入 Pod template，因此仅修复 metadata 不会触发 Pod rollout。缺失、格式错误或不匹配的 canonical revision metadata 不能授权定向观测，必须选择完整调谐。对于 canonical-owned workload，完整 Apply 只有在证明 Task ownership（包括 Task UID annotation）、UID-fenced reference 所指向的当前对象实例以及 selector 兼容后，才会修复整组 canonical execution revision。窄化兼容的 legacy workload 在不存在冲突 ownership metadata 时，可依据 name、namespace、kind 与 selector shape 进行接管；该接管会写入 canonical Task 身份和 revision metadata，但保留 Pod template。后续 canonical-owned Apply 即使在相同 Task generation 也可修复 revision，从而允许 identifier 规范化或 digest 算法升级自愈。Execution revision annotation 只是顺序与优化证据，本身不构成 ownership 证明。

Binding revision 是标准化的 `sha256:` digest，输入包括 binding kind/name、可信 profile identity 与 revision、排序后的具体 target consumer、ownership、lifecycle、source revision、handler algorithm 和 value type；不包含 raw binding value 或 Secret 内容。计算 executable hash 时也会先把 Kubernetes Secret 的 `data` 和 `stringData` 替换为固定标记；Runtime comparator 仍获得真实 desired Secret，因此 key 轮换仍会触发原地 Secret 更新。`ResolvedBinding.Value` 只能承载受控、可序列化且非敏感的 option；raw key、token 和 credential 必须使用单独受限的进程内 material 机制，绝不能进入 portable Plan、transport、Status、诊断、error、hash 或 revision。生产诊断脱敏不能通过运行时配置关闭；observer 失败或 panic 不会改变调谐结果、Status 或 Runtime effect。接入新的诊断 payload shape 前，必须针对每个承载值的字段扩展并测试脱敏：当前通用 sanitizer 覆盖 environment `value`、名称含敏感词的 key，以及 Secret `data`/`stringData`，但不会推断 container `command`/`args` 中嵌入的秘密，也不会处理非敏感 key 下 `TypedOptions` value 中的秘密。基于 Environment 的 Network、workload SSH 和 TensorBoard mutation，以及 Runtime 强制提供的 RLark tools 与 supervisor assembly，均已进入生产装配。

受限 material 机制仅存在于 Binder 本地，是通用且不可序列化的能力。可信 `MaterialProvider` 接收 Task identity 和 resolved binding identity，并返回不透明的 `MaterialLease`。声明 semantic write intent 时 `HandlerRequest.Materials` 始终为 nil；只有配置了有效 Provider 时，它在 `Apply` 阶段才为非 nil，因此需要 material 的 handler 必须将 nil 视为能力不可用。框架会在每个 handler 成功、报错或 panic 后关闭所有 lease。实现不得保留、记录、序列化、哈希或暴露 lease bytes，error 也必须脱敏。Environment core 刻意不定义 SSH key 名称、注入模式或兼容 profile；这些语义由 controller-owned Kubernetes SSH package 负责。Coordinator 生产装配可以基于已经解码的 `TaskSnapshot` 构造 per-Task Binder；该 factory 不得执行外部 I/O，得到的 material provider 只作用于本次 evaluation，不能跨 Task 共享。`HandlerRequest.Task` 携带与 resolved environment 相同的显式 identity，使 Handler 能声明有 ownership 的对象，而无需从 manifest 重建身份。

正常 pull reconciliation 不会仅因为 Task 已处于 `Running` 就停止。后续 Task generation 仍会重新编译并计算 diff，使 replicas、镜像和 Pod template 等受支持变更通过同一 Coordinator 路径收敛。Generation metadata 是顺序 fence，并不能证明 Runtime 状态仍与期望状态一致：即使 Task generation 未变化，完整调谐也会修复 controller-owned replicas 和 Pod template 字段的带外漂移；这些字段及 canonical metadata 已一致时仍保持零写入。旧 workload 首次接管以及在 adopted generation 内的重复调谐仍只处理 metadata，避免 Agent 升级静默触发 Pod rollout；后续 Task generation 才开始收敛期望字段。

## 当前能力矩阵

| 能力 | Kubernetes | Docker | Raw |
| --- | --- | --- | --- |
| Task claim 与 finalizer | 支持 | 未实现 | 未实现 |
| Deployment | 支持 | 不适用 | 不适用 |
| StatefulSet | 支持 | 不适用 | 不适用 |
| DaemonSet | 支持 | 不适用 | 不适用 |
| 容器/进程执行 | Kubernetes Pod | 仅有 Spec 骨架 | 仅有 Spec 骨架 |
| 状态观察 | 支持 | 未实现 | 未实现 |
| 运行时日志 | Kubernetes Pod fallback | 未实现 | 未实现 |
| Exec | Kubernetes 路径 | 未实现 | 未实现 |
| 已有 PVC 引用与 generic ephemeral volume | 采用 Kubernetes 原生 Pod 语义；RLark 不拥有被引用 PVC | 未实现 | 未实现 |
| GPU 与设备 | Kubernetes resource 语义 | 未实现 | 未实现 |
| Addon 与 Delivery | 支持 | 未实现 | 未实现 |
| 跨集群 workload 网络 | Kubernetes sidecar 路径 | 未实现 | 未实现 |
| 镜像拉取观察 | 支持 | 仅有部分 monitor，无执行链路 | 不适用 |

该矩阵只描述当前行为。这些能力不会通过 Node 或其他外部资源发布。

## 契约扩展要求

Runtime Provider 与 Workload Adapter 可以扩展版本协商、幂等操作、状态观察、日志、Exec、恢复、服务身份和一致性测试。此类扩展需要单独进行 API 评审，并保持上述 Kubernetes 兼容契约和 Runtime selector 契约。
