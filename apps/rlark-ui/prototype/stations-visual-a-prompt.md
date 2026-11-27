方向 A「监控墙」。先读 `apps/rlark-ui/prototype/stations-visual-brief.md`（为什么重出题、不变的契约、交付要求都在里面）。

核心：画面就是界面。机器人是无边框相机 tile，状态压在画面上，异常 tile 发光跳出，库存/控制面是等宽数字表格不是卡片。暗色优先，亮色也要成立。第一眼必须和现役页面（白卡片+chip+边框）分得开。

只新增 `apps/rlark-ui/prototype/stations-lang-a.html`，自包含、无外链、无请求。12 张截图 `/tmp/rlark-ui-verify/shots/stations-lang-a/`，量测照跑，报告写清与现役的本质区别（≤3 条）、牺牲了什么。完成后停下，不改生产。
