# Sourced by calico-endpoint.sh and calico-upgrade.sh, with K the cluster's kubeconfig.
k() { kubectl --kubeconfig "$K" "$@"; }
# renew_calico: calico-node made anew on every node, one at a time, whatever the
# DaemonSet's update strategy (OnDelete replaces nothing by itself).
renew_calico() {
  local pod node deadline
  for pod in $(k -n kube-system get pods -l k8s-app=calico-node -o name); do
    node=$(k -n kube-system get "$pod" -o jsonpath='{.spec.nodeName}')
    k -n kube-system delete "$pod" --wait=true --timeout=120s >/dev/null
    deadline=$((SECONDS + 300))
    until [[ $(k -n kube-system get pods -l k8s-app=calico-node --field-selector "spec.nodeName=$node" -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null) == true ]]; do
      (( SECONDS < deadline )) || { echo "$(date +%T) calico-node on $node not ready after 300s" >&2; exit 1; }
      sleep 2
    done
  done
}
