# kindtest — 代码优先的 Kind 回归框架

**配置只描述“在哪里测”**（集群、kubeconfig、镜像、命名版本、报告目录），
**“测什么”一律用 Python 代码写**。框架只提供很少的语法糖，其余复用 Python
自身的特性（装饰器、生成器、参数注入、异常）。

报告采用分层语义：

```
测试流程 (flow)  →  测试观测点 (observation point)  →  测试项 (item = 报告的一行)
```

* **测试流程**：用装饰器定义的一段过程，只运行一次；它可以建立/销毁环境。
* **测试观测点**：流程里的一个 `with t.observe("REG-01"):` 块，评估一个或多个测试项。
* **测试项**：报告矩阵里的行；同一个 id 可以在同一流程的多个观测点出现，结果合并。
  id 是矩阵的主键，**不同流程/用例不能重复声明同一个 id**——框架启动时直接报错，不会让后者静默覆盖前者。

---

## 目录

```
test/kindtest/
  kindtest/            # 框架包（无第三方依赖，仅 PyYAML）
    registry.py        # group / flow / item / case / fixture / 参数展开
    context.py         # 上下文 t：kubectl 操作、渲染、等待、证据、断言、观测点
    kube.py            # Kube / Cluster / Kind 句柄
    versions.py        # 命名版本与滚动部署（config.versions / t.upgrade）
    runner.py          # 全表先行、流程执行、实时报告
    report.py / model.py / config.py / shell.py / cli.py
  kt                   # 启动器（纯脚本，无需安装）
  config.example.yaml  # 配置模板（复制为 config.yaml 后填写）
  config.yaml          # 本地配置：不入库（见 .gitignore）
  tests/               # 框架自带样例
    00_env.py          # ENV-*：环境健康（单点 case）
    05_mig.py          # MIG-*：命名版本在途升级（未配置 versions 则“受限”）
    10_ray.py          # REG-*：跨集群 Ray（一个流程 + 四个观测点）
    20_task.py         # TASK-*：单任务生命周期（一个流程 + 三个观测点）
    90_cleanup.py      # CLEAN-*：清理与最终状态
  test_framework.py    # 框架自检（无需集群）：python3 test_framework.py
  reports/             # 运行产物（.gitignore 忽略）
```

---

## 框架与测例分离

框架和测例放两个地方；**测例目录里只放自己的 `config.yaml` 和测例代码**，
运行只用框架自带的启动器（第 ① 种，纯脚本，无需安装）：

```bash
cd /your/own/tests                        # 你自己写测例的目录
KT=<rlark-repo>/test/kindtest/kt          # 指向本仓库框架的启动器（绝对或相对路径）
$KT validate -c config.yaml  # 校验配置
$KT plan     -c config.yaml  # 列出流程/观测点
$KT run      -c config.yaml  # 实机执行，实时写报告
$KT run      -c config.yaml --only REG     # 只跑某组（组名/流程名精确匹配）
$KT run      -c config.yaml --only REG-01  # 只跑某项（id 精确匹配）
$KT run      -c config.yaml --only 'REG-0*' # 尾部 * 通配，按前缀选多项
$KT report   -c config.yaml  # 由 manifest 重新生成报告
```

测例代码**只 import `kindtest`**，不引用框架目录里的任何文件。`config.yaml` 里
`tests.dir` / `report.dir` / `manifests.dir` 相对配置文件解析，也接受绝对路径。

框架自带样例的用法：

```bash
cd <rlark-repo>/test/kindtest
cp config.example.yaml config.yaml   # 改成你自己的运行环境，config.yaml 不入库
./kt validate -c config.yaml
./kt plan     -c config.yaml
./kt run      -c config.yaml
```

---

## 写测例

### 一个流程 + 多个观测点

```python
from kindtest import group

reg = group("REG", title="基本功能回归")

# ① 装饰器定义流程，items= 先声明本次流程的所有观测点（报告全表）
@reg.flow("跨集群 StatefulSet Ray", items=[
    ("REG-01", "Ray Active",  "head+2 worker，3 Active"),
    ("REG-02", "rank 和放置", "head 与 worker 落在配置的节点"),
])
def cross_cluster_ray(t):
    job = f"{t.run_id}-ray"
    t.delete_job(job)
    t.apply(t.render(RAY_JOB), on="mgmt")
    try:
        t.require(lambda: t.kexists("pod", t.head_pod(job), on="data", cluster="a"),
                  timeout=300, desc="head 出现")

        # ② 观测点：每个 with 产出一行报告
        with t.observe("REG-01"):
            t.evidence(t.kexec("a", t.head_pod(job), "ray", "status").combined(), "REG-ray.txt")
            t.ge(t.ray_active("a", t.head_pod(job)), 3, "Ray Active")
        with t.observe("REG-02"):
            t.eq(t.pod_node("a", t.head_pod(job)), t.config.get("placement.a"), "head 节点")
    finally:
        t.delete_job(job)
```

语义：

* 流程启动时，`items=` 声明的测试项先建成「未执行」的全表；
* 每退出一个 `observe`，实时刷新该行（断言累加成「实际结果」）；
* 流程中途抛 `Blocked`，已声明但未观测到的项统一标 `Blocked`；
* 同一 id 可在多个观测点出现，结果合并。

也可以用叠加的 `@group.item(...)` 声明观测点：

```python
@reg.flow("跨集群 StatefulSet Ray")
@reg.item("REG-01", "Ray Active", expect="3 Active")
@reg.item("REG-02", "rank 和放置", expect="head 与 worker 落在配置的节点")
def cross_cluster_ray(t):
    ...
```

### 单点 case（一个观测点的流程）

```python
env = group("ENV", title="环境和组件健康")

@env.case("ENV-01", "Kind 节点容器", expect="配置中的集群节点容器全部运行", source="环境")
def kind_containers(t):
    names = [str(t.config.get("clusters.control.name"))] + [t.cl(k).name for k in t.data_keys()]
    expected = [n for name in names for n in t.kind.nodes(name)]
    up = set(t.kind.containers_up())
    return t.eq([n for n in expected if n not in up], [], "未运行节点容器")   # 返回值即“实际结果”
```

规则：

1. 第一个参数永远是上下文 `t`，其余参数按名字从 **参数 / fixture** 注入。
2. 想记录「受限 / 前置不满足」，抛 `Restricted` / `Blocked`，或用 `t.restrict(...)` / `t.block(...)`。
3. 断言即事实：`t.eq/ge/...` 失败抛 `AssertionError`，成功返回可审计的事实串。

### 参数化：一行生成多行

```python
@env.case("ENV-02", "Kubernetes 节点 Ready", expect="各集群节点全部 Ready", source="环境",
          params=[{"cluster": "control"}, {"cluster": "a"}, {"cluster": "b"}],
          ids=["control", "data-a", "data-b"])
def nodes_ready(t, cluster):
    kube = t.control if cluster == "control" else t.data(cluster)
    lines = [ln for ln in kube("get", "nodes", "--no-headers").stdout.splitlines() if ln.strip()]
    return t.eq(len([l for l in lines if " Ready" in l]), len(lines), f"{cluster} Ready 节点")
```

生成 `ENV-02[control]`、`ENV-02[data-a]`、`ENV-02[data-b]`。

### 共享前置：生成器 fixture

```python
from kindtest import fixture

@fixture(scope="group")            # "group"（一组共享）或 "case"（每次执行独立）
def ray(t):
    t.delete_job("{{runId}}-ray")
    t.apply(t.render(RAY_JOB), on="mgmt")
    t.require(lambda: t.kexists("pod", head, on="data", cluster="a"), desc="head 出现")
    yield {"head": head, "workers": [...]}   # yield 之前是 setup
    t.delete_job("{{runId}}-ray")            # yield 之后是 teardown，永远执行
```

---

## 基本操作函数（挂在 `t` 上）

| 类别 | 函数 |
| --- | --- |
| 通用 kubectl | `t.kget / t.kjp / t.kjson / t.knames / t.kexists / t.kexec / t.klogs / t.kwait_pod` |
| 变更 | `t.apply / t.apply_manifest / t.kdelete / t.kpatch` |
| Job/Task | `t.delete_job / t.job_stopped / t.restart_job / t.task_name / t.task_uid / t.task_phase` |
| Pod/网络 | `t.head_pod / t.pod_ready / t.pod_node / t.dns_resolve / t.tcp_connect / t.ray_active` |
| Agent | `t.agent_image / t.agent_ready / t.set_agent_image / t.rollout_agent` |
| 版本（见下节） | `t.version_names / t.version_image / t.version_ref / t.current_version / t.upgrade(name) / t.deploy_version(name)` |
| 等待 | `t.wait / t.require / t.wait_pod / t.sleep` |
| 断言（累加进观测点） | `t.eq / t.ne / t.ge / t.le / t.truthy / t.contains / t.matches / t.succeeded / t.failed / t.nonempty` |
| 判定与证据 | `t.restrict / t.block / t.skip / t.evidence` |
| 句柄与模板 | `t.mgmt / t.control / t.data("a") / t.cl("a") / t.kind / t.render(text) / t.manifest_text(name) / t.vars` |

`on=` 取值 `mgmt`（KCP）/`control`/`data`；`data` 时带 `cluster="a"|"b"`；`ns` 省略即用各侧默认命名空间。
`Kube(...)` 直接调用是**原样** `kubectl`（不加默认命名空间）；需要默认命名空间时用上面的方法。
句柄自身的方法（`get/jp/json/exists/names/delete/apply/patch/exec/logs/rollout_status` 等）见 `kindtest/kube.py`；`t.kind` 提供宿主机 `kind`/`docker` 能力（节点容器、`image_id`、`load`）。

---

## 配置 `config.yaml`

模板见 `config.example.yaml`：复制为 `config.yaml`（不入库）后填你自己的环境。
只放环境事实。`tests.dir` / `report.dir` / `manifests.dir` 相对配置文件解析，也接受绝对路径。

```yaml
run:
  id: kt-1023                    # 对象名前缀 + rlark.io/test-run 标签值
  label: rlark.io/test-run
  domain: default
  theme: kindtest

clusters:
  control:
    name: control
    kubeconfig: /path/to/control/kubeconfig
    kcp: /path/to/control/kcp.kubeconfig
    namespace: rlark-system
  data:
    a: {name: worker-a, clusterId: rlark-data-a, kubeconfig: /path/to/worker-a/kubeconfig}
    b: {name: worker-b, clusterId: rlark-data-b, kubeconfig: /path/to/worker-b/kubeconfig}

versions:                      # 可选：多版本与升级，见“多版本与升级”
  initial: target
  definitions:
    base:   {ref: main@<base-sha>,   images: {rlark: rlark:baseline, ui: rlark-ui:baseline}}
    target: {ref: main@<target-sha>, images: {rlark: rlark:target,   ui: rlark-ui:target}}
  rollout:
    data:
      - {kind: deploy, name: rlark-agent, container: rlark-agent, slot: rlark, args: ["--image="]}
      - {kind: ds, name: rlark-agent-node, container: rlark-agent-node, slot: rlark}

images:
  task: task:test

report:
  dir: reports

tests:
  dir: tests
```

---

## 前置检查（preflight）

`kt run` 在任何流程开始之前先做前置检查，并在需要时先部署 `versions.initial`：

- PATH 中是否存在 `kubectl`；
- 每个 `Kube`（KCP、控制集群、各数据集群）的 kubeconfig 是否可读；
- 各集群是否可达（`kubectl get --raw=/readyz`）。

任一项失败、或初始版本部署失败，框架都会把**整个矩阵标记为 `Blocked`**（不是失败），
并在报告**顶层**用醒目横幅写明原因与修复提示。这样“环境根本没起来”不会被误读成
一堆断言失败或一份看似正常的报告。区块失败与 `Blocked` 都不计入通过。

---

## 多版本与升级

配置里可以定义**多个命名版本**（每个是 `{ref, image}`），并声明切换版本时要
滚动的工作负载。框架会在**任何流程开始之前**先部署 `versions.initial`；测例里
再按需调用 `t.upgrade(代号)` 在受控时点切到某个已定义版本。

```yaml
versions:
  initial: target              # 运行开始先部署的版本（代号）
  definitions:                 # 一个版本 = ref + 若干命名镜像（不同组件不同镜像）
    base:
      ref: main@aaaaaaa
      images: {rlark: rlark:baseline-aaaaaaa, ui: rlark-ui:baseline}
    target:
      ref: main@bbbbbbb
      images: {rlark: rlark:target-bbbbbbb, ui: rlark-ui:target}
  rollout:                     # 切换版本时滚动哪些负载，以及各自用哪个镜像槽
    control:                   # 控制集群内
      - {kind: deploy, name: rlark-controller-manager, container: rlark-controller-manager, slot: rlark}
      - {kind: deploy, name: rlark-gateway, container: rlark-gateway, slot: rlark}
      - {kind: deploy, name: rlark-server, container: rlark-server, slot: rlark}
      - {kind: deploy, name: rlark-ui, container: rlark-ui, slot: ui}
    data:                      # 每个数据集群内
      - {kind: deploy, name: rlark-agent, container: rlark-agent, slot: rlark, args: ["--image="]}
      - {kind: ds, name: rlark-agent-node, container: rlark-agent-node, slot: rlark}
```

规则：

* 一个版本可含**多个命名镜像**（`images: {rlark: …, ui: …}`）；`rollout` 的每个
  目标用 `slot:` 选择用哪个镜像。省略 `slot` 时只用 `default` 槽或唯一镜像；
  定义了多个镜像槽却没有 `default` 的版本**必须显式指定 `slot`**，否则报错，不会
  静默挑一个。单镜像版本也可写成 `image: <ref>`（等价于 `default` 槽）。UI 与
  rlark 是不同镜像，因此必须分开成两个槽。
* `rollout.control` 作用于控制集群，`rollout.data` 作用于**每个**数据集群；
  `kind` 取 `deploy` / `ds` / `sts`。切换时把该容器的镜像与 `args` 合并成**一次
  patch**（一次 rollout，无“新镜像 + 旧参数”的中间态），再等待 `rollout status`。
* 目标的 `args` 列出要改写的参数前缀（如 `--image=`）。框架只改写**已存在**的
  前缀；前缀缺失时默认报错（避免悄悄给容器追加参数、让它跑在与环境不同的配置上），
  确需缺失即追加时在该目标上设置 `appendArgs: true`。
* **幂等**：live 镜像已等于目标版本时不做改动，只校验就绪；重复调用或部署
  当前版本都很便宜。
* `versions.rollout` 缺省时框架**不实际切换**，只在报告登记版本声明；
  `t.upgrade()` 此时抛 `Restricted`。未配置 `versions` 时一切照旧，
  `t.version_names()` 返回空列表。
* 配置 `versions` 后，报告的“目标版本/目标镜像”取 `initial` 的 `ref`/`image`，
  “基线”取 `definitions` 中名为 `base`/`baseline` 的版本（若有）。**取值**接口
  （`config.target_image`/`baseline_image`、`t.version_image(代号, 槽)`、
  `{{targetImage}}`/`{{baselineImage}}`）按严格规则解析：用 `default` 槽或唯一
  镜像；同一版本多槽且无 `default` 时必须指定槽，否则抛 `VersionError`，不会
  静默挑一个。**展示**接口（报告“镜像”列）则列出全部槽（`rlark=…、ui=…`）。
  模板里用 `{{image.<代号>.<槽>}}` 引用具体镜像；`{{image.<代号>}}` 在唯一/default
  时给出该镜像、多槽时列出全部槽；`{{targetImage}}`/`{{baselineImage}}` 只在模板
  真正用到时解析，未用到的模板不会因目标镜像不明确而报错。

测例写法：

```python
from kindtest import Restricted, group

mig = group("MIG", title="在途升级和不变量")

@mig.flow("命名版本在途升级", items=[
    ("MIG-01", "初始版本就绪", "运行开始部署的 initial 可被识别"),
    ("MIG-02", "升级到目标版本", "t.upgrade(目标) 后镜像切换且 Ready"),
])
def named_version_upgrade(t):
    if not t.versions.enabled or not t.versions.rollout_configured:
        raise Restricted("未配置 versions.rollout")     # 整组记为“受限”
    with t.observe("MIG-01"):
        t.eq(t.current_version(), t.versions.initial, "当前版本")
    with t.observe("MIG-02"):
        t.upgrade("target")             # 也可写 t.deploy_version("target")
        t.eq(t.current_version(), "target", "升级后版本")
```

`t.current_version()` 通过比对 live 目标镜像与 `definitions` 得出；`t.upgrade(name)`
对未知代号抛 `VersionError`，rollout 超时同样抛错（观测点内记为失败，观测点外
中止该流程并把其余项标 `Blocked`）。框架自带样例见 `tests/05_mig.py`。

---

## 报告与证据

```
<report.dir>/
  full-test-result.md          # 实时更新的矩阵报告
  run-manifest.json            # 机器可读结果
  run-<runId>.log              # 每条命令与抛错
  evidence/                    # t.evidence 追加的证据
```

矩阵类别顺序为 `ENV → BUILD → UNIT → BIND → MIG → CRD → REG → TASK → SSH → VALID → OBS → CLEAN`；
目标 ref 变化时，上一轮报告、清单与证据自动存档为 `full-test-result-<oldsha>.md`、
`run-manifest-<oldsha>.json`、`evidence-<oldsha>/`。

---

## 约定

- 每轮 Job/Task 用唯一 `run.id` 前缀与配置的标签（默认 `rlark.io/test-run`），只清理本轮拥有的对象。
- 不触碰与本轮无关的既有对象。
- 只观察与报告，不为让测试通过而改动产品代码。
