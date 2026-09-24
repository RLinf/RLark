# 配置项参考

## 数据库配置

通过 `--db-config` 参数加载的 PostgreSQL 连接配置。rlark-server、rlark-gateway 和 rlark-controller-manager 使用此配置。

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `host` | string | `localhost` | PostgreSQL 主机 |
| `port` | int | `5432` | PostgreSQL 端口 |
| `database` | string | `rlark` | 数据库名 |
| `user` | string | `rlark` | 数据库用户 |
| `password` | string | `rlark` | 数据库密码 |
| `maxOpenConns` | int | `25` | 最大打开连接数 |
| `maxIdleConns` | int | `5` | 最大空闲连接数 |
| `connMaxLifetime` | duration | `30m` | 连接最大生命周期 |
| `connMaxIdleTime` | duration | `5m` | 连接最大空闲时间 |
| `debug` | bool | `false` | 启用查询日志 |

!!! note "连接池调优"
    根据实际负载调整 `maxOpenConns` 和 `maxIdleConns`。高并发场景建议增大这两个值。

**示例：**

```yaml
host: postgresql
port: 5432
database: rlark
user: rlark
password: CHANGE_ME
maxOpenConns: 25
maxIdleConns: 5
connMaxLifetime: 30m
connMaxIdleTime: 5m
debug: false
```

## rlark-server

控制面服务器。管理 TLS/SSH 证书、Agent 注册和 Gateway API。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--https-port` | int | `8443` | HTTPS 监听端口 |
| `--ssh-port` | int | `2222` | SSH 监听端口 |
| `--unsafe-http-port` | int | `8888` | 内部 HTTP：`/healthz`、`/readyz`、`/livez`、`/metrics` 和 Peer 代理 |
| `--auto-sign-tls-ca-cert` | bool | `false` | 若 TLS CA 证书不存在则自动签发 |
| `--tls-domains` | strings | `["localhost"]` | TLS 证书域名列表 |
| `--db-config` | string | `""` | 数据库配置文件路径 |
| `--peer-service` | string | `""` | 集群对等服务 DNS 名称 |
| `--peers` | strings | `[]` | 集群对等服务器地址列表 |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig 文件路径 |
| `--master` | string | `""` | Kubernetes API Server 地址 |
| `--in-cluster` | bool | `false` | 使用 in-cluster 配置 |
| `--kube-namespace` | string | `""` | Kubernetes 命名空间 |
| `--kube-qps` | float32 | `5000` | Kubernetes 客户端 QPS |
| `--kube-burst` | int | `8000` | Kubernetes 客户端 Burst |
| `--kube-timeout` | duration | `0` | Kubernetes 客户端请求超时 |

!!! tip "`--unsafe-http-port` 用途"
    该端点没有认证。应保持内部可见；Agent TLS 连接和证书操作使用 8443 端口。

**示例：**

```bash
rlark-server \
  --https-port=8443 \
  --ssh-port=2222 \
  --auto-sign-tls-ca-cert \
  --tls-domains=localhost,rlark.example.com \
  --db-config=/etc/rlark/db-config.yaml
```

## rlark-gateway

API 网关。处理所有 REST API 请求，包括集群管理、任务管理和存储操作。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--addr` | string | `:8080` | API 网关绑定地址；`rlarkadm` 会覆盖为 `:8090` |
| `--db-config` | string | `""` | 数据库配置文件路径 |
| `--server-address` | string | `https://rlark-server.rlark-system.svc:8443` | 证书签名的 RLark Server 地址 |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig 文件路径 |
| `--master` | string | `""` | Kubernetes API Server 地址 |
| `--in-cluster` | bool | `false` | 使用 in-cluster 配置 |
| `--kube-namespace` | string | `""` | Kubernetes 命名空间 |
| `--kube-qps` | float32 | `5000` | Kubernetes 客户端 QPS |
| `--kube-burst` | int | `8000` | Kubernetes 客户端 Burst |
| `--kube-timeout` | duration | `0` | Kubernetes 客户端请求超时 |

**示例：**

```bash
rlark-gateway \
  --addr=:8080 \
  --db-config=/etc/rlark/db-config.yaml \
  --server-address=https://rlark-server:8443
```

## rlark-controller-manager

控制器管理器。调和 Job 和 Domain 资源。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--server-address` | string | `https://rlark-server.rlark-system.svc:8443` | RLark Server 地址 |
| `--db-config` | string | `""` | 数据库配置文件路径 |
| `--leader-election` | bool | `true` | 启用 Leader Election（高可用） |
| `--leader-election-key` | string | `rlark-controller-manager` | Leader Election 锁键（`name` 或 `namespace/name`） |
| `--leader-election-id` | string | `""` | 预留的参与者标识；controller-runtime 当前自行生成标识 |
| `--metrics-bind-address` | string | `:8080` | Metrics 端点绑定地址 |
| `--health-probe-bind-address` | string | `:8081` | 健康检查端点绑定地址 |
| `--job-controller-workers` | int | `8` | Job 控制器最大并发调和数 |
| `--task-controller-workers` | int | `8` | Task 控制器最大并发调和数 |
| `--workflow-controller-workers` | int | `8` | Workflow 控制器最大并发调和数 |
| `--node-controller-workers` | int | `8` | Node 控制器最大并发调和数 |
| `--domain-controller-workers` | int | `8` | Domain 控制器最大并发调和数 |
| `--job-sync-controller-workers` | int | `8` | Job 数据库同步控制器最大并发调和数 |
| `--task-sync-controller-workers` | int | `8` | Task 数据库同步控制器最大并发调和数 |
| `--workflow-sync-controller-workers` | int | `8` | Workflow 数据库同步控制器最大并发调和数 |
| `--node-sync-controller-workers` | int | `8` | Node 数据库同步控制器最大并发调和数 |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig 文件路径 |
| `--master` | string | `""` | Kubernetes API Server 地址 |
| `--in-cluster` | bool | `false` | 使用 in-cluster 配置 |
| `--kube-namespace` | string | `""` | Kubernetes 命名空间 |
| `--kube-qps` | float32 | `5000` | Kubernetes 客户端 QPS |
| `--kube-burst` | int | `8000` | Kubernetes 客户端 Burst |
| `--kube-timeout` | duration | `0` | Kubernetes 客户端请求超时 |

!!! note "单实例部署"
    单实例部署时建议设置 `--leader-election=false` 以避免不必要的选举开销。

**示例：**

```bash
rlark-controller-manager \
  --server-address=https://rlark-server:8443 \
  --db-config=/etc/rlark/db-config.yaml \
  --leader-election=false \
  --metrics-bind-address=:8080 \
  --health-probe-bind-address=:8081
```

## rlark-agent

数据面 Agent。部署在每个集群或节点上。管理节点注册、Task 执行和跨集群网络。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--server-address` | string | `https://localhost:8443` | RLark Server 地址 |
| `--server-hostname` | string | `""` | 服务器 TLS 预期主机名 |
| `--client-cert` | string | `""` | 客户端 TLS 证书路径 |
| `--client-key` | string | `""` | 客户端 TLS 私钥路径 |
| `--ca-cert` | string | `""` | CA 证书路径 |
| `--insecure-skip-tls-verify` | bool | `false` | 跳过 TLS 证书验证 |
| `--agent-type` | string | `Kubernetes` | Agent 类型：Kubernetes、Docker、Raw |
| `--mode` | string | `cluster` | Agent 模式：cluster、node、both |
| `--leader-election` | bool | `false` | 启用 Leader Election |
| `--leader-election-key` | string | `default/rlark-agent` | Leader Election Key（namespace/name） |
| `--leader-election-id` | string | `hostname-pid` | Leader Election 标识 |
| `--metrics-bind-address` | string | `:8081` | Metrics 端点绑定地址 |
| `--task-pull-controller-workers` | int | `8` | Task pull 控制器最大并发调和数 |
| `--addon-pull-controller-workers` | int | `8` | Addon pull 控制器最大并发调和数 |
| `--task-deployment-push-controller-workers` | int | `8` | Task Deployment push 控制器最大并发调和数 |
| `--task-daemonset-push-controller-workers` | int | `8` | Task DaemonSet push 控制器最大并发调和数 |
| `--task-statefulset-push-controller-workers` | int | `8` | Task StatefulSet push 控制器最大并发调和数 |
| `--node-push-controller-workers` | int | `8` | Node push 控制器最大并发调和数 |
| `--pod-push-controller-workers` | int | `8` | Pod push 控制器最大并发调和数 |
| `--pod-orphan-sweep-interval` | duration | `5m` | Agent 范围内管理 Pod 孤儿扫描间隔 |
| `--pod-orphan-sweep-page-size` | int | `200` | 每页扫描的管理 Pod 数量 |
| `--pod-stale-ttl` | duration | `15m` | 本地 Pod 缺失后以 `Unknown`/陈旧状态保留、再删除管理 Pod 的时长 |

Pod 孤儿扫描用于兜底处理遗漏的本地删除事件。它只删除 Agent 作用域内、本地 Pod UID 或已验证管理 Task UID 已失效的镜像。旧版镜像仅在以 UID 命名的镜像、存活本地 Pod 注解、管理命名空间、Task UID 以及可用的 Domain 全部一致时才会被接管；身份不明确的旧对象保持不变，需要手动清理。延迟删除会有意保留同名替代 Pod，因此旧镜像可能持续到下一次扫描。
| `--rlark-server-ssh-address` | string | `""` | RLark Server SSH 地址（user@host:port） |
| `--rlark-server-ssh-host-key` | string | `""` | RLark Server SSH Host Key |
| `--ssh-max-connections-per-domain` | int | `4` | 每个 Domain 按负载自适应扩展的物理 SSH 连接上限 |
| `--image` | string | `""` | RLark 网络 Sidecar 镜像 |
| `--enable-same-cluster-direct` | bool | `true` | 启用同集群 Pod 直接访问 |
| `--enable-cross-cluster-direct` | bool | `true` | 启用跨集群 Pod 直接访问 |
| `--kubelet-dir` | string | `""` | Kubelet 目录（用于查找 Pod UID） |
| `--image-pull-enabled` | bool | `true` | 启用镜像预拉取 |
| `--containerd-socket` | string | `/run/containerd/containerd.sock` | Containerd Socket 地址 |
| `--containerd-namespace` | string | `k8s.io` | Containerd 命名空间 |
| `--node-name` | string | `$NODE_NAME` | 本地节点名称 |
| `--nodeserver-unix-socket` | string | `/var/run/rlark/nodeserver.sock` | NodeServer Unix Socket 地址 |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig 文件路径 |
| `--master` | string | `""` | Kubernetes API Server 地址 |
| `--in-cluster` | bool | `false` | 使用 in-cluster 配置 |
| `--kube-namespace` | string | `""` | Kubernetes 命名空间 |
| `--kube-qps` | float32 | `5000` | Kubernetes 客户端 QPS |
| `--kube-burst` | int | `8000` | Kubernetes 客户端 Burst |
| `--kube-timeout` | duration | `0` | Kubernetes 客户端请求超时 |

!!! tip "`--mode` 参数说明"
    - `cluster`：仅运行集群级 Agent，管理集群范围内的资源
    - `node`：仅运行节点级 Agent，管理单个节点的 Task
    - `both`：同时运行集群和节点级 Agent

**示例：**

```bash
rlark-agent \
  --mode=both \
  --server-address=https://rlark-server:8443 \
  --client-cert=/etc/rlark/agent-cert.pem \
  --client-key=/etc/rlark/agent-key.pem \
  --ca-cert=/etc/rlark/ca-cert.pem \
  --image=rlark:latest \
  --node-name=worker-01
```

## rlark-network-sidecar

网络 Sidecar。与每个 Task Pod 一起运行，通过 TUN 设备和 gVisor netstack 实现跨集群 Pod 到 Pod 网络通信。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--sidecar-unix-socket` | string | `/var/run/rlark/nodeserver.sock` | NodeServer Unix Socket 路径 |
| `--sidecar-tun-name` | string | `gnet0` | TUN 设备名称 |
| `--sidecar-tun-mtu` | int | `1500` | TUN 设备 MTU |
| `--sidecar-proxy-listen` | string | `:5700` | Proxy TCP 监听地址 |
| `--sidecar-metrics-listen` | string | `:5790` | Metrics 与 pprof HTTP 监听地址；设为空值可禁用 |
| `--sidecar-hosts-sync-enabled` | bool | `true` | 启用 hosts 文件同步 |
| `--sidecar-hosts-sync-interval` | duration | `30s` | NodeServer 不支持 hosts watch 接口时的兜底轮询间隔 |
| `--sidecar-hosts-file` | string | `/etc/hosts` | hosts 文件路径 |

新版 sidecar 通过 NodeServer 的 `/watch_hosts` 长轮询接口接收 hosts 变化，通常可在约一秒内完成更新。如果接口不可用，会自动退回配置的轮询间隔，从而兼容旧版 NodeServer。

**示例：**

```bash
rlark-network-sidecar \
  --sidecar-unix-socket=/var/run/rlark/nodeserver.sock \
  --sidecar-tun-name=gnet0 \
  --sidecar-tun-mtu=1500
```

## rlark-tools sshd

`sshd` 子命令提供对运行中 Task Pod 的 SSH 访问。Agent 总是将 `rlark-tools` 注入到 `/rlark-tools/rlark-tools`，需要 SSH 的工作负载通过 `rlark-tools sshd` 启动服务。

| 参数 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `--port` | string | `22` | SSH 监听端口 |
| `--shell` | string | `""` | Shell 二进制路径（默认 /bin/bash） |

**环境变量：**

| 变量 | 说明 |
| ------ | ------ |
| `RLARK_SSH_PUBLIC_KEY` | 用于 authorized_keys 的 SSH 公钥 |

Agent 环境变量 `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES=true` 用于开启旧版任务模式：授予所有 Task 容器 privileged 权限，并为 Ray Head 以外的任务启用宿主机网络。该功能默认关闭，仅应在可信集群中使用。

## 存储 Provider 配置

Gateway 使用的对象存储后端配置。

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `accessKeyId` | string | `""` | 访问密钥 ID |
| `secretAccessKey` | string | `""` | 访问密钥 Secret |
| `bucket` | string | `""` | Bucket 名称 |
| `endpoint` | string | `http://localhost:9000` | 存储端点 |
| `region` | string | `us-east-1` | 区域 |
| `usePathStyle` | bool | `false` | 使用路径风格寻址 |
| `provider` | string | `""` | 存储提供商名称 |

!!! tip "MinIO vs AWS S3"
    使用 MinIO 时建议设置 `usePathStyle: true`，使用 AWS S3 时无需设置。

## rlarkadm 部署配置

`rlarkadm install -f` 使用的 YAML 配置文件。使用方法参见 [CLI 参考](cli.md#rlarkadm)。

### 顶层字段

下表名称是 `rlarkadm` 接受的准确 YAML 键名。

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `apiVersion` | string | — | API 版本（必填）；仓库示例使用 `rlark.io/v1alpha1` |
| `kind` | string | — | 必须为 `DeployConfig` |
| `plane` | string | — | 必填：`control`（控制面）或 `data`（数据面） |
| `control-plane-address` | string | `""` | Server HTTPS/WSS 地址；数据面部署时必填 |
| `db` | DBConfig | 未设置 | 数据库配置；在 `rlarkadm` 部署中启用 PostgreSQL 组件 |
| `kubernetes` | KubernetesEnv | 未设置 | Kubernetes 部署环境 |
| `docker` | DockerEnv | 未设置 | Docker 部署环境 |
| `raw` | RawEnv | 未设置 | Raw 部署环境 |
| `cert` | CertConfig | 未设置 | 证书配置；数据面部署时必填 |
| `insecure-skip-tls-verify` | bool | `false` | 跳过 Server TLS 验证 |

!!! warning "运行时支持范围"
    配置 schema 仍保留 `kubernetes`、`docker` 和 `raw`，但当前仅支持 Kubernetes 工作负载路径。现阶段部署不应使用 Docker 或 Raw；这两种路径不受支持，也不推荐使用。Kubernetes 数据面还必须提供 `control-plane-address` 和 `cert`。

### DBConfig

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `host` | string | `postgresql` | PostgreSQL 主机 |
| `port` | int | `5432` | PostgreSQL 端口 |
| `database` | string | `rlark` | 数据库名 |
| `user` | string | `rlark` | 数据库用户 |
| `password` | string | `rlark` | 数据库密码 |

### KubernetesEnv

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `management-api` | string | `kcp` | 管理 API 模式：`kcp` 部署 kcp 和可选 etcd；`kubernetes` 将 RLark 资源存储在目标集群中，且不部署 kcp/etcd |
| `kubeconfig` | string | `""` | kubeconfig 文件路径；空值使用 client-go 常规加载规则 |
| `gateway-image` | string | `""` | Gateway 镜像 |
| `controller-manager-image` | string | `""` | Controller Manager 镜像 |
| `server-image` | string | `""` | Server 镜像 |
| `agent-image` | string | `""` | Agent 镜像 |
| `image` | string | `""` | RLark 共享镜像回退值；在数据面还会启用网络 Sidecar 和 SSH 支持 |
| `kcp-image` | string | `""` | kcp 镜像 |
| `etcd-image` | string | `""` | 内置 etcd 镜像；仅设置该字段且未配置外部地址时启用内置 etcd |
| `postgresql-image` | string | `""` | PostgreSQL 镜像；仅设置顶层 `db` 块时启用 PostgreSQL |
| `ui-image` | string | `""` | UI 镜像 |
| `image-pull-secrets` | 字符串列表 | 空 | `rlark-system` 命名空间中已有的镜像拉取 Secret 名称，应用到所有组件 Pod |
| `replicas` | int | `0`（解析为 `1`） | 组件默认副本数 |
| `storage` | StorageConfig | 未设置 | 默认存储配置 |
| `kcp` | ComponentConfig | 未设置 | kcp 当前限制为单副本。未配置 `etcd` 时使用 StatefulSet 并支持持久化存储；部署或指定外部 etcd 时使用 Deployment |
| `etcd` | EtcdConfig | 未设置 | etcd 组件配置。`address` 为空时部署 etcd，非空时使用外部 etcd |
| `postgresql` | ComponentConfig | 未设置 | PostgreSQL 组件配置 |
| `containerd-socket` | string | `/run/containerd/containerd.sock` | 节点 Agent 的 Containerd Socket 路径 |

!!! tip "镜像优先级"
    `gateway-image` 等组件专用镜像优先于 `image`。

### DockerEnv

!!! warning "当前不支持"
    这些字段为兼容性保留在 schema 中，但 Docker 不是当前支持的工作负载路径，不建议用于部署。

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `gateway-image` | string | Gateway 镜像 |
| `controller-manager-image` | string | Controller Manager 镜像 |
| `server-image` | string | Server 镜像 |
| `agent-image` | string | Agent 镜像 |
| `image` | string | RLark 共享镜像回退值 |
| `kcp-image` | string | kcp 镜像 |
| `etcd-image` | string | etcd 镜像 |
| `postgresql-image` | string | PostgreSQL 镜像 |
| `ui-image` | string | UI 镜像 |

### RawEnv

!!! warning "当前不支持"
    这些字段为兼容性保留在 schema 中，但 Raw 不是当前支持的工作负载路径，不建议用于部署。请使用 Kubernetes。

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `gateway-artifact` | string | Gateway 二进制路径 |
| `controller-manager-artifact` | string | Controller Manager 二进制路径 |
| `server-artifact` | string | Server 二进制路径 |
| `agent-artifact` | string | Agent 二进制路径 |
| `network-sidecar-artifact` | string | Network Sidecar 二进制路径 |
| `kcp-artifact` | string | kcp 二进制路径 |
| `etcd-artifact` | string | etcd 二进制路径 |
| `postgresql-artifact` | string | PostgreSQL 二进制路径 |

### CertConfig

数据面部署时必须提供。

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `ca-cert` | string | 内联 CA PEM 或已存在的文件路径 |
| `agent-cert` | string | 内联 Agent 证书 PEM 或已存在的文件路径 |
| `agent-key` | string | 内联 Agent 私钥 PEM 或已存在的文件路径 |

### StorageConfig

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | -------- | ------ |
| `type` | string | | 存储类型：emptyDir、hostPath、pvc |
| `host-path` | string | `""` | hostPath 类型的主机路径 |
| `storage-class` | string | `""` | PVC 类型的 StorageClass；空值使用集群默认值 |
| `size` | string | `""` | 存储大小 |
| `node-selector` | map | 空 | 工作负载节点选择器 |

### ComponentConfig

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `replicas` | int | 副本数 |
| `storage` | StorageConfig | 存储配置 |

### EtcdConfig

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `address` | string | etcd 地址 |
| `replicas` | int | 副本数 |
| `storage` | StorageConfig | 存储配置 |

**示例（控制面）：**

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  postgresql-image: postgres:15
  ui-image: rlark-ui:latest
  replicas: 1
db:
  host: postgresql
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

**示例（数据面）：**

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
```
