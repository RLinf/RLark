#!/bin/bash
# The images of the isolated run, from this tree with the local Go toolchain
# (no base image is pulled): the four of Station and station-v2/rlark, RLark's
# own programs in one image as astribot/release/rlark.Dockerfile lays them out.
#   build-local.sh TAG
set -euo pipefail
TAG=$1
REPO=$(cd "$(dirname "$0")/../../../.." && pwd)
W=$(mktemp -d); trap 'rm -rf "$W"' EXIT
image() { # name entrypoint user directory package
  mkdir -p "$W/$1$(dirname "$2")" "$W/$1/tmp" && chmod 1777 "$W/$1/tmp"
  (cd "$REPO/$4" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$W/$1$2" "$5")
  printf 'FROM scratch\nCOPY . /\nUSER %s\n' "$3" > "$W/$1.Dockerfile"
  # station-services holds several programs in the real image and is started by path.
  [[ $1 == station-services ]] || printf 'ENTRYPOINT ["%s"]\n' "$2" >> "$W/$1.Dockerfile"
  docker build -q -f "$W/$1.Dockerfile" -t "station-v2/$1:$TAG" "$W/$1" >/dev/null
}
image station-services /usr/local/bin/station-api 65532 apps/rlark ./astribot/cmd/station-api
mkdir -p "$W/station-controller/usr/share/rlark" && cp -r "$REPO/apps/rlark/pkg/addons/catalog" "$W/station-controller/usr/share/rlark/addons"
image station-controller /station-controller 65532 astribot/station ./cmd/station-controller
image station-agent /usr/local/bin/station-agent 65532 apps/rlark ./astribot/cmd/station-agent
image station-wireguard /station-gateway 0 astribot/station-gateway .
# RLark: server, controller-manager, gateway, the agents and their tools.
mkdir -p "$W/rlark/usr/local/bin" "$W/rlark/tmp" && chmod 1777 "$W/rlark/tmp"
for program in $(cd "$REPO/apps/rlark" && ls -d cmd/* astribot/cmd/* | grep -v crd-api-docgen); do
  (cd "$REPO/apps/rlark" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$W/rlark/usr/local/bin/${program##*/}" "./$program")
done
printf 'FROM alpine:3.20\nCOPY . /\n' > "$W/rlark.Dockerfile"
docker build -q -f "$W/rlark.Dockerfile" -t "station-v2/rlark:$TAG" "$W/rlark" >/dev/null
# The console, built with the local npm and served as astribot/release/rlark-ui.Dockerfile serves it; nginx is the one base image.
(cd "$REPO/apps/rlark-ui" && npm ci --no-audit --no-fund >/dev/null && npm run build >/dev/null)
mkdir -p "$W/rlark-ui" && cp -r "$REPO/apps/rlark-ui/dist" "$W/rlark-ui/" && cp "$REPO/astribot/release/rlark-ui.nginx.conf" "$W/rlark-ui/nginx.conf"
printf 'FROM nginx:1.29.1\nENV RLARK_GATEWAY_URL=http://rlark-gateway.rlark-system.svc:8090\nCOPY nginx.conf /etc/nginx/templates/default.conf.template\nCOPY dist /usr/share/nginx/html\n' > "$W/rlark-ui.Dockerfile"
docker build -q -f "$W/rlark-ui.Dockerfile" -t "station-v2/rlark-ui:$TAG" "$W/rlark-ui" >/dev/null
docker images --format '{{.Repository}}:{{.Tag}} {{.Size}}' | grep "^station-v2/.*:$TAG"
