# Build from the repository root.
# The images it starts from and where Go modules come from, as a release's build names them (astribot/release/images: the
# office registry's mirror/…, goproxy.cn).
ARG GO_IMAGE=golang:1.26
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot
FROM ${GO_IMAGE} AS build
ARG GOPROXY
WORKDIR /src
COPY astribot/station-gateway/ ./
RUN CGO_ENABLED=0 GOWORK=off go build -trimpath -o /station-gateway .
FROM ${RUNTIME_IMAGE}
USER 0:0
COPY --from=build /station-gateway /station-gateway
ENTRYPOINT ["/station-gateway"]
