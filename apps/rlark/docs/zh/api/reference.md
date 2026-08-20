# API 参考

本页仅列出 Gateway 在 [`pkg/gateway/router.go`](https://github.com/RLinf/RLark/tree/main/apps/rlark/pkg/gateway/router.go) 中实际注册的 HTTP 路由。可运行请求参见 [API 调用样例](examples.md)，机器可读子集参见 [OpenAPI 规范](../../api/swagger.yaml)。

!!! warning "认证限制"
    `POST /api/v1/auth/login` 只校验内置 Web UI 凭据并返回角色，不会建立服务端会话或签发 token。Gateway 当前不会在下述 API 路由上强制校验该登录结果。请勿将 Gateway 直接暴露到不受信任的网络；应在入口前配置带认证的反向代理或其他可信访问控制层。

下表用 `{name}` 表示路径参数；Router 源码中的 Gin 等价写法为 `:name`。Namespaced CRD 路由需要 `namespace` query 参数。

## CRD 资源

| 资源 | 作用域 | 路由 |
|------|--------|------|
| `nodes` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/nodes`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}` |
| `workflows` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/workflows`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/workflows/{name}` |
| `jobs` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/jobs`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/jobs/{name}`；`GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs`；`GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/metrics` |
| `tasks` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/tasks`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}`；`/api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}` 接受所有方法 |
| `pods` | Namespaced | `GET /api/v1/rlinf.io/v1alpha1/pods`；`GET, PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}`；`GET /api/v1/rlinf.io/v1alpha1/pods/{name}/events`；`GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal` |
| `domains` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/domains`；`GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}` |

Gateway Router 未暴露 CRD status 子资源路由；状态随普通资源响应返回。

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
| `DELETE` | `/api/v1/ssh-user-keys/{id}` |

## 镜像仓库与系统配置

| 方法 | 路径 |
|------|------|
| `GET, POST` | `/api/v1/image-registries` |
| `GET, PUT, DELETE` | `/api/v1/image-registries/{name}` |
| `GET, PUT` | `/api/v1/system-config` |

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
