# 开发者指南

本节面向修改 RLark 项目本身的贡献者。建议先阅读[项目设计原则](design-principles.md)、[项目结构](project-layout.md)和[技术架构](../architecture.md)，再搭建[本地源码环境](local-development.md)。产品部署和集群纳管属于[管理员指南](../admin-guide/index.md)。

在修改 API 类型、控制器 status 写入或 Gateway 接口之前，请阅读[API 约定](api-conventions.md)：其中定义了共享术语、字段所有权、Phase 与 Condition 规则、稳定身份、租户元数据，以及 REST 错误、分页和过滤契约。

开发者文档以 API、CRD、CLI、源码和调试流程为主，不依赖 Web UI 截图。面向用户的新能力必须保证 UI 与 API 语义一致。

Task 生命周期协调、Workload、Environment 与 Runtime 之间的内部执行架构和兼容规则，请参阅 [Runtime、Workload 与 Environment 边界](runtime-provider.md)。
