# astribot-iam 的独立镜像： rlark/agent 镜像以外的快速滚动通道（服务只有一个二进制 + CA）。
# -- Base image arguments --------------------------------------------------
ARG GO_BASE_IMAGE=golang:1.27-alpine
ARG RUNTIME_BASE_IMAGE=ubuntu:22.04

FROM ${GO_BASE_IMAGE} AS builder
ARG GOPROXY
WORKDIR /src
COPY api/go.mod api/go.sum ./api/
COPY sdks/execution-go/go.mod sdks/execution-go/go.sum ./sdks/execution-go/
COPY apps/rlark/go.mod apps/rlark/go.sum ./apps/rlark/
RUN --mount=type=cache,target=/go/pkg/mod \
    GOMODCACHE=/go/pkg/mod go mod download -C apps/rlark
COPY api/ ./api/
COPY sdks/execution-go/ ./sdks/execution-go/
COPY apps/rlark/ ./apps/rlark/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOMODCACHE=/go/pkg/mod GOCACHE=/root/.cache/go-build \
    cd apps/rlark && CGO_ENABLED=0 GOOS=linux go build -o /astribot-iam ./astribot/cmd/astribot-iam

FROM ${RUNTIME_BASE_IMAGE}
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /astribot-iam /usr/local/bin/astribot-iam
