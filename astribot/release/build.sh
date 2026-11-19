#!/bin/bash
# Builds a release, or the installer's base, on the office's management machine from this clone of RLark:
#   astribot/release/build.sh RLARK_COMMIT FLEET_COMMIT   the release ci-<r7>-<f7>; its file comes back into astribot/release/releases/
#   astribot/release/build.sh base RLARK_COMMIT            the installer's base of the commit's Dockerfile.base
# The commit's tree (git archive) goes to the machine (ENTRY, its ssh destination, whose user runs sudo) into
# /var/lib/rlark/release/builds/<name>, and the tree's own run.sh builds it there (see run.sh for what a build is and
# how it goes live). One build at a time: the machine's lock (/var/lib/rlark/release/builds/lock) is taken before the
# build's directory is laid down and held until run.sh ends, so a build started again while one runs leaves that one
# alone. FLEET_COMMIT is Fleet's commit in full, whose images Fleet's build Jobs made; a release of RLark alone names
# the live release's (its source.fleet). BUILD_MEMORY, BUILD_RESERVE, BUILD_CPUS, BUILD_PARALLELISM and BUILD_WAIT set here are
# run.sh's on the machine. A build takes long: run it where the ssh session is not lost (tmux); its log
# stays in the build's directory either way.
set -euo pipefail
entry=${ENTRY:-cailiang_rlark}
cd "$(dirname "$0")/../.."
if [[ $1 == base ]]; then
  rlark=$(git rev-parse --verify "${2:?the RLark commit}^{commit}")
  name=base-${rlark:0:7} run=(base)
else
  rlark=$(git rev-parse --verify "$1^{commit}") fleet=${2:?the Fleet commit, in full}
  [[ $fleet =~ ^[0-9a-f]{40}$ ]] || { echo "Fleet's commit is named in full (40 hex digits), not $fleet" >&2; exit 1; }
  name=ci-${rlark:0:7}-${fleet:0:7} run=(release)
fi
builds=/var/lib/rlark/release/builds work=/var/lib/rlark/release/builds/$name
run+=("$work" "$rlark" ${fleet:+"$fleet"})
limits=$(env | grep -E '^BUILD_(MEMORY|RESERVE|CPUS|PARALLELISM|WAIT)=[0-9]+$' | tr '\n' ' ' || true)
remote="mkdir -p $builds && exec 9> $builds/lock && { flock -n 9 || { echo 'another build holds $builds/lock' >&2; exit 1; }; } && rm -rf $work && mkdir -p $work/rlark && cat > $work/rlark.tar && tar -C $work/rlark -xf $work/rlark.tar && exec env $limits $work/rlark/astribot/release/run.sh ${run[*]}"
git archive "$rlark" | ssh -o ServerAliveInterval=30 "$entry" sudo bash -c "$(printf %q "$remote")"
[[ $1 == base ]] && exit 0
ssh "$entry" sudo cat "$work/rlark/astribot/release/releases/$name.yaml" > "astribot/release/releases/$name.yaml"
echo "astribot/release/releases/$name.yaml: commit it once it is live, with environments/live.yaml's release: naming it"
