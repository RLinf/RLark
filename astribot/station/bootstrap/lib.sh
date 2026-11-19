# What install.sh (a machine becomes a worker) and control-plane.sh (a machine
# becomes a control plane) do the same way. Sourced, not run.
B=/var/lib/rlark/bootstrap
started=$(date +%s)
stamp() { echo "$(date +%T) +$(( $(date +%s) - started ))s $*"; }
fail() { echo "$(date +%T) FAILED: $*" >&2; exit 1; }

# hosts_name ADDRESS NAME: /etc/hosts holds the one line
# "ADDRESS NAME # rlark control-plane"; without an address it holds none.
# This is a control plane's own name on its own machine, which must resolve
# there before any site network declares it and whatever one declares later.
# The names a site declares are the tunnel service's lines ("# rlark names").
# The file keeps its place and is never empty in between: it is written over
# from its start, then cut to the new length.
hosts_name() {
  # Where the line stands among the others is not this function's: the
  # tunnel service puts its own lines last whenever it writes them.
  [ "$(grep ' # rlark control-plane$' /etc/hosts)" != "${1:+$1 $2 # rlark control-plane}" ] || return 0
  { grep -v ' # rlark control-plane$' /etc/hosts || true; [ -z "$1" ] || printf '%s %s # rlark control-plane\n' "$1" "$2"; } > "$B/hosts"
  dd if="$B/hosts" of=/etc/hosts conv=notrunc 2>/dev/null && truncate -s "$(wc -c < "$B/hosts")" /etc/hosts
  changed=hosts
}

# ctr_of SOCKET: the ctr beside the containerd that serves the socket. A
# machine's containerd need not be in the PATH, nor be the only one.
ctr_of() {
  echo "$(dirname "$(readlink "/proc/$(ss -Hxlp | awk -v s="$(readlink -f "$1")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')/exe")")/ctr"
}

# node_address HOST: the address this machine registers its node with: the
# one it reaches its cluster's API from (HOST, a name of the site's names or
# an address). A cluster whose nodes meet on a network of their own (an office
# LAN, a cloud VPC) has its API there, and the node is known on that network;
# a cluster whose control plane is a name of the tunnel has every node known
# by its tunnel address. A node known by another address than its peers'
# answers them from where they do not expect it.
node_address() {
  target=$(getent ahostsv4 "$1" | awk 'NR == 1 {print $1}')
  [ -n "$target" ] || fail "this machine cannot resolve $1, its cluster's API"
  ip -4 route get "$target" | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -1
}

# tunnel_of_before ADDRESS: a machine that was a node before, under another
# tunnel address, still has the interface of that time (a node removed keeps
# it; so does one whose join stopped half way). station-gateway never moves an
# interface to another address: it refuses the document of this join, and the
# join would go on over the tunnel of before, at an address the site network
# no longer gives this machine. The interface of before goes; the tunnel
# service makes this join's.
tunnel_of_before() {
  before=$(ip -4 -o addr show dev "$TUNNEL_INTERFACE" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -1)
  [ -n "$before" ] && [ "$before" != "$1" ] || return 0
  systemctl stop rlark-station-gateway 2>/dev/null || true
  ip link del "$TUNNEL_INTERFACE" || fail "the tunnel interface $TUNNEL_INTERFACE of an earlier join ($before) cannot be removed"
  stamp "the tunnel interface of an earlier join ($before) is removed: this join's address is $1"
}

# tunnel_address_is ADDRESS: the tunnel runs on the document of this join.
tunnel_address_is() {
  ip -4 -o addr show dev "$TUNNEL_INTERFACE" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | grep -qx "$1" ||
    fail "$TUNNEL_INTERFACE does not hold this join's tunnel address $1 ($(ip -4 -o addr show dev "$TUNNEL_INTERFACE" 2>/dev/null | awk '{print $4}' | tr '\n' ' ')): journalctl -u rlark-station-gateway says why the document was not taken"
}

# keep_old_cluster: what makes this machine a node of the cluster it is in,
# kept in $OLD before it leaves; restore_old_cluster puts it back and starts
# the old kubelet with its old identity.
keep_old_cluster() {
  rm -rf "$OLD" && install -d "$OLD/kubelet" "$OLD/units"
  cp -a /etc/kubernetes "$OLD/etc-kubernetes"
  cp -a /var/lib/kubelet/config.yaml /var/lib/kubelet/kubeadm-flags.env /var/lib/kubelet/pki "$OLD/kubelet/"
  [ ! -e /etc/systemd/system/kubelet.service ] || cp -a /etc/systemd/system/kubelet.service "$OLD/units/"
  [ ! -d /etc/systemd/system/kubelet.service.d ] || cp -a /etc/systemd/system/kubelet.service.d "$OLD/units/"
  [ ! -d /etc/cni/net.d ] || cp -a /etc/cni/net.d "$OLD/cni-net.d"
  ! systemctl is-enabled rlark-station-gateway >/dev/null 2>&1 || touch "$OLD/had-host-gateway"
}
restore_old_cluster() {
  rm -f /etc/systemd/system/kubelet.service /etc/systemd/system/kubelet.service.d/zz-rlark.conf
  [ ! -e "$OLD/units/kubelet.service" ] || cp -a "$OLD/units/kubelet.service" /etc/systemd/system/
  [ ! -d "$OLD/units/kubelet.service.d" ] || { rm -rf /etc/systemd/system/kubelet.service.d && cp -a "$OLD/units/kubelet.service.d" /etc/systemd/system/; }
  rm -rf /etc/kubernetes && cp -a "$OLD/etc-kubernetes" /etc/kubernetes
  install -d /var/lib/kubelet && cp -a "$OLD/kubelet/." /var/lib/kubelet/
  # The directory itself stays: the container runtime watches it, and would
  # never see what is written into a new one in its place.
  [ ! -d "$OLD/cni-net.d" ] || { install -d /etc/cni/net.d && rm -rf /etc/cni/net.d/* && cp -a "$OLD/cni-net.d/." /etc/cni/net.d/; }
  # The old cluster's own gateway pod takes the tunnel again.
  [ -e "$OLD/had-host-gateway" ] || systemctl disable --now rlark-station-gateway >/dev/null 2>&1 || true
  systemctl daemon-reload
  systemctl start kubelet
}

# tunnel_service: the site tunnel as a host service, started before the
# kubelet on every boot. It applies the configuration the agent stores once
# the node has enrolled, the last one it applied when the machine boots or
# when the pod that ran the tunnel is gone, and the administrator's document
# until then; the names its document holds it keeps in /etc/hosts, the name
# servers (the site's, else the machine's own) in
# /etc/rlark/station/resolv.conf. A service that is running as written is left alone.
tunnel_service() {
  install -d /etc/rlark/station
  cat > "$B/rlark-station-gateway.service" <<UNIT
[Unit]
Description=RLark site-node tunnel
After=network-online.target
Wants=network-online.target
Before=kubelet.service

[Service]
ExecStartPre=/bin/sh -c 'mkdir -p /run/station-network /run/station-observation && chown 65532 /run/station-network'
ExecStart=/opt/rlark/bin/station-gateway --config /etc/rlark/station/bootstrap.json --computed-config /run/station-network/config.json --identity-file /run/station-observation/identity.json --observation-file /run/station-observation/wireguard.json --prune-routes --hosts-file /etc/hosts --resolv-file /etc/rlark/station/resolv.conf
ExecStartPost=/bin/sh -c 'for i in \$(seq 100); do ip -4 addr show dev $TUNNEL_INTERFACE 2>/dev/null | grep -q inet && exit 0; sleep 0.2; done; exit 1'
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
UNIT
  if cmp -s "$B/rlark-station-gateway.service" /etc/systemd/system/rlark-station-gateway.service &&
    systemctl is-enabled rlark-station-gateway >/dev/null 2>&1 && systemctl is-active rlark-station-gateway >/dev/null 2>&1; then return 0; fi
  cp "$B/rlark-station-gateway.service" /etc/systemd/system/rlark-station-gateway.service
  systemctl daemon-reload
  systemctl enable rlark-station-gateway >/dev/null 2>&1
  systemctl restart rlark-station-gateway || fail "the tunnel service did not start: journalctl -u rlark-station-gateway"
  # Its first pass also writes the names and the name servers, which the next steps read.
  for try in $(seq 100); do [ -e /etc/rlark/station/resolv.conf ] && break; sleep 0.2; done
  changed=tunnel
}

# kernel: the modules and settings a node's pod network and its pods need,
# now and at every boot, whatever container runtime the node has.
kernel() {
  for module in overlay br_netfilter; do modprobe $module || fail "the kernel has no $module module"; done
  printf '%s\n' overlay br_netfilter wireguard > /etc/modules-load.d/rlark.conf
  # inotify instances and watches are counted for the whole machine, all its
  # pods together: the kernel's 128 instances are used up by some tens of
  # pods ("too many open files"). A machine's higher limit is kept.
  {
    printf '%s\n' 'net.ipv4.ip_forward = 1' 'net.bridge.bridge-nf-call-iptables = 1'
    for limit in max_user_instances=1024 max_user_watches=524288; do
      held=$(sysctl -n "fs.inotify.${limit%=*}")
      [ "$held" -gt "${limit#*=}" ] || held=${limit#*=}
      echo "fs.inotify.${limit%=*} = $held"
    done
  } > /etc/sysctl.d/90-rlark.conf
  sysctl -q -p /etc/sysctl.d/90-rlark.conf
}

# own_runtime: a containerd of its own for the kubelet, the one the bundle
# carries (containerd, its shim, ctr and runc in /opt/rlark/bin): every node
# runs the runtime its installer has, whatever the machine came with, and a
# machine needs none. The machine's docker and its containerd are not
# touched, nor restarted.
own_runtime() {
  SOCKET=/run/containerd-fleet/containerd.sock
  [ ! -S $SOCKET ] || ! systemctl is-enabled containerd-fleet >/dev/null 2>&1 || return 0
  install -d /etc/containerd-fleet
  /opt/rlark/bin/containerd config default | sed -e 's#/var/lib/containerd#/var/lib/containerd-fleet#g' -e 's#/run/containerd#/run/containerd-fleet#g' \
    -e 's#SystemdCgroup = false#SystemdCgroup = true#' -e "s#registry.k8s.io/pause:[0-9.]*#$(cat "$B"/images/pause.ref)#" \
    -e "s#^\\( *config_path = \\).*#\\1'/etc/containerd-fleet/certs.d'#" \
    > /etc/containerd-fleet/config.toml
  cat > /etc/systemd/system/containerd-fleet.service <<UNIT
[Unit]
Description=containerd for the RLark kubelet
After=network.target

[Service]
# The shim and runc are found beside it.
Environment=PATH=/opt/rlark/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/opt/rlark/bin/containerd --config /etc/containerd-fleet/config.toml
Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
LimitNOFILE=1048576
TasksMax=infinity

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now containerd-fleet >/dev/null 2>&1 || fail "containerd-fleet did not start: journalctl -u containerd-fleet"
  changed=runtime
}

# registry: the site's registry, as the runtime the kubelet will use sees it.
# The bundle's certs.d is the first trust this machine has in it, put where
# that runtime reads it (the site's addon writes the same there later). Then
# the runtime asks the registry for an image without a credential: "not
# authorized" is the answer of a registry that was reached and is trusted. A
# site whose network names no registry (SiteNetwork, names) has none to ask.
REGISTRY=registry.rlark.internal:5443
registry() {
  CTR=$(ctr_of "$SOCKET")
  pid=$(ss -Hxlp | awk -v s="$(readlink -f "$SOCKET")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')
  config=$(tr '\0' '\n' < "/proc/$pid/cmdline" | sed -n '/^--config$/{n;p}')
  hosts=$("$(dirname "$CTR")/containerd" ${config:+--config "$config"} config dump | sed -n "s/^ *config_path = ['\"]\(.*\)['\"]\$/\1/p" | head -1)
  [ ! -d "$B"/certs.d ] || [ -z "$hosts" ] || { install -d "$hosts" && cp -r "$B"/certs.d/. "$hosts"/; }
  grep -q "[[:space:]]${REGISTRY%:*} # rlark names\$" /etc/hosts || return 0
  answer=$("$CTR" -a "$SOCKET" -n k8s.io images pull ${hosts:+--hosts-dir "$hosts"} "$REGISTRY/rlark/probe:none" 2>&1 | tail -1 || true)
  case $answer in
  *401*|*nauthorized*|*"authorization failed"*|*"access denied"*) ;;
  *x509*|*certificate*) fail "the container runtime at $SOCKET does not trust the site's registry $REGISTRY (it reads its registries' certificates in ${hosts:-no directory: config_path is not set in ${config:-/etc/containerd/config.toml}}): $answer" ;;
  *) fail "the container runtime at $SOCKET does not reach the site's registry $REGISTRY ($(getent hosts "${REGISTRY%:*}" | awk '{print $1}')): $answer" ;;
  esac
}

# import_images DIR: the archives a node needs before it can pull anything:
# DIR's, and those of DIR/<os>-<arch> for this machine's platform (the images
# the installer carries by their names, rlarkctl release archive).
import_images() {
  CTR=$(ctr_of "$SOCKET")
  [ -x "$CTR" ] || fail "cannot find ctr beside the containerd of $SOCKET"
  # This machine's platform alone: an archive saved from an image store that
  # keeps an image's index names other platforms and attestations whose
  # content it does not hold, and an import of everything fails for them.
  case $(uname -m) in aarch64) platform=linux/arm64 ;; *) platform=linux/amd64 ;; esac
  platform_dir=$(echo "$platform" | tr / -)
  # They are pinned: a kubelet under disk pressure removes the images no
  # container uses, and these are what the node cannot do without and, with
  # its registry out of reach, cannot get again.
  for archive in "$1"/*.tar "$1/$platform_dir"/*.tar; do
    [ -e "$archive" ] || continue
    "$CTR" -a "$SOCKET" -n k8s.io images import --platform $platform "$archive" > "$B/import.log" || fail "import $archive"
    for image in $(sed -n 's/^unpacking \([^ ]*\) .*/\1/p' "$B/import.log"); do
      "$CTR" -a "$SOCKET" -n k8s.io images label "$image" io.cri-containerd.pinned=pinned >/dev/null
    done
  done
}

# kubelet_settings: what every node's kubelet is told, as a drop-in. Swap
# stays on for the machine's own software; pods get none. Images are pulled
# four at a time: one after the other, a node that lost its images (a disk
# that was full) waits for the largest before it has the smallest.
kubelet_settings() {
  cat > "$B/10-rlark.conf" <<CONF
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
failSwapOn: false
memorySwap:
  swapBehavior: NoSwap
serializeImagePulls: false
maxParallelImagePulls: 4
CONF
  cmp -s "$B/10-rlark.conf" /etc/kubernetes/kubelet.conf.d/10-rlark.conf || { cp "$B/10-rlark.conf" /etc/kubernetes/kubelet.conf.d/10-rlark.conf; changed=kubelet; }
}

# disk: the kubelet evicts pods and removes images when the filesystem of its
# data or its runtime's has less than 15 % free. A machine that starts
# below 20 % is one step from it, and its node would not hold what the
# installer gives it. The node's volumes still to be made are taken from it
# first; where the kubelet's data and its runtime's are both in volumes, the
# 15 % are counted in them, and this filesystem only has to hold the volumes.
disk() {
  need=0
  for volume in $(echo "${NODE_VOLUMES:-}" | tr , ' '); do
    [ -e "$(volume_image "$volume")" ] || need=$((need + $(volume_bytes "$volume") / 1024))
    path=$(volume_path "$volume")
    mountpoint -q "$path" || [ -z "$(ls -A "$path" 2>/dev/null)" ] ||
      fail "$path holds data and is not the volume ${volume%%=*}: the data of a node is moved into its volumes with node-volumes.sh, the node stopped meanwhile"
  done
  free=$(df -P -k /var/lib | awk -v need=$need 'NR == 2 {print int(($4 - need) * 100 / $2)}')
  at=$(df -P /var/lib | awk 'NR == 2 {print $6}')
  floor=20
  echo ",${NODE_VOLUMES:-}," | grep -q ':/var/lib/kubelet,' && echo ",${NODE_VOLUMES:-}," | grep -q ':/var/lib/containerd-fleet,' && floor=0
  [ "$free" -ge $floor ] ||
    fail "the filesystem of /var/lib ($at) has $free % free${NODE_VOLUMES:+ once the node's volumes are made ($NODE_VOLUMES)}: a kubelet evicts its pods and removes their images below 15 %, and the installer asks for 20. Free space on it, or give the kubelet's data and its runtime's volumes of their own (NODE_VOLUMES)"
  [ $floor = 0 ] || [ "$at" != "$(df -P /home | awk 'NR == 2 {print $6}')" ] ||
    stamp "note: /var/lib shares its filesystem ($at, $free % free) with /home: what people and builds put there counts against the node's 15 %"
}

# volumes: the node's volumes, NODE_VOLUMES="NAME=SIZE:PATH,..." (SIZE as
# fallocate reads it: 80G). Each is a file of that size on the machine's disk,
# /var/lib/rlark/volumes/NAME.img, with a filesystem of its own, mounted at
# PATH on every boot. Its blocks are taken when it is made: what fills the
# machine's disk later (builds, images of the machine's docker, people's
# files) does not fill it, and the kubelet counts what is in it apart from the
# machine's disk. A volume is never resized, nor removed when it is no longer
# declared: that is a person's decision, and the file is at a fixed path.
volumes() {
  for volume in $(echo "${NODE_VOLUMES:-}" | tr , ' '); do
    volume_make "$volume"
    volume_mount "$volume"
  done
}
volume_bytes() { numfmt --from=iec "$(echo "${1#*=}" | cut -d: -f1)"; }
volume_image() { echo "/var/lib/rlark/volumes/${1%%=*}.img"; }
volume_path() { echo "${1#*:}"; }
volume_unit() { systemd-escape --path --suffix=mount "$(volume_path "$1")"; }
# volume_make VOLUME: its file with a filesystem, where there is none.
volume_make() {
  image=$(volume_image "$1")
  if [ -e "$image" ]; then
    [ "$(stat -c %s "$image")" = "$(volume_bytes "$1")" ] ||
      fail "the volume ${1%%=*} is $(stat -c %s "$image" | numfmt --to=iec) on this machine, not the declared $(echo "${1#*=}" | cut -d: -f1): a volume is not resized"
    return 0
  fi
  install -d /var/lib/rlark/volumes
  fallocate -l "$(volume_bytes "$1")" "$image.new" || fail "make the volume ${1%%=*} in /var/lib/rlark/volumes"
  mkfs.ext4 -q -F -m 0 -L "${1%%=*}" "$image.new" || fail "make a filesystem in the volume ${1%%=*}"
  # Empty, as the directory it takes the place of: lost+found is made again
  # by e2fsck when it needs one.
  debugfs -w -R 'rmdir lost+found' "$image.new" >/dev/null 2>&1
  mv "$image.new" "$image"
  stamp "volume ${1%%=*}: $(echo "${1#*=}" | cut -d: -f1) in $image"
}
# volume_mount VOLUME: mounted at its path, now and on every boot (disk has
# refused a path that holds data of its own; volumes.sh moves such data in).
volume_mount() {
  path=$(volume_path "$1") unit=$(volume_unit "$1")
  printf '[Unit]\nDescription=RLark node volume %s\n\n[Mount]\nWhat=%s\nWhere=%s\nType=ext4\nOptions=loop\n\n[Install]\nWantedBy=local-fs.target\n' \
    "${1%%=*}" "$(volume_image "$1")" "$path" > "$B/$unit"
  cmp -s "$B/$unit" "/etc/systemd/system/$unit" || { cp "$B/$unit" "/etc/systemd/system/$unit"; systemctl daemon-reload; }
  systemctl enable "$unit" >/dev/null 2>&1
  mountpoint -q "$path" && return 0
  install -d "$path"
  systemctl start "$unit" || fail "mount the volume ${1%%=*} at $path: journalctl -u $unit"
}

# cgroup_v1 VERSION: from 1.35 a kubelet does not start on cgroup v1 unless
# told, and kubeadm's system verification fails there by design. On a machine
# still on v1 (a robot before its node profile's restart into v2) the kubelet
# of VERSION is told so by a drop-in, and the check kubeadm is to skip is
# printed (for ignorePreflightErrors). On v2 the drop-in does nothing;
# s1u-robot's node profile removes it there (kubelet's ExecStartPre).
CGROUP_V1_CONF=/etc/kubernetes/kubelet.conf.d/90-rlark-cgroup-v1.conf
cgroup_v1() {
  [ "$(stat -fc %T /sys/fs/cgroup)" != cgroup2fs ] && [ "$(echo "$1" | sed -E 's/^v1\.([0-9]+)\..*/\1/')" -ge 35 ] || return 0
  install -d "${CGROUP_V1_CONF%/*}"
  printf 'apiVersion: kubelet.config.k8s.io/v1beta1\nkind: KubeletConfiguration\nfailCgroupV1: false\n' > "$CGROUP_V1_CONF"
  echo SystemVerification
}

# kubelet_unit: the kubelet of the bundle as a service that reads drop-ins
# from /etc/kubernetes/kubelet.conf.d.
kubelet_unit() {
  install -d /etc/kubernetes/kubelet.conf.d /etc/systemd/system/kubelet.service.d
  kubelet_settings
  # Pods get the name servers the tunnel service keeps: the site's where
  # its network declares them (SiteNetwork, sites, dns), else the machine's
  # own. A robot's computer has none of its own to give.
  dns=/etc/kubernetes/kubelet.conf.d/15-rlark-dns.conf
  [ -e $dns ] || { printf 'apiVersion: kubelet.config.k8s.io/v1beta1\nkind: KubeletConfiguration\nresolvConf: /etc/rlark/station/resolv.conf\n' > $dns; changed=kubelet; }
  cat > "$B/kubelet.service" <<UNIT
[Unit]
Description=RLark kubelet
After=network-online.target containerd-fleet.service rlark-station-gateway.service
Wants=network-online.target rlark-station-gateway.service

[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml --config-dir=/etc/kubernetes/kubelet.conf.d"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=/opt/rlark/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
  # A distribution's or an earlier installation's drop-in may set ExecStart; this one is read last.
  cat > "$B/zz-rlark.conf" <<UNIT
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml --config-dir=/etc/kubernetes/kubelet.conf.d"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=
ExecStart=/opt/rlark/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS
UNIT
  if ! cmp -s "$B/kubelet.service" /etc/systemd/system/kubelet.service || ! cmp -s "$B/zz-rlark.conf" /etc/systemd/system/kubelet.service.d/zz-rlark.conf; then
    cp "$B/kubelet.service" /etc/systemd/system/kubelet.service
    cp "$B/zz-rlark.conf" /etc/systemd/system/kubelet.service.d/zz-rlark.conf
    systemctl daemon-reload
    changed=kubelet
  fi
  systemctl enable kubelet >/dev/null 2>&1
}

# node_install SETTINGS STATE: the node's pair of annotations. The spec is
# what the node was installed from: the bundle's tag and a digest of the site
# settings the installer was given (the same on every node of a kind, so
# nothing of one node is among them); the state is "ok" or a short reason.
# Written with the kubelet's own credentials. The digest is kept on the node
# for the upgrade tool, which changes the bundle and not the settings.
node_install() {
  install -d /etc/rlark
  printf '%s' "$1" | sha256sum | cut -c1-12 > /etc/rlark/node-install
  kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$NODE_NAME" --overwrite \
    "rlark.io/node-install-spec=$(cat "$B/tag").$(cat /etc/rlark/node-install)" "rlark.io/node-install-state=$2" "rlark.io/node-volumes=${NODE_VOLUMES:-}" >/dev/null
}
