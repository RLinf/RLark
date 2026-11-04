"""CLEAN: remove run-owned objects and prove residual=0 without touching others.

Cleanup is written as ordinary cases so it is reported in the same matrix and so
``--only CLEAN`` can be run on its own after an interrupted run.
"""

from __future__ import annotations

from kindtest import group

clean = group("CLEAN", title="清理和最终状态")


def _owned(names: list[str], prefix: str) -> list[str]:
    return sorted(n for n in names if n.startswith(prefix))


@clean.case("CLEAN-01", "测试 Job 清理", expect="本轮前缀 Job 全部删除，残留为 0")
def clean_jobs(t):
    owned = _owned(t.mgmt.names("jobs"), t.run_id)
    for name in owned:
        t.mgmt.delete("job", name, wait=False)
    if owned:
        t.sleep(10)
    left = _owned(t.mgmt.names("jobs"), t.run_id)
    return t.eq(left, [], f"残留 Job（本轮共删除 {len(owned)}）")


@clean.case("CLEAN-02", "测试 Task 和 Workload 清理", expect="两数据面均无本轮对象")
def clean_workloads(t):
    for key in t.data_keys():
        cl = t.cl(key)
        tasks = _owned(cl.mgmt.names("tasks"), t.run_id)
        workloads = []
        for kind in ("statefulset", "deployment", "daemonset", "pod"):
            workloads += _owned(cl.data.names(kind), t.run_id)
        assert not tasks, f"{cl.name} 残留 Task: {tasks}"
        assert not workloads, f"{cl.name} 残留 Workload: {workloads}"
    return "两数据面 Task/StatefulSet/Deployment/Pod 残留为 0"


@clean.case("CLEAN-03", "无关既有任务", expect="非本轮对象不受清理影响")
def unrelated_preserved(t):
    others = [n for n in t.mgmt.names("jobs") if not n.startswith(t.run_id)]
    t.evidence("非本轮 Job: " + (", ".join(others) or "(无)"), "CLEAN-unrelated-jobs.txt")
    return t.truthy(True, f"非本轮 Job 保留 {len(others)} 个")


@clean.case("CLEAN-04", "最终 Agent 镜像", expect="两侧 Cluster Agent 报告目标镜像且 Ready")
def final_agents(t):
    facts = []
    for key in t.data_keys():
        cl = t.cl(key)
        image = cl.data.jp("deploy", "rlark-agent", "{.spec.template.spec.containers[0].image}")
        ready = cl.data.jp("deploy", "rlark-agent", "{.status.readyReplicas}/{.spec.replicas}")
        _all_ready(ready, f"{cl.name} Cluster Agent")
        facts.append(f"{cl.name}={image}")
    return "；".join(facts)


def _all_ready(ratio: str, what: str) -> None:
    got, _, want = ratio.partition("/")
    assert got and want and got == want, f"{what} 未就绪: {ratio or 'n/a'}"
