# 发布计划 fix-0126（liangcai/dev 合进 main，robot-restart、Station 名、站点时间、灯桥）

现网是 `ci-cb84c00-e60a306-portal`（liangcai/dev f4fc28d 的树，少了 main 的 NAS 转发、Station 标记、station-logs、灯桥载荷和 Fleet 8762eca 设备镜像）。这次从分叉 main（合并后的 fix-0126）重新发布，按 [release.md](../docs/release.md) 的“快速上线”：只构建改了的镜像，发布文件由 `ci-cb84c00-e60a306-portal` 替换摘要得到。合并把站点网络文档升到第 4 级（两边各自的第 3 级合在一起），所以分**两次 apply**（release.md“提高文档级别的改动”）。

下面 `<R>` 是这次 RLark 提交（40 位），`<r7>` 它的前 7 位；`<F>` 是包 D 并入后的 Fleet main 提交，`<f7>` 前 7 位。

## 1. 要构建的镜像（只这些）

| 镜像（`images` 的行） | 组件 | 为什么 |
|---|---|---|
| `rlark/agent`（`astribot/release/rlark.Dockerfile`） | `rlark-server`、`rlark-controller-manager`、`rlark-gateway`、`cluster-agent`、`node-agent`、`station-api` 换成同一个新摘要 | Gateway：`POST nodes/{name}/robot-restart`、`PUT nodes/{name}/station`、main 的 `station-mark`/`station-logs`；agent：`rlark.io/robot-restart` 注解带到成员集群、`rlark.io/station` 归管理面；Addon 目录（s1u-robot v1.7.2）编译在里面 |
| `rlark/station-wireguard`（`astribot/station-gateway/Dockerfile`） | `station-wireguard` | 文档第 4 级（转发 + 发现两边的字段）；每台网关在隧道地址的 udp/123 给出本机时间（Hub 的就是站点时间） |
| `rlark/station-controller`（`astribot/station/Dockerfile`） | `station-controller` | 写第 4 级文档（含 main 的 NAS 转发）；NodeClass 给机器人分配 `rlark.io/station`。**只在第二次 apply 换** |
| `rlark/s1u-robot`（`astribot/s1u/Dockerfile`） | `s1u-robot` | power.py 执行重启（C1）；clock 只在没认领、没在用、驱动停时跳；tuner 的 `clock` 维护 |
| Fleet `fleet/robot`（包 D 构建，Fleet 的 `scripts/deploy/rlark/Dockerfile.robot`） | `fleet-robot`（`deviceImage` 和 Station） | 包 D 的设备层（C3 灯态、C5、C7、in-use 标记写明 Run） |
| Fleet `fleet/lamp-bridge`（包 D 构建，`Dockerfile.lamp-bridge`） | `lamp-bridge` | 灯桥读设备层 `lamp.{color, blink, period_ms, on_ratio}`，按这个节奏驱动灯带 |

不构建、沿用现网摘要：`rlark-ui`（`localhost:5555/rlark-ui@sha256:551deabf…`：liangcai 管理机工作区未提交的控制台改动构建的；这棵树里提交了的那部分 `npx tsc -b` 有 4 处错误，要等他提交那些文件后再从 main 构建）、`astribot-iam`（`localhost:5555/astribot-iam@sha256:82f5fd55…`，IAM 代码没改）、`station-agent`、`tools`、`quest-bridge`、`installer-live`（`astribot/station/bootstrap/` 没改）及 `pinned.yaml` 里的镜像。

构建（管理机，tmux 里；`images` 那一行的 BuildKit 参数照抄）：按 release.md 快速上线第 2 步，各镜像推进 `127.0.0.1:5443`，记下摘要 `<AGENT>`、`<WG>`、`<CTRL>`、`<S1U>`。包 D 的两个 Fleet 镜像按 Fleet 自己的构建推进金山云 KS3 仓库后，按摘要拷进办公室仓库（`rlarkctl release copy` 或 `run.sh` 第 5 步的做法），记下 `<FLEET_ROBOT>`、`<LAMP>`；两边仓库各放一份，不跨仓库拉。

## 2. 发布文件

树和 rlarkctl（release.md 快速上线第 3、4 步，`W=/var/lib/rlark/release/builds/ci-<r7>-<f7>`）：

```bash
W=/var/lib/rlark/release/builds/ci-<r7>-<f7>
ssh cailiang_rlark sudo mkdir -p $W/rlark
git -C rlark archive <R> | ssh cailiang_rlark sudo tee $W/rlark.tar >/dev/null
ssh cailiang_rlark sudo tar -C $W/rlark -xf $W/rlark.tar
# 按 <R> 编译 rlarkctl（Addon 目录在里面：s1u-robot v1.7.2），拷到 $W/rlarkctl
```

第一次（不换 `station-controller`），在 `$W/rlark/astribot/release` 下：

```bash
$W/rlarkctl release candidate --base releases/ci-cb84c00-e60a306-portal.yaml --tree $W/rlark.tar \
  --name ci-<r7>-<f7> --fleet <F> \
  --image rlark-server=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image rlark-controller-manager=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image rlark-gateway=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image cluster-agent=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image node-agent=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image station-api=registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> \
  --image station-wireguard=registry.rlark.internal:5443/rlark/station-wireguard@sha256:<WG> \
  --image s1u-robot=registry.rlark.internal:5443/rlark/s1u-robot@sha256:<S1U> \
  --image fleet-robot=registry.rlark.internal:5443/fleet/robot@sha256:<FLEET_ROBOT> \
  --image lamp-bridge=registry.rlark.internal:5443/fleet/lamp-bridge@sha256:<LAMP> \
  --image astribot-iam=localhost:5555/astribot-iam@sha256:82f5fd5566228741825f34a1c34707bb0983caeb2fd1fdee32e048aeb4e2c56c
$W/rlarkctl release copy -e environments/live.yaml --to source \
  registry.rlark.internal:5443/rlark/agent@sha256:<AGENT> registry.rlark.internal:5443/rlark/station-wireguard@sha256:<WG> \
  registry.rlark.internal:5443/rlark/s1u-robot@sha256:<S1U>
```

（`astribot-iam` 不在 `release.Components` 里，不给 `--image` 时 candidate 会把它从发布里去掉，所以原样给一次。）

然后在生成的 `releases/ci-<r7>-<f7>.yaml` 里把 Fleet 的载荷声明加回去（`ci-cb84c00-e60a306` 丢掉了它，`orin.py` 因此从三台 Orin 上卸了灯桥）：

```yaml
  s1u-robot:
    values:
      orinPayloads: quest-bridge={{images.quest-bridge}},lamp-bridge={{images.lamp-bridge}}
  site-registry:
    values:
      prewarm: '{{images.s1u-robot}},{{images.fleet-robot}},{{images.quest-bridge}},{{images.lamp-bridge}}'
```

核对生成的文件：`s1u-robot` 是 `v1.7.2`、`timeSource: time.rlark.internal`（`pinned.yaml` 带来的）、`deviceImage: '{{images.fleet-robot}}'`；`station-controller` 仍是现网的 `…@sha256:09ffdbec…`；`rlark-ui` 仍是 `551deabf…`；`source.rlark` 是 `<R>`。

第二次（只多 `station-controller`），等第一次的网关都报到第 4 级后：

```bash
$W/rlarkctl release candidate --base releases/ci-<r7>-<f7>.yaml --tree $W/rlark.tar \
  --name ci-<r7>-<f7>-controller --fleet <F> \
  --image station-controller=registry.rlark.internal:5443/rlark/station-controller@sha256:<CTRL> \
  --image astribot-iam=localhost:5555/astribot-iam@sha256:82f5fd5566228741825f34a1c34707bb0983caeb2fd1fdee32e048aeb4e2c56c
$W/rlarkctl release copy -e environments/live.yaml --to source registry.rlark.internal:5443/rlark/station-controller@sha256:<CTRL>
```

## 3. 上线前：站点时间的名字

机器人的 `timeSource` 是名字 `time.rlark.internal`，每个站点在 SiteNetwork 的 `sites[].names` 里把它指到本站 Hub 的隧道地址（实验室机器人所在的 `lab` 站点 → 办公室 Hub `cailiang` 的 `10.254.240.1`；搬到数采场时指到那里的 Hub）。改 RLark 自己的 SiteNetwork 对象（管理面，不是成员集群），在第一次 apply 之前，这样 v1.7.2 的 `clock` 容器一起来就量得到。`clock` 容器把机器自己的 `/etc/hosts`（网关就地改写，行尾 `# rlark names`）挂成自己的 `/etc/hosts`，不用 kubelet 在 Pod 起时拷的那份，所以以后 Hub 换地址（搬数采场）只改 SiteNetwork 的 `names`，下一次测量就跟上，不用重启 s1u-robot 的 Pod：

```bash
# 管理机，root；kubeconfig 是 live.yaml 的 management.kubeconfig（发布身份）
K=/home/cailiang/.local/share/rlark-runtime/identity/rlark-release/kubeconfig
kubectl --kubeconfig $K -n station-system get sitenetwork default -o jsonpath='{range .spec.sites[*]}{.name}{"\t"}{.names}{"\n"}{end}'
# 在 lab 站点的 names 里加一项（站点下标按上一条的输出）
kubectl --kubeconfig $K -n station-system patch sitenetwork default --type=json \
  -p '[{"op":"add","path":"/spec/sites/<lab 的下标>/names/-","value":{"name":"time.rlark.internal","address":"10.254.240.1"}}]'
```

（`lab` 还没有 `names` 时 `path` 用 `/spec/sites/<下标>/names`、`value` 写成一项的列表。）读回：NetworkEnrollment 里机器人文档的 `names` 有这一项（网关写进机器人 `/etc/hosts`，行尾 `# rlark names`）；第一次 apply 后三台节点的 `rlark.io/clock-state` 从 `no_source` 变成 `slewing`/`locked`（还是 `unreachable` 就是名字或 Hub 的 udp/123 不通）。

## 4. 上线

管理机，root，`$W/rlark/astribot/release` 下：

```bash
$W/rlarkctl release apply -e environments/live.yaml -r releases/ci-<r7>-<f7>.yaml --tree $W/rlark.tar --dry-run
$W/rlarkctl release apply -e environments/live.yaml -r releases/ci-<r7>-<f7>.yaml --tree $W/rlark.tar
```

dry-run 里应当看到：`gateway`、`station-api`、`cluster-agents`、`node-agents`、`server` 换 `<AGENT>`；`controller` 无变化；`station-agents` 换 `<WG>`，模板差异只有网关容器多 `NET_BIND_SERVICE`；`console` 无变化；`addons` 里 `s1u-robot` v1.7.1 → v1.7.2（`deviceImage`、`image`、`orinPayloads` 加回 lamp-bridge、`timeSource`）、`site-registry` 的 `prewarm`。不应出现 `rlark-ui`、`astribot-iam` 的变化。

第一次之后等所有有文档的成员的网关报到第 4 级（NetworkEnrollment 的 `status.observation.gateway.documentLevel`），再：

```bash
$W/rlarkctl release apply -e environments/live.yaml -r releases/ci-<r7>-<f7>-controller.yaml --tree $W/rlark.tar --dry-run
$W/rlarkctl release apply -e environments/live.yaml -r releases/ci-<r7>-<f7>-controller.yaml --tree $W/rlark.tar
```

两次的日志最后一行都是 `release <名字>: every stage done`。上线后把两个发布文件拷回工作站，提交到分叉 main，并把 `environments/live.yaml` 的 `release:` 指向 `ci-<r7>-<f7>-controller.yaml`。以后的发布都从分叉 main 切，不从个人分支（这次的起因）。

## 5. 现有三台机器人的 Station 名

第二次 apply 换上 station-controller 后，NodeClass 的协调给每台**带 NodeClass 准入**、还没有 `rlark.io/station` 的机器人按节点号分配：`s1u-01-x86 → S01`、`s1u-02-x86 → S02`、`s1u-03-x86 → S03`，写进 NodeAdmission 的机器自有标签，再写到 RLark 节点上，五分钟内（准入一变就立即）生效，不用人做。

手工准入（没有 `spec.nodeClass`）的机器人不归 NodeClass 管，用 Gateway 设一次（operator 令牌；准入记录和节点一起改）：

```bash
curl -X PUT "$GATEWAY/api/v1/rlinf.io/v1alpha1/nodes/s1u-01-x86/station?namespace=<节点的命名空间>" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"station":"S01"}'
# s1u-02-x86 → S02，s1u-03-x86 → S03 同样
```

先读 `GET /api/v1/station/node-admissions?cluster_id=rlark-robot-s1u-01` 看三台准入有没有 `nodeClass`，有就什么都不做。读回：`GET /api/v1/rlinf.io/v1alpha1/nodes` 里三台的 `metadata.labels["rlark.io/station"]`，以及 Fleet `/api/hg/site/robots` 的 `station_name`。

## 6. 读回和验收

- 控制面：`docker ps` 里 gateway、server、controller-manager 是 `<AGENT>`；rlark-ui、astribot-iam 没变；`http://10.11.14.128/` 经 IAM 登录正常。
- 接口：`GET /api/hg/robots/s1u_0{1,2,3}/station-logs`（Dashboard）回 200，不再是 `RLark 404`；`swagger` 里有 `robot-restart`、`station`、`station-mark`、`station-logs`。
- 网络：第二次之后 NetworkEnrollment 的文档级别 4，`network-connected`；196 经 cailiang 的转发挂 NAS 正常。
- 机器人（`rlarkctl release kubectl -e environments/live.yaml --cluster rlark-robot-s1u-01 -- …`，只读）：DaemonSet `rlark-system/s1u-robot` 的 `image`、`deviceImage` 是新摘要，Pod 全就绪、0 重启；节点注解 `s1u.rlark.io/orin` 的 payloads 有 `lamp-bridge`；`curl http://10.254.240.{3,7,8}:33334/api/lamp` 回 200 且带 `period_ms`、`on_ratio`；现场有人看灯带颜色和闪法与 Dashboard 一致。
- 时钟：三台节点 `rlark.io/clock-state=locked`，注解 `rlark.io/clock-report` 的 `offset_ms` 在 ±50 内；偏差大的空闲机器人在 `rlark.io/maintenance` 里出现一次 `kind: clock`，之后回到 `none`、驱动回到 ready。
- 重启（C1，要 Fleet 包 S 的 Runner 已上线、机器人在 Run 里）：Dashboard 上对一台点“② 重启驱动”，节点注解 `rlark.io/power.restart` 依次 `lowering → raising → done`，手臂缓降再起；期望电源标签 `rlark.io/robot-power` 不变。别的 Run 的 holder 发请求回 409。
- s1u_03 现在的期望电源是 `powered`（Runner 命令 20 写的）：这次 Run 结束前后由协调者经 `PUT nodes/s1u-03-x86/robot-power {desired: ready}` 改回 `ready`，否则 Run 结束后 RLark 会自己停它的驱动。这是电源动作，由协调者定时机。

## 7. 回退

`rlarkctl release apply -e environments/live.yaml --to /var/lib/rlark/release/runs/<时间>-before.json --back`。第二次 apply 之后回退到第一次即可；回退过第一次会回到第 3 级的 liangcai 网关，此时控制器若已是第 4 级会被门挡住，先退控制器（第二次的 before）。

## 8. 实际上线（2026-11-26）

两次都已 `every stage done`，现网记录 `rlark-system/rlark-release-current` = `ci-49580ee-64864d2-controller`。发布文件只比 §2 多了 §2 要求的 `orinPayloads`/`prewarm` 加回 lamp-bridge，阶段顺序没改。经过：

- 第一次停在 `controller` 的门 `gateways-understand: 4`（run 20261126-040209）。不是顺序错：第一次里 `controller` 没有变化，门本来不查；但 SiteNetwork 在那次运行取基线之后才加了 `time.rlark.internal`，控制器（旧的）重写了机器人的文档，`documents-unchanged` 对旧基线不成立，阶段走“as stated but not settled: done again”，于是去查门、停下，什么也没改。同一命令再跑一次（新基线带新文档）即过。给 RLark 的要求：没有变化、只是没稳定的阶段不该再查门（门管的是“写新级别的东西”）。
- 第二次换了 `station-controller` 后，新控制器按第 4 级重写 cailiang、wendychen、alexfu 的文档，同一次运行里 `station-agents` 的 `documents-unchanged` 对基线必然不成立，只能等 30 分钟超时。停掉该次（只在等，0 处改动），同一命令再跑：0 changes，写记录。给 RLark 的要求：`controller` 阶段换了控制器时，后面阶段的 `documents-unchanged` 用 `controller` 之后的读数做基线。
- 管理机 cailiang 盘余量在 kubelet 的 imagefs 15% 门槛上下（liangcai 同时在机器上构建 e2e 镜像），DiskPressure 反复：驱逐了 Dashboard（jo-f706800a9a904c84）、它的数据库和 package-proxy；`station-agent-network` 在 cailiang 的拉取 Pod 被拒（`Evicted`），而 rollout 对一个 Failed 的拉取 Pod 永远等它（`apps/rlark/astribot/rollout/update.go` `pulled`），整个办公室集群的网关滚动卡住。处理：压力解除后删掉那个 Evicted 的拉取 Pod，agent 重建后拉取成功；`docker image prune --filter until=168h`（只删超过 7 天、无标签、无容器引用的镜像）腾出约 90 GB。给 RLark 的要求：拉取 Pod 失败（Evicted/Failed）时重建，并带 DaemonSet 的 priorityClass；管理机要有盘的清理/告警。
- 运行中 liangcai 把 Addon `s1u-robot` 用 `kubectl replace` 改成 `v1.7.1-timing-8b84e07a8b72`（相机时序预览，ConfigMap `s1u-camera-timing-addon-8b84e07a8b72`），被这次 `addons` 阶段按用户“直接覆盖发布”换成 v1.7.2。原对象备份在管理机 `/var/lib/rlark/backup/liangcai-override-20261126/`。
