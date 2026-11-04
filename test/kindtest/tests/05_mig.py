"""MIG: 在途升级 - 用配置里定义的命名版本，在流程中途要求框架升级。

只有 ``config.yaml`` 定义了 ``versions`` **且** ``versions.rollout`` 时才能
真正执行；否则整组记为“受限”。框架会在这组流程之前先部署
``versions.initial``，因此这里只需要 ``t.upgrade(name)`` 在受控时点切换。

配置示例见 README“多版本与升级”与 ``config.example.yaml``：:

    versions:
      initial: target
      definitions:
        base:   {ref: main@aaaaaaa, images: {rlark: rlark:baseline, ui: rlark-ui:baseline}}
        target: {ref: main@bbbbbbb, images: {rlark: rlark:target,   ui: rlark-ui:target}}
      rollout:
        data:
          - {kind: deploy, name: rlark-agent, container: rlark-agent, slot: rlark, args: ["--image="]}
          - {kind: ds, name: rlark-agent-node, container: rlark-agent-node, slot: rlark}
"""

from __future__ import annotations

from kindtest import Restricted, group

mig = group("MIG", title="在途升级和不变量")


@mig.flow(
    "命名版本在途升级",
    items=[
        ("MIG-01", "初始版本就绪", "运行开始部署的初始版本可被识别且目标 Ready"),
        ("MIG-02", "升级到目标版本", "t.upgrade(目标) 后镜像切到目标且 Ready"),
        ("MIG-03", "版本可回退", "t.upgrade(初始) 后镜像回到初始"),
    ],
)
def named_version_upgrade(t):
    if not t.versions.enabled or not t.versions.rollout_configured:
        raise Restricted("未配置 versions.rollout，跳过在途升级场景")

    initial = t.versions.initial
    target = next((n for n in t.version_names() if n != initial), initial)

    with t.observe("MIG-01"):
        current = t.current_version()
        t.truthy(current == initial, "初始版本已部署",
                  f"detected={current!r} initial={initial!r}")
        t.evidence(f"detected={current} initial={initial}", "MIG-initial.txt")

    with t.observe("MIG-02"):
        result = t.upgrade(target)
        t.eq(t.current_version(), target, "升级后版本")
        t.evidence(
            f"{initial} -> {target} images={result.images} changed={result.changed}", "MIG-upgrade.txt"
        )

    with t.observe("MIG-03"):
        result = t.upgrade(initial)
        t.eq(t.current_version(), initial, "回退后版本")
        t.evidence(f"{target} -> {initial} images={result.images} changed={result.changed}", "MIG-rollback.txt")
