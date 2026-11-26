# 架构

```
管理机 10.11.14.128（办公室）
  docker compose：rlark-server、rlark-controller-manager、rlark-gateway（:19000）、rlark-ui（:80）
  另一组容器：kcp、数据库、本机仓库 localhost:5555（发布不动）
  site-registry 办公室仓库（:5443）
        │ 场站隧道 WireGuard 10.254.240.0/24（station-gateway）
        ├── rlark-local-dev-gpu   办公室集群；Station API、station-controller 在它的 station-system 里
        ├── rlark-robot-s1u-01    机器人集群（S1U 的 x86）
        ├── rlark-ksyun-gpu       金山云
        └── rlark-baidu-gpu       百度云
```

成员集群以 kcp 里的 `Cluster` 为准。每个集群跑一个集群 agent（`rlark-system`，`--mode=cluster`）、每台节点一个节点 agent、Station 的 DaemonSet 和装上的 Addon。

## 控制面

`astribot/release/control-plane/rlark.compose.yaml` 定义四个服务，镜像都出自同一个发布：

| 服务 | 做什么 | 端口 |
|---|---|---|
| `rlark-server` | 官方 Server：agent 的连接、证书 | 8443、2222 |
| `rlark-controller-manager` | 官方控制器：Job、Task | — |
| `rlark-gateway` | REST 接口和我们加的全部接口（[swagger.yaml](swagger.yaml)）、新建集群、SSH 入网、机器人占用清扫 | 19000 |
| `rlark-ui` | 控制台，`/api/` 转给 gateway | 80 |

- compose 文件和 env 文件由发布写（`/var/lib/rlark/rlark.compose.yaml`、`/var/lib/rlark/control-plane.env`），机器上不手改。
- 每个组件用自己在 kcp 里的身份：`${R}/identity/<组件>/{kubeconfig,token}`，`R=/home/cailiang/.local/share/rlark-runtime`。`rlarkadm identity issue` 写出，`rlarkadm identity rotate` 轮换（一分钟内生效，不用重启）。
- Gateway 必须有 `--station-api-address`、`--station-api-ca-file`、`--station-gateway-credential-file`、`--station-api-bootstrap-url`、`--registry-browse-allowed-hosts`，缺一个不启动。
- Addon 目录：内置的编在程序里；`RLARK_ADDON_CATALOG_DIR`（`/var/lib/rlark/addons`）放别的版本，每版本一个子目录，5 秒内生效。
- 发布记录：kcp 里 `rlark-system/rlark-release-current`。新建集群时 Gateway 只从它取镜像和 Addon 取值。

## 成员集群里的 RLark

- **集群 agent**：宿主网络，优先在控制面节点上，经隧道按名字 `rlark-server` 连管理面；Deployment 是 `Recreate`。它从 Addon 的包渲染 Addon，滚动节点 agent 和 Station 的 DaemonSet。
- **节点 agent**：在 Cluster 上声明，`PUT /api/v1/clusters/{id}/node-agents` `{"enabled", "image", "nodeSelector"}`（默认选 `rlark.io/enabled=true`）。集群 agent 建 DaemonSet `rlark-agent-node`。节点 agent 把 `/run/rlark/in-use/` 下的文件名写到节点注解 `rlark.io/in-use`。指标端口 10381，健康 10382。
- **滚动**：带注解 `rlark.io/rollout` 的 DaemonSet 是 `OnDelete`，集群 agent 先换一台，再每次换就绪节点的十分之一；关机的节点不挡别人；机器人只在空闲时换。
- **按节点放置的 Task**：Job 的 `placement.perNode: true` 加 `nodeSelector`，每台匹配节点一个实例（carrier `rlinf.io/kubernetes-daemonset`），`whenIdle: true` 只在 `rlark.io/in-use` 为空时替换。没有 head 的 Job 不组 Ray 集群。

## Station（场站节点）

界面上叫“场站节点”：一台节点在场站里的记录（身份、心跳、入网），按节点名一台一条。“Station”这个词在 Fleet 里是 Run 的 station；CRD 组 `station.rlark.io`、程序名 `station-*` 是代号。

### 对象（kcp 管理面，`station.rlark.io/v1alpha1`）

| 对象 | 用途 |
|---|---|
| `SiteNetwork`（`station-system/default`） | 全站网络的唯一声明：地址池、场站、名字表、非叶子节点。改拓扑只改它 |
| `NetworkEnrollment` | 场站节点：每台节点一个，在集群的命名空间里按 Node 名命名。spec 是身份（`nodeUID`、`systemUUID`：station-api 验上报的令牌时读到的 Node 的 UID 和 `status.nodeInfo.systemUUID`，不由 agent 自报；建记录时写入，名字转给另一个 Node 时由 controller 改写）、入网登记（公钥、自报地址；没有公钥的节点只有身份、不在隧道网里）和 `retired`（节点已删除）；status 分三块，各有一个写者：算出来的隧道文档（controller 的网络部分）、`observation`（agent 每 30 s 的上报：上报的 Node 的 `nodeUID` 和 `systemUUID`（控制台显示和搜索的 System UUID）、心跳、开机时间、隧道探测、网关版本，station-api 写）、`node`（`state` Bound / Offline / Conflict / Retired、重启次数、各条件，controller 写） |
| `NodeClass`（集群级） | 一类机器：标签和污点、命名、怎么在网上认出（discovery）、登录账号放哪（access）、上联（uplink，如机器人经 Orin 上网的脚本） |
| `Machine` | RLark 知道的一台机器：扫到的或自己登记的，管理员从这里勾选入网 |
| `NodeAdmission` | 一次入网许可：只存入网码的 SHA-256；兑换后留作这台节点的网络声明 |
| `NodeOperation` | 一次节点操作：`Join`（用 NodeClass 的账号登录装机）或 `Remove`（删除） |

身份规则：station-api 用 TokenReview 认出上报的 Pod 在哪台 Node 上，得到节点名和 UID。没有这台的记录就建一条；记录的 `spec.nodeUID` 等于这个 UID 就只写 `status.observation`。UID 不同（重装、换机）或记录已 `retired` 时，上报照写，入网登记回 409；这个名字的入网许可（NodeAdmission）兑换得比记录最后一次上报晚，controller 就把记录转给新的 Node（`status.node` 重来，许可上写注解 `station.rlark.io/self-healed`），否则 `status.node.state` 是 `Conflict`、入网被拒，管理员在控制台删除这条记录，节点下一次上报按新机器重新登记。

### 程序

| 程序 | 在哪 | 做什么 |
|---|---|---|
| station-api | `rlark-local-dev-gpu` 的 `station-system`；入网的机器经 `https://10.11.14.128:18443`，Gateway 经 `https://10.254.240.1:18443` | 节点上报（TokenReview 核对 Pod→Node，写进它的 NetworkEnrollment）、入网登记、发安装包和入网码兑换（`/b/<码>`、`/v1/bootstrap/*`）、节点凭隧道密钥取文档（`/v1/network/document`）、扫到的机器（`/v1/network/discovered`） |
| station-controller | 同上 | 每台节点的在线状态和身份变化、每台节点的隧道文档、NodeClass、Machine、NodeOperation |
| station-agent | 每个成员集群的 DaemonSet | 心跳、登记公钥、取回文档、探测对端 |
| station-gateway | 每台节点 | 按文档维护 WireGuard 接口、对端、路由、`/etc/hosts` 里的站点名字、给 kubelet 的 `resolv.conf`、到场站网段的转发；每 10 s 协调一次 |

两个 DaemonSet，按节点标签 `station.rlark.io/network` 放置：

| DaemonSet | 标签 | 内容 |
|---|---|---|
| `station-agent-network` | `enabled` | agent + `wireguard` 容器（网关在 Pod 里） |
| `station-agent-host` | `host`（安装器装的节点） | agent + `gateway-program` 容器：把主机上的 `/opt/rlark/bin/station-gateway` 换成本镜像的版本（机器在用时不换，起不来换回），维护 `/etc/rlark/station/station-api.json` 和 CA。网关本身是主机服务 `rlark-station-gateway.service`，在 kubelet 之前起来 |

### 网络

- 拓扑：叶子拨枢纽，枢纽之间按需互联，转发枢纽替叶子转发。没在 `spec.nodes` 里列出的节点都是叶子。
- `sites[]`：`clusters`、`reaches`（能拨到哪些场站的 `Site` 端点）、`mesh`（本站枢纽之间建隧道）、`relay`、`leafUplink`（同型号叶子共用的上联，如机器人经 Orin）、`discovery`（枢纽在 `ranges` 和自己接着的每个私有 IPv4 网段里按 NodeClass 的出厂主机密钥找未入网的机器，比 /22 大的网段只扫自己地址所在的 /22，不含公网、`st-*` 隧道、容器和集群接口；每轮最多 128 个并发探测，三个 /22 最多约 25 s，每轮从上一轮开始算起隔 `intervalSeconds`；几台枢纽在同一网段时，机器跟着先报它的枢纽，那台三个周期没再看到它才换）、`dns`、`names`（本站的名字，如 `registry.rlark.internal`）。
- 控制台“场站节点 → 扫描设置”可改 `sites[].discovery` 的 `enabled`/`ranges`/`intervalSeconds`；`from` 是 station-controller 的拨号源，扫描由该 site 的所有 Hub 执行。
- `nodes[]`：`role: Hub` 加 `endpoints`（`Public` 谁都能拨，`Site` 本站和 `reaches` 它的场站能拨）、`relay`、`nodeAddress`（别人找它用的地址不是隧道地址时，如云上 VPC 地址）、`advertise`（经它可达的网段：`cidr`、`sites`、`forward`，见下面“转发到场站的网段”）、`uplink`、`publicKey`（还连不上 Station API 的节点先声明公钥）。
- 有上联（`uplink` 或本站的 `leafUplink`）的节点，文档里的 `uplinkRoutes` 是它拨的每个端点和本站 `dns` 的每台域名服务器各一条经上联网关的 /32 路由；站点不写 `dns` 时网关把机器自己的 resolv.conf 交给 kubelet，机器人上那份没有域名服务器，所以机器人所在的站点要写 `dns`。
- `names`：全站同一地址的名字，如各集群的 `cp.<集群>.rlark.internal`、`rlark-server`、`gateway.rlark.internal`。网关写进每台节点的 `/etc/hosts`（行尾 `# rlark names`）。
- 连接规则：两台之间有隧道，当且仅当至少一台是枢纽且至少一方能拨到另一方；能拨的一方写端点和 25 s 保活；AllowedIPs 是对端的隧道地址加它的 `nodeAddress`、`advertise`，转发枢纽对叶子再加整个池。
- 默认 MTU 1380，接口 `st-wg0`，端口 51820。controller 每次算完检查任意两台之间有没有路，没有的列在 `status.unreachable`，条件 `FullyConnected` 为假。
- 网关只动自己的东西：只接管别名 `rlark-station-gateway/v1` 的接口；路由用协议号 242、metric 49321；对端逐个增删改，不动没变的会话；iptables 只动自己的两条链 `RLARK-STATION-FORWARD`、`RLARK-STATION-SNAT` 和进入它们的那两条（外加：有转发的网关删掉 station-forwarder 留下的、带注释 `rlark-station-forwarder-v1` 的规则，那个程序从不删自己的规则）。
- 网关取文档的顺序：agent 或自己从 Station API 取回的 → 私钥旁的 `last-applied.json` → 安装器写的静态文件 `/etc/rlark/station/bootstrap.json`（只有主机上的网关有，给不经隧道连不上 Station API 的节点）。有 `station-api.json` 时网关每 30 s 用自己的私钥向 Station API 证明身份取文档，所以 agent 不在时也能跟上控制面的变化。
- 文档版本 `documentLevel`（`astribot/station-gateway/main.go`、`astribot/station/network`）：给文档加字段时加一（3 加了 `forwards`）；发布要等所有网关报到新级别才换 controller（阶段 `controller` 的 `gateways-understand`），所以提级别的改动分两次上线，见 [release.md](release.md)。

### “在线”是三件事

| 事实 | 条件 | 来源 |
|---|---|---|
| 心跳 | `Observed`，90 s 没有上报为 Offline | agent 每 30 s |
| 隧道 | `NetworkVerified` | agent 对每个在线对端经隧道建 TCP；对端自己离线不算失败 |
| RLark 节点 | `RLarkLinked` | 同名 Node 的 Ready |
| 重启 | `Rebooted`、`status.node.reboots` | 开机时间比上次晚 30 s 以上 |

### 部件停了会怎样

| 停的是 | 后果 |
|---|---|
| station-api、station-controller | 已有隧道不变；新节点进不来，地址变化不传播 |
| 某台的 agent | 它的隧道不变；主机服务形式的网关自己取文档 |
| 某台的网关 | 内核里的接口和会话还在，不再协调 |
| 转发枢纽 | 经它的叶子之间不通，叶子到其他枢纽不受影响 |

### 转发到场站的网段

`advertise` 写了 `forward: true` 的网段（如办公室的 NAS `10.11.5.12/32`）在通告它的节点的网络上、不回隧道，那台节点替收到通告的场站转发：

- controller 在通告节点的文档里写 `forwards`：每个这样的网段一条，`sources` 是收到通告的场站（`sites`，空为其他所有场站）全部成员的隧道地址。新节点入网后自动在里面，不用改声明。
- 通告节点的网关按文档维护两条自己的链：`filter` 的 `RLARK-STATION-FORWARD`（`FORWARD` 第一条进入）只放这些源从隧道接口到这个网段、和回来的已建立连接；`nat` 的 `RLARK-STATION-SNAT`（`POSTROUTING` 第一条进入）把它们的源换成节点在这个网段上的地址（内核到这个网段的路由的接口和源地址），所以对面不用有回隧道的路由。链和文档不同就整条重写（`iptables-restore --noflush`，一次提交），相同就不动；文档里没有了就连同进入它的那条一起删掉。到这个网段的路由走回隧道或没有源地址时，这一轮协调失败。
- 没写 `forward` 的网段（各节点的 Pod 网段）只通告路由，不转发、不换源。
- 容器里的网关用 kube-proxy 镜像带的 iptables，按 kube-proxy 的办法选节点在用的后端（nft 或 legacy）；主机服务形式的网关用机器自己的 `iptables`。
- 收到通告的节点经隧道发出时源地址是它的隧道地址（路由不带源，内核取隧道接口的地址）；Pod 的流量由 calico 的 natOutgoing 换成同一地址。

以前办公室手装的 `rlark-station-system/rlark-station-nas-forwarder`（station-forwarder，只放 196 一个源）由发布删除：`release.RetiredNamespaces`，见 [release.md](release.md)。

## 我们加的 Addon

| Addon | 做什么 |
|---|---|
| `s1u-robot` | S1U 机器人 x86 的一切：节点规格、Orin、电源、相机、时钟、设备层、常驻 Station（下一节） |
| `site-registry` | 站点镜像仓库：`role=registry`（可推可拉，存储 `filesystem` 或 `s3`）、`role=cache`（拉取穿透缓存，回源 `upstream`）、`role=mirror`（每节点只读缓存）。密钥在一个 RLark 密钥对象里 |
| `registry-trust` | 每台节点 kubelet 所用 containerd 的 `certs.d/<仓库名>/` 写 `hosts.toml` 和 CA；containerd 2.x 的 `config_path` 是列表时改成第一个目录并重启一次运行时（机器人上只记注解）。状态在注解 `rlark.io/registry-trust.state` |
| `package-proxy` | 没有公网的站点取软件包的出口：只做 HTTP CONNECT、只放行 `allow` 里的域名，监听在节点的隧道地址上 |
| `csi-driver-nodepath` | 节点上已挂好的文件系统（如 KPFS）当存储：核对挂载类型后 bind mount 进 Pod，给有它的节点打 `storage.rlark.io/<sc-id>=true` |
| `nvidia-gpu` | GPU 节点：NVIDIA Container Toolkit 装进运行时并设为默认，加设备插件；节点只需自带驱动 |

官方的 `csi-driver-rclone` 我们加了 `nodeUpdateStrategy`、`nodeImagePullPolicy`、`controllerNodeSelector`、`nodeTpsLimit`；镜像用 `astribot/csi-rclone-ks3` 的 KS3 补丁版（`v0.2.0-ks3.3`）。补丁只对 `ks3-*.ksyuncs.com` 生效：不把 KS3 分块上传的 ETag 当 MD5（KS3 的这个 ETag 不是内容的 MD5，否则 close 报 EIO）、改用长度和 CRC64 校验、HTML 的 429 交给 rclone 重试、改名覆盖时先删目标、每个卷用自己的 remote。

Addon 的包：装或更新时，Gateway（或发布命令）把“名字@版本”发布成集群管理命名空间里的不可变 ConfigMap `addon-package-<名字>-<版本>`；集群 agent 只从包渲染，每个 generation 一次，结果在 Secret `addon-rendered-<名字>`。参数可以写 `requiredWith`：另一个参数有值时本参数必填。

### s1u-robot

一个 DaemonSet，选 `rlark.io/robot-model=s1u` 的节点。新机器入网后自动套上，没人登录机器人或 Orin。

| 容器 | 源码 | 做什么 |
|---|---|---|
| `tuner` | `s1u/tuner/` | 宿主机符合节点规格（隔离核、cgroup v2、CPU 和内存预留、kubelet CPU 管理、调频、调度参数）；需要时在没人用时重启整机或 kubelet（维护）；发布自检结果 |
| `orin` | `s1u/orin/` | 经机器人内网维护 Orin：上网单元、相机 USB 规则（Orbbec `2bc5`、RealSense `8086`）和按序列号的单路相机 USB 复位、adb 密钥、`orinPayloads` 载荷 |
| `power` | `s1u/device/power.py` | 电源对账（下面）；把占用镜像到宿主机文件 |
| `video` | `s1u/device/cameras.py` | 机器人相机的唯一出口和看护：一直订阅本机体的各路相机，只读 HTTP（端口 18891），没画面的相机按阶梯自动恢复（下面） |
| `clock` | `s1u/clock/` | 把 x86 和 Orin 对到站点时间源（`timeSource`） |
| `device`、`station` | `deviceImage`（Fleet 的镜像）的 `/device/start`、`--station` | 设备层、机器人上常驻的 Station。声明了 `deviceImage` 才有 |

和其他组件的接口：

| 东西 | 在哪 |
|---|---|
| 期望电源 | 节点标签 `rlark.io/robot-power` = `off`/`powered`/`ready`（没有按 `ready`），经 Gateway `PUT .../nodes/{name}/robot-power` 或控制台改 |
| 电源现状 | 标签 `rlark.io/power-state`（`off`/`powered`/`ready`/`estop`/`fault`/`unknown`），注解 `rlark.io/power` |
| 电源服务 | 本机 socket `/run/rlark/services/s1u-power.sock`（`GET /power`、`POST /power {action, force, reason}`、`POST /pose {pose, action}`：按住移动的初始姿势，网关 `POST nodes/{name}/robot-pose` 经 Server 的 `nodeproxy` 和该节点的节点 agent 转到这里） |
| 占用 | 节点标签 `rlark.io/robot-claim`；`power` 盯着自己的节点，一秒内写到 `/run/rlark/s1u/claim`（`{"holder"}`，没人持有时没有文件） |
| 在用 | `/run/rlark/in-use/<组件>`：Station 服务 Run 时写 `fleet-station`，设备层有会话时写 `s1u-device`，内容写明 Run（`... Run <id> ...`）；上电、驱动在跑不算在用 |
| 重启 | 认领者经 Gateway `POST .../nodes/{name}/robot-restart {level: driver/power, holder, request_id}` 写管理面注解 `rlark.io/robot-restart`，agent 带到集群 Node；`power` 每个 id 只做一次：停驱动 → 等手臂缓降（`S1U_DRIVER_STOP_SECONDS`）→［power：下电 → 等 10 s］→ 上电起驱动到 ready。认领不是 holder、有标记不属于它的 Run、有维护时失败并写原因。期望电源不动。进度在注解 `rlark.io/power.restart` 和 `rlark.io/power` 的 `restart`：`{id, level, holder, phase: lowering/raising/done/failed, error, at}`，socket 的 `GET /power` 也带 |
| 维护 | 租约 `/run/rlark/s1u/maintenance`；标签 `rlark.io/maintenance-state`，注解 `rlark.io/maintenance` |
| 相机 | 标签 `rlark.io/camera-state`（`ok`/`recovering`/`degraded`/`stuck`），注解 `rlark.io/cameras`（和 `GET /cameras` 同一份 JSON），只在变化时写；节点事件 `CameraDriverRestart`、`CameraUsbReset`、`CameraStuck`、`CameraBack`；看画面经 Gateway `GET .../nodes/{name}/cameras…`（下面） |
| 自检 | 设备层写 `/run/rlark/s1u/self-check.json`；标签 `rlark.io/self-check-state`，注解 `rlark.io/self-check` |
| 机器人是谁 | tuner 从标签 `rlark.io/robot-id`、`rlark.io/robot-body` 写 `/run/rlark/s1u/robot.json` |
| 规格和机体 | `/etc/rlark/s1u/node_profile.env`、`bodies.json` |
| Orin 主机密钥 | 标签 `s1u.rlark.io/orin-state`；密钥变了由人把注解 `s1u.rlark.io/orin-host-key-confirm` 设成新指纹 |
| 重启机器人 | 节点标签 `rlark.io/restart-request` 设一个新值（同一个值只做一次），作为一次维护做；结果在注解 `rlark.io/restart-request-state` |
| 无人下电 | 参数 `powerOff`：`unattended`（默认）tuner 自己下电再重启；`attended` 电机带电的要等人在 RLark 上下电 |
| 分批 | 维护按节点标签 `rlark.io/rollout-wave`（整数，没有的是最后一批）分批，同时进行的台数不超过参数 `rolloutParallel`；进度在标签 `rlark.io/rollout-state`、注解 `rlark.io/rollout` |

端口（都在主机网络上）：

| 端口 | 绑定 | 做什么 |
|---|---|---|
| 18891/tcp | 节点地址（站点隧道的，`status.hostIP`） | `video` 的只读相机接口；只由 Gateway 经成员集群 API 的 Pod 代理访问 |

电源的控制只走 `/run/rlark/services/s1u-power.sock`。

相机（`video`，`s1u/device/cameras.py`）：
- 视角四个，顺序固定：`head`、`torso` / `wrist_left`、`wrist_right`（2x2）。厂商名 `head_rgbd`（Orbbec Femto Bolt）、`torso_rgbd`（Orbbec Gemini 335）、`left_wrist_rgbd`、`right_wrist_rgbd`（Intel RealSense D405）。机体的视角在 `bodies.json`：`s1u` 四个，`s1u_observer` 只有 `head`、`torso`。
- 从启动起一直订阅 `/astribot_camera/<厂商名>/color_compress/compressed`（`rospy.AnyMsg`，不解码），留最新一帧 JPEG、收到的时刻和帧率。
- 每路状态只看话题本身：`ok`；`slow`（低于 10 帧每秒持续 30 s）；`stale`（5 s 没有帧）；`absent`（启动以来没见过）。厂商的 `/astribot_error_code/multi_error_code` 只留最后一个码，不用。
- 接口（只读）：

| 路径 | 回答 |
|---|---|
| `GET /cameras` | `{state, cameras: {<view>: {state, fps, age_seconds, step, attempts, next_attempt_at, last_action, last_result, diagnosis, usb, usb_advice}}, updated_at}` |
| `GET /cameras/<view>.jpg` | 最新一帧 JPEG（头 `X-Age-Seconds`）；机体没有这路 404，还没有帧 503 |
| `GET /cameras/<view>/stream?fps=N` | `multipart/x-mixed-replace`，相机自己的 JPEG 原样转发，N 为 1–30（默认 10） |
| `GET /cameras/wall.jpg`、`/cameras/wall/stream?fps=N` | 2x2 缩略图 256x144（每格 128x72），没有新鲜帧的格子是黑的，N 为 1–10（默认 5）；只有这里解码 |

- 自动恢复（不管有没有 Run 在用都做；每一步在节点上记一条事件）：容器启动后 90 s 内不动手。每路相机各在阶梯的一级上，按它自己 `stale`/`absent` 连续 30 s 算（比一次驱动重启的黑屏长，谁重启的都一样，别的相机的重启让它黑屏不算）：① 重启厂商相机驱动（`/astribot_system_monitor/camera_control_command` False，2 s，True；所有相机约 25 s 没画面；每次调用 30 s 不答算失败），最多 3 次，间隔 60 s 起翻倍、最长 900 s；一次重启算给所有还没用完 3 次的没画面相机；厂商服务不在不算一次，也不翻倍，一分钟后再试。② 经 Pod 内共享目录 `/camera` 请 `orin` 容器对这几路做 USB 复位（`reset-request {id, cameras: 厂商的相机名}` → `reset-result {id, ok, detail, reset, missing, finished_at}`，`reset`、`missing` 是复位了的和 USB 上没有的相机，复位没跑成时两个都没有；`orin` 按 Orin 上 `/etc/config/camera_info.json`（相机名 → 序列号）找到序列号对应的设备，经 usbfs `USBDEVFS_RESET` 复位，同一个 id 只做一次，容器重启也不重做；`orin` 动手前画面回来了就撤回请求，`video` 容器重启时也撤回上次的请求；请求收到回答或不再等（300 s）就收回，`orin` 那边出错或超时也写回答，同一个请求只做一次），再重启一次驱动（也算给其余还有重启次数的没画面相机）；复位的时刻从这次重启结束算。复位没跑成（ssh、Orin、`orin` 300 s 不答）不重启驱动，按这路相机的间隔（之后翻倍）再请；USB 上没有这路相机的直接 `stuck`，原因写“Orin 的 USB 上没有这路相机…USB 复位没做”。③ `stuck`：相机需要断电或 USB 复位。USB 复位后 5 分钟内又停的相机直接 `stuck`，原因写“疑似硬件：USB 复位后 N 秒又停，需要断电或现场检查接线”（s1u-03 11-21：头部彩色流每次复位后只撑 25 秒到 3 分钟）。`stuck` 的相机退出阶梯，直到它又连续 5 分钟有画面（只回来一下又停的仍是 `stuck`），或改接到别的 USB 口（`/camera/usb` 里的路径变了；复位只改设备号），从阶梯开头重来；其余相机照常走各自的阶梯。`stuck` 后只在机器人没人用（`/run/rlark/in-use` 下没有标记）时每小时重启一次驱动：以前不分在不在用每小时整套重来，s1u-03 11-21 的 26 小时里所有相机断帧 89 次，头部每次回来几十秒到半小时。整机重启（控制台的“重启”、断电再开）后 ROS master 重启，相机容器从头开始；控制台的启动/关闭驱动不重启 ROS master，不解除 `stuck`。进入 `stuck` 时经 `/camera` 请 `orin` 诊断（`diagnose-request {id, cameras}` → `diagnose-result {id, cameras: {厂商名: 一行诊断}, finished_at}`）：在不在 USB 上、在哪（路径、速率、经几级 hub）、厂商相机进程 `astribot_camera_main` 在不在、Orbbec SDK 打开的相机（接口驱动 `usbfs`）在 `~/Log/OrbbecSDK.log.txt` 尾部彩色和深度流还在不在；诊断写进这路相机的 `diagnosis` 和事件 `CameraStuck`（`orin` 300 s 不答也发，写明没答；Orin 上没有 SDK 日志时写“没有日志”）；回到阶梯时记事件 `CameraBack`。`orin` 每分钟把各路相机在 USB 上的位置写到 `/camera/usb`（`{厂商名: {path, device, speed, hubs}}`），报告里每路的 `usb`；一路相机所在的 hub 上的相机比同一根口下另一个 hub 上的少（一样多时两边都给）时给 `usb_advice`（与谁共用根口、经几级 hub、建议改接独立根口），只是建议，不改状态：s1u-03 的头部经两级 hub 和躯干、两腕（它们在另一个 hub 上）共用根口 2-3，只提示头部；s1u-01、s1u-02 的头部独占根口，躯干和两腕在同一个 hub 下，不提示。一路相机连续 5 分钟有画面，它的阶梯才回到起点：时好时坏的相机接着往上走，不会每回来一下就从头重启驱动。只有 `slow` 的相机：只做 ①，每小时最多一次。机器人的 ROS master 重启了（进程号变了），`video` 容器自己退出由 kubelet 重启（rospy 不会在新 master 上重新订阅），`stuck` 随之清空。
- 节点状态：标签 `rlark.io/camera-state` = `ok` / `recovering`（有相机没画面）/ `degraded`（只有帧率低的）/ `stuck`，注解 `rlark.io/cameras`；agent 把它们带到管理面的 Node 上。
- Gateway：`GET /api/v1/rlinf.io/v1alpha1/nodes/{name}/cameras`、`.../cameras/{camera}`（`<view>.jpg`、`wall.jpg`）、`.../cameras/{camera}/stream?fps=N`（`<view>`、`wall`），`?namespace=<集群命名空间>`；鉴权同 `robot-claim`（登录即可）。Gateway 在成员集群里找这个节点上的 `s1u-robot` Pod，经 API 的 Pod 代理（`/api/v1/namespaces/rlark-system/pods/<pod>:18891/proxy/cameras…`）转发，流不缓冲。控制台站点节点页的“相机”一栏用它。

电源规则：
- `power` 每几秒对账，不在任务里也一样：该上电就上电，驱动掉了就重启并记一次恢复；`estop`/`fault` 时不拉，解除后再收敛；失败从 30 s 起退避，最长 10 分钟。
- 往下只在人调低期望时做一次：先停驱动（手臂缓缓下落），5 秒后下电，并等机器人不在用。
- 任务用了机器人（设备层按 `bring_up`），期望就成 `ready`，直到有人在 RLark 上重设。要机器人保持下电，就在 RLark 上设 `off`：在厂商页面上下电会被恢复。
- 维护期间按租约临时降低，不改写期望。

Station 容器：机器人被一个 Run 占用时，带站点令牌向 `stationControlURL` 取这个 Run 的连接参数，起 Ray 节点和 env worker，经 `/run/fleet` 接设备层，期间写在用标记；占用换了或没了就退出。内存请求 2 GiB、上限 2304 MiB（整个 Pod 的上限放得进节点规格给 Pod 的内存），`/dev/shm` 是 512 MiB 内存卷，Pod 停止给 60 s。

撤除：`profile` 设为 `removed`，等 `rlark.io/node-profile-state`、`s1u.rlark.io/orin-state`、`rlark.io/clock-state` 都是 `removed` 再卸载 Addon。

## 存储

- Gateway 的存储对象（类型 S3、`smb`、`hostpath`）按集群分发成 StorageClass：每个档位一个，`sc-<id>`、`sc-<id>-default`/`-read`/`-archive`（SMB 和节点目录只有 `sc-<id>`、`sc-<id>-read`）。S3 另有 `mount_endpoint`、`cluster_mount_endpoints`（按集群的挂载入口）、`prefix`、`vfs_cache_mode`、`mount_profile`、`mount_options`。
- 按名字引用：卷写 `csi.driver: storage.rlark.io`、`volumeAttributes: {storage: <名字>, profile: <可选>}`。Job 控制器按 Task 所在集群解析，解析不了时不建 Task，条件 `StorageAvailable=False`。存储视图是各集群命名空间里的 ConfigMap `<sc-id>-storage`，不含凭据。
- 改名不换 id 和 StorageClass。
- KS3 上未知长度的流在默认分片下最大约 48.8 GiB。

## 镜像仓库

所有镜像按同一个名字、按摘要引用：`registry.rlark.internal:5443/<仓库>@sha256:<摘要>`。这个名字在每个场站解析到本站的仓库（SiteNetwork 场站的 `names`）。证书由站点 CA `rlark-registry-local` 签发；节点从安装包拿到这张 CA，之后由 `registry-trust` 维持。

| 仓库 | 谁从这里拉 | 存储 | 入口 |
|---|---|---|---|
| 办公室（管理机） | 办公室集群、机器人集群 | 节点卷 `/var/lib/rlark/site-registry`（loop ext4，200G） | `10.11.14.128:5443`（TLS，要账号）；管理机本机 `127.0.0.1:5443`（HTTP，不要账号） |
| 金山云（`10-0-2-57`） | 金山云集群；发布的镜像都存一份在这里 | KS3 `stardust` `/ks3-regular/post_train/rlark/registry` | `0.0.0.0:5443`；管理机经隧道 `https://10.0.2.57:5443` |
| Gateway 新建的场站 | 本场站 | 本机 `site-cache` 卷 | `role=cache`，回源 `origin.rlark.internal` |

- 每边只从自己的仓库拉。一个镜像两边都要，就按摘要各放一份（发布的 `images` 阶段和 `rlarkctl release copy` 做）。
- 拉取凭据：`ir-8be41d25ef634987` 发到办公室和机器人集群，`ir-667c8af90d294c2b` 发到金山云；仓库名都是 `registry.rlark.internal:5443`。集群 agent 按镜像前缀给任务和 Addon 的 Pod 注入；Station 的 Pod 用环境文件的 `station.pullSecret`。凭据变了，已就绪的 Pod 不换，要等它的任务或 Addon 下次变更。
- 引导镜像：`site-registry`、`site-registry-relay`、`registry-trust`、`registry-trust-kubectl` 在节点能拉镜像之前就要有，写在 `astribot/release/pinned.yaml` 的 `installer`，打进安装包，入网时导入并钉住。`site-registry` 的 `image`、`relayImage` 是各站点自己的取值，发布不写。它的 Deployment 是 `Recreate`，新 Pod 要从刚停掉的仓库拉：要换，先用一次性 RLark Job（钉在仓库节点，`imagePullPolicy: IfNotPresent`）把新镜像拉到那台节点并读回节点上有了，再 PUT Addon。
- 列表和详情不返回仓库密码；管理员 `GET /api/v1/image-registries/{id}/password` 读，记审计。控制台可浏览仓库（`--registry-browse-allowed-hosts` 放行的地址）。

## 机器人占用、服务账号、角色

- 角色：`user`、`operator`、`admin`。`operator` 能设机器人电源、强制释放占用。
- 占用：`PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/robot-claim` `{holder, job, leaseSeconds}`（5–600 s）。空闲则授予，持有者再申请即续租，别人申请回 409 `ROBOT_CLAIM_BUSY`，当前持有者在 `details.current`。`?renew=true` 只续租，不授予。`DELETE ...?holder=` 释放，`?force=true&reason=` 由管理员或 operator 强制释放，回被放掉的占用。
- 占用只由释放或清扫结束：租期过了还没被清扫的仍属原持有者。Gateway 每 10 s 清扫；启动后和上游恢复后先等 1 分钟。机器人不数租期。
- 服务账号：`/api/v1/service-accounts`，有角色和租户，一个不过期的令牌，只存哈希；轮换或删除立即失效。令牌可以作为 Job 密钥下发。
- Job 密钥：`/api/v1/secrets`，按集群下发成 Job 命名空间里同名的 Secret，列表不返回值。

## Fleet 怎么用 RLark

| Fleet 要的 | RLark 接口 |
|---|---|
| 提交、停止 Run | 官方 Job 接口；停止是 `PATCH spec.suspended`；用服务账号（`operator`）的令牌 |
| 节点身份、地址、在线 | `GET /api/v1/site-nodes[/{node}]?cluster_id=&label=k=v`：`node_uid`、`phase`、`online`、`tunnel`、`rlark_node`、`boot_time`、`reboots`、`network.address`（别的节点找它用的地址）、`robot_id`、全部标签 |
| 用机器人 | 占用接口（上一节）；机器人上的 Station 容器读占用，向 `stationControlURL` 取连接参数 |
| 机器人电源 | `PUT .../nodes/{name}/robot-power` |
| Run 里重启驱动或断电重启 | `POST .../nodes/{name}/robot-restart`（上一节“重启”），进度读节点的 `rlark.io/power` 的 `restart` |
| 工位停用、维修或解除 | `PUT .../nodes/{name}/station-mark`，网关记录原因、操作者和时间。标记同时释放既有认领，拒绝新的认领和续租，直到解除标记 |
| 数据、模型 | 按名字引用的存储 |
| 令牌、密钥 | Job 密钥 |
| 设备层、Station 的镜像 | `s1u-robot` 的 `deviceImage`（Fleet 的 `fleet/robot`），随 RLark 的发布上线 |
