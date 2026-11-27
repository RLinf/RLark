# RobotModel 本体信息设计

## 1. CRD 形状

在 `station.rlark.io/v1alpha1` 的 `RobotModelSpec` 增加可选字段 `body *RobotBodySpec`，保留全集群作用域、现有型号 ID 和原有 `self.spec == oldSelf.spec` CEL。目录保存厂商或平台审核的型号定义。NodeClass 继续负责接入配置，并以 `spec.model` 引用目录 ID。以下示例只用于说明结构，参数与名称均为示意值。

```yaml
apiVersion: station.rlark.io/v1alpha1
kind: RobotModel
metadata:
  name: example-arm-r1
spec:
  displayName: Example Arm
  body:
    joints:
      - name: shoulder_yaw
        type: revolute
        units: rad
        limits: { lower: -2.6, upper: 2.6 }
      - name: lift
        type: prismatic
        units: m
        limits: { lower: 0, upper: 0.3 }
    endEffectors:
      - name: left_tool
        kind: gripper
        dof: 1
        placement: left_wrist
    sensors:
      - name: head_rgb
        kind: camera
        count: 2
        placement: head
      - name: torso_imu
        kind: imu
        count: 1
        placement: torso
    physicalSpecs:
      payloadKg: 3.0
      runtimeMinutes: 90
      dimensionsMm: { length: 600, width: 420, height: 1100 }
      battery: { capacityWh: 480, nominalVoltageV: 48 }
```

| 字段 | 语义与结构校验 |
|---|---|
| `joints` | 按 `name` 唯一的 list-map。类型为 `revolute`、`continuous`、`prismatic` 或 `fixed`。旋转为 `rad`，直线为 `m`，固定为 `unitless`。限位为静态位置区间，以关节定义的零位和坐标方向为准。 |
| `endEffectors` | 按安装槽名称 `name` 唯一的 list-map。类型为 `gripper`、`dexterousHand` 或 `passiveTool`。`dof` 为末端独立可动自由度，主动末端大于零，被动工具为零。可选 `placement` 供展示。 |
| `sensors` | 按 `name` 唯一的 list-map。`kind` 首批支持 `camera`、`imu`。`count` 为该装位组的正整数，`placement` 为非空装位名。相同类型的不同装位分条记录。 |
| `physicalSpecs` | 子项均可选，缺省表示未公布。负载以 kg、标称续航以分钟、尺寸以 mm、能量以 Wh、电压以 V 表示。负载为厂商额定工作负载，可用 0 表示无额定负载。尺寸三个轴齐全且均大于零，电池至少声明一个正值。发布型号资料时同时记录额定负载及续航的测试工况。 |

采用结构化 enum、必填字段、数值下界和 list-map 唯一键。CEL 附在对应子对象，校验关节类型与单位的组合，限制 `limits` 只出现在 `revolute/prismatic` 上且 `lower < upper`，两个端点须同时存在。允许省略有界关节的限位，显示「未公布」。`body` 一旦提供，至少声明一类数据，拒绝空对象，不给旧对象默认填充。字段为非 nullable，按 Kubernetes 的结构化 schema 缺省与裁剪规则处理，并测试 `null` 的实际行为。用可选字段保留零值与缺省的区别。控制台按明确定义的单位显示。v1 限位只供查阅。

关节的 CEL 可拆为以下三条，条件分支里先检查字段是否存在。类型枚举和上下限必填由结构化 schema 管理。

```cel
self.units == (self.type == 'prismatic' ? 'm' : self.type == 'fixed' ? 'unitless' : 'rad')
!has(self.limits) || self.type in ['revolute', 'prismatic']
!has(self.limits) || self.limits.lower < self.limits.upper
```

现有无 `body` 的 `s1u` 可继续被引用、读取和使用，详情显示「尚未发布本体定义」，节点仍可显示实报。用现行 CEL 对现有 `s1u` 直接 apply 新 `body` 会被拒绝。保留不可变契约的发布路径是审核后新增带版本的目录 ID（如 `s1u-r1`），为新接入配置相应 NodeClass，逐批通过正式迁移同步 admission、NodeClass 引用和节点的型号标记。先验证旧节点能否原位迁移，需重入网时单独安排窗口，混用目录 ID 期间保留旧条目。不要用删掉重建同名对象或停用引用保护来填充旧条目。如果产品要求原有 `s1u` 无需迁移即可获得定义，需要另行批准「已有 spec 完全相等且 body 只允许从缺省变为存在一次」的 CEL 例外，并单独验证字段删除和修改均被拒绝。本方案默认保持现行 CEL。

## 2. 上报与对照通路

### 证据来源和权限

1. 由 S1U 侧只读采集器读取现用驱动实际枚举的关节与已连接的工具。先用供应商接口样本验证名称、范围和工具识别能否由当前硬件独立给出。驱动描述或 URDF 只有在可确认绑定到当前驱动实例时才可作为运行时声明，并显式标记来源为 `runtimeDescription`。运行时没有 EEF 枚举时，该组为 `unavailable`。公开的 `GetRobotStatus` 当前只含机器人 ID、运行模式、生命周期及 ROS 配置等，缺少 joints/EEF 清单。可以用它发现正在工作的运行时，v1 的清单由 Astribot 适配器提供。参考 `proto/embodied-runtime/roscontroller/v1/robot.proto:122`。
2. 安装时只配置采集适配器与对应设备端点。S1U 的 `robot_param.yaml` 是 x86 上的配置，虽然包含夹爪版本、序列号和关节偏移，不能证明当前装机。它可供采集器定位和诊断。Node 标签继续承担型号声明的兼容读取，无需把清单编码进标签。采集和对照均不启动驱动、不上电、不发运动指令。参考 `astribot/s1u/tuner/node_profile.env:92`。
3. 由受信任的模型 Addon 生成有界、原子更新的本机清单快照。优先复用站点代理的本机观测文件模式，文件由受控进程写入，station-agent-host 只读挂载。Orin 是数据源时，由 S1U 适配器经明确配置的受信任设备绑定读取，拒绝跨节点或从任意工作负载提交的清单。上线前确认实际由哪套 station-agent 负责该节点上报。清单接入现有的 `POST /v1/observations`，沿用站点身份鉴权、Node UID 绑定、限流和管理集群命名空间解析。认证可证明报告来自绑定节点，证据来源仍须显示。

### 状态与生命周期

- 请求增加可选的 `observation.robotBody`。Station API 验证后写入 `NetworkEnrollment.status.observation.robotBody`。包含 `schemaVersion: 1`、`joints` 与 `endEffectors` 两组，组形状为 `{ state, source, items }`。状态为 `complete | partial | unavailable`，来源为 `driverInventory | runtimeDescription`。每个 item 必含规范名称，关节类型或 EEF 类型和 DOF 在来源可给出时才传。每组来源和完整性独立，`complete + []` 表示明确报告零项。v1 不上报 sensors 或 physicalSpecs。
- 快照缺失或采集失败时保留正常心跳。报告只附当前采集有效期内的快照，例如 30 秒采集、90 秒过期。通过本机单调计龄及重启后要求新采样避免重复发送旧文件。请求携带有界的采样年龄，API 以接收时间推算 `sampledAt`，保留服务器 `receivedAt` 及归一化快照摘要用于绑定。拒绝格式错误、重复名称、非法枚举及超出各组和字节预算的清单。采集器先校验，遇坏清单发送正常心跳并省略扩展。接收端显式替换 `robotBody` 子树，旧 agent 省略该字段时显式清除旧快照。用回归测试覆盖现有 merge patch 不会因字段缺省自动清除历史值。允许单个节点代理串行提交报告。同一次采样携带稳定采样 ID 和启动标识，重发时重新计算采样年龄。服务端对同一采样保留首次确认的采样时间，对较早的同启动样本丢弃清单更新，按 resourceVersion 重读后提交。验证重启后仅接受新采样，不能利用重复提交延长已失效清单。过期时重新对照，节点心跳更新不能延长清单有效期。
- station-controller 在节点重协调时按绑定的 enrollment、同集群的有效 NodeAdmission 解析期望身份，再复用现有 NodeClass 兼容解析。型号解析只用期望身份。Admission 缺失、存在绑定争议、观测 UID 与 enrollment 不符或型号未核验时，当前对照为未知。涉及换机或退役时清掉旧判断。监听观测更新及有影响的目录、admission 和 class 变化，按清单到期时间重新入队。使用确定性、幂等的状态 patch。
- 在 `NetworkEnrollment.status.node.conditions` 新增 `RobotBodyVerified`，并以控制器拥有的 `status.node.robotBodyComparison` 绑定本次判断的型号 ID、目录 UID、快照摘要及采样时间。条件记录已处理的 enrollment generation。Gateway 只有在型号、快照与绑定版本仍一致且新鲜时才采用结果，防止读到更新中的旧判断。原有 `IdentityVerified` 和 Machine 投影继续表示类型/型号一致性。
- 对照按关节名及 EEF 装位名排序、匹配。两组清单均来自设备枚举、应验字段皆可比较且一致时为 `True/Matched`。完整设备清单缺项或多项，或可确认的类型/DOF 差异，为 `False/Mismatch`。部分清单不能用缺项作证，但其中明确报告的类型/DOF 冲突可判为不一致。仅有驱动描述时可展示差异供排查，整体为 `Unknown/InsufficientEvidence`。其余未知原因包括 `ModelBodyUndefined`、`ModelUnresolved`、`IdentityUnverified`、`InventoryNotReported`、`InventoryStale` 和 `ComparisonPending`。目录省略某组表示未声明，整体显示无法完成核验。主动 EEF 的 DOF 和关节类型无法取得时可展示存在性，整体保持 Unknown。限位、感知器、负载、续航和电池规格 v1 均不参与判定。
- 仅机器人节点适用。比较不会改变 Ready、调度、入网、节点权限或自动修改任何硬件设置。持久化的条件消息只写简短汇总。差异明细由同一份比较规则生成，避免重复存储。在 UI 归类为已确认差异、证据不足和型号未定义三种不同提示。

## 3. Gateway API

| 接口 | 契约 |
|---|---|
| `GET /api/v1/station/robot-models/{name}` | 沿用管理员路由组。返回 `{name, displayName, vendor?, description?, body?}`；缺少 body 仍为 200。无型号为 404。列表保留轻量视图和已有引用计数。删除继续走当前引用保护，禁止通过控制台新增或编辑。 |
| `GET /api/v1/site-nodes/{name}/robot-body?cluster_id=<id>` | v1 限管理员。用可信 Cluster 注册解析 agent namespace，再读取该命名空间中同名 enrollment 并校验节点绑定。返回 `{cluster_id, node_name, model, expected, observed, comparison}`。退役记录依照现有站点节点 GET 的可见规则处理。缺少必填 cluster_id 为 400，越权 403，节点不存在 404，上游不可用 502。旧机器人没有 body 或观测仍为 200，两个槽位分别返回 null 或实际数据，附明确 reason。 |
| 现有站点节点列表和详情 | 对有权查看该节点的人返回轻量 `robot_body_status` 与 `robot_body_reason`。批量响应省略完整目录和清单。先确认现有可见性保护可用于新字段，否则只向管理员投影。 |

`expected` 为完整本体定义，`observed` 为有关节和末端证据来源、完整性和采样时间的快照。示例比较对象为 `{"status":"False","reason":"Mismatch","differences":[{"section":"endEffectors","name":"left_tool","field":"kind","expected":"gripper","observed":"dexterousHand"}]}`。零差异与未核验用状态区分。快照已变化时只展示待核验状态，避免旧结果配上新数据。复用 controller 的 Astribot 纯比较函数生成排序、有界的差异和缺项提示。Swagger 标明响应投影、401/403 等现有鉴权行为及各空态。下发数据限定为节点清单，排除底层配置路径、端点和凭据。

## 4. UI 设计

### 交互流程

- 在 RobotModels 列表点击型号名进入详情。采用现有 admin router 的 `stations` 子路由机制，`navigate("stations", "models/<id>")` 生成一个编码后的子路径，AstribotAdminPages 解出 `models/<id>` 并路由，在原生路由文件零改动的情况下测试直接打开、刷新和返回。详情保留返回目录，显示名称、厂商和 ID，下方为关节、末端、传感器、物理参数四个区块。关节表列出名称、类型、单位和限位，末端与传感器分行展示装位。缺少定义、请求失败及空字段各自有明确说明。
- 站点机器人卡片显示「本体一致」「本体不一致」或「待核验」的简短状态。展开现有 `StationDetailPanel` 时请求节点对照接口。显示来源、采样时间及完整性，先列异常行。行内同时放型号定义和实例上报，区分缺失、额外、已报告冲突和证据不足。传感器和物理参数标为型号数据。节点离线或刷新失败时，旧内容如保留须标明历史时间；不会显示为当前通过。复用页面的取消请求和选中节点保护。
- 颜色使用现有 RLark 语义 token，暗色沿用主题变量。差异用警告图标和文字，证据不足用中性状态，失败用错误样式。颜色不单独承载语义。键盘可进入详情和返回，长名称换行，移动端用单列并将定义与实报置于同一行组。

### 高保真 Prompt

沿用 RLark 控制台的表格、页面标题和场站卡片。桌面型号详情采用两列，其中关节清单占主列，右列显示末端、传感器和物理规格。390 px 以阅读顺序叠成单列。节点详情以「型号定义」「实例上报」为两栏，移动端在每个设备名下顺序显示两值。状态使用可读文本和主题语义色，空态和失败使用现有控制台组件。

## 5. 文件清单

| 范围 | 未来实施位置 |
|---|---|
| CRD | `astribot/station/api/v1alpha1/robotmodel_types.go`、`types.go` 和生成的 DeepCopy；同步 `astribot/station/config/crd/bases/station.rlark.io_{robotmodels,networkenrollments}.yaml`。型号示例归 `astribot/station/`。 |
| 上报 | `astribot/s1u/` 内新增只读采集器和 fixtures；`apps/rlark/astribot/stationidentity/{agent,http,kubernetes_observation}.go` 及配套测试；按需调整 `apps/rlark/astribot/cmd/station-agent/`、`astribot/station/deploy/station-agent.yaml`。 |
| 对照 | `astribot/station/controller/node_reconciler.go` 及测试。可共享的归一化与对照逻辑放在 `astribot/station/` 内的独立包，供 Gateway 和 controller 调用。 |
| Gateway | `apps/rlark/pkg/gateway/astribot_{gateway,robot_model_handler,site_node_handler}.go`、配套测试、`astribot/docs/swagger.yaml`。 |
| 控制台与文档 | `apps/rlark-ui/src/astribot/` 内的 `api.ts`、`AdminPages.tsx`、`pages/RobotModels.tsx`、新增详情组件、`StationCard.tsx`、`StationDetailPanel.tsx`、DTO、CSS 和 mock。测试在 `apps/rlark-ui/tests/astribot-*.test.mjs`。英文和中文操作说明及发布示例均放在 `astribot/docs/`。 |

## 6. 发布顺序与验证

1. 用真实 S1U 样本核对厂商申明、驱动的枚举语义、关节名称覆盖范围和末端识别。无法确认的字段保持缺省，避免先给现有 S1U 编造数值。验证旧模型同原样 apply、旧节点正常启动、旧 spec 改写仍被 CEL 拒绝；验证新 schema 在实际目标管理 API 上拒绝非法字段。
2. 先发布可选 CRD 字段，再发布能接收、验证和清除扩展的 Station API，随后发布 controller、Gateway 和 UI。现有 Station API 严格解码，先确认所有接收副本支持新字段才启用报告。UI 先验证旧型号、旧 agent 和临时上游故障的表现。
3. 经审核以后台 apply 新型号条目。先让一台已授权的 S1U 以灰度配置启用清单发送，再验证同型号完整匹配、交换 EEF、缺项及部分上报、离线过期、重启、UID 更替、乱序和节点同名跨集群的行为。用真实有效、部分及失效的 fixture 做 UI mock，验证 EN/ZH、明暗主题与 1440/390 px。观察错误率、状态对象体积和控制器收敛，确认无健康或入网退化再扩大范围。

## 7. 风险与回滚

- 磁盘配置冒充实装、驱动只枚举当前模式下的部件、换机继承旧快照是主要误判来源。使用来源等级、按组完整性、UID 和采样新鲜度四道约束。独立测得的证据不足时保持 Unknown。规范名称映射由适配器版本管理，不按模糊匹配合并。
- 现有型号引用可能遍及离线节点和历史对象。保留旧 ID 及其兼容入口。删除保护与迁移并发须验证，避免把同名目录删除重建误认为同一实例。按型号 ID 与 UID 校验对照缓存。
- 心跳与采集隔离，控制器只在条件或快照语义变化时更新判定。限制清单、差异数量和对象体积。只在站点管理员授权范围内展示详细硬件盘点。
- 回滚时先关闭灰度报告并等待在途请求完成，再回滚 Station API。控制台可先隐藏对照入口，controller 停止写新条件，旧版本继续处理原有身份和网络状态。保留向后兼容的扩展 schema 及已发布型号，避免 CRD 缩减后被裁剪。需要退回型号迁移时，按节点反向同步 admission、class 和型号标记；只有确认目录已无引用后才删除新型号。原有部署和回滚步骤无需触发硬件动作。

## 8. 分期

- **v1** 覆盖静态型号目录与详情、低频关节/末端清单、实例差异和未知状态。传感器与物理参数只显示目录值。设备来源不足的型号仍可读取目录和保留空或部分观测。
- **二期** 的实时关节角度及曲线采用独立授权与数据通路。需另行确定采样、时钟、单位、流控、会话生命周期和可观测性。实时数据不进入本期 CRD、心跳或对照接口。
