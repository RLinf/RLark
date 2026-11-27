#!/bin/bash
# Isolated run of hack/upgrade-cluster.sh: a kind cluster of a control-plane
# node and a worker at v1.31 goes to v1.32 and v1.33. A static pod on each
# node stands for what must keep running (a robot's device layer): its
# container must be the same one before and after.
#
#   upgrade.sh bundle VERSION   build the upgrade bundle of VERSION (pulls its images once)
#   upgrade.sh up               the v1.31 cluster with the static pods
#   upgrade.sh moved            make the control-plane node one whose address changed after the cluster was made
#   upgrade.sh to VERSION       load the bundle into the nodes and upgrade
#   upgrade.sh back VERSION     roll the cluster back to VERSION, the version before the last upgrade
#   upgrade.sh witness          the static pods' container IDs and start times
#   upgrade.sh down
#
# KUBE_DIR holds <version>/{kubeadm,kubelet,kubectl}. Not covered here: cgroup
# v1, a vendor's containerd, Calico (kind uses kindnet), a registry pull.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../../.." && pwd)
D=${E2E_DIR:-$HOME/.cache/station-e2e-upgrade}
KUBE_DIR=${KUBE_DIR:-$REPO/../kube}
NAME=e2e-up
K=$D/kubeconfig
image() { echo "rlark/node-bootstrap:upgrade-$1"; }

bundle() {
  local version=$1 images=$D/images-$1
  mkdir -p "$images"
  refs=$("$KUBE_DIR/$version/kubeadm" config images list --kubernetes-version "$version")
  for ref in $refs; do docker image inspect "$ref" >/dev/null 2>&1 || docker pull -q "$ref"; done
  docker save $refs -o "$images/control-plane.tar"
  KUBE_BIN=$KUBE_DIR/$version CNI_BIN= IMAGES=$images "$REPO/astribot/station/bootstrap/build.sh" "$(image "$version")" "$D/bundle-$version.tar"
}

up() {
  mkdir -p "$D"
  # /boot: kubeadm's system verification reads the kernel configuration, which a node container lacks.
  printf 'kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n  extraMounts: [{hostPath: /boot, containerPath: /boot, readOnly: true}]\n- role: worker\n  extraMounts: [{hostPath: /boot, containerPath: /boot, readOnly: true}]\n' > "$D/kind.yaml"
  kind create cluster --name $NAME --image "${NODE_IMAGE:-kindest/node:v1.31.0}" --config "$D/kind.yaml" --kubeconfig "$K" --wait 120s 2>&1 | tail -1
  pause=$(docker exec $NAME-control-plane ctr -n k8s.io images ls -q | grep '/pause:' | grep -v sha256 | head -1)
  for node in $NAME-control-plane $NAME-worker; do
    docker exec -i "$node" sh -c 'cat > /etc/kubernetes/manifests/witness.yaml' <<POD
apiVersion: v1
kind: Pod
metadata: {name: witness, namespace: default}
spec:
  hostNetwork: true
  containers: [{name: witness, image: $pause, imagePullPolicy: Never}]
POD
  done
  # A setting of one node only, as a workstation's swap handling is: it must survive the upgrades.
  docker exec $NAME-worker sh -c "sed -i '/^maxPods:/d' /var/lib/kubelet/config.yaml && echo 'maxPods: 77' >> /var/lib/kubelet/config.yaml && systemctl restart kubelet"
  until [[ $(kubectl --kubeconfig "$K" get pods -o name 2>/dev/null | grep -c witness) == 2 ]]; do sleep 1; done
  until [[ $(kubectl --kubeconfig "$K" get node $NAME-worker -o jsonpath='{.status.capacity.pods}') == 77 ]]; do sleep 1; done
  kubectl --kubeconfig "$K" wait --for=condition=Ready node --all --timeout=90s >/dev/null
  kubectl --kubeconfig "$K" wait --for=condition=Ready pod --all --timeout=60s >/dev/null
  witness
}

# moved: the state a robot's control-plane node was found in. The node has a
# new address beside the old one and one in the station's tunnel; its
# manifests, every kubeconfig, cluster-info and kube-proxy name the new one;
# the API server's certificate was issued again for it, etcd's two were not;
# the kubeadm configuration and etcd's member list still tell of the day the
# cluster was made. The node's kubelet reserves CPUs, as a robot's does.
moved() {
  local cp=$NAME-control-plane new=${1:-172.18.0.250} old
  old=$(docker inspect $cp -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
  kubectl --kubeconfig "$K" -n kube-system get configmap kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | sed "s/^  certSANs:/  certSANs:\n  - $old/" > "$D/ClusterConfiguration"
  kubectl --kubeconfig "$K" -n kube-system create configmap kubeadm-config --from-file="ClusterConfiguration=$D/ClusterConfiguration" --dry-run=client -o yaml | kubectl --kubeconfig "$K" replace -f - >/dev/null
  for map in kube-system/kube-proxy kube-public/cluster-info; do
    kubectl --kubeconfig "$K" -n ${map%/*} get configmap ${map#*/} -o yaml | sed "s#server: https://.*#server: https://$new:6443#" | kubectl --kubeconfig "$K" replace -f - >/dev/null
  done
  { printf 'apiVersion: kubeadm.k8s.io/v1beta4\nkind: InitConfiguration\nlocalAPIEndpoint: {advertiseAddress: "%s"}\n---\n' "$new"; sed "s/^  certSANs:/  certSANs:\n  - $new/" "$D/ClusterConfiguration"; } |
    docker exec -i $cp sh -c 'cat > /tmp/moved.yaml'
  docker exec $NAME-worker sed -i "s#server: https://.*#server: https://$new:6443#" /etc/kubernetes/kubelet.conf
  docker exec $NAME-worker systemctl restart kubelet
  docker exec $cp sh -ec "
    ip addr add $new/32 dev eth0
    ip link add st-wg0 type dummy && ip addr add 10.254.240.3/32 dev st-wg0 && ip link set st-wg0 up
    mkdir /tmp/moved && mv /etc/kubernetes/pki/apiserver.crt /etc/kubernetes/pki/apiserver.key /tmp/moved/
    kubeadm init phase certs apiserver --config /tmp/moved.yaml >/dev/null 2>&1
    sed -i 's#server: https://.*#server: https://$new:6443#' /etc/kubernetes/*.conf
    sed -i 's/--node-ip=$old/--node-ip=$new/' /var/lib/kubelet/kubeadm-flags.env
    printf 'cpuManagerPolicy: static\nreservedSystemCPUs: \"0\"\nsystemReserved: {memory: 300Mi}\n' >> /var/lib/kubelet/config.yaml
    mv /var/lib/kubelet/cpu_manager_state /tmp/moved/
    sed -i 's/$old/$new/g' /etc/kubernetes/manifests/etcd.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
    systemctl restart kubelet"
  until [[ $(kubectl --kubeconfig "$K" get node $cp -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null) == "$new" ]] &&
    [[ $(kubectl --kubeconfig "$K" -n kube-system get pod etcd-$cp -o jsonpath='{.metadata.annotations.kubeadm\.kubernetes\.io/etcd\.advertise-client-urls}' 2>/dev/null) == "https://$new:2379" ]]; do sleep 2; done
  kubectl --kubeconfig "$K" wait --for=condition=Ready node --all --timeout=120s >/dev/null
  kubectl --kubeconfig "$K" -n kube-system wait --for=condition=Ready pod etcd-$cp kube-apiserver-$cp --timeout=120s >/dev/null
  kubectl --kubeconfig "$K" -n kube-system delete pod -l k8s-app=kube-proxy >/dev/null
  local etcdctl="kubectl --kubeconfig $K -n kube-system exec etcd-$cp -- etcdctl --endpoints https://127.0.0.1:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key"
  $etcdctl member update "$($etcdctl member list | cut -d, -f1)" --peer-urls=https://127.0.0.1:2380 >/dev/null
  $etcdctl member list
  kubectl --kubeconfig "$K" get nodes -o wide
}

witness() {
  kubectl --kubeconfig "$K" get nodes -o jsonpath='{range .items[*]}{.metadata.name} {.status.nodeInfo.kubeletVersion} ready={.status.conditions[?(@.type=="Ready")].status} maxPods={.status.capacity.pods}{"\n"}{end}'
  kubectl --kubeconfig "$K" get pods -o jsonpath='{range .items[*]}{.metadata.name} restarts={.status.containerStatuses[0].restartCount} started={.status.containerStatuses[0].state.running.startedAt} id={.status.containerStatuses[0].containerID}{"\n"}{end}' | cut -c1-140
}

to() {
  kind load docker-image --name $NAME "$(image "$1")" >/dev/null
  "$REPO/astribot/station/hack/upgrade-cluster.sh" "$K" "$1" "$(image "$1")"
  witness
}

# back: the control plane on its node (the installer shares the node's
# namespaces by container name, a kind node having no docker of its own),
# then the worker through the API.
back() { # the version to return to; the bundle used is the one of the failed upgrade
  local image
  image=$(image "$(kubectl --kubeconfig "$K" version -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["serverVersion"]["gitVersion"])')")
  kubectl --kubeconfig "$K" create configmap written-after-the-snapshot --from-literal=a=b >/dev/null
  local t0=$(date +%s)
  docker run --rm --privileged --pid=container:$NAME-control-plane --net=container:$NAME-control-plane -e PHASE=rollback -e KUBE_VERSION="$1" "$image" 2>&1 | sed 's/^/    /'
  until kubectl --kubeconfig "$K" get --raw /readyz >/dev/null 2>&1; do sleep 1; done
  echo "$(date +%T) control plane back at $1 in $(( $(date +%s) - t0 )) s"
  ROLLBACK=yes "$REPO/astribot/station/hack/upgrade-cluster.sh" "$K" "$1" "$image"
  echo "$(date +%T) cluster back at $1 in $(( $(date +%s) - t0 )) s"
  kubectl --kubeconfig "$K" get configmap written-after-the-snapshot 2>&1 | tail -1 || true
  kubectl --kubeconfig "$K" -n kube-system get pods -o wide --no-headers | awk '{print $1, $2, $3}'
  witness
}

case ${1:-} in
down) kind delete cluster --name $NAME >/dev/null 2>&1 || true; rm -rf "$D" ;;
*) "$@" ;;
esac
