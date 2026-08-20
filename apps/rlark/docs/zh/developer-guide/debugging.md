# 开发与调试

开发过程中运行相关单元测试，提交前再执行仓库 lint 与测试目标。通过结构化组件日志、Kubernetes 事件和 CR 状态变化跟踪调谐流程。修改 CRD 后同步生成 API Client；面向用户的改动需要同时验证 API 与 Web UI。

## 前端数据模式

Web UI 使用 `VITE_DATA_MODE` 区分数据来源：开发环境默认使用 `mock`，联调和生产使用 `backend`。Mock 仅用于界面开发与文档截图，不代表真实资源状态；验证集群、节点、任务、存储和容量统计时必须显式使用后端模式：

```bash
cd apps/rlark-ui
VITE_DATA_MODE=backend npm run dev
```

语言、主题和侧栏状态保存在浏览器本地。列表与详情使用稳定 URL，便于刷新、复制链接和返回导航。这些属于前端实现与调试约定，不作为平台用户操作指南。
