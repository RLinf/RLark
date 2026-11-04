"""Run context (``t``) - the ergonomic surface tests are written against.

Two jobs:

1. **Observation machinery** - the report model is
   测试流程 (flow) → 测试观测点 (observation point) → 测试项 (item).  A flow body
   wraps each block of checks in ``with t.observe("REG-01"):`` and the assertion
   helpers accumulate auditable facts into the active observation.
2. **Basic operation helpers** - small, reusable functions so a test can be
   written with almost no boilerplate: ``t.kget``/``t.kapply``/``t.kdelete``/
   ``t.kpatch``/``t.kexec``/``t.klogs``/``t.kwait_pod`` for kubectl, plus
   environment shortcuts such as ``t.pod_ready``, ``t.task_uid``,
   ``t.job_stopped``, ``t.agent_image``, ``t.ray_active``, ``t.dns_resolve``
   and ``t.tcp_connect``.
"""

from __future__ import annotations

import json
import re
import time
from contextlib import contextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterator

from .kube import Cluster, Kind, Kube
from .model import Status
from .registry import Blocked, Restricted
from .shell import Ran, Shell
from .versions import VersionDeployer, VersionError


class EvidenceBook:
    """Append-only evidence files under ``<theme>/evidence/``.

    The directory is created on demand rather than in ``__init__`` so that a
    run can move a previous ``evidence/`` aside (when the target ref changed)
    before this run writes its own files.
    """

    def __init__(self, root: Path):
        self.root = root
        self.files: list[str] = []

    def ensure(self) -> None:
        """(Re)create the evidence directory if it is missing."""
        self.root.mkdir(parents=True, exist_ok=True)

    def add(self, name: str, text: str) -> str:
        if name not in self.files:
            self.files.append(name)
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a") as handle:
            handle.write(text if text.endswith("\n") else text + "\n")
        return name


@dataclass
class Observation:
    """The recorded result of one 测试观测点 (one report row)."""

    item_id: str
    status: Status = Status.PASS
    actual: str = ""
    facts: list[str] = field(default_factory=list)
    evidence: list[str] = field(default_factory=list)
    started: float = 0.0
    duration_s: float = 0.0


class Context:
    def __init__(
        self,
        config,
        *,
        mgmt: Kube,
        control: Kube,
        clusters: dict[str, Cluster],
        shell: Shell,
        kind: Kind,
        evidence: EvidenceBook,
        log: Callable[[str], None] | None = None,
    ) -> None:
        self.config = config
        self.mgmt = mgmt
        self.control = control
        self.clusters = clusters
        self.shell = shell
        self.kind = kind
        self.evidence_book = evidence
        self._log = log
        # named versions + rollout (see versions.py); empty when unconfigured
        self.versions = VersionDeployer(config, control=control, clusters=clusters, log=self.log)
        self.run_id = config.run_id
        self.label_key = config.label_key
        self.domain = config.domain
        self.theme = config.theme
        # per-run scratch space shared by tests (e.g. an object name handed on)
        self.vars: dict[str, Any] = {}
        # runner bookkeeping
        self.current_case = ""
        self._case_evidence: set[str] = set()
        # observation state (see begin_flow / observe)
        self.results: dict[str, Observation] = {}
        self._declared: list[str] = []
        self._active: Observation | None = None
        self.on_item: Callable[[str], None] | None = None

    # --------------------------------------------------------------- logging
    def log(self, message: str) -> None:
        if self._log is not None:
            self._log(message)

    def begin_case(self, case_id: str) -> None:
        self.current_case = case_id
        self._case_evidence = set()

    # ======================================================== observations
    def begin_flow(self, items: list[Any]) -> None:
        """Reset observation state for one flow run."""
        self.results = {}
        self._declared = [it.id for it in items]
        self._active = None

    @contextmanager
    def observe(self, item_id: str) -> Iterator[Observation]:
        """Open a 测试观测点: checks inside become this item's auditable result.

        Exceptions raised inside the block are captured on the item and do not
        abort the rest of the flow; a non-pass verdict was already recorded.
        """
        obs = Observation(item_id=item_id, started=time.time())
        previous, previous_case = self._active, self.current_case
        self._active = obs
        self.current_case = item_id
        try:
            yield obs
        except Blocked as exc:
            obs.status, obs.actual = Status.BLOCKED, str(exc)
        except Restricted as exc:
            obs.status, obs.actual = Status.RESTRICTED, str(exc)
        except AssertionError as exc:
            obs.status, obs.actual = Status.FAIL, str(exc) or "断言失败"
        except Exception as exc:  # noqa: BLE001 - any error is a failed observation
            obs.status, obs.actual = Status.FAIL, f"{type(exc).__name__}: {exc}"
        finally:
            obs.duration_s = time.time() - obs.started
            if not obs.actual:
                obs.actual = "；".join(obs.facts) if obs.facts else "ok"
            self.results[item_id] = obs
            self._active, self.current_case = previous, previous_case
            if self.on_item is not None:
                self.on_item(item_id)

    def finish_flow(self, reason: str = "流程未到达该观测点") -> None:
        """Mark every declared-but-unobserved item Blocked."""
        for item_id in self._declared:
            if item_id not in self.results:
                self._record(item_id, Status.BLOCKED, reason)

    def fail_remaining(self, reason: str, status: Status = Status.BLOCKED) -> None:
        """Mark every not-yet-observed declared item with ``status``."""
        for item_id in self._declared:
            if item_id not in self.results:
                self._record(item_id, status, reason)

    def restrict(self, reason: str, item_id: str | None = None) -> None:
        """Record an item as 受限 (observed but limited by an existing constraint)."""
        self._record(item_id or self.current_case, Status.RESTRICTED, reason)

    def block(self, reason: str, item_id: str | None = None) -> None:
        """Record an item as Blocked without raising (flow continues)."""
        self._record(item_id or self.current_case, Status.BLOCKED, reason)

    def skip(self, reason: str, item_id: str | None = None) -> None:
        self._record(item_id or self.current_case, Status.SKIPPED, reason)

    def _record(self, item_id: str, status: Status, actual: str) -> None:
        obs = self.results.get(item_id) or Observation(item_id=item_id)
        obs.status, obs.actual = status, actual
        self.results[item_id] = obs
        if self.on_item is not None:
            self.on_item(item_id)

    # --------------------------------------------------------------- handles
    def cl(self, key: str) -> Cluster:
        return self.clusters[key]

    def data(self, key: str) -> Kube:
        return self.clusters[key].data

    def data_keys(self) -> list[str]:
        return list(self.clusters)

    def data_ns(self, key: str) -> str:
        return self.clusters[key].data_ns

    def mgmt_ns(self, key: str) -> str:
        return self.clusters[key].mgmt_ns

    # ------------------------------------------------------------- templating
    def render(self, text: str) -> str:
        subs = {
            "{{runId}}": self.run_id,
            "{{labelKey}}": self.label_key,
            "{{domain}}": self.domain,
            "{{taskImage}}": self.config.task_image,
            "{{appImage}}": self.config.agent_image,
            "{{agentImage}}": self.config.agent_image,
            "{{tensorboardImage}}": self.config.tensorboard_image or "rlinf/tensorboard:2.15.1-rc1",
            "{{nodeA}}": str(self.config.get("placement.a", "")),
            "{{nodeB}}": str(self.config.get("placement.b", "")),
            "{{nodeB2}}": str(self.config.get("placement.b2", "")),
        }
        # These resolve strictly (a multi-slot version without a default is an
        # error), so only evaluate them when the template actually uses them -
        # otherwise an unrelated manifest on such a config would fail to render.
        lazy = {
            "{{baselineImage}}": lambda: self.config.baseline_image,
            "{{targetImage}}": lambda: self.config.target_image,
        }
        for placeholder, resolve in lazy.items():
            if placeholder in text:
                text = text.replace(placeholder, resolve())
        for name, version in self.versions.definitions.items():
            subs[f"{{{{image.{name}}}}}"] = version.image
            for slot, image in version.images.items():
                subs[f"{{{{image.{name}.{slot}}}}}"] = image
        for name, value in self.vars.items():
            subs[f"{{{{{name}}}}}"] = str(value)
        for key, value in subs.items():
            text = text.replace(key, value)
        return text

    def manifest_text(self, name: str) -> str:
        path = self.config.manifests_dir / name
        if not path.is_file():
            raise FileNotFoundError(f"manifest not found: {path}")
        return self.render(path.read_text())

    # ================================================== basic ops: kubectl
    def _kube(self, on: str, cluster: str | None) -> Kube:
        if on == "mgmt":
            return self.mgmt
        if on == "control":
            return self.control
        if cluster is None:
            raise ValueError("on='data' requires cluster=<key>")
        return self.data(cluster)

    def kget(self, kind: str, name: str | None = None, *, on: str = "mgmt",
             cluster: str | None = None, ns: str | None = None,
             selector: str | None = None, jsonpath: str | None = None,
             output: str | None = None, all_namespaces: bool = False) -> Ran:
        return self._kube(on, cluster).get(
            kind, name, ns=ns, selector=selector, jsonpath=jsonpath,
            output=output, all_namespaces=all_namespaces,
        )

    def kjp(self, kind: str, name: str, expr: str, *, on: str = "mgmt",
            cluster: str | None = None, ns: str | None = None, default: str = "") -> str:
        return self._kube(on, cluster).jp(kind, name, expr, ns=ns, default=default)

    def kjson(self, kind: str, name: str | None = None, *, on: str = "mgmt",
              cluster: str | None = None, ns: str | None = None,
              selector: str | None = None) -> Any:
        return self._kube(on, cluster).json(kind, name, ns=ns, selector=selector)

    def knames(self, kind: str, *, on: str = "mgmt", cluster: str | None = None,
               ns: str | None = None, selector: str | None = None,
               all_namespaces: bool = False) -> list[str]:
        return self._kube(on, cluster).names(
            kind, ns=ns, selector=selector, all_namespaces=all_namespaces
        )

    def kexists(self, kind: str, name: str, *, on: str = "mgmt",
                cluster: str | None = None, ns: str | None = None) -> bool:
        return self._kube(on, cluster).exists(kind, name, ns=ns)

    def kexec(self, cluster: str, pod: str, *argv: str, container: str | None = None,
              ns: str | None = None, timeout: int | None = None) -> Ran:
        return self.data(cluster).exec(pod, *argv, container=container,
                                       ns=ns or self.data_ns(cluster), timeout=timeout)

    def klogs(self, *, on: str = "mgmt", cluster: str | None = None, ns: str | None = None,
              selector: str | None = None, since: str | None = None,
              tail: int | None = None) -> Ran:
        return self._kube(on, cluster).logs(ns=ns, selector=selector, since=since, tail=tail)

    def kwait_pod(self, cluster: str, pod: str, timeout: int = 300) -> bool:
        return self.data(cluster).wait_pod(pod, ns=self.data_ns(cluster), timeout=timeout).ok

    # ---------------------------------------------------- basic ops: effects
    def apply(self, manifest: str, *, on: str = "mgmt", cluster: str | None = None,
              ns: str | None = None, server_side: bool = False) -> Ran:
        result = self._kube(on, cluster).apply(self.render(manifest), ns=ns, server_side=server_side)
        if not result.ok:
            raise Blocked(f"apply failed rc={result.rc}: {_clip(result.combined())}")
        return result

    def apply_manifest(self, name: str, **kw) -> Ran:
        return self.apply(self.manifest_text(name), **kw)

    def kdelete(self, kind: str, name: str | None = None, *, on: str = "mgmt",
                cluster: str | None = None, ns: str | None = None,
                selector: str | None = None, wait: bool = True,
                ignore_not_found: bool = True, timeout: int | None = None) -> Ran:
        return self._kube(on, cluster).delete(
            kind, name, ns=ns, selector=selector, wait=wait,
            ignore_not_found=ignore_not_found, timeout=timeout,
        )

    def kpatch(self, kind: str, name: str, patch: Any, *, on: str = "mgmt",
               cluster: str | None = None, ns: str | None = None, type: str = "merge") -> Ran:
        return self._kube(on, cluster).patch(kind, name, patch, ns=ns, type=type)

    def delete_job(self, name: str, *, wait: bool = True, timeout: int = 180) -> None:
        self.mgmt.delete("job", name, ignore_not_found=True, wait=wait, timeout=timeout)
        if wait:
            self.wait(lambda: not self.mgmt.exists("job", name), timeout=timeout, desc=f"job {name} 消失")

    def wait_job_gone(self, name: str, timeout: int = 180) -> bool:
        return self.wait(lambda: not self.mgmt.exists("job", name), timeout=timeout)

    def job_stopped(self, name: str, stopped: bool) -> Ran:
        # Job 的停止开关是 spec.suspended；spec.stopped 不存在，API server 会静默剪掉。
        return self.succeeded(self.mgmt.patch("job", name, {"spec": {"suspended": stopped}}),
                              f"{'stop' if stopped else 'start'} Job {name}")

    def restart_job(self, name: str) -> Ran:
        stamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        return self.succeeded(self.mgmt.patch("job", name, {"metadata": {"annotations": {
            "rlark.io/restarted-at": stamp}}}), f"restart Job {name}")

    # ------------------------------------------------- basic ops: shortcuts
    def head_pod(self, job: str, task: str = "head") -> str:
        return f"{job}-{task}-0"

    def task_name(self, job: str, task: str = "main") -> str:
        return f"{job}-{task}"

    def task_uid(self, cluster: str, task: str) -> str:
        return self.mgmt.jp("task", task, "{.metadata.uid}", ns=self.mgmt_ns(cluster))

    def task_phase(self, cluster: str, task: str) -> str:
        return self.mgmt.jp("task", task, "{.status.phase}", ns=self.mgmt_ns(cluster))

    def pod_ready(self, cluster: str, pod: str) -> bool:
        return self.data(cluster).jp(
            "pod", pod, "{.status.conditions[?(@.type=='Ready')].status}"
        ) == "True"

    def pod_node(self, cluster: str, pod: str) -> str:
        return self.data(cluster).jp("pod", pod, "{.spec.nodeName}")

    def exec_ok(self, cluster: str, pod: str, *argv: str) -> bool:
        return self.kexec(cluster, pod, *argv).ok

    def dns_resolve(self, cluster: str, pod: str, name: str) -> str:
        out = self.kexec(
            cluster, pod, "bash", "-c",
            f"getent hosts {name} || python3 -c \"import socket;print(socket.gethostbyname('{name}'))\"",
        )
        return (out.stdout + out.stderr).strip()

    def tcp_connect(self, cluster: str, pod: str, host: str, port: int) -> bool:
        out = self.kexec(
            cluster, pod, "bash", "-c",
            f"exec 3<>/dev/tcp/{host}/{port} && echo CONNECT_OK",
        )
        return "CONNECT_OK" in (out.stdout + out.stderr)

    def ray_active(self, cluster: str, pod: str) -> int:
        out = self.kexec(cluster, pod, "ray", "status")
        return _active_nodes(out.stdout + out.stderr)

    def agent_image(self, cluster: str) -> str:
        return self.cl(cluster).data.jp(
            "deploy", "rlark-agent", "{.spec.template.spec.containers[0].image}"
        )

    def agent_ready(self, cluster: str) -> str:
        return self.cl(cluster).data.jp(
            "deploy", "rlark-agent", "{.status.readyReplicas}/{.spec.replicas}"
        )

    def set_agent_image(self, cluster: str, image: str) -> Ran:
        return self.succeeded(
            self.cl(cluster).data(
                "set", "image", "deploy/rlark-agent", f"rlark-agent={image}",
                "-n", self.data_ns(cluster),
            ),
            f"{cluster} Cluster Agent -> {image}",
        )

    def rollout_agent(self, cluster: str, timeout: int = 240) -> Ran:
        return self.succeeded(
            self.cl(cluster).data.rollout_status(
                "deploy/rlark-agent", ns=self.data_ns(cluster), timeout=timeout
            ),
            f"{cluster} Cluster Agent rollout",
        )

    # ------------------------------------------------------------- versions
    # 配置里定义的命名版本（images/refs）；见 versions.py 与 README“多版本与升级”。
    def version_names(self) -> list[str]:
        """Names of every version declared in the config (may be empty)."""
        return self.versions.names()

    def version_image(self, name: str, slot: str = "") -> str:
        """Container image of a declared version (optionally a named slot)."""
        return self.versions.image(name, slot)

    def version_ref(self, name: str) -> str:
        """Git ref of a declared version."""
        return self.versions.ref(name)

    def current_version(self) -> str:
        """The declared version matching the live target, or the last deployed."""
        return self.versions.detect() or self.versions.current_name

    def version_audit(self, name: str | None = None) -> list[dict]:
        """Each rollout target's expected vs live image (default: initial/current)."""
        target = name or self.versions.current_name or self.versions.initial
        if not target:
            raise Restricted("配置未定义 versions，无法核对版本")
        return self.versions.audit(target)

    def deploy_version(self, name: str, *, timeout: int = 300, wait: bool = True):
        """Deploy a declared version now (idempotent) and record evidence.

        Raises :class:`Restricted` when the config declares no versions, and
        :class:`VersionError` for an unknown name or a rollout that never
        completes.  The framework also deploys ``versions.initial`` once before
        the first flow, so a test only needs this to switch mid-run.
        """
        if not self.versions.enabled:
            raise Restricted("配置未定义 versions，无法部署指定版本")
        result = self.versions.deploy(name, timeout=timeout, wait=wait)
        self.evidence(
            f"version={result.name} images={result.images} changed={result.changed} targets={result.targets}",
            f"version-{_safe(result.name)}.txt",
        )
        return result

    def upgrade(self, name: str, *, timeout: int = 300, wait: bool = True):
        """Move the environment to a declared version (alias of ``deploy_version``)."""
        return self.deploy_version(name, timeout=timeout, wait=wait)

    # ----------------------------------------------------------------- waits
    def wait(self, predicate: Callable[[], bool], *, timeout: float = 300, interval: float = 5, desc: str = "条件") -> bool:
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                if predicate():
                    return True
            except Exception:  # noqa: BLE001 - polling must not abort the run
                pass
            time.sleep(interval)
        try:
            return bool(predicate())
        except Exception:  # noqa: BLE001
            return False

    def require(self, predicate: Callable[[], bool], *, timeout: float = 300, interval: float = 5, desc: str = "条件") -> None:
        """Wait for ``predicate`` or raise :class:`Blocked`."""
        if not self.wait(predicate, timeout=timeout, interval=interval):
            raise Blocked(f"{desc} 未在 {int(timeout)}s 内满足")

    def wait_pod(self, key: str, pod: str, timeout: int = 300) -> bool:
        return self.kwait_pod(key, pod, timeout)

    def sleep(self, seconds: float) -> None:
        self.shell.run(["sleep", str(seconds)])

    # -------------------------------------------------------------- evidence
    def evidence(self, text: str, name: str | None = None) -> str:
        ev = name or f"case-{_safe(self.current_case)}.txt"
        self.evidence_book.add(ev, text)
        self._case_evidence.add(ev)
        if self._active is not None and ev not in self._active.evidence:
            self._active.evidence.append(ev)
        return ev

    def case_evidence(self) -> list[str]:
        return sorted(self._case_evidence)

    # ----------------------------------------------------------- assertions
    # Each helper raises AssertionError on mismatch; on success it records a
    # compact fact on the active observation and returns it.
    def _fact(self, text: str) -> str:
        if self._active is not None:
            self._active.facts.append(text)
        return text

    def eq(self, got: Any, want: Any, what: str = "值") -> str:
        if got != want:
            raise AssertionError(f"{what}={got!r} 期望 {want!r}")
        return self._fact(f"{what}={got!r}")

    def ne(self, got: Any, want: Any, what: str = "值") -> str:
        if got == want:
            raise AssertionError(f"{what}={got!r} 不应等于 {want!r}")
        return self._fact(f"{what}={got!r}")

    def ge(self, got: Any, want: Any, what: str = "值") -> str:
        if not got >= want:
            raise AssertionError(f"{what}={got} 期望 ≥{want}")
        return self._fact(f"{what}={got}")

    def le(self, got: Any, want: Any, what: str = "值") -> str:
        if not got <= want:
            raise AssertionError(f"{what}={got} 期望 ≤{want}")
        return self._fact(f"{what}={got}")

    def truthy(self, condition: Any, what: str, detail: str = "") -> str:
        if not condition:
            raise AssertionError(f"{what} 不成立" + (f"（{detail}）" if detail else ""))
        return self._fact(what)

    def contains(self, haystack: str, needle: str, what: str = "输出") -> str:
        if needle not in (haystack or ""):
            raise AssertionError(f"{what} 不含 {needle!r}: {_clip(haystack)}")
        return self._fact(f"{what}含 {needle!r}")

    def matches(self, text: str, pattern: str, what: str = "输出") -> str:
        if not re.search(pattern, text or ""):
            raise AssertionError(f"{what} 未匹配 /{pattern}/: {_clip(text)}")
        return self._fact(f"{what} 匹配 /{pattern}/")

    def succeeded(self, ran: Ran, what: str) -> Ran:
        if not ran.ok:
            raise AssertionError(f"{what} 失败 rc={ran.rc}: {_clip(ran.combined())}")
        return ran

    def failed(self, ran: Ran, what: str) -> Ran:
        if ran.ok:
            raise AssertionError(f"{what} 本应失败却成功: {_clip(ran.combined())}")
        return ran

    def nonempty(self, text: str, what: str = "输出") -> str:
        if not (text or "").strip():
            raise AssertionError(f"{what} 为空")
        return self._fact(f"{what}非空")


def _active_nodes(status: str) -> int:
    match = re.search(r"(\d+)\s+active", status)
    if match:
        return int(match.group(1))
    return len(re.findall(r"\bnode_[0-9a-f]+\b", status))


def _safe(name: str) -> str:
    return "".join(c if c.isalnum() or c in "-_." else "_" for c in name) or "case"


def _clip(text: str, limit: int = 300) -> str:
    text = " ".join((text or "").split())
    return text if len(text) <= limit else text[:limit] + "…"
