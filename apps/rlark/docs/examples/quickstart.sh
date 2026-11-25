#!/usr/bin/env bash
set -euo pipefail
set +H  # disable history expansion (warn/err use !)

# =============================================================================
# rlark Quick Start Deployment Script
# Deploys rlark control plane in Docker Compose and data plane agents in kind.
# =============================================================================

CLUSTER_COUNT="${CLUSTER_COUNT:-2}"
KIND_IMAGE="${KIND_IMAGE:-kindest/node:v1.31.0}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

log()  { echo -e "\033[1;34m[$(date +%H:%M:%S)]\033[0m $*"; }
ok()   { echo -e "\033[1;32m[$(date +%H:%M:%S)] ✓\033[0m $*"; }
warn() { echo -e "\033[1;33m[$(date +%H:%M:%S)] !\033[0m $*"; }
err()  { echo -e "\033[1;31m[$(date +%H:%M:%S)] ✗\033[0m $*"; exit 1; }

# Docker 29 dropped the legacy top-level NetworkSettings.IPAddress field, so
# "-f '{{.NetworkSettings.IPAddress}}'" aborts with a template parsing error.
# Read the address from the per-network map, which works on every version.
container_ip() {
  docker inspect "$1" \
    -f '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{.IPAddress}}{{end}}{{end}}' 2>/dev/null
}

# Image used throughout (built locally and pushed to registry)
IMAGE="localhost:5555/rlark:latest"

# =============================================================================
# Cleanup previous run
# =============================================================================
log "Cleaning up previous run (if any)..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" down -v 2>/dev/null || true
for i in $(seq 1 $CLUSTER_COUNT); do
  kind delete cluster --name "rlark-data-$i" 2>/dev/null || true
done
docker rm -f local-registry 2>/dev/null || true
rm -rf /tmp/rlark /tmp/kind-kubeconfig-* /tmp/kind-config.yaml /tmp/rlark-bin 2>/dev/null || true
ok "Cleanup complete"

# =============================================================================
# Step 0: Prerequisites
# =============================================================================
log "Step 0: Checking prerequisites..."
command -v docker  >/dev/null 2>&1 || err "docker is required"
command -v kind    >/dev/null 2>&1 || err "kind is required"
command -v kubectl >/dev/null 2>&1 || err "kubectl is required"
command -v jq      >/dev/null 2>&1 || err "jq is required"
command -v curl    >/dev/null 2>&1 || err "curl is required"
command -v python3 >/dev/null 2>&1 || err "python3 is required"
ok "All prerequisites satisfied"

# =============================================================================
# Step 1: Create runtime directories
# =============================================================================
log "Step 1: Creating runtime directories..."
mkdir -p /tmp/rlark
ok "Directories created: /tmp/rlark"

# =============================================================================
# Step 2: Start local registry
# =============================================================================
log "Step 2: Starting local Docker registry..."
docker rm -f local-registry 2>/dev/null || true
docker run -d --name local-registry --restart=always -p 5555:5000 registry:2
REGISTRY_IP=$(container_ip local-registry)
[ -n "$REGISTRY_IP" ] || err "Cannot determine the local registry IP address"
ok "Local registry: localhost:5555 (IP: $REGISTRY_IP)"

# =============================================================================
# Step 3: Build and push images
# =============================================================================
log "Step 3: Building and pushing Docker images..."

cd "$PROJECT_ROOT/apps/rlark"
mkdir -p /tmp/rlark-bin

# Pull busybox (try Docker Hub, then mirror, use local if available). It backs
# both the sample workloads and the rlark image base, so pull it before build.
if ! docker pull busybox:latest 2>/dev/null; then
  if ! docker pull docker.m.daocloud.io/library/busybox:latest 2>/dev/null; then
    warn "Cannot pull busybox, using local copy"
  else
    docker tag docker.m.daocloud.io/library/busybox:latest busybox:latest
    docker rmi docker.m.daocloud.io/library/busybox:latest 2>/dev/null
  fi
fi
docker image inspect busybox:latest >/dev/null 2>&1 || \
  err "busybox:latest is required as the rlark image base"
docker tag busybox:latest localhost:5555/busybox:latest 2>/dev/null || true
docker push localhost:5555/busybox:latest 2>/dev/null || warn "Cannot push busybox"

# Pull the Ray image used by the sample workloads. The Kubernetes runtime only
# supports the "rlinf.io/ray" workload and replaces the main container command
# with "rlark-tools ray-launcher", so a workload image must ship the Ray CLI,
# bash and python. These tags are single-architecture, so pick by host arch.
RAY_VERSION="${RAY_VERSION:-2.9.0}"
case "$(uname -m)" in
  arm64|aarch64) RAY_TAG="${RAY_VERSION}-py310-aarch64" ;;
  *)             RAY_TAG="${RAY_VERSION}-py310" ;;
esac
if ! docker image inspect "rayproject/ray:$RAY_TAG" >/dev/null 2>&1; then
  log "Pulling rayproject/ray:$RAY_TAG (large image, this takes a while)..."
  if ! docker pull "rayproject/ray:$RAY_TAG" 2>/dev/null; then
    docker pull "docker.m.daocloud.io/rayproject/ray:$RAY_TAG" || \
      err "Cannot pull rayproject/ray:$RAY_TAG"
    docker tag "docker.m.daocloud.io/rayproject/ray:$RAY_TAG" "rayproject/ray:$RAY_TAG"
    docker rmi "docker.m.daocloud.io/rayproject/ray:$RAY_TAG" 2>/dev/null
  fi
fi
docker tag "rayproject/ray:$RAY_TAG" localhost:5555/ray:quickstart
docker push localhost:5555/ray:quickstart || \
  err "Cannot push the Ray workload image to the local registry"

# Build all binaries in parallel
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/server ./cmd/server/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/agent ./cmd/agent/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/controller-manager ./cmd/controller-manager/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/gateway ./cmd/gateway/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/network-sidecar ./cmd/network-sidecar/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/rlark-tools ./cmd/rlark-tools/ &
wait

# busybox, not scratch: the Kubernetes Runtime injects an rlark-tools-init
# init container that runs "cp /usr/local/bin/rlark-tools" from this image, so
# the image must ship rlark-tools and a working cp.
cat > /tmp/rlark-bin/Dockerfile <<'DOCKERFILE'
FROM busybox:latest
COPY server /rlark-server
COPY agent /rlark-agent
COPY controller-manager /rlark-controller-manager
COPY gateway /rlark-gateway
COPY network-sidecar /usr/local/bin/network-sidecar
COPY rlark-tools /usr/local/bin/rlark-tools
DOCKERFILE

docker build -t "$IMAGE" /tmp/rlark-bin
docker push "$IMAGE"

ok "Images pushed: $IMAGE, localhost:5555/busybox:latest, localhost:5555/ray:quickstart"

# =============================================================================
# Step 4: Ensure kind node image
# =============================================================================
log "Step 4: Ensuring kind node image is available..."
if docker image inspect "$KIND_IMAGE" &>/dev/null; then
  ok "Kind node image already present: $KIND_IMAGE"
elif docker pull "$KIND_IMAGE" 2>/dev/null; then
  ok "Pulled $KIND_IMAGE from Docker Hub"
elif docker pull "docker.m.daocloud.io/$KIND_IMAGE" 2>/dev/null; then
  docker tag "docker.m.daocloud.io/$KIND_IMAGE" "$KIND_IMAGE"
  docker rmi "docker.m.daocloud.io/$KIND_IMAGE" 2>/dev/null
  ok "Pulled $KIND_IMAGE via mirror"
else
  err "Cannot pull $KIND_IMAGE. Please manually pull: docker pull $KIND_IMAGE"
fi

# =============================================================================
# Step 5: Start kcp and PostgreSQL
# =============================================================================
log "Step 5: Starting kcp and PostgreSQL..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d kcp postgresql
log "Waiting for kcp to be healthy..."
for i in $(seq 1 60); do
  if docker inspect kcp --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy; then
    break
  fi
  sleep 2
done
docker inspect kcp --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy || err "kcp failed to start"
ok "kcp and PostgreSQL are running"

# =============================================================================
# Step 6: Configure kubeconfig
# =============================================================================
log "Step 6: Configuring kubeconfig..."

docker cp kcp:/.kcp/admin.kubeconfig /tmp/rlark/kcp-raw.kubeconfig

# Docker-internal kubeconfig (shard-admin + root cluster for full permissions)
python3 -c "
import yaml
with open('/tmp/rlark/kcp-raw.kubeconfig') as f:
    cfg = yaml.safe_load(f)
new_clusters, new_users = [], []
for cluster in cfg.get('clusters', []):
    if cluster['name'] == 'root':
        cluster['cluster']['server'] = 'https://kcp:6443/clusters/root'
        cluster['cluster']['insecure-skip-tls-verify'] = True
        cluster['cluster'].pop('certificate-authority-data', None)
        new_clusters.append(cluster)
for user in cfg.get('users', []):
    if user['name'] == 'shard-admin':
        new_users.append(user)
cfg['clusters'] = new_clusters
cfg['users'] = new_users
cfg['contexts'] = [{'context': {'cluster': 'root', 'user': 'shard-admin'}, 'name': 'root'}]
cfg['current-context'] = 'root'
with open('/tmp/rlark/kcp-kubeconfig.yaml', 'w') as f:
    yaml.dump(cfg, f)
"

# User-facing kubeconfig (localhost)
python3 -c "
import yaml
with open('/tmp/rlark/kcp-raw.kubeconfig') as f:
    cfg = yaml.safe_load(f)
for cluster in cfg.get('clusters', []):
    if cluster['name'] == 'root':
        cluster['cluster']['server'] = 'https://localhost:6443/clusters/root'
    elif cluster['name'] == 'system:admin':
        cluster['cluster']['server'] = 'https://localhost:6443/clusters/system:admin'
    elif cluster['name'] == 'base':
        cluster['cluster']['server'] = 'https://localhost:6443'
    cluster['cluster']['insecure-skip-tls-verify'] = True
    cluster['cluster'].pop('certificate-authority-data', None)
# Add root-shard context for CRD install (shard-admin has elevated permissions)
cfg['contexts'].append({
    'context': {'cluster': 'root', 'user': 'shard-admin'},
    'name': 'root-shard'
})
with open('/tmp/rlark/admin.kubeconfig', 'w') as f:
    yaml.dump(cfg, f)
"

# DB config
cat > /tmp/rlark/db-config.yaml << 'EOF'
host: postgresql
port: 5432
user: postgres
password: postgres
database: rlark
sslmode: disable
EOF

# Install CRDs: regenerate with maxDescLen=0 to fit kcp's 256KB annotation limit,
# then install from /tmp to avoid modifying committed CRD files.
# Use the pinned generator from api/bin instead of whatever sits in GOPATH/bin,
# so these CRDs match the committed manifests.
log "Installing pinned controller-gen..."
make -C "$PROJECT_ROOT/api" controller-gen
CONTROLLER_GEN="$(make -s -C "$PROJECT_ROOT/api" print-controller-gen)"
log "Regenerating CRDs for kcp (maxDescLen=0)..."
"$CONTROLLER_GEN" crd:maxDescLen=0,allowDangerousTypes=true \
  paths="$PROJECT_ROOT/api/rlark.io/..." \
  output:crd:artifacts:config=/tmp/rlark/crds
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  apply -f /tmp/rlark/crds/ --validate=false
# The controller manager builds field indexes for these kinds at startup and
# exits if a CRD is not yet served, so block until the API server accepts them.
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  wait --for=condition=Established --timeout=120s \
  -f /tmp/rlark/crds/

# Local quickstart credentials. Production installs use rlarkadm-generated
# random credentials and must not reuse these ephemeral values.
ADMIN_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(8))')
USER_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(8))')
# The Gateway refuses to start unless jwt-signing-key holds at least 32 bytes.
JWT_SIGNING_KEY=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  create namespace default --dry-run=client -o yaml 2>/dev/null | \
  kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard apply --validate=false -f - 2>/dev/null || true
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  delete secret rlark-ui-auth -n default --ignore-not-found 2>/dev/null || true
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  create secret generic rlark-ui-auth -n default \
  --from-literal="admin-password=$ADMIN_PASSWORD" \
  --from-literal="user-password=$USER_PASSWORD" \
  --from-literal="jwt-signing-key=$JWT_SIGNING_KEY" \
  --validate=false
ok "kubeconfig, DB config, CRDs, and UI credentials ready"

# =============================================================================
# Step 7: Start control plane
# =============================================================================
log "Step 7: Starting control plane..."

KCP_KUBECONFIG=/tmp/rlark/kcp-kubeconfig.yaml \
DB_CONFIG=/tmp/rlark/db-config.yaml \
IMAGE="$IMAGE" \
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d --force-recreate rlark-server

log "Waiting for Server to initialize Gateway certificates..."
CERTS_READY=false
for i in $(seq 1 60); do
  ADMIN_CERT=$(kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
    get secret rlark-admin-cert -n default \
    -o jsonpath='{.data.client\.crt}' 2>/dev/null || true)
  ADMIN_KEY=$(kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
    get secret rlark-admin-cert -n default \
    -o jsonpath='{.data.client\.key}' 2>/dev/null || true)
  TLS_CA=$(kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
    get secret rlark-tls-ca -n default \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || true)
  if [ -n "$ADMIN_CERT" ] && [ -n "$ADMIN_KEY" ] && [ -n "$TLS_CA" ]; then
    CERTS_READY=true
    break
  fi
  sleep 2
done
$CERTS_READY || err "Server did not initialize Gateway certificates"

KCP_KUBECONFIG=/tmp/rlark/kcp-kubeconfig.yaml \
DB_CONFIG=/tmp/rlark/db-config.yaml \
IMAGE="$IMAGE" \
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d --no-deps --force-recreate rlark-gateway rlark-controller-manager

log "Waiting for Gateway..."
# Every /api/v1 route except the login endpoint requires a JWT, so an anonymous
# probe always returns 401. Log in instead: a successful login proves the
# Gateway is serving and yields the token used by the rest of this script.
RLARK_TOKEN=""
for i in $(seq 1 30); do
  RLARK_TOKEN=$(jq -n --arg password "$ADMIN_PASSWORD" '{username:"admin",password:$password}' | \
    curl -fsS --connect-timeout 5 --max-time 15 \
      -H 'Content-Type: application/json' --data-binary @- \
      "http://localhost:9000/api/v1/auth/login" 2>/dev/null | \
    jq -er 'select(.role == "admin") | .token | select(type == "string" and length > 0)' 2>/dev/null) || RLARK_TOKEN=""
  [ -n "$RLARK_TOKEN" ] && break
  sleep 2
done
[ -n "$RLARK_TOKEN" ] || err "Gateway failed to become ready (login to http://localhost:9000 did not succeed; check 'docker logs rlark-gateway')"

gateway_curl() {
  curl -fsS --connect-timeout 5 --max-time 30 \
    -H "Authorization: Bearer $RLARK_TOKEN" "$@"
}

ok "Control plane is running"

# =============================================================================
# Step 8: Create kind clusters
# =============================================================================
log "Step 8: Creating kind clusters..."

cat > /tmp/kind-config.yaml << KEOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5555"]
    endpoint = ["http://${REGISTRY_IP}:5000"]
KEOF

COMPOSE_NETWORK=$(docker inspect kcp -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')

# Create kind clusters in parallel
for i in $(seq 1 $CLUSTER_COUNT); do
  CLUSTER_NAME="rlark-data-$i"
  if kind get clusters 2>/dev/null | grep -q "$CLUSTER_NAME"; then
    warn "Cluster '$CLUSTER_NAME' already exists"
  else
    kind create cluster --name "$CLUSTER_NAME" --image "$KIND_IMAGE" --config /tmp/kind-config.yaml &
  fi
done
wait

# Extract kubeconfigs and connect networks
for i in $(seq 1 $CLUSTER_COUNT); do
  CLUSTER_NAME="rlark-data-$i"
  kind get kubeconfig --name "$CLUSTER_NAME" > "/tmp/kind-kubeconfig-$i"
  docker network connect "$COMPOSE_NETWORK" "${CLUSTER_NAME}-control-plane" 2>/dev/null || true
done

# Connect registry to kind network so nodes can pull images
docker network connect kind local-registry 2>/dev/null || true
# Also connect kind nodes to bridge network (registry may be on bridge)
docker network connect bridge rlark-data-1-control-plane 2>/dev/null || true
docker network connect bridge rlark-data-2-control-plane 2>/dev/null || true

ok "All $CLUSTER_COUNT kind cluster(s) ready"

# =============================================================================
# Step 9: Deploy Agents
# =============================================================================
log "Step 9: Deploying Agents..."

# Request agent certificates sequentially (to avoid gateway race conditions)
for i in $(seq 1 $CLUSTER_COUNT); do
  CID="agent-my-cluster-$i"
  jq -n --arg cluster_id "$CID" '{cluster_id:$cluster_id}' | \
    gateway_curl -X POST "http://localhost:9000/api/v1/certificates/agent" \
      -H "Content-Type: application/json" \
      --data-binary @- > "/tmp/rlark/agent-cert-$i.json" || \
    err "Agent certificate request failed for $CID"
  jq -e 'all(.ca_cert, .agent_cert, .agent_key; type == "string" and length > 0)' \
    "/tmp/rlark/agent-cert-$i.json" >/dev/null || err "Incomplete agent certificate response for $CID"
  jq -r .ca_cert "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/ca-$i.pem"
  jq -r .agent_cert "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/cert-$i.pem"
  jq -r .agent_key "/tmp/rlark/agent-cert-$i.json" > "/tmp/rlark/key-$i.pem"
done

# Deploy agents in parallel
deploy_agent() {
  local i=$1
  local KC="/tmp/kind-kubeconfig-$i"
  local CID="agent-my-cluster-$i"

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

  NODE_NAME=$(kubectl --kubeconfig "$KC" get nodes -o jsonpath='{.items[0].metadata.name}')
  # The Agent only reports Nodes opted in with rlark.io/enabled=true, and the
  # node Agent DaemonSet selects the same label, so apply it before waiting.
  kubectl --kubeconfig "$KC" label node "$NODE_NAME" \
    "rlark.io/enabled=true" "rlark.io/cluster-id=rlark-${CID}" --overwrite || \
    err "Failed to label node $NODE_NAME in cluster $i"

  kubectl --kubeconfig "$KC" rollout status \
    deployment/rlark-agent -n rlark-system --timeout=120s >/dev/null || \
    err "Agent $i deployment did not become ready"
  # The node Agent owns the node-heartbeat Lease; Nodes stay Offline without it.
  kubectl --kubeconfig "$KC" rollout status \
    daemonset/rlark-agent-node -n rlark-system --timeout=180s >/dev/null || \
    err "Node agent $i daemonset did not become ready"

  echo -e "\033[1;32m[$(date +%H:%M:%S)] ✓\033[0m Agent $i deployed"
}
export -f deploy_agent err
export SCRIPT_DIR IMAGE

for i in $(seq 1 $CLUSTER_COUNT); do
  deploy_agent "$i" &
done
wait

# =============================================================================
# Step 10: Verify nodes
# =============================================================================
log "Step 10: Verifying node registration..."
for attempt in $(seq 1 30); do
  REGISTERED=0
  for i in $(seq 1 $CLUSTER_COUNT); do
    CID="rlark-agent-my-cluster-$i"
    if gateway_curl "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" | \
      jq -e --arg cid "$CID" '.items[] | select(.metadata.labels["rlark.io/cluster-id"] == $cid)' >/dev/null; then
      REGISTERED=$((REGISTERED + 1))
    fi
  done
  [ "$REGISTERED" -eq "$CLUSTER_COUNT" ] && break
  sleep 2
done
[ "${REGISTERED:-0}" -eq "$CLUSTER_COUNT" ] || err "Only ${REGISTERED:-0}/$CLUSTER_COUNT expected nodes registered"
gateway_curl "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" | \
  jq -r '.items[] | "  \(.metadata.name)  cluster-id=\(.metadata.labels["rlark.io/cluster-id"])"'
ok "All $CLUSTER_COUNT nodes verified"

# =============================================================================
# Step 11: Create workspace, domain, and cross-cluster test Job
# =============================================================================
if [ "$CLUSTER_COUNT" -ge 2 ]; then
  log "Step 11: Creating cross-cluster test resources..."

  # Create workspace for each agent
  for i in $(seq 1 $CLUSTER_COUNT); do
    kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root create -f - <<EOF
apiVersion: tenancy.kcp.io/v1alpha1
kind: Workspace
metadata:
  name: rlark-agent-my-cluster-$i
EOF
  done
  sleep 5

  # Create namespace for each agent in root workspace (agent's cache namespace)
  for i in $(seq 1 $CLUSTER_COUNT); do
    kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root \
      create namespace rlark-agent-my-cluster-$i 2>/dev/null || true
  done

  # Create Domain
  kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root apply -f "$SCRIPT_DIR/domain.yaml"

  # Create Jobs (controller-manager will create Tasks from them)
  kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root apply -f "$SCRIPT_DIR/cross-cluster-ping.yaml"

  # Wait for controller-manager to create one Task in each agent namespace.
  log "Waiting for controller-manager to create Tasks..."
  for attempt in $(seq 1 30); do
    SERVER_TASK=$(kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root \
      get task cross-cluster-server-app -n rlark-agent-my-cluster-1 --ignore-not-found -o name 2>/dev/null)
    CLIENT_TASK=$(kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root \
      get task cross-cluster-client-app -n rlark-agent-my-cluster-2 --ignore-not-found -o name 2>/dev/null)
    [ -n "$SERVER_TASK" ] && [ -n "$CLIENT_TASK" ] && break
    sleep 2
  done
  [ -n "${SERVER_TASK:-}" ] && [ -n "${CLIENT_TASK:-}" ] || \
    err "Controller-manager did not create Tasks in the agent namespaces"

  log "Waiting for Task pods..."
  # The Agent creates the Deployment shortly after the Task appears, and
  # "rollout status" fails immediately on a missing object, so wait for it.
  wait_for_workload() {
    local kubeconfig=$1 name=$2
    for _ in $(seq 1 60); do
      if [ -n "$(kubectl --kubeconfig "$kubeconfig" get deployment "$name" \
        -n rlark-system --ignore-not-found -o name 2>/dev/null)" ]; then
        return 0
      fi
      sleep 2
    done
    return 1
  }
  wait_for_workload /tmp/kind-kubeconfig-1 cross-cluster-server-app || \
    err "Agent did not create the server Deployment"
  wait_for_workload /tmp/kind-kubeconfig-2 cross-cluster-client-app || \
    err "Agent did not create the client Deployment"
  # The Ray image is large and the Ray head start plus readiness check adds
  # startup time on top of the image pull, so allow a generous timeout.
  kubectl --kubeconfig /tmp/kind-kubeconfig-1 rollout status \
    deployment/cross-cluster-server-app -n rlark-system --timeout=600s >/dev/null || \
    err "Server workload did not become ready"
  kubectl --kubeconfig /tmp/kind-kubeconfig-2 rollout status \
    deployment/cross-cluster-client-app -n rlark-system --timeout=600s >/dev/null || \
    err "Client workload did not become ready"

  ok "Cross-cluster test workloads are ready"

  # =============================================================================
  # Step 12: Verify cross-cluster network connectivity
  # =============================================================================
  log "Step 12: Verifying cross-cluster network connectivity..."

  SERVER_POD=$(kubectl --kubeconfig /tmp/kind-kubeconfig-1 get pods -n rlark-system \
    -l app=cross-cluster-server-app -o jsonpath='{.items[0].metadata.name}')
  log "Server pod: $SERVER_POD"

  # The Ray head runs the HTTP server only after its readiness check, and the
  # sidecar syncs peer names into /etc/hosts periodically, so retry.
  RESULT=""
  for _ in $(seq 1 30); do
    RESULT=$(kubectl --kubeconfig /tmp/kind-kubeconfig-2 exec -n rlark-system \
      deploy/cross-cluster-client-app -c main -- \
      python -c "import urllib.request; print(urllib.request.urlopen('http://${SERVER_POD}.rlark-domain:8000/', timeout=5).read().decode())" 2>&1) || true
    echo "$RESULT" | grep -q "hello from server" && break
    sleep 5
  done

  echo "--- Cross-cluster test result ---"
  echo "$RESULT"

  if echo "$RESULT" | grep -q "hello from server"; then
    ok "Cross-cluster network connectivity verified!"
  else
    err "Cross-cluster connectivity test failed: unexpected response"
  fi
fi

# =============================================================================
# Final: Print summary
# =============================================================================
echo ""
echo "=============================================="
ok "Deployment complete!"
echo ""
echo "Architecture:"
echo "  Docker Compose:"
echo "    ├── local-registry :5555"
echo "    ├── kcp            :6443"
echo "    ├── postgresql     :5432"
echo "    ├── rlark-server   :8443 + :2222"
echo "    ├── rlark-gateway  :9000"
echo "    └── rlark-controller-manager"
for i in $(seq 1 $CLUSTER_COUNT); do
echo "  kind (rlark-data-$i):"
echo "    └── rlark-agent"
done
echo ""
echo "Web UI credentials (local quickstart only):"
echo "  admin / $ADMIN_PASSWORD"
echo "  user  / $USER_PASSWORD"
echo ""
echo "Kubeconfigs:"
echo "  kcp:  /tmp/rlark/admin.kubeconfig"
for i in $(seq 1 $CLUSTER_COUNT); do
echo "  cluster-$i: /tmp/kind-kubeconfig-$i"
done
echo ""
echo "Cleanup:"
echo "  docker compose -f $SCRIPT_DIR/docker-compose.yml down"
for i in $(seq 1 $CLUSTER_COUNT); do
echo "  kind delete cluster --name rlark-data-$i"
done
echo "  docker rm -f local-registry"
