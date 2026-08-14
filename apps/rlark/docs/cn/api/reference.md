# API 参考

API 参考文档由 CRD 代码自动生成，请查看英文版本：

> [API Reference (English)](../api/reference.md)

## 支持的操作

所有 CRD 资源支持以下标准 Kubernetes-style REST 操作：

| 操作 | HTTP 方法 | 路径 |
|------|-----------|------|
| List | `GET` | `/api/v1/rlinf.io/v1alpha1/{resources}` |
| Create | `POST` | `/api/v1/rlinf.io/v1alpha1/{resources}` |
| Get | `GET` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}` |
| Replace | `PUT` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}` |
| Patch | `PATCH` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}` |
| Delete | `DELETE` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}` |
| Get Status | `GET` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}/status` |
| Replace Status | `PUT` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}/status` |
| Patch Status | `PATCH` | `/api/v1/rlinf.io/v1alpha1/{resources}/{name}/status` |

## 资源列表

| 资源 | 作用域 | 说明 |
|------|--------|------|
| `addons` | Namespaced | 组件管理，安装在数据面集群的插件 |
| `domains` | Cluster | 安全域，网络隔离和证书边界 |
| `domainpeers` | Namespaced | 域对等，数据面集群内 Domain 的 Pod 路由表 |
| `jobs` | Cluster | 训练作业，包含多个 Task 模板 |
| `nodes` | Namespaced | 计算节点，自动注册和上报 |
| `pods` | Namespaced | 控制面中的 Pod 镜像 |
| `tasks` | Namespaced | 任务执行单元，由 Job Controller 自动创建 |
| `workflows` | Cluster | 工作流，多个 Job 的 DAG 编排 |

## 详细字段说明

详细的请求/响应 Schema 请参考：

- [API 调用样例](../api/examples.md) — 端到端调用示例
- [核心概念](../concepts.md) — 资源模型和概念解释