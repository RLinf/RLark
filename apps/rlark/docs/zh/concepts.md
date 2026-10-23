# 核心概念

## 1. 资源层次

RLark 采用多层资源抽象，从底层基础设施到上层具身智能工作负载逐层封装：

```
  │
  └── Job  ──── 训练作业（一个完整的具身智能任务）
        │
        └── Task  ──── 任务单元（Actor/Rollout/Env 等角色）
              │
              └── K8s Workload / Docker Container  ──── 底层运行时
                    │
                    └── Pod / Container  ──── 实际运行的工作负载
```

**具身智能场景**：典型流水线包括 GPU 集群训练策略模型、端侧设备（机械臂、传感器）在物理环境中执行策略、数据回流到训练 —— 全链路通过同一平台编排。

## 2. Domain（网络域）

Domain 是 RLark 中**虚拟网络分组和寻址**的基本单位。

### 概念

一个 Domain 代表一个逻辑上的训练网络，同一 Domain 内的 Pod 可以通过虚拟 IP 通信，跨集群转发会校验 Domain 归属。Domain 不会隔离底层集群、节点、运行时、存储或普通 Kubernetes 网络；需要隔离时，仍须使用基础设施提供的网络策略和安全控制。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Domain
metadata:
  name: ppo-experiment-1
spec:
  cidr: "10.0.1.0/24"    # 该 Domain 的 IP 子网
status:
  ipAllocations:          # 已分配的 IP 列表
    - ip: "10.0.1.1"
      task: "actor-head"
      job: "ppo-cartpole-v1"
```

### 设计意图

- **网络隔离**：不同具身智能任务（如团队 A 的机械臂操作实验和团队 B 的导航实验）分配到不同 Domain，互不干扰
- **IP 管理**：Controller-Manager 的 Domain Controller 负责分配 IP 子网，并为每个工作负载分配唯一 IP
- **证书粒度**：每个 Domain 有独立的 X.509 证书，跨集群通信时携带 Domain 证书做身份认证

## 3. DomainPeer（域对等体）

DomainPeer 是 Domain 在**某个数据面集群中的视图**，由 Domain Controller 自动创建。

### 概念

每个 Domain 在每个数据面集群中有一个对应的 DomainPeer 对象，包含该 Domain 在该集群中所有 Pod 的集合。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: DomainPeer
metadata:
  name: ppo-experiment-1
  namespace: agent-beijing    # 该 Agent 的 workspace
spec:
  cert: "-----BEGIN CERTIFICATE-----..."  # Domain 证书
  key: "-----BEGIN PRIVATE KEY-----..."   # Domain 私钥
  prefixLen: 24
  pods:
    - name: "actor-head-0"
      namespace: "default"
      uid: "abc-123"
      ip: "10.0.1.1"           # Domain 虚拟 IP
      localIP: "172.17.0.5"    # 实际 Pod IP（k8s 集群内）
      node: "node-01"
      globalNamespace: "agent-beijing"
```

### 用途

- **路由表**：NodeServer 通过 DomainPeer 查找目标 Pod 所在集群和实际 IP
- **证书分发**：DomainPeer 携带 Domain 的证书和私钥，Agent 通过它建立跨集群 SSH 隧道
- **自动更新**：Pod 创建/删除时，Domain Controller 自动更新 DomainPeer 的 pods 列表

## 4. Node（计算节点）

Node 代表数据面集群中的一个**物理/虚拟计算节点**。

### 概念

Node 由 Agent 的 Push 控制器自动上报，反映节点的实际状态（地址、资源容量、GPU 数量等）。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Node
metadata:
  name: gpu-node-01
  namespace: agent-beijing
  labels:
    nvidia.com/gpu: "true"
spec:
  agentType: Kubernetes
  unschedulable: false
status:
  phase: Online
  addresses:
    - type: InternalIP
      address: "10.0.1.23"
    - type: Hostname
      address: "gpu-node-01"
  capacity:
    cpu: "32"
    memory: "128Gi"
    nvidia.com/gpu: "8"
  allocatable:
    cpu: "30"
    memory: "120Gi"
    nvidia.com/gpu: "8"
  nodeInfo:
    architecture: amd64
    operatingSystem: linux
    kernelVersion: "5.15.0-91-generic"
    agentVersion: "0.1.0"
```

### 用途

- **资源感知**：控制面根据 Node 的资源状态做调度决策（通过 NodeSelector）
- **标签管理**：控制面可下发标签到 Node，Agent 的 Pull 控制器同步到本地 k8s Node
- **污点管理**：通过 `unschedulable` 字段控制节点是否可调度

### 管理员标注

管理员可在管理端多选 Node 并批量设置由管理端和业务端共同使用的元数据：

- `rlark.io/city` Annotation：管理员维护的物理位置
- `rlark.io/node-category-{cloud,edge,robot}=true` Label：一个或多个节点分类
- `rlark.io/gpu-model` Annotation：云算力节点的 GPU 型号
- `rlark.io/device-model` Annotation：端算力和端真机节点的具身设备型号

这些业务字段存储在 KCP Node CR 上，Agent 刷新 Kubernetes 自动发现状态时会保留它们。批量编辑器会带入共同的当前值、标识多值选择，并且只修改管理员明确启用的属性；清空已启用属性表示删除该值。管理端同时支持全选筛选结果、取消全选及批量 Cordon/Uncordon。一个节点可同时属于多个分类，并同时具有 GPU 和具身设备型号。

业务平台的节点总数和集群详情节点列表包含带有 RLark 分类 Label 的可用 Worker；旧版 `rlark.io/node-category` 值及明确上报 GPU 或具身设备资源的未标注节点仍兼容展示。带有 Kubernetes `master` 或 `control-plane` 角色标签的节点仍只在管理平台中展示，不作为业务任务 Worker 统计和展示。

节点详情中的 CPU、内存和 GPU 占用量按运行 Worker 的 Kubernetes `resources.requests` 汇总，反映调度器已预留资源，不代表 metrics-server 的实时硬件利用率。磁盘数据单独来自 kubelet Stats Summary：Agent 汇总 nodefs 与独立 imagefs，并在二者指向同一文件系统时避免重复计算。页面展示真实已使用、总量和剩余量，使用率达到 90% 或 kubelet 上报 `DiskPressure=True` 时显示告警。详情页同时列出该节点上的 Worker、所属 Job、角色、IP、资源申请和运行状态。

## 5. Node 调度控制（Cordon/Uncordon）

通过 `unschedulable` 字段控制节点是否可调度。

### 概念

设置 `spec.unschedulable: true` 将节点标记为不可调度，阻止新工作负载分配到该节点，等同于 Kubernetes 的 `kubectl cordon`。

### 使用方式

```bash
# PATCH 切换不可调度状态
curl -X PATCH "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{ "spec": { "unschedulable": true } }'
```

Web UI 在节点列表中为每个节点提供 Cordon/Uncordon 按钮。

## 6. Job（训练作业）

Job 是用户**直接操作的核心资源**，代表一个完整的 RL 训练任务。

### 概念

一个 Job 包含多个 Task 模板，每个 Task 代表训练中的一个角色（如 Actor、Rollout、Env）。Controller-Manager 的 Job Controller 根据模板创建和管理 Task。

分发任何 Task 前，Job Controller 会把所有模板作为一个整体逐一校验。模板名称必须非空且唯一，`runtime.kind`、workload kind/version 与 carrier type/version 必须是非空的开放标识符。对于当前 Job 到 Ray-on-Kubernetes 路径，controller 还要求提供 Pod 模板，并且其中必须且只能有一个名为 `main` 的普通容器。这是实现能力校验，不是通用 Task CRD 约束：CRD 允许省略 `workload.template`，其他 Workload Adapter/Runtime 组合自行定义要求。任一模板无效时，controller 不会创建其中任何 Task，而是在 Job 上报告 `Validated=False` 并通过消息指出失败字段路径；它不会静默跳过单个模板。Agent 会在 Runtime effect 前独立校验所选实现。Job Controller 不校验 Ray head 数量或 head 副本拓扑。

Web UI 保持现有的 Job 和 Workflow 创建、编辑、克隆、生命周期、存储及 Worker 交互不变，但会直接按生成的 CRD 合同序列化数据。新请求使用 `runtime`、`workload`、`carrier`、`placement` 和 `environments`；生命周期操作写入 `spec.suspended`；存储选择写成 Kubernetes 原生 Pod 模板 volume。UI 不再发送已移除的 `agentType`、嵌套 `kubernetes.workload`、`stopped`、`pvcStorageMap` 或 `pvcSizeGbMap` 结构。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Job
metadata:
  name: ppo-cartpole-v1
  labels:
    framework: ppo
    env: cartpole
spec:
  domain: ppo-experiment-1    # 生成的 Task 接入的虚拟网络
  tasks:
    - name: actor-head
      head: true               # 是否为主节点
      role: Actor              # 角色：Actor/Rollout/Env
      placement:
        nodeSelector:
          nvidia.com/gpu: "true"
      runtime:
        kind: Kubernetes
      workload:
        kind: rlinf.io/ray
        version: v1
        parameters:
          role: head
          prepare:
            inline: pip install -r requirements.txt
          run:
            inline: python main.py --role=head
        carrier:
          type: rlinf.io/kubernetes-deployment
          version: v1
          parameters:
            replicas: 1
        template:
          spec:
            containers:
              - name: main
                image: pytorch/pytorch:2.3.0
                resources:
                  limits:
                    nvidia.com/gpu: "1"
    - name: rollout
      role: Rollout
      runtime:
        kind: Kubernetes
      workload:
        kind: rlinf.io/ray
        version: v1
        parameters:
          role: worker
          headTaskName: ppo-cartpole-v1-actor-head
        carrier:
          type: rlinf.io/kubernetes-statefulset
          version: v1
          parameters:
            replicas: 2
        template: ...
status:
  phase: Running               # Pending/Running/Succeeded/Failed
  startTime: "2026-06-25T03:20:00Z"
  tasks:
    - name: actor-head
      phase: Running
    - name: rollout
      phase: Running
```

### 状态机

```
(空) ──init──▶ Pending ──tasks-running──▶ Running
                     │             │              │
                     │ stop        │ stop         │ all-tasks-succeeded
                     ▼             ▼              ▼
                  Stopping ──cleanup-complete──▶ Stopped    Succeeded
                     ▲
                     │ failed-run stop/restart cleanup
                   Failed
```

`Stopping` 是过渡状态：RLark 会删除子 Task，并等待其 Worker 清理完成。引用的外部 PVC 不由 RLark 管理；由 `ephemeral.volumeClaimTemplate` 创建的 Pod 级临时 PVC 仍由 Kubernetes 管理并随 Pod 删除。停止后再启动终态 Job 会创建新的 Task 和 Worker，但不能据此假定持久存储为空。

### 与 Task 的关系

Job Controller 调谐完成后：
- 每个 Task 模板生成一个 Task CR，名称为 `<job-name>-<task-name>`
- Task 带有 `rlinf.io/job=<job-name>` 标签，可据此查询
- Task 设置 OwnerReference 指向 Job，删除 Job 时级联删除 Task

## 7. Job 停止/启动

通过 Job spec 中的 `suspended` 字段，可以停止和重启 Job，提供对运行中工作负载的手动生命周期控制。

### 概念

将 Job 的 `spec.suspended` 设为 `true` 会启动有序停止流程，但不删除 Job 资源。RLark 等待 Job 及其所有子 Task 和 Worker 停止期间显示 `Stopping`，完成后进入 `Stopped`。`ArchiveTTL`（默认 5 分钟）后，Archive Controller 会从 kcp 删除 Job CR（归档），持久化后端（PostgreSQL 或 Kubernetes Secret，测试模式）保留完整配置和最终状态；之后通过 `POST /jobs/:name/start` 从持久化后端恢复同名同 spec 的新 CR。

### 工作原理

1. **停止**：RLark 删除子 Task CR，等待其底层工作负载清理完成，同时保留 Job CR 和配置；引用的外部 PVC 仍不由 RLark 管理；Pod 级临时 PVC 随 Pod 删除
2. **归档**：`ArchiveTTL` 后，Archive Controller 从 kcp 删除 Job CR。持久化后端保留完整 Job 记录（spec + status）；删除 Job CR 释放 etcd 和 controller 内存，业务记录仍可查询可恢复
3. **启动**：`POST /jobs/:name/start` 是统一启动接口。活跃 Job（KCP 有 CR）直接 PATCH `spec.suspended=false`；归档 Job（KCP CR 已删）从持久化后端恢复同名同 spec 的新 CR，并置 `spec.suspended=false`。新 CR 的 Kubernetes UID 会变化，但业务标识（name）不变
4. **重启**：Job controller 将 restart annotation 视为 Job 层操作标识，删除当前 Task 对象，等待每个 Agent-owned cleanup finalizer 完成 Runtime cleanup，再按模板创建具有新 Kubernetes UID 的 Task 对象；Agent 本身不解释 restart annotation。引用的外部 PVC 不受影响，Pod 级临时 PVC 会替换；`Succeeded` 和 `Failed` Job 都会按模板开始一轮新运行
5. **删除**：先执行停止并等待完成，再删除 Job 和子 Task；RLark 不会删除引用的外部 PVC 和 hostPath 数据，Pod 级临时 PVC 随 Pod 删除；持久化后端记录同步删除（PostgreSQL 行或 Secret）

### 关键特性

- **外部持久存储**：停止、重启和删除不会管理通过 `persistentVolumeClaim.claimName` 引用的 PVC；其生命周期由管理员负责
- **引用 claim**：新 Task 可显式引用同一个已有 PVC。Pod 级 `ephemeral.volumeClaimTemplate` PVC 不同，由 Kubernetes 随 Pod 创建和删除
- **统一启动接口**：`POST /jobs/:name/start` 同时支持活跃和归档 Job；前端调用时无需感知 Job 是否在 kcp 中
- **Web UI 集成**：Web UI 提供停止、启动、重启和删除操作，并根据生命周期状态控制可用性

停止 Workflow 时，RLark 会删除当前运行的全部 Job，包括已经完成的 Job。恢复 Workflow 会创建一轮全新执行，从 DAG 起点重新运行，不复用上一轮的完成状态。

## 8. Task（任务单元）

Task 是 Job 的**执行单元**，由 Job Controller 自动创建，**用户不应直接创建 Task**。

### 概念

每个 Task 代表一个具体的"训练角色实例"，Agent 的 Pull 控制器监听到 Task 后，创建对应的 K8s 工作负载。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Task
metadata:
  name: ppo-cartpole-v1-actor-head
  namespace: agent-beijing
  labels:
    rlinf.io/job: ppo-cartpole-v1
spec:
  role: Actor
  placement:
    nodeSelector:
      nvidia.com/gpu: "true"
  runtime:
    kind: Kubernetes
  workload:
    kind: rlinf.io/ray
    version: v1
    parameters:
      role: head
    carrier:
      type: rlinf.io/kubernetes-deployment
      version: v1
      parameters:
        replicas: 1
    template:
      metadata:
        labels:
          app: ppo-trainer
      spec:
        containers:
          - name: trainer
            image: pytorch/pytorch:2.3.0
  environments:
    - type: rlinf.io/network
      version: v1
      parameters:
        domain: ppo-experiment-1
status:
  phase: Running
  observedNodes: ["gpu-node-01"]
  startTime: "2026-06-25T03:20:30Z"
  conditions:
    - type: Ready
      status: "True"
      reason: PodRunning
```

### 任务角色

| 角色 | 说明 | 典型用途 |
|------|------|----------|
| `Actor` | 执行策略推理，生成训练数据 | PPO 的 Actor 进程 |
| `Rollout` | 环境交互，收集轨迹 | 游戏环境模拟器 |
| `Env` | 环境/辅助服务 | 数据预处理、参数服务器 |

### Runtime 类型

| 形态 | 说明 | 适用场景 |
|------|------|----------|
| `Kubernetes` | 使用 Deployment、DaemonSet 或 StatefulSet 作为 Kubernetes carrier 运行 RLark/Ray Task | GPU 集群大规模训练 |
| `Docker` | 计划中的 Provider，把统一 Pod template lowering 为 Docker 容器；当前未实现、未广告 | 端侧设备（机械臂、传感器、摄像头） |
| `Raw` | 计划中的 Provider，把统一 Pod template lowering 为宿主机进程；当前未实现、未广告 | 裸金属服务器、嵌入式设备 |

`runtime.kind`、`workload.kind` 与 `workload.carrier.type` 是开放标识，而不是 CRD enum。开放性使 Provider 可在不替换 Task Schema 的情况下扩展，但不表示相应能力已受支持。Agent 必须在任何外部副作用前解析已注册 Provider 与 Adapter，并完成能力校验。Workload 语义使用 `kind`、`version` 和 `parameters`；carrier 独立使用 `type`、`version` 和 `parameters`。`environments` 是有序列表，同一种 Environment 可使用不同参数重复装配。

**具身智能场景映射**：

```mermaid
graph LR
    subgraph Cloud["云端 (k8s)"]
        Training["训练 (GPU)"]
        Env["环境模拟 (Env)"]
    end
    subgraph Edge["端侧 (k8s / Docker / Raw)"]
        Robot["机械臂 (推理)"]
        Camera["摄像头 (传感器)"]
    end
    Training -->|"梯度"| Robot
    Training <-->|"数据"| Robot
    Env -->|"控制"| Camera
    Env <-->|"观测"| Camera
```

## 9. Pod（容器实例）

Pod CR 是数据面 Pod 的**控制面镜像**，由 Agent 的 Push 控制器上报。

### 概念

当数据面集群中创建了 Pod 后，Agent 的 Pod Push 控制器会将 Pod 信息上报到控制面，创建对应的 Pod CR。Pod CR 包含 Pod 的标识信息（名称、所属 Task）和运行状态（IP、节点、阶段）。

### 用途

- **状态追踪**：控制面通过 Pod CR 了解底层 Pod 的实时状态
- **SSH 查找**：Server 的 PodCache 基于 Pod CR 快速定位 Pod 所在 Agent
- **日志查询**：Gateway 通过 Pod CR 找到 Pod 所在 Agent，转发日志请求

## 9. 资源关系总结

```mermaid
graph TD
    job -->|"1:N"| task["Task<br/>(Namespaced: agent-{id})<br/>任务执行单元"]
    task -->|"1:1 (K8s workload)"| workload["Deployment /<br/>DaemonSet /<br/>StatefulSet<br/>(本地 k8s 集群)"]
    workload -->|"1:N"| pod["Pod + Sidecar<br/>Agent Push 上报 → Pod CR"]
    domain["Domain<br/>(Cluster scoped)<br/>网络隔离"] -->|"1:N (每个集群一个)"| dp["DomainPeer<br/>(Namespaced: agent-{id})<br/>Pod 路由表"]
    node["Node<br/>(Namespaced)<br/>计算节点信息"]
```

## 9. 命名约定

| 命名空间前缀 | 含义 | 示例 |
|-------------|------|------|
| `agent-` | Agent 的 workspace | `agent-beijing-01` |
| `rlark-system` | Agent 创建的默认 namespace | `rlark-system` |
| Label `rlinf.io/job` | Pod/Task 所属 Job | `rlinf.io/job=ppo-cartpole-v1` |
| Annotation `rlinf.io/ray-role` | Ray 集群角色 | `head` / `worker` |

## 9. Ray 集群集成

Ray 配置由版本为 `v1` 的 `rlinf.io/ray` workload 的 `spec.workload.parameters` 承载。worker 使用 `parameters.headTaskName` 引用同一 namespace 中的具体 head Task；Job Controller 还会写入 launcher 使用的派生值 `parameters.totalNodes` 和 `parameters.nodeRankStart`。实现必须验证可信 Job 关系，不能只因名称匹配就信任目标对象。既有 Ray annotation 仅作为实现兼容元数据，不应作为新 Task 的 canonical 声明：

```yaml
annotations:
  rlinf.io/ray-role: "head"          # head | worker
  rlinf.io/ray-total-nodes: "5"     # 仅 head 需要
  rlinf.io/ray-head-task-name: "actor-head"  # 仅 worker 需要
```

**进程装配与启动流程**：

1. Agent 的 Pull 控制器解析 `rlinf.io/ray/v1`，并严格解码 `role`、`headTaskName`、`prepare.inline` 和 `run.inline`
2. Workload Adapter 只将 `ray-launcher` 声明为 required primary process，SSH Environment Handler 将 `sshd` 声明为 optional auxiliary process。Tools 安装与 supervisor 是 Runtime 强制能力，不需要 Task 或 Adapter 声明。如果 workload unit 没有声明 primary，assembler 会补充 `/rlark-tools/rlark-tools idle`。SSH 故障不会停止 Task：首次启动失败会使 SSH 不可用，非零退出会获得有限次数的重启尝试；重启启动失败或耗尽尝试后 SSH 同样保持不可用。Network 和 TensorBoard 仍是独立管理的 Environment 功能。
3. Binder 的 Runtime workload assembler 安装 `rlark-tools`，写入归当前 Task incarnation 所有的 supervisor ConfigMap，并把 main container 入口改为 `rlark-tools supervisor`。supervisor 将 `ray-launcher` 与 `sshd` 作为独立 OS 子进程运行；RLark 自有的 Ray 编排不再使用挂载的 shell 脚本。
4. Adapter 消费 canonical workload 参数并注入 launcher 环境，包括总节点数、rank 元数据与 Head Pod IP。具体而言，没有网络配置时，worker 的 `parameters.headTaskName` 会转换为同一 namespace 中的 Head Service DNS 名称；声明 Network Environment 时则转换为 `<headTaskName>-0.rlark-domain`。后一种情况下 Adapter 还会启用 launcher 内置的网络屏障：执行 prepare script 前，Head 等待自身 `.rlark-domain` 名称可解析，Worker 等待 Head 虚拟域名可解析；取消操作会终止等待。该 Go 实现替代了已不再生效的外部 `WAIT_NETWORK_SCRIPT` 兼容 hook。这些值不以兼容 annotation 为来源。
5. Head 节点创建属于当前 Task incarnation 的 Service（暴露 6379/8265/8080 端口）。RLark拥有的Service字段发生漂移时使用删除后重建的Replace语义，可能短暂中断访问并改变API Server分配的地址或端口；仅有Kubernetes默认值和分配字段不会触发Replace。
6. `ray-launcher` 在执行 Ray 命令前会确认配置的 Ray CLI 可执行。随后 Worker 解析并等待 Head 后加入集群。Head 等待声明的节点数量并运行 `ray status`；为保持兼容，集群就绪检查失败仍不作为致命错误，但 `ray status` 失败属于致命错误。

supervisor 配置变化会更新 Pod template revision annotation，从而触发 workload rollout。首次迁移 rollout 后，SSH 公钥轮换只更新独立 projection 的 Secret；原始 key bytes 不会写入 supervisor ConfigMap。

关闭顺序以 primary 为先。Pod 终止时，supervisor 先请求 `ray-launcher` 停止并等待它实际退出，此阶段不启动 supervisor 自身的超时；若 primary 卡住，Kubernetes 的 Pod 终止宽限期仍是最终边界。只有 primary 退出后，supervisor 才终止 `sshd` 等已声明 auxiliary，并给予它们最多 15 秒，随后强制结束仍未退出的进程。

Ray 平台端口仍是固定兼容合同：集群 6379、dashboard 8265、metrics 8080、head SSH 22、worker SSH 2222。Task/Job 配置可控制 role、replica（据此派生总节点数与 rank 范围）、head Task、domain、`prepare.inline` 和 `run.inline`。workload image 仍可设置 `RAY_TEMP_DIR` 以及既有的 `RAY_health_check_*` / `RAY_num_heartbeats_timeout` 环境变量。重试次数和端口目前不是 Task 级配置。单独调用 `ray-launcher` 命令时还可指定 Ray、Bash 与 Python binary flag，但内置 Adapter 使用其默认值。

## 14. 对象存储与 PVC

RLark 通过 Kubernetes 通用临时卷为训练任务挂载远程存储。

### 概念

每个 Pod 根据卷中的 `ephemeral.volumeClaimTemplate` 获得一个 PVC。该 PVC 由 Kubernetes 管理并随 Pod 删除。

### 配置方式

```yaml
workload:
  template:
    spec:
      volumes:
        - name: data
          ephemeral:
            volumeClaimTemplate:
              spec:
                accessModes: [ReadWriteOnce]
                storageClassName: ceph-rbd
                resources:
                  requests:
                    storage: 10Gi
```

### 工作流程

1. 前端通过 `GET /api/v1/storage/storageclass?clusters=<agent-id>` 获取 RLark 管理的 StorageClass 声明；该操作仅筛选已配置的目标集群，不会查询 Agent。
2. 所选存储类和申请容量写入 `volumeClaimTemplate`。
3. 声明下发完成后，Kubernetes 为每个 Pod 创建一个 PVC，并通过所选 StorageClass 绑定存储。
4. Pod 删除时，Kubernetes 自动删除对应 PVC。

列表中的声明表示已配置的分发意图。PVC 供给还依赖异步下发成功，以及目标集群中的 rclone CSI 驱动正常运行。

`pvcStorageMap` 和 `pvcSizeGbMap` 不属于 canonical Task Schema。请使用 `ephemeral.volumeClaimTemplate`；升级部署时可能需要删除并重建旧 Task 或 Job。

## 15. 用户认证

RLark 使用短期 JWT access token 认证 Web UI 和 API 请求。经验证的 `admin` 与 `user` 角色用于执行粗粒度 API 授权：平台业务操作对两个角色开放，控制面配置和凭据管理仅允许 `admin`。

### 认证流程

1. 部署时，`rlarkadm` 生成随机密码和 JWT 签名密钥，并存储在 KCP Secret（`rlark-ui-auth`）中
2. Web UI 发送 `POST /api/v1/auth/login` 携带用户名和密码
3. Gateway 对比 Secret 中的凭据，返回包含主体、角色、签发时间和过期时间的 HS256 JWT
4. 前端将 token 存储在 `sessionStorage`，后续 API 请求携带 `Authorization: Bearer <token>`；Gateway 在分发受保护路由前验证 token

`user` 可管理 Job、Workflow、Task、Pod、终端会话、SSH 密钥和存储对象，并读取集群、节点、Domain、镜像、StorageClass 和系统配置。节点和 Domain 变更、证书、镜像仓库、系统配置更新、StorageClass 管理以及 Addon 管理要求 `admin`。这是角色级授权，尚未实施逐用户资源所有权隔离，包括 SSH 密钥所有权限制。

## 16. Addon（组件管理）

Addon 是 RLark 的组件管理系统，允许用户在多个数据面集群中安装、配置和管理第三方组件（设备插件、监控代理等）。

### 概念

Addon 通过三个层次进行管理：

1. **Addon 目录**（`/api/v1/addons`）— 精选的可用 Addon 列表（如 `embodied-runtime-device-plugin`）。每个 Addon 包含 Kubernetes 清单（DaemonSet、ConfigMap、RBAC）和可配置的值。
2. **Addon CRD**（`rlinf.io/v1alpha1/Addon`）— 代表在特定数据面集群中安装的 Addon 实例的 Kubernetes 风格 CR。Spec 指定 Addon 名称、版本和配置值。
3. **Addon 控制器**（`../pkg/agent/controllers/addon/pull.go`）— Agent 的 Pull 控制器监听 Addon CR，使用配置的值渲染 Addon 清单，并应用到本地 Kubernetes 集群。

### Addon 生命周期

```
目录项 ──安装──▶ Addon CR ──pull──▶ Agent 应用清单
                    │
                    ├── 更新 values ──▶ Agent 重新应用
                    └── 删除 ──▶ Agent 移除清单
```

### Addon 状态

```
Pending ──▶ Installing ──▶ Ready
                │
                └──▶ Failed
                      │
                      └──▶ Upgrading ──▶ Ready
```

### 示例：具身运行时设备插件

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Addon
metadata:
  name: embodied-device-plugin
  namespace: agent-beijing
spec:
  addonName: embodied-runtime-device-plugin
  version: "0.1.0"
  values:
    image: "rlark/embodied-device-plugin:0.1.0"
    nodeSelector: "nvidia.com/gpu=true"
status:
  phase: Ready
  version: "0.1.0"
```

### 关键特性

- **多集群**：每个数据面集群可以安装不同的 Addon
- **可配置**：Addon 的值可通过 `spec.values` 按集群自定义
- **版本化**：Addon 支持通过 `spec.version` 进行版本升级
- **自动应用**：Agent 的 Pull 控制器在 Addon CR 创建或更新时自动应用清单
- **Mutating Webhook**：Device Plugin 内置 mutating admission webhook，自动向申请 `rlinf.io/device` 的 Pod 注入 `devinit` init 容器，在 Pod 网络命名空间中创建 macvlan，无需手动配置

## 17. Web Terminal（Web 终端）

Web Terminal 提供从 Web UI 直接交互式访问 Pod 终端的能力。

### 概念

Web Terminal 允许用户打开任何 RLark 管理的 Pod 的终端会话，无需 SSH 到底层节点或本地安装 kubectl。

### 架构

```
浏览器 ──WebSocket──▶ Gateway ──通过 Server 代理──▶ Agent ──exec──▶ Pod
```

### 工作原理

1. 用户在 Web UI 中点击 Pod 的“终端”按钮，会在浏览器新标签页中打开会话，因此可以同时打开多个 Pod 终端
2. 新标签页通过 WebSocket 连接到 Gateway 的 `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal`
3. Gateway 通过 Server 将 WebSocket 连接代理到 Pod 所在的 Agent
4. Agent 打开 Pod 容器的 exec 会话，流式传输 I/O
5. 终端会话保持到 WebSocket 关闭

Web Terminal 支持在 macOS Safari 中使用 Vim 等交互式全屏程序。可打印字符和终端控制键会直接传给终端，同时保留浏览器快捷键及输入法组合输入的正常行为。
Shell 退出（例如执行 `exit`）时，代理链会转发 WebSocket 正常关闭帧；非零退出状态会在页面保留退出码，传输故障则与进程退出明确区分。

## 18. Pod HTTP Proxy（Pod HTTP 代理）

Pod HTTP Proxy 允许通过 Server → Agent 代理链直接向 RLark 管理的 Pod 发送 HTTP 请求。

### 概念

Pod HTTP Proxy 使用户无需知道 Pod 的真实 IP 地址或建立 SSH 隧道即可向特定 Pod 发送 HTTP 请求。Server 通过 Pod 缓存解析 Pod 位置，并通过 Agent 的本地 HTTP 服务器代理请求。

### 架构

```
客户端 ──HTTP──▶ Gateway ──通过 Server 代理──▶ Agent ──反向代理──▶ Pod (http://<podIP>:<port>/<path>)
```

### 工作原理

1. 客户端向 Gateway 发送 `ANY /api/podproxy/{podName}:{port}/*path` 或 `ANY /api/taskproxy/{taskName}:{port}/*path`
2. Gateway 将请求转发到 Server
3. Server 在 Pod 缓存中查找 Pod（通过 Pod 名称或 Task 名称），获取 Pod 的真实 IP 和 Agent ID
4. Server 构造到目标 Agent 的反向代理，目标为 `0.0.0.0:1`（Agent 的本地服务器）
5. Agent 收到 `/api/proxy/http://<podIP>:<port>/<path>` 请求，反向代理到 Pod
6. 响应通过相同链路返回

### 关键特性

- **无需 Pod IP**：用户只需 Pod 名称或 Task 名称和端口
- **双重解析**：支持通过 Pod 名称（`/api/podproxy/`）或 Task 名称（`/api/taskproxy/`）代理
- **基于证书的访问控制**：访问权限由客户端证书的权限决定
- **透明代理**：支持所有 HTTP 方法（GET、POST、PUT、DELETE 等）

## 19. TensorBoard Proxy（TensorBoard 代理）

TensorBoard Proxy 提供基于 Web 的训练指标可视化仪表板（损失曲线、标量摘要、直方图等），可直接从 RLark Web UI 访问，无需对外暴露 TensorBoard 端口。

### 概念

当训练 Task 运行 TensorBoard（监听端口 6006）时，RLark 通过 Gateway 自动代理 TensorBoard UI。用户可以在 Web UI 中点击链接打开 TensorBoard，浏览器通过 Gateway 代理访问。

### 架构

```
浏览器 ──HTTP──▶ Gateway ──代理到 Server──▶ Server ──podproxy──▶ Agent ──▶ Pod:6006 (TensorBoard)
```

### 工作原理

1. `rlinf.io/tensorboard/v1` Environment Handler 在 `parameters.logDir` 非空时将 TensorBoard 声明为可重启的 Kubernetes sidecar；它独立于 Ray supervisor process
2. Gateway 在 Task 状态响应中注入 `tensorBoardProxy` 字段，指向代理 URL
3. 用户在 Web UI 中点击 TensorBoard 链接
4. 浏览器发送请求到 `GET /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/`
5. Gateway 通过 KCP API 解析 Task 对应的 Pod，并将请求代理到 Server 的 Pod Proxy
6. Gateway 重写 HTML/CSS 响应，确保所有资源路径（字体、JS、API 调用）通过代理前缀正常工作
7. TensorBoard UI 在浏览器中渲染，如同直接访问

### 关键特性

- **无需端口暴露**：TensorBoard 的 6006 端口保持在 Pod 内部
- **自动代理注入**：Task 列表/详情响应自动包含 `tensorBoardProxy` URL
- **HTML 重写**：代理的 TensorBoard 页面经过重写，确保所有相对和绝对路径正确工作
- **显式集成**：Task 请求以下 Environment 时启动 TensorBoard；它不由 `rlark-tools supervisor` 管理

```yaml
environments:
  - type: rlinf.io/tensorboard
    version: v1
    parameters:
      logDir: /data/tensorboard
```

## 20. SSH 密钥管理

SSH 密钥管理允许用户通过 API 或 Web UI 上传 SSH 公钥。已登记的密钥用于以对应用户名认证到 RLark SSH 堡垒机。workload SSH 通过 `rlinf.io/ssh/v1` 声明，其 `parameters.authorizedKeys` 数组是公钥认证配置，可保留在 Agent 本地 execution snapshot 中，但不会进入 portable Plan 或远程 workload transport。

### 概念

密钥按用户名存储在控制平面命名空间的 Kubernetes Secret（`rlark-ssh-user-keys`）中。SSH Server 会验证客户端密钥是否登记在当前 SSH 用户名下。当前实现**没有**检查该用户是否获准访问某个 Pod，即堡垒机认证通过后不存在按用户或按 Pod 的授权策略。创建 Job 时选择密钥与堡垒机授权是两件独立的事：前者只是把公钥写入生成的工作负载配置。

### API

- `GET /api/v1/ssh-user-keys` — 列出所有 SSH 密钥（可按用户过滤）
- `POST /api/v1/ssh-user-keys` — 为用户添加新的 SSH 公钥
- `DELETE /api/v1/ssh-user-keys/:id` — 按索引删除密钥

### 关键特性

- **Web UI 管理**：Web UI 中提供专用的 SSH 密钥管理页面
- **按任务注入**：workload 通过 `rlinf.io/ssh/v1` 和 `authorizedKeys` 数组请求 SSH Environment。公钥字节只作为 Task-scoped desired-state material，不得进入 portable Plan、远程协议、Status、日志、错误、revision 或 operation key
- **冲突检测**：重复密钥会被检测并返回 409 响应
- **写入重试**：API 在写入冲突时自动重试（最多 5 次）
- **密钥验证**：使用 `golang.org/x/crypto/ssh` 在存储前验证公钥格式
