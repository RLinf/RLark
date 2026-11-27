#!/usr/bin/env bash
# Move a cluster's control plane to another machine of the cluster, without
# logging in to either: a Job on the node that takes over waits for the old
# control plane's last snapshot, a Job on the old one hands it over
# (bootstrap/control-plane.sh: handover with SUCCESSOR, FROM_CONTROL_PLANE),
# and the site network is told that the control plane's name is the new
# machine's. The nodes stay where they are; so does everything in the cluster.
#
#   move-control-plane.sh KUBECONFIG TO_NODE IMAGE
#
#   TO_NODE  a worker of the cluster, at the installer's Kubernetes version
#   IMAGE    the installer image, present on both machines or pullable there
# Environment: KCP (management kubeconfig), PULL_SECRET and JOB_NAMESPACE
# (kube-system), TIMEOUT (seconds from the handover to all nodes Ready; 600).
# Afterwards the old machine keeps everything it had, stopped: once the
# cluster is seen to be well, "IMAGE control-plane undo" there removes it.
# Back, before that: give the name back (SiteNetwork, names), "control-plane
# resume" on the old machine, "control-plane undo" on the new one.
set -euo pipefail
: "${KCP:=/home/cailiang/.local/share/rlark-runtime/kcp-admin.kubeconfig}"
K=$1 TO=$2 IMAGE=$3
k() { kubectl --kubeconfig "$K" "$@"; }
mgmt() { kubectl --kubeconfig "$KCP" "$@"; }
stamp() { echo "$(date +%T) $*"; }
NS=${JOB_NAMESPACE:-kube-system}
pull=; [[ -z ${PULL_SECRET:-} ]] || pull="imagePullSecrets: [{name: $PULL_SECRET}]"

from=$(k get nodes -l node-role.kubernetes.io/control-plane -o jsonpath='{.items[0].metadata.name}')
address() { k get node "$1" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}'; }
old=$(address "$from") new=$(address "$TO")
configuration=$(k -n kube-system get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}')
setting() { echo "$configuration" | sed -n "s/^ *$1: *//p" | tr -d '"'; }
name=$(setting controlPlaneEndpoint) name=${name%:*} pods=$(setting podSubnet) services=$(setting serviceSubnet)
nodes=$(k get nodes -o name | grep -v -x "node/$from")
stamp "$name: $from ($old) -> $TO ($new); pods $pods, Services $services"

job() { # NAME NODE ARGUMENTS ENV...
  local job=$1 node=$2 arguments=$3; shift 3
  k -n "$NS" delete job "$job" --ignore-not-found >/dev/null
  k apply -f - >/dev/null <<JOB
apiVersion: batch/v1
kind: Job
metadata: {name: $job, namespace: $NS}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    spec:
      nodeName: $node
      hostNetwork: true
      hostPID: true
      restartPolicy: Never
      tolerations: [{operator: Exists}]
      $pull
      containers:
      - name: control-plane
        image: $IMAGE
        imagePullPolicy: IfNotPresent
        securityContext: {privileged: true}
        args: [$arguments]
        env:
        - {name: RLARK_DETACH, value: "yes"}
$(for pair in "$@"; do echo "        - {name: ${pair%%=*}, value: \"${pair#*=}\"}"; done)
JOB
}
# The new one first: it waits for the snapshot, then for the name.
job "control-plane-to-$TO" "$TO" control-plane "CONTROL_PLANE_NAME=$name" "POD_SUBNET=$pods" "SERVICE_SUBNET=$services" "FROM_CONTROL_PLANE=$old"
k -n "$NS" wait --for=jsonpath='{.status.phase}'=Running pod -l "job-name=control-plane-to-$TO" --timeout=180s >/dev/null
# The old one is not waited for: its Job's first act is to stop the API this would ask.
job "control-plane-from-$from" "$from" 'control-plane, handover' "SUCCESSOR=$TO"
started=$(date +%s)
stamp "$from hands over; $TO waits for its snapshot"
while k get --raw /readyz >/dev/null 2>&1; do sleep 1; done
stamp "the API at $from has stopped; the site network is told that $name is $new"
index=$(mgmt -n station-system get sitenetwork default -o jsonpath='{range .spec.names[*]}{.name}{"\n"}{end}' | grep -n -x "$name" | cut -d: -f1)
mgmt -n station-system patch sitenetwork default --type=json -p "[{\"op\": \"replace\", \"path\": \"/spec/names/$((index - 1))/address\", \"value\": \"$new\"}]" >/dev/null
until k get --raw /readyz >/dev/null 2>&1 && k wait --for=condition=Ready "node/$TO" $nodes --timeout=10s >/dev/null 2>&1; do
  (( $(date +%s) - started < ${TIMEOUT:-600} )) || { echo "the nodes are not Ready under $TO after ${TIMEOUT:-600} s. On $TO: /var/lib/rlark/bootstrap/control-plane.log. Back: give $name the address $old again, \"control-plane resume\" on $from, \"control-plane undo\" on $TO" >&2; exit 1; }
  sleep 2
done
stamp "the nodes are Ready under $TO $(( $(date +%s) - started )) s after the handover began"
k get nodes -o wide
echo "$from keeps what it had, stopped. When the cluster is seen to be well: \"$IMAGE control-plane undo\" on $from."
