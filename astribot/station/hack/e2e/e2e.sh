#!/bin/bash
# A throwaway copy of the whole Station network on one machine: kcp as the
# management plane, station-api and station-controller as
# containers, and three kind clusters whose nodes are the sites' machines.
# Every node runs the real station-agent-network pod, so WireGuard interfaces
# come up in the node containers' own network namespaces.
#
#   e2e.sh up      create everything (about 6 minutes)
#   e2e.sh plane   restart only the management plane, keeping the clusters
#   e2e.sh down    remove everything
#
# Needs docker, kind, kubectl, openssl, python3 and the host's wireguard
# module. Nothing here touches a real site: all names start with E2E_PREFIX
# (e2e); two runs with different prefixes stand beside each other.
# Images: IMAGES_TAG names station-v2/{station-services,station-controller,
# station-agent,station-wireguard} in the local docker.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../../.." && pwd)
X=${E2E_PREFIX:-e2e}
D=${E2E_DIR:-$HOME/.cache/station-$X}
TAG=${IMAGES_TAG:-e2e-1}
NODE_IMAGE=${NODE_IMAGE:-kindest/node:v1.31.0}
AGENT_IMAGE=${AGENT_IMAGE:-station-v2/station-agent:$TAG}
WIREGUARD_IMAGE=${WIREGUARD_IMAGE:-station-v2/station-wireguard:$TAG}
# cluster ID=number of nodes (the first is the control plane, which also runs pods).
# Every node container needs inotify instances: with fs.inotify.max_user_instances=128
# only about three start; raise it to 8192 for five.
declare -A CLUSTERS
for entry in ${E2E_CLUSTERS:-$X-office=2 $X-lab=1 $X-ksyun=2}; do CLUSTERS[${entry%=*}]=${entry#*=}; done

mgmt() { kubectl --kubeconfig "$D/kcp.kubeconfig" "$@"; }
member() { local cluster=$1; shift; kubectl --kubeconfig "$D/$cluster.kubeconfig" "$@"; }
ip_of() { docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$1"; }

down() {
  for cluster in "${!CLUSTERS[@]}" ${EXTRA_CLUSTERS:-}; do kind delete cluster --name "$cluster" >/dev/null 2>&1 || true; done
  docker rm -f $X-station-api $X-station-controller $X-kcp >/dev/null 2>&1 || true
  rm -rf "$D"
}

certificates() {
  mkdir -p "$D/pki" && cd "$D/pki"
  openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=station-$X-ca" -keyout ca.key -out ca.crt 2>/dev/null
  for name in $X-station-api; do
    openssl req -newkey rsa:2048 -nodes -subj "/CN=$name" -keyout $name.key -out $name.csr 2>/dev/null
    openssl x509 -req -in $name.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 30 -out $name.crt \
      -extfile <(echo "subjectAltName=DNS:$name${STATION_API_IP:+,IP:$STATION_API_IP}") 2>/dev/null
  done
  chmod 644 ./*.key
}

create_cluster() { # cluster nodes
  local cluster=$1 nodes=$2 config="$D/$1.kind.yaml"
  # /boot: kubeadm's system verification reads the kernel configuration, which a node container lacks.
  local boot='  extraMounts: [{hostPath: /boot, containerPath: /boot, readOnly: true}]'
  { echo "kind: Cluster"; echo "apiVersion: kind.x-k8s.io/v1alpha4"; echo "nodes:"; echo "- role: control-plane"; echo "$boot"
    for _ in $(seq 2 "$nodes"); do echo "- role: worker"; echo "$boot"; done; echo "${KIND_NETWORKING:-}"; } > "$config"
  kind create cluster --name "$cluster" --image "$NODE_IMAGE" --config "$config" --kubeconfig "$D/$cluster.kubeconfig" --wait "${KIND_WAIT:-120s}" 2>&1 | tail -2
  load "$cluster"
}

# register makes the cluster a member: the reader identity station-api uses in
# it, the Cluster object and the credential Secret on the management plane.
register() { # cluster [id: the name of its Cluster object, where RLark made one]
  local cluster=$1 id=${2:-$1}
  member "$cluster" apply -f "$REPO/astribot/station/deploy/member-rbac.yaml" >/dev/null
  local token uid ca
  token=$(member "$cluster" -n station-system create token station-api-reader --duration 48h)
  uid=$(member "$cluster" -n station-system get sa station-agent -o jsonpath='{.metadata.uid}')
  ca=$(member "$cluster" config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
  cat > "$D/$cluster.member.kubeconfig" <<EOF
apiVersion: v1
kind: Config
clusters: [{name: member, cluster: {server: "https://$cluster-control-plane:6443", certificate-authority-data: $ca}}]
users: [{name: reader, user: {token: $token}}]
contexts: [{name: member, context: {cluster: member, user: reader}}]
current-context: member
EOF
  local secret
  secret=$(python3 -c "import base64,hashlib,sys; print('station-member-'+base64.b32encode(hashlib.sha256(sys.argv[1].encode()).digest()[:10]).decode().rstrip('=').lower())" "$id")
  if ! mgmt get clusters.rlinf.io "$id" >/dev/null 2>&1; then
    mgmt create namespace "$id" --dry-run=client -o yaml | mgmt apply -f - >/dev/null
    mgmt apply -f - >/dev/null <<EOF
apiVersion: rlinf.io/v1alpha1
kind: Cluster
metadata: {name: $id}
spec: {agentNamespace: $id}
EOF
  fi
  mgmt -n station-member-credentials create secret generic "$secret" --from-file=kubeconfig="$D/$cluster.member.kubeconfig" \
    --from-literal=service-account-namespace=station-system --from-literal=service-account-name=station-agent \
    --from-literal=service-account-uid="$uid" --from-literal=audience=station.rlark.io --dry-run=client -o yaml | mgmt apply -f - >/dev/null
  mgmt -n station-member-credentials annotate secret "$secret" --overwrite station.rlark.io/cluster-id="$id" >/dev/null
  member "$cluster" -n station-system create secret generic station-api-ca --from-file=ca.crt="$D/pki/ca.crt" --dry-run=client -o yaml | member "$cluster" apply -f - >/dev/null
}

# agents installs the repository's DaemonSets with the images and endpoint filled in.
agents() { # cluster [id]
  sed -e "s#REPLACE_STATION_API_ENDPOINT#${STATION_API_ENDPOINT:-$X-station-api:18443}#" -e "s#REPLACE_REGISTERED_CLUSTER_ID#${2:-$1}#" \
      -e "s#REPLACE_RLARK_IMAGE#$AGENT_IMAGE#" -e "s#REPLACE_PRIVATE_WIREGUARD_IMAGE#$WIREGUARD_IMAGE#" -e "s#REPLACE_IMAGE_PULL_SECRET#station-registry#" \
      "${AGENT_MANIFEST:-$REPO/astribot/station/deploy/station-agent.yaml}" | member "$1" apply -f - >/dev/null
}

# services (re)starts station-controller and station-api from the images of IMAGES_TAG.
services() {
  docker rm -f $X-station-api $X-station-controller >/dev/null 2>&1 || true
  docker run -d --restart unless-stopped --name $X-station-controller ${STATION_CONTROLLER_RUN:---network kind} -v "$D/identity/station-controller:/identity:ro" -v "$D/pki:/pki:ro" station-v2/station-controller:"$TAG" \
    --management-kubeconfig=/identity/kubeconfig --member-credentials-namespace=station-member-credentials ${STATION_CONTROLLER_ARGS:-} >/dev/null
  docker run -d --restart unless-stopped --name $X-station-api --network kind ${STATION_API_IP:+--ip $STATION_API_IP} -v "$D/identity/station-api:/identity:ro" -v "$D/pki:/pki:ro" \
    station-v2/station-services:"$TAG" /usr/local/bin/station-api --addr=:18443 --kubeconfig=/identity/kubeconfig \
    --member-credentials-namespace=station-member-credentials --tls-cert-file=/pki/$X-station-api.crt --tls-key-file=/pki/$X-station-api.key ${STATION_API_ARGS:-} >/dev/null
}

# load puts the agent and gateway images of IMAGES_TAG on every node of a cluster.
load() { kind load docker-image --name "$1" "$AGENT_IMAGE" "$WIREGUARD_IMAGE" >/dev/null; }

# plane_base starts the management plane's API, with Station's objects declared and no member yet.
plane_base() {
  docker rm -f $X-station-api $X-station-controller $X-kcp >/dev/null 2>&1 || true
  docker run -d --restart unless-stopped --name $X-kcp --network kind --user 0 ghcr.io/kcp-dev/kcp:v0.30.0 start --root-directory=/data >/dev/null
  until docker cp $X-kcp:/data/admin.kubeconfig "$D/kcp-admin.kubeconfig" 2>/dev/null && [[ -s $D/kcp-admin.kubeconfig ]]; do sleep 1; done
  # kcp's serving certificate names its container address; the host and the other containers both reach it there.
  sed -E "s#server: https://[^/]+:6443#server: https://$(ip_of $X-kcp):6443#" "$D/kcp-admin.kubeconfig" > "$D/kcp.kubeconfig"
  kubectl --kubeconfig "$D/kcp.kubeconfig" config use-context root >/dev/null
  until mgmt get namespaces >/dev/null 2>&1; do sleep 1; done

  mgmt apply --server-side -f "$REPO/api/config/crd/bases/rlinf.io_clusters.yaml" -f "$REPO/api/config/crd/bases/rlinf.io_nodes.yaml" -f "$REPO/astribot/station/config/crd/bases" >/dev/null
  mgmt wait --for condition=Established crd --all --timeout 60s >/dev/null
  for namespace in station-system station-member-credentials; do mgmt create namespace $namespace >/dev/null; done
  # kcp makes the workspace's default namespace a moment after it answers; Station's roles have one in it.
  until mgmt get namespace default >/dev/null 2>&1; do sleep 1; done
  # Every client of the management plane has an identity of its own (kcp writes its administrator's credential
  # anew at every start): Station's permissions as Station declares them, the credentials as rlarkadm issues them.
  mgmt apply -f "$REPO/astribot/station/deploy/management-rbac.yaml" -f "$REPO/astribot/station/deploy/node-operation-management-rbac.yaml" >/dev/null
  (cd "$REPO/apps/rlark" && GOWORK=off go build -o "$D/rlarkadm" ./cmd/rlarkadm)
  "$D/rlarkadm" identity issue --kcp-kubeconfig "$D/kcp.kubeconfig" --output-dir "$D/identity" --station >/dev/null
  # Station's images run as a user of their own, who reads the credentials docker hands it from this machine's files.
  chmod -R go+rX "$D/identity"
}

# plane starts the management plane and makes every existing cluster a member.
plane() {
  plane_base
  for cluster in "${!CLUSTERS[@]}"; do register "$cluster"; done

  services
  echo "plane: management plane at $D/kcp.kubeconfig, members at $D/<cluster>.kubeconfig"
}

# site_network declares the three sites the way the live site of 2026-11-12 is
# declared: an office with two hubs that mesh (the second on a reported
# address), a lab with a leaf, a cloud site with two hubs that reach each other
# on their own "VPC" addresses and are dialled on another one.
# Addresses are from 198.18.0.0/15 (reserved for benchmarking): nothing on a
# real network answers them, so a ping that leaves the tunnel is a lost ping.
site_network() {
  local dev k57 k196
  dev=$(ip_of $X-office-control-plane) k57=$(ip_of $X-ksyun-control-plane) k196=$(ip_of $X-ksyun-worker)
  docker exec $X-ksyun-control-plane sh -c "ip addr replace 198.19.2.57/32 dev lo && ip route replace 198.19.2.196/32 via $k196"
  docker exec $X-ksyun-worker sh -c "ip addr replace 198.19.2.196/32 dev lo && ip route replace 198.19.2.57/32 via $k57"
  docker exec $X-office-control-plane ip addr replace 198.19.5.12/32 dev lo # stands for the NAS behind the office hub
  mgmt apply -f - <<EOF
apiVersion: station.rlark.io/v1alpha1
kind: SiteNetwork
metadata: {name: default, namespace: station-system}
spec:
  mode: ${1:-Tunnel}
  pool: 198.18.99.0/24
  sites:
  - {name: office, clusters: [$X-office], mesh: true}
  - {name: lab, clusters: [$X-lab], reaches: [office]}
  - {name: ksyun, clusters: [$X-ksyun]}
  nodes:
  - clusterID: $X-office
    nodeName: $X-office-control-plane
    role: Hub
    address: 198.18.99.1
    endpoints: [{address: "$dev:51820", scope: Site}]
    advertise: [{cidr: 198.19.5.12/32, sites: [ksyun]}]
  - clusterID: $X-ksyun
    nodeName: $X-ksyun-control-plane
    role: Hub
    address: 198.18.99.2
    nodeAddress: 198.19.2.57
    endpoints: [{address: "$k57:51820", scope: Public}]
    advertise: [{cidr: 172.27.0.0/24, sites: [office]}]
  - clusterID: $X-lab
    nodeName: $X-lab-control-plane
    role: Leaf
    address: 198.18.99.3
  - clusterID: $X-ksyun
    nodeName: $X-ksyun-worker
    role: Hub
    address: 198.18.99.4
    nodeAddress: 198.19.2.196
    interface: st-hub0
    listenPort: 51821
    endpoints: [{address: "$k196:51821", scope: Public}]
    advertise: [{cidr: 172.27.1.0/24, sites: [office]}]
  - clusterID: $X-office
    nodeName: $X-office-worker
    role: Hub
    address: 198.18.99.5
    endpoints: [{scope: Site}]
EOF
}

# enable CLUSTER [enabled|host]: every node of the cluster is in the network, its tunnel run by a pod or by the
# machine's own service.
enable() { member "$1" label nodes --all --overwrite station.rlark.io/network="${2:-enabled}" >/dev/null; }

# ping runs busybox ping in a node's network namespace: ping NODE ARGS...
ping() { local node=$1; shift; docker run --rm --network "container:$node" busybox:1.36 ping "$@"; }

# wg shows a node's tunnel as the gateway sees it: wg CLUSTER NODE
wg() { member "$1" -n station-system exec "$(member "$1" -n station-system get pods -o name --field-selector spec.nodeName="$2" -l app=station-agent-network)" -c wireguard -- /station-gateway --status; }

up() {
  down
  certificates
  for cluster in "${!CLUSTERS[@]}"; do create_cluster "$cluster" "${CLUSTERS[$cluster]}"; done
  plane
  for cluster in "${!CLUSTERS[@]}"; do agents "$cluster"; done
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  case ${1:-} in
  up) up ;;
  down) down ;;
  *) "$@" ;;
  esac
fi
