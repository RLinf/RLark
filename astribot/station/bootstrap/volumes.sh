#!/bin/sh
# Moves the data of a node into its volumes (NODE_VOLUMES, lib.sh volumes):
# the kubelet's (/var/lib/kubelet), its container runtime's, a site cache's.
# A node made before its volumes were declared holds that data on the
# machine's disk. The node is stopped while the data is copied, as for a
# reboot: the kubelet, and every container of its runtime; on a control
# plane the API with them. Its pods start again on it as after a reboot.
# node-volumes.sh runs it from a Job on the node (RLARK_DETACH: it ends that
# Job's own pod).
#   volumes.sh          the data into the volumes; what was there stays beside, PATH.before-volume
#   volumes.sh undo     back to the data as it was (what was written since into the volumes is lost)
#   volumes.sh confirm  PATH.before-volume removed
set -eu
. /var/lib/rlark/bootstrap/lib.sh
export PATH=/opt/rlark/bin:$PATH
NODE=${NODE_NAME:-$(hostname | tr '[:upper:]' '[:lower:]')}
SOCKET=$(sed -n 's/.*--container-runtime-endpoint=unix:\/\/\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env)
[ -S "$SOCKET" ] || fail "cannot find the node's container runtime socket"
CTR=$(ctr_of "$SOCKET")
RUNTIME=$(ps -o unit= -p "$(ss -Hxlp | awk -v s="$(readlink -f "$SOCKET")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')")
annotate() { kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$NODE" --overwrite "rlark.io/node-volumes=$1" >/dev/null 2>&1 || true; }
# The Job that started this has ended: the node says how it went.
trap '[ $? = 0 ] || annotate failed' EXIT

# stop: the kubelet, then every container of the runtime (a runtime that stops
# leaves its containers running), what pods had mounted, then the runtime.
stop() {
  systemctl stop kubelet
  for task in $("$CTR" -a "$SOCKET" -n k8s.io tasks ls -q); do "$CTR" -a "$SOCKET" -n k8s.io tasks delete --force "$task" >/dev/null 2>&1 || true; done
  for path in $(echo "$NODE_VOLUMES" | tr , '\n' | sed 's/^[^:]*://'); do
    findmnt -rn -o TARGET | grep "^$path/" | sort -r | xargs -r -n 1 umount -l
  done
  systemctl stop "$RUNTIME"
  stamp "node stopped: kubelet, $(echo "$RUNTIME" | sed 's/.service$//') and its containers"
}
# start: the runtime and the kubelet, until the node reports Ready anew.
start() {
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  systemctl start "$RUNTIME" kubelet
  tries=0
  until [ "$(kubectl --kubeconfig /etc/kubernetes/kubelet.conf get node "$NODE" -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status} {.lastHeartbeatTime}{end}' 2>/dev/null |
    awk -v since="$since" '$1 == "True" && $2 >= since')" ]; do
    tries=$((tries + 1))
    [ $tries -lt 150 ] || fail "the node is not Ready 5 min after its kubelet started: journalctl -u kubelet"
    sleep 2
  done
  stamp "node Ready"
}

# back: each path that was moved holds its data as it was again; the volume
# and its copy go. The node is stopped.
back() {
  for volume in $(echo "$NODE_VOLUMES" | tr , ' '); do
    path=$(volume_path "$volume")
    [ -d "$path.before-volume" ] || continue
    systemctl disable --now "$(volume_unit "$volume")" >/dev/null 2>&1 || true
    rm -f "/etc/systemd/system/$(volume_unit "$volume")" "$(volume_image "$volume")"
    rmdir "$path" && mv "$path.before-volume" "$path"
    stamp "$path: the data as it was, on the machine's disk"
  done
  ! mountpoint -q /var/lib/rlark/volumes/.copy || umount /var/lib/rlark/volumes/.copy
  systemctl daemon-reload
}

case ${1:-} in
undo)
  stop
  back
  start
  annotate undone
  exit 0 ;;
confirm)
  for volume in $(echo "$NODE_VOLUMES" | tr , ' '); do
    path=$(volume_path "$volume")
    mountpoint -q "$path" || fail "$path is not the volume ${volume%%=*}: nothing removed"
    [ ! -d "$path.before-volume" ] || { rm -rf "$path.before-volume"; stamp "$path.before-volume removed"; }
  done
  exit 0 ;;
esac

# Before anything is stopped: each volume takes what its path holds, with the
# 20 % that the installer leaves the kubelet, and the machine's disk the volumes.
# A node whose paths are all volumes already is not stopped.
need=0 moving=
for volume in $(echo "$NODE_VOLUMES" | tr , ' '); do
  path=$(volume_path "$volume")
  mountpoint -q "$path" && continue
  moving=yes
  held=$(du -skx "$path" 2>/dev/null | cut -f1) held=${held:-0}
  [ $((held * 100)) -le $(($(volume_bytes "$volume") / 1024 * 80)) ] ||
    fail "$path holds $(numfmt --to=iec $((held * 1024))): the volume ${volume%%=*} of $(echo "${volume#*=}" | cut -d: -f1) would be more than 80 % full"
  [ -e "$(volume_image "$volume")" ] || need=$((need + $(volume_bytes "$volume") / 1024))
done
avail=$(df -P -k /var/lib/rlark | awk 'NR == 2 {print $4}')
[ $need -lt "$avail" ] || fail "the machine's disk has $(numfmt --to=iec $((avail * 1024))) free, the volumes take $(numfmt --to=iec $((need * 1024)))"
[ -n "$moving" ] || { annotate "$NODE_VOLUMES"; echo "every path is its volume already: nothing to do"; exit 0; }

annotate moving
# A failure from here on puts the node back as it was and starts it.
recover() {
  code=$?
  [ $code != 0 ] || return 0
  trap - EXIT
  echo "$(date +%T) the move failed; the node goes back to its data as it was" >&2
  back
  start
  annotate "failed; back to the data as it was"
  exit $code
}
trap recover EXIT
stop
for volume in $(echo "$NODE_VOLUMES" | tr , ' '); do
  path=$(volume_path "$volume")
  mountpoint -q "$path" && continue
  volume_make "$volume"
  install -d /var/lib/rlark/volumes/.copy
  mount -o loop "$(volume_image "$volume")" /var/lib/rlark/volumes/.copy
  t0=$(date +%s)
  [ ! -d "$path" ] || cp -a "$path/." /var/lib/rlark/volumes/.copy/ || fail "copy $path into the volume ${volume%%=*}"
  umount /var/lib/rlark/volumes/.copy
  [ ! -d "$path" ] || mv "$path" "$path.before-volume"
  volume_mount "$volume"
  stamp "$path: $(du -sh "$path" | cut -f1) in the volume ${volume%%=*}, copied in $(( $(date +%s) - t0 )) s"
done
start
annotate "$NODE_VOLUMES"
echo "Done. The data as it was stays beside each path (PATH.before-volume) until \"volumes confirm\"."
