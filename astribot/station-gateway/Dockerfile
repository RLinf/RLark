# Build from the repository root.
# The images it starts from and where Go modules come from, as a release's build names them (astribot/release/images: the
# office registry's mirror/…, goproxy.cn).
ARG GO_IMAGE=golang:1.26
# The runtime is kube-proxy's image for its iptables, which a node's forwarding is kept with (forward.go): in the
# backend the node's own rules are in, as kube-proxy chooses it (iptables-wrapper). Its kube-proxy is not run.
ARG RUNTIME_IMAGE=registry.k8s.io/kube-proxy:v1.36.6
FROM ${GO_IMAGE} AS build
ARG GOPROXY
WORKDIR /src
COPY astribot/station-gateway/ ./
RUN CGO_ENABLED=0 GOWORK=off go build -trimpath -o /station-gateway .
FROM ${RUNTIME_IMAGE}
USER 0:0
COPY --from=build /station-gateway /station-gateway
ENTRYPOINT ["/station-gateway"]
