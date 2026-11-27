#!/usr/bin/env bash
set -euo pipefail

fixture_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_name="rlark-mediamtx-fixture-$(date +%s)-$$"
compose=(docker compose -p "$project_name" -f "$fixture_dir/compose.yaml" --profile test)

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans
}

trap cleanup EXIT
"${compose[@]}" up --detach mediamtx publisher
"${compose[@]}" run --rm --no-deps test-client python /tests/smoke.py
"${compose[@]}" run --rm --no-deps control-publish-probe
"${compose[@]}" run --rm --no-deps publisher-path-probe
