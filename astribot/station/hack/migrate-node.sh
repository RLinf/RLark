#!/usr/bin/env bash
# Move a node from the cluster it is in to another registered cluster, without
# logging in to it: an admission into the new cluster is written, and a Job in
# the old cluster runs the bootstrap image on the node, which leaves the old
# cluster and joins the new one (install.sh, step 0). The node keeps its
# container runtime and images, its WireGuard key, its tunnel address and
# uplink. The same command serves every node of every cluster.
#
#   migrate-node.sh OLD_KUBECONFIG NEW_KUBECONFIG NEW_CLUSTER_ID NODE STATION_API IMAGE [label=value ...]
#
# Environment: KCP (management kubeconfig), CA (Station API's CA file),
# RLARK_URL and RLARK_TOKEN_FILE (the Gateway that gives out the admission's
# code, and an administrator's token: admit-node.sh), TAINTS,
# PULL_SECRET and JOB_NAMESPACE (where it is and the Job runs; kube-system),
# TIMEOUT (seconds to Ready in the new cluster; 600). A cluster's control-plane
# node does not move: its cluster would end with it.
# A node that does not become Ready in the new cluster goes back to the old
# one by itself (install.sh); this script then withdraws its admission, so
# that its enrollment in the old cluster stands again.
# Afterwards the node's leftovers in the old cluster are removed: its Node
# object and its NetworkEnrollment (its site node record there).
set -euo pipefail
: "${KCP:=/home/cailiang/.local/share/rlark-runtime/kcp-admin.kubeconfig}" "${CA:=/tmp/station-api-ca.crt}"
OLD=$1 NEW=$2 CLUSTER=$3 NODE=$4 STATION_API=$5 IMAGE=$6; shift 6
HERE=$(cd "$(dirname "$0")" && pwd)
old() { kubectl --kubeconfig "$OLD" "$@"; }
new() { kubectl --kubeconfig "$NEW" "$@"; }
mgmt() { kubectl --kubeconfig "$KCP" "$@"; }
stamp() { echo "$(date +%T) $*"; }
started=$(date +%s)
pull=; [[ -z ${PULL_SECRET:-} ]] || pull="imagePullSecrets: [{name: $PULL_SECRET}]"
NS=${JOB_NAMESPACE:-kube-system}

# What the node has now is what it keeps.
read -r old_namespace address < <(mgmt get networkenrollments.station.rlark.io -A -o json | python3 -c '
import json, sys
for e in json.load(sys.stdin)["items"]:
    if e["metadata"]["name"] == sys.argv[1] and e["status"].get("phase") == "Enrolled":
        print(e["metadata"]["namespace"], e["status"]["tunnelAddress"])' "$NODE") || true
[[ -n ${old_namespace:-} ]] || { echo "$NODE has no Enrolled NetworkEnrollment in the management plane: it is not a node of the site network under that name (kubectl get networkenrollments.station.rlark.io -A)" >&2; exit 1; }
old_cluster=$(mgmt -n "$old_namespace" get networkenrollments.station.rlark.io "$NODE" -o jsonpath='{.spec.clusterID}')
uplink=$(mgmt -n station-system get sitenetwork default -o json | python3 -c '
import json, sys
for n in json.load(sys.stdin)["spec"]["nodes"]:
    if (n["clusterID"], n["nodeName"]) == tuple(sys.argv[1:3]) and n.get("uplink"):
        print(n["uplink"]["gateway"])' "$old_cluster" "$NODE")
! old get nodes -l node-role.kubernetes.io/control-plane -o name | grep -qx "node/$NODE" || { echo "$NODE is the control plane of $old_cluster: that cluster would end with it. A control plane moves to another machine with its cluster (the installer's control-plane mode, FROM_SNAPSHOT)" >&2; exit 1; }
[[ $old_cluster != "$CLUSTER" ]] || { echo "$NODE is a node of $CLUSTER already. A cluster whose control plane moves to another machine keeps its nodes where they are (the installer's control-plane mode, FROM_SNAPSHOT)" >&2; exit 1; }
stamp "$NODE: $old_cluster -> $CLUSTER, keeps $address${uplink:+ uplink $uplink}"

command=$(ADDRESS=$address UPLINK=$uplink TAINTS=${TAINTS:-} "$HERE/admit-node.sh" "$CLUSTER" "$NODE" "$@")
code=$(echo "$command" | tail -1 | sed -n 's#.*/b/\([a-z0-9]*\) .*#\1#p')
name=$(echo "node-migrate-$NODE" | cut -c1-63)
old -n "$NS" delete job "$name" --ignore-not-found >/dev/null
old apply -f - >/dev/null <<JOB
apiVersion: batch/v1
kind: Job
metadata: {name: $name, namespace: $NS}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    spec:
      nodeName: $NODE
      hostNetwork: true
      hostPID: true
      restartPolicy: Never
      tolerations: [{operator: Exists}]
      $pull
      containers:
      - name: migrate
        image: $IMAGE
        imagePullPolicy: IfNotPresent
        securityContext: {privileged: true}
        env:
        - {name: RLARK_SERVER, value: "https://$STATION_API"}
        - {name: RLARK_CODE, value: "$code"}
        - {name: RLARK_MIGRATE, value: "node"}
        - name: RLARK_CA
          value: |
$(sed 's/^/            /' "$CA")
JOB
stamp "$NODE: the move is running on the node"
# Ready in the new cluster, said by its kubelet after the move began: a
# control plane made from a snapshot holds the node's last word from before.
ready_since() {
  local at; at=$(new get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status} {.status.conditions[?(@.type=="Ready")].lastHeartbeatTime}' 2>/dev/null) || return 1
  [[ $at == True\ * ]] && (( $(date -d "${at#True }" +%s) >= started ))
}
until ready_since; do
  # The Job has failed and the node is Ready where it was: it did not leave,
  # or it is back. (A node that has left takes its Job's pod along; that pod
  # is called failed in the old cluster some minutes later.)
  if [[ $(old -n "$NS" get job "$name" -o jsonpath='{.status.failed}' 2>/dev/null) -ge 1 && $(old get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null) == True ]]; then
    echo "$NODE did not move: the installer on it failed and the node is Ready under its old control plane. The end of its log:" >&2
    old -n "$NS" logs "job/$name" --tail=8 >&2 || true
    exit 1
  fi
  if (( $(date +%s) - started > ${TIMEOUT:-600} )); then
    mgmt -n "$(mgmt get clusters.rlinf.io "$CLUSTER" -o jsonpath='{.spec.agentNamespace}')" delete nodeadmissions.station.rlark.io "$NODE" --ignore-not-found >/dev/null
    new delete node "$NODE" --ignore-not-found >/dev/null
    echo "$NODE is not Ready in $CLUSTER after ${TIMEOUT:-600} s. The node returns to its old control plane by itself when its join fails: old get node $NODE" >&2
    exit 1
  fi
  sleep 2
done
stamp "$NODE: Ready in $CLUSTER after $(( $(date +%s) - started )) s"
namespace=$(mgmt get clusters.rlinf.io "$CLUSTER" -o jsonpath='{.spec.agentNamespace}')
tries=0
until [[ $(mgmt -n "$namespace" get nodeadmissions.station.rlark.io "$NODE" -o jsonpath='{.status.phase}') == Joined ]]; do
  tries=$((tries + 1))
  (( tries < 150 )) || { echo "$NODE is Ready in $CLUSTER, and its admission is not Joined after 5 min: it has not enrolled from there (its station-agent, its NetworkEnrollment there). Its admission keeps it among its peers meanwhile" >&2; exit 1; }
  sleep 2
done
stamp "$NODE: enrolled in $CLUSTER under its own identity after $(( $(date +%s) - started )) s"

# What it left behind.
old delete node "$NODE" --ignore-not-found >/dev/null
mgmt -n "$old_namespace" delete networkenrollments.station.rlark.io "$NODE" --ignore-not-found >/dev/null
stamp "$NODE: its site node record in $old_cluster is removed"
new get node "$NODE" -o wide
