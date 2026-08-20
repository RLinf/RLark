# Release Notes / Changelog

RLark 当前没有发布稳定版本。仓库的 `main` 分支是开发快照；正式 Release 可用后，发布说明将随仓库 Release 提供。不要根据开发元数据推断已经发布了 `0.1.x` 版本。

## 开发版本兼容性

| 来源 | 控制面 / Agent | Kubernetes 数据面 | kcp | PostgreSQL | 状态 |
|------|----------------|---------------------|-----|------------|------|
| `main` | 所有 RLark 组件使用同一提交构建 | 1.31（kind 开发环境） | 0.30 | 15 | 仅限开发；不保证升级兼容性 |

其他组合尚未验证。控制面和全部 Agent 应使用同一提交，不支持混用不同 RLark 提交。

- [GitHub Releases](https://github.com/RLinf/RLark/releases)
- [仓库提交记录](https://github.com/RLinf/RLark/commits/main/)
