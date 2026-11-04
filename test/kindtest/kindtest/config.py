"""Minimal run configuration.

Only *environment* facts live here - where the clusters are, which images and
named versions identify the stack, which tenant object-name prefix to use and
where to write the report.  Anything that describes *what* to verify is written
as a Python test (see ``tests/``).

Relative paths are resolved against the config file's directory.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml

from .versions import display_image, resolve_image


class ConfigError(RuntimeError):
    pass


_REQUIRED = ("run.id", "clusters.control.kubeconfig", "clusters.data")


class Config:
    def __init__(self, raw: dict[str, Any], path: Path):
        self.raw = raw
        self.path = path
        self.base = path.resolve().parent

    # ------------------------------------------------------------------ load
    @classmethod
    def load(cls, path: str | Path) -> "Config":
        p = Path(path)
        if not p.is_file():
            p = Path.cwd() / path
        if not p.is_file():
            raise ConfigError(f"config not found: {path}")
        raw = yaml.safe_load(p.read_text()) or {}
        if not isinstance(raw, dict):
            raise ConfigError(f"config must be a YAML mapping: {p}")
        return cls(raw, p)

    # --------------------------------------------------------------- access
    def get(self, dotted: str, default: Any = None) -> Any:
        node: Any = self.raw
        for part in dotted.split("."):
            if isinstance(node, dict) and part in node:
                node = node[part]
            else:
                return default
        return node

    def require(self, dotted: str) -> Any:
        value = self.get(dotted)
        if value in (None, ""):
            raise ConfigError(f"missing required config key: {dotted}")
        return value

    def resolve(self, value: Any) -> Path:
        p = Path(str(value))
        if p.is_absolute():
            return p
        return (self.base / p).resolve()

    # ------------------------------------------------------------- validate
    def validate(self) -> list[str]:
        problems: list[str] = []
        for key in _REQUIRED:
            if self.get(key) in (None, ""):
                problems.append(f"missing required config key: {key}")
        data = self.get("clusters.data") or {}
        if not isinstance(data, dict) or not data:
            problems.append("clusters.data must be a non-empty mapping")
        else:
            for key, entry in data.items():
                for field in ("name", "clusterId", "kubeconfig"):
                    if not (entry or {}).get(field):
                        problems.append(f"clusters.data.{key}.{field} is required")
        problems += self._validate_versions()
        return problems

    def _validate_versions(self) -> list[str]:
        problems: list[str] = []
        defs = self.get("versions.definitions")
        if defs is None:
            return problems
        if not isinstance(defs, dict) or not defs:
            problems.append("versions.definitions must be a non-empty mapping")
            return problems
        for name, entry in defs.items():
            entry = entry or {}
            has_images = isinstance(entry.get("images"), dict) and entry["images"]
            if not (entry.get("image") or has_images):
                problems.append(f"versions.definitions.{name} needs 'images' or 'image'")
        initial = self.get("versions.initial")
        if initial and initial not in defs:
            problems.append(f"versions.initial={initial!r} is not defined in versions.definitions")
        for side in ("control", "data"):
            for i, target in enumerate(self.get(f"versions.rollout.{side}") or []):
                if not (target or {}).get("name"):
                    problems.append(f"versions.rollout.{side}[{i}].name is required")
        return problems

    # ------------------------------------------------------------- identity
    @property
    def theme(self) -> str:
        return str(self.get("run.theme", "kindtest"))

    @property
    def run_id(self) -> str:
        return str(self.require("run.id"))

    @property
    def label_key(self) -> str:
        return str(self.get("run.label", "rlark.io/test-run"))

    @property
    def domain(self) -> str:
        return str(self.get("run.domain", "default"))

    # --------------------------------------------------------------- paths
    @property
    def report_dir(self) -> Path:
        return self.resolve(self.get("report.dir", f"../test-reports/{self.theme}"))

    @property
    def report_file(self) -> str:
        return str(self.get("report.file", "full-test-result.md"))

    @property
    def evidence_dir(self) -> Path:
        return self.report_dir / "evidence"

    @property
    def manifests_dir(self) -> Path:
        return self.resolve(self.get("manifests.dir", self.get("report.dir", f"../test-reports/{self.theme}")))

    # ------------------------------------------------------------- clusters
    @property
    def control_kubeconfig(self) -> Path:
        return self.resolve(self.require("clusters.control.kubeconfig"))

    @property
    def kcp_kubeconfig(self) -> Path:
        return self.resolve(self.get("clusters.control.kcp", self.get("clusters.control.kcpKubeconfig", "")))

    @property
    def control_namespace(self) -> str:
        return str(self.get("clusters.control.namespace", "rlark-system"))

    def data_clusters(self) -> dict[str, dict[str, Any]]:
        return dict(self.get("clusters.data") or {})

    # --------------------------------------------------------------- images
    @property
    def task_image(self) -> str:
        return str(self.get("images.task", "task:test"))

    @property
    def agent_image(self) -> str:
        return str(self.get("images.agent", ""))

    @property
    def tensorboard_image(self) -> str:
        return str(self.get("images.tensorboardStub", ""))

    # ------------------------------------------------------------- versions
    @property
    def versions_definitions(self) -> dict[str, Any]:
        out = self.get("versions.definitions") or {}
        return dict(out) if isinstance(out, dict) else {}

    @property
    def versions_initial(self) -> str:
        initial = str(self.get("versions.initial", ""))
        if initial:
            return initial
        return next(iter(self.versions_definitions), "")

    def version_entry(self, name: str) -> dict[str, Any]:
        entry = self.versions_definitions.get(name) or {}
        return dict(entry) if isinstance(entry, dict) else {}

    def _version_ref(self, name: str) -> str:
        return str(self.version_entry(name).get("ref", ""))

    def version_images(self, name: str) -> dict[str, str]:
        """The image slots declared for a version (``image:`` -> ``default``)."""
        entry = self.version_entry(name)
        if entry.get("image"):
            return {"default": str(entry["image"])}
        images = entry.get("images")
        if isinstance(images, dict) and images:
            return {str(k): str(v) for k, v in images.items()}
        return {}

    def _version_image(self, name: str) -> str:
        """The single concrete image of a version; raises when ambiguous."""
        images = self.version_images(name)
        if not images:
            return ""
        return resolve_image(images, "", what=f"版本 {name!r}")

    # ------------------------------------------------------ baseline / target
    # Report-relevant baseline/target come from the named ``versions`` block:
    # ``base``/``baseline`` (if present) is the 基线, the version named
    # ``target`` or else ``initial`` is the 目标.
    def _first_version(self, names: tuple[str, ...]) -> str:
        for name in names:
            if name in self.versions_definitions:
                return name
        return ""

    @property
    def baseline_ref(self) -> str:
        name = self._first_version(("base", "baseline"))
        return self._version_ref(name) if name else ""

    @property
    def target_ref(self) -> str:
        name = self._first_version(("target",)) or self.versions_initial
        return self._version_ref(name) if name else ""

    @property
    def baseline_image(self) -> str:
        name = self._first_version(("base", "baseline"))
        return self._version_image(name) if name else ""

    @property
    def target_image(self) -> str:
        name = self._first_version(("target",)) or self.versions_initial
        return self._version_image(name) if name else ""

    @property
    def baseline_image_display(self) -> str:
        """All image slots of the baseline version, for reports/logs (never raises)."""
        return display_image(self.version_images(self._first_version(("base", "baseline"))))

    @property
    def target_image_display(self) -> str:
        """All image slots of the target version, for reports/logs (never raises)."""
        name = self._first_version(("target",)) or self.versions_initial
        return display_image(self.version_images(name))

    # ---------------------------------------------------------------- tests
    @property
    def tests_dir(self) -> Path:
        return self.resolve(self.get("tests.dir", "tests"))

    def test_globs(self) -> list[str]:
        include = self.get("tests.include")
        if isinstance(include, list) and include:
            return [str(x) for x in include]
        return ["*.py"]

    @property
    def title(self) -> str:
        return str(self.get("run.title", f"{self.theme} 功能回归结果"))
