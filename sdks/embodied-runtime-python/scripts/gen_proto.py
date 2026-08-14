#!/usr/bin/env python3
"""Generate Python gRPC stubs from the embodied-runtime .proto files.

Runs grpc_tools.protoc and rewrites the generated ``*_pb2_grpc.py`` imports so
they resolve inside the ``embodied_runtime.gen`` package (protoc emits imports
relative to ``--proto_path``, which don't match the installed package layout).

Usage:
    python scripts/gen_proto.py            # uses bundled grpcio-tools
    python scripts/gen_proto.py --protoc   # uses system protoc + grpc plugin

Regenerate whenever the .proto files change. The output is checked in.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

# repo layout
REPO_ROOT = Path(__file__).resolve().parents[3]
PROTO_DIR = REPO_ROOT / "proto" / "embodied-runtime"
GEN_ROOT = REPO_ROOT / "sdks" / "embodied-runtime-python" / "embodied_runtime" / "gen"

# (proto_path_relative, module, alias_prefix)
PROTO_SPECS = [
    ("roscontroller/v1/robot.proto", "roscontroller", "v1", "robot"),
    ("cameracontroller/v1/camera.proto", "cameracontroller", "v1", "camera"),
]


def run_grpcio_tools() -> None:
    """Generate stubs using the grpcio-tools package (no external protoc)."""
    from grpc_tools import protoc  # noqa: PLC0415

    args = [
        "grpc_tools.protoc",
        f"--proto_path={PROTO_DIR}",
        f"--python_out={GEN_ROOT}",
        f"--grpc_python_out={GEN_ROOT}",
    ] + [str(PROTO_DIR / spec[0]) for spec in PROTO_SPECS]

    rc = protoc.main(args)
    if rc != 0:
        raise SystemExit(f"grpc_tools.protoc failed with exit code {rc}")


def fix_grpc_imports() -> None:
    """Rewrite ``from <pkg>.v1 import <mod>_pb2`` → package-qualified imports.

    protoc emits e.g. ``from roscontroller.v1 import robot_pb2 as ...`` which
    only resolves if ``roscontroller`` is importable at top level. We rewrite to
    ``from embodied_runtime.gen.roscontroller.v1 import robot_pb2 as ...`` so
    the stubs work once the package is installed.
    """
    pattern = re.compile(
        r"^from (?P<pkg>\w+)\.(?P<ver>\w+) import (?P<mod>\w+_pb2) as (?P<alias>\w+)$",
        re.MULTILINE,
    )
    replacement = r"from embodied_runtime.gen.\g<pkg>.\g<ver> import \g<mod> as \g<alias>"

    for _proto_rel, pkg, ver, mod in PROTO_SPECS:
        grpc_file = GEN_ROOT / pkg / ver / f"{mod}_pb2_grpc.py"
        text = grpc_file.read_text()
        new_text, count = pattern.subn(replacement, text)
        if count == 0:
            raise SystemExit(
                f"expected pb2 import not found in {grpc_file}; "
                f"protoc output format may have changed"
            )
        grpc_file.write_text(new_text)


def ensure_init_files() -> None:
    """Create __init__.py in every generated package directory."""
    GEN_ROOT.mkdir(parents=True, exist_ok=True)
    (GEN_ROOT / "__init__.py").write_text(
        '"""Generated gRPC stubs for the embodied-runtime services.\n'
        "\n"
        "Do not edit by hand — regenerate via ``make proto-python``.\n"
        '"""\n'
    )
    for _proto_rel, pkg, ver, _mod in PROTO_SPECS:
        (GEN_ROOT / pkg).mkdir(parents=True, exist_ok=True)
        (GEN_ROOT / pkg / "__init__.py").write_text("")
        (GEN_ROOT / pkg / ver).mkdir(parents=True, exist_ok=True)
        (GEN_ROOT / pkg / ver / "__init__.py").write_text("")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--protoc",
        action="store_true",
        help="use the system protoc with grpc_python plugin instead of grpcio-tools",
    )
    args = parser.parse_args()

    if args.protoc:
        raise SystemExit("--protoc mode is not implemented yet; use grpcio-tools")
    else:
        run_grpcio_tools()

    fix_grpc_imports()
    ensure_init_files()

    print(f"Generated Python stubs in {GEN_ROOT.relative_to(REPO_ROOT)}/")
    for _proto_rel, pkg, ver, mod in PROTO_SPECS:
        print(f"  {pkg}/{ver}/{mod}_pb2.py + {mod}_pb2_grpc.py")


if __name__ == "__main__":
    sys.exit(main())
