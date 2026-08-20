# Agent 安装与升级

## 安装

Agent 运行在每个数据面集群中。`rlarkadm` 创建的 Kubernetes 部署包括：

- `rlark-agent` Deployment，以 `--mode=cluster` 运行，负责集群级同步。
- `rlark-agent-node` DaemonSet，以 `--mode=node` 运行，负责节点网络和镜像预拉取。

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

三个证书字段都可填写内联 PEM 或已存在的文件路径。`kubernetes.image` 可选；设置后启用网络 Sidecar 和 SSH 支持。建议从仓库维护的示例开始：

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

多节点 Kubernetes 数据面不要在单个 Deployment 中混合 cluster 和 node 模式。`rlarkadm` 会创建正确的 Deployment、DaemonSet、证书 Secret、RBAC、Socket 挂载和容器运行时挂载。

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

## 配置参考

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--mode` | `cluster` | `cluster`、`node` 或 `both`；Kubernetes `rlarkadm` 使用分离的 cluster/node 工作负载 |
| `--agent-type` | `Kubernetes` | 运行时类型：Kubernetes、Docker 或 Raw |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS 地址 |
| `--metrics-bind-address` | `:8081` | 指标监听地址 |
| `--image` | `""` | 网络 Sidecar 和 SSH 镜像 |
| `--leader-election` | `false` | Leader Election；需要多副本时 `rlarkadm` 为集群 Agent 启用 |
| `--enable-cross-cluster-direct` | `true` | 允许跨集群 Pod 直连路由 |
| `--containerd-socket` | `/run/containerd/containerd.sock` | 节点 Agent 的 Containerd Socket |

完整参数列表参见[配置项参考](../reference/configuration.md#rlark-agent)。
