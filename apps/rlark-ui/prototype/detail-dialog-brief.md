# 设计/实施 brief：详情弹框化 + UI 版本显示

## 背景

生产场站节点页已采用「机器人相机墙 tile + 安静清单」语言（用户已认可）。剩两个问题，用户拍板：

1. **详情不再行内展开**——用户认为行内整行面板难看，要求：点击 tile 右上角的展开按钮 → **弹框（modal dialog）** 呈现详情。库存行的展开同理。
2. **显示当前运行的 docker 镜像版本**——用户想随时知道 80 端口在跑哪个构建（特别是现在处于「代码目录挂载」过渡态）。

本轮只出 plan（交互规格 + 实施方案 + 风险清单），不改生产、不写 src。两位 Codex 各出一版方案，交叉评审后由 station-node-ui-polish（Claude t3）合成终版 plan 给用户。

## 不可破坏的契约（现有行为，必须保留或等价迁移）

- Escape 关闭、焦点回到触发按钮（tile 的 `station-expand-*` / 库存行的 `station-inv-expand`）。
- 单开语义：一次只开一个详情（弹框化后天然满足，但 aria-expanded 状态要继续反映）。
- 相机放大（详情里的四路实时相机 → 点击放大）——弹框之上再弹一层放大框：原生 `<dialog>.showModal()` 的 top-layer 嵌套是否可行、关闭返回焦点链是否正确。
- 电源/驱动/回位操作 + **固定 toast**（右下角 `stations-power-toast`）——toast 是 fixed 定位，dialog 的 top-layer 永远在普通 z-index 之上：**弹框打开时 toast 会被盖住看不见**。方案必须解决（toast 移入 dialog？portal？还是 dialog 内联反馈？）。
- NodeCameras 实时轮询的生命周期（弹框关闭必须停轮询）。
- 键盘可达：Tab 循环限制在 dialog 内（原生 dialog 自带）。
- 按住回位的 deadman 语义（指针移出/失焦释放）在弹框内继续成立。

## 弹框设计要求

- 原生 `<dialog>` + `showModal()`（项目已有 CameraPreviewDialog 先例），不用自造 overlay。
- 宽度 `min(1560px, 100vw - 48px)`、高度 `min(92vh, ...)`、内部滚动；内容沿用刚重排的 quiet 语言（身份/网络与入网/条件 + 机器人电源 + 实时相机）。
- 关闭按钮（右上 ×）+ Escape + 点击背板关闭（背板点击可选，说明取舍）。
- 移动端 390：全屏抽屉式（inset 0）。
- 动画克制（150ms fade/scale，prefers-reduced-motion 关闭）。

## 版本显示要求

目标：任何时刻能回答「80 端口跑的是哪个构建」。要点：

- **数据源**：镜像构建时注入（Dockerfile build arg `RLARK_UI_VERSION`，内容 = 镜像 tag + git commit + 构建时间），产物写到 dist 的 `version.json` + 注入 `index.html` meta；**代码目录挂载模式**下由构建脚本从 `git describe` + 时间戳生成同一格式。
- **运行时获取**：应用启动时 fetch `/version.json`（no-cache），失败则显示「未知构建」。
- **展示位**：右下 footer 现有版权行旁加一段 `ui: <tag> · <commit 短> · <构建时间>`；可选：顶栏 Mock 环境胶囊旁加同信息的简版。给出你的推荐和理由。
- **交接风险**：隔壁 release（ci-cb84c00-e60a306）的 Dockerfile/compose 也要吃到这个改动——方案要注明哪些文件是共享的（rlark-ui.Dockerfile、compose env），避免和 release 构建冲突（当前 release 正在构建中，生产实现要等它落地后再动）。

## 交付

- 各自写 `/tmp/detail-dialog-plan-a.md`（node-removal）和 `/tmp/detail-dialog-plan-b.md`（station-cluster），中文，包含：
  1. 交互规格（打开/关闭/焦点/toast/相机嵌套逐条）
  2. DOM/CSS/TS 结构方案（改哪些文件、复用哪些现有组件）
  3. 版本显示方案（数据流 + 展示位）
  4. 风险清单 + 每条的规避
  5. 实施步骤拆解（含测试/文档影响面）
  6. 你预计的取舍和理由
- **禁**：改 src/tests/docs、build/deploy、git 写操作。只写你自己的 plan 文件。
- 写完停下，等交叉评审安排。
