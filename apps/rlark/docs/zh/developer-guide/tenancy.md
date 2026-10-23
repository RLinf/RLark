# 用户、租户与业务命名空间

RLark 将认证用户与业务租户分开建模。用户表示谁执行操作；租户表示业务资源归属。数据面 Namespace 根据 Tenant、集群上下文和部署策略解析，而不是用来定义 Tenant 身份。

RLark 核心模型不要求 Tenant 与 Namespace 是同一个概念，同时也不干涉上层产品或具体部署配置一对一的 Tenant-to-Namespace 映射。该映射应保存在明确的租户配置或 Namespace Resolver 中，以便未来在不改变资源归属语义的情况下扩展为按集群映射或一对多放置。

## 兼容模型

当前用户 Provider 继续读取已有的 `rlark-ui-auth` Secret，并在内存中构造 `admin` 和 `user` 两个 API 对象。Secret 的 Data 键和密码格式均不改变。当前租户 Provider 同样只在内存中提供一个租户：

| 租户 | 数据面 Namespace |
| --- | --- |
| `default` | `rlark-system` |

上表描述当前默认部署映射，不是核心模型的永久约束。

旧版本创建的资源没有租户元数据。RLark 在读取、鉴权、查询和派生子资源时，将这些资源视为属于 `default` 租户。默认租户信息仅在内存中构造，不会回写旧记录。

## 元数据

新业务资源使用 Kubernetes 已有的 metadata 字段，因此不改变 CRD spec、数据库 Schema 或已存储的 Secret 内容格式：

```yaml
metadata:
  labels:
    rlark.io/tenant-id: default
  annotations:
    rlark.io/created-by: user
```

租户元数据沿 Workflow → Job → Task → Pod 继承，并附加到 Task 创建的数据面工作负载。StorageClass 和镜像仓库资源也会展示所属租户。现有 CSI Secret 的放置方式以及镜像凭据分发规则保持不变。

## 强制点

租户元数据之所以构成边界，是因为 Gateway 在每次请求上强制校验它。Workflow、Job、Task、Pod 属于租户作用域；Node 与 Domain 是基础设施对象，不在该作用域内。`admin` 角色的调用方不受限制；没有任何租户归属的调用方会被直接拒绝，而不是当作 `default`——上文的 `default` 回退只适用于租户元数据出现之前写入的对象，不适用于调用方。

List 结果会收窄到调用方所属的租户；归属其他租户的具名对象报告为 `404` 而不是 `403`；作用域之外的 `?tenant=` 取值以 `403` 拒绝。完整的入口点清单与失败语义见[租户作用域强制](api-conventions.md#51)。

## 只读接口

- `GET /api/v1/users`（仅管理员）
- `GET /api/v1/users/me`
- `GET /api/v1/users/{id}`
- `GET /api/v1/tenants`
- `GET /api/v1/tenants/{id}`

`GET /api/v1/users/me` 始终解析为已认证的调用方，对所有角色开放。`GET /api/v1/users/{id}` 在 `{id}` 即调用方本人时对所有角色开放，否则要求管理员角色，不满足时以 `403` 拒绝。

在引入可持久化的 Provider 之前，暂不开放用户和租户写接口。
