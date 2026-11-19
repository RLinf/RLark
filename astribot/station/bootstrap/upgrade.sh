#!/bin/sh
# Upgrades this node's Kubernetes by one minor version; station-bootstrap runs
# it as a host process from a Job that upgrade-cluster.sh pins to the node.
#   PHASE=certs     on a control-plane node, before anything else: the API
#                   server's certificate and etcd's two must name every
#                   address the node is reached at now. kubeadm reaches etcd
#                   at the address etcd advertises and verifies etcd's
#                   certificate for it, so on a node whose address has changed
#                   since the cluster was made "kubeadm upgrade apply" fails
#                   with etcd already replaced. Certificates that lack a name
#                   are issued anew by kubeadm from the cluster's CA, and etcd
#                   and the API server started again: the API is away for ten
#                   seconds with etcd and then until the kubelet has started
#                   the API server again: a minute, more where the kubelet
#                   holds a container back that was started not long ago. No
#                   other container on any node restarts.
#                   Nothing is done on a node whose certificates are complete.
#   PHASE=images    import the version's images: on every node before the
#                   first is upgraded, because the control plane's upgrade
#                   rolls kube-proxy and CoreDNS out to all of them at once
#   PHASE=control   import the version's images, then "kubeadm upgrade apply"
#                   on a control-plane node or "kubeadm upgrade node" elsewhere
#   PHASE=runtime   on a drained node, before its kubelet: the kubelet runs on
#                   the runtime the installer gives every node, containerd-fleet
#                   with the bundle's containerd (kubelet 1.36 runs on no
#                   containerd 1.x). A runtime as new as the bundle's
#                   containerd or newer is kept. Where containerd-fleet runs an
#                   older one, the bundle's is put in its place and the service
#                   restarted: its containers go on running. Where the kubelet
#                   uses another runtime (a robot's containerd-robot, a
#                   machine's own containerd), containerd-fleet is set up beside
#                   it with the node's images, and a unit of the host switches
#                   the kubelet over 10 s after this phase ended: the old
#                   runtime's Kubernetes containers, this phase's pod among them,
#                   are stopped and the kubelet starts every pod anew on
#                   containerd-fleet. The old runtime's unit is disabled unless
#                   it holds containers of its own or an active docker runs on
#                   it. A kubelet that is not healthy 2 min after the switch is put
#                   back on the old runtime by the same unit. Its log:
#                   /var/lib/rlark/kube-upgrade/runtime-switch.log
#   PHASE=kubelet   replace kubelet and kubectl and restart the kubelet. A
#                   watchdog is set on the node first: a systemd timer that
#                   puts the old kubelet and its configuration back
#                   WATCHDOG_SECONDS (600) later, whatever has become of the
#                   upgrade's session, the API or the new kubelet
#   PHASE=confirm   the node runs KUBE_VERSION and the API has seen it Ready:
#                   remove the watchdog. Nothing else keeps the new kubelet.
#   PHASE=rollback  put the node back to KUBE_VERSION, the version it had
#                   before its last upgrade: binaries, configuration, and on a
#                   control-plane node the static pods and etcd's content of
#                   the snapshot taken before that upgrade. What was written to
#                   the cluster since the snapshot is lost. On a control-plane
#                   node the API is down, so the image is started on the node:
#                   docker run --rm --privileged --pid=host --net=host -e PHASE=rollback -e KUBE_VERSION=... IMAGE
# The node's own layout is read from the node: where its kubelet binary is and
# which containerd it uses. What it replaces is kept under /var/backups.
set -eu
B=/var/lib/rlark/bootstrap
. "$B/lib.sh"
stamp() { printf '%s\n' "$(date +%T) $*"; }
fail() { printf '%s\n' "$(date +%T) FAILED: $*" >&2; exit 1; }
BIN=$(dirname "$(systemctl show kubelet -p ExecStart --value | sed -n 's/.*path=\([^ ;]*kubelet\) .*/\1/p')")
[ -x "$BIN/kubelet" ] || fail "cannot find the kubelet binary of kubelet.service"
# The kubelet's runtime: its flag, or where kubeadm 1.34 on keeps it
# (NodeLocalCRISocket: instance-config.yaml, merged into config.yaml).
SOCKET=$({ sed -n 's/.*--container-runtime-endpoint=unix:\/\/\([^ "]*\).*/\1/p' /var/lib/kubelet/kubeadm-flags.env
  sed -n 's#^containerRuntimeEndpoint: *"\{0,1\}unix://\([^"]*\)"\{0,1\} *$#\1#p' /var/lib/kubelet/config.yaml /var/lib/kubelet/instance-config.yaml 2>/dev/null; } | head -1)
[ -S "$SOCKET" ] || fail "cannot find the node's container runtime socket"
# ctr is the one beside the containerd that serves that socket: a node's
# containerd need not be in the PATH, nor be the only one on the node.
CTR=$(dirname "$(readlink "/proc/$(ss -Hxlp | awk -v s="$(readlink -f "$SOCKET")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')/exe")")/ctr
[ -x "$CTR" ] || fail "cannot find ctr beside the containerd of $SOCKET"
from=$("$BIN/kubelet" --version | awk '{print $2}')
keep=/var/backups/rlark-kube-upgrade/$from
[ "$PHASE" = rollback ] || install -d "$keep"
# kubectl and kubeadm are the node's own where it has them; a node joined by
# the installer has the kubelet alone, and uses the bundle's.
export PATH="$BIN:$PATH:$B/bin"
WATCHDOG=rlark-kube-upgrade-watchdog
WATCH=/var/lib/rlark/kube-upgrade
disarm() {
  systemctl disable --now $WATCHDOG.timer >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/$WATCHDOG.timer /etc/systemd/system/$WATCHDOG.service "$WATCH/restore.sh"
  systemctl daemon-reload
}
# A kubelet checks for swap only when it starts. On a node where swap was
# turned on under a running kubelet that is not configured for it, no kubelet
# starts again, the old one included: nothing is changed on such a node.
if [ "$PHASE" != rollback ] && [ "$(wc -l < /proc/swaps)" -gt 1 ] &&
  ! grep -rqs -E '^failSwapOn: *false' /var/lib/kubelet/config.yaml /etc/kubernetes/kubelet.conf.d &&
  ! grep -qs -e '--fail-swap-on=false' /var/lib/kubelet/kubeadm-flags.env /etc/default/kubelet /etc/sysconfig/kubelet; then
  fail "swap is on ($(sed -n 2p /proc/swaps | cut -d' ' -f1)) and this node's kubelet is not configured for it (failSwapOn: false): a restarted kubelet would exit at once. Turn the swap off or declare failSwapOn: false for the node, then upgrade it"
fi

# The node's name: upgrade-cluster.sh names it; run by hand, the kubelet's
# flags or the hostname do.
node=${NODE_NAME:-$(cat /var/lib/kubelet/kubeadm-flags.env /etc/default/kubelet /etc/sysconfig/kubelet 2>/dev/null | sed -n 's/.*--hostname-override=\([^ "]*\).*/\1/p' | head -1)}
[ -n "$node" ] || node=$(hostname | tr '[:upper:]' '[:lower:]')

case $PHASE in
certs)
  M=/etc/kubernetes/manifests P=/etc/kubernetes/pki
  [ -e $M/kube-apiserver.yaml ] || { stamp "not a control-plane node: no certificates to look at"; exit 0; }
  admin() { kubectl --kubeconfig /etc/kubernetes/admin.conf -n kube-system "$@"; }
  etcdctl() { admin exec "etcd-$node" -- etcdctl --endpoints https://127.0.0.1:2379 --cacert $P/etcd/ca.crt --cert $P/etcd/server.crt --key $P/etcd/server.key "$@"; }
  # What the node is reached at: where its API server and its etcd say they
  # are (the manifests are what an address change is made in), and its
  # address in the station's tunnel, which no kubeadm configuration names.
  endpoint=$(sed -n 's/^ *kubeadm.kubernetes.io\/kube-apiserver.advertise-address.endpoint: *//p' $M/kube-apiserver.yaml)
  client=$(sed -n 's/^ *kubeadm.kubernetes.io\/etcd.advertise-client-urls: *//p' $M/etcd.yaml)
  peer=$(sed -n 's/^ *- --initial-advertise-peer-urls=//p' $M/etcd.yaml)
  tunnel=$(ip -4 -o addr show dev st-wg0 2>/dev/null | awk '{print $4}' | cut -d/ -f1)
  install -d "$WATCH"
  admin get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' > "$WATCH/ClusterConfiguration.before.yaml"
  lacking=$("$B/station-bootstrap" cert-sans "$WATCH/ClusterConfiguration.before.yaml" $P "${endpoint%:*}" $(echo "$client,$peer" | tr , '\n' | sed 's#^https://##; s#:[0-9]*$##') $tunnel 2>&1 > "$WATCH/ClusterConfiguration") || fail "$lacking"
  if [ -n "$lacking" ]; then
    echo "$lacking" | while read -r line; do stamp "$line"; done
    dir=/var/backups/rlark-kube-upgrade/certs-$(date +%Y%m%d%H%M%S)
    install -d -m 700 "$dir" "$dir/replaced"
    # Without kubeadm's own copies of etcd from earlier upgrades.
    (umask 077 && tar -cpf "$dir/etc-kubernetes.tar" -C / --exclude=etc/kubernetes/tmp etc/kubernetes)
    etcdctl snapshot save /var/lib/etcd/rlark-snapshot-before-certs.db > "$dir/etcd-snapshot.log" 2>&1 || { tail -5 "$dir/etcd-snapshot.log" >&2; fail "etcd snapshot"; }
    mv /var/lib/etcd/rlark-snapshot-before-certs.db "$dir/etcd-snapshot.db"
    cp "$WATCH/ClusterConfiguration.before.yaml" "$dir/"
    # kubeadm would take the node's name and address from the hostname and the
    # default route; they are the ones the node has in the cluster.
    { printf 'apiVersion: %s\nkind: InitConfiguration\nlocalAPIEndpoint: {advertiseAddress: "%s", bindPort: %s}\nnodeRegistration: {name: %s, criSocket: "unix://%s"}\n---\n' \
        "$(sed -n 's/^apiVersion: //p' "$WATCH/ClusterConfiguration")" "${endpoint%:*}" "${endpoint##*:}" "$node" "$SOCKET"; cat "$WATCH/ClusterConfiguration"; } > "$dir/kubeadm.yaml"
    # Only the certificates are taken out of the archive on the way back: the
    # kubelet makes every static pod anew whose manifest is written again.
    pid() { ss -Hltnp "sport = :$1" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1; }
    undo="tar -xpf $dir/etc-kubernetes.tar -C / etc/kubernetes/pki && kill \$(ss -Hltnp 'sport = :${client##*:}' | sed -n 's/.*pid=\\([0-9]*\\).*/\\1/p' | head -1) \$(ss -Hltnp 'sport = :${endpoint##*:}' | sed -n 's/.*pid=\\([0-9]*\\).*/\\1/p' | head -1)"
    stamp "kept in $dir: /etc/kubernetes (etc-kubernetes.tar), etcd (etcd-snapshot.db), the cluster's kubeadm configuration (ClusterConfiguration.before.yaml)"
    stamp "to put the old certificates back, as root on this node (the kubelet starts etcd and the API server again, the API is away for about a minute; the names stay in the kubeadm configuration): $undo"
    for crt in $(echo "$lacking" | cut -d' ' -f1); do
      # kubeadm keeps a certificate it finds. etcd reads its certificate from
      # the file for every connection: no failure may leave the node without it.
      mv "$P/$crt" "$P/${crt%.crt}.key" "$dir/replaced/"
      kubeadm init phase certs "$(echo "${crt%.crt}" | tr / -)" --config "$dir/kubeadm.yaml" > "$dir/kubeadm-$(basename "$crt").log" 2>&1 ||
        { tar -xpf "$dir/etc-kubernetes.tar" -C / etc/kubernetes/pki; tail -15 "$dir/kubeadm-$(basename "$crt").log" >&2; fail "kubeadm init phase certs for $crt; the old certificates are back"; }
    done
    stamp "issued anew by kubeadm $(kubeadm version -o short): $(echo "$lacking" | cut -d' ' -f1 | tr '\n' ' ')"
    # The names go into the cluster's kubeadm configuration: kubeadm issues a
    # certificate from it whenever it makes one anew.
    admin create configmap kubeadm-config --from-file="ClusterConfiguration=$WATCH/ClusterConfiguration" --dry-run=client -o yaml | admin replace -f - >/dev/null
    # A static pod's container is started again by the kubelet when its
    # process ends, at once or, one that was started not long ago, up to five
    # minutes later; etcd first, the API server does not come up without it.
    t0=$(date +%s)
    old=$(pid "${client##*:}"); kill "$old"
    while kill -0 "$old" 2>/dev/null; do sleep 1; done
    tries=0
    until curl -fsS -m 2 --cacert $P/etcd/ca.crt --cert $P/etcd/healthcheck-client.crt --key $P/etcd/healthcheck-client.key "$client/health" >/dev/null 2>&1; do
      tries=$((tries + 1))
      [ $tries -lt 330 ] || fail "etcd does not answer at $client 330 s after it was stopped to load the new certificates. Back: $undo"
      sleep 1
    done
    stamp "etcd answers at $client, the address it advertises, $(( $(date +%s) - t0 )) s after it was stopped"
    t0=$(date +%s)
    # The API server closes its port at once and then waits a minute for
    # its watchers to leave: it is not waited for.
    old=$(pid "${endpoint##*:}"); kill "$old"
    sleep 3
    kill -9 "$old" 2>/dev/null || true
    tries=0
    until curl -fsS -m 2 --cacert $P/ca.crt "https://$endpoint/readyz" >/dev/null 2>&1; do
      tries=$((tries + 1))
      [ $tries -lt 330 ] || fail "the API server is not ready at $endpoint 330 s after it was stopped to load the new certificate. Back: $undo"
      sleep 1
    done
    stamp "the API server is ready at $endpoint $(( $(date +%s) - t0 )) s after it was stopped"
  fi
  # A member that etcd knows at another peer address than the one it is
  # started with (the address of the day the cluster was made) is one that a
  # member joining later could not reach.
  member=$(etcdctl member list | grep ", $(sed -n 's/^ *- --name=//p' $M/etcd.yaml), ")
  registered=$(echo "$member" | cut -d, -f4 | tr -d ' ')
  if [ "$registered" != "$peer" ]; then
    etcdctl member update "${member%%,*}" --peer-urls="$peer" >/dev/null
    stamp "etcd member ${member%%,*}: peer address $registered -> $peer"
  fi
  [ -n "$lacking" ] || [ "$registered" != "$peer" ] || stamp "the certificates name every address of this node: nothing to do"
  ;;
images|control)
  # The control plane's images on every node: CoreDNS, which the control
  # plane's upgrade replaces, may be placed on any of them, and a node may
  # reach no registry that has it (a robot).
  import_images "$B/images"
  [ ! -d "$B/images/control-plane" ] || import_images "$B/images/control-plane"
  stamp "images of $KUBE_VERSION imported and pinned"
  [ "$PHASE" = control ] || exit 0
  [ -e "$keep/kubeadm" ] || cp -a "$BIN/kubeadm" "$keep/kubeadm"
  [ -d "$keep/etc-kubernetes" ] || cp -a /etc/kubernetes "$keep/etc-kubernetes"
  [ -d "$keep/kubelet" ] || { install -d "$keep/kubelet" && cp -a /var/lib/kubelet/config.yaml /var/lib/kubelet/kubeadm-flags.env "$keep/kubelet/"; }
  # upgrade-cluster.sh took this snapshot just before it started this phase.
  # A run that goes on after a failed one finds the control plane at
  # KUBE_VERSION already: its snapshot is not of the state before the upgrade,
  # and the one that is stays.
  snapshot=/var/lib/etcd/rlark-snapshot-before-$KUBE_VERSION.db
  if [ -e "$snapshot" ]; then
    if grep -q "kube-apiserver:$KUBE_VERSION\$" /etc/kubernetes/manifests/kube-apiserver.yaml; then rm -f "$snapshot"; else mv "$snapshot" "$keep/etcd-snapshot.db"; fi
  fi
  # What this node's kubelet configuration has that the cluster's does not is
  # carried across: kubeadm writes the file anew from the cluster's.
  patches=
  if [ ! -e "$keep/kubelet-overrides.yaml" ]; then
    kubectl --kubeconfig /etc/kubernetes/kubelet.conf -n kube-system get configmap kubelet-config -o jsonpath='{.data.kubelet}' > "$keep/cluster-kubelet.yaml"
    "$B/station-bootstrap" kubelet-overrides /var/lib/kubelet/config.yaml "$keep/cluster-kubelet.yaml" > "$keep/kubelet-overrides.yaml"
  fi
  if [ -s "$keep/kubelet-overrides.yaml" ]; then
    install -d "$keep/patches"
    cp "$keep/kubelet-overrides.yaml" "$keep/patches/kubeletconfiguration+merge.yaml"
    patches="--patches $keep/patches"
    stamp "kept from this node's kubelet configuration: $(grep -v -E '^(apiVersion|kind):' "$keep/kubelet-overrides.yaml" | grep -E '^[a-zA-Z]' | cut -d: -f1 | tr '\n' ' ')"
  fi
  install -m 755 "$B/bin/kubeadm" "$BIN/kubeadm"
  preflight=$(cgroup_v1 "$KUBE_VERSION")
  if [ -e /etc/kubernetes/manifests/kube-apiserver.yaml ]; then
    # What a control plane made by this bundle is given (control-plane.sh),
    # one made before gets here: kubeadm writes the manifests from it.
    admin() { kubectl --kubeconfig /etc/kubernetes/admin.conf -n kube-system "$@"; }
    admin get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' > "$keep/ClusterConfiguration.before.yaml"
    if ! grep -q terminated-pod-gc-threshold "$keep/ClusterConfiguration.before.yaml"; then
      "$B/station-bootstrap" controller-manager-arg "$keep/ClusterConfiguration.before.yaml" terminated-pod-gc-threshold 100 > "$keep/ClusterConfiguration"
      admin create configmap kubeadm-config --from-file="ClusterConfiguration=$keep/ClusterConfiguration" --dry-run=client -o yaml | admin replace -f - >/dev/null
    fi
    if [ "$(sed -n 's/^ *image: .*\/kube-\(apiserver\|controller-manager\|scheduler\):\(.*\)$/\2/p' /etc/kubernetes/manifests/kube-*.yaml | sort -u)" = "$KUBE_VERSION" ]; then
      # At the version it runs, the control plane is not upgraded again: that
      # renews every certificate and starts etcd and the API server anew (the
      # API is away for a minute and more). What the cluster's configuration
      # says of the controller manager is written into its manifest alone, and
      # the kubelet starts it anew if that changed it.
      { printf 'apiVersion: kubeadm.k8s.io/v1beta4\nkind: InitConfiguration\n---\n'; admin get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}'; } > "$keep/kubeadm.yaml"
      kubeadm init phase control-plane controller-manager --config "$keep/kubeadm.yaml" > "$keep/controller-manager.log" 2>&1 || { tail -15 "$keep/controller-manager.log" >&2; fail "kubeadm init phase control-plane controller-manager"; }
      stamp "control plane at $KUBE_VERSION already: the controller manager's manifest written from the cluster's configuration"
    else
      kubeadm upgrade apply "$KUBE_VERSION" --yes $patches ${preflight:+--ignore-preflight-errors=$preflight} > "$keep/upgrade-apply.log" 2>&1 || { tail -25 "$keep/upgrade-apply.log" >&2; fail "kubeadm upgrade apply"; }
      stamp "control plane at $KUBE_VERSION"
    fi
  else
    # kubeadm reads the node's runtime socket from an annotation of the Node,
    # which a node that was not joined by kubeadm join alone may lack.
    [ -n "$(kubectl --kubeconfig /etc/kubernetes/kubelet.conf get node "$node" -o jsonpath='{.metadata.annotations.kubeadm\.alpha\.kubernetes\.io/cri-socket}')" ] ||
      kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$node" "kubeadm.alpha.kubernetes.io/cri-socket=unix://$SOCKET" >/dev/null
    kubeadm upgrade node $patches ${preflight:+--ignore-preflight-errors=$preflight} > "$keep/upgrade-node.log" 2>&1 || { tail -25 "$keep/upgrade-node.log" >&2; fail "kubeadm upgrade node"; }
    stamp "node configuration at $KUBE_VERSION"
  fi
  ;;
runtime)
  bundled=$("$B/bin/containerd" --version | awk '{print $3}' | sed 's/^v//')
  fleet=/run/containerd-fleet/containerd.sock
  pid=$(ss -Hxlp | awk -v s="$(readlink -f "$SOCKET")" '$5 == s' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')
  running=$("/proc/$pid/exe" --version | awk '{print $3}' | sed 's/^v//')
  # A runtime as new as the bundle's or newer is not replaced by it.
  if [ "$bundled" = "$running" ] || [ "$(printf '%s\n%s\n' "$bundled" "$running" | sort -V | tail -1)" != "$bundled" ]; then
    stamp "the kubelet runs on containerd $running ($SOCKET), not older than the bundle's $bundled: kept"; exit 0
  fi
  # A machine's own containerd 2 stays: it may hold what the machine needs of
  # it (the NVIDIA runtime of a GPU node), and the kubelet runs on it.
  if [ "$SOCKET" != $fleet ] && [ "${running%%.*}" -ge 2 ]; then
    stamp "the kubelet runs on the machine's containerd $running ($SOCKET): kept"; exit 0
  fi
  install -d /opt/rlark/bin
  for binary in containerd containerd-shim-runc-v2 ctr runc; do
    install -m 755 "$B/bin/$binary" "/opt/rlark/bin/$binary.new" && mv "/opt/rlark/bin/$binary.new" "/opt/rlark/bin/$binary"
  done
  if [ "$SOCKET" = $fleet ]; then
    # KillMode=process: the shims, and with them the containers, go on.
    systemctl restart containerd-fleet
    tries=0
    until /opt/rlark/bin/ctr -a $fleet version >/dev/null 2>&1; do
      tries=$((tries + 1)); [ $tries -lt 60 ] || fail "containerd-fleet does not answer 2 min after its restart: journalctl -u containerd-fleet"; sleep 2
    done
    stamp "containerd-fleet $running -> $bundled; its containers went on running"; exit 0
  fi
  old=$SOCKET old_ctr=$CTR
  old_unit=$(ps -o unit= -p "$pid" | sed 's/\.service$//')
  # What the old runtime's unit was given (a robot's CPU affinity) goes with
  # its successor, before containerd-fleet first starts.
  if [ -d "/etc/systemd/system/$old_unit.service.d" ]; then
    install -d /etc/systemd/system/containerd-fleet.service.d
    cp -a "/etc/systemd/system/$old_unit.service.d/." /etc/systemd/system/containerd-fleet.service.d/
  fi
  old_config=$(tr '\0' '\n' < "/proc/$pid/cmdline" | sed -n '/^--config$/{n;p}')
  old_hosts=$("/proc/$pid/exe" ${old_config:+--config "$old_config"} config dump | sed -n "s/^ *config_path = ['\"]\([^'\":]*\).*/\1/p" | head -1)
  # The registries the old runtime trusts (registry-trust wrote them), the
  # bundle's own entry kept; then the node's images, and the images of the
  # Kubernetes containers it runs now, so that nothing is pulled again.
  (own_runtime
   install -d /etc/containerd-fleet/certs.d
   [ -z "$old_hosts" ] || [ ! -d "$old_hosts" ] || cp -rn "$old_hosts"/. /etc/containerd-fleet/certs.d/
   registry; import_images "$B/images"
   [ ! -d "$B/images/control-plane" ] || import_images "$B/images/control-plane"
   # One at a time: an image whose content the old runtime no longer holds
   # whole (layers it discarded once unpacked) is pulled again instead.
   for ref in $("$old_ctr" -a "$old" -n k8s.io containers ls | awk 'NR > 1 {print $2}' | grep -v '^sha256:' | sort -u); do
     if "$old_ctr" -a "$old" -n k8s.io images export --platform linux/amd64 "$B/node-image.tar" "$ref" 2>/dev/null &&
       /opt/rlark/bin/ctr -a $fleet -n k8s.io images import --platform linux/amd64 "$B/node-image.tar" >/dev/null; then
       stamp "carried over: $ref"
     else
       stamp "not carried over (pulled again by the kubelet): $ref"
     fi
     rm -f "$B/node-image.tar"
   done)
  stamp "containerd-fleet $bundled set up beside $old_unit ($running), with the node's images"
  install -d "$WATCH"
  rm -f "$WATCH/runtime-switched"
  # The machine's own containers (docker's namespace moby, or any but
  # Kubernetes') keep their runtime running, and so does an active docker
  # whose unit wants or needs the old runtime's (its containerd.service),
  # whether it runs a container now or not. A runtime of its own beside
  # docker's (a robot's containerd-robot) is disabled.
  others=$("$old_ctr" -a "$old" namespaces ls -q | grep -v -x k8s.io | while read -r namespace; do
    [ -z "$("$old_ctr" -a "$old" -n "$namespace" containers ls -q)" ] || echo "$namespace"; done)
  if [ -n "$others" ]; then
    retire="stamp 'the kubelet runs on containerd-fleet $bundled; $old_unit goes on for the containers of $(echo $others)'"
  elif systemctl is-active --quiet docker &&
    systemctl show -p Wants,Requires,BindsTo --value docker | tr ' ' '\n' | grep -qx "$old_unit.service"; then
    retire="stamp 'the kubelet runs on containerd-fleet $bundled; $old_unit goes on for docker'"
  else
    retire="systemctl disable --now $old_unit >/dev/null 2>&1; stamp 'the kubelet runs on containerd-fleet $bundled; $old_unit disabled'"
  fi
  cat > "$WATCH/runtime-switch.sh" <<SWITCH
#!/bin/sh
# Written by upgrade.sh (PHASE=runtime): the kubelet of $node from $old_unit ($old) to containerd-fleet $bundled.
set -u
export PATH="$PATH"
stamp() { printf '%s\\n' "\$(date +%T) \$*"; }
cp -a /var/lib/kubelet/kubeadm-flags.env /var/lib/kubelet/config.yaml "$keep/"
[ ! -e /var/lib/kubelet/instance-config.yaml ] || cp -a /var/lib/kubelet/instance-config.yaml "$keep/"
systemctl stop kubelet
sed -i 's#unix://$old#unix://$fleet#' /var/lib/kubelet/kubeadm-flags.env
sed -i 's#^containerRuntimeEndpoint:.*#containerRuntimeEndpoint: unix://$fleet#' /var/lib/kubelet/config.yaml
[ ! -e /var/lib/kubelet/instance-config.yaml ] || sed -i 's#^containerRuntimeEndpoint:.*#containerRuntimeEndpoint: "unix://$fleet"#' /var/lib/kubelet/instance-config.yaml
for id in \$("$old_ctr" -a "$old" -n k8s.io tasks ls -q); do "$old_ctr" -a "$old" -n k8s.io tasks kill -a -s SIGKILL "\$id" 2>/dev/null; done
sleep 2
for id in \$("$old_ctr" -a "$old" -n k8s.io containers ls -q); do "$old_ctr" -a "$old" -n k8s.io tasks delete -f "\$id" >/dev/null 2>&1; "$old_ctr" -a "$old" -n k8s.io containers delete "\$id" >/dev/null 2>&1; done
stamp "the Kubernetes containers of $old_unit stopped"
systemctl start kubelet
for try in \$(seq 60); do
  if curl -fsS -m 2 http://127.0.0.1:10248/healthz >/dev/null 2>&1; then
    kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$node" --overwrite "kubeadm.alpha.kubernetes.io/cri-socket=unix://$fleet" >/dev/null || stamp "could not set the cri-socket annotation of $node"
    # The way back to the old kubelet (watchdog, rollback) stays on containerd-fleet.
    sed -i 's#unix://$old#unix://$fleet#' "$keep/kubelet/kubeadm-flags.env"
    sed -i 's#^containerRuntimeEndpoint:.*#containerRuntimeEndpoint: unix://$fleet#' "$keep/kubelet/config.yaml"
    $retire
    date > "$WATCH/runtime-switched"
    exit 0
  fi
  sleep 2
done
journalctl -u kubelet --no-pager --since "-3min" > "$keep/kubelet-runtime-failed.log" 2>&1
systemctl stop kubelet
cp -a "$keep/kubeadm-flags.env" "$keep/config.yaml" /var/lib/kubelet/
[ ! -e "$keep/instance-config.yaml" ] || cp -a "$keep/instance-config.yaml" /var/lib/kubelet/
systemctl start kubelet
stamp "the kubelet was not healthy on containerd-fleet 2 min after its start ($keep/kubelet-runtime-failed.log): back on $old_unit"
exit 1
SWITCH
  systemctl reset-failed rlark-runtime-switch >/dev/null 2>&1 || true
  systemd-run --unit rlark-runtime-switch --on-active=10 --timer-property=AccuracySec=1 \
    /bin/sh -c "exec /bin/sh $WATCH/runtime-switch.sh > $WATCH/runtime-switch.log 2>&1" >/dev/null
  stamp "the kubelet will run on containerd://$bundled: the switch starts in 10 s ($WATCH/runtime-switch.log)"
  ;;
kubelet)
  [ "$from" != "$KUBE_VERSION" ] || { stamp "the kubelet is already $KUBE_VERSION"; exit 0; }
  # kubectl is replaced where the node has one; none is given to a node without.
  [ -e "$keep/bin/kubelet" ] || { install -d "$keep/bin" && cp -a "$BIN/kubelet" "$keep/bin/" && { [ ! -e "$BIN/kubectl" ] || cp -a "$BIN/kubectl" "$keep/bin/"; }; }
  install -m 755 "$B/bin/kubelet" "$BIN/kubelet.new" && mv "$BIN/kubelet.new" "$BIN/kubelet"
  preflight=$(cgroup_v1 "$KUBE_VERSION")
  [ -z "$preflight" ] || stamp "this node runs cgroup v1: the kubelet of $KUBE_VERSION is told to start on it ($CGROUP_V1_CONF)"
  minor=$(echo "$KUBE_VERSION" | sed -E 's/^v1\.([0-9]+)\..*/\1/')
  # kubelet 1.35 has no --pod-infra-container-image; kubeadm 1.35 on does not
  # take it out of a node's flags.
  if [ "$minor" -ge 35 ]; then
    for file in /var/lib/kubelet/kubeadm-flags.env /etc/default/kubelet /etc/sysconfig/kubelet; do
      [ ! -e "$file" ] || sed -i 's/ *--pod-infra-container-image=[^ "]*//' "$file"
    done
  fi
  [ ! -e "$keep/bin/kubectl" ] || { install -m 755 "$B/bin/kubectl" "$BIN/kubectl.new" && mv "$BIN/kubectl.new" "$BIN/kubectl"; }
  # The watchdog: the way back needs neither the kubelet, nor the API, nor
  # this process. It survives a reboot (the timer then counts anew).
  install -d "$WATCH"
  rm -f "$WATCH/restored"
  cat > "$WATCH/restore.sh" <<RESTORE
#!/bin/sh
# Written by upgrade.sh before the kubelet of $KUBE_VERSION was started: back to $from.
journalctl -u kubelet --no-pager --since "-5min" > "$keep/kubelet-failed.log" 2>&1
install -m 755 "$keep/bin/kubelet" "$BIN/kubelet.old" && mv "$BIN/kubelet.old" "$BIN/kubelet"
[ ! -e "$keep/bin/kubectl" ] || { install -m 755 "$keep/bin/kubectl" "$BIN/kubectl.old" && mv "$BIN/kubectl.old" "$BIN/kubectl"; }
cp -a "$keep/kubelet/config.yaml" "$keep/kubelet/kubeadm-flags.env" /var/lib/kubelet/
systemctl disable --now $WATCHDOG.timer
systemctl restart kubelet
# Once, and the outcome is left here: nothing tries again.
state="NOT healthy 2 min after its start either (journalctl -u kubelet): the node needs a person"
for try in \$(seq 60); do
  if curl -fsS -m 2 http://127.0.0.1:10248/healthz >/dev/null 2>&1; then state=healthy; break; fi
  sleep 2
done
date "+%F %T the kubelet of $KUBE_VERSION was not confirmed; back on $from, whose kubelet is \$state" > "$WATCH/restored"
[ "\$state" = healthy ]
RESTORE
  printf '[Unit]\nDescription=Back to the kubelet of %s unless the upgrade to %s is confirmed\n[Service]\nType=oneshot\nExecStart=/bin/sh %s/restore.sh\n' "$from" "$KUBE_VERSION" "$WATCH" > /etc/systemd/system/$WATCHDOG.service
  printf '[Timer]\nOnActiveSec=%s\nAccuracySec=1\n[Install]\nWantedBy=timers.target\n' "${WATCHDOG_SECONDS:-600}" > /etc/systemd/system/$WATCHDOG.timer
  systemctl daemon-reload
  systemctl enable $WATCHDOG.timer >/dev/null 2>&1
  systemctl restart $WATCHDOG.timer
  stamp "watchdog set: back on $from in ${WATCHDOG_SECONDS:-600} s unless confirmed"
  systemctl restart kubelet
  tries=0
  until curl -fsS -m 2 http://127.0.0.1:10248/healthz >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ $tries -ge 60 ]; then
      # No reason to wait for the timer: the same way back, now.
      systemctl start $WATCHDOG.service || true
      [ ! -e /etc/rlark/node-install ] || kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$node" --overwrite \
        "rlark.io/node-install-state=the kubelet of $KUBE_VERSION did not start; back on $from" >/dev/null 2>&1 || true
      grep -v -E 'Failed to ensure lease|Unable to register node|node .* not found' "$keep/kubelet-failed.log" | tail -15 >&2
      fail "the kubelet of $KUBE_VERSION was not healthy 2 min after its start; its log is $keep/kubelet-failed.log. $(cat "$WATCH/restored")"
    fi
    sleep 2
  done
  stamp "kubelet $("$BIN/kubelet" --version | awk '{print $2}') healthy"
  ;;
confirm)
  [ "$from" = "$KUBE_VERSION" ] || fail "the node runs $from, not $KUBE_VERSION: $(cat "$WATCH/restored" 2>/dev/null)"
  disarm
  stamp "watchdog removed: the node stays on $KUBE_VERSION"
  # A node whose tunnel is a host service gets the bundle's station-gateway
  # here: no DaemonSet brings it one. The interface stays up across the restart.
  if systemctl is-enabled rlark-station-gateway >/dev/null 2>&1 && ! cmp -s "$B/bin/station-gateway" /opt/rlark/bin/station-gateway; then
    (TUNNEL_INTERFACE=$(sed -n 's/.*addr show dev \([^ ]*\) .*/\1/p' /etc/systemd/system/rlark-station-gateway.service)
     install -m 755 "$B/bin/station-gateway" /opt/rlark/bin/station-gateway
     tunnel_service
     [ "${changed:-}" = tunnel ] || systemctl restart rlark-station-gateway)
    stamp "the tunnel service runs the station-gateway of this bundle"
  fi
  # The kubelet's settings of this bundle, on a node the installer made.
  if [ -e /etc/kubernetes/kubelet.conf.d/10-rlark.conf ]; then
    (changed=; kubelet_settings; [ -z "$changed" ] || systemctl restart kubelet)
    stamp "the kubelet has this bundle's settings"
  fi
  # The node's install annotations follow it (lib.sh, node_install): the
  # bundle is this one now, the site settings are the ones the node was
  # installed from. A node the installer did not make declares nothing.
  [ ! -e /etc/rlark/node-install ] || kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node "$node" --overwrite \
    "rlark.io/node-install-spec=$(cat "$B/tag").$(cat /etc/rlark/node-install)" rlark.io/node-install-state=ok >/dev/null
  ;;
rollback)
  disarm
  keep=/var/backups/rlark-kube-upgrade/$KUBE_VERSION
  [ -d "$keep/etc-kubernetes" ] || { stamp "nothing of $KUBE_VERSION is kept under $keep: this node was not upgraded from it"; exit 0; }
  if [ -e "$keep/etc-kubernetes/manifests/kube-apiserver.yaml" ]; then
    [ -e "$keep/etcd-snapshot.db" ] || fail "no etcd snapshot of $KUBE_VERSION in $keep"
    # The kubelet stops the control plane when its manifests go.
    aside=/var/backups/rlark-kube-upgrade/failed-$from-$(date +%Y%m%d%H%M%S)
    install -d "$aside"
    mv /etc/kubernetes/manifests "$aside/manifests" && install -d /etc/kubernetes/manifests
    tries=0
    while ss -ltn | grep -q -E ':(2379|2380) '; do
      tries=$((tries + 1))
      [ $tries -lt 60 ] || fail "etcd is still running 2 min after its manifest was removed"
      sleep 2
    done
    systemctl stop kubelet
    [ ! -d /var/lib/etcd ] || mv /var/lib/etcd "$aside/etcd"
    manifest=$keep/etc-kubernetes/manifests/etcd.yaml
    # The revision is raised past anything a client has seen: a kubelet that
    # holds a later resource version than the snapshot's would otherwise wait
    # for it for ever, and the other nodes are taken back through their kubelets.
    flag() { sed -n "s/^ *- --$1=//p" "$manifest"; }
    "$CTR" -a "$SOCKET" -n k8s.io run --rm --mount "type=bind,src=/var/lib,dst=/var/lib,options=rbind:rw" --mount "type=bind,src=$keep,dst=/keep,options=rbind:ro" \
      "$(sed -n 's/^ *image: //p' "$manifest")" rlark-etcd-restore etcdutl snapshot restore /keep/etcd-snapshot.db --data-dir /var/lib/etcd \
      --bump-revision 1000000000 --mark-compacted \
      --name "$(flag name)" --initial-cluster "$(flag initial-cluster)" --initial-advertise-peer-urls "$(flag initial-advertise-peer-urls)" > "$aside/etcd-restore.log" 2>&1 ||
      { tail -20 "$aside/etcd-restore.log" >&2; fail "etcd restore; the data directory of $from is $aside/etcd"; }
    stamp "etcd holds the snapshot taken before the upgrade from $KUBE_VERSION"
    cp -a /etc/kubernetes "$aside/etc-kubernetes"
    rm -rf /etc/kubernetes && cp -a "$keep/etc-kubernetes" /etc/kubernetes
    install -m 755 "$keep/kubeadm" "$BIN/kubeadm"
  else
    systemctl stop kubelet
  fi
  cp -a "$keep/kubelet/config.yaml" "$keep/kubelet/kubeadm-flags.env" /var/lib/kubelet/
  if [ -e "$keep/bin/kubelet" ]; then
    install -m 755 "$keep/bin/kubelet" "$BIN/kubelet.old" && mv "$BIN/kubelet.old" "$BIN/kubelet"
    [ ! -e "$keep/bin/kubectl" ] || { install -m 755 "$keep/bin/kubectl" "$BIN/kubectl.old" && mv "$BIN/kubectl.old" "$BIN/kubectl"; }
  fi
  systemctl start kubelet
  tries=0
  until curl -fsS -m 2 http://127.0.0.1:10248/healthz >/dev/null 2>&1; do
    tries=$((tries + 1))
    [ $tries -lt 90 ] || fail "the kubelet is not healthy 3 min after the rollback: journalctl -u kubelet"
    sleep 2
  done
  stamp "node back at $("$BIN/kubelet" --version | awk '{print $2}')"
  ;;
*) fail "PHASE must be certs, images, control, runtime, kubelet, confirm or rollback" ;;
esac
