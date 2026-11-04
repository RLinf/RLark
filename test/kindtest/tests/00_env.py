"""ENV: layered environment health - read-only checks on the live clusters.

This file is a good template: each case is a small function, the parametrized
ones share one body, and every assertion returns an auditable fact.
"""

from __future__ import annotations

from kindtest import group

env = group("ENV", title="环境和组件健康")


@env.case("ENV-01", "Kind 节点容器", expect="配置中的集群节点容器全部运行", source="环境")
def kind_containers(t):
    names = [str(t.config.get("clusters.control.name"))] + [t.cl(k).name for k in t.data_keys()]
    expected = [n for name in names for n in t.kind.nodes(name)]
    up = set(t.kind.containers_up())
    missing = [n for n in expected if n not in up]
    assert expected, "未发现任何 Kind 节点容器"
    assert not missing, f"未运行节点容器: {missing}"
    return f"{len(expected)} 个节点容器 Up"


@env.case(
    "ENV-02",
    "Kubernetes 节点 Ready",
    expect="各集群节点全部 Ready",
    source="环境",
    params=[{"cluster": "control"}, {"cluster": "a"}, {"cluster": "b"}],
    ids=["control", "a", "b"],
)
def nodes_ready(t, cluster):
    kube = t.control if cluster == "control" else t.data(cluster)
    lines = [ln for ln in kube("get", "nodes", "--no-headers").stdout.splitlines() if ln.strip()]
    ready = [ln for ln in lines if " Ready" in ln]
    return t.eq(len(ready), len(lines), f"{cluster} Ready 节点") + f"/{len(lines)}"


@env.case("ENV-03", "控制面组件", expect="KCP/PostgreSQL/Server/Gateway/Controller Manager/UI Ready", source="环境")
def control_plane(t):
    lines = [ln for ln in t.control("get", "pods", "-n", t.config.control_namespace,
                                    "--no-headers").stdout.splitlines() if ln.strip()]
    ready, bad = 0, []
    for line in lines:
        parts = line.split()
        if len(parts) < 3 or "/" not in parts[1]:
            continue
        got, want = parts[1].split("/")
        if got == want and parts[2] in ("Running", "Completed"):
            ready += 1
        else:
            bad.append(f"{parts[0]}({parts[2]})")
    assert not bad, f"异常组件: {bad}"
    assert ready >= 6, f"控制面 Ready Pod 过少: {ready}"
    return f"{ready} 个控制面 Pod Running/Completed"


@env.case(
    "ENV-04",
    "数据面 Agent",
    expect="两 Cluster Agent 与 Node Agent Ready",
    source="环境",
    params=[{"cluster": "a"}, {"cluster": "b"}],
    ids=["a", "b"],
)
def data_agents(t, cluster):
    cl = t.cl(cluster)
    dep = cl.data.jp("deploy", "rlark-agent", "{.status.readyReplicas}/{.spec.replicas}")
    ds = cl.data.jp("ds", "rlark-agent-node", "{.status.numberReady}/{.status.desiredNumberScheduled}")
    _all_ready(dep, "Cluster Agent")
    _all_ready(ds, "Node Agent")
    return f"deploy={dep} ds={ds}"


@env.case("ENV-05", "管理面 API 和 CRD", expect="KCP API 可访问且 rlinf.io CRD 已注册", source="环境")
def management_api(t):
    ran = t.succeeded(t.mgmt("api-resources", "--api-group=rlinf.io", "--no-headers"), "api-resources")
    names = {ln.split()[0] for ln in ran.stdout.splitlines() if ln.strip()}
    required = {"jobs", "tasks", "workflows", "domains", "domainpeers", "nodes", "pods", "addons"}
    missing = sorted(required - names)
    assert not missing, f"缺少 CRD 资源: {missing}"
    return f"{len(names)} 个 rlinf.io 资源可用"


@env.case(
    "ENV-06",
    "默认 StorageClass",
    expect="两侧数据集群均有默认 StorageClass",
    source="环境",
    params=[{"cluster": "a"}, {"cluster": "b"}],
    ids=["a", "b"],
)
def default_storageclass(t, cluster):
    out = t.data(cluster)("get", "sc", "--no-headers").stdout
    defaults = [ln.split()[0] for ln in out.splitlines() if "(default)" in ln]
    assert defaults, "未发现默认 StorageClass"
    return f"default={','.join(defaults)}"


@env.case("ENV-07", "基础镜像预检", expect="配置的离线基础镜像保留在宿主", source="环境")
def base_images(t):
    wanted = [str(x) for x in (t.config.get("images.base") or [])]
    if not wanted:
        t.restrict("未配置 images.base，跳过基础镜像预检")
        return "未配置 images.base"
    present = [img for img in wanted if t.kind.image_id(img)]
    missing = [img for img in wanted if img not in present]
    assert not missing, f"缺失基础镜像: {missing}"
    return f"{len(present)}/{len(wanted)} 个离线基础镜像保留"


def _all_ready(ratio: str, what: str) -> None:
    got, _, want = ratio.partition("/")
    assert got and want and got == want, f"{what} 未就绪: {ratio or 'n/a'}"
