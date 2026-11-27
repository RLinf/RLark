# RobotModel 本体信息方案交叉评审

## 确认正确的关键假设

- **现行 CEL 冻结整个 spec。** RobotModel 集群级，metadata.name 是 NodeClass 与节点身份使用的稳定型号 ID；规则为 self.spec == oldSelf.spec。给 schema 新增无默认值的可选 body，旧无 body 的对象仍可正常读取及原样更新，但直接给现有 s1u 补 body 会被现行规则拒绝。Station 自有 Go 类型与生成 CRD 均声明了这条规则。参见 astribot/station/api/v1alpha1/robotmodel_types.go:5、astribot/station/config/crd/bases/station.rlark.io_robotmodels.yaml。
- **GetRobotStatus 未提供本体清单，“只返回模式和生命周期”需要修正。** Proto 还定义 robot_id、current_mode、ros_master_uri、params 和 ros_domain_id。ROS 1 实现返回进程/roscore 检查后的状态及配置参数，ROS 2 检查启动进程并返回 DDS domain 和配置参数。两者均无独立枚举的 joints/EEF，params 是配置映射。仓库内 astribot/s1u/ 没有该 RPC 实现，不能把公开控制器响应当成已验证的现场厂商接口。参见 proto/embodied-runtime/roscontroller/v1/robot.proto:126、apps/embodied-runtime/pkg/roscontroller/robot_query.go:17、apps/embodied-runtime/pkg/ros2controller/robot_query.go:16。
- **robot_param.yaml 不能证明装机。** 代码从 x86 的 /etc/astribot_config/robot_param.yaml 读取左右夹爪版本、马达版本、serial_number 和文件摘要。配置注释还记录关节偏移，明确文件保存在主机磁盘、可不随更换本体迁移。原文件不在仓库且本机没有可读副本，因此只能确认代码读取项和注释记载，不能核验现场全部键或值。参见 astribot/s1u/tuner/node_profile.env:92、node_profile.sh:339、node_tuner.py:424（均在 tuner/）。
- **可复用现有观察通路。** Station API 通过成员集群的 Pod-bound token、Pod 和 Node 查验得到真实节点身份，然后按 Cluster 对应管理命名空间写 NetworkEnrollment.status.observation。当前 hardware 仅由已认证 NodeInfo 生成基础信息。请求严格解码，merge patch 会保留省略键，已有代码用 null 清除缺省项。现有 sink 还允许不同 UID 的观察到达同名 enrollment，供节点冲突处理，新增本体信息不得阻断这一链路。参见 apps/rlark/astribot/stationidentity/kubernetes.go:62、http.go:195、http.go:321、kubernetes_observation.go:118 和 :146。

## 发现的问题（按严重度）

### 高

1. **真实硬件证据仍未验证，应作为动态核验的上线门槛。** GetRobotStatus 的 RUNNING 证明的是运行时进程状态，不能证明驱动已枚举实际设备。先采集目标 S1U 的厂商只读响应，验证断开、换夹爪、模式切换、机器人停机和驱动缓存仍存在时的结果。只枚举当前模式或只加载 URDF 时，标记部分清单或运行时声明。驱动把离线设备误报成零项时禁止映射为 complete + []。静态目录可先发，动态状态在证据成立前显示 Unknown。没有现成枚举能力时，禁止为核验启动驱动、上电或发运动指令。
2. **同名节点换机、旧报告重放和多写者需要明确事务规则。** 现有 sink 在同名记录按 status merge patch 写入，controller 才处理 UID 变化。保留新 UID 的基础心跳路径，但清掉旧本体、对照及缓存，且返回旧记录或历史时不显示当前 Verified；旧 UID 晚到不能重新覆盖已经收敛的新 UID。针对旧/新 agent 重叠、在途旧心跳清除新快照、双写者、跨启动乱序、TTL 到期无新心跳、应用重启分别测试。resourceVersion 能检测并发，不能证明事件新旧。若 v1 能保证单一 writer，以绑定 UID 的有序采样与到期失效为先；只有确认重试/乱序要求后再加入持久化重放元数据。必须证明谁能写本机快照、挂载只读且回收旧文件，避免通过同节点共享目录伪造来源。碰到不合法的可选清单，优先丢弃清单并保留基础心跳，验证现有全请求 400 路径不会误伤。
3. **核验结果需绑定期望依赖与证据强度。** 方案已有型号 ID/目录 UID/样本摘要，还应验证有效 admission 的实例和版本、当前绑定的 Node UID、期望解析结果是否仍一致。NodeReconciler 目前只对 enrollment 的 generation/observation 变化监听，须加相关依赖触发或有界复核，并避免目录短暂不可读时让节点就绪/网络状态停止更新。现有 IdentityVerified 来自 Node 标签与声明的比较，且可只匹配类别；本体核验另须要求解析出非空型号。完整枚举中的明确额外件可判不一致；部分清单只对可确认冲突判不一致。参见 astribot/station/controller/node_reconciler.go:52、:210 和 machine_identity.go:62。
4. **旧 ID 迁移和删除保护不能默认为完整。** Gateway 的引用计数后删除有竞争窗口，后台直接操作 CRD 也绕过 Gateway 检查。若要清退旧 ID，先冻结新引用并检查 NodeClass、admission、Machine、节点声明和观察，删除需带对象 UID 前置条件。遇正在进行的加入、离线节点和历史对象，保留旧 ID。迁移还要核对旧的 NodeClass 指纹与新 class 是否重叠、Machine 历史关联、canonical/legacy 标签如何传播；重叠可能使发现分流含糊。参见 apps/rlark/pkg/gateway/astribot_robot_model_handler.go:30、:130。

### 中

- **边界：B 的文件清单在允许范围内，部分实施细节尚未闭合。** 对 proto 与原生 ROS 实现仅作读取引用，无需改 api/**、原生 router 或 src/generated/**。现有原生路由已编码/解码子路径，前端可用现有扩展入口，加直接打开和刷新测试。S1U 采集器的交付需落在 astribot/**，用运行时外置 Addon 目录或现有 Astribot 镜像配置，不要改原生嵌入目录。Station 是独立 Go module，根 go.work 当前不包含它，Gateway 直接 import 比较包会带出未列出的依赖改动。v1 优先由 controller 生成有界比较明细，Gateway 校验绑定并投影；若确需共享包，先证明 workspace、独立模块和镜像构建的依赖路径都在允许范围。
- **CEL 和对照预算还需落地。** 明确 maxItems、名称 maxLength、单样本字节上限及差异截断规则；对目标 kcp/Kubernetes API 做 schema/CEL 执行测试，覆盖空数组和空子对象、null 裁剪、重复 list-map key、大清单及有效零值。静态字段称为「目录标称」，末端 DOF 和关节位置限制首批只读展示；可以晚于目录发布完整传感器规格与厂商工况，避免让未知信息阻塞首版。
- **读接口及失败态。** 管理员专用完整数据做真实权限测试。通用站点列表初版可不加状态投影；若加，先证明 cluster 可见性并限定到管理员。抽屉 key 用 Cluster ID 与 Node UID；旧请求返回、切节点、换机、401/403/502、空清单和过期时分别验证。故障回滚先停扩展发包（含灰度 agent 和在途请求），再回滚严格解码的 Station API；schema 保留到确认无需新字段。不能只隐藏 UI 就回滚写路径。

## 与硬件方案的冲突与共享改动

| 位置 | 共享与落地方式 |
|---|---|
| apps/rlark-ui/src/astribot/api.ts、mock、StationDetailPanel.tsx | 共同修改。共享节点标识、权限与取消请求模式，拆分「机器人本体」和「计算硬件」子组件及 DTO。两个查询独立加载及报错，切节点同时取消；硬件定时刷新不重置核验视图。 |
| StationCard.tsx、Stations.tsx、样式及 UI 测试 | 本体方案明确加卡片状态。硬件方案原文只要求详情，不需要卡片采样或列表逐节点请求；若后续共用卡片布局或标记，统一由列表投影驱动。共享明暗 token、加载和过期状态样式。 |
| Station Gateway 路由、Swagger 与节点身份解析 | 共同修改 astribot_gateway.go，统一使用 Cluster 映射和 Node UID、管理员权限与响应上限。接口各自投影；机器人清单不走 PromQL，GPU 采样不挂到机器人目录。 |
| NetworkEnrollment、Station API 与 station-agent | 本体使用 status.observation.robotBody 和 status.node.robotBodyComparison；硬件主方案 A 走 Prometheus，不改该 status。只有正式采用 B 时才增加独立 hardwareMetricsSummary，保留现有 NodeInfo hardware。合并评审 types.go、生成 CRD、agent/http/sink 与 RBAC，规定字段所有权、merge/null 语义、总字节预算与接收先于发送的发布顺序。 |

交叉审阅也发现硬件方案自身的路径清单有越界项：apps/rlark/pkg/addons/catalog/ 与原生 router.go 不在硬边界内。落地时 Addon 包应置于 astribot/**，通过既有 RLARK_ADDON_CATALOG_DIR/发布机制装载，路由使用 astribot_gateway.go。参见 astribot/release/control-plane/rlark.compose.yaml:69、apps/rlark/pkg/addons/astribot_catalog.go:273、astribot_publish.go:21。

## 对 CEL 决策的建议

**如目标是给现有 s1u 补资料，我建议经单独批准采用一次性 CEL 例外。** 现有定义将 metadata.name 当稳定身份；仅补目录说明就改成 s1u-r1 会让 onboarding、标签、核验与历史对象迁移成上线依赖。规则应精确保留原有 spec 字段的值和存在性，只允许 body 从缺省变成一次有效非空值；存在后，继续要求整个 spec 等值。把补全与其他字段编辑分开，避免放宽原有不变量；新增字段也须被不可变默认规则覆盖。目标 API 上验证原样 apply、首次补齐、重复同值、并发补齐不同值、编辑旧字段、删掉或置空 body、修改 body、历史对象与回滚。保留扩展 schema；补写前备齐已审核目录内容。

若产品尚未批准例外，保持现行 CEL，先用现有 s1u 显示「尚未发布本体定义」；确实代表不同产品/硬件修订，或必须坚持目录条目创建起完整不可变时，再用新 ID 与显式迁移。删除后同名重建会换 UID 且绕开历史关联，不能作为补齐方法。
