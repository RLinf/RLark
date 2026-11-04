"""Self-tests for the kindtest framework itself.

No cluster is required: these exercise config parsing, the evidence book, the
run-archive step and the setup preflight (which fails as Blocked, not FAIL).
Run from ``test/kindtest``:

    python3 test_framework.py
    python3 -m unittest test_framework -v
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from kindtest.config import Config  # noqa: E402
from kindtest.context import EvidenceBook  # noqa: E402
from kindtest.model import Status  # noqa: E402
from kindtest.registry import Blocked, FlowRun, Item, RegistryError  # noqa: E402
from kindtest.runner import Runner  # noqa: E402
from kindtest.shell import Ran  # noqa: E402
from kindtest.versions import (  # noqa: E402
    RolloutTarget,
    Version,
    VersionDeployer,
    VersionError,
    _rewrite_args,
)

_TESTS_DIR = Path(__file__).resolve().parent / "tests"

_CONFIG = """\
run:
  id: selftest
clusters:
  control:
    kubeconfig: {base}/kubeconfig
    kcp: {base}/kcp.kubeconfig
  data:
    a:
      name: data-a
      clusterId: rlark-data-a
      kubeconfig: {base}/a.kubeconfig
report:
  dir: {base}/reports
tests:
  dir: {tests}
"""


def _runner(base: Path, *, extra: str = "", dry_run: bool = True) -> Runner:
    cfg_path = base / "config.yaml"
    cfg_path.write_text(_CONFIG.format(base=base, tests=_TESTS_DIR) + extra)
    return Runner(Config.load(cfg_path), dry_run=dry_run)


class EvidenceBookTest(unittest.TestCase):
    def test_add_creates_directory_on_demand(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "evidence"
            book = EvidenceBook(root)
            # nothing is created before the run archives a previous directory
            self.assertFalse(root.exists())
            book.add("a.txt", "x")
            self.assertTrue((root / "a.txt").is_file())

    def test_add_self_heals_after_directory_moves(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "evidence"
            book = EvidenceBook(root)
            book.add("a.txt", "x")
            # _archive_previous() renames evidence/ -> evidence-<oldsha>/
            root.rename(root.with_name("evidence-old"))
            book.add("b.txt", "y")  # must not raise FileNotFoundError
            self.assertTrue((root / "b.txt").is_file())


class RunnerArchiveTest(unittest.TestCase):
    def _seed_previous_run(self, runner: Runner) -> Path:
        rd = runner.report_dir
        (rd / "run-manifest.json").write_text(json.dumps({"target_ref": "main@470a3d6c"}))
        (rd / "full-test-result.md").write_text("# old\n")
        (rd / "evidence").mkdir()
        (rd / "evidence" / "old.txt").write_text("old\n")
        return rd

    def test_constructor_does_not_archive(self) -> None:
        """``kt plan`` only constructs a Runner; it must not touch the report dir."""
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            (base / "reports").mkdir()
            (base / "reports" / "run-manifest.json").write_text(
                json.dumps({"target_ref": "main@470a3d6c"})
            )
            _runner(base)
            self.assertTrue((base / "reports" / "run-manifest.json").is_file())

    def test_archive_then_evidence_write_recovers(self) -> None:
        """Regression: after archiving evidence/, the next add() must still work."""
        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(Path(tmp))
            # construction is lazy: no evidence/ exists before archiving
            self.assertFalse(runner.config.evidence_dir.exists())
            rd = self._seed_previous_run(runner)

            runner._archive_previous()
            runner.evidence.ensure()

            self.assertTrue((rd / "run-manifest-470a3d6c.json").is_file())
            self.assertTrue((rd / "full-test-result-470a3d6c.md").is_file())
            self.assertTrue((rd / "evidence-470a3d6c" / "old.txt").is_file())

            runner.evidence.add("new.txt", "new")
            self.assertTrue((rd / "evidence" / "new.txt").is_file())

    def test_archive_survives_alternating_targets(self) -> None:
        """Repeated upgrade checks that flip the target must not collide on archive."""
        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(Path(tmp))
            rd = runner.report_dir
            for i, target in enumerate(["main@sha1", "main@sha2", "main@sha1", "main@sha2"]):
                (rd / "run-manifest.json").write_text(json.dumps({"target_ref": target}))
                (rd / "evidence").mkdir(exist_ok=True)
                (rd / "evidence" / f"e{i}.txt").write_text(target)
                runner._archive_previous()  # must not raise OSError
            self.assertTrue((rd / "evidence-sha1").is_dir())
            self.assertTrue((rd / "evidence-sha2").is_dir())
            self.assertTrue((rd / "evidence-sha1-1").is_dir())
            self.assertTrue((rd / "evidence-sha2-1").is_dir())


class PreflightTest(unittest.TestCase):
    """Setup/preflight problems must be Blocked, never a pile of test failures."""

    def test_missing_kubeconfig_blocks_every_item(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(Path(tmp), dry_run=False)
            runner.load_tests()
            report = runner.run()

            self.assertTrue(report.blocked_reason.startswith("前置检查失败"), report.blocked_reason)
            self.assertTrue(report.cases)
            self.assertTrue(all(c.status is Status.BLOCKED for c in report.cases))
            self.assertFalse([c for c in report.cases if c.status is Status.FAIL])
            markdown = (runner.report_dir / runner.config.report_file).read_text()
            self.assertIn("运行未开始", markdown)
            manifest = json.loads((runner.report_dir / "run-manifest.json").read_text())
            self.assertEqual(manifest["blocked_reason"], report.blocked_reason)

    def test_kubectl_missing_blocks(self) -> None:
        from unittest import mock

        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(Path(tmp), dry_run=False)
            with mock.patch("kindtest.runner.shutil.which", return_value=None):
                with self.assertRaises(Blocked) as ctx:
                    runner._preflight()
            self.assertIn("kubectl", str(ctx.exception))

    def test_initial_version_failure_blocks(self) -> None:
        from unittest import mock

        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(Path(tmp), dry_run=False, extra="""
versions:
  initial: target
  definitions:
    target: {ref: t, images: {rlark: rlark:target}}
  rollout:
    control:
      - {kind: deploy, name: rlark-gateway, container: rlark-gateway, slot: rlark}
""")
            with mock.patch.object(runner.ctx, "deploy_version", side_effect=RuntimeError("boom")):
                with self.assertRaises(Blocked) as ctx:
                    runner._deploy_initial_version()
            self.assertIn("初始版本", str(ctx.exception))


class UniqueItemIdTest(unittest.TestCase):
    """Item ids key the report matrix; a duplicate must be rejected, not merged."""

    @staticmethod
    def _runs(*ids: str) -> list[FlowRun]:
        return [
            FlowRun("REG", "REG", f"flow-{item_id}", "flow", lambda t: None, [Item(item_id, item_id)])
            for item_id in ids
        ]

    def test_duplicate_ids_are_rejected(self) -> None:
        with self.assertRaises(RegistryError) as ctx:
            Runner._assert_unique_item_ids(self._runs("REG-01", "REG-01"))
        msg = str(ctx.exception)
        self.assertIn("REG-01", msg)
        self.assertIn("flow-REG-01", msg)  # both declaring flows are listed

    def test_unique_ids_pass(self) -> None:
        Runner._assert_unique_item_ids(self._runs("REG-01", "REG-02"))  # must not raise


class OnlySelectionTest(unittest.TestCase):
    """``--only`` is exact by default; a trailing ``*`` opts into prefix matching."""

    def _runner(self, tmp: str) -> Runner:
        return _runner(Path(tmp))

    def test_exact_id_matches_base_and_parametrized(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            runner = self._runner(tmp)
            runner.only = ["REG-01"]
            self.assertTrue(runner._item_selected("REG-01"))
            self.assertTrue(runner._item_selected("REG-01[data-a]"))
            self.assertFalse(runner._item_selected("REG-011"))
            self.assertFalse(runner._item_selected("REG-010"))

    def test_trailing_star_selects_a_prefix(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            runner = self._runner(tmp)
            runner.only = ["REG-0*"]
            self.assertTrue(runner._item_selected("REG-01"))
            self.assertTrue(runner._item_selected("REG-09[data-a]"))
            self.assertFalse(runner._item_selected("REG-10"))


class RewriteArgsTest(unittest.TestCase):
    """``_rewrite_args`` rewrites in place and errors on a missing prefix."""

    def test_rewrites_in_place(self) -> None:
        self.assertEqual(
            ["--image=rlark:new", "--mode=cluster"],
            _rewrite_args(["--image=rlark:old", "--mode=cluster"], ["--image="], "rlark:new"),
        )

    def test_noop_returns_none(self) -> None:
        self.assertIsNone(_rewrite_args(["--image=rlark:new"], ["--image="], "rlark:new"))

    def test_missing_prefix_raises_by_default(self) -> None:
        with self.assertRaises(VersionError) as ctx:
            _rewrite_args(["--mode=cluster"], ["--image="], "rlark:new")
        self.assertIn("--image=", str(ctx.exception))

    def test_append_missing_is_explicit_opt_in(self) -> None:
        self.assertEqual(
            ["--mode=cluster", "--image=rlark:new"],
            _rewrite_args(["--mode=cluster"], ["--image="], "rlark:new", append_missing=True),
        )


class _RecordingKube:
    """Kube stand-in for ``_apply``: records patches and bare ``kubectl`` calls."""

    def __init__(self, obj: dict) -> None:
        self._obj = obj
        self.patches: list[tuple[str, str, list]] = []
        self.verbs: list[list] = []

    def json(self, kind: str, name: str, *, ns: str | None = None) -> dict:
        return self._obj

    def patch(self, kind: str, name: str, patch: list, *, ns: str | None = None,
              type: str | None = None) -> Ran:  # noqa: A002 - mirrors Kube.patch
        self.patches.append((kind, name, patch))
        return Ran(argv=[], rc=0)

    def __call__(self, *argv: object) -> Ran:
        self.verbs.append(list(argv))
        return Ran(argv=[], rc=0)


class ApplyPatchTest(unittest.TestCase):
    """Issue: image and args must land in ONE spec write (one rollout, no halfway state)."""

    def _apply(self, obj: dict, target: RolloutTarget, image: str) -> tuple[_RecordingKube, bool]:
        kube = _RecordingKube(obj)
        changed = VersionDeployer._apply(None, kube, "rlark-system", target, image)  # type: ignore[arg-type]
        return kube, changed

    def test_image_and_args_patched_once(self) -> None:
        obj = {"spec": {"template": {"spec": {"containers": [
            {"name": "rlark-agent", "image": "rlark:old",
             "args": ["--mode=cluster", "--image=rlark:old"]},
        ]}}}}
        target = RolloutTarget(kind="deploy", name="rlark-agent", container="rlark-agent",
                               slot="rlark", args=["--image="])
        kube, changed = self._apply(obj, target, "rlark:new")
        self.assertTrue(changed)
        self.assertEqual([], kube.verbs)  # no separate `kubectl set image`
        self.assertEqual(1, len(kube.patches))  # exactly one spec write
        ops = kube.patches[0][2]
        self.assertEqual(
            ["/spec/template/spec/containers/0/image",
             "/spec/template/spec/containers/0/args"],
            [op["path"] for op in ops],
        )
        self.assertEqual("rlark:new", ops[0]["value"])
        self.assertEqual(["--mode=cluster", "--image=rlark:new"], ops[1]["value"])

    def test_absent_args_are_added(self) -> None:
        obj = {"spec": {"template": {"spec": {"containers": [{"name": "c", "image": "old"}]}}}}
        target = RolloutTarget(kind="deploy", name="d", container="c", args=["--image="],
                               append_missing=True)
        kube, changed = self._apply(obj, target, "new")
        self.assertTrue(changed)
        op = kube.patches[0][2][-1]
        self.assertEqual("add", op["op"])
        self.assertEqual(["--image=new"], op["value"])

    def test_no_change_patches_nothing(self) -> None:
        obj = {"spec": {"template": {"spec": {"containers": [
            {"name": "c", "image": "new", "args": ["--image=new"]},
        ]}}}}
        target = RolloutTarget(kind="deploy", name="d", container="c", args=["--image="])
        kube, changed = self._apply(obj, target, "new")
        self.assertFalse(changed)
        self.assertEqual([], kube.patches)

    def test_missing_prefix_fails_the_rollout(self) -> None:
        obj = {"spec": {"template": {"spec": {"containers": [
            {"name": "c", "image": "old", "args": ["--mode=cluster"]},
        ]}}}}
        target = RolloutTarget(kind="deploy", name="d", container="c", args=["--image="])
        with self.assertRaises(VersionError) as ctx:
            self._apply(obj, target, "new")
        self.assertIn("--image=", str(ctx.exception))


class SlotSemanticsTest(unittest.TestCase):
    """Slots are resolved one way: display lists all, selection is strict."""

    def test_display_lists_every_slot(self) -> None:
        version = Version(name="target", images={"rlark": "rlark:x", "ui": "ui:x"})
        self.assertEqual("rlark=rlark:x、ui=ui:x", version.image)

    def test_display_prefers_default_then_single(self) -> None:
        self.assertEqual("d", Version(name="t", images={"default": "d", "ui": "u"}).image)
        self.assertEqual("only", Version(name="t", images={"rlark": "only"}).image)

    def test_selection_without_slot_is_strict(self) -> None:
        version = Version(name="target", images={"rlark": "a", "ui": "b"})
        with self.assertRaises(VersionError):
            version.image_for("")
        self.assertEqual("b", version.image_for("ui"))
        with self.assertRaises(VersionError):
            version.image_for("nope")

    def test_config_and_version_agree(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            cfg_path = base / "config.yaml"
            cfg_path.write_text(
                _CONFIG.format(base=base, tests=_TESTS_DIR)
                + "\nversions:\n"
                + "  initial: target\n"
                + "  definitions:\n"
                + "    target: {ref: t, images: {rlark: rlark:x, ui: ui:x}}\n"
            )
            config = Config.load(cfg_path)
            # display path lists every slot and agrees with Version.image
            self.assertEqual(
                Version(name="target", images={"rlark": "rlark:x", "ui": "ui:x"}).image,
                config.target_image_display,
            )
            self.assertEqual("rlark=rlark:x、ui=ui:x", config.target_image_display)
            # value path is strict: several slots without a default is an error
            with self.assertRaises(VersionError):
                _ = config.target_image

    def test_config_single_image_resolves(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            cfg_path = base / "config.yaml"
            cfg_path.write_text(
                _CONFIG.format(base=base, tests=_TESTS_DIR)
                + "\nversions:\n"
                + "  initial: target\n"
                + "  definitions:\n"
                + "    target: {ref: t, image: rlark:only}\n"
            )
            config = Config.load(cfg_path)
            self.assertEqual("rlark:only", config.target_image)
            self.assertEqual("rlark:only", config.target_image_display)

    def test_render_is_lazy_for_ambiguous_target(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            runner = _runner(
                Path(tmp),
                extra="\nversions:\n"
                + "  initial: target\n"
                + "  definitions:\n"
                + "    target: {ref: t, images: {rlark: rlark:x, ui: ui:x}}\n",
            )
            # a template that does not use the ambiguous image still renders
            self.assertEqual("hi", runner.ctx.render("hi"))
            # using it is an explicit error, not a silent pick
            with self.assertRaises(VersionError):
                runner.ctx.render("image={{targetImage}}")


if __name__ == "__main__":
    unittest.main(verbosity=2)
