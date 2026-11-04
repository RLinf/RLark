"""Thin, explicit kubectl/kind handles.

No implicit kubeconfig is ever used: the management API, the control cluster and
each data cluster have their own :class:`Kube`.  A :class:`Cluster` bundles the
data-plane and management-plane views of one data cluster, including the
namespaces each side uses.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Iterable

from .shell import Ran, Shell


class Kube:
    """A kubectl bound to one kubeconfig with an optional default namespace."""

    def __init__(self, shell: Shell, kubeconfig: str | None, *, namespace: str | None = None, label: str = ""):
        self.shell = shell
        self.kubeconfig = kubeconfig
        self.namespace = namespace
        self.label = label or (kubeconfig or "?")

    # ------------------------------------------------------------------ raw
    def __call__(self, *args: object, **kw) -> Ran:
        argv: list[object] = ["kubectl"]
        if self.kubeconfig:
            argv += ["--kubeconfig", self.kubeconfig]
        argv += list(args)
        return self.shell.run(argv, **kw)

    def _ns(self, ns: str | None) -> list[str]:
        ns = ns if ns is not None else self.namespace
        return ["-n", ns] if ns else []

    # --------------------------------------------------------------- reads
    def get(
        self,
        kind: str,
        name: str | None = None,
        *,
        ns: str | None = None,
        selector: str | None = None,
        jsonpath: str | None = None,
        output: str | None = None,
        all_namespaces: bool = False,
    ) -> Ran:
        args = [*self._ns(ns), "get", kind]
        if name:
            args.append(name)
        if selector:
            args += ["-l", selector]
        if all_namespaces:
            args.append("-A")
        if jsonpath:
            args += ["-o", f"jsonpath={jsonpath}"]
        elif output:
            args += ["-o", output]
        return self(*args)

    def jp(self, kind: str, name: str, expr: str, *, ns: str | None = None, default: str = "") -> str:
        """One jsonpath value, or ``default`` when the object/expression is missing."""
        ran = self.get(kind, name, ns=ns, jsonpath=expr)
        return ran.out if ran.ok and ran.out else default

    def probe(self, timeout: float = 15) -> Ran:
        """Cheap reachability check (``GET /readyz``) for the preflight.

        Distinguishes an unreachable cluster (non-zero rc) from an object that
        is simply absent, which :meth:`jp` deliberately collapses to ``default``.
        """
        return self("get", "--raw=/readyz", timeout=timeout)

    def json(self, kind: str, name: str | None = None, *, ns: str | None = None, selector: str | None = None) -> Any:
        ran = self.get(kind, name, ns=ns, selector=selector, output="json")
        if not ran.ok:
            raise RuntimeError(f"kubectl get {kind} failed: {ran.err[:200]}")
        return json.loads(ran.out) if ran.out else None

    def exists(self, kind: str, name: str, *, ns: str | None = None) -> bool:
        return self.get(kind, name, ns=ns, output="name").ok

    def names(self, kind: str, *, ns: str | None = None, selector: str | None = None,
              all_namespaces: bool = False) -> list[str]:
        """Object names (``name`` only, no kind prefix) for a list query."""
        ran = self.get(kind, ns=ns, selector=selector, output="name", all_namespaces=all_namespaces)
        return [ln.split("/")[-1] for ln in ran.stdout.splitlines() if ln.strip()]

    # ------------------------------------------------------------ mutations
    def apply(self, manifest: str, *, ns: str | None = None, server_side: bool = False) -> Ran:
        args = [*self._ns(ns), "apply", "-f", "-"]
        if server_side:
            args.append("--server-side")
        return self(*args, input=manifest)

    def delete(
        self,
        kind: str,
        name: str | None = None,
        *,
        ns: str | None = None,
        selector: str | None = None,
        wait: bool = True,
        ignore_not_found: bool = True,
        timeout: int | None = None,
    ) -> Ran:
        args = [*self._ns(ns), "delete", kind]
        if name:
            args.append(name)
        if selector:
            args += ["-l", selector]
        if ignore_not_found:
            args.append("--ignore-not-found")
        if not wait:
            args.append("--wait=false")
        if timeout:
            args.append(f"--timeout={timeout}s")
        return self(*args)

    def patch(self, kind: str, name: str, patch: Any, *, ns: str | None = None, type: str = "merge") -> Ran:
        text = patch if isinstance(patch, str) else json.dumps(patch)
        return self(*self._ns(ns), "patch", kind, name, f"--type={type}", "-p", text)

    def exec(self, pod: str, *argv: str, container: str | None = None, ns: str | None = None, timeout: int | None = None) -> Ran:
        args = [*self._ns(ns), "exec", pod]
        if container:
            args += ["-c", container]
        args += ["--", *argv]
        return self(*args, timeout=timeout)

    def logs(self, *, ns: str | None = None, selector: str | None = None, since: str | None = None, tail: int | None = None) -> Ran:
        args = [*self._ns(ns), "logs"]
        if selector:
            args += ["-l", selector]
        if since:
            args.append(f"--since={since}")
        if tail:
            args.append(f"--tail={tail}")
        return self(*args)

    def rollout_status(self, target: str, *, ns: str | None = None, timeout: int = 240) -> Ran:
        return self(*self._ns(ns), "rollout", "status", target, f"--timeout={timeout}s")

    def wait_pod(self, pod: str, *, ns: str | None = None, timeout: int = 300) -> Ran:
        return self(*self._ns(ns), "wait", "--for=condition=Ready", f"pod/{pod}", f"--timeout={timeout}s")


@dataclass
class Cluster:
    key: str
    name: str
    cluster_id: str
    data: Kube
    mgmt: Kube
    data_ns: str = "rlark-system"

    @property
    def mgmt_ns(self) -> str:
        return self.cluster_id


class Kind:
    """Host-level ``kind`` helpers (node containers / image loading)."""

    def __init__(self, shell: Shell):
        self.shell = shell

    def clusters(self) -> list[str]:
        return [n for n in self.shell.run(["kind", "get", "clusters"]).stdout.split() if n]

    def nodes(self, cluster: str | None = None) -> list[str]:
        argv = ["kind", "get", "nodes"]
        if cluster:
            argv += ["--name", cluster]
        return [n for n in self.shell.run(argv).stdout.split() if n]

    def containers_up(self) -> list[str]:
        ran = self.shell.run(["docker", "ps", "--format", "{{.Names}}\t{{.Status}}"])
        return [ln.split("\t", 1)[0] for ln in ran.stdout.splitlines() if "\tUp" in ln]

    def image_id(self, image: str) -> str:
        ran = self.shell.run(["docker", "image", "inspect", "-f", "{{.Id}}", image], quiet=True)
        return ran.out if ran.ok else ""

    def load(self, cluster: str, images: Iterable[str]) -> Ran:
        return self.shell.run(["kind", "load", "docker-image", *images, "--name", cluster], timeout=1800)

    def node_has_image(self, node: str, needle: str) -> bool:
        ran = self.shell.run(["docker", "exec", node, "crictl", "images", "-q"], quiet=True)
        return needle in ran.stdout
