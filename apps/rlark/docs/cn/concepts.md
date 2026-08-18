# 核心概念

## 1. 资源层次

rlark 采用多层资源抽象，从底层基础设施到上层具身智能工作负载逐层封装：

```
Workflow  ──── 工作流（DAG 编排多个 Job）
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

## 2. Domain（安全域）

Domain 是 rlark 中**网络隔离**和**安全边界**的基本单位。

### 概念

一个 Domain 代表一个逻辑上的训练网络，同一 Domain 内的 Pod 可以通过虚拟 IP 直接通信，不同 Domain 之间完全隔离。

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

节点详情中的 CPU、内存和 GPU 占用量按运行 Worker 的 Kubernetes `resources.requests` 汇总，反映调度器已预留资源，不代表 metrics-server 的实时硬件利用率。详情页同时列出该节点上的 Worker、所属 Job、角色、IP、资源申请和运行状态。

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
  domain: ppo-experiment-1    # 所属 Domain
  tasks:
    - name: actor-head
      head: true               # 是否为主节点
      role: Actor              # 角色：Actor/Rollout/Env
      agentType: Kubernetes    # 接入形态：Kubernetes/Docker/Raw
      nodeSelector:            # 节点选择器
        nvidia.com/gpu: "true"
      kubernetes:
        workload:
          kind: Deployment     # 工作负载类型
          replicas: 1
          template:
            spec:
              containers:
                - name: trainer
                  image: pytorch/pytorch:2.3.0
                  resources:
                    limits:
                      nvidia.com/gpu: "1"
    - name: rollout
      role: Rollout
      agentType: Kubernetes
      kubernetes:
        workload:
          kind: StatefulSet
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
                              │                  │
                              │ any-task-failed  │ all-tasks-succeeded
                              ▼                  ▼
                          Failed            Succeeded
```

### 与 Task 的关系

Job Controller 调谐完成后：
- 每个 Task 模板生成一个 Task CR，名称为 `<job-name>-<task-name>`
- Task 带有 `rlinf.io/job=<job-name>` 标签，可据此查询
- Task 设置 OwnerReference 指向 Job，删除 Job 时级联删除 Task

## 7. Job 停止/启动

通过 Job spec 中的 `stopped` 字段，可以停止和重启 Job，提供对运行中工作负载的手动生命周期控制。

### 概念

将 `spec.stopped: true` 设置为 Job 会通知 Job 控制器停止所有关联的工作负载（Pod、Deployment、StatefulSet），但不删除 Job 资源。将其设置回 `false`（或移除该字段）会重新启动工作负载。

### 工作原理

1. **停止**：当 `spec.stopped` 设置为 `true` 时，Job 控制器检测到变化并删除底层 Kubernetes 工作负载（Deployment/StatefulSet），同时保留 Job CR
2. **重启**：当 `spec.stopped` 被移除或设置为 `false` 时，Job 控制器根据 Task 模板重新创建工作负载
3. **状态保留**：Job 的 phase 和 status 字段在停止/重启周期中保持不变

### 关键特性

- **非破坏性**：停止 Job 不会删除 Job CR 或其 Task
- **持久化状态**：PVC 和其他持久化资源不受停止影响
- **Web UI 集成**：Web UI 在 Job 列表中提供一键停止/启动按钮

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
  agentType: Kubernetes
  domain: ppo-experiment-1
  nodeSelector:
    nvidia.com/gpu: "true"
  kubernetes:
    workload:
      kind: Deployment
      replicas: 1
      template:
        namespace: default
        spec:
          containers:
            - name: trainer
              image: pytorch/pytorch:2.3.0
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

### 接入形态

| 形态 | 说明 | 适用场景 |
|------|------|----------|
| `Kubernetes` | 创建 K8s 原生工作负载（Deployment/DaemonSet/StatefulSet） | GPU 集群大规模训练 |
| `Docker` | 通过 Docker API 管理容器（TODO） | 端侧设备（机械臂、传感器、摄像头） |
| `Raw` | 下载 artifact 后直接执行二进制（TODO） | 裸金属服务器、嵌入式设备 |

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

## 9. Workflow（工作流）

Workflow 是**多 Job 的 DAG 编排**，支持有依赖关系的训练流水线。

### 概念

一个 Workflow 包含多个 Job 模板，每个模板可通过 `dependencies` 声明前置依赖。Workflow Controller 按拓扑顺序调度 Job：前置 Job 成功后，依赖它的 Job 才能启动。

### 关键属性

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Workflow
metadata:
  name: training-pipeline-v1
spec:
  jobTemplates:
    - name: prepare-data
      dependencies: []            # 无依赖，立即启动
      spec:
        tasks:
          - name: prep
            role: Env
            agentType: Kubernetes
            kubernetes: ...
    - name: train
      dependencies: ["prepare-data"]  # 等 prepare-data 成功后才启动
      spec:
        tasks:
          - name: actor-head
            head: true
            role: Actor
            agentType: Kubernetes
            kubernetes: ...
    - name: evaluate
      dependencies: ["train"]
      spec:
        tasks: ...
```

### 典型流水线

```
数据准备 ──▶ 模型训练 ──▶ 模型评估
 prepare      train       evaluate
```

### 状态机

与 Job 类似，Workflow 的状态由各 Job 的状态汇总决定：
- 所有 Job 成功 → Workflow Succeeded
- 任一 Job 失败 → Workflow Failed

## 10. Pod（容器实例）

Pod CR 是数据面 Pod 的**控制面镜像**，由 Agent 的 Push 控制器上报。

### 概念

当数据面集群中创建了 Pod 后，Agent 的 Pod Push 控制器会将 Pod 信息上报到控制面，创建对应的 Pod CR。Pod CR 包含 Pod 的标识信息（名称、所属 Task）和运行状态（IP、节点、阶段）。

### 用途

- **状态追踪**：控制面通过 Pod CR 了解底层 Pod 的实时状态
- **SSH 查找**：Server 的 PodCache 基于 Pod CR 快速定位 Pod 所在 Agent
- **日志查询**：Gateway 通过 Pod CR 找到 Pod 所在 Agent，转发日志请求

## 11. 资源关系总结

```mermaid
graph TD
    wf["Workflow<br/>(Cluster scoped)<br/>DAG 编排"] -->|"1:N"| job["Job<br/>(Cluster scoped)<br/>训练任务定义"]
    job -->|"1:N"| task["Task<br/>(Namespaced: agent-{id})<br/>任务执行单元"]
    task -->|"1:1 (K8s workload)"| workload["Deployment /<br/>DaemonSet /<br/>StatefulSet<br/>(本地 k8s 集群)"]
    workload -->|"1:N"| pod["Pod + Sidecar<br/>Agent Push 上报 → Pod CR"]
    domain["Domain<br/>(Cluster scoped)<br/>网络隔离"] -->|"1:N (每个集群一个)"| dp["DomainPeer<br/>(Namespaced: agent-{id})<br/>Pod 路由表"]
    node["Node<br/>(Namespaced)<br/>计算节点信息"]
```

## 12. 命名约定

| 命名空间前缀 | 含义 | 示例 |
|-------------|------|------|
| `agent-` | Agent 的 workspace | `agent-beijing-01` |
| `rlark-system` | Agent 创建的默认 namespace | `rlark-system` |
| Label `rlinf.io/job` | Pod/Task 所属 Job | `rlinf.io/job=ppo-cartpole-v1` |
| Annotation `rlinf.io/ray-role` | Ray 集群角色 | `head` / `worker` |

## 13. Ray 集群集成

rlark 支持通过 Task 注解声明式创建 Ray 集群：

```yaml
annotations:
  rlinf.io/ray-role: "head"          # head | worker
  rlinf.io/ray-total-nodes: "5"     # 仅 head 需要
  rlinf.io/ray-head-task-name: "actor-head"  # 仅 worker 需要
```

**自动初始化流程**：

1. Agent 的 Pull 控制器检测到 Ray 注解
2. 创建 ConfigMap 挂载初始化脚本（`ray_head.sh` / `ray_worker.sh` / `ray_check.py`）
3. 修改容器启动命令为 `bash ray_head.sh`（或 `ray_worker.sh`）
4. 注入环境变量（`RLARK_RAY_PORT`、`RLARK_HEAD_ADDRESS` 等）
5. Head 节点创建 Service（暴露 6379/8265/8080 端口）
6. Worker 节点等待 Head 就绪后加入集群

## 14. 对象存储与 PVC

rlark 支持通过 Task 的 `pvcStorageMap` 为训练任务挂载持久化存储卷。

### 概念

当 Task 指定 `pvcStorageMap` 时，Agent 的 Pull 控制器在创建工作负载前自动创建指定 StorageClass 的 PVC，并在任务删除时自动清理。

### 配置方式

```yaml
kubernetes:
  workload:
    pvcStorageMap:
      my-data-pvc: "ceph-rbd"    # PVC 名称 → StorageClass 名称
```

### 工作流程

1. Agent 通过 `GET /api/v1/storage/storageclass?clusters=<agent-id>` 查询可用 StorageClass
2. 创建工作负载时，Agent 调用 `ensurePVCs` 创建指定 StorageClass 的 PVC
3. PVC 创建在目标命名空间中，作用域为当前 Task
4. 任务删除时，PVC 自动清理

## 15. 用户认证

rlark 为 Web UI 提供简单的基于角色的认证系统。

### 角色

| 角色 | 权限 |
|------|------|
| `admin` | 完全访问：创建/管理 Job、Node、Domain、Workflow |
| `user` | 只读访问：查看 Job、Node 和系统状态 |

### 认证流程

1. 部署时，`rlarkadm` 生成随机密码并存储在 KCP Secret（`rlark-ui-auth`）中
2. Web UI 发送 `POST /api/v1/auth/login` 携带用户名和密码
3. Gateway 对比 KCP Secret 中的凭据，返回角色
4. 前端将认证结果存储在 `sessionStorage` 中

## 16. Addon（组件管理）

Addon 是 rlark 的组件管理系统，允许用户在多个数据面集群中安装、配置和管理第三方组件（设备插件、监控代理等）。

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

Web Terminal 允许用户打开任何 rlark 管理的 Pod 的终端会话，无需 SSH 到底层节点或本地安装 kubectl。

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

Pod HTTP Proxy 允许通过 Server → Agent 代理链直接向 rlark 管理的 Pod 发送 HTTP 请求。

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

TensorBoard Proxy 提供基于 Web 的训练指标可视化仪表板（损失曲线、标量摘要、直方图等），可直接从 rlark Web UI 访问，无需对外暴露 TensorBoard 端口。

### 概念

当训练 Task 运行 TensorBoard（监听端口 6006）时，rlark 通过 Gateway 自动代理 TensorBoard UI。用户可以在 Web UI 中点击链接打开 TensorBoard，浏览器通过 Gateway 代理访问。

### 架构

```
浏览器 ──HTTP──▶ Gateway ──代理到 Server──▶ Server ──podproxy──▶ Agent ──▶ Pod:6006 (TensorBoard)
```

### 工作原理

1. Agent 的 Ray 初始化脚本在 Ray head 进程旁启动 TensorBoard
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
- **Ray 集成**：TensorBoard 随 Ray 任务自动启动，无需额外配置

## 20. SSH 密钥管理

SSH 密钥管理允许用户通过 API 或 Web UI 上传 SSH 公钥，实现免密 SSH 登录 Pod，无需共享证书。

### 概念

每位用户可以上传一个或多个 SSH 公钥。这些密钥存储在控制平面命名空间的 Kubernetes Secret（`rlark-ssh-user-keys`）中。当 Pod 创建时，Agent 会将用户的公钥注入到 Pod 的 `authorized_keys` 文件中，实现免密 SSH 访问。

### API

- `GET /api/v1/ssh-user-keys` — 列出所有 SSH 密钥（可按用户过滤）
- `POST /api/v1/ssh-user-keys` — 为用户添加新的 SSH 公钥
- `DELETE /api/v1/ssh-user-keys/:id` — 按索引删除密钥

### 关键特性

- **Web UI 管理**：Web UI 中提供专用的 SSH 密钥管理页面
- **按任务注入**：单个 Job 和 Task 可通过 `sshPublicKey` 字段指定密钥，对特定工作负载优先于集中管理的密钥
- **冲突检测**：重复密钥会被检测并返回 409 响应
- **写入重试**：API 在写入冲突时自动重试（最多 5 次）
- **密钥验证**：使用 `golang.org/x/crypto/ssh` 在存储前验证公钥格式
