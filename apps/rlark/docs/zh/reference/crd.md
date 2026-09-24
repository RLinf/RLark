# CRD 参考

> **生成说明：** 完整英文参考由 `api/config/crd/bases` 中的 CRD manifests 自动生成，请勿手工修改；应运行 `make generate-crd-schema-docs` 重新生成。本中文页是人工维护的概要。

本页说明由 kcp/Kubernetes API Server 提供的底层 Kubernetes CRD API，而不是 RLark Gateway HTTP API。Gateway 仅暴露其中一部分资源操作，并增加日志、镜像、存储、证书等路由；其准确边界参见 [Gateway API 参考](../api/reference.md)。

## 资源与作用域

当前 `rlinf.io/v1alpha1` CRD 包括：

| 资源 | 作用域 |
|------|--------|
| `addons` | Namespaced |
| `domains` | Cluster |
| `domainpeers` | Namespaced |
| `jobs` | Cluster |
| `nodes` | Namespaced |
| `pods` | Namespaced |
| `tasks` | Namespaced |

底层 CRD API 遵循 Kubernetes 资源语义，可包含列表、创建、读取、替换、Patch、删除、集合删除及 status 子资源等操作。是否支持某个字段或操作，以当前 CRD manifest 和生成结果为准。

状态阶段枚举中，`Pod.status.phase` 支持 `Pending`、`Running`、`Succeeded`、`Failed`、`Unknown`。

Gateway 不会原样透传这套完整 API：它实际只开放部分资源和方法，且没有 collection delete（集合删除）或 `/status` 路由；因此不能根据本页推断 Gateway 存在同名接口。Gateway 的准确路由边界请查阅 [Gateway API 参考](../api/reference.md)，完整字段和底层操作请查阅 [英文生成参考](../../reference/crd.md)。

## 生成来源与维护方式

CRD 类型定义位于 `api/rlark.io/v1alpha1/`，CRD manifests 和生成脚本位于 `api/` 下，生成的 Go client 位于 `api/kubeclients/`。精确字段、枚举、必填项和完整操作清单以英文生成参考为准：

- [完整英文 CRD Schema Reference](../../reference/crd.md)
- [Gateway API 参考](../api/reference.md)
- [核心概念](../concepts.md)

中文页刻意保持为可维护摘要，不手工复制完整生成内容；CRD 变更后应重新生成英文参考，并同步更新本页的资源清单与边界说明。
