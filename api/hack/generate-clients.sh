#!/usr/bin/env bash

# Copyright 2024 The RLInf Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generate typed clientset, listers, and informers for rlark CRDs.
#
# Usage:
#   ./hack/generate-clients.sh [--with-watch]
#
# Flags:
#   --with-watch    Generate listers and informers in addition to clientset

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

# Pin code-generator version to match k8s.io/apimachinery in go.mod
export KUBE_CODEGEN_TAG="v0.36.1"

# source the code-gen helpers
source "${SCRIPT_ROOT}/hack/kube_codegen.sh"

WITH_WATCH=false
for arg in "$@"; do
    case "$arg" in
        --with-watch)
            WITH_WATCH=true
            shift
            ;;
    esac
done

OUTPUT_PKG="github.com/rlinf/rlark/api/kubeclients"
OUTPUT_DIR="${SCRIPT_ROOT}/kubeclients"

echo ">>> Generating typed clients..."

WATCH_FLAGS=()
if [ "$WITH_WATCH" = true ]; then
    WATCH_FLAGS=(--with-watch)
    echo "    (with listers + informers)"
fi

kube::codegen::gen_client \
    "${SCRIPT_ROOT}" \
    --output-dir "${OUTPUT_DIR}" \
    --output-pkg "${OUTPUT_PKG}" \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    --clientset-name "clientset" \
    --versioned-name "versioned" \
    --with-applyconfig \
    "${WATCH_FLAGS[@]:+"${WATCH_FLAGS[@]}"}"

echo ">>> Done! Generated code is in ${OUTPUT_DIR}"