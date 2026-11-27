#!/bin/sh
# Second half of the node bootstrap; station-bootstrap runs it as a host
# process with the answer of Station API in the environment. Order matters:
# the tunnel comes first: the site's names, the control plane's among them, come with it,
# and a node of a cluster whose control plane is on the tunnel registers with its tunnel address.
#
# Leaves on the machine:
#   /opt/rlark/bin                      kubeadm, kubelet, kubectl, station-gateway, containerd, its shim, ctr, runc
#   rlark-station-gateway.service       the tunnel, started before the kubelet on every boot
#   /etc/rlark/station                  bootstrap.json, the document the machine was admitted with; station-api.json and
#                                       ca.crt, where the tunnel service asks for the document of now with the machine's key
#   containerd-fleet.service            the bundle's containerd, for the kubelet (the machine's docker, if it has one, is not touched)
#   kubelet.service                     reads drop-ins from /etc/kubernetes/kubelet.conf.d
#   /etc/hosts                          the site's names, kept by the tunnel service (lines ending in "# rlark names")
#   /etc/rlark/station/resolv.conf      the site's name servers, likewise, for the kubelet to give to pods
#   /var/lib/rlark/volumes/NAME.img     the node's volumes (NODE_VOLUMES, from its admission), each mounted at its
#                                       path by a mount unit before the runtime and the kubelet start (lib.sh)
# and on its Node the annotations rlark.io/node-install-spec, -state and rlark.io/node-volumes (lib.sh).
set -eu
. /var/lib/rlark/bootstrap/lib.sh
SOCKET=/run/containerd-fleet/containerd.sock
# The site settings this node is installed from; what is this node's alone
# (name, tunnel address, labels) is not among them.
SETTINGS="worker api=$JOIN_API_SERVER interface=$TUNNEL_INTERFACE"
modprobe wireguard || fail "the kernel has no wireguard module"
disk 10

# 0. A machine that is a node already leaves its cluster first (RLARK_MIGRATE,
#    set by migrate-node.sh). It keeps its container runtime, and with it the
#    images it has, and its WireGuard key, and with that its tunnel address.
#    A machine that kept the installation of a node that is no more (the
#    node was removed, or its cluster is gone) has it cleared first; one whose
#    cluster still has its node is left alone: it is removed there first.
if [ -z "${RLARK_MIGRATE:-}" ] && { [ -e /etc/kubernetes/kubelet.conf ] || [ -e /etc/kubernetes/bootstrap-kubelet.conf ]; }; then
  [ ! -e /etc/kubernetes/manifests/kube-apiserver.yaml ] || fail "this machine is a cluster's control plane; it does not join another cluster"
  if [ -e /etc/kubernetes/kubelet.conf ]; then
    old=$(timeout 20 "$B"/bin/kubectl --kubeconfig /etc/kubernetes/kubelet.conf auth whoami -o jsonpath='{.status.userInfo.username}' 2>/dev/null || true)
    old=${old#system:node:}
    if [ -n "$old" ] && timeout 20 "$B"/bin/kubectl --kubeconfig /etc/kubernetes/kubelet.conf get node "$old" >/dev/null 2>&1; then
      fail "this machine is still node $old of the cluster at $(sed -n 's/^ *server: *//p' /etc/kubernetes/kubelet.conf | head -1); remove that node first"
    fi
  fi
  socket=$(sed -n 's|.*--container-runtime-endpoint=\(unix://[^ "]*\).*|\1|p' /var/lib/kubelet/kubeadm-flags.env 2>/dev/null || true)
  systemctl stop kubelet 2>/dev/null || true
  "$B"/bin/kubeadm reset --force ${socket:+--cri-socket "$socket"} > "$B"/reset.log 2>&1 || { tail -20 "$B"/reset.log >&2; fail "kubeadm reset of the installation left on this machine"; }
  rm -rf /etc/cni/net.d/*
  stamp "cleared the installation of node ${old:-unknown} left on this machine: its cluster has no such node or cannot be reached"
fi
if [ -e /etc/kubernetes/kubelet.conf ]; then
  [ ! -e /etc/kubernetes/manifests/kube-apiserver.yaml ] || fail "this node is its cluster's control plane: the cluster would end with it. A control plane moves to another machine with its cluster (control-plane, FROM_SNAPSHOT), not into another cluster"
  SOCKET=$(sed -n 's/.*--container-runtime-endpoint=unix:\/\/\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env)
  [ -S "$SOCKET" ] || fail "cannot find the node's container runtime socket"
  # While the machine is still a node of its cluster: a kernel without the
  # modules is no reason to leave it.
  kernel
  # What makes this machine a node of the old cluster is kept: if it does not
  # become a node of the new one, it goes back by itself (back_to_old_cluster).
  OLD=$B/old-cluster
  keep_old_cluster
  systemctl stop kubelet
  "$B"/bin/kubeadm reset --force --cri-socket "unix://$SOCKET" > "$B"/reset.log 2>&1 || { tail -20 "$B"/reset.log >&2; fail "kubeadm reset"; }
  rm -rf /etc/cni/net.d/*
  KEEP_RUNTIME=1
  stamp "left the old cluster; runtime $SOCKET and its images are kept"
  back_to_old_cluster() {
    code=$?
    [ $code != 0 ] || return 0
    trap - EXIT
    echo "$(date +%T) the move failed; back to the old cluster" >&2
    systemctl stop kubelet 2>/dev/null || true
    [ ! -x /opt/rlark/bin/kubeadm ] || /opt/rlark/bin/kubeadm reset --force --cri-socket "unix://$SOCKET" >/dev/null 2>&1 || true
    restore_old_cluster
    echo "$(date +%T) the old kubelet is started with its old identity; what failed is in $B/install.log" >&2
    exit $code
  }
  trap back_to_old_cluster EXIT
fi

# The node's volumes (NODE_VOLUMES, lib.sh), before anything is written where they go.
volumes
install -d /opt/rlark/bin /opt/cni/bin /etc/rlark/station /etc/kubernetes/kubelet.conf.d
install -m 755 "$B"/bin/* /opt/rlark/bin/
cp -n "$B"/cni/* /opt/cni/bin/

# 1. The tunnel, as a host service (lib.sh). The document Station API has
#    just given this machine is the one it runs on: what an agent stored
#    while the machine was a node before is older, and would be preferred
#    (a machine that joins again kept the names of that time, and looked for
#    its control plane where it no longer was).
rm -f /run/station-network/config.json /var/lib/station/wg/last-applied.json
tunnel_of_before "$TUNNEL_ADDRESS"
tunnel_service
# The peers learn this machine's key from their agents and gateways; Station
# API says from their reports how far it has gone, and when it will not.
station-gateway --config /etc/rlark/station/bootstrap.json --await-peers ||
  fail "no peer has answered through the tunnel: station-gateway --config /etc/rlark/station/bootstrap.json --status"
tunnel_address_is "$TUNNEL_ADDRESS"
stamp "tunnel up: $TUNNEL_ADDRESS on $TUNNEL_INTERFACE"

# 2. A containerd for the kubelet, with the images a node needs before it
#    can pull anything.
[ -n "${KEEP_RUNTIME:-}" ] || { kernel; own_runtime; }
registry
import_images "$B"/images
stamp "container runtime ready, $(ls "$B"/images/*.tar | wc -l) image archives imported"

# 3. The kubelet (lib.sh). A node joins at the control plane's name, which
#    the tunnel service has written into /etc/hosts from the site's names: the
#    control plane can move to another machine or network without any node's
#    certificates or kubeconfig changing.
kubelet_unit
preflight=$(cgroup_v1 "$("$B"/bin/kubelet --version | awk '{print $2}')")
case ${JOIN_API_SERVER%:*} in *[a-z]*) getent hosts "${JOIN_API_SERVER%:*}" >/dev/null ;; esac ||
  fail "this machine cannot resolve ${JOIN_API_SERVER%:*}, the name of its cluster's control plane: it is not among the names its site network gave it (SiteNetwork, names)"

# 4. Join. The node registers with the address it reaches its cluster's API
#    from (lib.sh, node_address), its labels and taints.
NODE_ADDRESS=$(node_address "${JOIN_API_SERVER%:*}")
[ -n "$NODE_ADDRESS" ] || fail "this machine has no route to its cluster's API ${JOIN_API_SERVER%:*}"
stamp "the node's address: $NODE_ADDRESS (the one it reaches its cluster's API ${JOIN_API_SERVER%:*} from)"
{
  cat <<JOIN
apiVersion: kubeadm.k8s.io/v1beta4
kind: JoinConfiguration
discovery:
  bootstrapToken:
    apiServerEndpoint: "$JOIN_API_SERVER"
    token: "$JOIN_TOKEN"
    caCertHashes: ["$JOIN_CA_HASH"]
nodeRegistration:
  name: "$NODE_NAME"
  criSocket: unix://$SOCKET
  ignorePreflightErrors: [Swap, FileExisting-conntrack, FileExisting-socat${preflight:+, $preflight}]
  kubeletExtraArgs:
  - {name: node-ip, value: "$NODE_ADDRESS"}
  - {name: node-labels, value: "${NODE_LABELS:+$NODE_LABELS,}station.rlark.io/network=host"}
  taints:
JOIN
  for taint in $(echo "$NODE_TAINTS" | tr ',' ' '); do
    pair=${taint%:*}
    echo "  - {key: \"${pair%%=*}\", value: \"${pair#*=}\", effect: \"${taint##*:}\"}"
  done
} > "$B"/join.yaml
[ -n "$NODE_TAINTS" ] || sed -i 's/^  taints:$/  taints: []/' "$B"/join.yaml
# A join that fails half-way leaves files that make the next try fail before it starts: they go with it.
kubeadm join --config "$B"/join.yaml > "$B"/join.log 2>&1 || { tail -20 "$B"/join.log >&2; kubeadm reset -f --cri-socket unix://$SOCKET >/dev/null 2>&1; fail "kubeadm join"; }
stamp "joined $JOIN_API_SERVER as $NODE_NAME"
tries=0
until kubectl --kubeconfig /etc/kubernetes/kubelet.conf get node "$NODE_NAME" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; do
  tries=$((tries + 1))
  [ $tries -lt 90 ] || { node_install "$SETTINGS" "not Ready 3 min after its join" || true; fail "the node is registered but not Ready after 3 min: kubectl describe node $NODE_NAME"; }
  sleep 2
done
stamp "node Ready"
node_install "$SETTINGS" ok
rm -rf "$B"/images "$B"/join.yaml
echo "Done. Nothing else is needed on this machine."
