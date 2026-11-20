# 构建和上线

发布在办公室管理机（10.11.14.128，ssh 别名 `cailiang_rlark`，用 sudo）上构建和上线，不经 CI。

## 文件

| 文件 | 是什么 |
|---|---|
| `astribot/release/build.sh` | 在工作站执行：把提交的树（`git archive`）送到管理机 `/var/lib/rlark/release/builds/<名字>/`，由树里的 `run.sh` 构建；构建完把发布文件拷回 `releases/` |
| `astribot/release/run.sh` | 在管理机上构建（见下） |
| `astribot/release/images` | 要构建的镜像：仓库、Dockerfile、它是发布里的哪些组件、BuildKit 参数 |
| `astribot/release/mirror` | 构建要用的公网底座镜像，按摘要拷进办公室仓库的 `mirror/…` |
| `astribot/release/pinned.yaml` | 不由本树构建的镜像（按摘要）和各集群都一样的 Addon 取值；`installer` 列出打进安装包的镜像 |
| `astribot/release/releases/<名字>.yaml` | 一个发布：`source`（RLark 和 Fleet 的提交）、`images`（组件 → 摘要）、`addons`（版本、取值、`after` 顺序） |
| `astribot/release/environments/live.yaml` | 现网：用哪个发布、管理面身份、控制面 compose 的位置和取值、Station 所在集群、安装器底座、两个仓库、运行记录目录 |
| `apps/rlark/astribot/release/stages.yaml` | 上线的阶段、顺序、检查 |
| `astribot/release/control-plane/rlark.compose.yaml` | 控制面四个服务 |
| `astribot/release/member-rbac.yaml` | 每个成员集群里发布和只读账号的 RBAC |

## 整套构建

```bash
astribot/release/build.sh <RLark 提交> <Fleet 提交（40 位）>   # 发布 ci-<r7>-<f7>
astribot/release/build.sh base <RLark 提交>                      # 安装器底座（换 Kubernetes 版本时）
```

`run.sh release` 依次：
1. 缺的底座镜像拷进办公室仓库 `mirror/…`；
2. 跑测试（`Dockerfile.test` 和 `s1u-robot` 的 test target），按 `images` 构建并推进 `127.0.0.1:5443`；
3. 从 rlark-server 镜像里取出这次的 `rlarkctl`；
4. 安装包和升级包（`installer-live`、`rlark/node-bootstrap`），底座是 `live.yaml` 的 `installer.base`；
5. Fleet 的镜像（`fleet-robot`、`fleet-run-gpu`）从金山云仓库按摘要拷进办公室仓库；Fleet 提交和上一发布相同就沿用；
6. `rlarkctl release candidate` 写发布文件；
7. 这次构建的镜像按摘要拷进金山云仓库（`rlarkctl release copy --to source`）。

- 同一时间只有一个构建（锁 `/var/lib/rlark/release/builds/lock`）。日志 `<构建目录>/build.log`，每步还有自己的 `*.log`。
- 构建在 BuildKit 容器里，默认 6 GB 内存、4 核、并行 2；内存不够时等最多 120 分钟。`BUILD_MEMORY`、`BUILD_RESERVE`、`BUILD_CPUS`、`BUILD_PARALLELISM`、`BUILD_WAIT` 可改。
- 镜像里的时间戳都是 0：内容不变，镜像摘要就不变。
- 构建时间长，在 tmux 里跑。

## 快速上线（只换改了的镜像）

改动只涉及一两个程序时，不跑整套构建。改动碰到 `astribot/station/bootstrap/` 或 `pinned.yaml` 的 `installer` 时不适用：`installer-live` 只有整套构建会重做，快速上线沿用上一个发布的安装包。

前两步在工作站，其余在管理机上以 root（发布身份的 kubeconfig 在 `/home/cailiang/…` 下，见 `live.yaml`）：

1. 跑全量测试（[README](../README.md) 的命令）。
2. 只构建改了源码的镜像，推进管理机的 `127.0.0.1:5443`，记下摘要。一个程序的改动可以把新编译的二进制叠在现网镜像上（`FROM <现网镜像>@sha256:…` 加一行 `COPY`）。注意 `rlark/agent` 一个镜像是六个组件（见 `images`），只给改了的组件换摘要。
3. 树送到管理机并解开（发布文件的 `source.rlark` 必须是这个提交，`apply` 会核对）：
   ```bash
   W=/var/lib/rlark/release/builds/<名字>
   ssh cailiang_rlark sudo mkdir -p $W/rlark
   git archive <提交> | ssh cailiang_rlark sudo tee $W/rlark.tar >/dev/null
   ssh cailiang_rlark sudo tar -C $W/rlark -xf $W/rlark.tar
   ```
4. 按这个提交编译 `rlarkctl`（`cd apps/rlark && go build -o rlarkctl ./cmd/rlarkctl`），拷到 `$W/rlarkctl`。Addon 目录编在 `rlarkctl` 里，用旧的会发布旧的 Addon 版本或报某版本没发布过。
5. 在管理机 `$W/rlark/astribot/release` 下，从上一个发布生成新的：
   ```bash
   $W/rlarkctl release candidate --base releases/<上一个>.yaml --tree $W/rlark.tar --name ci-<r7>-<f7> \
     --fleet <Fleet 提交> --image <组件>=registry.rlark.internal:5443/<仓库>@sha256:<摘要> ...
   $W/rlarkctl release copy -e environments/live.yaml --to source <新镜像>...
   ```
   没给 `--image` 的组件（包括 `installer-live`）沿用上一个发布的镜像；Addon 版本取树里的。
6. 同一目录照常 `apply`（下一节），发布文件拷回工作站提交。

## 上线

在管理机上，以 root，在 `astribot/release/` 下：

```bash
rlarkctl release apply -e environments/live.yaml -r releases/<名字>.yaml --tree <rlark.tar> --dry-run
rlarkctl release apply -e environments/live.yaml -r releases/<名字>.yaml --tree <rlark.tar>
```

- `--dry-run` 只打印每个阶段会改什么。
- 不带 `-r` 用 `live.yaml` 的 `release:`。`--tree` 是发布那个提交的 `git archive`（CRD、RBAC、清单从这里取）。`--timeout` 是每阶段的等待（默认 10 分钟，乘 `timeoutFactor`）。
- 运行前把现状记在 `/var/lib/rlark/release/runs/<时间>-before.json`，日志在同目录 `<时间>-<发布>.log`。同时只有一个运行（kcp 里的 Lease）。
- 每个阶段改完就读回并跑检查，直到成立或超时；一个阶段不成立就停下，打印回退命令。原地再执行同一条命令会从停下处接着做（已经符合的阶段跳过）。
- 成功后写发布记录 `rlark-system/rlark-release-current`。
- 上线后提交 `releases/<名字>.yaml`，并把 `live.yaml` 的 `release:` 指向它。

### 13 个阶段

| 阶段 | 改什么 | 检查 |
|---|---|---|
| `images` | 发布里办公室仓库缺的镜像，从金山云仓库按摘要拷进来 | 逐个读回 |
| `prewarm` | 控制面的镜像预先拉到管理机 | `nodes-unpressured` |
| `crds-rbac` | `rlinf.io`、`station.rlark.io` 的 CRD；station-api / controller 的管理面 RBAC；各成员集群的 Station RBAC | — |
| `gateway` | 控制面 gateway 容器（和 compose、env 文件） | `clusters-online`、`addons-ready` |
| `station-api` | Deployment station-api；安装器放到 Station API 的机器上 | `enrollments-enrolled`、`observations-fresh` |
| `controller` | Deployment station-controller（先等所有网关认得新文档级别） | `documents-unchanged`、`enrollments-enrolled`、`network-connected` |
| `cluster-agents` | 每个集群的 agent，一个集群一个集群来 | `clusters-online`、`addons-ready`、`nodes-ready`、`nodes-unpressured` |
| `node-agents` | 每个集群的节点 agent（Cluster `spec.nodeAgents.image`） | `clusters-online`、`nodes-ready`、`nodes-unpressured` |
| `station-agents` | 各成员集群已有的 Station DaemonSet 改成这棵树的 `station-agent.yaml` 写的样子（整个 `spec`，selector 除外），镜像和拉取账号在同一次写入里换成发布的，模板和它启动的二进制始终出自同一个提交。计划里逐个字段列出差异（和集群 API server 对这份清单 dry-run 的结果比，补的默认值不算差异）。DaemonSet 是 OnDelete，由集群 agent 按 `rlark.io/rollout` 换 Pod，这个阶段等它们换完。不带树的运行（`--back`、`--to`）只改镜像和拉取账号，模板其余部分不退回 | `observations-fresh`、`enrollments-enrolled`、`links-connected`、`documents-unchanged`、`network-connected`、`nodes-unpressured` |
| `server` | rlark-server、rlark-controller-manager 容器 | `clusters-online`、`nodes-ready` |
| `console` | rlark-ui 容器 | — |
| `addons` | 版本或取值和发布不同的 Addon，按 `after` 顺序 | `clusters-online`、`nodes-ready` |

- 一个阶段只允许它声明的 Pod（`recreates`）被重建；旁边有别的 Pod 新起，阶段会保守地停下。看清是无关的 Pod 后，再执行同一条命令。
- 换控制面的一个服务时，依赖它的服务也会重启（控制台的 nginx 记着 gateway 容器的地址）。
- 发布带的 Addon 跟着发布走；机器人上发布之外的取值（`powerOff`、`reboot` 等）保持站点设的。

## 上线后看什么

- 日志最后一行 `release <名字>: every stage done`。
- 控制面：`docker ps` 看四个容器的镜像；打开 `http://10.11.14.128/` 能登录。
- 各集群：`rlarkctl release kubectl -e environments/live.yaml --cluster <集群> -- get pods -A`，看 `rlark-system`、`station-system` 没有异常 Pod；节点都 Ready。这条命令用每个集群里的只读账号 `rlark-system/rlark-reader`（看得到工作负载、日志、事件，读不了 Secret），机器上不留管理员 kubeconfig。
- 机器人：`s1u-robot` Pod 全就绪，`rlark.io/power-state`、`rlark.io/self-check-state` 正常。

## 回退

```bash
rlarkctl release apply -e environments/live.yaml --to /var/lib/rlark/release/runs/<时间>-before.json --back
```

`--back` 倒序走标了 `back` 的阶段（先 server，再各 agent）。CRD 和 Station 的放置不退；一个发布删掉的 CRD 字段或枚举值，旧版本不再接受，有这类对象在用时不要退过这个发布。

## 不在发布构建里的镜像

- `csi-driver-rclone` 的 KS3 版：
  ```bash
  astribot/csi-rclone-ks3/build.sh /abs/new/dir   # 单测 + 静态编译出 rcloneplugin
  docker build -f astribot/csi-rclone-ks3/Dockerfile --build-arg BASE=<仓库>/csi-rclone/csi-driver-rclone:v0.2.0 \
    -t <仓库>/csi-rclone/csi-driver-rclone:v0.2.0-ks3.3 /abs/new/dir
  ```
  新摘要写进 `pinned.yaml` 的 `csi-rclone`，在 `addons` 阶段上线。替换插件 Pod 会断开该节点所有 rclone 卷。Addon 设了 `nodeUpdateStrategy=OnDelete` 的集群，确认没有 Pod 在用卷后逐个节点替换；默认的 `RollingUpdate` 下，新的 `rcloneImage` 会自己滚完每个节点、断开所有挂载，所以发布新摘要前先看各集群的取值。
- `station-forwarder`：`astribot/station-forwarder/Dockerfile`。
