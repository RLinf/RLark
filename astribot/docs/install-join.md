# 新建集群和节点入网

## 新建集群

控制台“新建集群”（`POST /api/v1/cluster-provisions`）只填：场站名、第一台机器的 SSH（地址、账号、口令）、集群名（默认 `rlark-<场站>-gpu`）。Gateway 把这台机器做成新场站的第一台枢纽和新集群的控制面。记录是 kcp 里的 `ClusterProvision`（集群级，名字 = 集群名）。

| # | 阶段 | 做什么 | 读回 |
|---|---|---|---|
| 1 | `precheck` | SSH 登录并记下主机指纹；系统、内核、`modprobe wireguard`、GPU、`/var/lib` 剩余、内网地址；节点名 = 主机名小写，不能重名 | 不满足的项写清楚并停下 |
| 2 | `bundle` | 从 Station API 取安装包流进 `/var/lib/rlark/bootstrap`；装 station-gateway；机器上生成 WireGuard 私钥 | 包的 sha256 一致 |
| 3 | `declare` | 写 SiteNetwork：名字 `cp.<集群>.rlark.internal`、新场站、这台枢纽（地址池里挑隧道地址，公网端点 `:51820`）；从 10.200.0.0/16 起挑不冲突的 Pod / Service 网段 | `status.declared` 里有这台 |
| 4 | `control-plane` | `station-bootstrap control-plane`；空闲盘 ≥300G 时加节点卷 `site-cache` | 节点 Ready；Gateway 经隧道连上 `<隧道地址>:6443` |
| 5 | `agent` | 签 agent 证书和拉取凭据，装集群 agent，声明节点 agent，给发布建凭据 | Cluster Online |
| 6 | `station` | Station 成员登记（`apps/rlark/astribot/stationmember`） | station-agent-host 就绪，节点 Enrolled |
| 7 | `addons` | 有卡时打 `rlark.io/accelerator=nvidia` 并装 `nvidia-gpu`；装 `registry-trust`、`csi-driver-rclone`；有 `site-cache` 卷时装 `site-registry role=cache` 并把本站的 `registry.rlark.internal` 指到本机；所有集群都有的 S3 存储加上新集群 | 每个 Addon Ready |
| 8 | `accept` | 提交一个小 Job（有卡跑 `nvidia-smi -L`，否则 `hostname`） | 日志里读到结果后删 Job |

- 口令只在请求体和内存里，不写对象、不进日志。1–6 阶段（`precheck` 到 `station`）要口令，从这些阶段重试要再填；只有 `addons`、`accept` 不要。
- 镜像和 Addon 取值只取发布记录 `rlark-system/rlark-release-current`，没有就拒绝。
- 每个阶段都能重做：先看机器和对象已是什么样。`POST .../{name}/retry` 从停下的阶段接着做；Gateway 重启时在跑的记录标为 Failed，重试即可。
- `POST .../{name}/clear` `{password?, user?, machine_gone?}`：撤销停下的新建（机器上 `control-plane undo`、停隧道），删没建完的 Cluster 和凭据，删掉它的场站节点记录和入网许可，从 SiteNetwork 去掉场站、名字和节点声明，删记录。返回每一步。同名再建在清掉之前回 409。
- 不是 Gateway 建的集群（如 Station 所在的集群）用 `rlarkctl station register` 登记成员，`rlarkctl release credential` 给发布建凭据。

## 机器怎么入网

三种方式，控制台“场站节点 → 加入节点”都有：

| 方式 | 谁登录机器 | 接口 |
|---|---|---|
| 勾选扫到的机器 | station-controller 用 NodeClass 的账号 | `POST /api/v1/station/machines/admit`；口令先录进 `PUT .../node-classes/{name}/access` |
| SSH 入网 | Gateway 用这次给的账号 | `POST /api/v1/station/node-joins`；阶段 precheck、bundle、declare、tunnel、install、node-ready、station、gpu，可重试、可清除 |
| 入网码 | 人在机器上粘一行命令 | `POST /api/v1/station/node-admissions`（也可 `astribot/station/hack/admit-node.sh CLUSTER NODE [label=value ...]`）；`/reissue` 重发，`/revoke` 作废 |

- 入网码只回答一次；NodeAdmission 只存它的 SHA-256，记发码人和时间。
- 命令先下载 Station API 的 CA 并核对摘要，再 `curl https://<Station API>/b/<码> | sudo sh`。机器在自带上联设备后面（NodeClass 的 `uplink.setup`，如机器人的 Orin）时，命令先 SSH 到那台设备放行。
- 扫到的机器：场站声明了 `discovery` 时，枢纽的网关在站点网段里找认得出的机器，报给 Station API，成为 `Machine`；它的转发口让管理面 SSH 过去。
- SSH 入网在机器上以 systemd 单元 `rlark-join` 运行，日志 `/var/log/rlark-join.log`；Gateway 断开不影响它。

## 安装器做什么

安装器是 `station-bootstrap`（`astribot/station/bootstrap/`）：拷安装包到 `/var/lib/rlark/bootstrap`，生成 WireGuard 私钥，向 Station API 兑换入网码，再交给 `install.sh` 在宿主机上执行：

1. 机器还留着已不存在的节点的安装时，先 `kubeadm reset`；还是别的集群的活节点就拒绝（先删那个节点，或用 `migrate-node.sh`）；是控制面就拒绝。
2. 建节点卷（`NODE_VOLUMES`，`/var/lib/rlark/volumes/<名>.img`，开机由 mount 单元挂载），装程序到 `/opt/rlark/bin`。
3. 隧道：主机服务 `rlark-station-gateway.service`，`station-gateway --await-peers` 等对端握手。站点名字随隧道写进 `/etc/hosts`。
4. 自己的 containerd（`containerd-fleet.service`，socket `/run/containerd-fleet/containerd.sock`；机器原有的 docker 不动），导入安装包里的镜像并钉住。
5. kubelet：读 `/etc/kubernetes/kubelet.conf.d` 的 drop-in，DNS 用 `/etc/rlark/station/resolv.conf`。
6. `kubeadm join` 到控制面的名字（不是地址），节点标签带 `station.rlark.io/network=host`；3 分钟内 Ready。
7. 节点注解 `rlark.io/node-install-spec`、`rlark.io/node-install-state`、`rlark.io/node-volumes`。

要注意的点：
- kubelet ≥1.35 默认不在 cgroup v1 上启动。机器还是 v1（机器人在节点规格重启之前）时，安装器写 `90-rlark-cgroup-v1.conf`（`failCgroupV1: false`）并让 kubeadm 跳过 `SystemVerification`；`s1u-robot` 把机器换到 v2 后删掉它。
- kubeadm 预检忽略 `Swap`：机器的 swap 不动。
- 加入失败的 `kubeadm join` 会当场 `reset`，下次可以直接重来。

控制面（`station-bootstrap control-plane`）：`CONTROL_PLANE_NAME`、`POD_SUBNET`、`SERVICE_SUBNET` 必填；端点是名字，etcd 听隧道地址；每天 etcd 快照到 `/var/backups/rlark-etcd`（留 14 份）；`WORKLOADS=yes` 时控制面也跑工作负载。`control-plane check` 只预检，`control-plane undo` 撤掉。控制面不知道机器自己网络的地址，所以机器可以换网络。

## 运维脚本（`astribot/station/hack/`）

| 脚本 | 做什么 |
|---|---|
| `admit-node.sh` | 发入网码，打印机器上要执行的一行 |
| `migrate-node.sh` | 把节点从一个集群迁到另一个，不登录机器；保留运行时、镜像、WireGuard 密钥和隧道地址 |
| `move-control-plane.sh` | 把控制面交给集群里另一台节点（`handover`、`FROM_CONTROL_PLANE`） |
| `upgrade-cluster.sh` | 逐台升级 Kubernetes，镜像是发布构建的 `rlark/node-bootstrap:ci-<r7>-installer-live` |
| `node-volumes.sh` | 把旧节点的数据挪进声明的节点卷（节点停一次） |
| `calico*.sh` | Calico 直连 API 端点、升级 |
| `e2e/` | 一次性的隔离演练环境（kind） |

## 删除节点

`POST /api/v1/station/node-operations` `{action: Remove, cluster_id, node_name, node_uid?}`，控制台节点页的“删除”。不登录机器。阶段：

`Preflight` → `PowerOff`（机器人先下电）→ `Drain`（节点 `rlark.io/in-use` 非空时拒绝，按 PDB 排空）→ `RemovePeer`（场站节点记录标 `retired`，全网去掉它的 WireGuard 对端，地址回池；去掉节点的 `station.rlark.io/network` 标签）→ `Reset`（kubelet 还在时，由放在它上面的 Pod 执行 `kubeadm reset`）→ `DeleteNode` → `VerifyRemoval`（确认管理面的节点没了之后删掉它的场站节点记录）。

- 不带 `node_uid`：删这个名字下的东西。集群里有这个 Node 就取它的 UID；没有就只把这个名字的场站节点记录标 `retired`、删没成节点的入网记录，集群不碰。
- 不上报的节点不会被 reset；下次入网时安装器清掉旧安装。
- 删除记录：`DELETE /api/v1/station/stations/{name}?cluster_id=`（控制台场站节点页的“删除记录”），`name` 是节点名。记录已 `retired`、`status.node.state` 是 `Conflict`、或节点不在上报（`status.node.state` 不是 `Bound`）且 RLark 里已没有这个名字的节点时删掉，否则回 409；读到之后记录又变了也回 409，重看一次再删。之后再上报的节点按新机器重新登记。

## 重新入网

| 情形 | RLark 做什么 |
|---|---|
| 一台机器兑换了一个码没成节点，又用另一个码入网 | 删掉第一条入网记录（同一把隧道公钥、从没 Joined），注解 `station.rlark.io/self-healed` 写明 |
| 节点没了（重装、手工 reset、删除），用入网码或 SSH 同名再入网，场站节点记录还是旧 Node UID 的或已 `retired` | 入网许可兑换得比记录最后一次上报晚：记录自动转给新的 Node（重启次数清零），写进 `self-healed` |
| 没有新的入网许可的同名新节点（Node UID 变了） | 记录进 `Conflict`，入网被拒，消息里写怎么办；管理员在控制台删除这条记录，节点下一次上报按新机器重新登记 |
| 换密钥 | 删节点上的 `/var/lib/station/wg/private.key` 重启网关；同一条记录登记新公钥，各对端约 40 s 内换掉 |
| 只是离线 | 什么都不变，回来就能连 |

## 入网排障

- 入网记录（控制台“入网记录”或 NodeAdmission）的 `status.reason` 写停在哪一步：没回来取令牌 / 发了令牌但没有节点上报 / 节点上报了但场站节点记录还是旧节点的 / 隧道登记没过。兑换 20 分钟后标为 `stuck`。Redeemed 的行有“清掉重来”（按名字删除）。
- 机器上：入网码那一行的输出就是安装日志，失败时最后一行是 `FAILED: <原因>`；SSH 入网看 `/var/log/rlark-join.log`；迁移看 `/var/lib/rlark/bootstrap/install.log`。
- `no peer has answered through the tunnel`：`station-gateway --config /etc/rlark/station/bootstrap.json --status` 看握手；看对端的 UDP 51820 是否可达。
- `cannot resolve cp.<集群>.rlark.internal`：SiteNetwork 的 `names` 里没有它。
- `Station API is not reachable` / 时钟偏差：机器到 Station API 的 HTTPS 不通，或时钟差太多。
- 节点 Ready 但没入网：看 `station-agent-host` Pod 和 NetworkEnrollment 的 `status.phase`、`reason`。
