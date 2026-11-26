方向 B「去框化清单」。先读 `apps/rlark-ui/prototype/stations-visual-brief.md`（为什么重出题、不变的契约、交付要求都在里面）。

核心：删掉所有盒子。没有面板没有卡片框，集群是标题+行清单，机器人行 = 缩略图 + 名字 + 一行彩色状态词 + hover 浮出操作，相机进 lightbox。全页面禁止 chip 堆，状态永远是一个色点+一个词，分组靠留白和字阶。宽屏是从容的列，不是空荡的大盒子。

只新增 `apps/rlark-ui/prototype/stations-lang-b.html`，自包含、无外链、无请求。12 张截图 `/tmp/rlark-ui-verify/shots/stations-lang-b/`，量测照跑，报告写清与现役的本质区别（≤3 条）、牺牲了什么。完成后停下，不改生产。
