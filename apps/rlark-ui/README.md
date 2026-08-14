# RLark Console Demo

基于 React、TypeScript 和 Vite 的纯 Mock 前端演示。

## 本地运行

```bash
cd web
npm install
npm run dev
```

生产构建：

```bash
npm run build
```

## 数据模式

前端只使用一种数据模式，避免页面之间混用 Mock 与真实数据：

```bash
# 所有 /api 请求使用统一 Mock 数据（开发环境默认值）
VITE_DATA_MODE=mock npm run dev

# 所有 /api 请求发送给真实后端（生产构建默认值）
VITE_DATA_MODE=backend npm run dev
```

Mock 数据由同一份拓扑生成，集群、节点、任务、工作流、网络域和存储类之间的名称与引用保持一致。真实模式下请求失败会显示错误或空状态，不会自动回退到 Mock 数据。修改模式后需要重启 Vite。

## Data mode

The frontend uses exactly one data mode so pages never mix mock and backend data. Set `VITE_DATA_MODE=mock` to serve every `/api` request from the shared in-browser mock topology, or `VITE_DATA_MODE=backend` to use the real backend exclusively. Development defaults to `mock`; production builds default to `backend`. Restart Vite after changing the mode.

## 当前页面

- Overview：平台健康、资源使用、活跃工作流和事件流
- Workflows：Workflow 列表、详情和 DAG 执行视图
- Jobs：Job 列表与 Task 执行进度
- Tasks：任务类型、角色、节点和状态
- Nodes：节点默认按集群和节点名称排序，展示物理位置、GPU/具身设备总量、空闲量与型号；从总览地图点击城市可直接进入对应位置筛选结果
- Cluster detail：集群详情中的节点表与 Nodes 使用相同的位置、资源数量、空闲状态和型号口径
- Node metadata：容量与可分配量由 Agent 从 Kubernetes Node 自动上报；位置使用 `rlark.io/ip-location` 注解，节点/GPU/具身型号使用 `rlark.io/model` 标签。未上报实时使用量时，界面会明确显示“空闲未知”
- Jobs：列表直接展示并支持复制 Kubernetes `metadata.name` 任务 ID；仅在配置 `rlark.io/display-name` 时补充显示名称，并展示去重节点数、创建时间和停止时间
- Lists：集群、节点、任务、工作流、存储和 SSH 公钥主列表支持点击表头切换升序与降序；分页基于排序后的完整筛选结果
- Time：创建时间、停止时间等统一转换为中国标准时间（`Asia/Shanghai`），格式为 `YYYY-MM-DD HH:mm:ss`
- Overview：核心指标聚焦具身集群数量、具身节点数量、具身设备种类，以及正在运行/全部任务数量
- Page headers：每个一级导航主页面统一保留一层“分类、标题、简介”页头，页面内部不再重复同级标题
- Naming：主页面标题统一使用管理语义，例如“节点管理”“任务管理”“工作流管理”；总览页不额外展示数据模式卡片
- Overview header：总览页复用其他一级页面的标准页头高度与间距，包含分类小标题、主标题和一句简介

管理员可以在数据面 Kubernetes 集群中补充业务元数据：

```bash
kubectl annotate node <node> rlark.io/ip-location='{"province":"上海市","city":"上海市"}' --overwrite
kubectl label node <node> rlark.io/node-category=cloud rlark.io/model='NVIDIA H800' --overwrite
kubectl annotate jobs.rlinf.io <job> rlark.io/display-name='策略训练任务' --overwrite
```

设备数量不应手工标注：GPU 容量由 NVIDIA Device Plugin 注册，具身设备容量由 embodied-runtime Device Plugin 以 `rlinf.io/device` 或 `rlinf.io/device-<model>` 资源注册。实时空闲量需要 Agent/后端提供 `status.used` 后才能精确展示。

Administrators may add business metadata on the data-plane Kubernetes cluster with the same labels and annotations shown above. Device counts must come from the NVIDIA or embodied-runtime Device Plugin rather than manual labels. Exact free capacity is only shown when the Agent/backend reports `status.used`.

The primary Cluster, Node, Job, Workflow, Storage, and SSH Key lists support ascending and descending sorting by clicking their column headers. Sorting is applied before pagination.

Job IDs use the Kubernetes resource `metadata.name` and can be copied directly. User-facing timestamps use China Standard Time (`Asia/Shanghai`) in `YYYY-MM-DD HH:mm:ss` format. The overview emphasizes embodied clusters, embodied nodes, unique device models, and running/total jobs.
- API Reference：后端资源 API 演示

数据集中维护在 `src/data.ts`，后续可替换为真实 API Client。
