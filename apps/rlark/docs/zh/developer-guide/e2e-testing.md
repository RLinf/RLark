# 端到端测试

RLark 的仓库级 E2E 框架位于 `test/e2e`。每个测例在代码中声明基础设施需求，RLark 入口为
本轮测试创建并销毁隔离的 Kind 集群。

共享框架支持任意声明的集群角色，不定义产品 API、镜像或拓扑。各产品的环境适配器保存在
对应测试套件下。RLark 入口使用 Kind：它按每个测例在代码中声明的需求创建带唯一名称的集群，
导出本轮 kubeconfig、收集日志，并且只删除本轮创建的集群；该入口不会连接已有集群。

集群角色和节点数量属于测例需求，不是全局环境配置。Provider 必须满足测例声明，不能根据
外部配置改变被测试的内容。

共享 scenario runner 统一负责基础设施创建、产品准备、测例执行、证据收集、产品清理和基础
设施销毁。证据收集和清理使用独立的有界 context，避免测例超时后丢失失败证据或跳过清理。
`E2E_RETAIN_ENVIRONMENT` 是唯一的环境保留开关。

框架的贡献者 README 为 `test/e2e/README.md`，其中说明了目录结构、可配置的镜像、运行命令、
资源所有权规则及证据输出契约。

常用命令：

```bash
# 创建 Kind、部署 RLark、验证、收集证据并清理
make test-e2e-rlark
```

这是唯一的 RLark E2E 入口。它会构建 `rlarkadm` 和默认 RLark/UI 镜像，创建隔离的
Kind 集群、加载镜像、使用 Kubernetes management API 安装 RLark、验证 API discovery 和组件
可用性、收集证据、卸载 RLark 并删除集群。可通过 `RLARK_SMOKE_IMAGE` 和
`RLARK_SMOKE_UI_IMAGE` 选择其他构建 tag；如需直接使用已有的本地不可变镜像，还需设置
`RLARK_SMOKE_BUILD_IMAGES=false`。入口会校验并加载指定镜像，不会替换它们。受限环境可通过
`RLARK_SMOKE_GO_BASE_IMAGE`、`RLARK_SMOKE_RUNTIME_BASE_IMAGE` 和
`RLARK_SMOKE_UI_BASE_IMAGE` 指定基础镜像；仓库默认值不包含私有 registry。

镜像、namespace、功能配置和升级版本等产品部署输入属于使用它们的测例，而不是全局框架
配置。真实场景必须使用不可变镜像身份、唯一运行标签、有界且感知 context 的轮询，以及经过
所有权证明的清理。测试框架不允许删除无测试标签的资源或产品 finalizer。
