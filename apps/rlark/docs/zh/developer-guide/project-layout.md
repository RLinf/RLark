# 项目结构

- `api/`：CRD 类型和生成的 Kubernetes Client。
- `apps/rlark/`：控制面、Agent、CLI 工具和本文档。
- `apps/rlark-ui/`：React 与 TypeScript Web 控制台；`src/generated/rlark/` 下的 CRD
  TypeScript 模型由 `api/config/crd/bases` 生成并签入仓库。
- `apps/embodied-runtime/`：设备插件及机器人、相机控制器。
- `proto/` 与 `sdks/`：运行时协议和客户端 SDK。

修改架构或行为前，请先阅读[项目设计原则](design-principles.md)。当前 Agent、构建与贡献规范参见仓库根目录的 `AGENTS.md` 和 `CONTRIBUTING.md`。
