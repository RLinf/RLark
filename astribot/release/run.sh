#!/bin/bash
# A build on the office's management machine (10.11.14.128), as root, of the tree build.sh sent into WORK
# (/var/lib/rlark/release/builds/<name>: rlark.tar, the git archive of RLARK_COMMIT, unpacked into rlark/). build.sh
# holds /var/lib/rlark/release/builds/lock from before it lays WORK down until this ends: one build at a time.
#   run.sh release WORK RLARK_COMMIT FLEET_COMMIT   the release ci-<r7>-<f7>
#   run.sh base WORK RLARK_COMMIT                   the installer's base of the tree's Kubernetes version
# A release:
#   1. the public images the Dockerfiles start from, into the office registry's mirror/… (the file mirror, skopeo);
#   2. RLark's tests (Dockerfile.test, s1u-robot's --target test), then its images (the file images), pushed into the
#      office registry at its local entry (127.0.0.1:5443, which every node names registry.rlark.internal:5443);
#   3. the release's command: rlarkctl of the rlark-server image built (WORK/rlarkctl);
#   4. the live environment's installer and upgrade bundle: Dockerfile.installer over environments/live.yaml's
#      installer.base, with its registry's CA, over the tree's installer images of each platform (rlarkctl release
#      archive), as an artifact (installer-live) and as an image (rlark/node-bootstrap, hack/upgrade-cluster.sh IMAGE);
#   5. Fleet's images of FLEET_COMMIT, each the base release has, which Fleet's build Jobs push into the cloud's
#      registry (fleet/<image>:<f7>), copied into the office registry by their digests; one not there stops the build
#      (a release of RLark alone names the base's Fleet commit, whose images are there);
#   6. the release: environments/live.yaml's with these images and the tree's pins and Addon versions (rlarkctl
#      release candidate), WORK/rlark/astribot/release/releases/<name>.yaml;
#   7. every image built here copied by digest into the cloud's registry (registry.source), which the cloud's cluster
#      pulls from and Station's installer is read from (rlarkctl release copy).
# The base: Dockerfile.base into rlark/staging-artifact:install-<version>-<r7>, an environment's next installer.base.
# BuildKit runs in a container of BUILD_MEMORY GB (6), BUILD_CPUS CPUs (4) and BUILD_PARALLELISM steps and Go packages
# at once (2): the machine is cailiang's desktop, often a few GB from running out of memory and without swap, and a
# build step starts only once BUILD_MEMORY GB and BUILD_RESERVE GB (3) more for the desktop are available, waited for
# up to BUILD_WAIT minutes (120). Fleet's own BuildKit on this machine is not bounded by these: the two are not run at
# once. The build's log is WORK/build.log.
set -euo pipefail
what=$1 WORK=$2 RLARK_COMMIT=$3 FLEET_COMMIT=${4:-}
MEMORY=${BUILD_MEMORY:-6} RESERVE=${BUILD_RESERVE:-3} CPUS=${BUILD_CPUS:-4} PARALLELISM=${BUILD_PARALLELISM:-2} WAIT=${BUILD_WAIT:-120}
LOCAL=127.0.0.1:5443 REGISTRY=registry.rlark.internal:5443
TREE=$WORK/rlark RELEASE=$WORK/rlark/astribot/release
exec > >(tee -a "$WORK/build.log") 2>&1
failed() { tail -50 "$WORK/$1"; echo "FAILED: $2 ($WORK/$1)"; exit 1; }
memory() {
  local available waited=0
  while available=$(awk '/^MemAvailable:/ {print int($2 / 1048576)}' /proc/meminfo); (( available < MEMORY + RESERVE )); do
    (( waited < WAIT * 60 )) ||
      { echo "stopped before $1: $available GB of memory available after $WAIT minutes, $((MEMORY + RESERVE)) GB needed (BUILD_MEMORY, BUILD_RESERVE, BUILD_WAIT)"; return 1; }
    (( waited % 600 )) || echo "waiting before $1: $available GB of memory available, $((MEMORY + RESERVE)) GB needed"
    sleep 30; waited=$((waited + 30))
  done
}

# 1. The base images the registry lacks, copied as they are (Docker Hub limits what is asked of it).
while read -r mirror source; do
  [[ -n $mirror && $mirror != \#* ]] || continue
  skopeo inspect --raw --tls-verify=false "docker://$LOCAL/${mirror%%:*}@${source#*@}" > /dev/null 2>&1 ||
    skopeo --insecure-policy copy -q --preserve-digests --retry-times 3 --dest-tls-verify=false "docker://$source" "docker://$LOCAL/$mirror"
done < "$TREE/astribot/release/mirror"

# BuildKit, bounded, reading the office registry's mirror/… at its local entry.
memory BuildKit || exit 1
cat > "$WORK/buildkitd.toml" <<TOML
[worker.oci]
  max-parallelism = $PARALLELISM
  gc = true
  reservedSpace = "10GB"
  maxUsedSpace = "40GB"
[registry."$REGISTRY"]
  mirrors = ["$LOCAL"]
[registry."$LOCAL"]
  http = true
TOML
buildkit=$(awk '$1 ~ /^mirror\/buildkit:/ {split($1, name, ":"); split($2, source, "@"); print name[1] "@" source[2]}' "$TREE/astribot/release/mirror")
docker rm -f rlark-build > /dev/null 2>&1 || true
docker run -d --name rlark-build --privileged --network host --memory "${MEMORY}g" --memory-swap "${MEMORY}g" --cpus "$CPUS" \
  -v /var/lib/rlark/release/buildkit:/var/lib/buildkit -v "$WORK:$WORK" \
  "$LOCAL/$buildkit" --config "$WORK/buildkitd.toml" --allow-insecure-entitlement network.host > /dev/null
trap 'docker rm -f rlark-build > /dev/null' EXIT
timeout 120 bash -c 'until docker exec rlark-build buildctl debug workers > /dev/null 2>&1; do sleep 1; done'
# Every file an image holds, and the image itself, is dated 0 (SOURCE_DATE_EPOCH, rewrite-timestamp): the same content
# is the same image whichever commit it is of, and a release changes only the components whose content changed.
build() { # <Dockerfile> [buildctl options]; once more if it fails: a package mirror that drops a connection (goproxy.cn's
  # GOAWAY, 11-16) fails a download that the second attempt takes from BuildKit's cache onwards.
  local attempt
  for attempt in 1 2; do
    memory "$1" || return 1
    docker exec rlark-build buildctl build --progress plain --frontend dockerfile.v0 --local "context=$TREE" --local "dockerfile=$TREE/$(dirname "$1")" \
      --opt "filename=$(basename "$1")" --opt build-arg:SOURCE_DATE_EPOCH=0 "${@:2}" && return
  done
  return 1
}
push() { echo "type=image,name=$LOCAL/$1,push=true,registry.insecure=true,rewrite-timestamp=true"; }
digest() { skopeo inspect --tls-verify=false --format '{{.Digest}}' "docker://$LOCAL/$1"; }

if [[ $what == base ]]; then
  tag=install-$(sed -n 's/^ARG VERSION=//p' "$TREE/astribot/station/bootstrap/Dockerfile.base")-${RLARK_COMMIT:0:7}
  # Its images are read from the office registry's local entry, which the build's steps reach on the host's network.
  build astribot/station/bootstrap/Dockerfile.base --allow network.host \
    --output "$(push "rlark/staging-artifact:$tag")" > "$WORK/build-base.log" 2>&1 || failed build-base.log "the installer's base"
  echo "base $REGISTRY/rlark/staging-artifact@$(digest "rlark/staging-artifact:$tag"): environments/live.yaml's installer.base, for the releases built after it"
  exit 0
fi

NAME=ci-${RLARK_COMMIT:0:7}-${FLEET_COMMIT:0:7} tag=ci-${RLARK_COMMIT:0:7}
BASE=$RELEASE/environments/$(awk '/^release:/ {print $2}' "$RELEASE/environments/live.yaml")
# 2. Tests, then images.
build astribot/release/Dockerfile.test --opt "build-arg:GO_IMAGE=$REGISTRY/mirror/golang@sha256:6327cb290853a5d13091c436b88259b4f35ff70e49b1f8d9e5d1dd1757e9c6ba" \
  --opt "build-arg:NODE_IMAGE=$REGISTRY/mirror/node@sha256:c385ec44d77c785e2364ac0c9b150809a0fdc17fde3dbf061e3dad07242c6a85" \
  --opt build-arg:GOPROXY=https://goproxy.cn --opt "build-arg:GOFLAGS=-p=$PARALLELISM" --opt build-arg:NPM_REGISTRY=https://registry.npmmirror.com/ \
  --output "type=local,dest=$WORK/tests" > "$WORK/tests.log" 2>&1 || failed tests.log "RLark's tests"
build astribot/s1u/Dockerfile --opt target=test > "$WORK/tests-s1u.log" 2>&1 || failed tests-s1u.log "s1u-robot's tests"
echo "tests passed: $(grep -c '^ok' "$WORK/tests/go-tests.log") Go packages, the console's, s1u-robot's ($(grep -o '[0-9]* passed' "$WORK/tests-s1u.log" | tail -1))"
images=() built=()
while read -r repository dockerfile components options; do
  [[ -n $repository && $repository != \#* ]] || continue
  read -ra options <<< "${options:-}"
  build "$dockerfile" "${options[@]}" --output "$(push "$repository:$tag")" > "$WORK/build-${repository#*/}.log" 2>&1 || failed "build-${repository#*/}.log" "the image $repository"
  reference=$REGISTRY/$repository@$(digest "$repository:$tag")
  built+=("$reference")
  for component in ${components//,/ }; do images+=(--image "$component=$reference"); done
  echo "image $reference"
done < "$TREE/astribot/release/images"

# 3. The release's command.
server=$(printf '%s\n' "${images[@]}" | sed -n 's/^rlark-server=//p')
rm -rf "$WORK/server"
skopeo --insecure-policy copy -q --src-tls-verify=false "docker://$LOCAL/${server#"$REGISTRY"/}" "oci:$WORK/server:release"
manifest=$(jq -r '.manifests[0].digest' "$WORK/server/index.json")
for layer in $(jq -r '.layers[].digest' "$WORK/server/blobs/sha256/${manifest#sha256:}" | tac); do
  tar -xf "$WORK/server/blobs/sha256/${layer#sha256:}" -O usr/local/bin/rlarkctl > "$WORK/rlarkctl" 2> /dev/null && [[ -s $WORK/rlarkctl ]] && break
done
[[ -s $WORK/rlarkctl ]] || { echo "FAILED: $server holds no usr/local/bin/rlarkctl"; exit 1; }
chmod 755 "$WORK/rlarkctl" && rm -rf "$WORK/server"
RLARKCTL=$WORK/rlarkctl
echo "the release's command: rlarkctl of $server"

# 4. The live environment's installer and upgrade bundle.
rm -rf "$WORK/installer-images"
for platform in linux/amd64 linux/arm64; do
  mkdir -p "$WORK/installer-images/${platform/\//-}"
  "$RLARKCTL" release archive --registry-address "http://$LOCAL" --tree "$WORK/rlark.tar" --platform "$platform" \
    --output "$WORK/installer-images/${platform/\//-}/installer.tar" > "$WORK/installer-images-${platform/\//-}.log" 2>&1 ||
    failed "installer-images-${platform/\//-}.log" "the installer's images of $platform"
done
installer() { awk -v key="$1" '/^installer:/ {inside = 1; next} inside && /^[^ #]/ {exit} inside && $1 == key":" {print $2; exit}' "$RELEASE/environments/live.yaml"; }
for target in artifact image; do
  repository=rlark/staging-artifact; [[ $target == image ]] && repository=rlark/node-bootstrap
  build astribot/station/bootstrap/Dockerfile.installer --local "installer-images=$WORK/installer-images" --opt context:installer-images=local:installer-images \
    --opt "target=$target" --opt "build-arg:BASE=$(installer base)" --opt "build-arg:SUFFIX=${RLARK_COMMIT:0:7}-live" \
    --opt "build-arg:REGISTRY_CA=$(base64 -w0 "$RELEASE/environments/$(installer registryCA)")" \
    --output "$(push "$repository:$tag-installer-live")" > "$WORK/build-installer-$target.log" 2>&1 || failed "build-installer-$target.log" "the installer, $target"
  reference=$REGISTRY/$repository@$(digest "$repository:$tag-installer-live")
  built+=("$reference")
  [[ $target == artifact ]] && images+=(--image "installer-live=$reference")
  echo "installer ($target) $reference"
done
docker rm -f rlark-build > /dev/null && trap - EXIT
rm -rf "$WORK/installer-images"

# 5. Fleet's images, those the base release has; a release of RLark alone (the base's Fleet commit) keeps the base's.
cd "$RELEASE"
if [[ $FLEET_COMMIT == $(awk '/^source:/ {inside = 1; next} inside && $1 == "fleet:" {print $2; exit}' "$BASE") ]]; then
  echo "Fleet's images of ${FLEET_COMMIT:0:7}: the base release's"
else
  for image in robot run-gpu; do
    grep -q "^  fleet-$image:" "$BASE" || continue
    reference=$("$RLARKCTL" release copy -e environments/live.yaml --to site "fleet/$image:${FLEET_COMMIT:0:7}") ||
      { echo "FAILED: Fleet's $image of ${FLEET_COMMIT:0:7}, copied from the cloud's registry"; exit 1; }
    images+=(--image "fleet-$image=$reference")
    echo "Fleet's $image of ${FLEET_COMMIT:0:7}: $reference"
  done
fi

# 6. The release.
CANDIDATE=$("$RLARKCTL" release candidate --base "$BASE" --tree "$WORK/rlark.tar" --name "$NAME" --fleet "$FLEET_COMMIT" "${images[@]}")
echo "release $CANDIDATE"

# 7. The images built here, in the cloud's registry too.
"$RLARKCTL" release copy -e environments/live.yaml --to source "${built[@]}"

cat <<EOF
built $NAME in $WORK. Going live, on this machine as root (--dry-run first prints what each stage would change):
  cd $RELEASE && $RLARKCTL release apply -e environments/live.yaml -r releases/$NAME.yaml --tree $WORK/rlark.tar --dry-run
  cd $RELEASE && $RLARKCTL release apply -e environments/live.yaml -r releases/$NAME.yaml --tree $WORK/rlark.tar
Then releases/$NAME.yaml is committed beside the others, environments/live.yaml's release: naming it.
EOF
