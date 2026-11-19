#!/bin/sh
# Makes this machine the control plane of a cluster; station-bootstrap runs it
# as a host process. One command, by a person with sudo, on the machine:
#
#   sudo docker run --rm --privileged --net=host --pid=host -e CONTROL_PLANE_NAME=cp.<cluster-id>.rlark.internal \
#     -e POD_SUBNET=... -e SERVICE_SUBNET=... IMAGE control-plane
#
# It asks Station API for nothing and carries no credential. Run again, it
# goes on where an earlier run stopped, and changes nothing on a machine that
# is finished. "IMAGE control-plane check" runs the pre-flight alone and
# changes nothing. "IMAGE control-plane undo" takes the control plane away
# again and puts the machine back into the cluster it had left.
#
# A cluster's control plane moves to another machine with the cluster:
#   on the machine it leaves   IMAGE control-plane handover   with SUCCESSOR=<node>: no more writes, a last snapshot,
#                                                             everything stopped; the snapshot and the cluster's keys
#                                                             are given to that node's kubelet, and to nobody else
#   on the machine it goes to  IMAGE control-plane            with FROM_CONTROL_PLANE=<the old one's tunnel address>,
#                                                             on that node, a worker of the cluster: it takes the
#                                                             snapshot and goes on from there; the site network is
#                                                             to give the name its address (SiteNetwork, names)
#   on the machine it leaves   IMAGE control-plane undo       once the nodes are Ready under the new one
# Until that undo, "IMAGE control-plane resume" on the old machine (after undo
# on the new one, and the name given back) takes up where handover stopped.
#
# In order:
#   pre-flight  every refusal, before anything is changed
#   leave       a machine that is a node of another cluster is drained there
#               with its kubelet's credentials and leaves: its pods, the
#               kubelet's state and the CNI configuration go; the container
#               runtime, its images, docker, drivers and the tunnel stay, and
#               nothing is restarted. What made it a node is kept for undo
#   tunnel      the site tunnel becomes a host service with the key and the
#               configuration the machine has (lib.sh); the interface is not
#               taken down in between
#   kubelet     the bundle's kubelet; this machine's share for everything that
#               is not Kubernetes as a drop-in (SYSTEM_RESERVED_*)
#   name        CONTROL_PLANE_NAME resolves to the tunnel address on this
#               machine by a line of its own in /etc/hosts; every other node
#               has it from the site's names (SiteNetwork, names), which
#               must not give it another address
#   init        kubeadm init at the bundle's version: the endpoint is the
#               name; the certificates name it, the tunnel address and the
#               node; etcd listens on the tunnel address; the four components
#               get requests and limits; with FROM_SNAPSHOT, etcd holds a
#               snapshot of the control plane this one replaces, and the
#               cluster's CAs are its
#   manifests   the bundle's: Calico over the tunnel interface, reaching the
#               API at the name and not through its Service; then the site's
#   snapshots   a daily etcd snapshot into ETCD_SNAPSHOT_DIR, the first one now
#   addresses   a refusal to call it done while a certificate, a kubeconfig,
#               etcd or an object of the cluster holds an address of this
#               machine other than the tunnel's
#   annotations rlark.io/node-install-spec and -state (lib.sh)
#
# Nothing of the control plane knows the machine by an address of its own
# network: the machine can be carried to another network, and comes back as
# the same control plane when its tunnel does. The machine's swap, its docker,
# its GPU and what runs outside Kubernetes are not touched or restarted.
#
# Parameters (environment):
#   CONTROL_PLANE_NAME      required: the name every node reaches the API at
#   POD_SUBNET, SERVICE_SUBNET   required
#   NODE_NAME               this node's name (the one it has, else the hostname)
#   TUNNEL_INTERFACE        st-wg0
#   POD_MTU                 1360
#   WORKLOADS               yes: the machine runs its cluster's work too (a new site's first machine, its
#                           GPUs the site's): no control-plane taint, and the reservations below default to
#                           what the system needs; otherwise it runs the control plane only
#   SYSTEM_RESERVED_MEMORY  what Kubernetes may never take (Gi or Mi; all but 8 Gi, or with WORKLOADS
#                           16 Gi, an eighth of a smaller machine)
#   SYSTEM_RESERVED_CPU     likewise, in CPUs (all but 4, or with WORKLOADS 4, a quarter of a smaller machine)
#   ETCD_SNAPSHOT_DIR       /var/backups/rlark-etcd; ETCD_SNAPSHOT_KEEP 14
#   NODE_VOLUMES            the machine's volumes, "NAME=SIZE:PATH,..." (lib.sh, volumes), made before its runtime
#   FROM_CONTROL_PLANE      the tunnel address of the control plane that was
#                           handed over to this node: its snapshot is taken from it
#   FROM_SNAPSHOT           a directory written by another control plane's
#                           snapshot service (etcd-*.db and pki.tar): this
#                           machine takes that control plane's place
# What it replaces or removes is kept under /var/backups/rlark-control-plane.
set -eu
. /var/lib/rlark/bootstrap/lib.sh
export PATH="/opt/rlark/bin:$PATH"
S=/var/lib/rlark/control-plane K=/etc/kubernetes
OLD=$S/old-cluster
changed=
: "${WORKLOADS:=no}" "${TUNNEL_INTERFACE:=st-wg0}" "${POD_MTU:=1360}" "${ETCD_SNAPSHOT_DIR:=/var/backups/rlark-etcd}" "${ETCD_SNAPSHOT_KEEP:=14}"
name=${CONTROL_PLANE_NAME:-}
VERSION=$("$B/bin/kubeadm" version -o short)
adm() { kubectl --kubeconfig $K/admin.conf "$@"; }
# The cluster the machine is a node of, asked as its kubelet.
old() { "$B/bin/kubectl" --kubeconfig $K/kubelet.conf "$@"; }
initialized() { [ -e "$S/initialized" ]; }
# The name this machine has as a node now: its kubelet's, else the hostname.
node_name() {
  sed -n 's/.*--hostname-override=\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env 2>/dev/null | grep . || hostname | tr '[:upper:]' '[:lower:]'
}
node=$(node_name)
NODE_NAME=${NODE_NAME:-$node}
kubelet=$(systemctl show kubelet -p ExecStart --value 2>/dev/null | sed -n 's/.*path=\([^ ;]*kubelet\) .*/\1/p')
# cpu request, cpu limit, memory request and limit (Mi) of the four static
# pods: enough for a site of a hundred nodes, and no more than a workstation
# can give beside its desktop.
RESOURCES='kube-apiserver 250m 2 768 2048
etcd 100m 1 384 1536
kube-controller-manager 100m 1 192 1024
kube-scheduler 50m 500m 96 512'

tunnel_address() {
  ip -4 -o addr show dev "$TUNNEL_INTERFACE" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | grep . && return 0
  for document in /var/lib/station/wg/last-applied.json /etc/rlark/station/bootstrap.json; do
    # The machine's own is the one with a prefix length; the names' have none.
    [ -s "$document" ] || continue
    tr -d ' \t\n' < "$document" | grep -o '"address":"[0-9.]*/' | head -1 | cut -d'"' -f4 | cut -d/ -f1 | grep . && return 0
  done
}
# named_elsewhere: what the site network gives this control plane's name, if
# it is not this machine's address; from the document the machine's tunnel
# runs on (station-gateway's order). The other nodes find the control plane
# by it: the name may be missing there still, not be elsewhere.
tunnel_document() {
  for document in /run/station-network/config.json /var/lib/station/wg/last-applied.json /etc/rlark/station/bootstrap.json; do
    [ ! -s "$document" ] || { echo "$document"; return; }
  done
}
named_elsewhere() {
  tr -d ' \t\n' < "$(tunnel_document)" | grep -o '"names":\[[^]]*\]' | grep -o "{\"name\":\"$name\",\"address\":\"[0-9.]*\"}" | grep -v -F "\"$(tunnel_address | head -1)\"" || true
}
# unanswered: this machine dials peers of the site tunnel and none has
# answered it. No node would reach a control plane here. (The first machine
# of a site dials nobody.)
unanswered() {
  grep -q '"endpoint"' "$(tunnel_document)" && ! "$B/bin/station-gateway" --config "$(tunnel_document)" --status 2>/dev/null | grep -q '"lastHandshake":"'
}
# same_cluster: this machine is a node of the very cluster whose control
# plane it takes over (FROM_SNAPSHOT): the cluster's CA is the one it trusts.
same_cluster() {
  [ -n "${FROM_SNAPSHOT:-}" ] && [ -e $K/pki/ca.crt ] && tar -xOf "$FROM_SNAPSHOT/pki.tar" pki/ca.crt 2>/dev/null | cmp -s - $K/pki/ca.crt
}
# held: the control plane's static pods are taken from the kubelet, or given
# back. What was running is ended either way: nothing of a control plane
# that is handed over goes on beside the one that takes its place.
ports_closed() { for port in "$@"; do [ -z "$(ss -Hltn "sport = :$port")" ] || return 1; done; }
handover() {
  initialized || fail "this machine is not a control plane made by this installer"
  [ ! -e "$S/handed-over" ] || { echo "Nothing to do: handed over $(cat "$S/handed-over"). The last snapshot is in $(cat "$S/snapshot-dir")."; exit 0; }
  SOCKET=$(cat "$S/socket")
  install -d "$S/held"
  # No more writes: the API, and what writes through it. etcd answers on.
  for component in kube-apiserver kube-controller-manager kube-scheduler; do [ ! -e $K/manifests/$component.yaml ] || mv $K/manifests/$component.yaml "$S/held/"; done
  tries=0
  until ports_closed 6443; do tries=$((tries + 1)); [ $tries -lt 90 ] || fail "the API server has not stopped after 3 min: journalctl -u kubelet"; sleep 2; done
  stamp "the API is stopped: nothing is written any more"
  /opt/rlark/bin/rlark-etcd-snapshot > "$S/last-snapshot" || fail "the last etcd snapshot: $(cat "$S/last-snapshot")"
  systemctl disable --now rlark-etcd-snapshot.timer >/dev/null 2>&1
  [ ! -e $K/manifests/etcd.yaml ] || mv $K/manifests/etcd.yaml "$S/held/"
  tries=0
  until ports_closed 2379; do tries=$((tries + 1)); [ $tries -lt 90 ] || fail "etcd has not stopped after 3 min: journalctl -u kubelet"; sleep 2; done
  # The node's other pods end too: the cluster starts them where it goes on.
  systemctl disable --now kubelet >/dev/null 2>&1
  CTR=$(ctr_of "$SOCKET")
  for task in $("$CTR" -a "$SOCKET" -n k8s.io tasks ls -q); do "$CTR" -a "$SOCKET" -n k8s.io tasks kill -s SIGKILL "$task" >/dev/null 2>&1 || true; done
  date '+%F %T' > "$S/handed-over"
  stamp "handed over. The last snapshot: $(cat "$S/last-snapshot")"
  address=$(tunnel_address | head -1)
  if [ -n "${SUCCESSOR:-}" ]; then
    printf '[Unit]\nDescription=The last snapshot of this control plane, for the node %s that takes over\nAfter=rlark-station-gateway.service\n[Service]\nExecStart=%s snapshot-serve %s:6444 %s %s %s\nRestart=always\nRestartSec=2\n[Install]\nWantedBy=multi-user.target\n' \
      "$SUCCESSOR" "$B/station-bootstrap" "$address" "$SUCCESSOR" "$(cat "$S/snapshot-dir")" $K/pki > /etc/systemd/system/rlark-control-plane-handover.service
    systemctl daemon-reload
    systemctl enable --now rlark-control-plane-handover >/dev/null 2>&1 || fail "the snapshot is not served: journalctl -u rlark-control-plane-handover"
    echo "Done. Next: run the installer on $SUCCESSOR with FROM_CONTROL_PLANE=$address, and give $(cat "$S/name") the tunnel address of $SUCCESSOR in the site network (SiteNetwork, names). This machine keeps everything until \"control-plane undo\"; \"control-plane resume\" takes up here again."
  else
    echo "Done. Next: carry $(cat "$S/snapshot-dir") (the snapshots and pki.tar; root only) to the machine that takes over, give $(cat "$S/name") its tunnel address in the site network (SiteNetwork, names), and run the installer there with FROM_SNAPSHOT. This machine keeps everything until \"control-plane undo\"; \"control-plane resume\" takes up here again."
  fi
}
resume() {
  [ -e "$S/handed-over" ] || fail "this control plane was not handed over"
  name=$(cat "$S/name")
  [ -z "$(named_elsewhere)" ] || fail "the site network gives $(cat "$S/name") to another machine ($(named_elsewhere)): the nodes look for the control plane there. Give the name back first (SiteNetwork, names)"
  systemctl disable --now rlark-control-plane-handover >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/rlark-control-plane-handover.service
  mv "$S"/held/*.yaml $K/manifests/
  systemctl enable --now kubelet >/dev/null 2>&1
  systemctl enable --now rlark-etcd-snapshot.timer >/dev/null 2>&1
  rm -f "$S/handed-over"
  tries=0
  until adm get --raw /readyz >/dev/null 2>&1; do tries=$((tries + 1)); [ $tries -lt 150 ] || fail "the API does not answer after 5 min: journalctl -u kubelet"; sleep 2; done
  stamp "the control plane is running again, as it was when it was handed over"
  echo "Done."
}

undo() {
  [ -e "$S/left" ] || [ -e "$S/init-started" ] || { stamp "no control plane was made on this machine by this installer: nothing to undo"; exit 0; }
  SOCKET=$(cat "$S/socket")
  keep=/var/backups/rlark-control-plane/removed-$(date +%Y%m%d%H%M%S)
  name=$(cat "$S/name" 2>/dev/null || true)
  # Its nodes have their control plane elsewhere when this one was handed
  # over, or when the site network gives the name to another machine.
  if initialized && [ ! -e "$S/handed-over" ] && [ -z "$(named_elsewhere)" ]; then
    NODE_NAME=$(sed -n 's/.* node=//p' "$S/cluster")
    adm get --raw /readyz >/dev/null 2>&1 || fail "the API of this control plane does not answer, so it cannot be seen whether other nodes are in its cluster: start it first (systemctl start kubelet)"
    others=$(adm get nodes -o name | grep -v "^node/$NODE_NAME\$" | tr '\n' ' ')
    [ -z "$others" ] || fail "other nodes are in this cluster ($others): they would be left without a control plane. Move them to another cluster first"
  fi
  install -d -m 700 "$keep"
  if initialized && [ -e "$S/handed-over" ]; then
    (umask 077 && tar -cpf "$keep/etc-kubernetes.tar" -C / etc/kubernetes)
    # A machine that was the cluster's control plane has no place to go back to.
    rm -rf "$OLD"
    stamp "kept in $keep: /etc/kubernetes; the last snapshot is in $(cat "$S/snapshot-dir")"
  elif initialized; then
    NODE_NAME=$(sed -n 's/.* node=//p' "$S/cluster")
    adm -n kube-system exec "etcd-$NODE_NAME" -- etcdctl --endpoints https://127.0.0.1:2379 --cacert $K/pki/etcd/ca.crt --cert $K/pki/etcd/server.crt --key $K/pki/etcd/server.key \
      snapshot save /var/lib/etcd/rlark-snapshot-removed.db >/dev/null 2>&1 || fail "etcd snapshot"
    mv /var/lib/etcd/rlark-snapshot-removed.db "$keep/etcd-snapshot.db"
    (umask 077 && tar -cpf "$keep/etc-kubernetes.tar" -C / etc/kubernetes)
    stamp "kept in $keep: /etc/kubernetes and a last snapshot of etcd"
  fi
  systemctl disable --now rlark-control-plane-handover >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/rlark-control-plane-handover.service
  systemctl disable --now rlark-etcd-snapshot.timer >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/rlark-etcd-snapshot.timer /etc/systemd/system/rlark-etcd-snapshot.service /opt/rlark/bin/rlark-etcd-snapshot
  systemctl stop kubelet 2>/dev/null || true
  "$B/bin/kubeadm" reset --force --cri-socket "unix://$SOCKET" > "$keep/reset.log" 2>&1 || { tail -20 "$keep/reset.log" >&2; fail "kubeadm reset"; }
  rm -rf /etc/cni/net.d/*
  rm -f $K/kubelet.conf.d/20-rlark-control-plane.conf
  hosts_name "" ""
  stamp "the control plane is removed"
  if [ -d "$OLD" ]; then
    restore_old_cluster
    node=$(node_name)
    tries=0
    until old uncordon "$node" >/dev/null 2>&1; do
      tries=$((tries + 1))
      [ $tries -lt 60 ] || fail "the old kubelet is started with its old identity, but its cluster did not let it uncordon $node in 2 min: kubectl uncordon $node there"
      sleep 2
    done
    stamp "back in the cluster this machine had left, as $node, schedulable"
  else
    systemctl disable kubelet >/dev/null 2>&1 || true
    systemctl daemon-reload
  fi
  rm -rf "$S"
  echo "Done."
}
case ${1:-} in
""|check) ;;
undo) undo; exit 0 ;;
handover) handover; exit 0 ;;
resume) resume; exit 0 ;;
*) fail "the arguments are check, undo, handover and resume" ;;
esac

# --- Pre-flight: every refusal comes before anything is changed.
[ "$(id -u)" = 0 ] || fail "this must run as root"
[ -n "$name" ] || fail "CONTROL_PLANE_NAME is required: the name every node reaches the control plane at, cp.<cluster-id>.rlark.internal. It goes into the certificates and into every node's kubeconfig, and is what lets the control plane move to another machine later"
{ echo "$name" | grep -q -E '^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$' && echo "$name" | grep -q '[a-z]'; } ||
  fail "CONTROL_PLANE_NAME=$name is not a name. An address is not accepted: a control plane that is known by an address cannot move without every node's certificates and kubeconfig changing"
[ -n "${POD_SUBNET:-}" ] && [ -n "${SERVICE_SUBNET:-}" ] || fail "POD_SUBNET and SERVICE_SUBNET are required: the cluster's pod and Service networks, which no other cluster of the site may use"
# The control plane that was handed over to this node gives it its last
# snapshot and the cluster's keys, for this node's kubelet certificate. It may
# be asked before it is handed over: this waits.
HANDOVER=/var/lib/rlark/control-plane-handover
if [ -n "${FROM_CONTROL_PLANE:-}" ] && ! initialized; then
  FROM_SNAPSHOT=$HANDOVER
  tries=0
  until [ -e $HANDOVER/pki.tar ] || "$B/station-bootstrap" snapshot-fetch "$FROM_CONTROL_PLANE:6444" "$name" $HANDOVER 2> "$B/handover.log"; do
    tries=$((tries + 1))
    [ $tries -lt 120 ] || fail "the control plane at $FROM_CONTROL_PLANE has not given this node its snapshot after 10 min ($(tail -1 "$B/handover.log")): it does once it is handed over to this node (control-plane handover with SUCCESSOR=$node)"
    sleep 5
  done
fi
[ ! -e $K/manifests/kube-apiserver.yaml ] || [ -e "$S/init-started" ] ||
  fail "this machine is the control plane of a cluster already ($(sed -n 's/^ *server: //p' $K/admin.conf 2>/dev/null)): that cluster ends when it leaves, which is not this command's to decide"
if [ -e $K/kubelet.conf ]; then
  have=$("$kubelet" --version | awk '{print $2}')
  [ "$have" = "$VERSION" ] || fail "this machine's kubelet is $have and this installer's Kubernetes is $VERSION. Upgrade the node in its cluster to $VERSION first (upgrade-cluster.sh), or use the installer of $have"
fi
ADDRESS=$(tunnel_address | head -1) || true
[ -n "$ADDRESS" ] || fail "this machine has no address in the site tunnel: $TUNNEL_INTERFACE has none, and neither /var/lib/station/wg/last-applied.json nor /etc/rlark/station/bootstrap.json holds a tunnel configuration. The nodes reach the control plane through the tunnel: enroll the machine in the site network first"
modprobe wireguard || fail "the kernel has no wireguard module"
initialized || disk
systemctl is-enabled rlark-station-gateway >/dev/null 2>&1 || [ -e /var/lib/station/wg/last-applied.json ] || [ -e /etc/rlark/station/bootstrap.json ] ||
  fail "the tunnel on $TUNNEL_INTERFACE is run by a pod of the cluster this machine would leave, and its configuration is not kept on the machine (/var/lib/station/wg/last-applied.json): the tunnel would not come back after a reboot"
# A tunnel that is up already is asked now; one this installer starts, when it has.
! ip link show dev "$TUNNEL_INTERFACE" >/dev/null 2>&1 || ! unanswered ||
  fail "no peer of the site tunnel that this machine dials has answered it on $TUNNEL_INTERFACE: no node would reach a control plane here. $B/bin/station-gateway --config $(tunnel_document) --status"
# The node a control plane was handed over to does not wait to be named: the
# other nodes learn the name's new address from their agents, whom Station
# API knows by asking their cluster's API, and that is this machine now.
elsewhere=$(named_elsewhere)
[ -n "${FROM_CONTROL_PLANE:-}" ] || [ -z "$elsewhere" ] || fail "the site network gives $name another address than this machine's $ADDRESS ($elsewhere): every other node would look for the control plane there. Change the SiteNetwork (names) first"
# The addresses the machine has in the networks it stands in: nothing of the
# control plane may hold one (the last step looks).
machine_addresses() {
  ip -4 -o addr show | awk -v tunnel="$TUNNEL_INTERFACE" '$2 != "lo" && $2 != tunnel && $2 != "tunl0" && $2 != "vxlan.calico" && $2 !~ /^cali/ {print $4}' | cut -d/ -f1
}

# Memory. The control plane's share is bounded twice: each component by its
# limit, and all pods together by what is not reserved for the system (the
# kubelet enforces it on their common cgroup). Pods get no swap.
total=$(( $(awk '/^MemTotal:/ {print $2}' /proc/meminfo) / 1024 ))
available=$(( $(awk '/^MemAvailable:/ {print $2}' /proc/meminfo) / 1024 ))
cpus=$(nproc)
need=$(( $(echo "$RESOURCES" | awk '{sum += $5} END {print sum}') + 1024 ))
if [ "$WORKLOADS" = yes ]; then
  : "${SYSTEM_RESERVED_MEMORY:=$(( total > 131072 ? 16384 : total / 8 ))Mi}" "${SYSTEM_RESERVED_CPU:=$(( cpus > 16 ? 4 : cpus / 4 ))}"
else
  : "${SYSTEM_RESERVED_MEMORY:=$(( total > 8192 ? total - 8192 : 0 ))Mi}" "${SYSTEM_RESERVED_CPU:=$(( cpus > 8 ? cpus - 4 : cpus / 2 ))}"
fi
case $SYSTEM_RESERVED_MEMORY in
*Gi) reserved=$(( ${SYSTEM_RESERVED_MEMORY%Gi} * 1024 )) ;;
*Mi) reserved=${SYSTEM_RESERVED_MEMORY%Mi} ;;
*) fail "SYSTEM_RESERVED_MEMORY=$SYSTEM_RESERVED_MEMORY: give it in Gi or Mi" ;;
esac
[ $(( total - reserved )) -ge $need ] ||
  fail "reserving $SYSTEM_RESERVED_MEMORY for the system leaves Kubernetes $(( total - reserved )) Mi of this machine's $total Mi, and the control plane may use $need Mi (the limits of its four components and 1 Gi for the cluster's own pods). Reserve less (SYSTEM_RESERVED_MEMORY), or use a machine with more memory"
initialized || [ "$available" -ge $need ] ||
  fail "only $available Mi of this machine's memory are available now (MemAvailable; swap is not counted, pods get none), and the control plane may use $need Mi: what runs on the machine already would pay for it. Free memory, then run this again"

if ! initialized; then
  [ "$(df -k --output=avail /var/lib | tail -1)" -ge 10485760 ] || fail "/var/lib has less than 10 GiB free: etcd and the images need them"
  if [ ! -e "$S/init-started" ]; then
    # (A worker of the cluster it takes over has that cluster's pod routes.)
    same_cluster || [ -e "$S/took-over" ] || for subnet in "$POD_SUBNET" "$SERVICE_SUBNET"; do
      routes=$( { ip -4 route show root "$subnet"; ip -4 route show match "$subnet"; } | grep -v -E '^default |proto bird' | sort -u | tr '\n' ';')
      [ -z "$routes" ] || fail "$subnet is in use on this machine's networks already ($routes): the cluster's pod and Service networks must be free"
    done
    for port in 6443 2379 2380 10257 10259; do
      [ -z "$(ss -Hltn "sport = :$port")" ] ||
        fail "port $port is in use on this machine ($(ss -Hltnp "sport = :$port" | sed -n 's/.*users:(("\([^"]*\)".*/\1/p' | head -1)); the control plane needs it"
    done
    [ -z "$(ls -A /var/lib/etcd 2>/dev/null)" ] || fail "/var/lib/etcd holds data that this installer did not put there; move it away"
  fi
  if [ -n "${FROM_SNAPSHOT:-}" ]; then
    snapshot=$(ls -1t "$FROM_SNAPSHOT"/etcd-*.db 2>/dev/null | head -1)
    [ -n "$snapshot" ] && [ -e "$FROM_SNAPSHOT/pki.tar" ] || fail "FROM_SNAPSHOT=$FROM_SNAPSHOT holds no etcd-*.db with a pki.tar: it is the directory another control plane's snapshot service writes"
  fi
else
  [ "$(cat "$S/cluster")" = "name=$name pods=$POD_SUBNET services=$SERVICE_SUBNET address=$ADDRESS node=$NODE_NAME" ] ||
    fail "this machine is a control plane already, made with: $(cat "$S/cluster"). Its name, networks, address and node name are not changed by running this again"
fi
at=$ETCD_SNAPSHOT_DIR
until [ -e "$at" ]; do at=$(dirname "$at"); done
[ -d "$at" ] && [ -w "$at" ] || fail "ETCD_SNAPSHOT_DIR=$ETCD_SNAPSHOT_DIR cannot be written"

# The runtime: the one the machine's kubelet has, or one of its own. After a
# reboot the control plane comes back only if the runtime does.
if [ -e "$S/socket" ]; then
  SOCKET=$(cat "$S/socket")
elif [ -e $K/kubelet.conf ]; then
  SOCKET=$(sed -n 's/.*--container-runtime-endpoint=unix:\/\/\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env)
  [ -S "$SOCKET" ] || fail "cannot find the node's container runtime socket"
  unit=$(sed -n 's#.*/\([^/]*\.service\)$#\1#p' "/proc/$(ss -Hxlp | awk -v s="$(readlink -f "$SOCKET")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')/cgroup" | head -1)
  systemctl is-enabled "$unit" >/dev/null 2>&1 || fail "the container runtime of this node ($SOCKET, ${unit:-not a service}) is not started at boot: after a reboot the control plane would not come back. systemctl enable $unit"
  same_cluster || old get node "$node" >/dev/null 2>&1 ||
    fail "the cluster this machine is a node of does not answer its kubelet ($(sed -n 's/^ *server: //p' $K/kubelet.conf)): the node cannot be drained there. Nothing was changed"
fi
stamp "pre-flight passed: $NODE_NAME becomes the control plane at $name ($ADDRESS, Kubernetes $VERSION); the system keeps $SYSTEM_RESERVED_MEMORY and $SYSTEM_RESERVED_CPU CPUs; swap ($(awk 'NR > 1 {sum += $3} END {print int(sum / 1048576)}' /proc/swaps) Gi) stays as it is and pods get none"
if [ "${1:-}" = check ]; then
  if initialized; then echo "Checked only: nothing was changed. This machine is the control plane at $name."
  elif [ -e $K/kubelet.conf ]; then echo "Checked only: nothing was changed. This machine would leave the cluster it is a node of ($(sed -n 's/^ *server: //p' $K/kubelet.conf)) and become the control plane at $name."
  else echo "Checked only: nothing was changed. This machine can become the control plane at $name."
  fi
  exit 0
fi
install -d "$S" "$ETCD_SNAPSHOT_DIR"
echo "$name" > "$S/name"
echo "$ETCD_SNAPSHOT_DIR" > "$S/snapshot-dir"

# --- leave
if { [ -e $K/kubelet.conf ] && [ ! -e "$S/init-started" ]; } || [ -e "$S/leaving" ]; then
  if [ ! -e "$S/leaving" ] && same_cluster; then
    # A worker of the cluster whose control plane it becomes: that control
    # plane is handed over and answers nobody, and the node and its pods are
    # in the snapshot. What is this node's own in its kubelet configuration
    # is in its drop-ins, which stay.
    echo "$SOCKET" > "$S/socket"
    dirname "$kubelet" > "$S/old-bin"
    : > "$S/kubelet-overrides.yaml"
    keep_old_cluster
    touch "$S/leaving" "$S/took-over"
  fi
  if [ ! -e "$S/leaving" ]; then
    echo "$SOCKET" > "$S/socket"
    dirname "$kubelet" > "$S/old-bin"
    # What this node's kubelet configuration has that its cluster's does not
    # (swap handling, a CPU policy) is the machine's own and goes along.
    old -n kube-system get configmap kubelet-config -o jsonpath='{.data.kubelet}' > "$S/old-cluster-kubelet.yaml"
    "$B/station-bootstrap" kubelet-overrides /var/lib/kubelet/config.yaml "$S/old-cluster-kubelet.yaml" > "$S/kubelet-overrides.yaml"
    old cordon "$node" >/dev/null
    # A kubelet may evict the pods of its own node. DaemonSet and static pods
    # end with the node; a pod that will not leave in two minutes does too.
    pods() { old get pods -A --field-selector "spec.nodeName=$node" -o jsonpath='{range .items[*]}{.metadata.namespace} {.metadata.name} {.metadata.ownerReferences[0].kind}{"\n"}{end}' | grep -v -E ' (DaemonSet|Node)$' || true; }
    pods | while read -r namespace pod kind; do
      printf '{"apiVersion":"policy/v1","kind":"Eviction","metadata":{"name":"%s","namespace":"%s"}}' "$pod" "$namespace" |
        old create --raw "/api/v1/namespaces/$namespace/pods/$pod/eviction" -f - >/dev/null 2>&1 || true
    done
    tries=0
    until [ -z "$(pods)" ] || [ $tries -ge 60 ]; do tries=$((tries + 1)); sleep 2; done
    stamp "$node is cordoned and drained in its cluster; $(pods | grep -c . || true) pods would not leave"
    keep_old_cluster
    touch "$S/leaving"
  fi
  systemctl stop kubelet
  kubeadm=$(cat "$S/old-bin")/kubeadm
  [ -x "$kubeadm" ] || kubeadm=$B/bin/kubeadm
  "$kubeadm" reset --force --cri-socket "unix://$SOCKET" > "$S/reset.log" 2>&1 || { tail -20 "$S/reset.log" >&2; fail "kubeadm reset"; }
  rm -rf /etc/cni/net.d/*
  date > "$S/left"
  rm -f "$S/leaving"
  changed=left
  [ ! -e "$S/took-over" ] || stamp "no longer a worker of its cluster, whose control plane it takes over; runtime $SOCKET and its images are kept"
  [ -e "$S/took-over" ] || stamp "left the old cluster; runtime $SOCKET and its images are kept. Its Node object $node stays there, cordoned, until an administrator of that cluster removes it (kubectl delete node $node); undo needs it gone only if the name is given to another machine"
fi

# --- tunnel, runtime, kubelet
install -d /opt/rlark/bin /opt/cni/bin
for binary in "$B"/bin/*; do
  cmp -s "$binary" "/opt/rlark/bin/$(basename "$binary")" || { install -m 755 "$binary" /opt/rlark/bin/; changed=binaries; }
done
[ ! -d "$B/cni" ] || cp -n "$B"/cni/* /opt/cni/bin/
tunnel_service
"$B/bin/station-gateway" --config "$(tunnel_document)" --await-peers ||
  fail "no peer of the site tunnel that this machine dials has answered it: no node would reach a control plane here. station-gateway --config $(tunnel_document) --status"
[ "$(ip -4 -o addr show dev "$TUNNEL_INTERFACE" | awk '{print $4}' | cut -d/ -f1)" = "$ADDRESS" ] || fail "the tunnel service is running, but $TUNNEL_INTERFACE does not hold $ADDRESS: journalctl -u rlark-station-gateway"
kernel
volumes
[ -e "$S/socket" ] || { own_runtime; echo "$SOCKET" > "$S/socket"; }
registry
if [ "$(cat "$S/images" 2>/dev/null)" != "$(cat "$B/tag")" ]; then
  import_images "$B/images"
  import_images "$B/images/control-plane"
  cat "$B/tag" > "$S/images"
  changed=images
  stamp "tunnel up ($ADDRESS on $TUNNEL_INTERFACE, a host service); the images of $VERSION are in the runtime at $SOCKET"
fi
kubelet_unit
preflight=$(cgroup_v1 "$VERSION")
cat > "$B/20-rlark-control-plane.conf" <<CONF
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
systemReserved:
  cpu: "$SYSTEM_RESERVED_CPU"
  memory: $SYSTEM_RESERVED_MEMORY
CONF
if ! cmp -s "$B/20-rlark-control-plane.conf" $K/kubelet.conf.d/20-rlark-control-plane.conf; then
  cp "$B/20-rlark-control-plane.conf" $K/kubelet.conf.d/20-rlark-control-plane.conf
  changed=reservation
fi
# A kubelet that is running reads its unit and its drop-ins when it starts.
case $changed in kubelet|reservation) ! initialized || systemctl restart kubelet ;; esac
hosts_name "$ADDRESS" "$name"
tries=0
# (This machine's own line is the first; the site's line below it may still
# say the machine the control plane was handed over from.)
until [ "$(getent hosts "$name" | awk 'NR == 1 {print $1}')" = "$ADDRESS" ]; do
  tries=$((tries + 1))
  [ $tries -lt 30 ] || fail "$name does not resolve to $ADDRESS on this machine ($(getent hosts "$name" || echo "not at all")): /etc/hosts, journalctl -u rlark-station-gateway"
  sleep 1
done

# --- init
if ! initialized; then
  if [ -e "$S/init-started" ]; then
    stamp "an earlier run stopped inside kubeadm init: starting it over"
    systemctl stop kubelet 2>/dev/null || true
    kubeadm reset --force --cri-socket "unix://$SOCKET" > "$S/reset.log" 2>&1 || { tail -20 "$S/reset.log" >&2; fail "kubeadm reset"; }
    rm -rf /etc/cni/net.d/*
  fi
  rm -rf "$S/patches" && install -d "$S/patches"
  echo "$RESOURCES" | while read -r component cpu cpus_limit memory memory_limit; do
    printf 'spec:\n  containers:\n  - name: %s\n    resources:\n      requests: {cpu: %s, memory: %sMi}\n      limits: {cpu: "%s", memory: %sMi}\n' \
      "$component" "$cpu" "$memory" "$cpus_limit" "$memory_limit" > "$S/patches/$component+strategic.yaml"
  done
  [ ! -s "$S/kubelet-overrides.yaml" ] || cp "$S/kubelet-overrides.yaml" "$S/patches/kubeletconfiguration+merge.yaml"
  # The node is reached at the name and at its tunnel address, and at
  # nothing else: the API server advertises the tunnel address, etcd listens
  # on it, advertises it and is a member at it, and the certificates name
  # these two and the node. kubeadm verifies etcd's certificate for the
  # address etcd advertises on every upgrade; an address of the machine's own
  # network in any of this would tie the control plane to that network.
  cat > "$S/kubeadm.yaml" <<KUBEADM
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint: {advertiseAddress: "$ADDRESS", bindPort: 6443}
nodeRegistration:
  name: "$NODE_NAME"
  criSocket: unix://$SOCKET
  ignorePreflightErrors: [Swap, FileExisting-conntrack, FileExisting-socat${FROM_SNAPSHOT:+, DirAvailable--var-lib-etcd}${preflight:+, $preflight}]
  kubeletExtraArgs:
  - {name: node-ip, value: "$ADDRESS"}
  - {name: node-labels, value: "station.rlark.io/network=host"}
patches: {directory: "$S/patches"}
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: $VERSION
controlPlaneEndpoint: "$name:6443"
networking: {podSubnet: "$POD_SUBNET", serviceSubnet: "$SERVICE_SUBNET"}
apiServer:
  certSANs: ["$name", "$ADDRESS", "$NODE_NAME"]
# Pods that have ended (evicted ones above all) are removed beyond a hundred,
# not beyond Kubernetes' 12500.
controllerManager:
  extraArgs:
  - {name: terminated-pod-gc-threshold, value: "100"}
etcd:
  local:
    serverCertSANs: ["$name", "$ADDRESS"]
    peerCertSANs: ["$name", "$ADDRESS"]
KUBEADM
  touch "$S/init-started"
  if [ -n "${FROM_SNAPSHOT:-}" ]; then
    # The cluster's own CAs and service-account keys: every node's kubelet
    # and every token go on being valid. The certificates are issued anew by
    # kubeadm, for this machine. etcd becomes a cluster of this one member;
    # the revision is raised past anything a client has seen (upgrade.sh, rollback).
    install -d $K && tar -xpf "$FROM_SNAPSHOT/pki.tar" -C $K
    rm -rf /var/lib/etcd
    "$(ctr_of "$SOCKET")" -a "$SOCKET" -n k8s.io run --rm --mount "type=bind,src=/var/lib,dst=/var/lib,options=rbind:rw" --mount "type=bind,src=$FROM_SNAPSHOT,dst=/from,options=rbind:ro" \
      "$(kubeadm config images list --kubernetes-version "$VERSION" 2>/dev/null | grep '/etcd:')" rlark-etcd-restore etcdctl snapshot restore "/from/$(basename "$snapshot")" --data-dir /var/lib/etcd \
      --bump-revision 1000000000 --mark-compacted \
      --name "$NODE_NAME" --initial-cluster "$NODE_NAME=https://$ADDRESS:2380" --initial-advertise-peer-urls "https://$ADDRESS:2380" > "$S/etcd-restore.log" 2>&1 ||
      { tail -20 "$S/etcd-restore.log" >&2; fail "etcd restore from $snapshot"; }
    basename "$snapshot" > "$S/from-snapshot"
    stamp "etcd holds $(basename "$snapshot") of $FROM_SNAPSHOT"
  fi
  kubeadm init --config "$S/kubeadm.yaml" > "$S/init.log" 2>&1 || { tail -25 "$S/init.log" >&2; fail "kubeadm init; its log is $S/init.log. Run the same command again to start it over, or with undo to go back"; }
  # Nodes join with a token Station API makes for each of them; none is left standing.
  for token in $(kubeadm token list -o jsonpath='{.token}{"\t"}{.description}{"\n"}' | grep "generated by 'kubeadm init'" | cut -f1); do
    kubeadm token delete "$token" >/dev/null
  done
  echo "name=$name pods=$POD_SUBNET services=$SERVICE_SUBNET address=$ADDRESS node=$NODE_NAME" > "$S/cluster"
  date > "$S/initialized"
  changed=init
  stamp "kubeadm init done: the API answers at https://$name:6443"
fi
tries=0
until adm get --raw /readyz >/dev/null 2>&1; do
  tries=$((tries + 1))
  [ $tries -lt 150 ] || fail "the API does not answer at https://$name:6443 after 5 min: journalctl -u kubelet"
  sleep 2
done
rm -rf "$HANDOVER"
if [ -e "$S/from-snapshot" ]; then
  # The snapshot's other control-plane nodes are none any more: etcd is this
  # one member. A machine among them that should stay in the cluster is
  # installed as a worker. kube-proxy reaches the API at the name, which its
  # pods resolve with the copy of /etc/hosts they were started with.
  for other in $(adm get nodes -l node-role.kubernetes.io/control-plane -o name | grep -v "^node/$NODE_NAME\$" || true); do
    adm delete "$other" --wait=false >/dev/null
    stamp "${other#node/} was the control plane this one replaces: its Node object is removed"
  done
  # the name got another address, and they were started with a copy of the
  # node's /etc/hosts: started again, they read the file as it is now.
  [ "$changed" != init ] || adm -n kube-system rollout restart daemonset kube-proxy calico-node >/dev/null
fi

# --- the cluster's own pieces: every manifest of the bundle, in name order.
# Calico is the project's manifest, set here to this cluster: the nodes find
# each other at their tunnel addresses, and a pod's MTU fits into the tunnel.
install -d "$S/manifests"
# calico-node and the CNI plugin reach the API at the name, as the kubelet
# and kube-proxy do. Through the Service address, a node's packets would leave
# with the source address of its default route, and robots all have the same.
printf 'apiVersion: v1\nkind: ConfigMap\nmetadata: {name: kubernetes-services-endpoint, namespace: kube-system}\ndata: {KUBERNETES_SERVICE_HOST: "%s", KUBERNETES_SERVICE_PORT: "6443"}\n' "$name" > "$S/manifests/00-api-endpoint.yaml"
adm apply -f "$S/manifests/00-api-endpoint.yaml" > "$S/apply.log" 2>&1 || { tail -20 "$S/apply.log" >&2; fail "apply the API endpoint of the CNI"; }
# A manifest is applied when it is not the one this machine applied last.
for manifest in "$B"/manifests/*.yaml; do
  [ -e "$manifest" ] || continue
  applied=$S/manifests/$(basename "$manifest")
  rendered=$applied.new
  if [ "$(basename "$manifest")" = calico.yaml ]; then
    sed -e "s#veth_mtu: \"0\"#veth_mtu: \"$POD_MTU\"#" \
      -e "s#^\( *\)- name: CALICO_IPV4POOL_IPIP#\1- name: IP_AUTODETECTION_METHOD\n\1  value: \"interface=$TUNNEL_INTERFACE\"\n\1- name: CALICO_IPV4POOL_CIDR\n\1  value: \"$POD_SUBNET\"\n\1- name: FELIX_IGNORELOOSERPF\n\1  value: \"true\"\n&#" \
      "$manifest" > "$rendered"
  else
    cp "$manifest" "$rendered"
  fi
  if ! cmp -s "$rendered" "$applied"; then
    adm apply -f "$rendered" > "$S/apply.log" 2>&1 || { tail -20 "$S/apply.log" >&2; fail "apply $(basename "$manifest")"; }
    mv "$rendered" "$applied"
    changed=manifests
    stamp "applied $(basename "$manifest")"
  fi
  rm -f "$rendered"
done
tries=0
until [ "$(adm get node "$NODE_NAME" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)" = True ]; do
  tries=$((tries + 1))
  [ $tries -lt 150 ] || fail "the node is not Ready after 5 min: kubectl --kubeconfig $K/admin.conf describe node $NODE_NAME"
  sleep 2
done

# --- a snapshot of etcd every day, and the keys a control plane is rebuilt
# from beside it (FROM_SNAPSHOT on the machine that takes this one's place).
cat > "$B/rlark-etcd-snapshot" <<SNAPSHOT
#!/bin/sh
# Written by control-plane.sh. A snapshot of this control plane's etcd into
# $ETCD_SNAPSHOT_DIR and, beside it, the cluster's CAs and service-account
# keys (pki.tar, root only): what another machine needs to take this one's
# place. The newest $ETCD_SNAPSHOT_KEEP snapshots stay.
set -eu
umask 077
P=/etc/kubernetes/pki
# Asked of etcd itself, not through the API: the last snapshot of a control
# plane that is handed over is taken when its API has stopped.
ctr="$(ctr_of "$SOCKET") -a $SOCKET -n k8s.io"
\$ctr run --rm --net-host --mount type=bind,src=\$P/etcd,dst=/pki,options=rbind:ro --mount type=bind,src=/var/lib/etcd,dst=/var/lib/etcd,options=rbind:rw \\
  "\$(\$ctr images ls -q | grep '/etcd:' | sort -V | tail -1)" rlark-etcd-snapshot etcdctl --endpoints https://127.0.0.1:2379 \\
  --cacert /pki/ca.crt --cert /pki/server.crt --key /pki/server.key snapshot save /var/lib/etcd/rlark-snapshot.db > "$ETCD_SNAPSHOT_DIR/snapshot.log" 2>&1 || { cat "$ETCD_SNAPSHOT_DIR/snapshot.log" >&2; exit 1; }
at=\$(date +%Y%m%d-%H%M%S)
mv /var/lib/etcd/rlark-snapshot.db "$ETCD_SNAPSHOT_DIR/etcd-\$at.db.part"
mv "$ETCD_SNAPSHOT_DIR/etcd-\$at.db.part" "$ETCD_SNAPSHOT_DIR/etcd-\$at.db"
tar -cpf "$ETCD_SNAPSHOT_DIR/pki.tar.part" -C /etc/kubernetes pki/ca.crt pki/ca.key pki/sa.key pki/sa.pub pki/front-proxy-ca.crt pki/front-proxy-ca.key pki/etcd/ca.crt pki/etcd/ca.key
mv "$ETCD_SNAPSHOT_DIR/pki.tar.part" "$ETCD_SNAPSHOT_DIR/pki.tar"
ls -1t "$ETCD_SNAPSHOT_DIR"/etcd-*.db | tail -n +$(( ETCD_SNAPSHOT_KEEP + 1 )) | xargs -r rm -f
echo "etcd-\$at.db \$(du -h "$ETCD_SNAPSHOT_DIR/etcd-\$at.db" | cut -f1)"
SNAPSHOT
printf '[Unit]\nDescription=etcd snapshot of this control plane into %s\n[Service]\nType=oneshot\nExecStart=/opt/rlark/bin/rlark-etcd-snapshot\n' "$ETCD_SNAPSHOT_DIR" > "$B/rlark-etcd-snapshot.service"
# Persistent: a machine that was off at four takes the snapshot when it is on again.
printf '[Unit]\nDescription=Daily etcd snapshot of this control plane\n[Timer]\nOnCalendar=*-*-* 04:00:00\nPersistent=true\n[Install]\nWantedBy=timers.target\n' > "$B/rlark-etcd-snapshot.timer"
if ! cmp -s "$B/rlark-etcd-snapshot" /opt/rlark/bin/rlark-etcd-snapshot || ! cmp -s "$B/rlark-etcd-snapshot.service" /etc/systemd/system/rlark-etcd-snapshot.service ||
  ! cmp -s "$B/rlark-etcd-snapshot.timer" /etc/systemd/system/rlark-etcd-snapshot.timer || ! systemctl is-enabled rlark-etcd-snapshot.timer >/dev/null 2>&1; then
  install -m 755 "$B/rlark-etcd-snapshot" /opt/rlark/bin/rlark-etcd-snapshot
  cp "$B/rlark-etcd-snapshot.service" "$B/rlark-etcd-snapshot.timer" /etc/systemd/system/
  systemctl daemon-reload
  systemctl enable --now rlark-etcd-snapshot.timer >/dev/null 2>&1
  systemctl start rlark-etcd-snapshot.service || fail "the first etcd snapshot: journalctl -u rlark-etcd-snapshot"
  changed=snapshots
  stamp "etcd snapshot every day at 04:00 into $ETCD_SNAPSHOT_DIR; the first: $(ls -1t "$ETCD_SNAPSHOT_DIR"/etcd-*.db | head -1)"
fi

# --- addresses: what would tie this control plane to the network the machine
# stands in now.
held=$("$B/station-bootstrap" cert-addresses $K/pki $(machine_addresses))
objects=$( { cat $K/*.conf $K/manifests/*.yaml /var/lib/kubelet/kubeadm-flags.env
  adm -n kube-system get configmap kubeadm-config kube-proxy kubernetes-services-endpoint -o yaml
  adm -n kube-public get configmap cluster-info -o yaml
  adm get endpoints kubernetes -o yaml
  adm get node "$NODE_NAME" -o jsonpath='{.status.addresses}'
  adm -n kube-system exec "etcd-$NODE_NAME" -- etcdctl --endpoints https://127.0.0.1:2379 --cacert $K/pki/etcd/ca.crt --cert $K/pki/etcd/server.crt --key $K/pki/etcd/server.key member list; } 2>&1)
for address in $(machine_addresses); do
  ! echo "$objects" | grep -q -w -F "$address" || held="$held
a kubeconfig, a static pod, etcd's members or an object of the cluster holds $address"
done
[ -z "$held" ] || fail "this control plane holds an address of the network the machine stands in, and would not survive the machine's move to another:
$held"

# --- annotations
SETTINGS="control-plane name=$name pods=$POD_SUBNET services=$SERVICE_SUBNET interface=$TUNNEL_INTERFACE mtu=$POD_MTU reserved=$SYSTEM_RESERVED_CPU/$SYSTEM_RESERVED_MEMORY workloads=$WORKLOADS snapshots=$ETCD_SNAPSHOT_DIR/$ETCD_SNAPSHOT_KEEP"
spec=$(cat "$B/tag").$(printf '%s' "$SETTINGS" | sha256sum | cut -c1-12)
if [ "$(kubectl --kubeconfig $K/kubelet.conf get node "$NODE_NAME" -o jsonpath='{.metadata.annotations.rlark\.io/node-install-spec} {.metadata.annotations.rlark\.io/node-install-state}')" != "$spec ok" ]; then
  node_install "$SETTINGS" ok
  changed=annotations
fi
# The cluster's agent registers with the management plane the nodes that
# carry this label; a control plane is one of them. Not its category: a
# node class's addons choose their nodes by that.
if [ "$(adm get node "$NODE_NAME" -o jsonpath='{.metadata.labels.rlark\.io/enabled}')" != true ]; then
  adm label node "$NODE_NAME" --overwrite rlark.io/enabled=true >/dev/null
  changed=label
fi
# kubeadm taints a control plane; a machine that runs its cluster's work has it taken off, and one that does not keeps it.
tainted=$(adm get node "$NODE_NAME" -o jsonpath='{.spec.taints[?(@.key=="node-role.kubernetes.io/control-plane")].effect}')
if [ "$WORKLOADS" = yes ] && [ -n "$tainted" ]; then
  adm taint node "$NODE_NAME" node-role.kubernetes.io/control-plane- >/dev/null
  changed=taint
elif [ "$WORKLOADS" != yes ] && [ -z "$tainted" ]; then
  adm taint node "$NODE_NAME" node-role.kubernetes.io/control-plane:NoSchedule >/dev/null
  changed=taint
fi
rm -rf "$B/images"
if [ -z "$changed" ]; then
  echo "Nothing to do: this machine is the control plane at https://$name:6443 (Kubernetes $VERSION, node $NODE_NAME, installed from $spec). Nothing was changed."
else
  echo "Done: this machine is the control plane at https://$name:6443 (node $NODE_NAME, installed from $spec)."
  echo "A node joins at $name:6443 (NodeAdmission, apiServer) and resolves the name to $ADDRESS by its site's names (SiteNetwork, names)."
fi
