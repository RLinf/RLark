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
| station-gateway | 每台节点 | 按文档维护 WireGuard 接口、对端、路由、`/etc/hosts` 里的站点名字、给 kubelet 的 `resolv.conf`；每 10 s 协调一次 |

两个 DaemonSet，按节点标签 `station.rlark.io/network` 放置：

| DaemonSet | 标签 | 内容 |
|---|---|---|
| `station-agent-network` | `enabled` | agent + `wireguard` 容器（网关在 Pod 里） |
| `station-agent-host` | `host`（安装器装的节点） | agent + `gateway-program` 容器：把主机上的 `/opt/rlark/bin/station-gateway` 换成本镜像的版本（机器在用时不换，起不来换回），维护 `/etc/rlark/station/station-api.json` 和 CA。网关本身是主机服务 `rlark-station-gateway.service`，在 kubelet 之前起来 |

### 网络

- 拓扑：叶子拨枢纽，枢纽之间按需互联，转发枢纽替叶子转发。没在 `spec.nodes` 里列出的节点都是叶子。
- `sites[]`：`clusters`、`reaches`（能拨到哪些场站的 `Site` 端点）、`mesh`（本站枢纽之间建隧道）、`relay`、`leafUplink`（同型号叶子共用的上联，如机器人经 Orin）、`discovery`（枢纽在 `ranges` 和自己接着的每个私有 IPv4 网段里按 NodeClass 的出厂主机密钥找未入网的机器，比 /22 大的网段只扫自己地址所在的 /22，不含公网、`st-*` 隧道、容器和集群接口；每轮最多 128 个并发探测，三个 /22 最多约 25 s，每轮从上一轮开始算起隔 `intervalSeconds`；几台枢纽在同一网段时，机器跟着先报它的枢纽，那台三个周期没再看到它才换）、`dns`、`names`（本站的名字，如 `registry.rlark.internal`）。
- `nodes[]`：`role: Hub` 加 `endpoints`（`Public` 谁都能拨，`Site` 本站和 `reaches` 它的场站能拨）、`relay`、`nodeAddress`（别人找它用的地址不是隧道地址时，如云上 VPC 地址）、`advertise`（经它可达的网段）、`uplink`、`publicKey`（还连不上 Station API 的节点先声明公钥）。
- 有上联（`uplink` 或本站的 `leafUplink`）的节点，文档里的 `uplinkRoutes` 是它拨的每个端点和本站 `dns` 的每台域名服务器各一条经上联网关的 /32 路由；站点不写 `dns` 时网关把机器自己的 resolv.conf 交给 kubelet，机器人上那份没有域名服务器，所以机器人所在的站点要写 `dns`。
- `names`：全站同一地址的名字，如各集群的 `cp.<集群>.rlark.internal`、`rlark-server`、`gateway.rlark.internal`。网关写进每台节点的 `/etc/hosts`（行尾 `# rlark names`）。
- 连接规则：两台之间有隧道，当且仅当至少一台是枢纽且至少一方能拨到另一方；能拨的一方写端点和 25 s 保活；AllowedIPs 是对端的隧道地址加它的 `nodeAddress`、`advertise`，转发枢纽对叶子再加整个池。
- 默认 MTU 1380，接口 `st-wg0`，端口 51820。controller 每次算完检查任意两台之间有没有路，没有的列在 `status.unreachable`，条件 `FullyConnected` 为假。
- 网关只动自己的东西：只接管别名 `rlark-station-gateway/v1` 的接口；路由用协议号 242、metric 49321；对端逐个增删改，不动没变的会话。
- 网关取文档的顺序：agent 或自己从 Station API 取回的 → 私钥旁的 `last-applied.json` → 安装器写的静态文件 `/etc/rlark/station/bootstrap.json`（只有主机上的网关有，给不经隧道连不上 Station API 的节点）。有 `station-api.json` 时网关每 30 s 用自己的私钥向 Station API 证明身份取文档，所以 agent 不在时也能跟上控制面的变化。
- 文档版本 `documentLevel`（`astribot/station-gateway/main.go`、`astribot/station/network`）：给文档加字段时加一；发布要等所有网关报到新级别才换 controller（阶段 `controller` 的 `gateways-understand`）。

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

### station-forwarder

到 NAS 的 SMB 转发：三条 iptables 规则（两条 `FORWARD`、一条 SNAT），只放一个源到一个目标的一个端口，每 10 s 补齐。配置是 JSON（`input`、`output`、`source`、`destination`、`snat`、`port`），首次运行记下哈希（`/var/lib/rlark-station-forwarder/config.sha256`），之后只认同一份；程序从不删规则，改配置要先手删三条规则和这个文件。镜像不在发布里。

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
| `orin` | `s1u/orin/` | 经机器人内网维护 Orin：上网单元、相机 USB 规则和复位、adb 密钥、`orinPayloads` 载荷 |
| `power` | `s1u/device/power.py` | 电源对账（下面）；把占用镜像到宿主机文件 |
| `video` | `s1u/device/video*.py` | 相机经 MediaMTX 给浏览器（端口见下） |
| `clock` | `s1u/clock/` | 把 x86 和 Orin 对到站点时间源（`timeSource`） |
| `device`、`device-video`、`station` | `deviceImage`（Fleet 的镜像）的 `/device/start`、`--video`、`--station` | 设备层、它的相机服务、机器人上常驻的 Station。声明了 `deviceImage` 才有 |

和其他组件的接口：

| 东西 | 在哪 |
|---|---|
| 期望电源 | 节点标签 `rlark.io/robot-power` = `off`/`powered`/`ready`（没有按 `ready`），经 Gateway `PUT .../nodes/{name}/robot-power` 或控制台改 |
| 电源现状 | 标签 `rlark.io/power-state`（`off`/`powered`/`ready`/`estop`/`fault`/`unknown`），注解 `rlark.io/power` |
| 电源服务 | 本机 socket `/run/rlark/services/s1u-power.sock`（`GET /power`、`POST /power {action, force, reason}`、`POST /pose {pose, action}`：按住移动的初始姿势，网关 `POST nodes/{name}/robot-pose` 经 Server 的 `nodeproxy` 和该节点的节点 agent 转到这里） |
| 占用 | 节点标签 `rlark.io/robot-claim`；`power` 盯着自己的节点，一秒内写到 `/run/rlark/s1u/claim`（`{"holder"}`，没人持有时没有文件） |
| 在用 | `/run/rlark/in-use/<组件>`：Station 服务 Run 时写 `fleet-station`，设备层有会话时写 `s1u-device`；上电、驱动在跑不算在用 |
| 维护 | 租约 `/run/rlark/s1u/maintenance`；标签 `rlark.io/maintenance-state`，注解 `rlark.io/maintenance` |
| 自检 | 设备层写 `/run/rlark/s1u/self-check.json`；标签 `rlark.io/self-check-state`，注解 `rlark.io/self-check` |
| 机器人是谁 | tuner 从标签 `rlark.io/robot-id`、`rlark.io/robot-body` 写 `/run/rlark/s1u/robot.json` |
| 规格和机体 | `/etc/rlark/s1u/node_profile.env`、`bodies.json` |
| Orin 主机密钥 | 标签 `s1u.rlark.io/orin-state`；密钥变了由人把注解 `s1u.rlark.io/orin-host-key-confirm` 设成新指纹 |
| 重启机器人 | 节点标签 `rlark.io/restart-request` 设一个新值（同一个值只做一次），作为一次维护做；结果在注解 `rlark.io/restart-request-state` |
| 无人下电 | 参数 `powerOff`：`unattended`（默认）tuner 自己下电再重启；`attended` 电机带电的要等人在 RLark 上下电 |
| 分批 | 维护按节点标签 `rlark.io/rollout-wave`（整数，没有的是最后一批）分批，同时进行的台数不超过参数 `rolloutParallel`；进度在标签 `rlark.io/rollout-state`、注解 `rlark.io/rollout` |

端口（都在主机网络上，避开 MediaMTX 默认端口和 8890）：

| 端口 | 绑定 | 做什么 |
|---|---|---|
| 18889/tcp | 所有地址 | MediaMTX 给浏览器的 WebRTC，地址随观看会话下发 |
| 18189/udp、tcp | 所有地址 | WebRTC ICE |
| 18890/tcp | 127.0.0.1 | MediaMTX 的鉴权回调（观看会话的令牌） |
| 18554/tcp | 127.0.0.1 | RTSP，相机推流 |
| 19997/tcp | 127.0.0.1 | MediaMTX 的 API |

电源和视频的控制只走 `/run/rlark/services/s1u-{power,video}.sock`。

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
| 数据、模型 | 按名字引用的存储 |
| 令牌、密钥 | Job 密钥 |
| 设备层、Station 的镜像 | `s1u-robot` 的 `deviceImage`（Fleet 的 `fleet/robot`），随 RLark 的发布上线 |
