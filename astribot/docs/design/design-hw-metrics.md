# RLark / Astribot 节点硬件详情指标方案

## 选型结论

**推荐 A 为主链路：集群内 Prometheus + Addon 管理 exporter + Gateway 受限查询代理。** Prometheus 为单副本、有界本地 TSDB，不把时间序列写进 Node CR 或 NetworkEnrollment observation。先保守设置 retention 和磁盘上限，按序列数、节点数和抓取周期压测。B 只作为 Prometheus 不可用时可选的“最后一次摘要”兜底，不作为第二套完整指标存储。UI 显示来源、采样时间和 stale 状态。metrics-server 不承担硬件历史指标。

- node-exporter 覆盖 Linux CPU、内存、文件系统、磁盘 I/O、网络接口。容器部署需配置 path.rootfs，并过滤虚拟接口、容器挂载和不展示的 block devices。
- DCGM Exporter 可暴露 GPU 利用率、显存、温度、功耗、时钟、ECC 等，字段依赖 GPU、驱动和权限。NVIDIA 当前文档列出 GeForce/Quadro，但说明非数据中心卡只有有限 DCGM 功能。不能保证每卡都提供温度、功率或序列号。
- Jetson/Orin 不应假设 DCGM 可用。以 Jetson 官方 tegrastats 为数据源，可做只读 exporter 解析 GR3D_FREQ、RAM、温度、VDD_GPU 等；格式和字段随 SoC/JetPack 变化，目标 Orin/L4T 要验收。DCGM 不作为 Jetson 通用采集器。
- metrics-server 只提供 Kubernetes Metrics API 的 Pod/容器 CPU/内存资源指标，不提供磁盘、网络、GPU 或历史曲线。已有可复用，不为此重复安装。
- 依据：NVIDIA DCGM User Guide 支持表及 non-datacenter GPU 限制；NVIDIA Jetson Linux Developer Guide 的 tegrastats；node_exporter README 容器监控 path.rootfs 说明；Kubernetes Metrics Server README 的用途边界。

## 架构与数据流

```text
node-exporter DaemonSet ─┐
dcgm-exporter (NVIDIA) ──┼─> 集群内 Prometheus (single replica, bounded TSDB)
tegrastats exporter ─────┘                 │
                                           │ Gateway admin proxy
                                           v
                                      节点详情 UI
```

每个集群内抓取其 exporter，避免管理面工作站跨站点 WAN 抓取、证书和网络耦合。Gateway 按已注册 Cluster 到 agentNamespace 的权威映射，把固定 Prometheus Service 查询代理到该集群。客户端不能传 Prometheus URL、Pod IP 或任意 PromQL。Gateway 使用白名单 metric ID 到固定查询模板的映射、超时和响应大小限制。若现有 Gateway 不支持可靠的 registered-cluster Service proxy，增加集群内受认证的窄 metrics-query Addon，由 Gateway 调用固定接口，不能公开 Prometheus NodePort。

Prometheus target labels 使用 cluster_id、node_uid、node_name。UID 避免节点重装后同名数据串线。Gateway 每次查询限定注册集群和 node UID。GPU 可用 UUID 作内部设备标识，序列号作为敏感信息默认不对 UI 暴露。不得将时序写进 Node status。

Prometheus v1 有界持久卷，双重限制 retention time 和 size。建议先 7 天并用真实 label cardinality 压测磁盘，再决定扩至 14 天。禁采进程、命令行、PID、任意路径等高 cardinality labels。单副本失效或管理工作站停机只影响指标，不影响调度、agent heartbeat 或 Station readiness。

Node Capacity/Allocatable 和 Pod requests 由 Kubernetes API 提供，Prometheus 提供 usage 与设备观测。UI 分别表达物理容量、可分配容量、已请求 allocation、实时使用率。分配的 GPU 数来自 Pod resource requests nvidia.com/gpu 和节点 capacity；GPU 利用率来自设备 exporter，含义不可混用。

A 是标准产品链路，有历史和 scrape freshness，支持查询、曲线与汇总；代价是 TSDB/PVC/维护。B 不增加 TSDB，但若只把最近摘要写 observation，则没有历史，并让 Station API 承担指标 ingest、验证、限频和状态尺寸。B 仅作可选 fallback，DTO 限体积/频率，带 observedAt/source；绝不存原始 series，也不与 A 同时作为无优先级的数据源。

## Addon 定义要点

在 apps/rlark/pkg/addons/catalog 下按版本化 package 定义 Addon，由 Gateway/release 发布不可变 package，Cluster Agent 拉取并渲染，保留现有漂移修复与回滚机制。

1. node-exporter：amd64/arm64 OCI manifest list，DaemonSet，配置 path.rootfs。只读 hostPath /、/proc、/sys，排除容器挂载；non-root、read-only rootfs、丢弃 capabilities、seccomp。仅给集群内抓取开放 Service/端口，避免外网暴露。
2. dcgm-exporter：只部署到 rlark.io/accelerator=nvidia 节点，双架构镜像须核实 ABI/driver compatibility。与现有 nvidia-gpu device-plugin/runtime Addon 分开启停。Exporter 只挂载实际需要的 GPU 设备和 driver libraries，不默认 privileged；3090、驱动和 CUDA 组合做 canary。
3. jetson-tegra-exporter：默认关闭，selector 匹配 Orin/Jetson 型号与 aarch64 节点。只读挂载 tegrastats 和必要 proc/sysfs。不得默认挂整个 /dev 或 privileged。若设备验证证明需要扩大权限，按设备节点精确声明。
4. prometheus：每集群单副本 StatefulSet、PVC、ClusterIP Service，固定 exporter selectors、scrape rules、retention time/size、CPU/内存 requests/limits。v1 不引入 Operator/Thanos 全栈。PVC/Service 不对集群外开放。
5. selectors 由管理员按集群启用，不由 UI 临时创建特权工作负载。缺 NVIDIA driver 或 exporter 不得让普通节点或 GPU workload rollout 失败。Addon 状态给出明确不兼容/无数据原因。
6. 优先复用已有 node selector 和 Addon 生命周期。采集 Addon 不安装驱动、不修改 containerd、不重启运行时。已有 nvidia-gpu Addon 会安装 toolkit 并配置 runtime，因此不能让 metrics addon 隐式触发它。

## API 与 UI 设计

所有新查询通过现有 JWT/admin authorization，校验 cluster 注册映射、namespace 和 node UID。响应 Cache-Control no-store；固定路由与查询模板，不透传 Secret、PromQL 或 exporter 原始错误。

- GET /api/v1/station/nodes/:name/hardware?cluster_id=...：静态 inventory 与当前 snapshot，字段建议 nodeUID、observedAt、stale、CPU 型号/核心/usagePercent、memory capacity/used/usagePercent、filesystems、网络 RX/TX rate、accelerators。数据缺失用 null 和 availability/reason，不伪造 0。
- GET /api/v1/station/nodes/:name/hardware/range?cluster_id=...&metric=...&from=...&to=...&step=...：v2 使用 metric 白名单与固定 PromQL 模板，from/to/step 严格限幅、点数上限，默认 1h，管理员最大 7d；拒绝任意 PromQL。
- GPU 卡数据每卡含 index、name/model、vendor、memory total/used、utilization、temperature、power draw/limit、driver、source、observedAt。UUID 默认可隐藏；序列号默认不返回，只为明确授权管理员提供并审计。
- CPU/内存/磁盘容量和 requests 来自 Node/Pod API；GPU requests 来自 Pod resources，物理卡容量来自 Node/device plugin，使用率来自 exporter。

在 apps/rlark-ui/src/astribot/pages/StationDetailPanel.tsx 增加硬件区，复用现有站点详情面板结构、主题 token 和 dark theme。按需加载，抽屉关闭取消查询，页面可见时约 30 秒刷新。展示 CPU、内存、磁盘、网络、容量/allocatable/requests；GPU 卡表显示型号、编号、显存、利用率、温度、功耗。Orin 展示 GR3D、RAM、温度/rail power 与 source badge。区分 allocated GPU 和 GPU utilization。无卡、字段不支持、采集失败、权限不足、旧节点无样本和 stale 均需可读状态，缺值显示“—”。v2 再加 15m/1h/6h 曲线与 min/max/avg。

## 文件清单

未来实现路径限于用户批准的 astribot-owned 路径：

- apps/rlark/pkg/addons/catalog/node-exporter/addon.yaml、manifests/*.yaml：双架构主机指标 DaemonSet 和集群内 Service。
- apps/rlark/pkg/addons/catalog/dcgm-exporter/addon.yaml、manifests/*.yaml：NVIDIA 卡选择、Exporter 和 Service。
- apps/rlark/pkg/addons/catalog/jetson-tegra-exporter/addon.yaml、manifests/*.yaml：Jetson tegrastats 解析 exporter。
- apps/rlark/pkg/addons/catalog/prometheus/addon.yaml、manifests/*.yaml：TSDB StatefulSet、PVC、retention 和 scrape rules。
- apps/rlark/pkg/gateway/astribot_node_metrics_handler.go：固定指标代理、注册集群边界、Prometheus Service 查询。
- apps/rlark/pkg/gateway/astribot_node_metrics_handler_test.go：RBAC、跨集群隔离、stale、缺组件、超时、查询白名单与响应上限。
- apps/rlark/pkg/gateway/astribot_gateway.go、router.go、Swagger/API reference tests：注册 admin API 并验证路由文档完整。
- apps/rlark/astribot/stationidentity/hardware_metrics.go：仅当实施 B fallback 时添加限频摘要 DTO/验证；不改原生 Agent protocol。
- apps/rlark-ui/src/astribot/api.ts：typed API DTO/client。
- apps/rlark-ui/src/astribot/pages/StationDetailPanel.tsx、对应 CSS：硬件状态、GPU 表格、暗色样式。
- apps/rlark-ui/tests/astribot-node-metrics.test.mjs：mapping、缺字段、stale、GPU/Orin 展示测试。
- astribot/docs/architecture.md 及中文对应页或新增运维文档：支持矩阵、权限、保留时间、升级/回滚和排障。
- 不新增 api/** CRD。容量与 Pod requests 复用 Kubernetes 原生对象。若需要跨资源持久 observation schema，超出范围的共享 API 必须另行批准。

## 发布顺序

1. 测试集群确认 node-exporter amd64/arm64；DCGM 对 3090 与目标 driver 的真实字段；Orin/目标 JetPack tegrastats 字段和运行权限。
2. 发布 exporter Addon 和 digest。先选一台 office NVIDIA 节点 canary，检查 host impact、driver 不变、权限和节点退出清理；再验证 Jetson。
3. 发布 Prometheus Addon 与 PVC、retention，在 canary 验证容量、写放大、TSDB 重启恢复、UID relabel 和 node down stale。
4. 发布 Gateway 固定查询代理，完成 unauthorized、cross-cluster、任意 query、响应限制和超时测试。
5. 发布 UI 并观察 Prometheus disk/series、target health、Gateway latency 和 Addon conditions。
6. 按集群逐步启用。NVIDIA 与 Jetson Addon 各自选择，异构集群中 exporter 不可用不能阻塞正常 node agent rollout。

## 风险与回滚

- 单副本 TSDB/PVC 非 HA，工作站停机会中断查询；不影响 RLark 核心控制。PVC 容量告警、备份/恢复演练是启用历史的上线门槛。
- GeForce 3090 DCGM 指标有限且受驱动版本影响。每字段建 capability/availability，不因温度或功率字段缺失判 exporter 故障。
- Jetson tegrastats 输出变化需版本化 parser fixtures；未知字段忽略并报采集能力条件，不能让节点变 NotReady。
- exporter hostPath/特权有攻击面。最小只读挂载、digest pin、明确 nodeSelector；不把 privileged 当默认。如果工具必需，应单独做威胁评审。
- 高 cardinality、抓取频率或缺限额可耗尽 PVC；先测 series/node、限制 labels 和 retention，监控 TSDB。
- 回滚顺序：Gateway 停用指标 API，再回滚 UI，再停 exporter Addon；Prometheus Addon 删除时先保留 PVC，待历史数据处置决定后再清理。Addon 不安装 GPU 驱动、不改 containerd，回滚不影响现有 GPU 运行时。
- 采集 unavailable 是观察面故障，必须与节点心跳、readiness、调度健康分离。

## 分期

### v1：当前值最小可用，可先无长历史

先发布 node-exporter、NVIDIA DCGM exporter 和 Jetson tegrastats exporter，按硬件选择启用。Prometheus 可以先只保留短期（1–3 天）或在产品 v1 首批仅用 instant query；UI 只显示当前值、采集时间和容量/requests。Gateway query path 已固定、RBAC 已有，不能因为“当前值”开放任意 PromQL。如果 Prometheus 部署阻碍试点，可临时选择 B：Station 侧限频 pull exporter 摘要，只保存最新受限 DTO，明确无历史并设 stale；B 不得变成写高频 Node status 或原始样本的正式方案。

### v2：历史曲线与阈值

启用 7–14 天受限 retention、recording rules/降采样、白名单 range API 和 15m/1h/6h/24h 曲线。增加 CPU、内存、GPU 温度/功率阈值和去抖；告警状态用独立 bounded alert/condition 设计，不靠每个采样更新 Node labels/status。是否引入 Alertmanager/通知由单独容量与运维评估决定。每版本验收磁盘容量、cardinality、PVC restore、驱动兼容和缺失指标语义。
