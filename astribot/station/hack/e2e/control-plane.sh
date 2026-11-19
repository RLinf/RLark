#!/bin/bash
# Isolated run of "this machine becomes the control plane of a cluster"
# (bootstrap/control-plane.sh) in the shape of the site's first use of it: a
# robot cluster whose control plane is on a robot gets a new one on a
# workstation that is a worker of the office cluster; the robots move over;
# later the control plane moves on to a server, from a snapshot.
#
# The machines are containers with systemd and a containerd (a node image
# with Kubernetes taken out), never more than three at a time:
#   office   control plane of the office cluster (made by the mode itself)
#   ws       the workstation: a worker of the office cluster whose tunnel a pod
#            runs, with a process and a container outside Kubernetes that must
#            never restart, and a kubelet setting of its own
#   robot1   control plane of the old robot cluster, built by hand: binaries in
#            /opt/robot-kubernetes, the machine's own containerd, no endpoint name
#   robot2   worker of the old robot cluster
#   server   the machine the control plane moves to at the end
# There is no management plane here. hack/e2e/stationstub answers the
# installer's redeem call, and this script does what station-controller and
# the agents do for the tunnel: it writes each machine's WireGuard document
# from the keys it has seen. The tunnels are real WireGuard interfaces in the
# machines' network namespaces. migrate-node.sh needs the management plane and
# is not run: its Job is, with install.sh's migrate path on the node.
#
#   control-plane.sh bundle             the installer of VERSION with the control plane's images and Calico
#   control-plane.sh up                 stub, office (control plane), ws (its worker, tunnel by a pod, see workstation)
#   control-plane.sh robot_cp ws        run the mode on ws for the robot cluster ("robot_cp ws undo" takes it back)
#   control-plane.sh old                robot1 + robot2: the old robot cluster
#   control-plane.sh move NODE          a robot from the old cluster to the one of ws (BREAK=yes: a join that fails)
#   control-plane.sh reinstall robot1   out of the old cluster, wiped, installed as a worker
#   control-plane.sh server             the control plane from ws to a new machine, from a snapshot
#   control-plane.sh takeover [A [B]]   the control plane moves with its cluster from A to B, a worker of it, while
#                                       both run (handover, FROM_SNAPSHOT, undo); "takeover_back" stops halfway and goes back
#   control-plane.sh workstation_shape  ws as the site's workstation is a node (package kubelet, swap on)
#   control-plane.sh refusals NODE      what the mode refuses before it changes anything
#   control-plane.sh addresses NODE     where an address of the machine's own network is held: nowhere
#   control-plane.sh relocate           ws is carried to another network and boots there
#   control-plane.sh reboot NODE        a machine restarts; its calico-node comes back by itself
#   control-plane.sh vip NODE...        the API's Service address from a robot's host network
#   control-plane.sh down
# Functions are run by name; cp-rehearsal/NOTES.md has the order they were run in.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../../.." && pwd)
P=${CPR_PREFIX:-cpr}
D=${CPR_DIR:-$REPO/../cp-rehearsal/run}
VERSION=${VERSION:-v1.33.13}
KUBE_DIR=${KUBE_DIR:-$REPO/../kube}
CALICO=${CALICO:-$REPO/../calico/v3.30.7/calico.yaml}
CALICO_VERSION=v3.30.7
NODE_IMAGE=${NODE_IMAGE:-kindest/node:v1.31.0}
INSTALLER=rlark/node-bootstrap:${TAG:-$P-$VERSION}
GATEWAY_IMAGE=station-v2/station-wireguard:$P
ROBOT=cp.cpr-robot.rlark.internal OFFICE=cp.cpr-office.rlark.internal
# The machines' addresses in the network they stand in: 172.18.0.<BASE + n>.
# ws-moved is the workstation's after it was carried to another network.
B=${CPR_BASE:-200}
declare -A IP=([office]=172.18.0.$((B+1)) [ws]=172.18.0.$((B+5)) [robot1]=172.18.0.$((B+3)) [robot2]=172.18.0.$((B+4)) [server]=172.18.0.$((B+9)) [ws-moved]=172.18.0.$((B+15)))
# A robot's own network: every robot's computer has this address in it.
ROBOT_INNER=192.168.0.10
[[ ! -e $D/ip-ws ]] || IP[ws]=$(cat "$D/ip-ws")
declare -A TUN=([office]=10.254.240.1 [ws]=10.254.240.5 [robot1]=10.254.240.3 [robot2]=10.254.240.4 [server]=10.254.240.9)
now() { date +%s.%N; }
since() { printf '%.1f s' "$(echo "$(now) - $1" | bc)"; }
stamp() { echo "$(date +%T) $*"; }
on() { local node=$1; shift; docker exec -i "$P-$node" "$@"; }
# k NODE ...: kubectl as the administrator of the cluster whose control plane NODE is.
k() { local node=$1; shift; on "$node" env PATH=/opt/rlark/bin:/opt/robot-kubernetes/bin:/usr/bin:/bin kubectl --kubeconfig /etc/kubernetes/admin.conf "$@"; }
exists() { docker inspect "$P-$1" >/dev/null 2>&1; }

bundle() {
  local images=$D/images refs
  rm -rf "$images" "$D/manifests"; mkdir -p "$images/control-plane" "$D/manifests" "$D/bin"
  refs=$("$KUBE_DIR/$VERSION/kubeadm" config images list --kubernetes-version "$VERSION")
  docker save --platform linux/amd64 $(echo "$refs" | grep -E '/(kube-proxy|pause):') -o "$images/node.tar"
  docker save --platform linux/amd64 $(echo "$refs" | grep -v -E '/(kube-proxy|pause):') calico/kube-controllers:$CALICO_VERSION -o "$images/control-plane/control-plane.tar"
  docker save --platform linux/amd64 calico/node:$CALICO_VERSION calico/cni:$CALICO_VERSION -o "$images/calico.tar"
  echo "$refs" | grep '/pause:' > "$images/pause.ref"
  cp "$CALICO" "$D/manifests/calico.yaml"
  # What a site adds to every cluster it makes goes in after the CNI; here a mark that it did.
  printf 'apiVersion: v1\nkind: ConfigMap\nmetadata: {name: site-manifest-applied, namespace: kube-system}\ndata: {by: "the bundle"}\n' > "$D/manifests/site.yaml"
  KUBE_BIN=$KUBE_DIR/$VERSION CNI_BIN=${CNI_BIN:-/opt/cni/bin} IMAGES=$images MANIFESTS=$D/manifests "$REPO/astribot/station/bootstrap/build.sh" "$INSTALLER" "$D/installer.tar"
  (cd "$REPO/astribot/station-gateway" && w=$(mktemp -d) && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$w/station-gateway" . &&
    printf 'FROM scratch\nCOPY station-gateway /station-gateway\nENTRYPOINT ["/station-gateway"]\n' > "$w/Dockerfile" && docker build -q -t "$GATEWAY_IMAGE" "$w" >/dev/null && rm -rf "$w")
  # station-gateway as the bundle has it: the administrator's first tunnel on a machine without one.
  docker rm -f $P-extract >/dev/null 2>&1 || true
  docker create --name $P-extract "$INSTALLER" >/dev/null && docker cp $P-extract:/bundle/bin/station-gateway "$D/bin/" && docker rm $P-extract >/dev/null
}

stub() {
  mkdir -p "$D/pki" "$D/stub/redeem" "$D/stub/answers" "$D/keys" "$D/storage"
  (cd "$D/pki"
   openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=cpr-ca" -keyout ca.key -out ca.crt 2>/dev/null
   openssl req -newkey rsa:2048 -nodes -subj "/CN=$P-station-api" -keyout api.key -out api.csr 2>/dev/null
   openssl x509 -req -in api.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 30 -out api.crt -extfile <(echo "subjectAltName=DNS:$P-station-api") 2>/dev/null
   chmod 644 ./*.key)
  local w; w=$(mktemp -d)
  (cd "$REPO/astribot/station" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$w/stationstub" ./hack/e2e/stationstub)
  printf 'FROM scratch\nCOPY stationstub /stationstub\nENTRYPOINT ["/stationstub"]\n' > "$w/Dockerfile"
  docker build -q -t $P/stationstub "$w" >/dev/null; rm -rf "$w"
  docker rm -f $P-station-api >/dev/null 2>&1 || true
  docker run -d --name $P-station-api --network kind --user "$(id -u)" -v "$D/pki:/pki:ro" -v "$D/stub:/data" $P/stationstub \
    --tls-cert-file=/pki/api.crt --tls-key-file=/pki/api.key --dir=/data >/dev/null
}

# machine NAME: a machine with systemd and a containerd and nothing of
# Kubernetes. Its /etc/hosts is a file of its own, as a machine's is: docker
# writes a container's anew at every start.
machine() {
  local name=$1
  docker rm -f -v "$P-$name" >/dev/null 2>&1 || true
  printf '127.0.0.1 localhost\n%s %s\n' "${IP[$name]}" "$name" > "$D/hosts-$name"
  docker run -d --name "$P-$name" --hostname "$name" --network kind --ip "${IP[$name]}" --privileged --cgroupns=private \
    --tmpfs /run --tmpfs /tmp -v /var -v /lib/modules:/lib/modules:ro -v /boot:/boot:ro \
    -v "$D/hosts-$name:/etc/hosts" -v "$D/storage:/mnt/site" "$NODE_IMAGE" >/dev/null
  until on "$name" systemctl is-active containerd >/dev/null 2>&1; do sleep 0.5; done
  on "$name" sh -c 'systemctl disable --now kubelet >/dev/null 2>&1; rm -rf /etc/systemd/system/kubelet.service* /usr/lib/systemd/system/kubelet.service* /etc/kubernetes /var/lib/kubelet /opt/cni/bin/*; rm -f /usr/bin/kubeadm /usr/bin/kubelet /usr/bin/kubectl; systemctl daemon-reload'
}

# document NODE: the machine's tunnel as station-controller would compute it
# from the keys that are known: every other machine is a peer at its address.
document() {
  local node=$1 peer peers=
  for peer in "${!IP[@]}"; do
    [[ $peer != "$node" && -e $D/keys/$peer ]] || continue
    peers+="${peers:+,}{\"publicKey\":\"$(cat "$D/keys/$peer")\",\"endpoint\":\"${IP[$peer]}:51820\",\"allowedIPs\":[\"${TUN[$peer]}/32\"],\"keepaliveSeconds\":25}"
  done
  # The names the site network declares (SiteNetwork, names): every machine's
  # document carries them and its tunnel service keeps them in /etc/hosts.
  local names="{\"name\":\"$ROBOT\",\"address\":\"${TUN[$(cat "$D/robot-cp" 2>/dev/null || echo ws)]}\"},{\"name\":\"$OFFICE\",\"address\":\"${TUN[office]}\"}"
  # NO_NAMES: a site network that declares no names and no name servers yet.
  local declared=",\"names\":[$names],\"dns\":[\"${SITE_DNS:-172.18.0.1}\"]"
  [[ -z ${NO_NAMES:-} ]] || declared=
  echo "{\"interface\":\"st-wg0\",\"address\":\"${TUN[$node]}/32\",\"listenPort\":51820,\"mtu\":1380,\"peers\":[$peers],\"routes\":[\"10.254.240.0/24\"]$declared}"
}
# tunnels: every machine gets its document the way its agent would store it:
# beside a host service, or for the pod that runs the tunnel of an office node.
tunnels() {
  local node
  for node in "${!IP[@]}"; do
    exists "$node" && [[ -e $D/keys/$node ]] || continue
    document "$node" > "$D/tunnel-$node.json"
    on "$node" sh -c '[ ! -d /run/station-network ] || cat > /run/station-network/config.json' < "$D/tunnel-$node.json"
  done
  ! exists office || [[ ! -e $D/tunnel-ws.json ]] || ! k office get namespace station-system >/dev/null 2>&1 ||
    k office -n station-system create configmap tunnel-documents --from-literal=ws.json="$(cat "$D/tunnel-ws.json")" --dry-run=client -o yaml | k office apply -f - >/dev/null
}
# first_tunnel NODE: what an administrator gives a machine that is to become a
# control plane and is in no cluster: its key is made and declared, and the
# machine gets a document of its own (a site has no tool for this: there, a
# machine gets its tunnel by joining a cluster as a node, admit-node.sh).
first_tunnel() {
  local node=$1
  on "$node" sh -c 'mkdir -p /opt/rlark/bin /etc/rlark/station && cat > /opt/rlark/bin/station-gateway && chmod 755 /opt/rlark/bin/station-gateway' < "$D/bin/station-gateway"
  on "$node" /opt/rlark/bin/station-gateway --public-key > "$D/keys/$node"
  tunnels
  on "$node" sh -c 'cat > /etc/rlark/station/bootstrap.json' < "$D/tunnel-$node.json"
}

# run_cp NODE [undo|check] [NAME=VALUE ...]: the mode, as the person's one command
# runs it, except that the installer shares the machine's namespaces by
# container name, the machine being a container without a docker of its own.
run_cp() {
  local node=$1 action= env=(); shift
  [[ ! ${1:-} =~ ^(undo|check|handover|resume)$ ]] || { action=$1; shift; }
  for pair in "$@"; do env+=(-e "$pair"); done
  docker run --rm --name "$P-run-$node" --privileged --net="container:$P-$node" --pid="container:$P-$node" ${DOCKER_ARGS:-} "${env[@]}" "$INSTALLER" control-plane $action
}
robot_cp() { run_cp "$1" "${@:2}" CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16 ETCD_SNAPSHOT_DIR=/mnt/site/etcd-robot; }

# admit NODE CONTROL_PLANE API_SERVER: what admit-node.sh, the
# controller and Station API do for a machine: a code, and once the machine
# has shown its key, the tunnel for it and for its peers and a join token.
# Prints the code; the answer is written in the background.
admit() {
  local node=$1 cp=$2 api=$3 code
  code=$(head -c 512 /dev/urandom | tr -dc 'a-z0-9' | cut -c1-8)
  (
    until [[ -s $D/stub/redeem/$code.key ]]; do sleep 0.5; done
    cp "$D/stub/redeem/$code.key" "$D/keys/$node"
    tunnels
    token=${BAD_TOKEN:-$(on "$cp" env PATH=/opt/rlark/bin:/opt/robot-kubernetes/bin:/usr/bin:/bin kubeadm token create --ttl 30m)}
    hash=sha256:$(on "$cp" cat /etc/kubernetes/pki/ca.crt | openssl x509 -pubkey -noout | openssl pkey -pubin -outform der | sha256sum | cut -d' ' -f1)
    cat > "$D/stub/answers/$code.json.part" <<ANSWER
{"state":"ready","cluster_id":"cpr","node_name":"$node","tunnel_address":"${TUN[$node]}","config":$(cat "$D/tunnel-$node.json"),
 "join":{"api_server":"$api","token":"$token","ca_cert_hash":"$hash"},
 "labels":"${LABELS:-}","taints":"${TAINTS:-}","volumes":"${VOLUMES:-}"}
ANSWER
    mv "$D/stub/answers/$code.json.part" "$D/stub/answers/$code.json"
  ) >>"$D/admit.log" 2>&1 &
  echo "$code"
}
# worker NODE CONTROL_PLANE API_SERVER: the installer's worker mode on a machine.
worker() {
  local code; code=$(admit "$@")
  docker run --rm --name "$P-run-$1" --privileged --net="container:$P-$1" --pid="container:$P-$1" -v "$D/pki/ca.crt:/ca.crt:ro" \
    -e RLARK_SERVER=https://$P-station-api:18443 -e RLARK_CODE="$code" "$INSTALLER"
}

# up: the office cluster. Its control plane is made by the mode on a machine
# that has only a tunnel; ws joins it with the worker mode, then its tunnel is
# given to a pod (the gateway container of station-agent.yaml, its document
# from a ConfigMap instead of an agent), which is how the workstation's is run.
up() {
  down; stub
  machine office; first_tunnel office
  run_cp office CONTROL_PLANE_NAME=$OFFICE POD_SUBNET=10.244.0.0/16 SERVICE_SUBNET=10.96.0.0/16 ETCD_SNAPSHOT_DIR=/mnt/site/etcd-office
  machine ws
  worker ws office $OFFICE:6443
  on ws sh -c "ctr -n k8s.io -a /run/containerd-fleet/containerd.sock images import -" < <(docker save "$GATEWAY_IMAGE")
  k office create namespace station-system
  tunnels
  k office apply -f - <<DS
apiVersion: apps/v1
kind: DaemonSet
metadata: {name: station-agent-network, namespace: station-system}
spec:
  selector: {matchLabels: {app: station-agent-network}}
  template:
    metadata: {labels: {app: station-agent-network}}
    spec:
      hostNetwork: true
      nodeSelector: {station.rlark.io/network: enabled}
      tolerations: [{operator: Exists}]
      containers:
      - name: wireguard
        image: $GATEWAY_IMAGE
        imagePullPolicy: IfNotPresent
        env:
        - {name: NODE_NAME, valueFrom: {fieldRef: {fieldPath: spec.nodeName}}}
        - {name: STATION_GATEWAY_CONFIG, value: /config/none.json}
        - {name: STATION_GATEWAY_COMPUTED_CONFIG, value: "/documents/\$(NODE_NAME).json"}
        - {name: STATION_GATEWAY_PRUNE_ROUTES, value: "true"}
        - {name: STATION_GATEWAY_HOSTS_FILE, value: /host/etc/hosts}
        securityContext: {runAsUser: 0, capabilities: {drop: [ALL], add: [NET_ADMIN]}}
        volumeMounts:
        - {name: key, mountPath: /var/lib/station/wg}
        - {name: documents, mountPath: /documents}
        - {name: hosts, mountPath: /host/etc/hosts}
      volumes:
      - {name: hosts, hostPath: {path: /etc/hosts, type: File}}
      - {name: key, hostPath: {path: /var/lib/station/wg, type: DirectoryOrCreate}}
      - {name: documents, configMap: {name: tunnel-documents}}
DS
  on ws systemctl disable --now rlark-station-gateway
  k office label node ws --overwrite station.rlark.io/network=enabled
  k office -n station-system rollout status ds/station-agent-network --timeout=120s
  until on ws test -s /var/lib/station/wg/last-applied.json; do sleep 1; done
  workstation
}

# workstation: what makes ws the workstation: what runs on it outside
# Kubernetes (a process of a systemd unit, and a container of the machine's
# own containerd, where a workstation has docker), a kubelet setting of this
# node only, and pods of the office cluster that must go elsewhere when it leaves.
workstation() {
  on ws systemd-run --unit desktop-process --quiet sleep infinity
  on ws sh -c "ctr -n desktop images import - >/dev/null" < <(docker save --platform linux/amd64 busybox:1.36)
  on ws ctr -n desktop run -d --net-host docker.io/library/busybox:1.36 desktop sleep 1000000
  # A setting of this node only, as a workstation's swap handling is.
  on ws sh -c "sed -i '/^maxPods:/d' /var/lib/kubelet/config.yaml && echo 'maxPods: 77' >> /var/lib/kubelet/config.yaml && systemctl restart kubelet"
  until [[ $(k office get node ws -o jsonpath='{.status.capacity.pods}') == 77 ]]; do sleep 1; done
  # Something of the office cluster that runs on ws and has to go elsewhere when ws leaves.
  k office create deployment guest --image="$(cat "$D/images/pause.ref")" --replicas=2
  k office patch deployment guest --type=merge -p '{"spec":{"template":{"spec":{"tolerations":[{"operator":"Exists"}]}}}}'
  k office rollout status deployment guest --timeout=120s
  k office get nodes -o wide; k office get pods -A -o wide | grep -v kube-system
}

# witness: what must be the same before and after on the workstation.
witness() {
  local node=${1:-ws}
  on "$node" sh -c 'echo "process: pid $(systemctl show desktop-process -p MainPID --value) started $(systemctl show desktop-process -p ActiveEnterTimestamp --value)"
    echo "container: pid $(ctr -n desktop task ls | awk "/desktop/ {print \$2, \$3}") started $(ps -o lstart= -p $(ctr -n desktop task ls | awk "/desktop/ {print \$2}"))"
    echo "machine containerd: pid $(systemctl show containerd -p MainPID --value) started $(systemctl show containerd -p ActiveEnterTimestamp --value)"
    echo "tunnel: $(ip -4 -o addr show dev st-wg0 | awk "{print \$4}") ifindex $(cat /sys/class/net/st-wg0/ifindex) $(cat /var/lib/station/wg/private.key | sha256sum | cut -c1-12)"'
}

# probe FROM ADDRESS PORT SECONDS: one TCP connection a second through the
# tunnel; prints how many did not get an answer (refused is an answer).
probe() {
  on "$1" bash -c "lost=0; for i in \$(seq $4); do timeout 1 bash -c 'echo > /dev/tcp/$2/$3' 2>&1 | grep -q -v refused && lost=\$((lost+1)); sleep 1; done; echo \"tunnel probe $1 -> $2: \$lost of $4 lost\""
}

# old: the old robot cluster, made by hand the way the robot's was: robot1
# with Kubernetes in /opt/robot-kubernetes on the machine's own containerd,
# joined to by address; robot2 a worker with the installer's layout. Both
# with a static pod that stands for the device layer.
calico() { # pod subnet
  sed -e "s#veth_mtu: \"0\"#veth_mtu: \"1360\"#" \
      -e "s#^\( *\)- name: CALICO_IPV4POOL_IPIP#\1- name: IP_AUTODETECTION_METHOD\n\1  value: \"interface=st-wg0\"\n\1- name: CALICO_IPV4POOL_CIDR\n\1  value: \"$1\"\n\1- name: FELIX_IGNORELOOSERPF\n\1  value: \"true\"\n&#" "$CALICO"
}
static_pod() {
  on "$1" sh -c 'mkdir -p /etc/kubernetes/manifests && cat > /etc/kubernetes/manifests/witness.yaml' <<POD
apiVersion: v1
kind: Pod
metadata: {name: witness, namespace: default}
spec:
  hostNetwork: true
  containers: [{name: witness, image: $(cat "$D/images/pause.ref"), imagePullPolicy: Never}]
POD
}
old() {
  machine robot1; robot_shape robot1; first_tunnel robot1
  on robot1 mkdir -p /opt/robot-kubernetes/bin /opt/cni/bin
  for binary in kubeadm kubelet kubectl; do on robot1 sh -c "cat > /opt/robot-kubernetes/bin/$binary && chmod 755 /opt/robot-kubernetes/bin/$binary" < "$KUBE_DIR/$VERSION/$binary"; done
  docker cp "${CNI_BIN:-/opt/cni/bin}/." $P-robot1:/opt/cni/bin/
  for archive in node.tar control-plane/control-plane.tar calico.tar; do on robot1 ctr -n k8s.io images import - < "$D/images/$archive" >/dev/null; done
  on robot1 sh -ec '
    cat > /etc/systemd/system/robot-tunnel.service <<UNIT
[Service]
ExecStart=/opt/rlark/bin/station-gateway --config /etc/rlark/station/bootstrap.json --computed-config /run/station-network/config.json --prune-routes
ExecStartPre=/bin/mkdir -p /run/station-network
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
    cat > /etc/systemd/system/kubelet.service <<UNIT
[Unit]
Description=kubelet, set up by hand
[Service]
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=/opt/robot-kubernetes/bin/kubelet --bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf --config=/var/lib/kubelet/config.yaml --fail-swap-on=false \$KUBELET_KUBEADM_ARGS
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload; systemctl enable --now robot-tunnel kubelet >/dev/null 2>&1
    until ip -4 addr show dev st-wg0 2>/dev/null | grep -q inet; do sleep 0.5; done
    PATH=/opt/robot-kubernetes/bin:$PATH kubeadm init --kubernetes-version '"$VERSION"' --apiserver-advertise-address '"${TUN[robot1]}"' --node-name robot1 \
      --pod-network-cidr 10.242.0.0/16 --service-cidr 10.243.0.0/16 --cri-socket unix:///run/containerd/containerd.sock \
      --ignore-preflight-errors=Swap,FileExisting-conntrack,FileExisting-socat > /var/log/kubeadm-init.log 2>&1
    sed -i "s/\"\$/ --node-ip='"${TUN[robot1]}"'\"/" /var/lib/kubelet/kubeadm-flags.env; systemctl restart kubelet'
  calico 10.242.0.0/16 | k robot1 apply -f - >/dev/null
  static_pod robot1
  machine robot2; robot_shape robot2
  LABELS=rlark.io/node-category=robot,rlark.io/robot-id=robot2 TAINTS=rlark.io/node-category=robot:NoSchedule worker robot2 robot1 "${TUN[robot1]}:6443"
  static_pod robot2
  k robot1 wait --for=condition=Ready node --all --timeout=180s
  k robot1 create deployment marker --image="$(cat "$D/images/pause.ref")" || true
  k robot1 get nodes -o wide; k robot1 get pods -A -o wide | grep -E 'witness|marker'
}

# move NODE: a robot leaves the old cluster for the one whose control plane
# is ws: the Job of migrate-node.sh, in the old cluster, runs the installer on
# the node. BREAK=yes gives it a join token that is none: the join fails and
# the node must be back in the old cluster by itself.
move() { # NODE [FROM [TO [API]]]
  local node=$1 from=${2:-robot1} to=${3:-ws} api=${4:-$ROBOT:6443} code t0 runtime
  runtime=$(on "$node" sed -n 's/.*--container-runtime-endpoint=unix:\/\/\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env)
  on "$node" sh -c "\$(command -v ctr || echo /opt/rlark-containerd/bin/ctr) -a $runtime -n k8s.io images import -" < "$D/installer.tar" >/dev/null
  code=$(LABELS=rlark.io/node-category=robot,rlark.io/robot-id=$node TAINTS=rlark.io/node-category=robot:NoSchedule \
    BAD_TOKEN=${BREAK:+abcdef.0123456789abcdef} admit "$node" "$to" "$api")
  k "$from" -n kube-system delete job "node-migrate-$node" --ignore-not-found >/dev/null
  on "$node" sh -c 'rm -f /var/lib/rlark/bootstrap/install.log'
  t0=$(now)
  k "$from" apply -f - >/dev/null <<JOB
apiVersion: batch/v1
kind: Job
metadata: {name: node-migrate-$node, namespace: kube-system}
spec:
  backoffLimit: 0
  template:
    spec:
      nodeName: $node
      hostNetwork: true
      hostPID: true
      restartPolicy: Never
      tolerations: [{operator: Exists}]
      containers:
      - name: migrate
        image: $INSTALLER
        imagePullPolicy: IfNotPresent
        securityContext: {privileged: true}
        env:
        - {name: RLARK_SERVER, value: "https://$P-station-api:18443"}
        - {name: RLARK_CODE, value: "$code"}
        - {name: RLARK_MIGRATE, value: "node"}
        - name: RLARK_CA
          value: |
$(sed 's/^/            /' "$D/pki/ca.crt")
JOB
  stamp "$node: the move is running on the node"
  until on "$node" sh -c '! systemctl is-active -q rlark-node-migrate && grep -q -E "^Done|old kubelet is started" /var/lib/rlark/bootstrap/install.log' 2>/dev/null; do sleep 2; done
  on "$node" cat /var/lib/rlark/bootstrap/install.log | sed 's/^/    /'
  stamp "$node: the installer ended $(since "$t0") after the Job was made"
}

# robot_shape NODE: what makes a machine a robot's computer on the network.
# Its address is the same on every robot (the robot's own network), and what
# it sends leaves through the robot's other computer, which translates it:
# here an interface with that address, the default route with it as source,
# and the translation on the way out. Set again on every boot, as a machine's
# network configuration is.
# reinstall NODE [CONTROL_PLANE [API]]: the old cluster's control plane, once
# its other nodes have moved. A control plane does not move into another
# cluster: its cluster is ended on it by hand, and the machine joins as any
# machine that was never a node does.
reinstall() {
  local node=$1
  on "$node" sh -c 'systemctl stop kubelet; PATH=/opt/robot-kubernetes/bin:$PATH kubeadm reset --force >/dev/null 2>&1; rm -rf /etc/kubernetes /var/lib/kubelet /var/lib/etcd /etc/cni/net.d/* /etc/systemd/system/kubelet.service /etc/systemd/system/kubelet.service.d; systemctl daemon-reload'
  LABELS=rlark.io/node-category=robot,rlark.io/robot-id=$node TAINTS=rlark.io/node-category=robot:NoSchedule worker "$node" "${2:-ws}" "${3:-$ROBOT:6443}"
}
# robot_real NODE: a worker of the old cluster as the site's second robot is
# one (11-13): containerd and ctr outside the PATH, a kubelet set up by hand
# with no kubeadm beside it and no --config-dir, and the robots' node profile:
# a drop-in that repeats the start line with --config-dir and runs the profile
# before every start, the static CPU manager, and a profile program whose
# "install" reads the running kubelet (so it fails while there is none).
robot_real() {
  local node=$1
  on "$node" sh -c 'cat > /opt/robot-kubernetes-kubelet' < "$KUBE_DIR/$VERSION/kubelet"
  on "$node" bash -ec '
    mkdir -p /opt/rlark-containerd/bin /opt/robot-kubernetes/bin /usr/local/sbin /etc/kubernetes/kubelet.conf.d
    install -m 755 /opt/robot-kubernetes-kubelet /opt/robot-kubernetes/bin/kubelet && rm /opt/robot-kubernetes-kubelet
    systemctl stop kubelet
    systemctl disable --now containerd >/dev/null 2>&1
    for binary in containerd ctr; do mv "$(command -v $binary)" /opt/rlark-containerd/bin/; done
    sed -i "s#^ExecStart=.*containerd #ExecStart=/opt/rlark-containerd/bin/containerd #" /etc/systemd/system/containerd-fleet.service
    rm -f /opt/rlark/bin/kubeadm /opt/rlark/bin/kubelet /etc/systemd/system/kubelet.service.d/zz-rlark.conf /etc/kubernetes/kubelet.conf.d/*
    cat > /etc/systemd/system/kubelet.service <<UNIT
[Unit]
Description=Robot Kubernetes kubelet
After=network-online.target containerd-fleet.service
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=/opt/robot-kubernetes/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS
Restart=always
RestartSec=10
[Install]
WantedBy=multi-user.target
UNIT
    cat > /etc/systemd/system/kubelet.service.d/fleet-cpu.conf <<UNIT
[Service]
ExecStartPre=/usr/local/sbin/fleet-node-profile kubelet
ExecStart=
ExecStart=/opt/robot-kubernetes/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS --config-dir=/etc/kubernetes/kubelet.conf.d
UNIT
    printf "apiVersion: kubelet.config.k8s.io/v1beta1\nkind: KubeletConfiguration\nfailSwapOn: false\ncpuManagerPolicy: static\nreservedSystemCPUs: \"0-1\"\ncpuManagerPolicyOptions:\n  strict-cpu-reservation: \"true\"\n" > /etc/kubernetes/kubelet.conf.d/50-fleet-node-profile.conf
    cat > /usr/local/sbin/fleet-node-profile <<"PROFILE"
#!/bin/bash
set -Eeuo pipefail
state=/var/lib/kubelet/cpu_manager_state
configured() { sha256sum < /etc/kubernetes/kubelet.conf.d/50-fleet-node-profile.conf; }
case $1 in
  install) tr "\0" "\n" < "/proc/$(systemctl show -p MainPID --value kubelet)/cmdline" > /dev/null ;;
  kubelet)
    if ! grep -q "\"policyName\":\"static\"" $state 2>/dev/null || [[ $(configured) != "$(cat $state.profile 2>/dev/null)" ]]; then rm -f $state; fi
    configured > $state.profile ;;
esac
PROFILE
    chmod 755 /usr/local/sbin/fleet-node-profile
    systemctl daemon-reload; systemctl restart containerd-fleet; systemctl start kubelet'
  sleep 20
  robot_real_state "$node"
}
robot_real_state() {
  on "$1" sh -c 'echo "kubelet: $(tr "\0" " " < /proc/$(systemctl show -p MainPID --value kubelet)/cmdline)"
    echo "CPU manager: $(cat /var/lib/kubelet/cpu_manager_state)"
    echo "drop-ins: $(ls /etc/systemd/system/kubelet.service.d /etc/kubernetes/kubelet.conf.d | xargs)"
    echo "in the PATH: containerd $(command -v containerd || echo no), kubeadm $(command -v kubeadm || echo no)"
    echo "modules at boot: $(cat /etc/modules-load.d/rlark.conf 2>/dev/null | xargs)"'
}
robot_shape() {
  on "$1" sh -c 'cat > /usr/local/sbin/robot-network && chmod 755 /usr/local/sbin/robot-network' <<SHAPE
#!/bin/sh
ip link add robot0 type dummy 2>/dev/null
ip addr replace $ROBOT_INNER/24 dev robot0 && ip link set robot0 up
until ip -4 route show default | grep -q via; do sleep 0.2; done
ip route replace default via \$(ip -4 route show default | awk '{print \$3; exit}') dev eth0 src $ROBOT_INNER
iptables -t nat -C POSTROUTING -o eth0 -s $ROBOT_INNER -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -o eth0 -s $ROBOT_INNER -j MASQUERADE
SHAPE
  on "$1" sh -c 'printf "[Unit]\nBefore=rlark-station-gateway.service robot-tunnel.service kubelet.service\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/usr/local/sbin/robot-network\n[Install]\nWantedBy=multi-user.target\n" > /etc/systemd/system/robot-network.service
    systemctl daemon-reload; systemctl enable --now robot-network >/dev/null 2>&1; ip -4 route show default'
}

# workstation_shape: ws as the site's workstation is a node: the kubelet of a
# package in /usr/bin with the package's unit and drop-in, which read no
# drop-in directory; swap on and allowed in the node's own kubelet
# configuration; registered by its hostname at the address of its network.
workstation_shape() {
  on ws sh -ec '
    systemctl stop kubelet
    mv /opt/rlark/bin/kubelet /opt/rlark/bin/kubeadm /opt/rlark/bin/kubectl /usr/bin/
    rm -f /etc/systemd/system/kubelet.service /etc/systemd/system/kubelet.service.d/zz-rlark.conf /etc/kubernetes/kubelet.conf.d/*
    mkdir -p /usr/lib/systemd/system/kubelet.service.d
    printf "[Unit]\nDescription=kubelet: The Kubernetes Node Agent\nWants=network-online.target\nAfter=network-online.target\n[Service]\nExecStart=/usr/bin/kubelet\nRestart=always\nStartLimitInterval=0\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n" > /usr/lib/systemd/system/kubelet.service
    cat > /usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf <<UNIT
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS \$KUBELET_EXTRA_ARGS
UNIT
    sed -i -e "/^failSwapOn:/d" -e "/^memorySwap:/,/^  swapBehavior:/d" /var/lib/kubelet/config.yaml
    printf "failSwapOn: false\nmemorySwap:\n  swapBehavior: NoSwap\n" >> /var/lib/kubelet/config.yaml
    sed -i -e "s/ --node-ip=[^ \"]*//" -e "s/ --node-labels=[^ \"]*//" /var/lib/kubelet/kubeadm-flags.env
    systemctl daemon-reload; systemctl enable kubelet >/dev/null 2>&1; systemctl start kubelet'
  until [[ $(k office get node ws -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}') == "${IP[ws]}" ]]; do sleep 1; done
  k office wait --for=condition=Ready node ws --timeout=120s >/dev/null
  on ws sh -c 'systemctl show kubelet -p ExecStart --value | sed -n "s/.*argv\[\]=\([^;]*\);.*/kubelet: \1/p"; grep -c . /proc/swaps | sed "s/^/swap lines: /"'
}

# refusals NODE: what the mode says no to before it changes anything; the
# machine's files are the same before and after.
refusals() {
  local node=$1 before
  before=$(on "$node" sh -c 'cat /etc/hosts /etc/kubernetes/kubelet.conf 2>/dev/null | sha256sum; ls /var/lib/rlark/control-plane 2>&1 | sha256sum; systemctl show kubelet -p ActiveEnterTimestamp --value')
  refusal() { # what, then the parameters
    local what=$1 out; shift
    if out=$(run_cp "$node" "$@" 2>&1); then echo "NOT REFUSED ($what): $out"; return 1; fi
    echo "refused ($what): $(echo "$out" | grep FAILED | cut -c1-260)"
  }
  refusal "the name is an address" CONTROL_PLANE_NAME=${TUN[$node]} POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16
  refusal "no name" POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16
  refusal "a name the site network gives another machine" CONTROL_PLANE_NAME=$OFFICE POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16
  refusal "a pod network in use on the machine's networks" CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=172.18.0.0/16 SERVICE_SUBNET=10.243.0.0/16
  refusal "more reserved than the machine has" CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16 SYSTEM_RESERVED_MEMORY=4096Gi
  on "$node" sh -c 'nohup sh -c "exec -a port-holder sleep 600" >/dev/null 2>&1 & python3 -c "import socket,time; s=socket.socket(); s.bind((\"0.0.0.0\",2379)); s.listen(); time.sleep(60)" >/dev/null 2>&1 &' ; sleep 1
  refusal "etcd's port in use" CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16
  on "$node" pkill -f "s.bind" || true
  [[ $before == "$(on "$node" sh -c 'cat /etc/hosts /etc/kubernetes/kubelet.conf 2>/dev/null | sha256sum; ls /var/lib/rlark/control-plane 2>&1 | sha256sum; systemctl show kubelet -p ActiveEnterTimestamp --value')" ]] && echo "nothing was changed by the refused runs"
}

# addresses NODE: where an address of the machine's own network is written
# in what makes it a control plane; nothing may be printed. The certificates'
# fingerprints are printed for a comparison across a move.
addresses() {
  local node=$1
  on "$node" sh -c '
    for address in $(ip -4 -o addr show | awk "\$2 != \"lo\" && \$2 != \"st-wg0\" && \$2 != \"tunl0\" && \$2 !~ /^cali/ {print \$4}" | cut -d/ -f1); do
      grep -r -l -w -F "$address" /etc/kubernetes /var/lib/kubelet/kubeadm-flags.env /var/lib/kubelet/config.yaml 2>/dev/null | sed "s/^/HOLDS $address: /"
      for c in /etc/kubernetes/pki/*.crt /etc/kubernetes/pki/etcd/*.crt; do openssl x509 -in $c -noout -ext subjectAltName 2>/dev/null | grep -q -w -F "$address" && echo "HOLDS $address: $c"; done
    done
    for c in apiserver etcd/server etcd/peer; do echo "$c: $(openssl x509 -in /etc/kubernetes/pki/$c.crt -noout -ext subjectAltName | tail -1 | tr -s " ")"; done
    for c in /etc/kubernetes/pki/*.crt /etc/kubernetes/pki/etcd/*.crt; do echo "$(openssl x509 -in $c -noout -fingerprint -sha256 | cut -d= -f2 | cut -c1-23) $c"; done
    grep -h "server:" /etc/kubernetes/*.conf | sort | uniq -c
    grep -h -E "advertise|listen-(client|peer)|initial-cluster=" /etc/kubernetes/manifests/etcd.yaml /etc/kubernetes/manifests/kube-apiserver.yaml'
  k "$node" -n kube-system exec "etcd-$node" -- etcdctl --endpoints https://127.0.0.1:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key member list
  k "$node" get endpoints kubernetes -o jsonpath='{"kubernetes endpoints: "}{.subsets[*].addresses[*].ip}{"\n"}'
  k "$node" -n kube-system get configmap kubernetes-services-endpoint -o jsonpath='{"calico reaches the API at: "}{.data.KUBERNETES_SERVICE_HOST}:{.data.KUBERNETES_SERVICE_PORT}{"\n"}'
  k "$node" get nodes -o wide
}

# relocate: the workstation is carried to another network: it gets another
# address there and boots; the site network learns its new endpoint
# (SiteNetwork, endpoints), which is all that anyone is told. Its tunnel
# address is the one it had.
relocate() {
  local t0; t0=$(now)
  # The node image's own start-up script rewrites Kubernetes' files and deletes
  # the API server's certificate when the container's address has changed; a
  # machine has no such script, so it is told that nothing changed.
  on ws sh -c "printf %s '${IP[ws-moved]}' > /kind/old-ipv4; printf %s '::ffff:${IP[ws-moved]}' > /kind/old-ipv6"
  docker stop "$P-ws" >/dev/null
  docker network disconnect kind "$P-ws"
  docker network connect --ip "${IP[ws-moved]}" kind "$P-ws"
  sed -i "s/^${IP[ws]} /${IP[ws-moved]} /" "$D/hosts-ws"
  IP[ws]=${IP[ws-moved]}; echo "${IP[ws]}" > "$D/ip-ws"
  docker start "$P-ws" >/dev/null
  until on ws systemctl is-active rlark-station-gateway >/dev/null 2>&1; do sleep 0.5; done
  tunnels
  until k ws get --raw /readyz >/dev/null 2>&1; do sleep 1; done
  stamp "ws is at ${IP[ws]} now; its API answered $(since "$t0") after the machine was switched off"
  k ws wait --for=condition=Ready node --all --timeout=300s
  stamp "every node Ready $(since "$t0") after the machine was switched off"
}

# reboot NODE: the whole machine restarts; printed is when its calico-node
# and the node were Ready again, by themselves.
reboot() {
  local node=$1 t0 started; t0=$(now)
  docker restart "$P-$node" >/dev/null
  until on "$node" systemctl is-active kubelet >/dev/null 2>&1; do sleep 0.5; done
  # The agent stores the node's document again; until then the tunnel runs on the last one applied.
  tunnels
  calico() { k ws -n kube-system get pods -l k8s-app=calico-node --field-selector "spec.nodeName=$node" -o jsonpath="{.items[0].status.containerStatuses[0].$1}" 2>/dev/null; }
  until started=$(calico state.running.startedAt) && [[ -n $started && $(date -d "$started" +%s) -ge ${t0%.*} && $(calico ready) == true ]]; do sleep 2; done
  stamp "$node: calico-node Ready again $(since "$t0") after the restart; nobody touched the machine"
  k ws wait --for=condition=Ready "node/$node" --timeout=180s
}

# vip NODE...: from a robot's host network, the Service address of the API.
# A robot's own address is the same on every robot; an answer that came back
# is one that found its robot. First as a process of the host, then as a
# host-network pod.
vip() {
  local node
  for node in "$@"; do
    echo "$node: robot0 $(on "$node" sh -c "ip -4 -o addr show dev robot0 | awk '{print \$4}'"), default route: $(on "$node" ip -4 route show default)"
    echo "$node host -> https://10.243.0.1:443/version: $(on "$node" curl -sk -m 5 -o /dev/null -w '%{http_code} in %{time_total}s' https://10.243.0.1:443/version || echo FAILED)"
    on "$node" sh -c 'ctr -a $(sed -n "s/.*--container-runtime-endpoint=unix:\/\/\([^ \"]*\).*/\1/p" /var/lib/kubelet/kubeadm-flags.env) -n k8s.io images import - >/dev/null' < <(docker save --platform linux/amd64 busybox:1.36)
    k ws delete pod "vip-$node" --ignore-not-found --wait >/dev/null
    k ws apply -f - >/dev/null <<POD
apiVersion: v1
kind: Pod
metadata: {name: vip-$node}
spec:
  nodeName: $node
  hostNetwork: true
  restartPolicy: Never
  tolerations: [{operator: Exists}]
  containers:
  - name: ask
    image: docker.io/library/busybox:1.36
    imagePullPolicy: Never
    command: [sh, -c, "nc -w 5 \$KUBERNETES_SERVICE_HOST \$KUBERNETES_SERVICE_PORT </dev/null && echo connected to \$KUBERNETES_SERVICE_HOST:\$KUBERNETES_SERVICE_PORT"]
POD
    k ws wait --for=jsonpath='{.status.phase}'=Succeeded "pod/vip-$node" --timeout=90s >/dev/null || k ws describe pod "vip-$node" | tail -5
    echo "$node host-network pod: $(k ws logs "vip-$node" | tr -s ' \n' ' ')"
  done
}

# server: the control plane moves from the workstation to another machine,
# from the workstation's last snapshot. The workstation is switched off; the
# site network gives the name the server's tunnel address (SiteNetwork,
# names), which is all the robots are told: their kubeconfigs and
# certificates stay as they are.
server() {
  local t0
  on ws systemctl start rlark-etcd-snapshot
  k ws get nodes -o wide
  on robot1 sh -c 'sha256sum /etc/kubernetes/kubelet.conf /var/lib/kubelet/pki/kubelet-client-current.pem' > "$D/robot1-before"
  t0=$(now)
  docker rm -f -v "$P-ws" >/dev/null
  stamp "ws is switched off and gone"
  machine server
  echo server > "$D/robot-cp"
  first_tunnel server
  run_cp server CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16 FROM_SNAPSHOT=/mnt/site/etcd-robot ETCD_SNAPSHOT_DIR=/mnt/site/etcd-robot-server
  stamp "the control plane is on server $(since "$t0") after ws was switched off"
  k server wait --for=condition=Ready node/server node/robot1 node/robot2 --timeout=300s
  stamp "the robots are Ready under it $(since "$t0") after ws was switched off"
  on robot1 sh -c 'sha256sum /etc/kubernetes/kubelet.conf /var/lib/kubelet/pki/kubelet-client-current.pem' | diff - "$D/robot1-before" && echo "robot1: its kubeconfig and its certificate are the ones it had"
  on robot1 getent hosts $ROBOT
  k server get nodes -o wide
  k server get pods -A -o wide
}

# The cluster as the nodes and its objects show it: who they are, not how they do.
cluster_state() {
  k "$1" get namespaces,nodes,deployments,daemonsets,secrets,configmaps,serviceaccounts -A -o jsonpath='{range .items[*]}{.kind} {.metadata.namespace}/{.metadata.name} {.metadata.uid}{"\n"}{end}' | sort
}
# api_watch NODE: the API as a node's host sees it at the name, twice a second, until $D/api-watch.stop.
api_watch() {
  rm -f "$D/api-watch.stop"
  while [[ ! -e $D/api-watch.stop ]]; do
    echo "$(now) $(on "$1" curl -sk -m 1 -o /dev/null -w '%{http_code}' "https://$ROBOT:6443/readyz" 2>/dev/null || true)"
    sleep 0.4
  done > "$D/api-watch.log"
}
api_outage() {
  touch "$D/api-watch.stop"; wait
  awk '$2 != 200 && $2 != 401 && $2 != 403 {if (!first) first = $1; last = $1; lost++} END {printf "the API at the name, seen from a robot: %d probes without an answer, from the first to the last of them %.1f s\n", lost, last - first}' "$D/api-watch.log"
}
# takeover_to A B: A hands its control plane over; B, a worker of the cluster,
# takes it from A's last snapshot; the site network gives the name to B. The
# snapshot directory is on storage both have (a person carries it, on a site).
takeover_to() {
  local from=$1 to=$2 t0
  exists "$to" || { machine "$to"; worker "$to" "$from" "$ROBOT:6443"; k "$from" wait --for=condition=Ready "node/$to" --timeout=180s; }
  cluster_state "$from" > "$D/state-before"
  for robot in robot1 robot2; do on $robot sh -c 'sha256sum /etc/kubernetes/kubelet.conf /var/lib/kubelet/pki/kubelet-client-current.pem; crictl ps -q 2>/dev/null | sort | sha256sum' > "$D/$robot-before"; done
  api_watch robot2 &
  t0=$(now)
  # B is started first and waits for A's snapshot.
  run_cp "$to" CONTROL_PLANE_NAME=$ROBOT POD_SUBNET=10.242.0.0/16 SERVICE_SUBNET=10.243.0.0/16 FROM_CONTROL_PLANE=${TUN[$from]} ETCD_SNAPSHOT_DIR=/mnt/site/etcd-robot-$to > "$D/takeover-$to.log" 2>&1 &
  local taking=$!
  run_cp "$from" handover SUCCESSOR=$to
  stamp "handed over $(since "$t0") after the command"
  [[ -z ${REBOOT_A:-} ]] || { docker restart "$P-$from" >/dev/null; sleep 25; tunnels; on "$from" sh -c 'echo "after the restart of the machine: kubelet $(systemctl is-active kubelet), API port $(ss -Hltn "sport = :6443" | wc -l) listeners, etcd port $(ss -Hltn "sport = :2379" | wc -l), snapshot service $(systemctl is-active rlark-control-plane-handover)"'; }
  # A robot, a node of the cluster too, is not given the cluster's keys.
  echo "robot1 asks for the snapshot with its kubelet's certificate: HTTP $(on robot1 curl -s -m 5 -o /dev/null -w '%{http_code}' --cacert /etc/kubernetes/pki/ca.crt --cert /var/lib/kubelet/pki/kubelet-client-current.pem --resolve "$ROBOT:6444:${TUN[$from]}" "https://$ROBOT:6444/")"
  until grep -q 'left the old cluster\|no longer a worker' "$D/takeover-$to.log"; do sleep 0.5; done
  stamp "$to has the snapshot $(since "$t0") after the handover began; the name is given to it now"
  echo "$to" > "$D/robot-cp"; tunnels
  wait $taking; sed 's/^/    /' "$D/takeover-$to.log"
  k "$to" wait --for=condition=Ready "node/$to" node/robot1 node/robot2 --timeout=300s
  stamp "the robots are Ready under $to $(since "$t0") after the handover began"
  cluster_state "$to" > "$D/state-after"
  echo "objects that are not the ones they were (none but the two control planes' own are expected):"; diff "$D/state-before" "$D/state-after" | grep '^[<>]' || echo "  none"
  for robot in robot1 robot2; do on $robot sh -c 'sha256sum /etc/kubernetes/kubelet.conf /var/lib/kubelet/pki/kubelet-client-current.pem; crictl ps -q 2>/dev/null | sort | sha256sum' | diff - "$D/$robot-before" >/dev/null && echo "$robot: its kubeconfig, its certificate and its containers are the ones it had"; done
}
takeover() {
  local from=${1:-ws} to=${2:-server}
  takeover_to "$from" "$to"
  sleep 60; api_outage
  run_cp "$from" undo
  k "$to" get nodes -o wide
}
# takeover_back: the same, and then back: the name is given back, A takes up
# where it stopped, B gives the control plane up and is the worker it was.
takeover_back() {
  local from=${1:-ws} to=${2:-server} t0
  takeover_to "$from" "$to"
  t0=$(now)
  echo "$from" > "$D/robot-cp"; tunnels
  run_cp "$from" resume
  run_cp "$to" undo
  k "$from" wait --for=condition=Ready "node/$from" "node/$to" node/robot1 node/robot2 --timeout=300s
  stamp "back under $from, $to a worker again, $(since "$t0") after the name was given back"
  sleep 30; api_outage
  cluster_state "$from" > "$D/state-back"
  echo "objects that are not the ones they were before the handover:"; diff "$D/state-before" "$D/state-back" | grep '^[<>]' || echo "  none"
  k "$from" get nodes -o wide
}
# move_cp TO: hack/move-control-plane.sh as an administrator runs it from a
# machine of the site (robot2 here: it resolves the control plane's name by
# the site's names, wherever that is). The harness has no management plane:
# the tool's kubectl is a stand-in that answers for the SiteNetwork from the
# harness's own record of the name and gives every other call to the cluster.
move_cp() {
  local to=$1 from; from=$(cat "$D/robot-cp" 2>/dev/null || echo ws)
  exists "$to" || { machine "$to"; worker "$to" "$from" "$ROBOT:6443"; k "$from" wait --for=condition=Ready "node/$to" --timeout=180s; }
  for node in "$from" "$to"; do on "$node" sh -c "ctr -a /run/containerd-fleet/containerd.sock -n k8s.io images import - || ctr -n k8s.io images import -" < "$D/installer.tar" >/dev/null 2>&1; done
  on "$from" cat /etc/kubernetes/admin.conf | on robot2 sh -c 'cat > /root/admin.conf'
  cluster_state "$from" > "$D/state-before"
  mkdir -p "$D/tool"
  cat > "$D/tool/kubectl" <<TOOL
#!/bin/bash
shift 2   # --kubeconfig FILE
case "\$*" in
*"get sitenetwork default"*) echo '{"spec":{"names":[{"name":"$ROBOT","address":"'\$(cat $D/name-address)'"},{"name":"$OFFICE","address":"${TUN[office]}"}]}}' ;;
*"patch sitenetwork default"*) python3 -c 'import json,sys; print(next(n["address"] for n in json.load(sys.stdin)["spec"]["names"] if n["name"] == sys.argv[1]))' $ROBOT > $D/name-address
  for node in ${!TUN[*]}; do [[ \$(bash $0 tun \$node) != \$(cat $D/name-address) ]] || echo \$node > $D/robot-cp; done; bash $0 tunnels ;;
*) exec docker exec -i -e PATH=/opt/rlark/bin:/opt/robot-kubernetes/bin:/usr/bin:/bin $P-robot2 kubectl --kubeconfig /root/admin.conf "\$@" ;;
esac
TOOL
  chmod 755 "$D/tool/kubectl"; echo "${TUN[$from]}" > "$D/name-address"
  api_watch robot1 &
  PATH=$D/tool:$PATH KCP=none "$REPO/astribot/station/hack/move-control-plane.sh" none "$to" "$INSTALLER"
  sleep 60; api_outage
  cluster_state "$to" > "$D/state-after"
  echo "objects that are not the ones they were (none but the two control planes' own are expected):"; diff "$D/state-before" "$D/state-after" | grep '^[<>]' || echo "  none"
  on "$to" tail -5 /var/lib/rlark/bootstrap/control-plane.log
}
tun() { echo "${TUN[$1]}"; }
down() {
  local name
  for name in $(docker ps -a --format '{{.Names}}' | grep "^$P-"); do docker rm -f -v "$name" >/dev/null; done
  # The machines wrote into the shared directory as root.
  [[ ! -d $D/storage ]] || docker run --rm -v "$D:/d" busybox:1.36 rm -rf /d/storage /d/stub
  rm -rf "$D/keys" "$D/pki" "$D"/hosts-* "$D"/tunnel-*.json "$D/admit.log"
}

"$@"
