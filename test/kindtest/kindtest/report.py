"""Markdown report generation aligned with the project's reporting conventions."""

from __future__ import annotations

from pathlib import Path

from .config import Config
from .model import CATEGORY_ORDER as DEFAULT_CATEGORY_ORDER
from .model import CATEGORY_TITLE as DEFAULT_CATEGORY_TITLE
from .model import Case, RunReport, Status


def write_report(report: RunReport, config: Config) -> Path:
    path = config.report_dir / config.report_file
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(render(report, config))
    return path


def render(report: RunReport, config: Config) -> str:
    counts = report.counts()
    lines: list[str] = [f"# {config.title}", "", _banner(report, counts), ""]
    if report.blocked_reason:
        lines += _blocked(report)
    lines += _criteria(config)
    lines += _baseline(report, config)
    lines += _summary(report, config)
    lines += _details(report, config)
    lines += _findings(config)
    lines += _conclusion(report, config, counts)
    lines += _environment(report)
    lines += _evidence_index(report)
    lines.append("")
    lines.append(_footer())
    return "\n".join(lines) + "\n"


def _banner(report: RunReport, counts: dict[str, int]) -> str:
    status = "已完成" if report.finished else "进行中（实时更新）"
    stamp = report.finished or report.started
    total = len(report.cases)
    done = total - counts["skipped"]
    where = f"目标 `{report.target_ref}`。" if report.target_ref else ""
    if report.base_ref:
        where = f"基线 `{report.base_ref}`，目标 `{report.target_ref}`。"
    return (
        f"> 状态：**{status}**。{where}"
        f"已记录 {done}/{total} 项：{counts['pass']} 通过 / {counts['restricted']} 受限 / "
        f"{counts['fail']} 分支相关失败 / {counts['blocked']} Blocked / {counts['skipped']} 未执行。"
        f"（更新于 {stamp}）"
    )


def _blocked(report: RunReport) -> list[str]:
    return [
        "",
        f"> ⛔ **运行未开始（环境/setup 未通过）**：{report.blocked_reason}",
        ">",
        "> 这是前置条件问题，不是测试项的失败；修复环境后重跑，不要按失败项排查。",
    ]


def _criteria(config: Config) -> list[str]:
    out = ["", "## 判据结论", ""]
    items = config.get("notes.criteria") or []
    if items:
        out += [f"- {x}" for x in items]
    else:
        out.append("- （在配置 `notes.criteria` 中填写判据结论）")
    return out


def _baseline(report: RunReport, config: Config) -> list[str]:
    out = ["", "## 测试基线", "", "| 项目 | 值 |", "| --- | --- |"]
    rows = []
    if report.base_ref:
        rows.append(("基线版本", f"`{report.base_ref}`"))
    rows.append(("目标版本", f"`{report.target_ref}`"))
    if config.baseline_image_display:
        rows.append(("基线镜像", f"`{config.baseline_image_display}`"))
    rows.append(("目标镜像", f"`{config.target_image_display}`"))
    rows.append(("Task 镜像", f"`{config.task_image}`"))
    if config.versions_definitions:
        rows.append(("初始版本", f"`{config.versions_initial}`"))
        rows.append(("定义版本", _versions_cell(config)))
    rows += [
        ("控制集群", f"`{config.get('clusters.control.name', '')}`"),
        ("数据集群", "、".join(f"`{v.get('name')}`" for v in config.data_clusters().values())),
        ("测试标签", f"`{config.label_key}={config.run_id}*`；名称前缀 `{config.run_id}*`"),
    ]
    excluded = config.get("notes.excluded")
    if excluded:
        rows.append(("排除集群", str(excluded)))
    out += [f"| {k} | {v} |" for k, v in rows]
    return out


def _versions_cell(config: Config) -> str:
    initial = config.versions_initial
    parts = []
    for name, entry in config.versions_definitions.items():
        entry = entry or {}
        mark = "（初始）" if name == initial else ""
        images = entry.get("images")
        if isinstance(images, dict) and images:
            shown = "、".join(f"{k}=`{v}`" for k, v in images.items())
        else:
            shown = f"`{entry.get('image', '')}`"
        detail = f"`{name}`{mark}: {shown}"
        if entry.get("ref"):
            detail += f" @ `{entry['ref']}`"
        parts.append(detail)
    return "；".join(parts)


def _summary(report: RunReport, config: Config) -> list[str]:
    out = ["", "## 总体汇总", "",
           "| 测试类别 | 通过 | 受限 | 分支相关失败 | Blocked | 未执行 | 结论 |",
           "| --- | ---: | ---: | ---: | ---: | ---: | --- |"]
    order, titles = _category_plan(report)
    total = {"pass": 0, "restricted": 0, "fail": 0, "blocked": 0, "skipped": 0}
    for key in order:
        cc = report.category_counts(key)
        if not any(cc.values()):
            continue
        for k in total:
            total[k] += cc[k]
        out.append(
            f"| {_title(key, titles)} ({key}) | {cc['pass']} | {cc['restricted']} | {cc['fail']} | "
            f"{cc['blocked']} | {cc['skipped']} | {_verdict(cc)} |"
        )
    out.append(
        f"| **合计** | **{total['pass']}** | **{total['restricted']}** | **{total['fail']}** | "
        f"**{total['blocked']}** | **{total['skipped']}** | "
        f"**{len(report.cases)} 项：{total['pass']} 通过 / {total['restricted']} 受限 / {total['fail']} 分支相关失败** |"
    )
    return out


def _details(report: RunReport, config: Config) -> list[str]:
    out: list[str] = []
    grouped = report.by_category()
    order, titles = _category_plan(report)
    for key in order:
        cases = grouped.get(key)
        if not cases:
            continue
        out += ["", f"## {_title(key, titles)}", "",
                "| ID | 测试项 | 预期 | 实际结果 | 来源 | 状态 |",
                "| --- | --- | --- | --- | --- | --- |"]
        for case in cases:
            actual = case.actual or ""
            if case.evidence:
                actual = f"{actual}（`{', '.join(case.evidence)}`）"
            out.append(
                f"| {case.id} | {case.name} | {case.expected} | {actual} | {case.source} | {case.status.value} |"
            )
    return out


def _findings(config: Config) -> list[str]:
    items = config.get("notes.findings") or []
    if not items:
        return []
    out = ["", "## 发现的问题", ""]
    out += [f"{i}. {x}" for i, x in enumerate(items, start=1)]
    return out


def _conclusion(report: RunReport, config: Config, counts: dict[str, int]) -> list[str]:
    out = ["", "## 最终结论", ""]
    if report.blocked_reason:
        out += [f"**运行未开始（环境/setup 未通过）**：{report.blocked_reason}", ""]
    for para in config.get("notes.conclusion") or []:
        out += [para, ""]
    out.append(
        f"**汇总：矩阵 {len(report.cases)} 项 = {counts['pass']} 通过 / {counts['restricted']} 受限 / "
        f"{counts['fail']} 分支相关失败 / {counts['blocked']} Blocked / {counts['skipped']} 未执行。**"
    )
    return out


def _environment(report: RunReport) -> list[str]:
    env = report.environment or {}
    if not env:
        return []
    out = ["", "## 最终环境状态", "", "| 项目 | 最终值 |", "| --- | --- |"]
    out += [f"| {k} | {v} |" for k, v in env.items()]
    return out


def _evidence_index(report: RunReport) -> list[str]:
    usage: dict[str, list[str]] = {}
    for case in report.cases:
        for ev in case.evidence:
            usage.setdefault(ev, []).append(case.id)
    if not usage:
        return []
    out = ["", "## 附：证据索引", "", "| 证据文件 | 覆盖项 |", "| --- | --- |"]
    for name in sorted(usage):
        out.append(f"| `{name}` | {', '.join(sorted(set(usage[name])))} |")
    return out


def _footer() -> str:
    return (
        "> 证据来源标注：Kind=真实数据面端到端；单元测试=确定性 go test；源码=静态检查；"
        "构建=镜像构建与 digest；Git=版本核对；环境=宿主资源。"
    )


def _category_plan(report: RunReport) -> tuple[list[str], dict[str, str]]:
    present = {case.category for case in report.cases}
    order = list(DEFAULT_CATEGORY_ORDER)
    for key in sorted(present):
        if key not in order:
            order.append(key)
    return order, dict(DEFAULT_CATEGORY_TITLE)


def _title(key: str, titles: dict[str, str]) -> str:
    return titles.get(key) or key


def _verdict(cc: dict[str, int]) -> str:
    if cc["fail"]:
        return "存在分支相关失败"
    if cc["blocked"]:
        return "存在 Blocked"
    if cc["restricted"]:
        return "受限"
    if cc["skipped"]:
        return "部分未执行"
    return "全部通过"
