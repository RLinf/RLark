#!/usr/bin/env bash
# Take a cluster's Calico to the version of a manifest as Calico's project publishes it (calico.yaml, the one an
# installer base carries: bootstrap/Dockerfile.base), with what the running Calico was set to for this cluster carried over:
# the calico-node environment control-plane.sh adds (IP_AUTODETECTION_METHOD, CALICO_IPV4POOL_CIDR,
# FELIX_IGNORELOOSERPF) and the pods' MTU (calico-config veth_mtu). calico-node is replaced node by node by its own
# rolling update; its images are on every node already when the upgrade bundle's images phase ran.
#
#   calico-upgrade.sh KUBECONFIG MANIFEST
set -euo pipefail
K=$1 MANIFEST=$2
. "$(dirname "$0")/calico.sh"
running=$(k -n kube-system get ds calico-node -o json)
mtu=$(k -n kube-system get configmap calico-config -o jsonpath='{.data.veth_mtu}')
rendered=$(mktemp)
render=$(cat <<'PY'
import json, re, sys
manifest, mtu = sys.argv[1], sys.argv[2]
env = {e["name"]: e.get("value") for c in json.load(sys.stdin)["spec"]["template"]["spec"]["containers"] if c["name"] == "calico-node"
       for e in c.get("env", []) if "value" in e}
carried = [(name, env[name]) for name in ("IP_AUTODETECTION_METHOD", "CALICO_IPV4POOL_CIDR", "FELIX_IGNORELOOSERPF") if name in env]
text = open(manifest).read()
text = text.replace('veth_mtu: "0"', 'veth_mtu: "%s"' % mtu)
def insert(match):
    indent = match.group(1)
    return "".join('%s- name: %s\n%s  value: "%s"\n' % (indent, n, indent, v) for n, v in carried) + match.group(0)
text, count = re.subn(r"^( *)- name: CALICO_IPV4POOL_IPIP\n", insert, text, count=1, flags=re.M)
assert count == 1, "the manifest has no CALICO_IPV4POOL_IPIP to put the cluster's settings before"
sys.stdout.write(text)
PY
)
python3 -c "$render" "$MANIFEST" "$mtu" <<<"$running" > "$rendered"
before=$(k -n kube-system get ds calico-node -o jsonpath='{.spec.template.spec.containers[0].image}')
k apply --server-side --force-conflicts -f "$rendered" >/dev/null
rm -f "$rendered"
# rollout status refuses an OnDelete DaemonSet: there renew_calico is the rollout.
if [[ $(k -n kube-system get ds calico-node -o jsonpath='{.spec.updateStrategy.type}') == OnDelete ]]; then
  renew_calico
else
  k -n kube-system rollout status ds/calico-node --timeout=600s >/dev/null
fi
deadline=$((SECONDS + 300))
until [[ $(k -n kube-system get ds calico-node -o jsonpath='{.status.updatedNumberScheduled}/{.status.numberReady}') == "$(k -n kube-system get ds calico-node -o jsonpath='{.status.desiredNumberScheduled}/{.status.desiredNumberScheduled}')" ]]; do
  if (( SECONDS >= deadline )); then
    echo "$(date +%T) calico: not every calico-node updated and ready after 300s; nodes not Ready:" >&2
    k get nodes --no-headers | awk '$2 != "Ready"' >&2
    exit 1
  fi
  sleep 3
done
k -n kube-system rollout status deploy/calico-kube-controllers --timeout=300s >/dev/null
echo "$(date +%T) calico: $before -> $(k -n kube-system get ds calico-node -o jsonpath='{.spec.template.spec.containers[0].image}'), every calico-node ready"
