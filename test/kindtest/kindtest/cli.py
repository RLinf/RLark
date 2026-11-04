"""Command line interface: ``python3 -m kindtest <command>``.

Commands:
  validate  check the config without touching the environment
  plan      list every case that would run (grouped), no side effects
  run       execute the selected cases and write the report live
  report    regenerate the Markdown report from run-manifest.json
"""

from __future__ import annotations

import argparse
import json
import sys

from . import __version__
from .config import Config, ConfigError
from .model import CATEGORY_ORDER, Case, RunReport, Status
from .registry import RegistryError
from .report import write_report
from .runner import Runner


def main(argv: list[str] | None = None) -> int:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("-c", "--config", default=None, help="run config (default: config.yaml)")

    parser = argparse.ArgumentParser(prog="kindtest", description="RLark code-first Kind regression",
                                     parents=[common])
    sub = parser.add_subparsers(dest="command", required=True)

    for name in ("validate", "plan", "run", "report"):
        p = sub.add_parser(name, help=f"{name} the run", parents=[common])
        p.add_argument("--only", help="comma-separated ids/groups, e.g. ENV,REG-01; "
                                      "ids match exactly, add a trailing '*' for a prefix (REG-0*)")
        p.add_argument("--json", action="store_true", help="machine-readable output")
        if name == "run":
            p.add_argument("--dry-run", action="store_true", help="print commands without executing")
            p.add_argument("--verbose", action="store_true", help="echo every command")

    parser.add_argument("-V", "--version", action="version", version=f"kindtest {__version__}")
    args = parser.parse_args(argv)

    try:
        config = Config.load(args.config or "config.yaml")
    except ConfigError as exc:
        print(f"config error: {exc}", file=sys.stderr)
        return 2

    if args.command == "validate":
        return _validate(config, args.json)

    only = [t.strip() for t in args.only.split(",")] if getattr(args, "only", None) else None
    runner = Runner(config, only=only, dry_run=args.dry_run if args.command == "run" else False,
                    verbose=args.verbose if args.command == "run" else False)

    if args.command == "plan":
        return _plan(config, runner, args.json)

    if args.command == "run":
        runner.load_tests()
        if args.dry_run:
            return _plan(config, runner, args.json)
        try:
            report = runner.run()
        except RegistryError as exc:
            print(f"registry error: {exc}", file=sys.stderr)
            return 2
        return _summarize(report, args.json)

    if args.command == "report":
        return _regenerate(config, args.json)
    return 2


def _validate(config: Config, as_json: bool) -> int:
    problems = config.validate()
    if as_json:
        print(json.dumps({"ok": not problems, "problems": problems}, indent=2))
    elif problems:
        print("config INVALID:")
        for problem in problems:
            print(f"  - {problem}")
    else:
        print(f"config OK: theme={config.theme} runId={config.run_id} target={config.target_ref}")
        print(f"  report dir : {config.report_dir}")
        print(f"  tests dir  : {config.tests_dir}")
    return 1 if problems else 0


def _plan(config: Config, runner: Runner, as_json: bool) -> int:
    runner.load_tests()
    try:
        plan = runner.plan()
    except RegistryError as exc:
        print(f"registry error: {exc}", file=sys.stderr)
        return 2
    if as_json:
        print(json.dumps([{"group": k, "flows": [
            {
                "name": run.name, "kind": run.kind,
                "items": [{"id": it.id, "name": it.name, "expect": it.expect} for it in run.items],
            } for run in runs
        ]} for k, runs in plan], indent=2, ensure_ascii=False))
        return 0
    total = 0
    for key, runs in plan:
        items = sum(len(run.items) for run in runs)
        print(f"\n== {key} ({len(runs)} 流程 / {items} 观测点) ==")
        for run in runs:
            if run.kind == "case":
                it = run.items[0]
                print(f"  {it.id:16s} {it.name}  [{it.expect}]")
            else:
                print(f"  流程 {run.name}")
                for it in run.items:
                    print(f"    {it.id:16s} {it.name}  [{it.expect}]")
            total += len(run.items)
    print(f"\ntotal: {total} 观测点")
    return 0


def _summarize(report: RunReport, as_json: bool) -> int:
    counts = report.counts()
    if as_json:
        print(json.dumps(report.to_dict(), indent=2, ensure_ascii=False))
    else:
        print("")
        print("=" * 60)
        for key in CATEGORY_ORDER:
            cc = report.category_counts(key)
            if not any(cc.values()):
                continue
            print(f"  {key:<6} 通过 {cc['pass']:>2}  受限 {cc['restricted']:>2}  "
                  f"失败 {cc['fail']:>2}  blocked {cc['blocked']:>2}")
        print("-" * 60)
        print(f"  合计 {len(report.cases)} 项: {counts['pass']} 通过 / {counts['restricted']} 受限 / "
              f"{counts['fail']} 分支相关失败 / {counts['blocked']} Blocked / {counts['skipped']} 未执行")
        print("=" * 60)
    return 1 if counts["fail"] else 0


def _regenerate(config: Config, as_json: bool) -> int:
    manifest = config.report_dir / "run-manifest.json"
    if not manifest.is_file():
        print(f"no run manifest at {manifest}", file=sys.stderr)
        return 1
    data = json.loads(manifest.read_text())
    report = RunReport(
        theme=data["theme"], run_id=data["run_id"], base_ref=data["base_ref"], target_ref=data["target_ref"],
        started=data["started"], finished=data.get("finished", ""), config=data.get("config", {}),
        evidence_dir=data.get("evidence_dir", ""), environment=data.get("environment", {}),
        blocked_reason=data.get("blocked_reason", ""),
        notes=data.get("notes", {}),
    )
    report.cases = [
        Case(id=c["id"], name=c["name"], expected=c.get("expected", ""), actual=c.get("actual", ""),
             status=Status(c.get("status", Status.SKIPPED.value)), source=c.get("source", ""),
             evidence=list(c.get("evidence", [])), duration_s=c.get("duration_s", 0.0))
        for c in data.get("cases", [])
    ]
    path = write_report(report, config)
    print(json.dumps({"report": str(path), "counts": report.counts()}, indent=2) if as_json
          else f"report regenerated: {path}")
    return 0
