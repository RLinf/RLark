# API 参考

本页仅列出 Gateway 在 [`pkg/gateway/router.go`](https://github.com/RLinf/RLark/tree/main/apps/rlark/pkg/gateway/router.go) 中实际注册的 HTTP 路由。可运行请求参见 [API 调用样例](examples.md)，机器可读子集参见 [OpenAPI 规范](../../api/swagger.yaml)。

除 `POST /api/v1/auth/login` 和 `/metrics` 外，Gateway 路由均要求在 `Authorization: Bearer <token>` 请求头中携带登录返回的 JWT。token 默认 8 小时过期，可通过 `--jwt-token-ttl` 配置。缺失或无效 token 返回 `401`；已认证 `user` 调用仅管理员接口返回 `403`。

仅管理员操作包括 Node 和 Domain 变更，以及全部证书、镜像仓库、Addon 接口、系统配置更新和 StorageClass provider/变更接口。系统配置读取及全部 SSH 密钥操作允许任一已认证角色访问。授权按角色执行，尚未按具体用户隔离资源，包括 SSH 密钥。

下表用 `{name}` 表示路径参数；Router 源码中的 Gin 等价写法为 `:name`。Namespaced CRD 路由需要 `namespace` query 参数。

## CRD 资源

| 资源 | 作用域 | 路由 |
|------|--------|------|
| `nodes` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/nodes`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}` |
| `jobs` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/jobs`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/jobs/{name}`；`GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs`；`GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs/label-values`；`GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/metrics` |
| `tasks` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/tasks`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}`；`/api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}` 接受所有方法 |
| `pods` | Namespaced | `GET /api/v1/rlinf.io/v1alpha1/pods`；`GET, PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}`；`GET /api/v1/rlinf.io/v1alpha1/pods/{name}/events`；`GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal` |
| `domains` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/domains`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}` |

创建 Job 时，Gateway 会将请求中的 `metadata.name` 保存为展示名，并在响应的 `metadata.name` 中返回系统生成的 `jo-<16 位十六进制字符>` 资源 ID。后续 Job API 请求应使用该返回 ID。

Gateway Router 未暴露 CRD status 子资源路由；状态随普通资源响应返回。

Job 日志接口已实现：`logs` 支持 `from`、`to`、`task`、`pod`、`query`、`cursor`、`order`。`from`、`to` 使用 RFC 3339 时间；`order=asc` 表示后端结果升序，其他值均为降序。提供 `from` 且已配置日志后端时，成功响应包含 `source: "backend"`、`entries`、`hasMore`、`nextCursor`；其他情况（包括后端查询失败）回退到 Pod 日志，响应包含 `source: "pod"` 和 `pods` 数组，数组项含 `taskName`、`podName`、`phase`、`node`、`logs`。

`logs/label-values` 支持 `label`（默认 `pod`）、`from`、`to`、`task`、`pod`，返回 `{ "values": [...] }`；未配置日志后端时数组为空。Job `metrics` 已注册，但当前仅返回 HTTP `501 Not Implemented`。

## 集群与证书

| 方法 | 路径 |
|------|------|
| `GET` | `/api/v1/clusters` |
| `GET` | `/api/v1/clusters/{cluster_id}` |
| `GET` | `/api/v1/certificates/agent` |
| `GET` | `/api/v1/certificates/agent/{cluster_id}` |
| `POST` | `/api/v1/certificates/agent` |

Gateway 虽注册了 `POST /api/v1/certificates/revoke`，但该接口尚未实现，不应视为可用 API。

## 认证与 SSH 密钥

| 方法 | 路径 |
|------|------|
| `POST` | `/api/v1/auth/login` |
| `GET` | `/api/v1/ssh-user-keys` |
| `POST` | `/api/v1/ssh-user-keys` |
| `DELETE` | `/api/v1/ssh-user-keys/{index}?user={user}` |

## API 参考元数据

| 方法 | 路径 |
|------|------|
| `GET` | `/api/v1/api-reference` |

该接口要求认证，返回本地化分类名称、接口方法与路径、描述及响应示例。Web UI 的“接口参考”页面以此接口为数据源，不再单独维护接口列表。

## Gateway 可观测性

| 方法 | 路径 |
|------|------|
| `GET` | `/metrics` |

`GET /metrics` 以 Prometheus 文本格式暴露 Gateway 指标。

## 镜像、镜像仓库与系统配置

| 方法 | 路径 |
|------|------|
| `GET` | `/api/v1/images` |
| `GET, POST` | `/api/v1/image-registries` |
| `GET, PUT, DELETE` | `/api/v1/image-registries/{id}` |
| `GET, PUT` | `/api/v1/system-config` |

系统配置分为 `ssh` 和 `log` 两类。`ssh.jumpHost` 必须是不含协议、用户、路径和端口的主机名或 IP；设置 `ssh.jumpPort` 时，其值必须为 1 到 65535 之间的整数。将 `log.backend` 设置为 `none` 可关闭历史日志查询，设置为 `sls` 时必须提供完整的 SLS 连接参数。响应中的 SLS 敏感凭据显示为 `****`，将该占位符原样提交表示保留已存储值。`PUT` 成功后返回最终生效且已掩码的配置。

可选的 `deployment` 分类沿用 `rlarkadm` `DeployConfig` 中与数据面 Agent 相关的子集，用于控制签发数据面 Agent 集群后展示的部署 YAML 默认值。仅接受 `controlPlaneAddress`、`sshAddress`、`insecureSkipTlsVerify`，以及 Kubernetes 下的 `kubeconfig`、`agentImage`、`image`、`imagePullPolicy`、`imagePullSecrets` 和 `containerdSocket`。控制面组件、数据库、证书、Docker 和 Raw 部署字段会被拒绝。`controlPlaneAddress` 为空时使用签发接口返回的 Server 地址；镜像拉取策略可为 `Always`、`IfNotPresent` 或 `Never`。

镜像仓库凭证使用不可变的 `ir-<16 位小写十六进制字符>` ID；展示 `name` 可以重复或修改。创建请求必须包含 `name`、`registry`、`username`、`password` 和 `clusterSelection`：

```json
{"name":"生产 Harbor","registry":"harbor.example.com","username":"robot","password":"secret","clusterSelection":{"mode":"Selected","clusters":["cluster-a"]}}
```

`clusterSelection.mode` 可为 `None`（仅保存）、`Selected`（一个或多个逻辑集群名）或 `All`（当前及未来集群）。`None` 和 `All` 模式下 `clusters` 必须为空。响应不返回密码，但会返回 `id`。`POST` 返回 `201 Created`。执行 `PUT` 时，省略 `password` 表示保留原密码，显式空密码非法。`DELETE` 返回 `202 Accepted`，因为 Replication 和 Delivery 会异步删除已分发的 Secret。

## 存储

| 方法 | 路径 |
|------|------|
| `GET, POST` | `/api/v1/storage/storageclass` |
| `PUT, DELETE` | `/api/v1/storage/storageclass/{name}` |
| `GET` | `/api/v1/storage/storageclass/provider` |
| `GET` | `/api/v1/storage/storageclass/{name}/{cluster}/list` |
| `POST` | `/api/v1/storage/storageclass/{name}/{cluster}/upload` |
| `GET, DELETE` | `/api/v1/storage/storageclass/{name}/{cluster}/object/{key}` |

## Addon

| 方法 | 路径 |
|------|------|
| `GET` | `/api/v1/addons` |
| `GET` | `/api/v1/addons/{name}` |
| `GET` | `/api/v1/installed-addons` |
| `GET, POST` | `/api/v1/clusters/{cluster_id}/addons` |
| `GET, PUT, DELETE` | `/api/v1/clusters/{cluster_id}/addons/{name}` |
