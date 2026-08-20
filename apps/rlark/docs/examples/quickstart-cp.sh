#!/usr/bin/env bash
set -euo pipefail
set +H  # disable history expansion (warn/err use !)

# =============================================================================
# rlark UI-based Quick Start — Control Plane
# Starts the control plane (kcp, postgresql, rlark-server, rlark-gateway,
# rlark-controller-manager) and the UI dev server.
#
# Usage:
#   bash quickstart-cp.sh              # start control plane + UI
#   bash quickstart-cp.sh --no-ui      # start control plane only
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
START_UI=true

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-ui) START_UI=false; shift ;;
    *) echo "Usage: $0 [--no-ui]"; exit 1 ;;
  esac
done

log()  { echo -e "\033[1;34m[$(date +%H:%M:%S)]\033[0m $*"; }
ok()   { echo -e "\033[1;32m[$(date +%H:%M:%S)] ✓\033[0m $*"; }
warn() { echo -e "\033[1;33m[$(date +%H:%M:%S)] !\033[0m $*"; }
err()  { echo -e "\033[1;31m[$(date +%H:%M:%S)] ✗\033[0m $*"; exit 1; }

IMAGE="localhost:5555/rlark:latest"

# =============================================================================
# Cleanup previous run
# =============================================================================
log "Cleaning up previous run (if any)..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" down -v 2>/dev/null || true
docker rm -f local-registry 2>/dev/null || true
rm -rf /tmp/rlark /tmp/kind-kubeconfig-* /tmp/kind-config.yaml /tmp/Dockerfile.rlark /tmp/rlark-bin 2>/dev/null || true
ok "Cleanup complete"

# =============================================================================
# Step 0: Prerequisites
# =============================================================================
log "Step 0: Checking prerequisites..."
command -v docker  >/dev/null 2>&1 || err "docker is required"
command -v kubectl >/dev/null 2>&1 || err "kubectl is required"
command -v jq      >/dev/null 2>&1 || err "jq is required"
command -v python3 >/dev/null 2>&1 || err "python3 is required"
if $START_UI; then
  command -v node >/dev/null 2>&1 || err "node is required (install Node.js or use --no-ui)"
fi
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
REGISTRY_IP=$(docker inspect local-registry -f '{{.NetworkSettings.IPAddress}}')
ok "Local registry: localhost:5555 (IP: $REGISTRY_IP)"

# =============================================================================
# Step 3: Build and push images
# =============================================================================
log "Step 3: Building 5 Go binaries (server, agent, controller-manager, gateway, network-sidecar)..."
cd "$PROJECT_ROOT/apps/rlark"
mkdir -p /tmp/rlark-bin

GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/server ./cmd/server/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/agent ./cmd/agent/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/controller-manager ./cmd/controller-manager/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/gateway ./cmd/gateway/ &
GOOS=linux CGO_ENABLED=0 go build -o /tmp/rlark-bin/network-sidecar ./cmd/network-sidecar/ &
wait
log "  server:               $(du -h /tmp/rlark-bin/server               | cut -f1)"
log "  agent:                $(du -h /tmp/rlark-bin/agent                | cut -f1)"
log "  controller-manager:   $(du -h /tmp/rlark-bin/controller-manager   | cut -f1)"
log "  gateway:              $(du -h /tmp/rlark-bin/gateway              | cut -f1)"
log "  network-sidecar:      $(du -h /tmp/rlark-bin/network-sidecar      | cut -f1)"

cat > /tmp/Dockerfile.rlark <<'DOCKERFILE'
FROM scratch
COPY server /rlark-server
COPY agent /rlark-agent
COPY controller-manager /rlark-controller-manager
COPY gateway /rlark-gateway
COPY network-sidecar /usr/local/bin/network-sidecar
DOCKERFILE

docker build -t "$IMAGE" -f /tmp/Dockerfile.rlark /tmp/rlark-bin
log "  Docker image built: $IMAGE"
docker push "$IMAGE"
log "  Docker image pushed to local registry"

# Pull busybox
if ! docker pull busybox:latest 2>/dev/null; then
  if ! docker pull docker.m.daocloud.io/library/busybox:latest 2>/dev/null; then
    warn "Cannot pull busybox, using local copy"
  else
    docker tag docker.m.daocloud.io/library/busybox:latest busybox:latest
    docker rmi docker.m.daocloud.io/library/busybox:latest 2>/dev/null
  fi
fi
docker tag busybox:latest localhost:5555/busybox:latest 2>/dev/null || true
docker push localhost:5555/busybox:latest 2>/dev/null || warn "Cannot push busybox"

ok "Images pushed: $IMAGE, localhost:5555/busybox:latest"

# =============================================================================
# Step 4: Start kcp and PostgreSQL
# =============================================================================
log "Step 4: Starting kcp and PostgreSQL..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d kcp postgresql
log "  kcp and PostgreSQL containers started"
log "Waiting for kcp to be healthy (may take up to 2 min)..."
for i in $(seq 1 60); do
  STATUS=$(docker inspect kcp --format='{{.State.Health.Status}}' 2>/dev/null || echo "starting")
  if [ "$STATUS" = "healthy" ]; then
    log "  kcp healthy after $((i*2))s"
    break
  fi
  log "  kcp status: $STATUS (attempt $i/60, waiting...)"
  sleep 2
done
docker inspect kcp --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy || err "kcp failed to start"
ok "kcp and PostgreSQL are running"

# =============================================================================
# Step 5: Configure kubeconfig
# =============================================================================
log "Step 5: Configuring kubeconfig..."

docker cp kcp:/.kcp/admin.kubeconfig /tmp/rlark/kcp-raw.kubeconfig

# Docker-internal kubeconfig
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

# User-facing kubeconfig
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

# Install CRDs
log "Regenerating CRDs for kcp (maxDescLen=0)..."
$(go env GOPATH)/bin/controller-gen crd:maxDescLen=0,allowDangerousTypes=true \
  paths="$PROJECT_ROOT/api/rlark.io/..." \
  output:crd:artifacts:config=/tmp/rlark/crds
log "  CRDs generated to /tmp/rlark/crds"
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  apply -f /tmp/rlark/crds/ --validate=false
log "  CRDs installed to kcp"

# UI credentials
log "Generating UI credentials..."
ADMIN_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(8))')
USER_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(8))')
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  create namespace default --dry-run=client -o yaml 2>/dev/null | \
  kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard apply --validate=false -f - 2>/dev/null || true
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  delete secret rlark-ui-auth -n default --ignore-not-found 2>/dev/null || true
kubectl --kubeconfig /tmp/rlark/admin.kubeconfig --context root-shard \
  create secret generic rlark-ui-auth -n default \
  --from-literal="admin-password=$ADMIN_PASSWORD" \
  --from-literal="user-password=$USER_PASSWORD" \
  --validate=false
ok "kubeconfig, DB config, CRDs, and UI credentials ready"

# =============================================================================
# Step 6: Start control plane
# =============================================================================
log "Step 6: Starting control plane..."
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
GATEWAY_READY=false
for i in $(seq 1 30); do
  if curl -s -o /dev/null -w "%{http_code}" "http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes" 2>/dev/null | grep -q "200"; then
    GATEWAY_READY=true
    break
  fi
  sleep 2
done
$GATEWAY_READY || err "Gateway failed to become ready"
ok "Control plane is running"

# =============================================================================
# Step 7: Start UI (optional)
# =============================================================================
if $START_UI; then
  log "Step 7: Starting UI dev server..."
  cd "$PROJECT_ROOT/apps/rlark-ui"
  if [ ! -d node_modules ]; then
    log "Installing UI dependencies..."
    npm install
  fi
  log "Starting UI on http://localhost:5173 (press Ctrl+C to stop)..."
  VITE_API_BASE_URL=http://localhost:9000 VITE_DATA_MODE=backend npm run dev
else
  log "Step 7: Skipping UI (--no-ui)"
fi

# =============================================================================
# Final: Print summary
# =============================================================================
echo ""
echo "=============================================="
ok "Control plane ready!"
echo ""
echo "Control plane:"
echo "  kcp:                      localhost:6443"
echo "  rlark-server:             localhost:8443"
echo "  rlark-gateway (REST API): localhost:9000"
if $START_UI; then
echo "  UI (admin console):       http://localhost:5173/admin"
echo "  UI (platform):            http://localhost:5173"
fi
echo ""
echo "Credentials:"
echo "  admin / $ADMIN_PASSWORD"
echo "  user  / $USER_PASSWORD"
echo ""
echo "Kubeconfig: /tmp/rlark/admin.kubeconfig"
echo ""

if $START_UI; then
  echo "Next steps:"
  echo "  1. Open http://localhost:5173/admin and sign in as admin"
  echo "  2. Go to Cluster Management → Create Cluster"
  echo "  3. Enter a cluster name (e.g. my-cluster) and create it"
  echo "  4. Copy the cluster-id from the UI and run: bash quickstart-dp.sh --cluster-id=<cluster-id>"
  echo "  5. Return to the UI to create domains and submit jobs"
  echo ""
  echo "Stop:"
  echo "  Press Ctrl+C to stop the UI, then:"
  echo "  docker compose -f $SCRIPT_DIR/docker-compose.yml down"
  echo "  docker rm -f local-registry"
else
  echo "Use the API directly:"
  echo "  curl http://localhost:9000/api/v1/rlinf.io/v1alpha1/nodes"
  echo ""
  echo "Stop:"
  echo "  docker compose -f $SCRIPT_DIR/docker-compose.yml down"
fi
echo "  docker rm -f local-registry"