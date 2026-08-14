#!/bin/bash
set -e

export RLINF_NODE_RANK=$((RLARK_NODE_RANK_START + ${POD_NAME##*-}))
echo "RLINF_NODE_RANK: $RLINF_NODE_RANK"

if [ -n "$WAIT_NETWORK_SCRIPT" ]; then
    bash "$WAIT_NETWORK_SCRIPT" "network"
fi

# Phase 0: Inject SSH public key into authorized_keys
if [ -n "$RLARK_SSH_PUBLIC_KEY" ]; then
    mkdir -p ~/.ssh && chmod 700 ~/.ssh
    echo "$RLARK_SSH_PUBLIC_KEY" >> ~/.ssh/authorized_keys
    chmod 600 ~/.ssh/authorized_keys
    echo "SSH public key injected into authorized_keys"
fi

# Phase 1: Prepare script (executed before Ray starts)
if [ -n "$RLARK_PREPARE_SCRIPT" ]; then
    echo "Executing prepare script..."
    eval "$RLARK_PREPARE_SCRIPT"
fi

# Phase 2: Start Ray head node
if ! command -v ray >/dev/null 2>&1; then
    echo "Error: Ray CLI is not found. Please ensure Ray is installed." >&2
    exit 1
fi

ray stop || true
TEMP_DIR=/tmp/ray
if [ -n "$RAY_TEMP_DIR" ]; then
    TEMP_DIR=$RAY_TEMP_DIR
fi

NODE_IP="${RLARK_NODE_IP:-$(hostname -I | awk '{print $1}')}"
ray start --head --dashboard-host=0.0.0.0 --disable-usage-stats \
    --node-ip-address="$NODE_IP" --port=${RLARK_RAY_PORT} --temp-dir $TEMP_DIR &
RAY_HEAD_PID=$!

cleanup() {
    local status=$?
    trap - EXIT INT TERM
    echo "Stopping Ray cluster..."
    ray stop --force >/dev/null 2>&1 || true
    if [ -n "$RAY_HEAD_PID" ]; then
        wait "$RAY_HEAD_PID" 2>/dev/null || true
    fi
    exit "$status"
}
trap cleanup EXIT INT TERM

# Phase 3: Wait for Ray cluster to be ready (including head node itself)
TOTAL_NODES="${RLARK_TOTAL_NODES:-1}"
echo "Waiting for $TOTAL_NODES nodes to join the Ray cluster..."
python -u /rlark/scripts/ray_check.py "$NODE_IP:${RLARK_RAY_PORT}" "$TOTAL_NODES" || {
    echo "Warning: Ray cluster check failed, proceeding anyway."
}

echo "Ray cluster is ready. Printing cluster information:"
echo "=========================================="
ray status
echo "=========================================="

# Phase 4: Run script (executed after Ray cluster is ready)
if [ -n "$RLARK_RUN_SCRIPT" ]; then
    echo "Executing run script..."
    eval "$RLARK_RUN_SCRIPT"
    exitCode=$?
else
    echo "No run script provided, keeping cluster alive."
    sleep inf
    exitCode=0
fi

exit $exitCode
