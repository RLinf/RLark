# 跟官方 RLark

- 官方仓库：`https://github.com/RLinf/RLark`。
- 基线：`9ad4a21`（官方 release-261104）。本仓库不带官方历史，先取一次官方仓库：`git remote add upstream https://github.com/RLinf/RLark.git && git fetch upstream`；之后 `git cat-file -e 9ad4a21:<路径>` 成功的是官方文件。
- 官方文档（`apps/rlark/docs/`、`mkdocs.yml`、`apps/rlark/pkg/addons/catalog/README.md`）保持原样；我们的接口写在 [swagger.yaml](swagger.yaml)。

## 改了哪些官方文件

`git diff --name-status 9ad4a21 HEAD | grep -v '^A'` 列出全部。按原因分：

| 类 | 文件 | 为什么 |
|---|---|---|
| 生成物 | `api/config/crd/bases/rlinf.io_*.yaml`、`api/kubeclients/**`、`api/rlark.io/v1alpha1/zz_generated.deepcopy.go`、`apps/rlark-ui/src/generated/**`、`apps/rlark/docs/reference/crd.md` | 我们给 CRD 加了字段；`make generate` 重新生成 |
| API 字段 | `api/rlark.io/v1alpha1/` 的 `cluster_types.go`（`spec.displayName`、`spec.nodeAgents`）、`node_types.go`（`spec.displayName`）、`task_types.go`（`placement.perNode`、`status.instances`） | 新类型整块在同目录的 `astribot_*_types.go` |
| 钩子 | `apps/rlark/cmd/{rlarkctl,rlarkadm,rlark-tools}/main.go`；`pkg/gateway/` 的 `router.go`（`registerAstribotRoutes`；`POST /certificates/revoke` 指到 `astribot_cert_handler.go` 的 `handleRevokeCertificateViaServer`）、`cert_handler.go`（`handleSignAgentCert` 的结尾改调 `respondAgentCertStored`）、`gateway.go`、`config.go`（`astribotState`、`AstribotConfig`、`setupAstribot`/`initAstribot`/`startAstribot`）、`swagger_test.go`；`pkg/agent/cluster_agent.go`（`setupAstribotControllers`）；`apps/rlark-ui/src/admin/AdminApp.tsx` | 把我们的命令、路由、控制器、页面挂进去 |
| Agent（`pkg/agent`） | 节点 agent 由 Cluster 声明、以集群 agent 为模板；RBAC 加 patch Node（写 `rlark.io/in-use`）；`--containerd-socket` 默认取节点注解 `kubeadm.alpha.kubernetes.io/cri-socket`；端口 10381/10382；集群 agent 宿主网络、`Recreate`；PriorityClass `rlark-node-resident`；按节点放置的 Task（新参数 `--task-daemonset-push-controller-workers`）；Addon 从包渲染、漂移时重新应用、归属不符不接管 | 见 [architecture.md](architecture.md) |
| Job（`pkg/controllermanager/job`） | 按名字解析存储（`StorageAvailable`）；没有 head 的 Job 不组 Ray 集群 | 同上 |
| Gateway（`pkg/gateway`） | 存储（`smb`、`hostpath`、S3 新字段、按档位的 StorageClass、改名）：`storage_handler.go` 里只有钩子，三个结构体各嵌入一个 `astribot…` 结构体，流程在要分叉的地方调 `astribot_storage_*.go` 的函数，渲染改调 `buildAstribotStorageReplicationSecret`（官方的 `buildStorageClassReplicationSecret` 留着但没有调用方），更新时官方的“不能改名”检查和访问密钥回填由 `prepareStorageUpdate` 一次读取代替；Addon 的包和 `requiredWith`：`addon_handler.go` 三处钩子（`publishAddonPackageFor`、`deleteAddonPackages`）；镜像仓库（不回密码、浏览）；集群和节点显示名；Agent 证书签发等 Secret 存好再返回并带拉取凭据（见上一行的 `cert_handler.go`；集群开通也用 `storeOwnedAgentCertSecret`，官方的 `storeAgentCertSecret` 留着但没有调用方） | 同上 |
| Server（`pkg/server`） | `POST /api/v1/certificates/revoke` 实现；每个集群 agent 只能读同名的 `Cluster`；agent 隧道就绪才记心跳 | 安全和发布顺序 |
| rlarkadm（`pkg/rlarkadm`） | 不再建节点 agent 的 DaemonSet；去掉 `containerd-socket`；`image-pull-credentials` 在 agent 之前放进 `rlark-system` | 节点 agent 改由 Cluster 声明 |
| 其他 Go | `pkg/distribution`、`pkg/gateway/addon_catalog_handler.go`（目录接口读 `addons.Catalog`：`pkg/addons/catalog.go` 保持官方原样，它的 `Registry` 只剩官方测试和比对测试在读，现网渲染走 `Item.Render`；版本化的包、目录文件夹、`requiredWith` 都在 `pkg/addons/astribot_*.go`）、`pkg/rlarktools/raylauncher`（无 head 的 Job 不等 Ray）、`pkg/network/nodeserver`、`pkg/common`、`apps/rlark/go.mod`、`go.sum` | 随上面的行为 |
| 内嵌 Addon | `pkg/addons/catalog/csi-driver-rclone/` 的 `addon.yaml` 和三个清单 | `nodeUpdateStrategy`、`nodeImagePullPolicy`、`controllerNodeSelector`、`nodeTpsLimit` |
| 控制台 | 官方页面里的显示名、按名字引用存储、SMB 和挂载档位、Job 编辑器选镜像、仓库口令、节点表的 WireGuard 地址，及相应的 `i18n.ts`、`mockBackend.ts`、样式和测试 | 见 [architecture.md](architecture.md) |
| 可执行示例 | `apps/rlark/docs/examples/` 的 `deploy-data-plane.yaml`（去掉 `containerd-socket`）、`quickstart-cp.sh`、`update-kcp-crds.sh`（读 kcp Pod 本次启动写的 `/.kcp/admin.kubeconfig`） | 随行为改 |

## 有意删除的官方文件

| 文件 | 为什么 |
|---|---|
| `apps/rlark-ui/src/admin/CreateCluster.tsx` | 官方“创建集群”页生成一份部署 YAML 让人手工执行；我们新建集群走 Gateway 的 ClusterProvision（控制台“新建集群”页），这个页面不恢复 |
| `apps/rlark-ui/src/utils/deployYaml.ts`、`apps/rlark-ui/tests/deploy-yaml.test.mjs` | 只给上面那个页面用 |

合入官方新版本时，官方对这三个文件的改动不合进来（`git apply -3` 会报它们不存在，直接丢掉这几处）。

Gateway 用我们的镜像时必须带 Station API 和仓库浏览的参数（[architecture.md](architecture.md)）。官方 rlarkadm 部署的 Gateway 和 `apps/rlark/docs/examples/docker-compose.yml` 不带，起不来；只有 `astribot/release/control-plane/rlark.compose.yaml` 带。

## 合入官方新版本

不 merge 官方的历史：把“旧基线 → 新版本”的差异三方应用到我们的树上。

```bash
git remote add upstream https://github.com/RLinf/RLark.git    # 一次
git fetch upstream
OLD=9ad4a21 NEW=<官方新提交>
git switch -c merge-upstream-${NEW:0:7}
# 预先看会冲突的文件：官方改了、我们也改了的
comm -12 <(git diff --name-only $OLD $NEW | sort) <(git diff --name-only --diff-filter=MD $OLD HEAD | sort)
git diff --binary $OLD $NEW | git apply -3
git status    # 冲突的文件带冲突标记，逐个解决后 git add
```

`git apply -3` 用 `$OLD` 的文件做共同祖先，所以只要本仓库有 `$OLD` 的对象就行，和两边的历史无关。

## 合入后的清单

1. 冲突只应出现在上表的文件里。生成物不手合：`make generate`。
2. 官方的 Dockerfile 变了就照样改我们的：`git diff $OLD $NEW -- apps/rlark/Dockerfile apps/rlark-ui/Dockerfile apps/rlark-ui/nginx.conf`，改到 `astribot/release/rlark.Dockerfile`、`rlark-ui.Dockerfile`、`rlark-ui.nginx.conf`。发布只用这三份，官方新加的程序不照搬就进不了镜像。
3. 跑 [README](../README.md) 的全部测试。`apps/rlark/pkg/gateway/swagger_test.go` 会指出官方新加而没写进文档的路由；我们的路由写进 `astribot/docs/swagger.yaml`。
4. 官方改了上表中我们也改的行为，就在本文件和 [architecture.md](architecture.md) 里跟着改。
5. 官方改了 `pkg/addons/catalog.go` 的 `embeddedAddon.Render`（参数默认值、模板数据、模板函数），就照样改 `pkg/addons/astribot_catalog.go` 的 `Item.Render`：现网渲染的每个 Addon 走后者。官方的 `Render` 只按目录读编进程序的文件，两者共用不了代码；`TestItemsRenderAsTheOfficialRegistryDoes` 逐个内置项比对两边的输出，不一致就失败。
6. 把本文件和 [README](../README.md) 里的基线 `9ad4a21` 换成 `$NEW`。
7. 照 [release.md](release.md) 整套构建一次再上线（官方的改动可能涉及任何镜像）。
