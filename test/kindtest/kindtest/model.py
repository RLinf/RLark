"""Result data model: one :class:`Case` per reported matrix row.

The status vocabulary follows the project's reporting conventions so the
generated report never silently turns a restricted/blocked observation into a
pass.
"""

from __future__ import annotations

import enum
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone


def utcnow() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


class Status(str, enum.Enum):
    PASS = "通过"
    FAIL = "失败"
    RESTRICTED = "受限"
    BLOCKED = "Blocked"
    SKIPPED = "未执行"

    @property
    def counted(self) -> bool:
        """True when the status represents a recorded observation."""
        return self is not Status.SKIPPED


@dataclass
class Case:
    id: str
    name: str
    expected: str = ""
    actual: str = ""
    status: Status = Status.SKIPPED
    source: str = "Kind"
    evidence: list[str] = field(default_factory=list)
    duration_s: float = 0.0

    @property
    def category(self) -> str:
        return self.id.split("-", 1)[0].split("[", 1)[0]

    def to_dict(self) -> dict:
        data = asdict(self)
        data["status"] = self.status.value
        data["category"] = self.category
        return data


CATEGORY_ORDER: list[str] = [
    "ENV",
    "BUILD",
    "UNIT",
    "BIND",
    "MIG",
    "CRD",
    "REG",
    "TASK",
    "SSH",
    "VALID",
    "OBS",
    "CLEAN",
]

CATEGORY_TITLE: dict[str, str] = {
    "ENV": "环境和组件健康",
    "BUILD": "镜像构建和加载",
    "UNIT": "单元测试和静态检查",
    "BIND": "新契约定向测试",
    "MIG": "在途升级和不变量",
    "CRD": "CRD Schema",
    "REG": "基本功能回归",
    "TASK": "生命周期和错误恢复",
    "SSH": "工作负载 SSH 访问",
    "VALID": "校验和拒绝",
    "OBS": "可观测性和 root metadata",
    "CLEAN": "清理和最终状态",
}


@dataclass
class RunReport:
    theme: str
    run_id: str
    base_ref: str
    target_ref: str
    started: str
    finished: str = ""
    config: dict = field(default_factory=dict)
    cases: list[Case] = field(default_factory=list)
    evidence_dir: str = ""
    environment: dict = field(default_factory=dict)
    blocked_reason: str = ""
    notes: dict = field(default_factory=dict)

    def counts(self) -> dict[str, int]:
        out = {"pass": 0, "fail": 0, "restricted": 0, "blocked": 0, "skipped": 0}
        for case in self.cases:
            out[_status_key(case.status)] += 1
        return out

    def by_category(self) -> dict[str, list[Case]]:
        grouped: dict[str, list[Case]] = {}
        for case in self.cases:
            grouped.setdefault(case.category, []).append(case)
        for cases in grouped.values():
            cases.sort(key=lambda c: _id_sort_key(c.id))
        return grouped

    def category_counts(self, key: str) -> dict[str, int]:
        out = {"pass": 0, "fail": 0, "restricted": 0, "blocked": 0, "skipped": 0}
        for case in self.cases:
            if case.category == key:
                out[_status_key(case.status)] += 1
        return out

    def to_dict(self) -> dict:
        data = asdict(self)
        data["cases"] = [c.to_dict() for c in self.cases]
        data["counts"] = self.counts()
        return data


def _status_key(status: Status) -> str:
    return {
        Status.PASS: "pass",
        Status.FAIL: "fail",
        Status.RESTRICTED: "restricted",
        Status.BLOCKED: "blocked",
        Status.SKIPPED: "skipped",
    }[status]


def _id_sort_key(case_id: str):
    prefix, _, suffix = case_id.partition("-")
    numeric = suffix.split("[", 1)[0]
    try:
        return (prefix, 0, int(numeric), suffix)
    except ValueError:
        return (prefix, 1, 0, suffix)
