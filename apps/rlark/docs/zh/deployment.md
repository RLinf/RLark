# 部署指南

## 1. 部署架构

rlark 支持三种部署方式，适用于不同场景：

| 方式 | 复杂度 | 适用场景 |
|------|--------|----------|
| Docker Compose | 低 | 本地开发、单机测试 |
| Kubernetes | 中 | 生产环境、集群部署 |
| Raw Binary | 高 | 极简部署、嵌入式场景 |

Kubernetes 场景下提供两种部署工具：

| 工具 | 说明 |
|------|------|
| `rlarkadm` | CLI 工具，读取 DeployConfig YAML 直接应用资源 |
| Helm Charts | 标准 Helm Chart，位于 `charts/` 目录下，分为控制面和数据面 |

## 2. Helm Chart 部署

Helm Chart 是 `rlarkadm` 的替代方案，适合已经使用 Helm 管理集群的用户。提供两个 Chart：

| Chart | 说明 |
|-------|------|
| `charts/rlark-control-plane` | KCP、etcd、PostgreSQL、Server、Gateway、Controller Manager、UI |
| `charts/rlark-data-plane` | Agent（Deployment）、Node Agent（DaemonSet） |

### 2.1 控制面

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --create-namespace \
  --set images.gateway=rlark:latest \
  --set images.controllerManager=rlark:latest \
  --set images.server=rlark:latest \
  --set images.ui=rlark-ui:latest
```

KCP 模式（默认）下，post-install hook Job 会自动从 KCP 提取 `admin.kubeconfig`、安装 CRD、创建 `rlark-ui-auth` secret。管理员密码可通过以下命令查看：

```bash
kubectl -n rlark-system logs job/kcp-post-install | grep -A2 "Admin password"
```

使用目标 Kubernetes 集群作为管理 API（不部署 KCP/etcd）：

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --create-namespace \
  --set managementApi=kubernetes \
  --set kcp.enabled=false \
  --set etcd.enabled=false \
  --set images.gateway=rlark:latest \
  --set images.controllerManager=rlark:latest \
  --set images.server=rlark:latest \
  --set images.ui=rlark-ui:latest
```

`managementApi=kubernetes` 模式下，CRD 由 Chart 直接安装到目标集群。默认 kcp 模式下，由 post-install hook 将 CRD 安装到 KCP。

#### 存储配置

```bash
# PVC 存储（生产推荐）
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --set global.storage.type=pvc \
  --set global.storage.storageClass=standard \
  --set global.storage.size=10Gi

# hostPath 存储（单节点）
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --set global.storage.type=hostPath \
  --set global.storage.hostPath=/data/rlark
```

#### 外部数据库

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --set db.host=pg-managed.example.com \
  --set db.port=5432 \
  --set db.database=rlark \
  --set db.user=rlark \
  --set db.password=CHANGE_ME
```

使用内置 PostgreSQL：

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --set postgresql.enabled=true \
  --set db.host=postgresql \
  --set db.port=5432 \
  --set db.database=rlark \
  --set db.user=rlark \
  --set db.password=CHANGE_ME
```

#### 外部 etcd

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --set etcd.address=https://my-etcd.example.com:2379
```

#### etcd 集群 + hostPath

使用 hostPath 存储部署 3 节点 etcd 集群：

```yaml
# values.yaml
etcd:
  enabled: true
  replicas: 3
  storage:
    type: hostPath
    hostPath: /data/rlark/etcd
```

```bash
helm install rlark-control-plane ./charts/rlark-control-plane \
  --namespace rlark-system \
  --create-namespace \
  -f values.yaml
```

> **注意**：使用 `hostPath` 或 `PVC` 存储重新部署时，需要先删除现有的 etcd PVC：
> ```bash
> kubectl delete pvc -n rlark-system etcd-data-0 etcd-data-1 etcd-data-2
> ```

#### 镜像配置

所有组件镜像都可通过 `values.yaml` 配置。默认情况下，RLark 自身的镜像使用本地 `make docker-build-*` 产生的标签（`rlark:latest`、`rlark-ui:latest`）；从镜像仓库安装时需要覆盖：

```yaml
images:
  gateway: your-registry/rlark:v0.0.50
  controllerManager: your-registry/rlark:v0.0.50
  server: your-registry/rlark:v0.0.50
  kcp: your-registry/kcp:v0.30.0
  etcd: your-registry/etcd:3.5.15-0
  ui: your-registry/rlark-ui:v0.0.50

# KCP post-install hook 镜像（需要包含 kubectl 和 shell）
hookImage: your-registry/bitnami/kubectl:1.33.1
```

### 2.2 数据面

首先从控制面获取 Agent 证书：

```bash
curl -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"cluster_id": "beijing"}'
```

`certs.caCert`、`certs.agentCert`、`certs.agentKey` 没有默认值，任一为空时 Chart 渲染会直接失败。安装时从文件传入，不要把证书和私钥写进 `values.yaml`：

```bash
helm install rlark-data-plane ./charts/rlark-data-plane \
  --namespace rlark-system \
  --set controlPlaneAddress=https://rlark-server.rlark-system.svc:8443 \
  --set-file certs.caCert=ca.crt \
  --set-file certs.agentCert=agent.crt \
  --set-file certs.agentKey=agent.key
```

为 node-agent DaemonSet 启用节点：

```bash
kubectl label node <node-name> rlark.io/enabled=true
```

k3s 或非标准 containerd 路径：

```bash
helm install rlark-data-plane ./charts/rlark-data-plane \
  --namespace rlark-system \
  --set controlPlaneAddress=https://rlark-server.rlark-system.svc:8443 \
  --set agentNode.containerdSocket=/run/k3s/containerd/containerd.sock \
  --set-file certs.caCert=ca.crt \
  --set-file certs.agentCert=agent.crt \
  --set-file certs.agentKey=agent.key
```

## 3. 部署工具：rlarkadm

`rlarkadm` 是 RLark 的部署 CLI，用于安装和卸载控制面、数据面组件。安装时会等待每个 Kubernetes 工作负载就绪，单个组件最长等待 180 秒；当前没有独立的 `health` 子命令。

```bash
# 安装
rlarkadm install -f <配置文件>

# 卸载
rlarkadm uninstall -f <配置文件>
```

### 配置文件结构

一个文件只能描述一个平面，并且只能配置 `kubernetes`、`docker`、`raw` 三种环境中的一种。请从仓库中维护的示例开始修改，不要把控制面和数据面拼到同一个 YAML 文档中：

```yaml
# 控制面
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  kubeconfig: /path/to/control-kubeconfig
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  ui-image: rlark-ui:latest
  storage:
    type: pvc
    storage-class: ""
    size: 10Gi
```

```yaml
# 数据面
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://rlark.example.com:8443
cert:
  ca-cert: |                # 可填写 PEM 内容或已存在的文件路径
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  agent-cert: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  agent-key: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
kubernetes:
  kubeconfig: /path/to/data-kubeconfig
  agent-image: rlark:latest
  image: rlark:latest       # 可选；启用 Pod 网络和 SSH 支持
```

## 3. Kubernetes 部署

默认情况下，`rlarkadm` 会部署 kcp 作为管理 API。如需直接使用目标 Kubernetes 集群，请配置：

```yaml
kubernetes:
  management-api: kubernetes
```

该模式下，`rlarkadm` 会将 RLark CRD 直接安装到目标集群，不部署 kcp 和 etcd。Server、Gateway 和 Controller Manager 使用各自的 ServiceAccount、ClusterRole 和集群内凭据。普通卸载会保留集群级 CRD、RLark 自定义资源和管理 Secret；删除这些数据属于独立的破坏性操作。

使用 kcp 时，`rlarkadm` 会将 Controller Manager 选举 Lease 存放在管理 API 的 `default` 命名空间中；直接使用目标 Kubernetes 集群作为管理 API 时，Lease 存放在控制面工作负载所在的 `rlark-system` 命名空间中。

### 3.1 控制面部署

```bash
# 1. 准备配置文件
cp apps/rlark/docs/examples/deploy-control-plane.yaml my-deploy.yaml

# 2. 修改镜像地址和 kubeconfig
# 3. 执行安装
rlarkadm install -f my-deploy.yaml

# 4. 验证
kubectl get pods -n rlark-system
```

控制面部署的组件：

| 组件 | 副本数 | 端口 | 说明 |
|------|--------|------|------|
| kcp | 1 | 6443 | API Server |
| etcd | 1 | 2379 | kcp 存储；仅设置 `etcd-image` 且未配置外部地址时部署 |
| postgresql | 1 | 5432 | RLark 存储；仅设置顶层 `db` 配置块时部署 |
| server | 1 | 8443, 2222, 8888 | HTTPS、SSH、健康检查和指标 HTTP |
| controller-manager | 1 | 8080, 8081 | 指标和健康探针 |
| gateway | 1 | 8090 | `rlarkadm` 部署中的 API Gateway |
| ui | 1 | 80 | Web 管理界面；将 `/api/` 代理到 Gateway |

### 3.2 数据面部署

```bash
# 1. 从控制面获取证书（通过 Gateway API 或手动签发）
# 2. 填写配置文件
cp apps/rlark/docs/examples/deploy-data-plane.yaml my-data.yaml

# 3. 执行安装
rlarkadm install -f my-data.yaml

# 4. 验证
kubectl get pods -n rlark-system
```

数据面部署的组件：

| 组件 | 副本数 | 说明 |
|------|--------|------|
| agent | Deployment | 集群级同步（`--mode=cluster`） |
| agent-node | DaemonSet | 节点网络和镜像预拉取（`--mode=node`） |
| network-sidecar | 注入的容器 | 配置 `kubernetes.image` 后加入符合条件的训练 Pod |

## 4. Docker Compose 部署（开发环境）

适用于本地开发和测试：

```yaml
# docker-compose.yml
services:
  kcp:
    image: kcp:v0.30.0
    command: ["start", "--root-directory", "/data/kcp"]
    ports:
      - "6443:6443"
    volumes:
      - kcp-data:/data

  postgresql:
    image: postgres:15
    environment:
      POSTGRES_DB: rlark
      POSTGRES_USER: rlark
      POSTGRES_PASSWORD: CHANGE_ME
    ports:
      - "5432:5432"
    volumes:
      - pg-data:/var/lib/postgresql/data

volumes:
  kcp-data:
  pg-data:
```

## 5. 组件配置

### 5.1 Server

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `$KUBECONFIG` | 控制面 kubeconfig |
| `--https-port` | `8443` | HTTPS 服务端口 |
| `--ssh-port` | `2222` | SSH 服务端口 |
| `--ssh-max-pending-targets` | `64` | 每个 SSH 连接的上游待接入限额，同时限制额外等待人数，必须为正；不计已建立 channel |
| `--ssh-admission-wait-timeout` | `5s` | open 额度等待预算，必须为正，包含在共用的 30 秒 context 时限内；等待队列也满时立即拒绝 |
| `--unsafe-http-port` | `8888` | `/healthz`、`/readyz` 和指标使用的无认证 HTTP 端口 |
| `--db-config` | `""` | 数据库配置文件路径 |
| `--auto-sign-tls-ca-cert` | `false` | Kubernetes 中不存在 TLS CA 和 Server 证书时自动生成 |
| `--tls-domains` | `localhost` | 生成的 Server 证书包含的逗号分隔 DNS 名称 |

### 5.2 Controller-Manager

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `$KUBECONFIG` | 控制面 kubeconfig |
| `--server-address` | `https://rlark-server.rlark-system.svc:8443` | Server 地址 |
| `--leader-election` | `true` | 是否启用 Leader 选举 |
| `--metrics-bind-address` | `:8080` | 指标监听地址 |
| `--health-probe-bind-address` | `:8081` | `/healthz` 和 `/readyz` 监听地址 |
| `--archive-ttl` | `5m` | 终态资源在 kcp 中保留的时长，超时后归档删除 |
| `--enable-secret-backend` | `false` | 使用 Kubernetes Secret 作为持久化后端（测试模式，仅在未配置 PostgreSQL 时生效） |

### 5.3 Gateway

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `$KUBECONFIG` | 控制面 kubeconfig |
| `--addr` | `:8080` | 独立二进制默认值；`rlarkadm` 会覆盖为 `:8090` |
| `--db-config` | `""` | 数据库配置文件路径 |
| `--enable-secret-backend` | `false` | 使用 Kubernetes Secret 作为持久化后端（测试模式，仅在未配置 PostgreSQL 时生效） |
| `--server-address` | `https://rlark-server.rlark-system.svc:8443` | 用于证书签发的 Server 地址 |

### 5.4 Agent

可直接使用的数据面清单：[agent-rbac.yaml](../examples/agent-rbac.yaml) 和 [agent-deploy.yaml](../examples/agent-deploy.yaml)。

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `$KUBECONFIG` | 数据面 kubeconfig |
| `--server-address` | `https://localhost:8443` | 控制面 Server 地址 |
| `--client-cert` | `""` | Agent 证书路径 |
| `--client-key` | `""` | Agent 私钥路径 |
| `--ca-cert` | `""` | CA 证书路径 |
| `--mode` | `cluster` | 角色：`cluster` / `node`；本机组合使用 `--node-startup=embedded` |
| `--rlark-server-ssh-address` | `""` | Server SSH 地址（跨集群网络） |
| `--rlark-server-ssh-host-key` | `""` | Server SSH 主机密钥 |
| `--agent-type` | `kubernetes` | 不区分大小写的启动 selector；Docker/Raw 执行不支持 |
| `--leader-election` | `false` | 安装器可设置此历史 flag，但当前 Agent manager 不据此执行选主门控 |
| `--image` | `""` | RLark tools/网络/SSH 镜像；Kubernetes cluster 模式 Task 装配必填 |
| `--in-cluster` | `false` | 使用集群内 Kubernetes 配置（K8s 模式下自动设置） |
| `--insecure-skip-tls-verify` | `false` | 跳过 TLS 证书验证 |

### 5.5 环境模式

rlark 支持三种部署环境模式，在 DeployConfig YAML 中配置：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `kubernetes` | 部署到 K8s 集群 | 生产环境、GPU 集群 |
| `docker` | 通过 Docker API 管理（TODO） | 端侧设备、单机部署 |
| `raw` | 下载 artifact 直接执行（TODO） | 裸金属、嵌入式设备 |

配置文件中需指定且仅指定 `kubernetes`、`docker`、`raw` 中的一个环境块。

### 5.6 Agent 运行模式

Agent 通过 `--mode` 支持两种角色，语义独立于部署方式：

| 模式 | 说明 | 部署形式 |
|------|------|----------|
| `cluster` | 集群级工作负载管理 | Deployment（agent） |
| `node` | 节点级网络操作 | DaemonSet（agent-node） |
| `cluster` + `--node-startup=embedded` | 在共同致命错误取消/join 下添加本机节点服务 | 手工监督进程 |

节点启动默认 external；`rlarkadm` 与 Helm 保持独立 Deployment/DaemonSet，使用小写 Runtime 参数。embedded 不发现或部署远程 Agent，需要两种角色的挂载/权限、节点名及独立 metrics 端口（cluster `:8081`、node `:8082`）。旧组合模式和未实现的 managed 明确拒绝。仅启动读取的 Agent YAML、本地校验、来源诊断及环境兼容见 [配置参考](reference/configuration.md#agent-startup-file-and-validation)，它不是 DeployConfig，没有 reload/rollout controller。

当 `--mode=node` 时，Agent 以 DaemonSet 形式在每个节点上运行，负责建立跨集群网络的 SSH 隧道。

集群 Agent 通过 API Server 的节点代理（`api/v1/nodes/<node>/proxy/stats/summary`）读取 kubelet stats，以上报 Node 存储容量。因此其 ServiceAccount 需要对 `nodes/proxy` 子资源具备 `get` 权限；缺少该权限时，Agent 会在每轮同步中记录 `failed to get kubelet storage stats ... 403 Forbidden`，且 Node 的 `status.storage` 始终为空。Helm Chart 与 `rlarkadm` 已默认授予该权限；手写 RBAC 清单必须自行包含（参见 [agent-rbac.yaml](../examples/agent-rbac.yaml)）。

## 6. Addon 配置

rlark 支持跨数据面集群的声明式 Addon 管理。Addon 定义在 addon 目录（`../pkg/addons/catalog/`）中，通过 Addon API 安装。

### 6.1 Addon 目录结构

```
pkg/addons/catalog/
├── embodied-runtime-device-plugin/
│   ├── addon.yaml          # Addon 元数据（名称、版本、描述、可配置参数）
│   └── manifests/
│       ├── daemonset.yaml  # 含模板值的 K8s DaemonSet
│       ├── configmap-template.yaml  # ConfigMap 模板（camera/ROS 控制器配置）
│       ├── headless-services.yaml   # Camera/ROS 控制器的 Headless Service
│       └── rbac.yaml       # ClusterRole + ClusterRoleBinding
├── csi-driver-rclone/
│   ├── addon.yaml          # Addon 元数据（名称、版本、类别：storage）
│   └── manifests/
│       ├── controller.yaml  # CSI Controller Deployment
│       ├── node.yaml        # CSI Node DaemonSet
│       ├── configmap.yaml   # RClone 配置
│       ├── csidriver.yaml   # CSIDriver 资源
│       └── rbac.yaml        # RBAC 权限
└── fluent-bit/
    ├── addon.yaml           # Addon 元数据和日志后端配置
    └── manifests/
        ├── daemonset.yaml   # Fluent Bit 日志采集 DaemonSet
        ├── configmap.yaml   # 输入、过滤和输出配置
        ├── secret.yaml      # 后端认证信息
        └── rbac.yaml        # RBAC 权限
```

`embodied-runtime-device-plugin` 的关键可配置参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `deviceCount` | 每个节点暴露的设备数量 | `"1"` |
| `robots` | 所有节点共用的全局机器人 YAML 配置 | `""` |
| `macvlans` | 所有节点共用的全局 MacVlan YAML 配置 | `""` |
| `nodeOverrides` | 以节点名为键的节点级 YAML 覆盖配置 | `""` |
| `image` | 设备插件容器镜像 | `rlinf/embodied-runtime:v0.1.0-b4b4d6f8` |
| `rendererImage` | 节点级配置渲染 initContainer 镜像（yq） | `yq:4.53.2` |
| `cameraImage` | Camera 控制器容器镜像 | `rlinf/camera-base:v0.1.0-946787a0` |
| `rosImage` | ROS 控制器容器镜像 | `rlinf/serl_franka_controllers:v0.1.0-libfranka-0.19.0-frankaros-0.10.2` |
| `nodeSelector` | DaemonSet 调度的节点选择器 | `""` |
| `robotTolerationKey` | 机器人节点的容忍度键 | `rlinf.io/robot` |

该 Addon 还会部署两个 Headless Service（`camera-controller-headless` 和 `ros-controller-headless`），用于集群内基于 DNS 的稳定发现 camera 和 ROS 控制器。

`csi-driver-rclone` 的关键可配置参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `rcloneImage` | RClone CSI 驱动容器镜像 | `rlinf/csi-rclone/csi-driver-rclone:v0.2.0` |
| `csiProvisionerImage` | CSI Provisioner sidecar 镜像 | `rlinf/csi-rclone/csi-provisioner:v6.2.0` |
| `livenessProbeImage` | Liveness Probe sidecar 镜像 | `rlinf/csi-rclone/livenessprobe:v2.18.0` |
| `nodeDriverRegistrarImage` | Node Driver Registrar sidecar 镜像 | `rlinf/csi-rclone/csi-node-driver-registrar:v2.16.0` |
| `driverName` | CSI 驱动注册名称 | `rclone.csi.veloxpack.io` |
| `nodeSelector` | Node DaemonSet 节点选择器，格式为 `label=value` | `""` |
| `controllerReplicas` | Controller Deployment 副本数 | `1` |
| `controllerLogLevel` | Controller 日志级别 (0-10) | `5` |
| `nodeLogLevel` | Node DaemonSet 日志级别 (0-10) | `5` |

RClone CSI 驱动支持通过 RClone 动态配置远程存储（S3、GCS、Azure Blob 等）支持的 PersistentVolume。

`fluent-bit` 的关键可配置参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `backend` | 日志后端；目前仅支持 `sls` | `sls` |
| `endpoint` | 后端地址；SLS 使用 Kafka 接入地址 | `""` |
| `project` | SLS Project 或后端租户/组织名 | `""` |
| `logstore` | SLS Logstore 或后端索引名 | `""` |
| `accessKeyId` | 后端认证 ID | `""` |
| `accessKeySecret` | 后端认证 Secret | `""` |
| `clusterId` | 写入日志的 `cluster_id` 标签值 | `""` |
| `image` | Fluent Bit 容器镜像 | `fluent/fluent-bit:3.1.8` |
| `cpuLimit` | Fluent Bit CPU 限额 | `200m` |
| `memoryLimit` | Fluent Bit 内存限额 | `256Mi` |

使用 SLS 时，`endpoint` 填写 `<project>.<endpoint>:<port>`（公网端口 `10012`，私网端口 `10011`），`project` 填写 SLS Project，`logstore` 作为 Kafka topic，并提供具有 SLS 写权限的 AccessKey。DaemonSet 采集 Pod stdout/stderr，并附加 `cluster_id`、`job`、`task`、`pod`、`namespace` 等标签。

### 6.2 安装 Addon

```bash
# 向指定集群安装 Addon
curl -X POST "http://localhost:8080/api/v1/clusters/agent-beijing/addons" \
  -H "Content-Type: application/json" \
  -d '{
    "addonName": "embodied-runtime-device-plugin",
    "version": "v0.1.0",
    "values": {
      "image": "rlinf/embodied-runtime:v0.1.0-b4b4d6f8"
    }
  }'
```

### 6.3 管理 Addon

```bash
# 列出目录中的可用 Addon
curl "http://localhost:8080/api/v1/addons"

# 列出所有集群已安装的 Addon
curl "http://localhost:8080/api/v1/installed-addons"

# 列出指定集群的 Addon
curl "http://localhost:8080/api/v1/clusters/agent-beijing/addons"

# 获取 Addon 详情
curl "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-runtime-device-plugin"

# 更新 Addon 配置
curl -X PUT "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-runtime-device-plugin" \
  -H "Content-Type: application/json" \
  -d '{
    "values": {
      "image": "rlinf/embodied-runtime:v0.1.0-b4b4d6f8"
    }
  }'

# 卸载 Addon
curl -X DELETE "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-runtime-device-plugin"
```

## 7. 存储配置

### 7.1 控制面存储

| 组件 | 数据内容 | 推荐大小 |
|------|----------|----------|
| kcp | 所有 CRD 对象 | 10Gi |
| etcd | kcp 元数据 | 8Gi |
| postgresql | SSH 密钥、用户数据 | 30Gi |

### 7.2 存储类型

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| `emptyDir` | 临时存储，Pod 删除后丢失 | 测试环境 |
| `hostPath` | 节点本地路径 | 单节点部署 |
| `pvc` | 持久化卷 | 生产环境 |

## 8. 网络要求

### 8.1 端口暴露

| 组件 | 端口 | 协议 | 说明 |
|------|------|------|------|
| UI | 80 | HTTP | 浏览器入口；将 `/api/` 代理到 Gateway |
| Gateway | 8090 | HTTP | `rlarkadm` 内部 API（独立二进制默认 `8080`） |
| Server | 8443 | HTTPS/WSS | Agent 隧道、代理和证书操作 |
| Server | 2222 | SSH | 用户和跨集群 SSH |
| Server | 8888 | HTTP | 内部健康检查和指标 |
| kcp | 6443 | HTTPS | 内部控制面 API |

### 8.2 网络拓扑

```text
用户 / 浏览器 ──HTTP──▶ UI (:80) ──/api 代理──▶ Gateway (:8090)
用户 SSH 客户端 ──────▶ Server (:2222)
                                        ▲
数据面集群 ──出方向 WSS─────────────────┤ Server (:8443)
  ├─ agent Deployment（cluster 模式）  │
  └─ agent-node DaemonSet（node 模式） ┘

Server / Gateway / Controller Manager ──HTTPS──▶ kcp (:6443)
```

两个数据面 Agent 都主动向 Server 发起出方向连接，因此无需开放数据面入站端口。在控制面按需开放 UI 80 和 Server 8443、2222 端口；Gateway、kcp、健康检查和指标端口应保持内部可见。

## 9. 证书管理

### 9.1 证书类型

| 证书 | 用途 | 签发方式 |
|------|------|----------|
| CA 证书 | 根证书，签发其他证书 | 部署时生成 |
| Agent 证书 | Agent 接入控制面 | 管理员通过 Gateway API 签发 |
| Domain 证书 | 跨集群 Pod 通信 | Controller-Manager 自动签发 |
| SSH 证书 | 用户 SSH 登录 | Server 在用户认证时临时签发 |

### 9.2 签发 Agent 证书

```bash
# UI 会把 /api/ 代理到 Gateway
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80

curl -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"cluster_id": "beijing"}'
```

返回证书和私钥，部署到数据面 Agent 的配置中。

### 9.3 UI 认证

部署时，`rlarkadm` 会自动创建包含随机 admin/user 角色密码和 JWT 签名密钥的 `rlark-ui-auth` Secret。使用 kcp 时存放在 kcp 的 `default` 命名空间；直接使用目标 Kubernetes 集群作为管理 API 时存放在 `rlark-system`：

| 键 | 用途 |
|-----|------|
| `admin-password` | 管理员角色密码（16 位随机字符） |
| `user-password` | 用户角色密码（16 位随机字符） |
| `jwt-signing-key` | 32 字节 HS256 签名密钥；升级已有 Secret 时补齐缺失字段，但不会轮换已有密钥 |

密码会在安装摘要中显示，签名密钥不会显示。Web UI 通过 `POST /api/v1/auth/login` 认证并获取有过期时间的 JWT。

## 10. 生产部署与高可用

### 10.1 当前 `rlarkadm` 能力范围

仓库维护的 `rlarkadm` 示例为每个启用的控制面组件部署 1 个副本。kcp 当前限制为单副本：显式设置大于 1 的 `kubernetes.kcp.replicas` 会被拒绝，全局 `replicas` 也不会扩展 kcp。RLark 目前没有为其他组件提供经过验证的生产高可用拓扑；仅增加副本数不能视为实现了高可用。

生产环境默认应沿用维护中的单副本拓扑，除非已独立设计并验证组件拓扑、共享状态、流量路由、故障恢复和存储行为。需要高可用数据服务时，应使用外部托管方案；`rlarkadm` 不会配置 PostgreSQL 主备复制。

### 10.2 外部 etcd

```yaml
kubernetes:
  etcd:
    address: https://my-etcd.example.com:2379
    # 不部署内置 etcd
```

### 10.3 外部 PostgreSQL

```yaml
db:
  host: pg-managed.example.com
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

## 11. 监控与运维

### 11.1 Prometheus 指标

Gateway 和 Server 暴露 Prometheus 指标：

- `rlark_gateway_requests_total`：Gateway 请求总数
- `rlark_gateway_request_duration_seconds`：请求延迟
- `rlark_proxy_requests_total`：Server 代理请求总数
- `rlark_peer_connections`：当前 Peer 连接数
- `rlark_ssh_connections_total`：SSH 连接总数

Controller Manager 在 `--metrics-bind-address` 上为每次调谐发布一组指标：

- `rlark_controller_reconcile_total{controller,outcome}`：已完成的调谐次数，按分类结论划分（`Succeeded`、`WaitingForChange`、`Retrying`、`CleanupInProgress`、`Blocked`、`Dropped`）
- `rlark_controller_reconcile_failures_total{controller,reason}`：返回错误的调谐次数，按 reason 码划分
- `rlark_controller_reconcile_duration_seconds{controller}`：单次调谐的耗时

标签取值全部来自 Go 源码中声明的闭集（资源 Kind、Outcome 枚举、reason 常量）。对象名、命名空间、UID 和原始错误字符串一律不作为标签，因此时间序列数量不会随对象数量增长。

### 11.2 日志

使用结构化日志（zap），通过 `LOG_LEVEL` 环境变量控制日志级别：

```bash
LOG_LEVEL=debug ./bin/server --kubeconfig ...
```

### 11.3 健康检查

`rlarkadm install` 会等待每个 Deployment、StatefulSet 和 DaemonSet 的期望副本全部就绪。后续可这样检查：

```bash
kubectl get deploy,statefulset,daemonset -n rlark-system
kubectl rollout status deployment/rlark-server -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system

# Server 健康检查（通常仅集群内部可见）
kubectl port-forward -n rlark-system svc/rlark-server 8888:8888
curl --fail http://localhost:8888/healthz
curl --fail http://localhost:8888/readyz

# Controller Manager 健康检查
kubectl port-forward -n rlark-system deployment/rlark-controller-manager 8081:8081
curl --fail http://localhost:8081/healthz
curl --fail http://localhost:8081/readyz
```

Gateway 和 Agent 暴露指标，但没有专用 HTTP 健康路由；请通过 Kubernetes 工作负载就绪状态和日志检查。

Controller Manager 的两个探针语义不同：

- `/healthz`（liveness）只回答进程自身是否存活。它刻意不检查管理 API 或任何其他外部依赖，否则依赖故障会被放大成重启风暴。
- `/readyz`（readiness）报告 informer 缓存是否已同步。缓存同步完成前，调谐会基于不完整的视图做决策，因此该实例尚不可提供服务。

## 12. 升级

```bash
# 1. 更新镜像版本
# 2. 滚动更新（Kubernetes 原生支持）
kubectl rollout restart deployment -n rlark-system

# 3. 验证
kubectl get pods -n rlark-system
```

## 13. 故障排查

### Agent 无法连接 Server

1. 检查 Agent 证书是否有效（未过期、由正确 CA 签发）
2. 检查网络连通性：`curl -k https://<server>:8443`
3. 检查 Server 日志：`kubectl logs -n rlark-system deployment/rlark-server`

### 训练任务无法启动

1. 检查 Node 是否有足够资源：`kubectl describe node <node-name>`
2. 检查 Task 状态：查询对应 Task CR
3. 检查集群 Agent 日志：`kubectl logs -n rlark-system deployment/rlark-agent`

### 跨集群网络不通

1. 检查 DomainPeer 是否已创建
2. 检查 Domain 证书是否签发成功
3. 检查 network-sidecar 是否注入到 Pod 中

## 14. 真机设备纳管

有关 GPU 节点、机器人、摄像头及其他物理设备的完整纳管流程、Embodied Runtime 配置和设备任务提交方式，请参阅[具身设备纳管指南](admin-guide/embodied-runtime.md)。
