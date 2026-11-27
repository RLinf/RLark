#!/usr/bin/env bash
# Calico reaches the cluster's API at the control-plane endpoint itself, not
# through the kubernetes Service's virtual address.
#
#   calico-endpoint.sh KUBECONFIG
#
# calico-node installs the Pod network of its node: it must come up where the
# Service's address does not work yet, or never does for a process of the host.
# On a robot node it does not: kube-proxy masquerades what the host sends to a
# Service, the kernel takes the interface's first address for it, and that is
# the address every robot's computer has on its own internal network. The
# setting is Calico's own (ConfigMap kubernetes-services-endpoint, read by
# calico-node and its CNI installer). It is one object for the cluster; the
# endpoint is the one kubeadm records (controlPlaneEndpoint), or the address
# the API server advertises where the cluster has none. calico-node's Pods are
# made anew, node by node, only when the setting changes; calico-kube-controllers
# is a Pod of the Pod network and is left where it runs.
set -euo pipefail
K=$1
. "$(dirname "$0")/calico.sh"
k -n kube-system get daemonset calico-node >/dev/null 2>&1 || { echo "$(date +%T) the cluster's Pod network is not Calico: nothing to do"; exit 0; }
endpoint=$(k -n kube-system get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | sed -n 's/^controlPlaneEndpoint: *//p' | tr -d '"')
[[ -n $endpoint ]] || endpoint=$(k -n default get endpoints kubernetes -o jsonpath='{.subsets[0].addresses[0].ip}:{.subsets[0].ports[0].port}')
host=${endpoint%:*} port=${endpoint##*:}
if [[ $(k -n kube-system get configmap kubernetes-services-endpoint -o jsonpath='{.data.KUBERNETES_SERVICE_HOST}:{.data.KUBERNETES_SERVICE_PORT}' 2>/dev/null) == "$host:$port" ]]; then
  echo "$(date +%T) Calico reaches the API at $host:$port already: nothing to do"
  exit 0
fi
k -n kube-system create configmap kubernetes-services-endpoint --from-literal=KUBERNETES_SERVICE_HOST="$host" --from-literal=KUBERNETES_SERVICE_PORT="$port" --dry-run=client -o yaml | k apply -f - >/dev/null
renew_calico
echo "$(date +%T) Calico reaches the API at $host:$port"
