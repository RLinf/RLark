#!/bin/bash
# Assembles an installer from its parts on a machine, for local runs and the scripts under hack/. A release's
# installer is built on the office's management machine (astribot/release/run.sh): Dockerfile.installer over the base of
# a Kubernetes version, which Dockerfile.base builds (astribot/release/build.sh base).
#
# Builds the installer of the node bootstrap in its two forms, with the same content: the image and its docker
# archive OUT.tar, which an operator's typed command loads and runs, and OUT.bundle.tar, which RLark sends to a
# machine over SSH and runs there as /var/lib/rlark/bootstrap/station-bootstrap. The second needs no docker.
#
#   build.sh IMAGE_REF OUT.tar
#
# What goes into the bundle is the site's choice, given by the environment:
#   KUBE_BIN   directory with kubeadm, kubelet and kubectl of the cluster's version
#   CONTAINERD_BIN  directory with containerd, containerd-shim-runc-v2, ctr and runc, statically linked: the
#              runtime of every node this installer makes, so that a machine needs none of its own
#   CNI_BIN    directory with the CNI plugins
#   IMAGES     directory with the image archives a node needs before it can pull
#              (pause, kube-proxy, the CNI's images, station-agent, the RLark node
#              agent) and pause.ref, the reference of the pause image among them;
#              in its subdirectory control-plane the archives only a control
#              plane needs (API server, controller manager, scheduler, etcd,
#              CoreDNS, the CNI's controllers), which no worker imports
#   MANIFESTS  optional directory of manifests a new control plane applies, in
#              name order (control-plane.sh): calico.yaml as its project
#              publishes it, then whatever the site adds to every cluster
#   REGISTRY_CA  optional file with the CA of the site's registry (a public certificate): the first trust a
#              machine has in registry.rlark.internal:5443, before the site's registry-trust addon writes the same
set -euo pipefail
REF=$1 OUT=$2
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../.." && pwd)
W=$(mktemp -d); trap 'rm -rf "$W"' EXIT
mkdir -p "$W/bundle/bin" "$W/bundle/cni" "$W/bundle/images"
(cd "$REPO/astribot/station-gateway" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$W/bundle/bin/station-gateway" .)
(cd "$REPO/astribot/station" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$W/station-bootstrap" ./bootstrap)
for binary in kubeadm kubelet kubectl; do install -m 755 "$KUBE_BIN/$binary" "$W/bundle/bin/"; done
for binary in containerd containerd-shim-runc-v2 ctr runc; do
  # A runtime linked against the build machine's C library does not start on an older machine (a kind node's
  # containerd on Ubuntu 20.04: "GLIBC_2.34 not found"); the projects' static releases start anywhere.
  ! ldd "$CONTAINERD_BIN/$binary" 2>&1 | grep -q "=>" || { echo "$CONTAINERD_BIN/$binary is dynamically linked; take the static release (containerd-static-*.tar.gz, runc.amd64)" >&2; exit 1; }
  install -m 755 "$CONTAINERD_BIN/$binary" "$W/bundle/bin/"
done
[[ -z ${CNI_BIN:-} ]] || find "$CNI_BIN" -maxdepth 1 -type f -perm -u+x -exec install -m 755 {} "$W/bundle/cni/" \;
cp "$IMAGES"/*.tar "$W/bundle/images/"
[[ ! -e $IMAGES/pause.ref ]] || cp "$IMAGES/pause.ref" "$W/bundle/images/"
[[ ! -d $IMAGES/control-plane ]] || cp -r "$IMAGES/control-plane" "$W/bundle/images/"
[[ -z ${MANIFESTS:-} ]] || cp -r "$MANIFESTS" "$W/bundle/manifests"
# The tag is what a node says it was installed from (rlark.io/node-install-spec).
echo "${REF##*:}" > "$W/bundle/tag"
# Byte for byte what registry-trust writes (v0.2.3): the CA beside hosts.toml,
# named relative to it, so that the two fit under any runtime's config_path.
if [[ -n ${REGISTRY_CA:-} ]]; then
  mkdir -p "$W/bundle/certs.d/registry.rlark.internal:5443"
  printf '%s\n' "$(cat "$REGISTRY_CA")" > "$W/bundle/certs.d/registry.rlark.internal:5443/ca.crt"
  printf '%s\n' 'server = "https://registry.rlark.internal:5443"' '' '[host."https://registry.rlark.internal:5443"]' '  capabilities = ["pull", "resolve"]' '  ca = "ca.crt"' \
    > "$W/bundle/certs.d/registry.rlark.internal:5443/hosts.toml"
fi
for script in lib.sh install.sh upgrade.sh control-plane.sh volumes.sh tunnel.sh; do install -m 755 "$HERE/$script" "$W/bundle/$script"; done
cat > "$W/Dockerfile" <<'DOCKERFILE'
FROM scratch
COPY bundle /bundle
COPY station-bootstrap /station-bootstrap
ENTRYPOINT ["/station-bootstrap"]
DOCKERFILE
docker build -q -t "$REF" "$W" >/dev/null
docker save "$REF" -o "$OUT"
# The same files as they lie on a machine: the bundle, and the program beside it.
install -m 755 "$W/station-bootstrap" "$W/bundle/station-bootstrap"
tar -C "$W/bundle" -cf "${OUT%.tar}.bundle.tar" .
chmod 644 "$OUT" "${OUT%.tar}.bundle.tar"
echo "$REF $(du -h "$OUT" | cut -f1) $OUT"
echo "$(sha256sum "${OUT%.tar}.bundle.tar" | cut -d' ' -f1) $(du -h "${OUT%.tar}.bundle.tar" | cut -f1) ${OUT%.tar}.bundle.tar"
