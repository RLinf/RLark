#!/usr/bin/env bash
set -euo pipefail
set +H

# =============================================================================
# rlark UI-based Quick Start — Data Plane
# Deploys a kind cluster with an rlark agent, connecting it to the control
# plane. Requires a running control plane (see quickstart-cp.sh).
#
# Usage:
#   bash quickstart-dp.sh --cluster-id <id> [--cluster-name <name>]
#   bash quickstart-dp.sh --cluster-id <id> --cluster-id <id2>  # deploy 2
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

KIND_IMAGE="${KIND_IMAGE:-kindest/node:v1.31.0}"
IMAGE="localhost:5555/rlark:latest"
CLUSTER_IDS=()
CLUSTER_NAME_PREFIX="rlark-data"

log()  { echo -e "\033[1;34m[$(date +%H:%M:%S)]\033[0m $*"; }
ok()   { echo -e "\033[1;32m[$(date +%H:%M:%S)] ✓\033[0m $*"; }
warn() { echo -e "\033[1;33m[$(date +%H:%M:%S)] !\033[0m $*"; }
err()  { echo -e "\033[1;31m[$(date +%H:%M:%S)] ✗\033[0m $*"; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-id) CLUSTER_IDS+=("$2"); shift 2 ;;
    --cluster-name) CLUSTER_NAME_PREFIX="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; echo "Usage: $0 --cluster-id <id> [--cluster-id <id2> ...] [--cluster-name <prefix>]"; exit 1 ;;
  esac
done

if [ ${#CLUSTER_IDS[@]} -eq 0 ]; then
  err "Required: --cluster-id <id>"
fi

CLUSTER_COUNT=${#CLUSTER_IDS[@]}

# =============================================================================
# Pre-flight checks
# =============================================================================
log "Checking prerequisites..."
command -v docker  >/dev/null 2>&1 || err "docker is required"
command -v kind    >/dev/null 2>&1 || err "kind is required"
command -v kubectl >/dev/null 2>&1 || err "kubectl is required"
command -v jq      >/dev/null 2>&1 || err "jq is required"

# Verify control plane is running
if [ "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes 2>/dev/null)" != "200" ]; then
  err "Control plane not reachable at http://localhost:9000. Run quickstart-cp.sh first."
fi
ok "Control plane is reachable"

# Get registry IP
REGISTRY_IP=$(docker inspect local-registry -f '{{.NetworkSettings.IPAddress}}' 2>/dev/null || echo "")
if [ -z "$REGISTRY_IP" ]; then
  warn "Local registry not found, attempting to start..."
  docker run -d --name local-registry --restart=always -p 5555:5000 registry:2
  REGISTRY_IP=$(docker inspect local-registry -f '{{.NetworkSettings.IPAddress}}')
fi
ok "Local registry: localhost:5555 (IP: $REGISTRY_IP)"

# =============================================================================
# Step 1: Ensure kind node image
# =============================================================================
log "Step 1: Ensuring kind node image is available..."
if docker image inspect "$KIND_IMAGE" &>/dev/null; then
  ok "Kind node image already present: $KIND_IMAGE"
elif docker pull "$KIND_IMAGE" 2>/dev/null; then
  ok "Pulled $KIND_IMAGE from Docker Hub"
elif docker pull "docker.m.daocloud.io/$KIND_IMAGE" 2>/dev/null; then
  docker tag "docker.m.daocloud.io/$KIND_IMAGE" "$KIND_IMAGE"
  docker rmi "docker.m.daocloud.io/$KIND_IMAGE" 2>/dev/null
  ok "Pulled $KIND_IMAGE via mirror"
else
  err "Cannot pull $KIND_IMAGE"
fi

# =============================================================================
# Step 2: Create kind clusters
# =============================================================================
log "Step 2: Creating kind cluster(s)..."

cat > /tmp/kind-config.yaml << KEOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5555"]
    endpoint = ["http://${REGISTRY_IP}:5000"]
KEOF

COMPOSE_NETWORK=$(docker inspect kcp -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' 2>/dev/null || echo "rlark_default")

for i in $(seq 1 $CLUSTER_COUNT); do
  CLUSTER_NAME="${CLUSTER_NAME_PREFIX}-$i"
  if kind get clusters 2>/dev/null | grep -q "$CLUSTER_NAME"; then
    warn "Cluster '$CLUSTER_NAME' already exists, deleting..."
    kind delete cluster --name "$CLUSTER_NAME"
  fi
  log "Creating kind cluster: $CLUSTER_NAME"
  kind create cluster --name "$CLUSTER_NAME" --image "$KIND_IMAGE" --config /tmp/kind-config.yaml
done

# Extract kubeconfigs and connect networks
for i in $(seq 1 $CLUSTER_COUNT); do
  CLUSTER_NAME="${CLUSTER_NAME_PREFIX}-$i"
  kind get kubeconfig --name "$CLUSTER_NAME" > "/tmp/kind-kubeconfig-$i"
  docker network connect "$COMPOSE_NETWORK" "${CLUSTER_NAME}-control-plane" 2>/dev/null || true
done

# Connect registry to kind network
docker network connect kind local-registry 2>/dev/null || true
for i in $(seq 1 $CLUSTER_COUNT); do
  CLUSTER_NAME="${CLUSTER_NAME_PREFIX}-$i"
  docker network connect bridge "${CLUSTER_NAME}-control-plane" 2>/dev/null || true
done

ok "All $CLUSTER_COUNT kind cluster(s) ready"

# =============================================================================
# Step 3: Request agent certificates
# =============================================================================
log "Step 3: Requesting agent certificates..."

for i in $(seq 1 $CLUSTER_COUNT); do
  CID="${CLUSTER_IDS[$((i-1))]}"
  log "Requesting cert for cluster-id: $CID"
  curl -s -X POST "http://localhost:9000/api/v1/certificates/agent" \
    -H "Content-Type: application/json" \
    -d "{\"cluster_id\":\"${CID}\"}" > "/tmp/rlark/agent-cert-$i.json"
  jq -r .ca_cert "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/ca-$i.pem"
  jq -r .agent_cert "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/cert-$i.pem"
  jq -r .agent_key "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/key-$i.pem"
  ok "Certificates for $CID saved"
done

# =============================================================================
# Step 4: Deploy Agents
# =============================================================================
log "Step 4: Deploying Agents..."

deploy_agent() {
  local i=$1
  local KC="/tmp/kind-kubeconfig-$i"
  local CID="${CLUSTER_IDS[$((i-1))]}"

  kubectl --kubeconfig "$KC" create namespace rlark-system 2>/dev/null || true
  kubectl --kubeconfig "$KC" create configmap kcp-kubeconfig \
    -n rlark-system --from-file=kubeconfig.yaml=/tmp/rlark/kcp-kubeconfig.yaml \
    --dry-run=client -o yaml | kubectl --kubeconfig "$KC" apply -f -

  kubectl --kubeconfig "$KC" create secret generic agent-certs -n rlark-system \
    --from-file=ca-cert.pem="/tmp/rlark/ca-$i.pem" \
    --from-file=cert.pem="/tmp/rlark/cert-$i.pem" \
    --from-file=key.pem="/tmp/rlark/key-$i.pem" \
    --dry-run=client -o yaml | kubectl --kubeconfig "$KC" apply -f -

  kubectl --kubeconfig "$KC" apply -f "$SCRIPT_DIR/agent-rbac.yaml"
  sed "s|\${IMAGE}|$IMAGE|g" "$SCRIPT_DIR/agent-deploy.yaml" | kubectl --kubeconfig "$KC" apply -f -

  log "  Waiting for Agent deployment to become ready..."
  kubectl --kubeconfig "$KC" rollout status \
    deployment/rlark-agent -n rlark-system --timeout=120s >/dev/null || \
    err "Agent $i deployment did not become ready"
  ok "  Agent $i pod ready"

  NODE_NAME=$(kubectl --kubeconfig "$KC" get nodes -o jsonpath='{.items[0].metadata.name}')
  kubectl --kubeconfig "$KC" label node "$NODE_NAME" "rlark.io/cluster-id=${CID}" --overwrite 2>/dev/null || true
  # Remove control-plane label so the platform UI recognizes the node as a worker.
  # Safe on kind: the taint is already removed by kind, and no system component
  # uses this label as a node selector. Only affects the ROLES column in kubectl.
  kubectl --kubeconfig "$KC" label node "$NODE_NAME" "node-role.kubernetes.io/control-plane-" 2>/dev/null || true
  kubectl --kubeconfig "$KC" taint node "$NODE_NAME" node-role.kubernetes.io/control-plane:NoSchedule- 2>/dev/null || true
  # Set node-category on the management plane (the push controller treats this label as management-owned)
  if [ -f /tmp/rlark/admin.kubeconfig ]; then
    kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
      -n "rlark-${CID}" label node "$NODE_NAME" "rlark.io/node-category=cloud" --overwrite 2>/dev/null || true
  fi

  echo -e "\033[1;32m[$(date +%H:%M:%S)] ✓\033[0m Agent $i deployed (cluster-id: $CID)"
}
export -f deploy_agent log ok warn err
export SCRIPT_DIR IMAGE CLUSTER_IDS

for i in $(seq 1 $CLUSTER_COUNT); do
  deploy_agent "$i"
done

# =============================================================================
# Step 5: Verify nodes
# =============================================================================
log "Step 5: Verifying node registration (waiting for agents to report)..."
for attempt in $(seq 1 30); do
  REGISTERED=0
  for CID in "${CLUSTER_IDS[@]}"; do
    if curl -fsS "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" | \
      jq -e --arg cid "rlark-${CID}" '.items[] | select(.metadata.labels["rlark.io/cluster-id"] == $cid)' >/dev/null; then
      REGISTERED=$((REGISTERED + 1))
    fi
  done
  [ "$REGISTERED" -eq "$CLUSTER_COUNT" ] && break
  sleep 2
done
[ "${REGISTERED:-0}" -eq "$CLUSTER_COUNT" ] || err "Only ${REGISTERED:-0}/$CLUSTER_COUNT expected nodes registered"
curl -fsS "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" | \
  jq -r '.items[] | "  \(.metadata.name)  cluster-id=\(.metadata.labels["rlark.io/cluster-id"])"'
ok "All $CLUSTER_COUNT nodes verified"

# =============================================================================
# Final: Print summary
# =============================================================================
echo ""
echo "=============================================="
ok "Data plane deployment complete!"
echo ""
echo "Clusters:"
for i in $(seq 1 $CLUSTER_COUNT); do
  echo "  ${CLUSTER_NAME_PREFIX}-$i  (cluster-id: ${CLUSTER_IDS[$((i-1))]})"
  echo "    kubeconfig: /tmp/kind-kubeconfig-$i"
done
echo ""
echo "Next:"
echo "  1. Open http://localhost:5173 and sign in"
echo "  2. Go to Jobs → Create Job to submit training tasks"
if [ "$CLUSTER_COUNT" -ge 2 ]; then
echo ""
echo "Cross-cluster networking:"
echo "  3. Go to Admin → Domains → Create Domain"
echo "  4. Create a Job with tasks on different clusters"
echo "  5. Select the domain in the Common Config step"
fi
echo ""
echo "Cleanup:"
for i in $(seq 1 $CLUSTER_COUNT); do
  echo "  kind delete cluster --name ${CLUSTER_NAME_PREFIX}-$i"
done