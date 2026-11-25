#!/usr/bin/env bash
#
# cleanup-orphan-nodes.sh — delete RLark Node CRs whose data-plane Kubernetes
# Node no longer exists or no longer carries rlark.io/enabled=true.
#
# Node CRs live in the KCP that backs an RLark installation, one namespace per
# Cluster (Cluster.spec.agentNamespace). A CR can outlive its data-plane Node
# when the Agent is unhealthy: nothing then removes the stale entry from the
# managed pool. This script compares the pool against the data-plane nodes that
# rlark-gateway reports through its admin API and removes the mismatches.
#
# Topology this script expects
#   * --kubeconfig points at the cluster that runs the RLark installation
#     (rlark-server / kcp Service / rlark-gateway);
#   * --namespace is the RLark installation namespace, not a Node CR namespace;
#   * Node CR namespaces are discovered from the Gateway cluster list.
#
# Safety
#   * dry run by default; pass --apply to delete;
#   * CRs without status.kubernetesNodeUID are skipped (the Agent never reported
#     them, so this script cannot prove they are stale);
#   * a Node CR that a Task still observes (status.observedNodes) is skipped,
#     mirroring the unmanage guard;
#   * a Cluster whose data-plane node list could not be fetched is skipped;
#   * Node CR namespaces that no reachable Cluster claims are reported, not
#     deleted.
#
# Authentication
#   The script logs in to rlark-gateway as "admin" using the admin-password in
#   the KCP secret rlark-ui-auth (namespace "default"), and reads the KCP admin
#   kubeconfig from the rlark-kcp-kubeconfig ConfigMap (falling back to
#   deploy/kcp:/.kcp/admin.kubeconfig). It only needs kubectl, curl and python3
#   against the host cluster.
#
# Usage
#   ./hack/cleanup-orphan-nodes.sh --kubeconfig ~/.kube/dev-adm --namespace rlark-system-qa
#   ./hack/cleanup-orphan-nodes.sh --kubeconfig ~/.kube/dev-adm --namespace rlark-system-qa --apply
#
# Flags
#   --kubeconfig PATH       host cluster kubeconfig (default: $KUBECONFIG or ~/.kube/config)
#   --namespace NS          RLark installation namespace (required)
#   --context NAME          host kubeconfig context
#   --agent-namespace NS    only inspect this Node CR namespace (repeatable)
#   --apply                 delete orphan Node CRs (default: dry run)
#   --keep-port-forwards    leave the kcp / gateway port-forwards running
#   -h, --help              show this help
#
set -euo pipefail

HOST_KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
HOST_CONTEXT=""
INSTALL_NS=""
APPLY=false
KEEP_PORT_FORWARDS=false
AGENT_NAMESPACES=()

usage() {
    awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "${BASH_SOURCE[0]}"
}

log()  { printf '\033[36m[INFO]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[WARN]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[31m[ERR]\033[0m %s\n' "$*" >&2; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --kubeconfig)       HOST_KUBECONFIG="$2"; shift 2 ;;
        --namespace)        INSTALL_NS="$2"; shift 2 ;;
        --context)          HOST_CONTEXT="$2"; shift 2 ;;
        --agent-namespace)  AGENT_NAMESPACES+=("$2"); shift 2 ;;
        --apply)            APPLY=true; shift ;;
        --keep-port-forwards) KEEP_PORT_FORWARDS=true; shift ;;
        -h|--help)          usage; exit 0 ;;
        *) err "unknown argument: $1"; usage >&2; exit 2 ;;
    esac
done

if [[ -z "$INSTALL_NS" ]]; then
    err "--namespace is required (for example --namespace rlark-system-qa)"
    exit 2
fi
if [[ ! -f "$HOST_KUBECONFIG" ]]; then
    err "kubeconfig not found: $HOST_KUBECONFIG"
    exit 2
fi
for bin in kubectl curl python3; do
    command -v "$bin" >/dev/null 2>&1 || { err "required command not found: $bin"; exit 2; }
done

TMP_DIR="$(mktemp -d /tmp/rlark-orphan-nodes.XXXXXX)"
KCP_PF_PID=""
GW_PF_PID=""

cleanup() {
    if [[ "$KEEP_PORT_FORWARDS" != "true" ]]; then
        [[ -n "$KCP_PF_PID" ]] && kill "$KCP_PF_PID" 2>/dev/null || true
        [[ -n "$GW_PF_PID" ]] && kill "$GW_PF_PID" 2>/dev/null || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

kc-host() {
    local args=(--kubeconfig="$HOST_KUBECONFIG")
    [[ -n "$HOST_CONTEXT" ]] && args+=(--context="$HOST_CONTEXT")
    kubectl "${args[@]}" "$@"
}

# kc-cp talks to the KCP that holds the Node CR pool.
kc-cp() { kubectl --kubeconfig="$TMP_DIR/kcp.kubeconfig" --insecure-skip-tls-verify "$@"; }

# wait_for_port LOGFILE REMOTE_PORT prints the local port kubectl chose.
wait_for_port() {
    local logfile="$1" remote_port="$2" line=""
    for _ in $(seq 1 40); do
        line="$(grep -oE "127\.0\.0\.1:[0-9]+ -> ${remote_port}" "$logfile" 2>/dev/null | head -1 || true)"
        if [[ -n "$line" ]]; then
            line="${line#127.0.0.1:}"
            printf '%s' "${line%% *}"
            return 0
        fi
        sleep 0.5
    done
    return 1
}

start_port_forward() {
    local resource="$1" remote_port="$2" logfile="$3"
    : > "$logfile"
    setsid kubectl --kubeconfig="$HOST_KUBECONFIG" ${HOST_CONTEXT:+--context="$HOST_CONTEXT"} \
        -n "$INSTALL_NS" port-forward "$resource" ":${remote_port}" \
        > "$logfile" 2>&1 < /dev/null &
    echo $!
}

# forward_and_wait RESOURCE REMOTE_PORT LOGFILE RETRIES
# Sets PF_PID / PF_PORT, or returns 1 when every attempt fails.
forward_and_wait() {
    local resource="$1" remote_port="$2" logfile="$3" retries="${4:-3}" attempt port pid
    for attempt in $(seq 1 "$retries"); do
        pid="$(start_port_forward "$resource" "$remote_port" "$logfile")"
        if port="$(wait_for_port "$logfile" "$remote_port")"; then
            PF_PID="$pid"
            PF_PORT="$port"
            return 0
        fi
        kill "$pid" 2>/dev/null || true
        [[ -s "$logfile" ]] && tail -2 "$logfile" >&2 || true
        warn "port-forward to $resource failed (attempt $attempt/$retries)"
        sleep 2
    done
    return 1
}

# ───────────────────────── KCP kubeconfig ─────────────────────────
bootstrap_kcp() {
    log "reading KCP admin kubeconfig from $INSTALL_NS/rlark-kcp-kubeconfig"
    local raw="$TMP_DIR/kcp-admin.kubeconfig" attempt
    for attempt in 1 2 3; do
        kc-host -n "$INSTALL_NS" get configmap rlark-kcp-kubeconfig \
            -o go-template='{{index .data "admin.kubeconfig"}}' > "$raw" 2>/dev/null || true
        grep -q 'apiVersion' "$raw" && break
        [[ "$attempt" -lt 3 ]] && sleep 2
    done
    if ! grep -q 'apiVersion' "$raw"; then
        log "configmap unavailable, reading /.kcp/admin.kubeconfig from the kcp Deployment"
        for attempt in 1 2 3; do
            kc-host -n "$INSTALL_NS" exec deploy/kcp -- cat /.kcp/admin.kubeconfig > "$raw" 2>/dev/null || true
            grep -q 'apiVersion' "$raw" && break
            [[ "$attempt" -lt 3 ]] && sleep 2
        done
    fi
    if ! grep -q 'apiVersion' "$raw"; then
        err "cannot read the KCP admin kubeconfig from $INSTALL_NS (no rlark-kcp-kubeconfig ConfigMap and no deploy/kcp); is that an RLark installation namespace?"
        exit 1
    fi
    chmod 600 "$raw"

    if ! forward_and_wait svc/kcp 6443 "$TMP_DIR/kcp-pf.log"; then
        err "port-forward to svc/kcp failed"
        exit 1
    fi
    KCP_PF_PID="$PF_PID"
    local port="$PF_PORT"
    # Each server line looks like https://<host>:6443[/clusters/<name>]. Point it
    # at the port-forward and keep the path suffix so logical clusters still work.
    KCP_ADMIN_RAW="$raw" KCP_LOCAL_PORT="$port" python3 - > "$TMP_DIR/kcp.kubeconfig" <<'PY'
import os, re

raw = open(os.environ["KCP_ADMIN_RAW"]).read()
port = os.environ["KCP_LOCAL_PORT"]
print(re.sub(r"(server:\s*https://)[^\s/]+", r"\g<1>127.0.0.1:%s" % port, raw), end="")
PY
    chmod 600 "$TMP_DIR/kcp.kubeconfig"
    if ! kc-cp get ns --request-timeout=10s -o name >/dev/null 2>&1; then
        err "KCP is not reachable through the port-forward"
        exit 1
    fi
    log "KCP reachable at 127.0.0.1:$port"
}

# ───────────────────────── Gateway data-plane view ─────────────────────────
start_gateway() {
    if ! forward_and_wait svc/rlark-gateway 8090 "$TMP_DIR/gw-pf.log"; then
        err "port-forward to svc/rlark-gateway failed"
        exit 1
    fi
    GW_PF_PID="$PF_PID"
    GW_URL="http://127.0.0.1:$PF_PORT"
}

login() {
    local secret_ns admin_password body
    # The post-install hook writes rlark-ui-auth into the KCP "default"
    # namespace; fall back to a cluster-wide search if it is not there.
    if kc-cp -n default get secret rlark-ui-auth >/dev/null 2>&1; then
        secret_ns="default"
    else
        secret_ns="$(kc-cp get secret rlark-ui-auth -A --no-headers \
            -o custom-columns='NS:.metadata.namespace' 2>/dev/null | head -1 || true)"
    fi
    if [[ -z "$secret_ns" ]]; then
        err "secret rlark-ui-auth not found in KCP; cannot authenticate to the Gateway"
        exit 1
    fi
    admin_password="$(kc-cp -n "$secret_ns" get secret rlark-ui-auth \
        --template='{{index .data "admin-password"}}' | base64 -d)"
    if [[ -z "$admin_password" ]]; then
        err "admin-password is empty in $secret_ns/rlark-ui-auth"
        exit 1
    fi
    body="$(ADMIN_PASSWORD="$admin_password" python3 -c \
        'import json,os; print(json.dumps({"username":"admin","password":os.environ["ADMIN_PASSWORD"]}))')"
    local response
    if ! response="$(curl -fsS -m 20 -X POST "$GW_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' --data-binary "$body" 2>/dev/null)"; then
        err "Gateway login failed"
        exit 1
    fi
    TOKEN="$(printf '%s' "$response" | python3 -c \
        'import json,sys; d=json.load(sys.stdin); print(d.get("token",""))')"
    if [[ -z "$TOKEN" ]]; then
        err "Gateway login returned no token"
        exit 1
    fi
}

fetch_data_plane_nodes() {
    if ! curl -fsS -m 60 -H "Authorization: Bearer $TOKEN" \
        "$GW_URL/api/v1/data-plane-nodes" -o "$TMP_DIR/data-plane.json"; then
        err "GET /api/v1/data-plane-nodes failed"
        exit 1
    fi
    python3 - "$TMP_DIR/data-plane.json" > "$TMP_DIR/data-plane.tsv" <<'PY'
import json, sys

data = json.load(open(sys.argv[1]))
for cluster in data.get("clusters") or []:
    namespace = cluster.get("namespace") or ""
    name = cluster.get("cluster") or ""
    if cluster.get("error"):
        print("ERR\t%s\t%s\t%s" % (namespace, name, cluster["error"]))
        continue
    for node in cluster.get("nodes") or []:
        managed = "true" if node.get("managed") else "false"
        print("NODE\t%s\t%s\t%s" % (namespace, node.get("name", ""), managed))
PY

    local clusters nodes
    clusters="$(awk -F'\t' '$1 == "NODE" || $1 == "ERR" { print $2 }' "$TMP_DIR/data-plane.tsv" | sort -u | wc -l)"
    nodes="$(awk -F'\t' '$1 == "NODE"' "$TMP_DIR/data-plane.tsv" | wc -l)"
    if [[ "$clusters" -eq 0 ]]; then
        warn "the Gateway reported no data-plane clusters; nothing to compare against"
    else
        log "Gateway reported $clusters cluster(s) and $nodes data-plane node(s)"
    fi
}

observed_nodes_in_namespace() {
    # Node names still observed by a Task in this namespace, one per line.
    kc-cp -n "$1" get tasks.rlinf.io --no-headers \
        -o go-template='{{range .items}}{{range .status.observedNodes}}{{.}}{{"\n"}}{{end}}{{end}}' \
        2>/dev/null | sort -u
}

node_state_in_pool() {
    # $1 namespace, $2 node name -> prints "managed", "unmanaged" or "missing"
    awk -F'\t' -v ns="$1" -v name="$2" '
        $1 == "NODE" && $2 == ns && $3 == name { found = 1; managed = ($4 == "true") }
        END {
            if (!found) print "missing"
            else if (managed) print "managed"
            else print "unmanaged"
        }
    ' "$TMP_DIR/data-plane.tsv"
}

# ───────────────────────── Cleanup ─────────────────────────
ORPHANS=0
DELETED=0
SKIPPED=0

cleanup_namespace() {
    local ns="$1"
    local crs
    if ! crs="$(kc-cp -n "$ns" get nodes.rlinf.io --no-headers \
        -o custom-columns='NAME:.metadata.name,UID:.status.kubernetesNodeUID' 2>/dev/null)"; then
        warn "skip namespace $ns: cannot list nodes.rlinf.io"
        return 0
    fi
    if [[ -z "$crs" ]]; then
        log "namespace $ns: no Node CRs"
        return 0
    fi

    # Mirror the unmanage guard: never drop a Node CR that a Task still observes.
    local observed
    if ! observed="$(observed_nodes_in_namespace "$ns")"; then
        warn "skip namespace $ns: cannot list tasks.rlinf.io to check observed nodes"
        return 0
    fi

    local name uid state
    while read -r name uid; do
        [[ -z "$name" ]] && continue
        state="$(node_state_in_pool "$ns" "$name")"
        [[ "$state" == "managed" ]] && continue

        if [[ -z "$uid" || "$uid" == "<none>" ]]; then
            warn "skip $ns/$name: CR has no status.kubernetesNodeUID (not reported by an Agent)"
            SKIPPED=$((SKIPPED + 1))
            continue
        fi

        if [[ -n "$observed" ]] && grep -qxF "$name" <<< "$observed"; then
            warn "skip $ns/$name: a Task still observes this node"
            SKIPPED=$((SKIPPED + 1))
            continue
        fi

        local reason
        if [[ "$state" == "missing" ]]; then
            reason="data-plane node does not exist"
        else
            reason="data-plane node lacks rlark.io/enabled=true"
        fi
        ORPHANS=$((ORPHANS + 1))
        printf '  %-26s %-24s %s\n' "$ns" "$name" "$reason"

        if [[ "$APPLY" == "true" ]]; then
            if kc-cp -n "$ns" delete nodes.rlinf.io "$name" --wait=false >/dev/null; then
                DELETED=$((DELETED + 1))
                log "deleted $ns/$name"
            else
                warn "failed to delete $ns/$name"
            fi
        fi
    done <<< "$crs"
}

namespace_selected() {
    local ns="$1"
    [[ ${#AGENT_NAMESPACES[@]} -eq 0 ]] && return 0
    local wanted
    for wanted in "${AGENT_NAMESPACES[@]}"; do
        [[ "$wanted" == "$ns" ]] && return 0
    done
    return 1
}

main() {
    log "host kubeconfig=$HOST_KUBECONFIG install namespace=$INSTALL_NS apply=$APPLY"

    bootstrap_kcp
    start_gateway
    login
    fetch_data_plane_nodes

    local namespaces
    namespaces="$(awk -F'\t' '$1 == "NODE" || $1 == "ERR" { print $2 }' "$TMP_DIR/data-plane.tsv" | sort -u)"

    # Clusters whose data-plane list could not be fetched are skipped: absence
    # of a node there is not evidence that the Node CR is stale.
    local errored_ns
    errored_ns="$(awk -F'\t' '$1 == "ERR" { print $2 }' "$TMP_DIR/data-plane.tsv" | sort -u)"
    local ns
    while read -r ns; do
        [[ -z "$ns" ]] && continue
        if grep -qxF "$ns" <<< "$errored_ns"; then
            warn "skip namespace $ns: Gateway could not list its data-plane nodes"
            continue
        fi
        namespace_selected "$ns" || continue
        cleanup_namespace "$ns"
    done <<< "$namespaces"

    # Node CR namespaces that no reachable Cluster claims are reported only.
    local pool_ns
    pool_ns="$(kc-cp get nodes.rlinf.io -A --no-headers \
        -o custom-columns='NS:.metadata.namespace' 2>/dev/null | sort -u || true)"
    while read -r ns; do
        [[ -z "$ns" ]] && continue
        grep -qxF "$ns" <<< "$namespaces" && continue
        warn "namespace $ns has Node CRs but no Cluster in this installation claims it; skipped"
    done <<< "$pool_ns"

    echo
    if [[ "$APPLY" == "true" ]]; then
        log "orphan Node CRs: $ORPHANS, deleted: $DELETED, skipped: $SKIPPED"
    else
        log "orphan Node CRs: $ORPHANS, skipped: $SKIPPED (dry run; pass --apply to delete)"
    fi
}

main "$@"
