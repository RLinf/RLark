#!/usr/bin/env bash
# Take a kubeadm cluster one Kubernetes minor version up (or to a newer patch),
# node by node, without logging in to any node. The same command serves every
# robot cluster and any number of nodes.
#
#   upgrade-cluster.sh KUBECONFIG VERSION IMAGE
#
#   IMAGE  the upgrade bundle of VERSION: the cluster's environment's installer
#          over the base of VERSION as an image (bootstrap/Dockerfile.installer,
#          --target image: rlark/node-bootstrap:ci-<commit>-installer-live,
#          built with a release on the office's management machine
#          (astribot/release/run.sh) once live.yaml's installer.base is that
#          base): kubeadm, kubelet, kubectl and every image the version's control
#          plane and nodes run, so that no node needs the internet.
# Order, as kubeadm prescribes: each control-plane node, then every other node,
# one at a time (each also moved onto the bundle's containerd-fleet before its
# kubelet: upgrade.sh, PHASE=runtime), after the certificates of each control-plane node have been
# completed for the node's present addresses: etcd snapshot (control plane) -> kubeadm upgrade -> drain ->
# kubelet -> confirm -> back to its previous scheduling state (a node whose
# kubelet is at VERSION already is not drained: at the cluster's own version
# the run brings the bundle's images and node settings and moves no pod). The kubelet
# phase leaves a watchdog on the node that puts the old kubelet back after
# WATCHDOG_SECONDS (600) unless this script, having seen the node Ready at
# VERSION, confirms: a node whose new kubelet does not start, or whose upgrade
# lost this session, returns by itself (cordoned, as the drain left it). It stops at the first
# failed check. What a node replaced is kept on it in
# /var/backups/rlark-kube-upgrade/<old version>/, the snapshot too
# (etcd-snapshot.db). ROLLBACK=yes with the old version as VERSION and the
# image of the failed upgrade takes the worker nodes back, after the control
# plane has been taken back on its node (upgrade.sh, PHASE=rollback).
# Environment: PULL_SECRET (name of an image pull secret) and JOB_NAMESPACE
# (where it is and the Jobs run; kube-system), NODES, WATCHDOG_SECONDS;
# MANAGEMENT_KUBECONFIG and MANAGEMENT_NAMESPACE for a cluster under an RLark
# agent (the management plane and the cluster's namespace in it).
set -euo pipefail
K=$1 VERSION=$2 IMAGE=$3
k() { kubectl --kubeconfig "$K" "$@"; }
stamp() { echo "$(date +%T) $*"; }
minor() { echo "$1" | sed -E 's/^v1\.([0-9]+)\..*/\1/'; }
started=$(date +%s)
watchdog=${WATCHDOG_SECONDS:-600}
# The Jobs run where the image pull secret of the cluster is.
NS=${JOB_NAMESPACE:-kube-system}
pull=; [[ -z ${PULL_SECRET:-} ]] || pull="imagePullSecrets: [{name: $PULL_SECRET}]"

current=$(k version -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["serverVersion"]["gitVersion"])')
step=$(( $(minor "$VERSION") - $(minor "$current") ))
[[ $step == 0 || $step == 1 ]] || { echo "the cluster is at $current: kubeadm goes one minor version at a time, not to $VERSION" >&2; exit 1; }
# Under an RLark agent a node's scheduling follows its management Node: without the management plane the run would
# leave every drained node cordoned there, and the agent cordons it again (10-0-2-196, 11-17).
if [[ -z ${ROLLBACK:-} && -z ${MANAGEMENT_KUBECONFIG:-} ]] && k -n rlark-system get daemonset rlark-agent-node >/dev/null 2>&1; then
  echo "this cluster runs under an RLark agent: give MANAGEMENT_KUBECONFIG and MANAGEMENT_NAMESPACE (the cluster's namespace there)" >&2; exit 1
fi
nodes="$(k get nodes -l node-role.kubernetes.io/control-plane -o name) $(k get nodes -l '!node-role.kubernetes.io/control-plane' -o name)"
# NODES limits the run to these nodes ("node/a node/b", control plane first):
# to go on after a stop, or to leave out a node that cannot be upgraded yet. A
# kubelet may stay up to three minor versions behind the control plane.
nodes=${NODES:-$nodes}
if [[ -z ${ROLLBACK:-} ]]; then
  for item in $nodes; do
    [[ $(k get "$item" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}') == True ]] || { echo "${item#node/} is not Ready: a node must be Ready before its upgrade" >&2; exit 1; }
  done
fi
stamp "cluster at $current -> $VERSION; order:$(echo $nodes | sed 's#node/# #g')"

# A node that restarts must get its Pod network back without the kubernetes
# Service's address (calico-endpoint.sh says why): set once for the cluster.
[[ -n ${ROLLBACK:-} ]] || "$(dirname "$0")/calico-endpoint.sh" "$K"

# job runs one phase of upgrade.sh on one node and waits for it; the API may
# be away for a while when the phase restarts the control plane.
job() { # node phase
  local node=$1 phase=$2 name status
  name=$(echo "kube-upgrade-$phase-$node" | cut -c1-63 | sed 's/-*$//')
  # With its pods: one left of a failed run would be taken for this run's.
  k -n "$NS" delete job "$name" --ignore-not-found --cascade=foreground >/dev/null
  k apply -f - >/dev/null <<JOB
apiVersion: batch/v1
kind: Job
metadata: {name: $name, namespace: $NS, labels: {app: rlark-kube-upgrade}}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    metadata: {labels: {app: rlark-kube-upgrade}}
    spec:
      nodeName: $node
      hostNetwork: true
      hostPID: true
      restartPolicy: Never
      tolerations: [{operator: Exists}]
      $pull
      containers:
      - name: upgrade
        image: $IMAGE
        imagePullPolicy: IfNotPresent
        securityContext: {privileged: true}
        env: [{name: PHASE, value: $phase}, {name: KUBE_VERSION, value: $VERSION}, {name: WATCHDOG_SECONDS, value: "$watchdog"}, {name: NODE_NAME, value: $node}]
JOB
  local began=$(date +%s)
  until status=$(k -n "$NS" get job "$name" -o jsonpath='{.status.succeeded}/{.status.failed}' 2>/dev/null) && [[ $status == 1/* || $status == */1 ]]; do
    # No phase takes this long. A kubelet phase that does not end has left a
    # node without a kubelet, the old one included: its Job never reports.
    if (( $(date +%s) - began > watchdog + 300 )); then
      echo "phase $phase on $node has not ended after $(( watchdog + 300 )) s: the node needs a person; on it, /var/lib/rlark/kube-upgrade/restored says what its watchdog did" >&2; exit 1
    fi
    # A node that cannot pull the bundle would be waited for for ever.
    if k -n "$NS" get pods -l "job-name=$name" -o jsonpath='{.items[*].status.containerStatuses[*].state.waiting.reason}' 2>/dev/null | grep -q -E 'ErrImagePull|ImagePullBackOff'; then
      echo "$node cannot pull $IMAGE (phase $phase); nothing was changed on it by this phase" >&2; exit 1
    fi
    sleep 2
  done
  k -n "$NS" logs "job/$name" 2>/dev/null | sed 's/^/    /'
  [[ $status == 1/* ]] || { echo "phase $phase failed on $node" >&2; exit 1; }
}

# Back to VERSION, the version before the last upgrade. The control plane
# first, on its node, where the API cannot be used for it (upgrade.sh,
# PHASE=rollback); then this, for every other node.
if [[ -n ${ROLLBACK:-} ]]; then
  # The snapshot the control plane came back from holds the note the failed
  # run left on its node; the scheduling state itself is the snapshot's.
  k annotate node --all rlark.io/unschedulable-before-upgrade- >/dev/null
  for item in $(k get nodes -l '!node-role.kubernetes.io/control-plane' -o name); do
    node=${item#node/}
    job "$node" rollback
    until [[ $(k get node "$node" -o jsonpath='{.status.nodeInfo.kubeletVersion} {.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null) == "$VERSION True" ]]; do sleep 2; done
    stamp "$node: back at $VERSION and Ready"
  done
  k get nodes -o wide
  exit 0
fi

# kubeadm reaches etcd at the address etcd advertises and verifies etcd's
# certificate for it. On a control-plane node whose address has changed since
# the cluster was made it fails there, with etcd already replaced: each
# control-plane node first issues the certificates that lack an address of
# the node (upgrade.sh, PHASE=certs; nothing is done where none does), and
# nothing else starts unless etcd then answers where it advertises.
for item in $(k get nodes -l node-role.kubernetes.io/control-plane -o name); do
  [[ " $nodes " == *" $item "* ]] || continue
  job "${item#node/}" certs
  until k get --raw /readyz >/dev/null 2>&1; do sleep 1; done
  url=$(k -n kube-system get pod "etcd-${item#node/}" -o jsonpath='{.metadata.annotations.kubeadm\.kubernetes\.io/etcd\.advertise-client-urls}')
  k -n kube-system exec "etcd-${item#node/}" -- etcdctl --endpoints "$url" --cacert /etc/kubernetes/pki/etcd/ca.crt \
    --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key endpoint health >/dev/null 2>&1 ||
    { echo "etcd on ${item#node/} does not answer at $url, the address it advertises, with a certificate valid for it: kubeadm would fail after replacing etcd" >&2; exit 1; }
done

# The control plane's upgrade replaces kube-proxy on every node and may move
# CoreDNS to any: each node that runs pods gets the images first.
# A run that only brings NODES up to a control plane already at VERSION rolls
# nothing out to the others.
for node in $(k get nodes -o jsonpath='{range .items[*]}{.metadata.name} {.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' | sed -n 's/ True$//p'); do
  [[ $step == 1 || " $nodes " == *" node/$node "* ]] || continue
  # IMAGES_SKIP: nodes left out here (one whose link takes too long for a bundle per hop); they get the images of
  # their own run, and pull kube-proxy's meanwhile.
  [[ " ${IMAGES_SKIP:-} " != *" $node "* ]] || { stamp "$node: images left to its own run (IMAGES_SKIP)"; continue; }
  job "$node" images
done
stamp "images of $VERSION on every Ready node"

for item in $nodes; do
  node=${item#node/}
  # A node whose kubelet runs VERSION already keeps it: its pods are not moved
  # off it (a run at the cluster's own version, which brings a bundle's node
  # settings, drains nothing). The kubelet phase does nothing there, and a
  # restart of the kubelet for its settings stops no container.
  [[ $(k get node "$node" -o jsonpath='{.status.nodeInfo.kubeletVersion}') != "$VERSION" ]] && drain=yes || drain=
  # The scheduling state to return to is kept on the node: a run that goes on
  # after a failed one finds the node cordoned by that one's drain.
  cordoned=$(k get node "$node" -o jsonpath='{.metadata.annotations.rlark\.io/unschedulable-before-upgrade}')
  if [[ -z $cordoned && -n $drain ]]; then
    cordoned=$(k get node "$node" -o jsonpath='{.spec.unschedulable}'); cordoned=${cordoned:-false}
    k annotate node "$node" "rlark.io/unschedulable-before-upgrade=$cordoned" >/dev/null
  fi
  if k get nodes -l node-role.kubernetes.io/control-plane -o name | grep -qx "node/$node"; then
    k -n kube-system exec "etcd-$node" -- etcdctl --endpoints https://127.0.0.1:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt \
      --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key \
      snapshot save "/var/lib/etcd/rlark-snapshot-before-$VERSION.db" >/dev/null
    stamp "$node: etcd snapshot saved"
  fi
  t0=$(date +%s)
  job "$node" control
  until k get --raw /readyz >/dev/null 2>&1; do sleep 1; done
  stamp "$node: kubeadm phase done in $(( $(date +%s) - t0 )) s"
  [[ -z $drain ]] || k drain "$node" --ignore-daemonsets --delete-emptydir-data --force --pod-selector='app!=rlark-kube-upgrade' --timeout=180s >/dev/null
  # The runtime every node's kubelet uses, the bundle's containerd-fleet
  # (upgrade.sh, PHASE=runtime): a switch from another runtime is made by a
  # unit on the node after the phase's Job ended, and is over when the node is
  # Ready on it. A switch that fails puts the old runtime back by itself.
  runtime=
  if [[ -n $drain ]]; then
    job "$node" runtime
    runtime=$(k -n "$NS" logs "job/$(echo "kube-upgrade-runtime-$node" | cut -c1-63 | sed 's/-*$//')" 2>/dev/null | sed -n 's/.*the kubelet will run on \(containerd:\/\/[^:]*\):.*/\1/p')
  fi
  if [[ -n $runtime ]]; then
    t0=$(date +%s)
    until [[ $(k get node "$node" -o jsonpath='{.status.nodeInfo.containerRuntimeVersion} {.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null) == "$runtime True" ]]; do
      (( $(date +%s) - t0 < 300 )) || { echo "$node is not Ready on $runtime 300 s after its switch began: /var/lib/rlark/kube-upgrade/runtime-switch.log on it says why (a kubelet not healthy there is back on its old runtime)" >&2; exit 1; }
      sleep 2
    done
    stamp "$node: Ready on $runtime"
  fi
  t0=$(date +%s)
  job "$node" kubelet
  until [[ $(k get node "$node" -o jsonpath='{.status.nodeInfo.kubeletVersion} {.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null) == "$VERSION True" ]]; do
    (( $(date +%s) - t0 < watchdog )) || { echo "$node was not Ready at $VERSION within $watchdog s: its watchdog has put the old kubelet back" >&2; exit 1; }
    sleep 2
  done
  job "$node" confirm
  # A container that ran through the kubelet's replacement may have lost its
  # devices: a GPU device plugin then offers none, or cannot see them. The
  # node's device-plugin pods are made anew, and what the node offers checked.
  plugins='
import json, sys
for p in json.load(sys.stdin)["items"]:
    if any(o["kind"] == "DaemonSet" for o in p["metadata"].get("ownerReferences", [])) and any(v.get("hostPath", {}).get("path", "").rstrip("/") == "/var/lib/kubelet/device-plugins" for v in p["spec"].get("volumes", [])):
        ready = any(c["type"] == "Ready" and c["status"] == "True" for c in p["status"].get("conditions", []))
        print(p["metadata"]["namespace"] + "/" + p["metadata"]["name"], ready)'
  short='
import json, sys
s = json.load(sys.stdin)["status"]
print(" ".join("%s %s/%s" % (r, s["allocatable"].get(r), c) for r, c in s["capacity"].items() if "/" in r and s["allocatable"].get(r) != c))'
  old=$(k get pods -A --field-selector "spec.nodeName=$node" -o json | python3 -c "$plugins" | cut -d' ' -f1)
  for pod in $old; do k -n "${pod%/*}" delete pod "${pod#*/}" >/dev/null; done
  t1=$(date +%s)
  until now=$(k get pods -A --field-selector "spec.nodeName=$node" -o json | python3 -c "$plugins") && [[ $(echo "$now" | grep -c ' True$') == $(echo "$old" | grep -c .) ]] &&
    less=$(k get node "$node" -o json | python3 -c "$short") && [[ -z $less ]]; do
    (( $(date +%s) - t1 < 180 )) || { echo "$node: 180 s after its device plugins were made anew they are not all Ready or the node offers fewer devices than it has: ${less:-}" >&2; exit 1; }
    sleep 3
  done
  # A node that was not drained, by this run or a failed one, keeps its state.
  if [[ -n $cordoned && $cordoned != true ]]; then
    # Under an RLark agent a node's scheduling state is its management Node's:
    # the agent takes a cordon up to it and puts an uncordon made here back.
    # Both are set: the agent itself may be a pod the drain took off this
    # node, which cannot return to carry the management Node's state down.
    [[ -z ${MANAGEMENT_KUBECONFIG:-} ]] ||
      kubectl --kubeconfig "$MANAGEMENT_KUBECONFIG" -n "$MANAGEMENT_NAMESPACE" patch nodes.rlinf.io "$node" --type=merge -p '{"spec":{"unschedulable":false}}' >/dev/null
    k uncordon "$node" >/dev/null
  fi
  k annotate node "$node" rlark.io/unschedulable-before-upgrade- >/dev/null
  stamp "$node: kubelet $VERSION and Ready in $(( $(date +%s) - t0 )) s"
done
k get nodes -o wide
[[ $(k version -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["serverVersion"]["gitVersion"])') == "$VERSION" ]]
stamp "cluster at $VERSION in $(( $(date +%s) - started )) s"
