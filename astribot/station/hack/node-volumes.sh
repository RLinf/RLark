#!/usr/bin/env bash
# Moves the data of a node made before its volumes were declared into them
# (bootstrap/volumes.sh), from the cluster's API, without logging in to it:
#
#   node-volumes.sh KUBECONFIG NODE VOLUMES IMAGE [undo|confirm]
#
#   VOLUMES  as a node is admitted with them (NODE_VOLUMES):
#            "kubelet=60G:/var/lib/kubelet,runtime=150G:/var/lib/containerd-fleet"
#   IMAGE    the installer bundle (astribot/station/bootstrap/build.sh)
#
# The node stops while its data is copied, as for a reboot: its kubelet, its
# container runtime and every container of it; a control plane's API is away
# with them. Its pods start again on it. Nothing else of the cluster is
# touched. The data as it was stays beside each path on the node until
# "confirm" removes it; "undo" goes back to it before that (what pods wrote
# into the volumes since is lost). A move that fails on the node goes back by
# itself. Environment: PULL_SECRET, JOB_NAMESPACE (kube-system), TIMEOUT (1800 s).
set -euo pipefail
K=$1 NODE=$2 VOLUMES=$3 IMAGE=$4 ACTION=${5:-}
k() { kubectl --kubeconfig "$K" "$@"; }
stamp() { echo "$(date +%T) $*"; }
NS=${JOB_NAMESPACE:-kube-system}
pull=; [[ -z ${PULL_SECRET:-} ]] || pull="imagePullSecrets: [{name: $PULL_SECRET}]"
want=$VOLUMES detach='{name: RLARK_DETACH, value: "1"},'
case $ACTION in undo) want=undone ;; confirm) detach= ;; '') ;; *) echo "the action is undo or confirm" >&2; exit 1 ;; esac
[[ $(k get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}') == True ]] || { echo "$NODE is not Ready" >&2; exit 1; }

name=$(echo "node-volumes-${ACTION:-move}-$NODE" | cut -c1-63 | sed 's/-*$//')
k -n "$NS" delete job "$name" --ignore-not-found --cascade=foreground >/dev/null
[[ $ACTION == confirm ]] || k annotate node "$NODE" rlark.io/node-volumes- >/dev/null
k apply -f - >/dev/null <<JOB
apiVersion: batch/v1
kind: Job
metadata: {name: $name, namespace: $NS, labels: {app: rlark-node-volumes}}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    metadata: {labels: {app: rlark-node-volumes}}
    spec:
      nodeName: $NODE
      hostNetwork: true
      hostPID: true
      restartPolicy: Never
      tolerations: [{operator: Exists}]
      $pull
      containers:
      - name: volumes
        image: $IMAGE
        imagePullPolicy: IfNotPresent
        args: [volumes${ACTION:+, $ACTION}]
        securityContext: {privileged: true}
        env: [$detach {name: NODE_NAME, value: $NODE}, {name: NODE_VOLUMES, value: "$VOLUMES"}]
JOB
t0=$(date +%s)
stamp "$NODE: ${ACTION:-move into $VOLUMES} (Job $NS/$name)"
if [[ $ACTION == confirm ]]; then
  k -n "$NS" wait --for=condition=complete "job/$name" --timeout=300s >/dev/null || { k -n "$NS" logs "job/$name" >&2; exit 1; }
  k -n "$NS" logs "job/$name" | sed 's/^/    /'
  exit 0
fi
# The Job's pod ends with the node's containers; the node says how it went.
until state=$(k get node "$NODE" -o jsonpath='{.metadata.annotations.rlark\.io/node-volumes} {.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null) && [[ $state == "$want True" ]]; do
  [[ $state != failed* ]] || { echo "$NODE: the move ${state% *}; on the node: /var/lib/rlark/bootstrap/volumes.log" >&2; exit 1; }
  (( $(date +%s) - t0 < ${TIMEOUT:-1800} )) || { echo "$NODE is not back after ${TIMEOUT:-1800} s (rlark.io/node-volumes: ${state% *}); on the node: /var/lib/rlark/bootstrap/volumes.log" >&2; exit 1; }
  sleep 2
done
stamp "$NODE: Ready, ${want} in $(( $(date +%s) - t0 )) s"
