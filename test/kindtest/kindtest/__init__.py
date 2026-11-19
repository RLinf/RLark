"""kindtest - a code-first Kind regression framework for RLark.

Configuration holds only environment facts (clusters, images, named versions,
report location).  Tests are ordinary Python functions and follow the reporting
model

    测试流程 (flow) -> 测试观测点 (observation point) -> 测试项 (report item)

Quick start::

    from kindtest import group

    reg = group("REG", title="基本功能回归")

    @reg.flow("跨集群 Ray", items=[
        ("REG-01", "Ray Active", "3 Active"),
        ("REG-02", "rank 和放置", "head 与 worker 落在配置的节点"),
    ])
    def cross_cluster_ray(t):
        job = f"{t.run_id}-ray"
        t.delete_job(job)
        t.apply(t.render(RAY_JOB), on="mgmt")
        try:
            t.require(lambda: t.kexists("pod", t.head_pod(job), on="data", cluster="a"),
                      timeout=300, desc="head 出现")
            with t.observe("REG-01"):
                t.ge(t.ray_active("a", t.head_pod(job)), 3, "Ray Active")
            with t.observe("REG-02"):
                t.eq(t.pod_node("a", t.head_pod(job)), t.config.get("placement.a"), "head 节点")
        finally:
            t.delete_job(job)

A single-observation case is just sugar::

    @env.case("ENV-01", "Kind 节点容器", expect="全部运行")
    def kind_containers(t):
        return f"{len(t.kind.containers_up())} 个容器 Up"

See ``README.md`` and the ``tests/`` directory.
"""

from __future__ import annotations

from .context import Context
from .kube import Cluster, Kind, Kube
from .model import Case, RunReport, Status
from .registry import Blocked, Restricted, fixture, group
from .versions import VersionError

__all__ = [
    "__version__",
    "group",
    "case",
    "flow",
    "fixture",
    "Blocked",
    "Restricted",
    "VersionError",
    "Context",
    "Cluster",
    "Kube",
    "Kind",
    "Case",
    "RunReport",
    "Status",
]

__version__ = "0.2.0"


def _default_group(kwargs: dict):
    from .registry import group as _group

    return _group(kwargs.pop("group", "MISC"), title=kwargs.pop("title", None))


def case(*args, **kwargs):
    """Decorator ``@case(...)`` using the default group.

    Prefer ``@mygroup.case(...)``.  This helper exists so a single-category test
    file can avoid creating an explicit group object.
    """
    return _default_group(kwargs).case(*args, **kwargs)


def flow(*args, **kwargs):
    """Decorator ``@flow(...)`` using the default group (see :func:`case`)."""
    return _default_group(kwargs).flow(*args, **kwargs)
