"""Execution engine: full-matrix-first, flow execution, live report.

The report matrix is built up front from the declared items (every 测试项 starts
as 未执行) and refreshed after each 测试观测点.  Group-scoped fixtures are created
once per group and torn down after it, case-scoped fixtures per flow run.
"""

from __future__ import annotations

import importlib.util
import inspect
import json
import shutil
import sys
import time
import traceback
from pathlib import Path
from typing import Any, Callable, Iterable

from . import registry
from .config import Config
from .context import Context, EvidenceBook
from .kube import Cluster, Kind, Kube
from .model import Case, RunReport, Status, utcnow
from .registry import Blocked, FIXTURES, FlowRun, RegistryError, Restricted
from .report import write_report
from .shell import Shell


class _Scope:
    """Cache + teardown stack for one fixture scope (a group or a flow run)."""

    def __init__(self) -> None:
        self.cache: dict[str, Any] = {}
        self.errors: dict[str, BaseException] = {}
        self.teardowns: list[Any] = []

    def close(self, log: Callable[[str], None]) -> None:
        for gen in reversed(self.teardowns):
            try:
                next(gen)
            except StopIteration:
                pass
            except Exception as exc:  # noqa: BLE001 - teardown must not abort a run
                log(f"fixture teardown error: {type(exc).__name__}: {exc}")
        self.teardowns.clear()
        self.cache.clear()


class Runner:
    def __init__(self, config: Config, *, only: Iterable[str] | None = None, dry_run: bool = False,
                 verbose: bool = False) -> None:
        self.config = config
        self.only = [t for t in (only or []) if t]
        self.dry_run = dry_run
        self.verbose = verbose
        self.report_dir = config.report_dir
        self.report_dir.mkdir(parents=True, exist_ok=True)
        self.log_path = self.report_dir / f"run-{config.run_id}.log"
        self._log_handle = self.log_path.open("a")
        self.shell = Shell(dry_run=dry_run, verbose=verbose, log=self._log)
        self.evidence = EvidenceBook(config.evidence_dir)
        self.kind = Kind(self.shell)
        self.ctx = self._build_context()
        self.ctx.on_item = self._refresh_item
        self.live: dict[str, Case] = {}
        self.started = ""
        self.blocked_reason = ""

    # ------------------------------------------------------------- wiring
    def _build_context(self) -> Context:
        cfg = self.config
        clusters: dict[str, Cluster] = {}
        for key, entry in cfg.data_clusters().items():
            kubeconfig = str(cfg.resolve(entry["kubeconfig"]))
            data_ns = str(entry.get("namespace", "rlark-system"))
            clusters[key] = Cluster(
                key=key,
                name=str(entry["name"]),
                cluster_id=str(entry["clusterId"]),
                data=Kube(self.shell, kubeconfig, namespace=data_ns, label=str(entry["name"])),
                mgmt=Kube(self.shell, str(cfg.kcp_kubeconfig), namespace=str(entry["clusterId"])),
                data_ns=data_ns,
            )
        mgmt = Kube(self.shell, str(cfg.kcp_kubeconfig), namespace=None, label="kcp")
        control = Kube(self.shell, str(cfg.control_kubeconfig), namespace=cfg.control_namespace, label="control")
        return Context(cfg, mgmt=mgmt, control=control, clusters=clusters, shell=self.shell,
                       kind=self.kind, evidence=self.evidence, log=self._log)

    # ------------------------------------------------------------- loading
    def load_tests(self) -> list[str]:
        modules: list[str] = []
        for pattern in self.config.test_globs():
            for path in sorted(self.config.tests_dir.glob(pattern)):
                if path.name.startswith("_"):
                    continue
                name = f"kindtest_tests_{path.stem}"
                spec = importlib.util.spec_from_file_location(name, path)
                if spec is None or spec.loader is None:
                    continue
                module = importlib.util.module_from_spec(spec)
                sys.modules[name] = module
                spec.loader.exec_module(module)
                modules.append(path.name)
        return modules

    # --------------------------------------------------------------- plan
    def runs(self) -> list[FlowRun]:
        registered = registry.collect()
        self._assert_unique_item_ids(registered)
        out: list[FlowRun] = []
        for run in registered:
            if not self.only:
                out.append(run)
                continue
            if run.group_key in self.only or run.name in self.only:
                out.append(run)
                continue
            items = [it for it in run.items if self._item_selected(it.id)]
            if items:
                out.append(
                    FlowRun(run.group_key, run.group_title, run.name, run.kind, run.fn,
                            items, dict(run.values))
                )
        return out

    @staticmethod
    def _assert_unique_item_ids(runs: list[FlowRun]) -> None:
        """Reject duplicate item ids before they collapse into one report row.

        The live matrix and the final report are keyed by item id, so a second
        item with the same id would silently overwrite the first.  Surface the
        mistake (and its declaring flows) instead.
        """
        owner: dict[str, str] = {}
        dupes: dict[str, list[str]] = {}
        for run in runs:
            for it in run.items:
                if it.id in owner:
                    dupes.setdefault(it.id, [owner[it.id]]).append(run.name)
                else:
                    owner[it.id] = run.name
        if dupes:
            detail = "；".join(f"{i}（{'、'.join(names)}）" for i, names in dupes.items())
            raise RegistryError(f"测试项 id 重复，报告矩阵会互相覆盖：{detail}")

    def _item_selected(self, item_id: str) -> bool:
        """``--only`` matches ids exactly; a trailing ``*`` selects a prefix.

        So ``REG-01`` selects one item while ``REG-0*`` selects ``REG-01``…
        ``REG-09``.  A parametrized id also matches when its base (the part
        before ``[``) is given.
        """
        base = item_id.split("[", 1)[0]
        for token in self.only:
            if token.endswith("*"):
                prefix = token[:-1]
                if item_id.startswith(prefix) or base.startswith(prefix):
                    return True
            elif token in (item_id, base):
                return True
        return False

    def plan(self) -> list[tuple[str, list[FlowRun]]]:
        out: list[tuple[str, list[FlowRun]]] = []
        by_key: dict[str, list[FlowRun]] = {}
        for run in self.runs():
            by_key.setdefault(run.group_key, []).append(run)
        for key in registry.ordered_group_keys():
            if key in by_key:
                out.append((key, by_key[key]))
        return out

    # ---------------------------------------------------------------- run
    def run(self) -> RunReport:
        self._archive_previous()
        self.evidence.ensure()
        self.started = utcnow()
        runs = self.runs()
        # full matrix first: every declared item starts as 未执行
        self.live = {
            it.id: Case(id=it.id, name=it.name, expected=it.expect, actual="",
                        status=Status.SKIPPED, source=it.source)
            for run in runs for it in run.items
        }
        self._write_live(finished="")
        try:
            self._preflight()
            self._deploy_initial_version()
        except Blocked as exc:
            return self._block_run(str(exc))

        group_key: str | None = None
        group_scope: _Scope | None = None
        for run in runs:
            if run.group_key != group_key:
                if group_scope is not None:
                    group_scope.close(self._log)
                group_key = run.group_key
                group_scope = _Scope()
                self._log(f"===== GROUP {group_key} =====")
            self._run_flow(run, group_scope)
        if group_scope is not None:
            group_scope.close(self._log)

        report = self._write_live(finished=utcnow(), environment=self._environment())
        self._log_handle.close()
        return report

    def _deploy_initial_version(self) -> None:
        """Deploy ``versions.initial`` once, before the first flow touches the env."""
        versions = self.ctx.versions
        if not versions.enabled:
            return
        initial = versions.initial
        if self.dry_run:
            self._log(
                f"[dry-run] initial version {initial} "
                f"(images={versions.version(initial).images})"
            )
            return
        if not versions.rollout_configured:
            self._log(
                f"versions defined ({'、'.join(versions.names())}) but versions.rollout "
                f"未配置；跳过初始版本 {initial} 的部署"
            )
            return
        self._log(
            f"===== INITIAL VERSION {initial} "
            f"(images={versions.version(initial).images}) ====="
        )
        try:
            self.ctx.deploy_version(initial)
        except Exception as exc:  # noqa: BLE001 - a failed setup blocks the run
            raise Blocked(
                f"初始版本 {initial} 部署失败：{type(exc).__name__}: {exc}"
            ) from exc

    def _probe_targets(self) -> list[tuple[str, Kube]]:
        targets: list[tuple[str, Kube]] = [
            ("KCP", self.ctx.mgmt),
            ("控制集群", self.ctx.control),
        ]
        for key in self.ctx.data_keys():
            targets.append((f"数据集群 {self.ctx.cl(key).name}", self.ctx.cl(key).data))
        return targets

    def _preflight(self) -> None:
        """Fail fast as :class:`Blocked` when the env cannot be used at all.

        Catches the cases that would otherwise masquerade as dozens of assertion
        failures: no kubectl, a missing/unreadable kubeconfig, an unreachable
        cluster.  Such a run is recorded as Blocked, never as a test failure.
        """
        if self.dry_run:
            return
        if shutil.which("kubectl") is None:
            raise Blocked("前置检查失败：PATH 中找不到 kubectl")
        problems: list[str] = []
        seen: set[str] = set()
        for label, kube in self._probe_targets():
            kubeconfig = kube.kubeconfig or ""
            if kubeconfig in seen:
                continue
            seen.add(kubeconfig)
            if kubeconfig and not Path(kubeconfig).is_file():
                problems.append(f"{label} kubeconfig 不可读（{kubeconfig}）")
                continue
            ran = kube.probe()
            if not ran.ok:
                lines = (ran.err or ran.out or "").strip().splitlines()
                detail = lines[0] if lines else f"rc={ran.rc}"
                problems.append(f"{label} 不可达（{detail}）")
        if problems:
            raise Blocked("前置检查失败：" + "；".join(problems))

    def _block_run(self, reason: str) -> RunReport:
        """Record a setup/preflight failure as Blocked across the whole matrix."""
        self.blocked_reason = reason
        self._log(f"run blocked: {reason}")
        for case in self.live.values():
            case.status = Status.BLOCKED
            case.actual = reason
        return self._write_live(finished=utcnow(), environment={})


    def _run_flow(self, run: FlowRun, group_scope: _Scope) -> None:
        self._log(f"--- {run.kind} {run.group_key}/{run.name} [{', '.join(it.id for it in run.items)}] ---")
        self.ctx.begin_flow(run.items)
        flow_scope = _Scope()
        try:
            kwargs = self._resolve_kwargs(run, group_scope, flow_scope)
            if run.kind == "case":
                item = run.items[0]
                with self.ctx.observe(item.id) as obs:
                    result = run.fn(self.ctx, **kwargs)
                    if isinstance(result, str) and result:
                        obs.actual = result
            else:
                run.fn(self.ctx, **kwargs)
        except Blocked as exc:
            self.ctx.fail_remaining(str(exc))
        except Restricted as exc:
            self.ctx.fail_remaining(str(exc), Status.RESTRICTED)
        except Exception as exc:  # noqa: BLE001 - abort the flow, block the rest
            self._log(traceback.format_exc())
            self.ctx.fail_remaining(f"{type(exc).__name__}: {exc}")
        finally:
            flow_scope.close(self._log)
            self.ctx.finish_flow()

    def _refresh_item(self, item_id: str) -> None:
        obs = self.ctx.results.get(item_id)
        case = self.live.get(item_id)
        if obs is None or case is None:
            return
        case.status = obs.status
        case.actual = obs.actual
        case.duration_s = round(obs.duration_s, 1)
        for ev in obs.evidence:
            if ev not in case.evidence:
                case.evidence.append(ev)
        self._log(f"[{case.category}] {item_id} {obs.status.value} — {obs.actual}")
        self._write_live(finished="")

    # --------------------------------------------------------- fixtures
    def _resolve_kwargs(self, run: FlowRun, group_scope: _Scope, flow_scope: _Scope) -> dict[str, Any]:
        kwargs: dict[str, Any] = {}
        for name in registry.case_signature(run.fn):
            if name in run.values:
                kwargs[name] = run.values[name]
            else:
                kwargs[name] = self._fixture_value(name, group_scope, flow_scope)
        return kwargs

    def _fixture_value(self, name: str, group_scope: _Scope, flow_scope: _Scope) -> Any:
        definition = FIXTURES.get(name)
        if definition is None:
            raise Blocked(f"未注册 fixture: {name}")
        holder = group_scope if definition.scope == "group" else flow_scope
        if name in holder.cache:
            return holder.cache[name]
        if name in holder.errors:
            raise holder.errors[name]
        kwargs: dict[str, Any] = {}
        try:
            for dep in registry.case_signature(definition.fn):
                kwargs[dep] = self._fixture_value(dep, group_scope, flow_scope)
            produced = definition.fn(self.ctx, **kwargs)
            if inspect.isgenerator(produced):
                value = next(produced)
                holder.teardowns.append(produced)
            else:
                value = produced
        except BaseException as exc:  # noqa: BLE001 - remembered for dependent flows
            holder.errors[name] = exc
            raise
        holder.cache[name] = value
        return value

    # ------------------------------------------------------------- output
    def _write_live(self, *, finished: str, environment: dict | None = None) -> RunReport:
        report = RunReport(
            theme=self.config.theme,
            run_id=self.config.run_id,
            base_ref=self.config.baseline_ref,
            target_ref=self.config.target_ref,
            started=self.started,
            finished=finished,
            config=self.config.raw,
            evidence_dir=str(self.config.evidence_dir),
            cases=list(self.live.values()),
            environment=environment or {},
            blocked_reason=self.blocked_reason,
            notes=self.config.get("notes") or {},
        )
        (self.report_dir / "run-manifest.json").write_text(
            json.dumps(report.to_dict(), indent=2, ensure_ascii=False)
        )
        if not self.dry_run:
            write_report(report, self.config)
        return report

    def _environment(self) -> dict[str, str]:
        env: dict[str, str] = {}
        agents = []
        for key in self.ctx.data_keys():
            cl = self.ctx.cl(key)
            image = cl.data.jp("deploy", "rlark-agent", "{.spec.template.spec.containers[0].image}")
            ready = cl.data.jp("deploy", "rlark-agent", "{.status.readyReplicas}/{.status.replicas}")
            agents.append(f"{cl.name} {image} ready {ready}")
        env["Cluster Agent 镜像"] = "；".join(agents)
        nodes = []
        for key in self.ctx.data_keys():
            lines = [ln for ln in self.ctx.data(key)("get", "nodes", "--no-headers").stdout.splitlines() if ln.strip()]
            nodes.append(f"{self.ctx.cl(key).name} {len(lines)} 节点")
        env["数据面节点"] = "；".join(nodes)
        env["控制组件"] = self.ctx.control.get("deploy").stdout.replace("\n", "；")
        if self.ctx.versions.enabled:
            env["版本配置"] = (
                f"initial={self.ctx.versions.initial}；"
                + "、".join(self.ctx.versions.names())
            )
            current = self.ctx.current_version()
            if current:
                env["当前版本"] = current
        return env

    # ------------------------------------------------------------ archive
    def _archive_previous(self) -> None:
        manifest = self.report_dir / "run-manifest.json"
        if not manifest.exists():
            return
        try:
            prev = json.loads(manifest.read_text())
        except (OSError, json.JSONDecodeError):
            return
        prev_target = prev.get("target_ref")
        if not prev_target or prev_target == self.config.target_ref:
            return
        old = str(prev_target).split("@")[-1].split("/")[-1][:12]
        report = self.report_dir / self.config.report_file
        if report.exists():
            report.rename(report.with_name(f"{report.stem}-{old}{report.suffix}"))
        evidence = self.report_dir / "evidence"
        if evidence.exists():
            evidence.rename(self._unique_archive(self.report_dir / f"evidence-{old}"))
        manifest.rename(self.report_dir / f"run-manifest-{old}.json")
        self._log(f"archived previous run ({prev_target}) as *-{old}")

    @staticmethod
    def _unique_archive(path: Path) -> Path:
        """Keep an existing archive: fall back to ``<name>-1``, ``-2``, ..."""
        if not path.exists():
            return path
        index = 1
        while True:
            candidate = path.with_name(f"{path.name}-{index}")
            if not candidate.exists():
                return candidate
            index += 1

    # ------------------------------------------------------------ logging
    def _log(self, message: str) -> None:
        self._log_handle.write(message + "\n")
        self._log_handle.flush()
        print(message, file=sys.stderr, flush=True)
