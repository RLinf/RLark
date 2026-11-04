#!/usr/bin/env bash
set -euo pipefail

# Updates RLark CRDs in the kcp API hosted by the Kubernetes cluster in KUBECONFIG.
# Usage: KUBECONFIG=/path/to/host.kubeconfig ./update-kcp-crds.sh [kcp-namespace] [kcp-pod]
: "${KUBECONFIG:?KUBECONFIG must point to the host Kubernetes kubeconfig}"
HOST_KUBECONFIG="$KUBECONFIG"
KCP_NAMESPACE="${1:-rlark-system}"
KCP_POD="${2:-kcp-0}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
KCP_KUBECONFIG="$(mktemp)"
CRD_DIR="$(mktemp -d)"
PORT_FORWARD_LOG="$(mktemp)"
PORT_FORWARD_PID=""
KCP_PORT=""

cleanup() {
  if [[ -n "$PORT_FORWARD_PID" ]]; then
    kill "$PORT_FORWARD_PID" 2>/dev/null || true
    wait "$PORT_FORWARD_PID" 2>/dev/null || true
  fi
  rm -f "$KCP_KUBECONFIG" "$PORT_FORWARD_LOG"
  rm -rf "$CRD_DIR"
}
trap cleanup EXIT

kubectl --kubeconfig "$HOST_KUBECONFIG" -n "$KCP_NAMESPACE" \
  get configmap rlark-kcp-kubeconfig \
  -o go-template='{{index .data "admin.kubeconfig"}}' > "$KCP_KUBECONFIG"

KCP_PORT="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"

kubectl --kubeconfig "$HOST_KUBECONFIG" -n "$KCP_NAMESPACE" \
  port-forward "pod/$KCP_POD" "$KCP_PORT:6443" >"$PORT_FORWARD_LOG" 2>&1 &
PORT_FORWARD_PID=$!

for _ in {1..20}; do
  if grep -q "Forwarding from 127.0.0.1:$KCP_PORT" "$PORT_FORWARD_LOG"; then
    break
  fi
  if ! kill -0 "$PORT_FORWARD_PID" 2>/dev/null; then
    cat "$PORT_FORWARD_LOG" >&2
    exit 1
  fi
  sleep 1
done

if ! grep -q "Forwarding from 127.0.0.1:$KCP_PORT" "$PORT_FORWARD_LOG"; then
  cat "$PORT_FORWARD_LOG" >&2
  exit 1
fi

KCP_CONTEXT="$(kubectl --kubeconfig "$KCP_KUBECONFIG" config current-context)"
KCP_SERVER="$(kubectl --kubeconfig "$KCP_KUBECONFIG" config view --raw --minify -o jsonpath='{.clusters[0].cluster.server}')"
KCP_CLUSTER="$(kubectl --kubeconfig "$KCP_KUBECONFIG" config view --raw --minify -o jsonpath='{.contexts[0].context.cluster}')"
KCP_SERVER_HOSTPORT="${KCP_SERVER#https://}"
KCP_SERVER_HOST="${KCP_SERVER_HOSTPORT%%:*}"
KCP_SERVER_PATH="${KCP_SERVER_HOSTPORT#*/}"
if [[ "$KCP_SERVER_PATH" == "$KCP_SERVER_HOSTPORT" ]]; then
  KCP_SERVER_PATH=""
else
  KCP_SERVER_PATH="/$KCP_SERVER_PATH"
fi
kubectl --kubeconfig "$KCP_KUBECONFIG" config set-cluster "$KCP_CLUSTER" \
  --server="https://127.0.0.1:$KCP_PORT$KCP_SERVER_PATH" \
  --tls-server-name="$KCP_SERVER_HOST" >/dev/null

make -C "$PROJECT_ROOT/api" controller-gen
CONTROLLER_GEN="$(make -s -C "$PROJECT_ROOT/api" print-controller-gen)"
"$CONTROLLER_GEN" crd:maxDescLen=0,allowDangerousTypes=true \
  paths="$PROJECT_ROOT/api/rlark.io/..." \
  output:crd:artifacts:config="$CRD_DIR"

kubectl --kubeconfig "$KCP_KUBECONFIG" --context "$KCP_CONTEXT" \
  apply --validate=false -f "$CRD_DIR"

kubectl --kubeconfig "$KCP_KUBECONFIG" --context "$KCP_CONTEXT" \
  get crd clusters.rlinf.io \
  -o jsonpath='{.spec.versions[?(@.storage==true)].schema.openAPIV3Schema.properties.spec.properties}'
echo
