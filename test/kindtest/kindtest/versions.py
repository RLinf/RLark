"""Named versions and rollout deployment.

A config may declare several named *versions*.  Each version is a git ``ref``
plus one or more **named images** (``rlark``, ``ui``, ...), because different
components ship different images.  The ``rollout`` block lists the workloads to
update and, per workload, which image slot of the version it should run.

The framework deploys the ``initial`` version before any flow runs and exposes
``t.upgrade(name)`` so a test can switch to any other declared version at a
controlled point in a flow.

Deployment is idempotent: when every live container already carries the
requested image (and ``--image=`` style argument), :meth:`VersionDeployer.deploy`
patches nothing and only verifies rollout status.  Repeating an upgrade, or
deploying the version that is already running, is therefore cheap and safe.

Config shape::

    versions:
      initial: target
      definitions:
        base:
          ref: main@aaaaaaa
          images: {rlark: rlark:baseline-aaaaaaa, ui: rlark-ui:baseline}
        target:
          ref: main@bbbbbbb
          images: {rlark: rlark:target-bbbbbbb, ui: rlark-ui:target}
      rollout:
        control:                       # applied in the control cluster
          - {kind: deploy, name: rlark-controller-manager, container: rlark-controller-manager, slot: rlark}
          - {kind: deploy, name: rlark-ui, container: rlark-ui, slot: ui}
        data:                          # applied in every data cluster
          - {kind: deploy, name: rlark-agent, container: rlark-agent, slot: rlark, args: ["--image="]}
          - {kind: ds,     name: rlark-agent-node, container: rlark-agent-node, slot: rlark}

A single-image version may be written as ``image: <ref>`` (shorthand for a
``default`` slot).  A rollout target without ``slot`` uses the version's
``default`` slot, or its only image; a version with several slots and no
``default`` must be addressed by slot.  ``kind`` accepts ``deploy``/``ds``/``sts``.
``args`` entries are argument prefixes whose value is rewritten to the selected
image, e.g. ``--image=`` turns ``--image=rlark:old`` into ``--image=rlark:new``.
A prefix that is absent is an error, because silently appending a new argument
would run the workload with a configuration the environment never had; set
``appendArgs: true`` on the target to opt into appending instead.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable

from .kube import Cluster, Kube

_KINDS = {
    "deploy": "deploy",
    "deployment": "deploy",
    "ds": "ds",
    "daemonset": "ds",
    "sts": "sts",
    "statefulset": "sts",
}


class VersionError(RuntimeError):
    """An undefined version, an unusable rollout config, or a failed rollout."""


def resolve_image(images: dict[str, str], slot: str = "", *, what: str = "版本") -> str:
    """The single image for ``slot``, or the ``default``/only slot.

    Raises :class:`VersionError` when a named ``slot`` is missing, or when the
    version defines several slots without a ``default`` and no slot was given -
    callers never silently pick an image the config did not intend.  For a
    human-readable summary use :func:`display_image`.
    """
    if slot:
        if slot in images:
            return images[slot]
        available = "、".join(images) or "（无）"
        raise VersionError(f"{what} 未定义镜像 slot {slot!r}；可用：{available}")
    if "default" in images:
        return images["default"]
    if len(images) == 1:
        return next(iter(images.values()))
    available = "、".join(images) or "（无）"
    raise VersionError(f"{what} 定义了多个镜像 slot，必须指定 slot；可用：{available}")


def display_image(images: dict[str, str]) -> str:
    """A human-readable image for a version, for reports and logs.

    Prefers the ``default`` slot or the only image.  A version that defines
    several slots without a ``default`` is listed in full (``rlark=…、ui=…``)
    rather than silently collapsed to one of them.  Use :func:`resolve_image`
    (or :meth:`Version.image_for`) when a single concrete image is required.
    """
    if not images:
        return ""
    if "default" in images:
        return images["default"]
    if len(images) == 1:
        return next(iter(images.values()))
    return "、".join(f"{slot}={image}" for slot, image in images.items())


@dataclass
class Version:
    name: str
    ref: str = ""
    images: dict[str, str] = field(default_factory=dict)

    @classmethod
    def parse(cls, name: str, raw: Any) -> "Version":
        raw = raw or {}
        if not isinstance(raw, dict):
            raise VersionError(f"versions.definitions.{name} must be a mapping")
        images: dict[str, str] = {}
        if isinstance(raw.get("images"), dict):
            images = {str(k): str(v) for k, v in raw["images"].items()}
        elif raw.get("image"):
            images = {"default": str(raw["image"])}
        if not images:
            raise VersionError(f"版本 {name!r} 未定义任何镜像（images 或 image）")
        return cls(name=str(name), ref=str(raw.get("ref", "")), images=images)

    @property
    def image(self) -> str:
        """Human-readable image(s) for display; see :func:`display_image`."""
        return display_image(self.images)

    def image_for(self, slot: str) -> str:
        """The single image for ``slot`` (or the default/only slot).

        Raises :class:`VersionError` when the version defines several slots and
        neither ``slot`` nor a ``default`` names one, so a rollout can never pick
        an image the config did not intend.
        """
        return resolve_image(self.images, slot, what=f"版本 {self.name!r}")


@dataclass
class RolloutTarget:
    kind: str
    name: str
    container: str
    slot: str = ""
    args: list[str] = field(default_factory=list)
    append_missing: bool = False

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> "RolloutTarget":
        if not isinstance(raw, dict):
            raise VersionError(f"rollout target must be a mapping: {raw!r}")
        kind = str(raw.get("kind", "deploy")).lower()
        if kind not in _KINDS:
            raise VersionError(f"unknown rollout kind {kind!r}")
        name = str(raw.get("name", ""))
        if not name:
            raise VersionError("rollout target requires 'name'")
        return cls(
            kind=_KINDS[kind],
            name=name,
            container=str(raw.get("container", name)),
            slot=str(raw.get("slot", "")),
            args=[str(a) for a in (raw.get("args") or [])],
            append_missing=bool(raw.get("appendArgs", False)),
        )

    @property
    def resource(self) -> str:
        return f"{self.kind}/{self.name}"


@dataclass
class DeployResult:
    name: str
    images: dict[str, str]
    changed: bool
    targets: int = 0


class VersionDeployer:
    """Resolves named versions and rolls the configured workloads to one of them."""

    def __init__(
        self,
        config: Any,
        *,
        control: Kube,
        clusters: dict[str, Cluster],
        log: Callable[[str], None] | None = None,
    ) -> None:
        self.config = config
        self.control = control
        self.clusters = clusters
        self._log = log or (lambda _m: None)

        raw_defs = config.get("versions.definitions") or {}
        if not isinstance(raw_defs, dict):
            raise VersionError("versions.definitions must be a mapping")
        self.definitions: dict[str, Version] = {
            str(name): Version.parse(str(name), entry) for name, entry in raw_defs.items()
        }

        self.initial = str(config.get("versions.initial", ""))
        if self.initial and self.initial not in self.definitions:
            raise VersionError(
                f"versions.initial={self.initial!r} 未在 versions.definitions 中定义"
            )
        if not self.initial and self.definitions:
            self.initial = next(iter(self.definitions))

        self.control_targets = [
            RolloutTarget.parse(x) for x in (config.get("versions.rollout.control") or [])
        ]
        self.data_targets = [
            RolloutTarget.parse(x) for x in (config.get("versions.rollout.data") or [])
        ]
        self.current_name = ""

    # ------------------------------------------------------------- identity
    @property
    def enabled(self) -> bool:
        return bool(self.definitions)

    @property
    def rollout_configured(self) -> bool:
        return bool(self.control_targets or self.data_targets)

    def names(self) -> list[str]:
        return list(self.definitions)

    def version(self, name: str) -> Version:
        try:
            return self.definitions[name]
        except KeyError:
            available = "、".join(self.definitions) or "（无）"
            raise VersionError(f"未定义版本 {name!r}；可用版本：{available}")

    def image(self, name: str, slot: str = "") -> str:
        """The concrete image for ``name`` (and optional ``slot``).

        Ambiguous versions raise rather than silently picking a slot; for a
        human-readable summary use ``version(name).image`` (or
        ``versions.version(name).images``).
        """
        return self.version(name).image_for(slot)

    def ref(self, name: str) -> str:
        return self.version(name).ref

    # ------------------------------------------------------------ deployment
    def deploy(self, name: str, *, timeout: int = 300, wait: bool = True) -> DeployResult:
        version = self.version(name)
        if not self.rollout_configured:
            raise VersionError("未配置 versions.rollout，无法部署/升级版本")

        changed = False
        targets = 0
        for target in self.control_targets:
            targets += 1
            changed |= self._apply(
                self.control, self.config.control_namespace, target, version.image_for(target.slot)
            )
        for cluster in self.clusters.values():
            for target in self.data_targets:
                targets += 1
                changed |= self._apply(
                    cluster.data, cluster.data_ns, target, version.image_for(target.slot)
                )

        if wait:
            for target in self.control_targets:
                self._await(self.control, self.config.control_namespace, target, timeout)
            for cluster in self.clusters.values():
                for target in self.data_targets:
                    self._await(cluster.data, cluster.data_ns, target, timeout)

        self.current_name = name
        self._log(
            f"version {name} deployed (images={version.images}, changed={changed}, targets={targets})"
        )
        return DeployResult(name=name, images=dict(version.images), changed=changed, targets=targets)

    def detect(self) -> str:
        """Best-effort: the declared version whose image matches the live target."""
        probe = self._probe()
        if probe is None:
            return ""
        kube, ns, target = probe
        live = self._image_of(kube, ns, target)
        if not live:
            return ""
        for version in self.definitions.values():
            try:
                if version.image_for(target.slot) == live:
                    return version.name
            except VersionError:
                continue
        return ""

    def audit(self, name: str) -> list[dict[str, Any]]:
        """Every rollout target with its expected and live image for ``name``."""
        version = self.version(name)
        rows: list[dict[str, Any]] = []
        for target in self.control_targets:
            rows.append(
                self._audit_one(self.control, self.config.control_namespace, "control", target, version)
            )
        for key, cluster in self.clusters.items():
            for target in self.data_targets:
                rows.append(self._audit_one(cluster.data, cluster.data_ns, key, target, version))
        return rows

    def _audit_one(self, kube, ns, scope, target, version) -> dict[str, Any]:
        expected = version.image_for(target.slot)
        try:
            live = self._image_of(kube, ns, target)
        except Exception:  # noqa: BLE001 - a missing target is just not ok
            live = ""
        return {
            "scope": scope,
            "resource": target.resource,
            "container": target.container,
            "slot": target.slot,
            "expected": expected,
            "live": live,
            "ok": live == expected,
        }

    # --------------------------------------------------------------- internal
    def _probe(self) -> tuple[Kube, str, RolloutTarget] | None:
        if self.control_targets:
            return self.control, self.config.control_namespace, self.control_targets[0]
        if self.data_targets and self.clusters:
            cluster = next(iter(self.clusters.values()))
            return cluster.data, cluster.data_ns, self.data_targets[0]
        return None

    def _image_of(self, kube: Kube, ns: str, target: RolloutTarget) -> str:
        obj = kube.json(target.kind, target.name, ns=ns)
        for container in _containers(obj):
            if container.get("name") == target.container:
                return str(container.get("image", ""))
        return ""

    def _apply(self, kube: Kube, ns: str, target: RolloutTarget, image: str) -> bool:
        obj = kube.json(target.kind, target.name, ns=ns)
        containers = _containers(obj)
        index = next((i for i, c in enumerate(containers) if c.get("name") == target.container), None)
        if index is None:
            raise VersionError(f"{target.resource} 中不存在容器 {target.container}")
        container = containers[index]
        path = f"/spec/template/spec/containers/{index}"

        # One JSON patch for image *and* args: a single spec write means a single
        # rollout, with no intermediate "new image, old args" state.
        ops: list[dict[str, Any]] = []
        if container.get("image") != image:
            ops.append({"op": "replace", "path": f"{path}/image", "value": image})
        if target.args:
            new_args = _rewrite_args(
                container.get("args") or [],
                target.args,
                image,
                append_missing=target.append_missing,
                where=target.resource,
            )
            if new_args is not None:
                # ``replace`` needs an existing member; ``add`` creates ``args``.
                op = "replace" if "args" in container else "add"
                ops.append({"op": op, "path": f"{path}/args", "value": new_args})

        if not ops:
            return False
        ran = kube.patch(target.kind, target.name, ops, ns=ns, type="json")
        if not ran.ok:
            raise VersionError(f"{target.resource} patch 失败: {ran.err.strip()[:200]}")
        return True

    def _await(self, kube: Kube, ns: str, target: RolloutTarget, timeout: int) -> None:
        ran = kube.rollout_status(target.resource, ns=ns, timeout=timeout)
        if not ran.ok:
            raise VersionError(
                f"{target.resource} rollout 未在 {timeout}s 内完成: {ran.err.strip()[:200]}"
            )


def _containers(obj: Any) -> list[dict]:
    spec = ((obj or {}).get("spec") or {}).get("template") or {}
    return list((spec.get("spec") or {}).get("containers") or [])


def _rewrite_args(
    args: list[str],
    prefixes: list[str],
    image: str,
    *,
    append_missing: bool = False,
    where: str = "",
) -> list[str] | None:
    """Return rewritten args, or ``None`` when nothing needs changing.

    A prefix that is not present rewrites nothing; that is an authoring/config
    error (the workload would keep running with a configuration the run did not
    intend) unless ``append_missing`` opted into appending the argument.
    """
    out = list(args)
    changed = False
    for prefix in prefixes:
        desired = prefix + image if prefix.endswith("=") else f"{prefix}={image}"
        found = False
        for i, arg in enumerate(out):
            if _matches_prefix(arg, prefix):
                found = True
                if arg != desired:
                    out[i] = desired
                    changed = True
                break
        if not found:
            if not append_missing:
                location = f"{where} " if where else ""
                raise VersionError(
                    f"{location}启动参数中未找到 {prefix!r}；框架默认只改写已存在的参数，"
                    f"如需缺失时追加请设置 appendArgs: true"
                )
            out.append(desired)
            changed = True
    return out if changed else None


def _matches_prefix(arg: str, prefix: str) -> bool:
    if prefix.endswith(("=", " ")):
        return arg.startswith(prefix)
    return arg == prefix or arg.startswith(prefix + "=") or arg.startswith(prefix + " ")
