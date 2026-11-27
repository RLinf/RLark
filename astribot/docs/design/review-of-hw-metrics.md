# 节点硬件指标方案交叉评审

范围：只读审阅 `/tmp/design-hw-metrics.md`，并与 `/tmp/design-robot-body.md` 对照当前仓库。未修改代码。

## 确认正确的关键假设

1. **Addon 按不可变 name/version 分发的概念符合现有实现。** `apps/rlark/pkg/addons/astribot_catalog.go` 定义 control-plane catalog，`CatalogDirEnv = RLARK_ADDON_CATALOG_DIR` 可加载外部目录。注释说明 agent 从已发布到集群的 Package 渲染 Addon。Catalog/发布逻辑按 name 和 version 管理；同一已发布版本内容变化会返回 `ErrPublishedOtherwise`，必须发新版本。控制面是 package 来源，目标集群使用持久化 package，因此 Gateway/发布动作必须在写 Addon 前把 package 发布到对应集群。这支持“Addon 清单按包版本发布”，不意味着必须把新包放进某个特定源目录。

2. **不能让指标 Addon 隐式安装 NVIDIA Toolkit 的约束完全正确，且原因比 proposal 写得更强。** 现有 `apps/rlark/pkg/addons/catalog/nvidia-gpu/manifests/runtime.yaml` 的 runtime DaemonSet 使用 `hostPID: true`、`privileged: true` 和宿主根目录 hostPath；会安装 toolkit，修改 kubelet 所用 containerd 配置，并在配置变化时重启运行时。其 `device-plugin.yaml` 依赖这套 runtime。指标 Addon 若通过隐式依赖启用该 Addon，就会扩大权限并可能触发 runtime 重启，影响 GPU 节点现有容器。proposal 把两个 Addon 独立启停、驱动/运行时不匹配时显示 unavailable 的原则正确。现有 NVIDIA Addon 不能误描述成“无特权”或“无副作用安装”。

3. **Gateway 已有按注册集群代理 Kubernetes API 的能力，Prometheus Service 查询有现实落点。** `apps/rlark/pkg/gateway/kube_proxy.go` 通过 Server 的 `/api/proxy/{agentID}/api/kubernetes/{path}` 向对应 agent 的 Kubernetes API 发送请求。`astribot_robot_camera_handler.go` 已用此能力列出目标集群内 Pod，再走 Kubernetes Service/Pod proxy 取数据。集群作用域有 `stationNamespace` 注册映射解析，site nodes 的 `clusterNamespace` 也以注册 Cluster 为准。因此可以通过已验证的集群 API Server Service-proxy 到固定 Prometheus ClusterIP Service，而不需要把 Prometheus 暴露成 NodePort/公网 endpoint。现有 Gateway 并没有面向 Prometheus 的通用查询或任意服务 HTTP 反向代理，需新增 Astribot handler；将用户输入转成任意 PromQL 或任意 Service URL 仍是 SSRF/越权路径，proposal 使用固定 metric ID、固定查询模板、集群限定和大小/时限上限是正确边界。除非目标集群代理在实际环境无法访问 Service proxy API，否则没有证据说明必须加 metrics-query Addon。

4. **metrics-server 不等同于硬件指标或历史存储，proposal 的定位正确。** Node/Pod API 的 capacity、allocatable、requests 和 Prometheus exporter 的使用率、温度、磁盘 I/O 是不同数据源与语义。proposal 将它们分开呈现，并不把时序塞进 Node status，是合理设计。

5. **指标故障与 Ready、heartbeat、调度解耦是必要条件。** 采集失效、Prometheus/PVC unavailable 或查询超时应该只影响硬件观察区，不能阻塞 Node readiness 或 Addon 业务 rollout。GPU exporter 字段须按卡型、驱动及 runtime 声明 availability，不能因 3090 缺某项 telemetry 伪造 0。

## 发现的问题（按严重度）

### 严重：catalog 文件路径违反明确的 Astribot 所有权边界

proposal 将 Addon 定义放在 `apps/rlark/pkg/addons/catalog/*`。该目录属于 RLark 原生 Addon control plane，不属于允许的 Astribot-owned 路径。仓库现有文件包括 catalog reader/publisher、内建目录扫描与嵌入行为，修改它会越过本任务及后续实现边界。

- Astribot-owned 源定义建议放 `astribot/addons/catalog/{node-exporter,dcgm-exporter,jetson-tegra-exporter,prometheus}/`，另配只在 `astribot/**` 内的校验/发布说明或脚本。
- 现有 catalog reader 明确支持 `RLARK_ADDON_CATALOG_DIR` 外置动态目录。发布系统可把上述 Astribot 源包 stage 到 Gateway 已配置的外置 catalog 目录，再用现有 immutable Package publish 流程投放各集群。该 staging/mount 是否已存在必须作为实现前置条件验证。
- 若部署尚未把 Astribot 目录提供给该变量，proposal 需要明确交付 mechanism、所有者和 rollback。不得因此把素材复制或 symlink 到原生 `apps/rlark/pkg/addons/catalog`、改其 Go embed、改 RLark 原生 deployment。若现有外围发布链无法在 Astribot-owned 范围内完成 staging，应先协调平台发布/运维，不要偷偷扩展文件范围。
- `apps/rlark/pkg/addons/catalog/README.md` 的开发说明仍描述编译期自动 embed，但当前 `astribot_catalog.go` 也支持 CatalogDirEnv；不可依赖 README 推断外置包未支持。新方案应描述使用外置目录与现有 Publish API 的真实路径。

### 高：自动启用 host exporter 的权限、节点覆盖和升级策略仍需变成硬验收

proposal 描述了只读挂载与最小权限，但清单仍把 node-exporter 安装到 DaemonSet 且“自动到 GPU/普通节点”的实现略过了边界细节。

- node-exporter 暴露主机 filesystem mountpoints、设备、网络接口等主机元数据。NetworkPolicy/Service 范围、read-only rootfs、drop capabilities、seccomp、非 root、hostNetwork/hostPID 设为 false（若具体 collector 不要求）、逐项只读 hostPath 以及 exporter collector allow-list 应成为安全验收。默认关闭 textfile collector 和可写 hostPath。
- exporter Service 仅允许 Prometheus 所在 namespace/Pod 抓取；默认不得允许任意 Pod 访问，也不得借 Service NodePort 暴露。Gateway 仅管理员路由返回经过整理的字段，不透传原始 metrics 文本。
- Addon 调度不得吞掉 taints 选择、抢占机器人工作负载或跑到 Orin 的关键资源节点。为 Prometheus 指定监控承载节点或管理员配置 selector；没有可承载节点时显示 Addon Pending/Unavailable，不因为强行调度损害机器人。DaemonSet rollout 使用有限 `maxUnavailable`/温和 priority，并对 exporter 设置 CPU/内存限制、探针、镜像 digest 和升级兼容策略。避免 `system-node-critical`。
- DCGM 容器对 NVIDIA 设备、driver libraries、host engine 的实际最小挂载/权限随部署模式而异；需要在 3090 + 当前 driver canary 验证。不要仅凭“不默认 privileged”断言运行可行。若必须 privileged 或扩大设备访问，作为独立安全评审门禁，而不是自动 fallback。
- Jetson tegrastats exporter 需验证只读执行入口、proc/sysfs/device 访问与 L4T 版本。不要让解析或设备读取权限失败令 Node 不 Ready。

### 中：7–14 天与 v1 1–3 天的保留目标互相矛盾，且缺少容量实测

proposal 选型部分建议先 7 天并考虑 14 天，v1 又允许 1–3 天甚至仅 instant query。应选一个发布承诺。对于节点级短期当前值和近期故障排查，1–3 天足以观察周内局部波动和定位近期问题；不够做跨周趋势、周末问题追溯或周期性运维回顾。若产品目标只是节点详情 current snapshot，initial v1 可只提供 instant query 加当前采样时间，不承诺历史；若承诺短曲线，选择 1–3 天并把 retention time 与 retention size 双限额写入 Addon defaults。7–14 天应等到真实采集后容量压测再决定。

容量不可只用“单副本/有 PVC”论证。以 15 秒 scrape 粗估，每个 active series 每日 5,760 samples。Prometheus sample 本体常见约 1–2 bytes/sample 的粗略估算法，另有 WAL、head、chunks、index、compaction 空间和峰值；实际需用目标 collector 的真实 series 数压测。若每节点 exporter 总计约 500–1,500 series，原始 sample 约 4–17 MB/节点/日，TSDB 实际占用会更高；GPU/容器 labels 和多个卡会增加 series。Prometheus head 的 active-series 内存也随 series 数增长，不能只算 PVC。

- office control-plane/workstation 是已有负载节点，安装 Prometheus StatefulSet 不应默认调度在 cailiang 上或占用关键 RLark workload 资源。
- 小型 robot cluster/Orin 的内存和本机盘可能紧张；在未采集现场 node allocatable、现有 Pod requests、PVC provisioner/local disk 和磁盘余量前，不能断言“每个集群一个 Prometheus StatefulSet + PVC”普遍现实。
- 给出监控节点约束、request/limit、PVC 初始 size、保留到何种目标、空间告警阈值与超限行为。PVC 满时 Prometheus 会停止/失败写数据，需说明这是观测降级而不是核心节点故障。
- 每集群副本会乘以 Prometheus 基础内存、WAL 和 PVC 下限；对多数小集群，仅为详情 current query 运行完整 TSDB 是否值得，需经节点资源测量确认。proposal 可把监控 Addon 默认关闭或按需启用作为初始发布控制。

### 中：指标命名与时间 API 尚缺可执行契约

proposal 的 hardware DTO 混用 inventory、snapshot、capacity、rate、accelerator telemetry，需要各字段明确来源、单位、时间戳、stale 判定和缺失语义。K8s data 与 Prometheus 时间点可能有延迟，不能合成看似同一时刻的一张“当前值”。range API 要定义允许的 metric ID、跨度、step 与最大点数，并为 1–3 天或更长 retention 提供兼容限制。不要把 user-supplied node_uid 当授权凭据，先通过 cluster namespace 中当前 Node UID 校验，按 Prom label 的 UID 过滤，处理重装同名节点。无 Node UID 或 stale target 应给 Unknown/Unavailable，不返回另一代节点历史。

### 低：单副本 Gateway 与 Prometheus 单副本的运维失败语义可以再明确

单副本 Compose Gateway 是唯一代理和 auth 门。它重启不会影响 TSDB，但整个 UI 指标查询暂时失败。Prometheus 每集群单副本失效仅影响该集群指标，不可由其他集群 query fallback 或共享数据。应分别暴露 QueryUnavailable、TargetDown、NoSamples、Stale、FieldUnsupported 等机器可读状态，不把 Prometheus text/error body显示给用户。JWT/admin 之外仍需按集群注册关系与当前用户作用域做服务端授权。

## 与本体的冲突与共享改动

两案均预期改 Astribot UI 的：

- `apps/rlark-ui/src/astribot/api.ts`：本体需要 RobotModel detail 和 robot-body 对照 DTO；硬件需要 inventory/current snapshot 与可选 range client。接口路径不直接冲突，但 RobotModel list 当前 item 应保持轻量，不应为列表膨胀出本体完整定义。建议独立 `robotModelsApi.get(name)`、`siteNodesApi.robotBody(clusterID,nodeName)`、`nodeHardwareApi.current/range(...)`；复用同一个受控请求/取消约定。
- `apps/rlark-ui/src/astribot/pages/StationDetailPanel.tsx`：本体方案需 expected-vs-observed 关节/末端对照和 `RobotBodyVerified`；硬件方案需 CPU/内存/磁盘/网络/accelerator cards。两个改动会同时扩展现有 detail columns/card layout、loading/error/stale state、暗色样式，若平行改文件有高 merge/UI 空间冲突。建议先共同确定一个「机器人硬件与本体」detail section 及分区，然后由一个 owner 一次合并，不要引入两套重复 refresh/query state。
- `StationCard.tsx`：本体方案在卡片上加一枚一致性摘要 badge；硬件指标通常应留在展开详情，不把动态 CPU/GPU 数塞进主卡片，以免 5 秒重绘和卡片网格拥挤。两案如都需 card 修改，只共享 identity/body status badge，hardware snapshot 不放卡面。
- 两案新增 mock fixtures/source tests 可能共用 `apps/rlark-ui/src/astribot/mockBackend.ts`、`astribotMockRoute` 与同一页 source test。统一数据模型必须包含同一 clusterID/nodeName/nodeUID/model，测试 loading/error/empty/stale 不串数据。
- 本体 Gateway 路由拟为 `/api/v1/site-nodes/:name/robot-body?cluster_id=`，硬件拟为 `/api/v1/station/nodes/:name/hardware?cluster_id=`。路由独立，但两者都必须先用同一个已注册 Cluster namespace/Node UID resolver 授权和取址，避免重复实现越权路径。接口共享 `RobotModel.metadata.uid` / name、NetworkEnrollment 当前 NodeUID、`ObservedAt` / freshness DTO。不应把 Prometheus label query 或本体对照走到不一致的 cluster namespace。
- 本体需要按不一致/证据不足区分的语义状态，硬件要标注各指标 source/time/stale。共享 UI helper 可处理 freshness 展示，但状态机、指标可用性与身体一致性不能合并成一个 badge。

## Live 资源评估

按题目给定拓扑，管理面 Docker Compose 为单副本；Prometheus 在各成员集群内，Gateway query 经各自已注册 cluster 的 agent/Kubernetes API proxy。这样可避免集中 WAN scrape，且 management compose 重启只导致查询短暂不可用；它不会给管理面提供 HA。Gateway 不应将 Prometheus 暴露到公网，也不应在管理机部署单一 Prometheus 去 scrape 所有站点。

每集群 Prometheus StatefulSet + PVC 的资源压力对不同站点并不等价：

- office 中 cailiang 同时是管理机/control-plane/station hub。若 office member cluster 承载真实 workloads，把 Prometheus 调度到该节点可能和 Gateway/控制面争内存、CPU、盘 I/O；不得默认绑定控制面/hub，也不得靠极高 priority 抢占。
- robot cluster 的 Orin/x86 是产品负载节点；node-exporter 是较轻的 DaemonSet，但 Prometheus 本身和 PVC 应尽可能调度/存储在有余量的节点。若 cluster 只有单台 Orin，或本地盘空间很小，每个集群一份 TSDB 可能不现实。没有已知 allocatable、现有 requests、实际 free disk/PVC backend 和 collector series 数时，评审不能给“安全可部署”的定量结论。
- 建议 v1 从 1 天 retention 起步，size cap 先按实测三天数据量加 WAL/head/compaction 空间余量配置，先观察至少一个完整 scrape/重启周期再提高到 3 天。1–3 天适合近期故障排查和短时峰值回看，不够跨周排查。当前值 instant-query 仍应保留采样时间与 stale 界限，不因 Prometheus query 成功就称为新鲜。
- 用固定抓取周期和真实启用的 exporter 实测 `active_series × samples_per_day × sample_bytes`，再加 WAL/head、index、compaction 峰值；记录 Prometheus RSS/working-set、PVC IOPS、增长速率、PVC 剩余和每目标样本数。用 1 节点 office canary、NVIDIA canary、Orin canary分别测，不能从普通 x86 推断 Jetson。
- 如果单节点/集群没有可承载 Prometheus 的资源，manifest 应避免硬性强制安装。将监控 Addon 的启用条件、Pending/Unavailable 解释及不影响现有 workloads 列为发布 gate。不能以无数据为由让 station readiness 或机器人驱动变红。

## 总评与建议定稿改动

A 的技术方向基本成立，关键前置修正为：将 catalog source 放 Astribot-owned 路径并接到现有外置 `CatalogDirEnv` / package publish 流程；用已有 agent Kubernetes API proxy + Service proxy 作为默认路由方案并给 metrics-query Addon 留作代理受限时备选；保留 NVIDIA Addon 是显式前置、绝不依赖触发安装；v1 retention 收敛为 current-only 或 1–3 天且按实测 size cap；增加 office/Orin placement/capacity gate、采集权限/rollout rollback 矩阵；与 RobotModel 本体在 StationDetailPanel 建一个共享数据和 freshness section。未确认 Prometheus Service proxy 在每种部署/kcp 注册模式都能通前，应把该验证作为实现前 gate，而不是声称现状已覆盖 Prometheus。
