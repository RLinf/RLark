# 工作流

使用 Workflow 将多个 Job 模板连接为 DAG。每个模板会在其依赖成功后生成一个子 Job。

## 前置条件

创建 Workflow 前：

- 确认控制面和 Workflow 控制器正常运行，并且当前用户可以创建独立 Job。
- 纳管所有目标集群，确认其节点已显示为可用 Worker。
- 确保各目标集群可以拉取对应镜像，并提前创建所需的 Domain、StorageClass 或 PVC。
- 确保每个阶段仅在工作真正完成后以状态码 0 退出；依赖阶段依据子 Job 状态放行，而不是依据 Shell 输出。

## 任务一：创建 DAG

1. 打开**工作流**并选择**创建工作流**。
2. 输入 Workflow 名称。
3. 在 **DAG 编排**中为每个阶段添加一个 Job 节点。
4. 双击节点名称进行重命名。
5. 从节点右侧输出端口拖到目标节点以创建依赖；点击连线可删除依赖。
6. 确认图中没有自环或环路；编辑器会拒绝这两种关系。

## 任务二：配置每个 Job

1. 进入 **Job 详情**。
2. 依次选择每个 Job 标签页。
3. 按需配置类型、角色、Header 角色、目标集群、Worker 资源、节点选择器、镜像、环境变量、存储、Domain 和运行脚本。
4. 确保每个角色都有目标集群和镜像，并至少匹配一个可用 Worker。

Job 配置与独立 Job 表单基本一致，但当前 Workflow 表单不包含 SSH 密钥和 TensorBoard 设置。

## 任务三：检查并提交

1. 进入 **YAML 预览**。
2. 确认 Workflow 和模板名称唯一，并符合 Kubernetes 资源命名规则。
3. 检查各模板的 `dependencies`、镜像、存储、Domain 和 Task 配置。
4. 选择**创建工作流**。

控制台提交的是 Workflow CR，不会生成 Shell 安装命令。

## 任务四：监控运行

1. 从列表打开 Workflow。
2. 查看 DAG 执行视图和子 Job 表格。
3. 选择非 Pending 的 DAG 节点打开对应子 Job。生成的 Job 名称采用 `<workflow-name>-<template-name>`。
4. 排障时分别检查子 Job 的 Worker 和日志。

依赖阶段只会在所有前置阶段成功后启动。如果前置 Job 的脚本已结束但状态仍为 Running，请检查是否还有后台进程。

## 验证结果

- 确认 Workflow 状态变为 `Succeeded`，并且所有 DAG 节点均为 `Succeeded`。
- 确认子 Job 表中每个模板都有一个 Job，且生成名称符合 `<workflow-name>-<template-name>`。
- 打开各子 Job，核对 Worker 数量、目标集群、日志以及预期输出或产物。仅 DAG 显示绿色并不能验证应用层结果。

## 失败处理

任一子 Job 变为 `Failed` 后，Workflow 会进入终态 `Failed`，尚未启动的依赖阶段不会再被放行。已有子 Job 不提供回滚能力；如有需要，应分别检查或停止这些 Job。

1. 打开失败的 DAG 节点，检查其 Worker、事件和容器日志。
2. 检查镜像拉取权限、集群与 Worker 可用性、选择器和资源申请、存储挂载、Domain 连通性以及脚本退出码。
3. 修正底层配置或工作负载。当前 Workflow 无法从失败节点续跑；请使用唯一名称提交新的 Workflow（或删除旧 Workflow 后复用其名称）。
4. 按上述检查项验证替代运行。

## API 等效操作

通过 `POST /api/v1/rlinf.io/v1alpha1/workflows` 创建 Workflow CR，再查询 Workflow 和生成的 Job。详见 [CRD 参考](../reference/crd.md)。
