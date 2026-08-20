# 快速开始

!!! warning "开发版本"
    RLark 当前尚无稳定版本。本文面向最新的 `main` 分支；该分支是开发快照，可能发生不保证向后兼容的变更。请克隆或更新 `main`，并基于同一提交构建所有组件。

选择以下两种方式之一完成 RLark 的最小完整闭环：

| 方式 | 说明 | 适合 |
|------|------|------|
| [**A: 一键部署（CLI）**](#a) | 一个脚本完成所有部署：控制面、数据面、跨集群测试 | 快速体验、CI/CD |
| [**B: UI 交互式**](#b-ui) | 启动控制面 + UI，通过管理平台纳管集群、下发任务 | 理解流程、演示 |

---

## 环境要求

| 工具 | 版本 | 说明 |
|------|------|------|
| Docker | >= 24.0 | 运行容器 |
| kind | >= 0.20 | 运行本地 k8s 数据面 |
| kubectl | >= 1.28 | 与集群交互 |
| jq | >= 1.6 | 解析 JSON |
| python3 | >= 3.8 | 处理 kubeconfig |
| node + npm | >= 18 | UI 开发服务器（仅方式 B） |

无需 root 权限，但当前用户必须有权访问 Docker daemon（例如加入系统的 Docker 用户组，或使用 Docker Desktop）。开始前请验证：

```bash
docker info
```

如果命令提示权限不足，请按操作系统的 Docker 指引修复用户权限，不要使用 `sudo` 运行整个快速开始流程。

### 失败后恢复

脚本再次启动时会清理由上一次运行遗留的资源。修复报错原因后，重新运行同一脚本即可。如果自动清理无法继续，请先手动清理本地环境：

```bash
docker compose -f apps/rlark/docs/examples/docker-compose.yml down -v
kind delete cluster --name rlark-data-1
kind delete cluster --name rlark-data-2
docker rm -f local-registry
rm -rf /tmp/rlark /tmp/kind-kubeconfig-*
```

清理时提示资源不存在可以安全忽略。上述命令会删除快速开始环境的状态，包括本地 PostgreSQL 数据卷；需要保留数据的环境请勿执行。

---

<a name="a"></a>
## A: 一键部署（CLI）

一个脚本构建镜像、启动控制面、创建 kind 集群、部署 Agent 并执行跨集群连通性测试。

### 1. 运行脚本

```bash
bash apps/rlark/docs/examples/quickstart.sh
```

脚本自动完成以下步骤：

| 步骤 | 说明 |
|------|------|
| 0 | 检查前置依赖 |
| 1 | 创建运行时目录 `/tmp/rlark` |
| 2 | 启动本地 Docker Registry |
| 3 | 编译 5 个二进制文件，构建 Docker 镜像并推送到本地 Registry |
| 4 | 确保 kind 节点镜像可用 |
| 5 | 启动 kcp 和 PostgreSQL |
| 6 | 配置 kubeconfig，安装 CRD，创建 UI 认证凭据 |
| 7 | 启动控制面组件（server、gateway、controller-manager） |
| 8 | 创建 kind 集群（`rlark-data-1`、`rlark-data-2`） |
| 9 | 部署 Agent（签发证书、RBAC、Agent Deployment） |
| 10 | 验证节点注册 |
| 11 | 创建跨集群测试资源（Workspace、Domain、Job） |
| 12 | 验证跨集群网络连通性 |

### 2. 登录 UI

脚本完成后，启动本地 UI：

```bash
cd apps/rlark-ui
npm install
VITE_DATA_MODE=backend npm run dev
```

打开 `http://localhost:5173/admin`。使用脚本输出中的凭据：

| 服务 | 地址 | 用途 |
|------|------|------|
| 管理平台 | `http://localhost:5173/admin` | 集群纳管、节点、证书 |
| 业务平台 | `http://localhost:5173` | 任务、Worker、工作流、存储 |
| Gateway API | `http://localhost:9000` | 自动化 |

### 3. 清理环境

```bash
docker compose -f apps/rlark/docs/examples/docker-compose.yml down
kind delete cluster --name rlark-data-1
kind delete cluster --name rlark-data-2
docker rm -f local-registry
rm -rf /tmp/rlark /tmp/kind-kubeconfig-*
```

---

<a name="b-ui"></a>
## B: UI 交互式

这种方式将控制面和数据面分开，让你通过管理平台创建集群，通过业务平台下发任务。

### 1. 启动控制面和 UI

```bash
bash apps/rlark/docs/examples/quickstart-cp.sh
```

脚本会：
- 构建并推送 Docker 镜像
- 启动 kcp、PostgreSQL 和控制面（server、gateway、controller-manager）
- 启动 UI 开发服务器（`http://localhost:5173`）
- 打印管理员凭据

!!! tip "保持终端打开"
    UI 开发服务器运行在前台。请保持此终端打开以便使用 UI。完成后按 `Ctrl+C` 停止。

输出示例：

```
控制面：
  kcp:                      localhost:6443
  rlark-server:             localhost:8443
  rlark-gateway (REST API): localhost:9000
  UI（管理平台）：            http://localhost:5173/admin
  UI（业务平台）：            http://localhost:5173

凭据：
  admin / <随机密码>
  user  / <随机密码>

后续步骤：
  1. 打开 http://localhost:5173/admin 以 admin 登录
  2. 进入集群管理 → 创建集群
  3. 输入集群名称（如 my-cluster）并创建
  4. 复制集群名称，运行：bash quickstart-dp.sh --cluster-id=my-cluster
  5. 返回 UI 创建域和下发任务
```

### 2. 在管理平台创建集群

1. 打开 `http://localhost:5173/admin`，以 `admin` 登录
2. 进入**集群管理** → **创建集群**
3. 输入集群名称（如 `my-cluster`）
4. 点击**签发证书** — UI 会显示 Server 地址和完整 `DeployConfig` YAML
5. 记下输入的集群名称（如 `my-cluster`），供下一步使用

### 3. 部署数据面

使用步骤 2 中的集群名称运行数据面脚本：

```bash
bash apps/rlark/docs/examples/quickstart-dp.sh --cluster-id my-cluster
```

脚本会：
- 创建 kind 集群
- 根据集群 ID 向 Gateway 自动申请 Agent 证书（无需 UI 中的 deploy-conf.yaml）
- 部署 Agent 并配置证书
- 验证节点注册

!!! note "deploy-conf.yaml 的用途"
    UI 创建集群时显示的 `deploy-conf.yaml` 用于**手动部署**（如在真实集群上部署 Agent）。脚本模式下，证书由脚本自动向 Gateway API 申请，无需此文件。

如需部署多个数据面集群，先在 UI 中创建所有集群，然后将所有集群 ID 一次性传给脚本：

```bash
# 在 UI 中创建 cluster-1 和 cluster-2 后，运行：
bash apps/rlark/docs/examples/quickstart-dp.sh \
  --cluster-id my-cluster-1 \
  --cluster-id my-cluster-2
```

### 4. 验证集群和节点

**通过 UI：** 管理平台 → 集群与节点。确认两个集群均在线且节点已同步。

**通过 API：**

```bash
curl -s "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" | \
  jq '.items[] | {name: .metadata.name, cluster: .metadata.labels["rlark.io/cluster-id"]}'
```

### 5. 通过 UI 创建任务

1. 打开 `http://localhost:5173`，以 `user` 登录
2. 进入**任务** → **创建任务**
3. 选择**自定义任务**作为任务类型
4. 添加一个名为 `worker` 的角色，设置为**主角色**
5. 配置 Worker：
   - **集群**：选择你的集群
   - **节点数**：1
   - **镜像**：`rayproject/ray:2.9.0-py310`
   - **运行脚本**：`echo hello from RLark; sleep 3600`

![配置任务 Worker 与调度位置](../images/ui/create-job-worker-configuration.png)

6. 检查 YAML 预览，点击**提交**

详细步骤参见 [RL 训练最佳实践](user-guide/best-practices.md)。

### 6. 创建并验证跨集群网络

部署两个数据面集群后，通过创建 Domain 和跨集群任务完成 UI 全流程，并验证 Pod 跨集群通信。

#### 6.1 创建 Domain

1. 登录管理平台（`http://localhost:5173/admin`）
2. 进入**网络域** → **创建域**

![进入网络域并创建 Domain](../images/ui/domain-ui.png)

3. 输入名称和 CIDR（如 `cross-cluster-net`，`10.200.0.0/24`）

#### 6.2 创建跨集群任务

1. 打开 `http://localhost:5173`，以 `user` 登录
2. 进入**任务** → **创建任务**
3. 选择**自定义任务**作为任务类型
4. 添加两个角色：
   - **server**：主角色，集群选 `rlark-my-cluster-1`，镜像 `rayproject/ray:2.9.0-py310`，运行脚本：
     ```bash
     python -u -m http.server 8000 --bind 0.0.0.0
     ```
   - **client**：集群选 `rlark-my-cluster-2`，镜像 `rayproject/ray:2.9.0-py310`，运行脚本：`sleep infinity`
5. 在**公共配置**步骤中，选择刚创建的 Domain
6. 检查并提交；在任务详情页确认 Worker 已在所选节点运行。

![确认 Worker 与 Pod 运行状态](../images/ui/job-details-worker-and-pod.png)

#### 6.3 验证跨集群连通性

任务运行后，检查 client pod 日志确认可访问 server：

```bash
# 查找 client pod
kubectl --kubeconfig /tmp/kind-kubeconfig-2 get pods -n rlark-system

# 从 client pod 测试连通性
kubectl --kubeconfig /tmp/kind-kubeconfig-2 exec -n rlark-system \
  <client-pod名称> -c main -- \
  python -c "import urllib.request; print(urllib.request.urlopen('http://<server-pod名称>.rlark-domain:8000').status)"
```

预期输出：`200`

详见 [网络与安全](admin-guide/network-security.md)。

### 7. 清理环境

```bash
# 停止控制面
docker compose -f apps/rlark/docs/examples/docker-compose.yml down

# 停止 UI 开发服务器
kill $(lsof -ti:5173) 2>/dev/null || true

# 删除 kind 集群
kind delete cluster --name rlark-data-1
kind delete cluster --name rlark-data-2

# 删除本地 Registry
docker rm -f local-registry

# 清理运行时文件
rm -rf /tmp/rlark /tmp/kind-kubeconfig-*
```

---

## 跨集群网络

数据面集群间跨集群网络通信配置：

### Agent 配置

```yaml
spec:
  hostNetwork: true
  hostPID: true
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - args:
    - "--server-address=https://rlark-server:8443"
    - "--rlark-server-ssh-address=client@rlark-server:2222"
    - "--network-sidecar-image=<image>"
```

### 数据流

```
客户端 Pod（集群 2）                        服务端 Pod（集群 1）
  ├── HTTP → Domain IP（10.200.0.x）          ├── Python HTTP 服务 :8000
  ├── gVisor netstack 拦截                    │
  ├── TUN 设备 → NodeServer socket            │
  └── NodeServer → SSH 隧道 → ────────────────→ Proxy → localhost:8000
```

## 下一步

- 阅读 [平台使用指南](user-guide/index.md) 通过图形界面管理资源和任务
- 阅读 [管理员指南](admin-guide/index.md) 了解生产部署、集群接入与运维
- 阅读 [核心概念](concepts.md) 了解资源模型和命名约定
- 阅读 [部署指南](deployment.md) 了解生产环境部署和真机设备纳管
- 阅读 [RL 训练最佳实践](user-guide/best-practices.md) 查看端到端的完整操作示例