# 部署指南

## 1. 部署架构

rlark 支持三种部署方式，适用于不同场景：

| 方式 | 复杂度 | 适用场景 |
|------|--------|----------|
| Docker Compose | 低 | 本地开发、单机测试 |
| Kubernetes | 中 | 生产环境、集群部署 |
| Raw Binary | 高 | 极简部署、嵌入式场景 |

## 2. 部署工具：rlarkadm

`rlarkadm` 是 rlark 的部署 CLI，统一管理控制面和数据面的安装、卸载和健康检查。

```bash
# 安装
rlarkadm install -f <配置文件>

# 卸载
rlarkadm uninstall -f <配置文件>

# 健康检查
rlarkadm health -f <配置文件>
```

### 配置文件结构

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control              # control | data

# 控制面配置
kubernetes:                 # 部署到 K8s 集群
  kubeconfig: /path/to/kubeconfig
  # 镜像配置
  gateway-image: rlark-gateway:latest
  controller-manager-image: rlark-controller-manager:latest
  server-image: rlark-server:latest
  kcp-image: kcp:v0.30.0
  postgresql-image: postgres:15
  ui-image: rlark-ui:latest
  # 存储配置
  storage:
    type: pvc               # emptyDir | hostPath | pvc
    storage-class: ""
    size: 10Gi
    node-selector:
      kubernetes.io/hostname: dev-worker

# 数据面配置
plane: data
cert:
  ca-cert: |
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
  agent-image: rlark-agent:latest
  network-sidecar-image: rlark-network-sidecar:latest
```

## 3. Kubernetes 部署

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
| etcd | 1 | 2379 | kcp 数据存储（可选外置） |
| postgresql | 1 | 5432 | rlark 数据存储 |
| server | 1 | 8443, 2222 | HTTPS + SSH |
| controller-manager | 1 | - | 控制面控制器 |
| gateway | 1 | 8080 | API Gateway |
| ui | 1 | 80 | Web 管理界面 |

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
| agent | DaemonSet | cluster + node 模式 |
| network-sidecar | Pod 注入 | 自动注入到训练 Pod |

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
| `--kubeconfig` | `""` | 控制面 kubeconfig |
| `--https-port` | `8443` | HTTPS 服务端口 |
| `--ssh-port` | `2222` | SSH 服务端口 |
| `--db-config` | `""` | 数据库配置文件路径 |
| `--ca-cert` | `""` | CA 证书路径 |
| `--ca-key` | `""` | CA 私钥路径 |
| `--auto-sign-tls-ca-cert` | `false` | 自动使用 CA 签署 TLS 证书 |
| `--tls-domains` | `""` | 自动证书生成的 TLS 域名列表 |

### 5.2 Controller-Manager

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `""` | 控制面 kubeconfig |
| `--server-address` | `""` | Server 地址（用于证书签发） |
| `--leader-elect` | `true` | 是否启用 Leader 选举 |

### 5.3 Gateway

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `""` | 控制面 kubeconfig |
| `--addr` | `:8080` | HTTP 服务地址 |
| `--db-config` | `""` | 数据库配置文件路径 |
| `--server-address` | `""` | 控制面 Server 地址，用于证书签发 |

### 5.4 Agent

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--kubeconfig` | `""` | 数据面 kubeconfig |
| `--server-address` | `""` | 控制面 Server 地址 |
| `--client-cert` | `""` | Agent 证书路径 |
| `--client-key` | `""` | Agent 私钥路径 |
| `--ca-cert` | `""` | CA 证书路径 |
| `--mode` | `both` | 运行模式：`cluster` / `node` / `both` |
| `--rlark-server-ssh-address` | `""` | Server SSH 地址（跨集群网络） |
| `--rlark-server-ssh-host-key` | `""` | Server SSH 主机密钥 |
| `--agent-type` | `auto` | Agent 类型：`kubernetes` / `docker` / `raw`（根据 env 模式自动检测） |
| `--leader-election` | `true` | 启用 Leader 选举（副本数=1 时自动禁用） |
| `--network-sidecar-image` | `""` | 网络 Sidecar 容器镜像 |
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

Agent 通过 `--mode` 支持三种运行模式：

| 模式 | 说明 | 部署形式 |
|------|------|----------|
| `cluster` | 集群级工作负载管理 | Deployment（agent） |
| `node` | 节点级网络操作 | DaemonSet（agent-node） |
| `both` | 同时运行 cluster 和 node 控制器 | 合并模式 |

当 `--mode=node` 时，Agent 以 DaemonSet 形式在每个节点上运行，负责建立跨集群网络的 SSH 隧道。

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
└── csi-driver-rclone/
    ├── addon.yaml          # Addon 元数据（名称、版本、类别：storage）
    └── manifests/
        ├── controller.yaml  # CSI Controller Deployment
        ├── node.yaml        # CSI Node DaemonSet
        ├── configmap.yaml   # RClone 配置
        ├── csidriver.yaml   # CSIDriver 资源
        └── rbac.yaml        # RBAC 权限
```

`embodied-runtime-device-plugin` 的关键可配置参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `image` | 设备插件容器镜像 | `rlark/embodied-device-plugin:0.1.0` |
| `rendererImage` | 节点级配置渲染 initContainer 镜像（yq） | `yq:4.53.2` |
| `cameraImage` | Camera 控制器容器镜像 | — |
| `rosImage` | ROS 控制器容器镜像 | — |
| `nodeSelector` | DaemonSet 调度的节点选择器 | `nvidia.com/gpu=true` |
| `robotTolerationKey` | 机器人节点的容忍度键 | — |

该 Addon 还会部署两个 Headless Service（`camera-controller-headless` 和 `ros-controller-headless`），用于集群内基于 DNS 的稳定发现 camera 和 ROS 控制器。

`csi-driver-rclone` 的关键可配置参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `rcloneImage` | RClone CSI 驱动容器镜像 | `csi-driver-rclone:v0.2.0` |
| `csiProvisionerImage` | CSI Provisioner sidecar 镜像 | `csi-provisioner:v6.2.0` |
| `livenessProbeImage` | Liveness Probe sidecar 镜像 | `livenessprobe:v2.18.0` |
| `nodeDriverRegistrarImage` | Node Driver Registrar sidecar 镜像 | `csi-node-driver-registrar:v2.16.0` |
| `driverName` | CSI 驱动注册名称 | `rclone.csi.veloxpack.io` |
| `controllerReplicas` | Controller Deployment 副本数 | `1` |
| `controllerLogLevel` | Controller 日志级别 (0-10) | `5` |
| `nodeLogLevel` | Node DaemonSet 日志级别 (0-10) | `5` |

RClone CSI 驱动支持通过 RClone 动态配置远程存储（S3、GCS、Azure Blob 等）支持的 PersistentVolume。

### 6.2 安装 Addon

```bash
# 向指定集群安装 Addon
curl -X POST "http://localhost:8080/api/v1/clusters/agent-beijing/addons" \
  -H "Content-Type: application/json" \
  -d '{
    "addonName": "embodied-runtime-device-plugin",
    "version": "0.1.0",
    "values": {
      "image": "rlark/embodied-device-plugin:0.1.0"
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
curl "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin"

# 更新 Addon 配置
curl -X PUT "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin" \
  -H "Content-Type: application/json" \
  -d '{
    "values": {
      "image": "rlark/embodied-device-plugin:0.2.0"
    }
  }'

# 卸载 Addon
curl -X DELETE "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin"
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
| Gateway | 8080 | HTTP | 用户 API 访问 |
| Server | 8443 | HTTPS | Agent 隧道连接 |
| Server | 2222 | TCP | SSH 用户登录 |
| kcp | 6443 | HTTPS | 控制面 API |

### 8.2 网络拓扑

```
Internet / 用户网络
    │
    ├──▶ Gateway (:8080) ── REST API
    └──▶ Server (:2222)  ── SSH 登录
         Server (:8443)  ◀── Agent WebSocket (出方向)
```

Agent 通过**出方向** WebSocket 连接 Server，无需暴露数据面端口。

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
curl -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"agentID": "agent-beijing"}'
```

返回证书和私钥，部署到数据面 Agent 的配置中。

### 9.3 UI 认证

部署时，`rlarkadm` 会在 kcp 集群的 `default` 命名空间自动创建 `rlark-ui-auth` Secret，包含随机生成的 admin 和 user 角色密码：

| 键 | 用途 |
|-----|------|
| `admin-password` | 管理员角色密码（16 位随机字符） |
| `user-password` | 用户角色密码（16 位随机字符） |

密码会在安装摘要中显示。Web UI 通过 `POST /api/v1/auth/login` 进行认证。

## 10. 高可用部署

### 10.1 多副本部署

生产环境建议的组件副本数：

| 组件 | 副本数 | 说明 |
|------|--------|------|
| Gateway | 2+ | 无状态，可水平扩展 |
| Server | 2+ | 通过 Peer Manager 互联 |
| Controller-Manager | 1 | Leader 选举，主备模式 |
| kcp | 1 | 单实例（可配合外部 etcd 集群） |
| etcd | 3 | 奇数节点 Raft 集群 |
| postgresql | 1 | 主备复制 |

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
  host: pg-primary.example.com
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

### 11.2 日志

使用结构化日志（zap），通过 `LOG_LEVEL` 环境变量控制日志级别：

```bash
LOG_LEVEL=debug ./bin/server --kubeconfig ...
```

### 11.3 健康检查

```bash
# 检查控制面
rlarkadm health -f deploy-control-plane.yaml

# 检查数据面
rlarkadm health -f deploy-data-plane.yaml
```

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
3. 检查 Server 日志：`kubectl logs -n rlark-system deployment/server`

### 训练任务无法启动

1. 检查 Node 是否有足够资源：`kubectl get nodes -n rlark-system`
2. 检查 Task 状态：查询对应 Task CR
3. 检查 Agent 日志：`kubectl logs -n rlark-system daemonset/agent`

### 跨集群网络不通

1. 检查 DomainPeer 是否已创建
2. 检查 Domain 证书是否签发成功
3. 检查 network-sidecar 是否注入到 Pod 中