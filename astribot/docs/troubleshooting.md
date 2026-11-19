# 排障

## 管理机

办公室管理机 10.11.14.128（`ssh cailiang_rlark`，用 sudo）。这里跑控制面和办公室仓库，它也是办公室集群 `rlark-local-dev-gpu`（Station API 所在）的节点；构建和上线都在这里。

| 东西 | 路径 |
|---|---|
| 控制面 compose 和 env | `/var/lib/rlark/rlark.compose.yaml`、`/var/lib/rlark/control-plane.env`（发布写，不手改） |
| 组件身份 | `/home/cailiang/.local/share/rlark-runtime/identity/<组件>/kubeconfig` |
| 发布用的身份 | `.../identity/rlark-release/kubeconfig`（`live.yaml` 的 `management.kubeconfig`） |
| kcp 管理员 | `/home/cailiang/.local/share/rlark-runtime/kcp-admin.kubeconfig`（运维脚本默认的 `KCP`） |
| Addon 目录 | `/var/lib/rlark/addons` |
| 构建 | `/var/lib/rlark/release/builds/<名字>/`（`build.log`、各步 `*.log`、`rlark.tar`、`rlarkctl`） |
| 上线记录 | `/var/lib/rlark/release/runs/<时间>-<发布>.log`、`<时间>-before.json` |
| 安装器 | `/var/lib/rlark/station-bootstrap`（Station API 发给入网的机器） |
| 办公室仓库的卷 | `/var/lib/rlark/site-registry`（`findmnt` 应是 loop、ext4） |

不在文档里写任何口令和令牌；凭据在 RLark 的对象里，按名字引用。

## 日志在哪

| 看什么 | 怎么看 |
|---|---|
| 控制面 | 管理机上 `docker logs --since 1h rlark-gateway`（`rlark-server`、`rlark-controller-manager`、`rlark-ui` 同理） |
| Station API、controller | `rlarkctl release kubectl -e environments/live.yaml --cluster rlark-local-dev-gpu -- -n station-system logs deploy/station-api`（`deploy/station-controller`） |
| 集群 agent、节点 agent | `... --cluster <集群> -- -n rlark-system logs <Pod>` |
| 某台节点的 Station | `... -n station-system logs <station-agent-host-…> -c station-agent`（或 `-c gateway-program`） |
| 节点上的隧道 | 机器上 `journalctl -u rlark-station-gateway`；`station-gateway --config /etc/rlark/station/bootstrap.json --status`（接口、对端、最近握手） |
| 入网 | 见 [install-join.md](install-join.md) 的“入网排障” |
| 机器人 | `... --cluster rlark-robot-s1u-01 -- -n rlark-system logs <s1u-robot-…> -c power`（`tuner`、`orin`、`device`、`station`）；节点标签和注解（[architecture.md](architecture.md) 的 s1u-robot 接口表） |
| Addon | kcp 里 Addon 对象的 status；集群里的 `addon-rendered-<名字>` |
| 审计 | Gateway 日志里 `audit` 的行（占用、电源、入网、仓库口令） |

## 看成员集群

只读：`rlarkctl release kubectl -e environments/live.yaml --cluster <集群> -- <kubectl 参数>`（在管理机的 `astribot/release/` 下）。用集群里的只读账号，读不了 Secret。

要在集群里做事，用 RLark 的对象（Job、Addon、Cluster 的声明），不直接 `kubectl apply`。

## 已知故障

| 现象 | 原因 | 修法 |
|---|---|---|
| 管理机节点 `DiskPressure`，kubelet 驱逐 Pod、回收没在用的镜像；上线停在 `nodes-unpressured` | 管理机根盘被构建和镜像占满（BuildKit 缓存 `/var/lib/rlark/release/buildkit`、旧构建目录、docker 镜像） | 删旧的 `/var/lib/rlark/release/builds/<名字>`、BuildKit 缓存、不用的 docker 镜像，让盘回到 85% 以下；手工导入的镜像被回收的要重新导入；然后再执行同一条 `apply` |
| Go 程序在某些机器上 OOMKilled，内存是平时的几倍 | 机器的透明大页是 `always`（百度的 GPU 镜像），Go 碰到的每块内存都成了 2 MB 页 | `station-gateway` 启动时为自己关掉透明大页（`thp.go`）。别的 Go 程序遇到同样的事照此处理，不调大内存上限 |
| 只换了 gateway 之后控制台 `/api` 全是 502 | 控制台的 nginx 启动时解析了 gateway 容器的地址 | 发布换一个服务时会重启依赖它的服务；手工换的要 `docker restart rlark-ui` |
| 机器人 Pod 里解析不到刚加进 SiteNetwork 的名字 | 容器的 `/etc/hosts` 是它启动时宿主机那份的拷贝 | 先确认名字已在每台机器人宿主机的 `/etc/hosts`，再重建用它的 Pod（或随 Addon 升级重建） |
| 某集群没有节点 agent，节点显示不在线 | 节点 agent 没在 Cluster 上声明，或节点没有 `rlark.io/enabled=true` | `PUT /api/v1/clusters/{id}/node-agents`；给节点打标签。不手建 DaemonSet |
| Addon `Failed`：`version … has not been published to this cluster` | 集群管理命名空间里没有 `addon-package-<名字>-<版本>` | 经 Gateway 装或更新这个 Addon（会先发布包）；上线时用和发布同一提交编译的 `rlarkctl` |
| Gateway 回 409，说 Addon 版本内容不同 | 改了 Addon 目录的文件没升版本 | 升 `addon.yaml` 的 `version` |
| 上线停在某阶段，说有别的 Pod 新起 | 阶段只允许它声明的 Pod 重建；旁边的任务或滚动新起了 Pod | 确认是无关的 Pod，再执行同一条命令 |
| 上线停在 `controller` 前，`gateways-understand` 不成立 | 有网关还没报到新文档级别（没换上新镜像、节点离线） | 看那台的 `station-agent-host`；离线的节点上线后会换 |
| 节点 `x509: certificate signed by unknown authority` 拉镜像 | 节点上 kubelet 用的 containerd 没有仓库 CA | 看节点注解 `rlark.io/registry-trust.state` 和 `certs.d/registry.rlark.internal:5443/` |
| 拉镜像时解析不到 `registry.rlark.internal` | 节点 `/etc/hosts` 没有这一行 | 看 SiteNetwork 本场站的 `names` 和该节点的网关 |
| 不带账号访问仓库回 401 | 正常 | 管理机本机 `127.0.0.1:5443` 不要账号 |
| 机器人在厂商页面上下电后又自己上电 | 期望还是 `ready` | 在 RLark 上把期望设成 `off` |
| 机器人不接会话，回 409 `maintenance: …` | tuner 在做维护（重启、换载荷） | 等它做完；原因在注解 `rlark.io/maintenance` |
| Orin 停下不动，`s1u.rlark.io/orin-state` 说主机密钥变了 | Orin 重装过 | 人核对后把注解 `s1u.rlark.io/orin-host-key-confirm` 设成新指纹 |
| rclone 卷 close 报 EIO，或改名覆盖失败 | 没用 KS3 补丁版的 rclone | `csi-driver-rclone` 的 `rcloneImage` 用 `pinned.yaml` 里的 `csi-rclone` |
| 经转发枢纽的叶子之间不通 | 枢纽主机的 `FORWARD` 策略是 `DROP`（docker 设的），网关不改防火墙 | 在转发枢纽上放行 `st-wg0`→`st-wg0` 的转发（RLark 还没有声明式的做法，这是给 RLark 的要求） |
| 加入节点页“未加入的机器”是空的，机器人删掉后回不来 | 场地的发现 Hub 不在机器人所在网段（看 Machine 的 `status.entry.lastSeen` 停在哪天；Hub 本机 `ip -br addr` 有没有机器人网段的地址） | 把一台 Hub 接回机器人网段（或让机器人网段里的机器当 Hub）；Hub 没扫到入口时删除对话框会先警告 |
| 网关停在某条路由上不协调 | 自己接口上有文档里没有、带协议号 242 的路由 | 开 `--prune-routes`，或手删那条路由 |

## 仓库里删镜像

- 只删按摘要列出、核对过没在用的：各集群的 Pod 和工作负载、Addon 取值、RLark Job、发布文件、回退记录、`pinned.yaml` 都没有引用。
- `registry garbage-collect` 不带 `--delete-untagged`，执行期间停掉所有推送。
- 金山云那份在金山云起一个一次性 RLark Job 来删（钉在 `10-0-2-57`，仓库程序只听 `127.0.0.1`）。删除不可逆。
