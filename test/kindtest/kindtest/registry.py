"""Test authoring registry - 测试流程 (flows), 观测点 (observations), 测试项 (items).

The public surface mirrors the vocabulary used in the reports:

* a **group** (``ENV``/``REG``/...) is a reporting category;
* a **flow** (测试流程) is a decorated procedure that runs *once* and may touch
  the environment; it is defined with :meth:`Group.flow` and may declare its
  observation points up front via ``items=`` and/or ``@group.item(...)``;
* an **observation point** (测试观测点) is a block inside a flow that evaluates
  one or more **items** (测试项) - the rows of the report - and recorded with
  ``with t.observe("REG-01"):``;
* a **case** is a one-line sugar for "a flow with a single observation point",
  registered with :meth:`Group.case` (optionally parametrized).

Flow functions are ordinary Python functions.  They receive the run context
(``t``) first, then any fixtures/parameters they name in their signature.  A
flow/case passes by raising nothing; it fails by raising.  ``Blocked`` and
``Restricted`` raise to record those non-pass verdicts - outside an observation
they abort the flow and mark every not-yet-observed declared item.
"""

from __future__ import annotations

import inspect
from dataclasses import dataclass, field
from typing import Any, Callable, Iterable, Mapping, Sequence


class Blocked(Exception):
    """A precondition could not be established; the item is reported Blocked."""


class Restricted(Exception):
    """Observed, but limited by an existing/environmental constraint."""


class RegistryError(RuntimeError):
    """The registered tests are invalid, e.g. two items share an id."""


@dataclass
class FixtureDef:
    name: str
    fn: Callable[..., Any]
    scope: str  # "group" | "case"


@dataclass
class Item:
    """One test item - a single row in the report matrix (测试项)."""

    id: str
    name: str
    expect: str = ""
    source: str = "Kind"


@dataclass
class FlowSpec:
    """A registered procedure (测试流程) or single-observation case."""

    group: "Group"
    name: str
    items: list[Item]
    fn: Callable[..., Any]
    kind: str = "flow"  # "flow" | "case"
    params: list[dict] | None = None
    ids: list[str] | None = None


@dataclass
class FlowRun:
    """A concrete execution of a flow: one params combination, expanded items."""

    group_key: str
    group_title: str
    name: str
    kind: str
    fn: Callable[..., Any]
    items: list[Item] = field(default_factory=list)
    values: dict = field(default_factory=dict)


class Group:
    def __init__(self, key: str, title: str, order: int):
        self.key = key
        self.title = title
        self.order = order
        self.flows: list[FlowSpec] = []

    # ------------------------------------------------------------- authoring
    def flow(
        self,
        name: str,
        items: Sequence[Any] | None = None,
        *,
        source: str = "Kind",
        params: list[dict] | None = None,
        ids: list[str] | None = None,
    ) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
        """Register a procedure (测试流程) run once, with declared observation points.

        ``items`` may mix tuples ``(id, name[, expect])``, mappings with those
        keys, or :class:`Item`.  Additional observation points may be declared by
        stacking ``@group.item(...)`` under the ``@group.flow(...)`` decorator.
        Items are ordered by id so the report matrix is deterministic.
        """

        declared = _normalize_items(items or [], source)

        def deco(fn: Callable[..., Any]) -> Callable[..., Any]:
            extra = _normalize_items(getattr(fn, "_kindtest_items", []), source)
            all_items = sorted(declared + extra, key=lambda it: _item_sort_key(it.id))
            if not all_items:
                raise ValueError(
                    f"流程 {name!r} 未声明任何观测点：使用 items= 或 @group.item(...)"
                )
            self.flows.append(
                FlowSpec(self, name, all_items, fn, kind="flow", params=params, ids=ids)
            )
            return fn

        return deco

    def item(
        self, id: str, name: str, *, expect: str = "", source: str = "Kind"
    ) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
        """Declare one observation point of the flow decorated below/above."""

        def deco(fn: Callable[..., Any]) -> Callable[..., Any]:
            existing = list(getattr(fn, "_kindtest_items", []))
            existing.append(Item(id, name, expect, source))
            fn._kindtest_items = existing
            return fn

        return deco

    def case(
        self,
        id: str,
        name: str,
        *,
        expect: str = "",
        source: str = "Kind",
        params: list[dict] | None = None,
        ids: list[str] | None = None,
    ) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
        """Register a single-observation case (sugar for a one-item flow)."""

        def deco(fn: Callable[..., Any]) -> Callable[..., Any]:
            item = Item(id, name, expect, source)
            self.flows.append(
                FlowSpec(self, name, [item], fn, kind="case", params=params, ids=ids)
            )
            return fn

        return deco

    # ------------------------------------------------------------------ plan
    def collect(self) -> list[FlowRun]:
        runs: list[FlowRun] = []
        for spec in self.flows:
            runs += _expand(spec)
        return runs


# --------------------------------------------------------------------------
# global registries
# --------------------------------------------------------------------------
GROUPS: dict[str, Group] = {}
_GROUP_ORDER: list[str] = []
FIXTURES: dict[str, FixtureDef] = {}


def group(key: str, title: str | None = None) -> Group:
    if key not in GROUPS:
        GROUPS[key] = Group(key, title or key, len(_GROUP_ORDER))
        _GROUP_ORDER.append(key)
    return GROUPS[key]


def fixture(*, scope: str = "group", name: str | None = None) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    if scope not in ("group", "case"):
        raise ValueError("fixture scope must be 'group' or 'case'")

    def deco(fn: Callable[..., Any]) -> Callable[..., Any]:
        FIXTURES[name or fn.__name__] = FixtureDef(name or fn.__name__, fn, scope)
        return fn

    return deco


# --------------------------------------------------------------------------
# helpers
# --------------------------------------------------------------------------
def _normalize_items(raw: Iterable[Any], default_source: str) -> list[Item]:
    out: list[Item] = []
    for entry in raw:
        if isinstance(entry, Item):
            out.append(entry)
        elif isinstance(entry, Mapping):
            out.append(
                Item(
                    str(entry["id"]),
                    str(entry.get("name", entry["id"])),
                    str(entry.get("expect", "")),
                    str(entry.get("source", default_source)),
                )
            )
        elif isinstance(entry, (tuple, list)):
            seq = list(entry)
            out.append(
                Item(
                    str(seq[0]),
                    str(seq[1]) if len(seq) > 1 else str(seq[0]),
                    str(seq[2]) if len(seq) > 2 else "",
                    str(seq[3]) if len(seq) > 3 else default_source,
                )
            )
        else:
            out.append(Item(str(entry), str(entry), "", default_source))
    return out


def _item_sort_key(item_id: str):
    head, _, tail = item_id.partition("-")
    numeric = tail.split("[", 1)[0]
    try:
        return (head, 0, int(numeric), item_id)
    except ValueError:
        return (head, 1, 0, item_id)


def _expand(spec: FlowSpec) -> list[FlowRun]:
    if not spec.params:
        return [
            FlowRun(
                spec.group.key, spec.group.title, spec.name, spec.kind, spec.fn,
                list(spec.items), {},
            )
        ]
    runs: list[FlowRun] = []
    suffix_all = len(spec.params) > 1 or bool(spec.ids)
    for index, values in enumerate(spec.params):
        label = spec.ids[index] if spec.ids and index < len(spec.ids) else str(index + 1)
        suffix = f"[{label}]" if suffix_all else ""
        items = [Item(it.id + suffix, it.name, it.expect, it.source) for it in spec.items]
        runs.append(
            FlowRun(
                spec.group.key, spec.group.title, spec.name, spec.kind, spec.fn,
                items, dict(values),
            )
        )
    return runs


def collect(selected_groups: list[str] | None = None) -> list[FlowRun]:
    """All registered flow runs in canonical category order."""
    runs: list[FlowRun] = []
    for key in ordered_group_keys(selected_groups):
        runs += GROUPS[key].collect()
    return runs


def ordered_group_keys(selected: list[str] | None = None) -> list[str]:
    from .model import CATEGORY_ORDER

    def rank(key: str) -> tuple[int, int]:
        canonical = CATEGORY_ORDER.index(key) if key in CATEGORY_ORDER else len(CATEGORY_ORDER)
        return (canonical, GROUPS[key].order)

    keys = [k for k in _GROUP_ORDER if not selected or k in selected]
    return sorted(keys, key=rank)


def case_signature(fn: Callable[..., Any]) -> list[str]:
    """Parameter names after the leading context parameter."""
    params = [p.name for p in inspect.signature(fn).parameters.values()
              if p.kind in (p.POSITIONAL_OR_KEYWORD, p.KEYWORD_ONLY)]
    return params[1:]
