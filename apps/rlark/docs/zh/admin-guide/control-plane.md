# 生产级控制面部署

## 架构

生产控制面由以下组件组成：

| 组件 | 角色 | `rlarkadm` 部署端口 |
|------|------|---------------------|
| kcp | 多集群控制面（Kubernetes API Server） | 6443 |
| PostgreSQL | 配置顶层 `db` 块时使用的可选持久化存储 | 5432 |
| rlark-server | 证书管理、Agent 隧道、SSH、健康检查和指标 | 8443（HTTPS/WSS）、2222（SSH）、8888（内部 HTTP） |
| rlark-gateway | 控制台和 CLI 的 REST API 网关 | 8090 |
| rlark-controller-manager | Job/Workflow/Domain 调和 | 8080（指标）、8081（健康检查） |
| rlark-ui | Web 管理控制台和 `/api/` 反向代理 | 80 |

Gateway 独立二进制默认监听 `:8080`，`rlarkadm` 部署时会覆盖为 `:8090`。

## 部署方式

### Docker Compose（开发/测试）

```bash
docker compose -f apps/rlark/docs/examples/docker-compose.yml up -d
```

### Kubernetes

从仓库维护的示例开始修改，并使用 `rlarkadm` 部署：

```bash
cp apps/rlark/docs/examples/deploy-control-plane.yaml deploy-control-plane.yaml
rlarkadm install -f deploy-control-plane.yaml
```

配置文件使用 kebab-case YAML 字段：

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  kubeconfig: ~/.kube/config
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  postgresql-image: postgres:15
  ui-image: rlark-ui:latest
  storage:
    type: pvc
    storage-class: ""
    size: 10Gi
```

`postgresql-image` 只选择镜像，不会单独启用 PostgreSQL。RLark 组件需要使用数据库时，应添加顶层 `db` 块，并替换所有示例凭据：

```yaml
db:
  host: postgresql
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

全部字段参见[配置项参考](../reference/configuration.md)。

## 关键配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| Server `--https-port` | `8443` | Agent 隧道、代理和证书操作 |
| Server `--ssh-port` | `2222` | 用户和跨集群 SSH |
| Server `--unsafe-http-port` | `8888` | 内部 `/healthz`、`/readyz`、`/livez`、`/metrics` 和 Peer 代理 HTTP |
| Server `--auto-sign-tls-ca-cert` | `false` | 缺少 TLS CA 和 Server 证书时生成；`rlarkadm` 会启用 |
| Server `--tls-domains` | `localhost` | 生成的 Server 证书中的 DNS 名称；`rlarkadm` 会传入 Service DNS 名称 |
| Gateway `--addr` | `:8080` | 独立二进制默认值；`rlarkadm` 使用 `:8090` |
| Controller Manager `--metrics-bind-address` | `:8080` | 指标端点 |
| Controller Manager `--health-probe-bind-address` | `:8081` | `/healthz` 和 `/readyz` |

## 安全加固

- 替换示例数据库凭据，不要把密钥写入源码仓库。
- 确保证书覆盖所有 Server 访问名称；生产入口使用可信 CA。
- 只按需开放 UI 80 和 Server 8443/2222 端口；Gateway、kcp、指标和健康检查端口保持内部可见。
- 通过可信且带认证的入口或反向代理限制 Gateway 访问。
- 接入生产集群前配置持久化存储、备份和升级流程。

## 部署后验证

`rlarkadm install` 会为每个 Kubernetes 工作负载等待最多 180 秒。当前没有 `rlarkadm health` 子命令，请检查工作负载和实际健康端点：

```bash
kubectl get deploy,statefulset,daemonset -n rlark-system
kubectl rollout status deployment/rlark-server -n rlark-system
kubectl rollout status deployment/rlark-controller-manager -n rlark-system

# Server 健康检查位于内部 HTTP 端口，而不是 HTTPS 8443。
kubectl port-forward -n rlark-system svc/rlark-server 8888:8888
curl --fail http://localhost:8888/healthz
curl --fail http://localhost:8888/readyz

# 在另一个终端中检查 Controller Manager。
kubectl port-forward -n rlark-system deployment/rlark-controller-manager 8081:8081
curl --fail http://localhost:8081/healthz
curl --fail http://localhost:8081/readyz

# 验证 UI 及其到 Gateway 的 /api/ 代理。
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80
curl --fail http://localhost:8080/
```

Gateway 没有专用健康路由，请通过 Deployment 就绪状态和日志检查。
