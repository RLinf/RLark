# The images it starts from and where Go modules come from, as a release's build names them (astribot/release/images: the
# office registry's mirror/…, goproxy.cn).
ARG GO_IMAGE=golang:1.26
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot
FROM ${GO_IMAGE} AS build
ARG GOPROXY
WORKDIR /src
COPY astribot/station/go.mod astribot/station/go.sum ./astribot/station/
COPY api/go.mod api/go.sum ./api/
COPY api/ ./api/
COPY astribot/station/ ./astribot/station/
WORKDIR /src/astribot/station
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /station-controller ./cmd/station-controller

FROM ${RUNTIME_IMAGE}
COPY --from=build /station-controller /station-controller
# What the built-in addons publish for machines that are not nodes yet (a NodeClass's uplink.setup).
COPY apps/rlark/pkg/addons/catalog /usr/share/rlark/addons
USER 65532:65532
ENTRYPOINT ["/station-controller"]
