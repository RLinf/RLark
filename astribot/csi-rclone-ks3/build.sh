#!/usr/bin/env bash
# Builds the rclone CSI node plugin with the KS3 patch into ./rcloneplugin of a new checkout.
# usage: build.sh /absolute/new/build-directory
set -euo pipefail
source_root=${1:?usage: build.sh /absolute/new/build-directory}
patch_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
base=5c9e16bbed61909dbe6e1a1b113e6037f550dd28 # upstream v0.2.0, rclone v1.71.2
version=v0.2.0-ks3.3
git init --quiet "$source_root"
cd -- "$source_root"
git fetch --quiet --depth 1 https://github.com/veloxpack/csi-driver-rclone.git "$base"
git checkout --quiet FETCH_HEAD
git apply "$patch_root/patches/ks3-integrity.patch"
go test -mod=vendor github.com/rclone/rclone/backend/s3 -run TestKS3 -count=1
go test -mod=vendor -tags all ./pkg/rclone -count=1
# "all" compiles every rclone backend in, as the upstream image does; without it the plugin has none.
CGO_ENABLED=0 go build -mod=vendor -trimpath -tags "netgo all" \
    -ldflags "-s -w -extldflags '-static' -X github.com/veloxpack/csi-driver-rclone/pkg/rclone.driverVersion=$version -X github.com/veloxpack/csi-driver-rclone/pkg/rclone.gitCommit=${base:0:8} -X github.com/veloxpack/csi-driver-rclone/pkg/rclone.rcloneVersion=1.71.2" \
    -o rcloneplugin ./cmd/rcloneplugin
