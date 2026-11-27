# 跟官方 RLark

- 官方仓库：`https://github.com/RLinf/RLark`。
- 官方基线：`d06745e`。`9ad4a21` 是最初的快照导入点，仍可用于追溯站点层起源。
- 官方文档（`apps/rlark/docs/`、`mkdocs.yml`、`apps/rlark/pkg/addons/catalog/README.md`）保持原样；我们的接口写在 [swagger.yaml](swagger.yaml)。

## 改了哪些官方文件

`git diff --name-status 9ad4a21 HEAD | grep -v '^A'` 列出全部。按原因分：

官方文件里只放钩子（尽量一行，调同包 `astribot_*.go` 里的函数）和必要的修正；我们的逻辑都在自己的文件里。下表的“钩子”指这种一行调用，“改动”指只能改在官方代码里的地方，各自写了原因。

| 类 | 文件 | 钩子 / 改动 |
|---|---|---|
| 生成物 | `api/config/crd/bases/rlinf.io_*.yaml`、`api/kubeclients/**`、`api/rlark.io/v1alpha1/zz_generated.deepcopy.go`、`apps/rlark-ui/src/generated/**`、`apps/rlark/docs/reference/crd.md` | 我们给 CRD 加了字段；`make generate` 重新生成 |
| API 字段 | `api/rlark.io/v1alpha1/` 的 `cluster_types.go`（`spec.displayName`、`spec.nodeAgents`）、`node_types.go`（`spec.displayName`）、`task_types.go`（`placement.perNode`、`status.instances`） | 改动：CRD 字段只能是官方结构体的字段（嵌入结构体会改变 controller-gen 和 applyconfiguration-gen 生成的 API）；新类型整块在同目录的 `astribot_*_types.go` |
| 命令 | `apps/rlark/cmd/{rlarkctl,rlarkadm,rlark-tools}/main.go` | 钩子：挂上我们的子命令；`rlark-tools ray-launcher --role` 的说明加了“空：不加入 Ray 集群” |
| Agent（`pkg/agent`） | `agent.go`、`config.go`、`cluster_agent.go`、`node_agent.go`、`tunnel.go`、`local_http.go` | 钩子：`localHTTPRouter` 挂 `astribotLocalRoutes`（`/api/astribot/services/<名字>/…` 转到节点上 `/run/rlark/services/<名字>.sock`，例如机器人电源服务的初始姿势，`astribot_node_services.go`）；`Config` 嵌入 `AstribotConfig`（`--health-probe-bind-address`、`--management-node-labels`、`--task-daemonset-push-controller-workers`，`setupFlags`/`apply`，`--metrics-bind-address` 默认 10381、`--containerd-socket` 默认空即取节点注解 `kubeadm.alpha.kubernetes.io/cri-socket`）；`Agent.ready` 的类型换成 `readiness`（`Store`/`Load` 保留官方调用，各部分都就绪才算就绪）；上游注册启动流程监督探针和指标服务；集群 agent 的管理面选项 `astribotManagementOptions`（只读自己的 `Cluster`、不起 manager 自己的指标服务）、`prepareAstribot` 在 Build 阶段安装站点控制器，运行时等待缓存同步后报告就绪；节点 agent 的 `runtimeSocket`、`waitForFirstSync`、`startAstribot`（`together`：任一部分停了都退出；占用目录上报）；隧道的 `announce`、`closeWhenChanged`、`changedOr`、`reconnectOnChange`（就绪变化就重连，Server 只给就绪的 agent 记心跳）。改动：`node_agent.go` 去掉官方的指标/pprof 服务（它和 Agent 在每种模式都起的指标服务占同一端口），所以官方的 `healthRouter` 没有调用方（见下节） |
| | `controllers/node/push.go`、`controllers/node/pull.go`、`controllers/addon/pull.go` | 钩子：`isSiteOwnedNodeLabel`/`isSiteOwnedNodeAnnotation`（机器人身份、批次、重启请求、加速卡、Station 名 `rlark.io/station`、`--management-node-labels`）；`pullManagementAnnotations`（管理面 Node 上节点自己要用的注解 `rlark.io/robot-restart`、`rlark.io/station-mark-set` 带到集群 Node，官方 pull 只带标签）；`beginNodeReport`/`patch`（按字段 merge patch，保留租户标签）；限定命名空间的 Task Pod Event 观察（挂载、挂接和调度失败触发重新观察）；Addon 从包渲染、漂移时重新应用、归属不符不接管、清理确认后才返回（`astribot_*.go`） |
| | `controllers/task/runtime/kubernetes/provider.go` | 钩子：`instanceNodes`（DaemonSet 的 Pod 所在节点是否就绪、是否在用）、`perNodeConditions`、`keepPlatformAdditions`（平台加的镜像和拉取凭据不是替换运行中 Pod 的理由）、`takeRollout`。改动：`podInstance` 多一个参数 `instanceNode`，条件要在它排序之前加进去 |
| | `controllers/task/workload/ray/kubernetes/adapter.go` | 钩子：`CarrierDaemonSet` 的 `daemonSetRollout`（紧跟载体解码，DaemonSet 载体的参数只在这里解码一次）、`BuildDaemonSet`、`observeDaemonSet`；`launcherArgs`（没有 Ray 角色的 Task 也经 launcher 只跑脚本）；`PendingReason`（挂载失败先于调度失败，带事件消息）。改动：Ray 角色允许为空、`gvkFor` 加 DaemonSet；官方的 `HasFailedScheduling`（`observe.go`）因此没有调用方。官方测试 `adapter_test.go` 里拒绝 DaemonSet 载体、期待消息 `FailedScheduling` 的两处随行为改了，表驱动测试加了 DaemonSet 的行 |
| | `controllers/task/{assembly_kubernetes.go,dispatch.go,execution_kubernetes.go,observation_events.go}`、`execution/host/{host.go,types.go}`、`network.go`、`supervision/tools.go` | 改动：按节点放置的 Task（DaemonSet 注册到上游观测器并按 Task 归属过滤，`status.instances` 通过共享状态意图上报、`role` 可省略）；两个平台容器名改用 `kubernetesobjects` 的常量 |
| Job（`pkg/controllermanager/job`） | `build.go`、`sync.go`、`controller/base.go` | 钩子：`resolvePerNodeNamespace`、`validatePerNodeTemplate`、`perNodeReplicas`；`setDeclaredSSHEnvironment`（模板声明了 SSH 环境就用它的端口，几个里取最后一个）；`clearRoleWithoutHead`/`dropRoleWithoutHead`（没有 head 的 Job 不组 Ray 集群）；按名字解析存储：`reconcileTaskInNamespace` 开头的 `resolveTemplateStorage`，`dispatchTasksWithTemplates` 里的 `storageWait`（等存储的 Task 跳过，记 `StorageAvailable`，返回 `ErrRequeueWaiting`），`reconcileWithStateMachine` 遇到 `ErrRequeueWaiting` 先保存状态；`ReconcileWith` 对 `ErrRequeueWaiting` 存状态后按 `WaitingRequeue` 重试 |
| Gateway（`pkg/gateway`） | `router.go`、`gateway.go`、`config.go`、`swagger_test.go`、`auth_middleware.go`、`api_reference.go`、`cert_handler.go`、`addon_handler.go`、`addon_catalog_handler.go` | 钩子：`registerAstribotRoutes`；`POST /certificates/revoke` 指到 `handleRevokeCertificateViaServer`；`astribotState`、`AstribotConfig`、`setupAstribot`/`initAstribot`/`startAstribot`；服务账号令牌；仓库浏览的接口说明；`handleSignAgentCert` 的结尾改调 `respondAgentCertStored`（Secret 存好再返回并带拉取凭据；集群开通也用 `storeOwnedAgentCertSecret`，官方的 `storeAgentCertSecret` 没有调用方）；`publishAddonPackageFor`、`deleteAddonPackages`；目录接口读 `addons.Catalog`（`pkg/addons/catalog.go` 保持官方原样，它的 `Registry` 只剩官方测试和比对测试在读，现网渲染走 `Item.Render`；版本化的包、目录文件夹、`requiredWith` 都在 `pkg/addons/astribot_*.go`）。`config.go` 里 `--jwt-token-ttl` 的说明改成“一次登录多长” |
| | `kube_handler.go` | 钩子：Node 的 PUT/PATCH 先过 `nodeUpdateAllowed`/`nodePatchAllowed`（机器人电源、认领和显示名只能经各自的接口改） |
| | `clusters_handler.go` | 钩子：`ClusterInfo` 嵌入 `AstribotClusterInfo`（`displayName`、`nodeAgents`），`setAstribotClusterInfo`（有显示名时 `name` 显示它）；显示名重复回 409 |
| | `imageregistry_handler.go` | 钩子：`withStationSecret`（凭据也发到 `station-system`）、`givenOr`。改动：更新请求的 `username` 改成可省略（省略即保留，和 `password` 一样），空值拒绝。官方测试 `imageregistry_handler_test.go` 里投递的对象数随行为从 1 改成 2；`station-system` 的副本、`/password` 接口、省略账号的更新测在 `astribot_imageregistry_handler_test.go`，吊销接口只给管理员测在 `astribot_cert_handler_test.go`（`auth_handler_test.go` 保持官方原样） |
| | `storage_handler.go` | 钩子：三个结构体各嵌入一个 `astribot…` 结构体，流程在要分叉的地方调 `astribot_storage_*.go` 的函数（`smb`、`hostpath`、S3 新字段、按档位的 StorageClass、改名），渲染改调 `buildAstribotStorageReplicationSecret`（官方的 `buildStorageClassReplicationSecret` 没有调用方），更新时官方的“不能改名”检查和访问密钥回填由 `prepareStorageUpdate` 一次读取代替 |
| | `systemconfig_handler.go`、`pkg/common/constants.go` | 改动：去掉系统配置里的 `deployment`（数据面部署 YAML 的默认值），它只给已删除的官方“创建集群”页用（见下节）；官方测试 `systemconfig_handler_test.go` 里相应的部分随之去掉 |
| Server（`pkg/server`） | `agent.go`、`peer_manager.go`、`handle_proxy.go`、`sign.go`、`ssh_server.go`、`server.go`、`https_server.go` | 钩子：`registerHTTPSHandlers` 挂 `nodeproxy/:target/:node/*path`（`handleNodeProxy`：只经这个节点自己的节点 agent 隧道到它的本地服务，`astribot_node_proxy.go`）；`grantOwnClusterReader`（每个集群 agent 只能读同名的 `Cluster`）、`keepLeading`（租约因管理面卡顿丢了就重新拿）、`invalidRevocation`。改动：agent 隧道说就绪才记心跳（`handle_proxy.go`）；证书吊销查询失败时拒绝而不是放行、只缓存“已吊销”、没有存储时吊销接口回 503、成功回 `{"persisted":true}`（`sign.go`、`ssh_server.go`；`rcStore` 换成接口以便测试） |
| rlarkadm（`pkg/rlarkadm`） | `component/component.go`、`types/types.go`、`deployer/kubernetes/{install.go,uninstall.go}`、`uninstall.go` | 钩子：`astribot_component.go` 的 `init` 改官方组件表：去掉节点 agent 的 DaemonSet（`rlark-agent-node`，节点 agent 改由 Cluster 声明）、集群 agent 用自己的端口和就绪探针、控制面组件用各自的 kcp 凭据（`kcpCredentialVolume`）而不是 kcp 管理员的 kubeconfig；`credentialPullSecrets`、`placeClusterAgent`（宿主网络、`Recreate`、优先控制面节点）；`KubernetesEnv` 嵌入 `AstribotKubernetesEnv`（`image-pull-credentials`）。改动：`ContainerdSocket` 的标签改成 `json:"-" yaml:"-"`，配置里写 `containerd-socket` 照旧在解析时被拒（字段保留只因官方节点 agent 组件的代码用它，见下节）；`issueKCPIdentities`、`deleteKCPIdentities`。改动：`Install` 拆出 `InstallTo`（Gateway 开通集群时凭内存里的凭据安装），其中 `putImagePullCredentials` 在 agent 之前放进 `rlark-system`；kcp 不再写 `rlark-kcp-kubeconfig` ConfigMap；卸载摘要随之改。保留官方节点 Agent 模板及其 RBAC 测试，站点安装由 Cluster 声明节点 Agent |
| 其他 Go | `pkg/distribution/{engine.go,store.go,types.go}` | 钩子：`replaceStorageClass`（改不了的 StorageClass 删了重建）、`namespaceReplaced`（命名空间重建后不认旧项）。改动：清单记声明内容的摘要而不是 resourceVersion（`ObservedDeclarationDigest`），没变就不写，避免每次保存都触发声明自己的 watch |
| | `pkg/rlarktools/raylauncher/launcher.go`、`pkg/network/nodeserver/metrics.go` | 钩子：`runWithoutRay`（无 head 的 Job 不等 Ray）；改动：角色允许为空；节点服务的指标注册到 controller-runtime 的 registry（Agent 的 `/metrics` 只出这一份） |
| | `apps/rlark/go.mod`、`go.sum` | 随上面的依赖 |
| 内嵌 Addon | `pkg/addons/catalog/csi-driver-rclone/` 的 `addon.yaml` 和三个清单 | `nodeUpdateStrategy`、`nodeImagePullPolicy`、`controllerNodeSelector`、`nodeTpsLimit` |
| 控制台（整页换成我们的） | `apps/rlark-ui/src/` 的 `App.tsx`、`admin/AdminApp.tsx`、`admin/AdminPage.tsx` | 钩子：只把存储、集群管理、集群列表三页和 `NodeResourceBrowser` 的 import 路径指到 `src/astribot/pages/`、`src/astribot/components/` 下的同名文件（存储的 SMB/主机目录/挂载档位、集群和节点的显示名、控制面和 Kubernetes 版本两列、节点的 WireGuard 地址和删除，见 [architecture.md](architecture.md)）；官方的那四个文件原样留着、不再挂载（见下节）。它们的样式在同目录的 `.css` 里（`ClusterManagement.css`、`Clusters.css`、`NodeResourceBrowser.css`、`nodeNetwork.css`、`controlPlaneBadge.css`）；我们别的组件的样式也从官方 `src/styles/` 挪到了 `src/astribot/components/` 下：`provision.css`（新建集群页和创建进度，原在 `admin/settings-and-certificates.css`；`.cert-form.new-cluster-form label`、`.cert-form label.provision-clear-gone` 盖过官方的 `.cert-form label`）、`RegistryImagePicker.css`（原在 `jobs/create.css`；`.image-picker .image-picker-*` 套在官方的 `.image-picker` 下，`.image-picker .image-picker-browser label` 盖过官方的 `.modal-body label`）、`RegistryPassword.css`（原在 `storage/index.css`，不盖官方规则）。官方 `src/styles/` 保持原样；组件引入的样式排在官方样式之前，要盖过官方值的规则多套一层类名。`data.ts` 的 `StorageClass` 是官方的，我们的字段在 `astribot/lib/storageClass.ts` 的 `StorageClass`（继承官方的）；存储页的文案是官方的加上 `astribot/i18n.ts` 的（`storageCopy`），`i18n.ts` 保持原样。`tests/astribot-pages.test.mjs` 把官方对这几页的测试指到我们的文件再跑一遍 |
| 控制台（官方文件里的钩子） | `apps/rlark-ui/` 下：`src/main.tsx`、`admin/{AdminApp,AdminPage,AdminDashboard}.tsx`、`constants.ts`、`utils/{route,crd,job,dag,nodeVisibility}.ts`、`pages/{Overview,Jobs,CreateJob,Workflows,ImageRegistries,SystemConfig}.tsx`、`components/{create,ResourcePlacementPicker}.tsx`、`backend.ts`、`data.ts`、`types.ts`、`mockBackend.ts`、`scripts/generate-crd-models.mjs`、`tsconfig.test.json`，及测试 `job-storage`、`job-clone-fidelity`、`clipboard`、`refresh-feedback` | 钩子：`src/main.tsx` 一行引入我们的全局样式 `src/astribot/styles.css`（`.real-name`，排在官方样式之后）；`admin/AdminApp.tsx` 挂 `AstribotAdminPages`（场站节点、加入节点）和 `NewClusterPage`（`constants.ts` 加“场站节点”导航，`utils/route.ts` 加 `stations`）；`admin/AdminPage.tsx`（显示名、改名后替换列表里的节点、节点表 `removable`）、`admin/AdminDashboard.tsx`、`pages/Overview.tsx`、`components/create.tsx`、`components/ResourcePlacementPicker.tsx`（`NodeName`）、`pages/Jobs.tsx`（`RealName`、`clusterLabel`、`clusterIDsLabel`、`workerNodeLabel`，挂载档位一栏 `storageMountLabel`）：集群和节点显示名；`pages/CreateJob.tsx`、`pages/Workflows.tsx`：选镜像 `RegistryImagePicker`、存储挂载的档位和只读 `StorageMountOptions`（代替官方的 PVC 大小）、按名字引用存储、集群只按 ID 选、取不到存储时的演示存储取我们的；`pages/ImageRegistries.tsx`：`RegistryPassword`、`RegistryBrowser`、`registryClusterOption`；`utils/crd.ts`、`utils/job.ts` 调 `astribot/lib/storageVolume.ts`（卷是 CSI `storage.rlark.io`，按名字引用存储）；`mockBackend.ts` 先问 `astribot/mockBackend.ts` 的 `astribotMockRoute`。改动：“创建集群”改叫“新建集群”（`AdminApp.tsx`、`constants.ts`）；`backend.ts` 的 `imageRegistriesApi.list` 多收一个 `signal`（`RegistryImagePicker` 关掉浏览时取消读仓库列表）；`pages/SystemConfig.tsx` 去掉“部署配置默认值”一栏（随删掉的创建集群页，见下面“有意删除”）；Job 的存储挂载：`data.ts`、`types.ts` 的挂载项由 `pvcSizeGb` 换成 `profile`、`readOnly`，`utils/dag.ts` 跟着换默认值；`types.ts` 给 Node 加 `spec.displayName`；`utils/nodeVisibility.ts` 把控制面判断抽成 `isControlPlaneLabels`，带 `rlark.io/enabled=true` 的控制面节点也算业务节点；`mockBackend.ts` 的集群加显示名，保存的存储加我们的字段，演示存储取我们的；`scripts/generate-crd-models.mjs` 加 `ClusterProvision`、`NodeJoin`；`tsconfig.test.json` 编译我们的模块。官方测试 `job-storage`、`job-clone-fidelity` 随存储挂载改，`clipboard`、`refresh-feedback` 去掉删掉的创建集群页和部署配置一栏 |
| 可执行示例 | `apps/rlark/docs/examples/` 的 `deploy-data-plane.yaml`（去掉 `containerd-socket`）、`quickstart-cp.sh`、`update-kcp-crds.sh`（读 kcp Pod 本次启动写的 `/.kcp/admin.kubeconfig`） | 随行为改 |

## 恢复成官方原样、但没有调用方的代码

这些官方代码原样留着（合入官方新版本时少冲突），现网不走它们：

| 代码 | 为什么没有调用方 |
|---|---|
| `pkg/agent/local_http.go` 的 `healthRouter` | 节点 agent 不再起官方的指标/pprof 服务；探针和指标由 Agent 在每种模式下分开起（`astribot_local_http.go` 的 `probeRouter`、`metricsRouter`） |
| `pkg/rlarkadm/component/component.go` 的 `rlark-agent-node` 组件（及 `DaemonSet` 里给它的节点选择器）、`types.KubernetesEnv.ContainerdSocket` | 组件在 `astribot_component.go` 的 `init` 里从组件表去掉；`containerd-socket` 只给它用，不再是配置的键（标签 `"-"`），字段恒为空 |
| `pkg/rlarkadm/component/component.go` 的 `kubeconfigVolume`、`constants.KCPKubeconfigPath` | 仍被调用，但 `init` 把 Gateway、controller-manager、Server 的这个卷和 `--kubeconfig` 参数换成各自的 kcp 凭据 |
| `pkg/gateway/cert_handler.go` 的 `storeAgentCertSecret` 和 `handleRevokeCertificate`（路由指到 Server 的吊销接口 `handleRevokeCertificateViaServer`）、`storage_handler.go` 的 `buildStorageClassReplicationSecret`、`pkg/addons/catalog.go` 的 `Registry` | 见上表 Gateway 行 |
| `pkg/agent/controllers/task/workload/ray/kubernetes/observe.go` 的 `HasFailedScheduling` | `Observe` 改用 `PendingReason` |
| `apps/rlark-ui/src/pages/` 的 `Storage.tsx`、`ClusterManagement.tsx`、`Clusters.tsx`，`src/components/NodeResourceBrowser.tsx` | 换成 `src/astribot/` 下的同名文件（上表“控制台（整页换成我们的）”行）；`tests/astribot-pages.test.mjs` 把官方对它们的测试指到我们的文件再跑 |
| `apps/rlark-ui/src/backend.ts` 的 `certificatesApi`、`DeploymentConfig`，`types.ts` 的 `SignAgentCertResponse`、`AgentCertListItem`，`mockBackend.ts` 的 `GET /api/v1/certificates/agent` | 原来给删掉的创建集群页和部署配置一栏用 |
| `apps/rlark-ui/src/data.ts` 的 `storageClasses` | 演示存储用 `astribot/mockBackend.ts` 的 |
| `apps/rlark-ui/src/pages/CreateJob.tsx` 的 `formatImageUsage` | 选镜像换成了 `RegistryImagePicker` |
| `apps/rlark-ui/src/styles/` 里创建集群页的样式（`.cert-result*`、`.cert-yaml-*`、`.cluster-enrollment-notes`、`.cluster-enrollment-result`、`.cluster-yaml-card`、`.signed-cluster-*`、`.cert-list-chevron`） | 创建集群页删掉了，不再匹配任何元素 |

## 有意删除的官方文件

| 文件 | 为什么 |
|---|---|
| `apps/rlark-ui/src/admin/CreateCluster.tsx` | 官方“创建集群”页生成一份部署 YAML 让人手工执行；我们新建集群走 Gateway 的 ClusterProvision（控制台“新建集群”页），这个页面不恢复 |
| `apps/rlark-ui/src/utils/deployYaml.ts`、`apps/rlark-ui/tests/deploy-yaml.test.mjs` | 只给上面那个页面用 |

合入官方新版本时，官方对这三个文件的改动不合进来（`git apply -3` 会报它们不存在，直接丢掉这几处）。

Gateway 用我们的镜像时必须带 Station API 和仓库浏览的参数（[architecture.md](architecture.md)）。官方 rlarkadm 部署的 Gateway 和 `apps/rlark/docs/examples/docker-compose.yml` 不带，起不来；只有 `astribot/release/control-plane/rlark.compose.yaml` 带。

## 合入官方新版本

Astribot 产品分支为 `astribot/main`，官方镜像分支的目标是 `main`，切换由团队另行确认。本次合并从导入基线 `9ad4a21` 接入官方历史，然后合入 `d06745e`。接入历史的提交保留了原 Astribot 文件树。后续从产品分支创建临时分支，用正常的三方合并纳入上游。共享开发分支保留自己的历史，个人开发分支可 rebase 到产品分支。

```bash
git remote add upstream https://github.com/RLinf/RLark.git    # 首次配置
git fetch upstream main
git switch -c codex/rlark-upstream-sync origin/astribot/main
OLD=$(git merge-base HEAD upstream/main)
NEW=$(git rev-parse upstream/main)
git merge --no-ff --no-commit "$NEW"
```

处理冲突后从源定义重新生成产物并验证。由评审后的 `astribot/main` 提交选择 Astribot 发布来源，见 [发布流程](release.md)。

## 合入后的清单

1. 冲突只应出现在上表的文件里。生成物不手合：`make generate`。
2. 官方的 Dockerfile 变了就照样改我们的：`git diff $OLD $NEW -- apps/rlark/Dockerfile apps/rlark-ui/Dockerfile apps/rlark-ui/nginx.conf`，改到 `astribot/release/rlark.Dockerfile`、`rlark-ui.Dockerfile`、`rlark-ui.nginx.conf`。发布只用这三份，官方新加的程序不照搬就进不了镜像。
3. 跑 [README](../README.md) 的全部测试。`apps/rlark/pkg/gateway/swagger_test.go` 会指出官方新加而没写进文档的路由；我们的路由写进 `astribot/docs/swagger.yaml`。
4. 官方改了上表中我们也改的行为，就在本文件和 [architecture.md](architecture.md) 里跟着改。
5. 官方改了 `pkg/addons/catalog.go` 的 `embeddedAddon.Render`（参数默认值、模板数据、模板函数），就照样改 `pkg/addons/astribot_catalog.go` 的 `Item.Render`：现网渲染的每个 Addon 走后者。官方的 `Render` 只按目录读编进程序的文件，两者共用不了代码；`TestItemsRenderAsTheOfficialRegistryDoes` 逐个内置项比对两边的输出，不一致就失败。
6. 官方改了 `apps/rlark-ui/src/pages/` 的 `Storage.tsx`、`ClusterManagement.tsx`、`Clusters.tsx` 或 `src/components/NodeResourceBrowser.tsx`（`git diff $OLD $NEW -- <这四个文件>`），这些改动合进去不会冲突，但现网不用这四个文件：把同样的改动做到 `src/astribot/` 下的同名文件里。官方为它们改的测试由 `tests/astribot-pages.test.mjs` 对着我们的文件再跑，漏改会失败。官方改了 `pages/CreateJob.tsx` 里旧的选镜像框或 `i18n.ts` 的 `storageClass`，同样看要不要改到 `RegistryImagePicker`、`astribot/i18n.ts`。官方改了被我们的样式盖过的规则（`.cert-form label`、`.modal-body label`、`.image-picker`，以及上面四页的样式），对照上面“控制台（整页换成我们的）”一行列的 `.css` 看我们的规则还对不对。
7. 更新本文件和 [README](../README.md) 里的官方基线。
8. 在隔离环境验证整套构建。发布按 [release.md](release.md) 单独执行。
