# -- Base image arguments --------------------------------------------------
ARG GO_BASE_IMAGE=golang:1.26-alpine
ARG RUNTIME_BASE_IMAGE=ubuntu:22.04

# -- Build stage -----------------------------------------------------------
FROM ${GO_BASE_IMAGE} AS builder
# Where Go modules come from: the build site names its proxy (goproxy.cn); empty is Go's own default.
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
    cd apps/rlark && \
    CGO_ENABLED=0 GOOS=linux go build -o /server             ./cmd/server/... && \
    CGO_ENABLED=0 GOOS=linux go build -o /controller-manager ./cmd/controller-manager/... && \
    CGO_ENABLED=0 GOOS=linux go build -o /gateway            ./cmd/gateway/... && \
    CGO_ENABLED=0 GOOS=linux go build -o /agent              ./cmd/agent/... && \
    CGO_ENABLED=0 GOOS=linux go build -o /network-sidecar    ./cmd/network-sidecar/... && \
    CGO_ENABLED=0 GOOS=linux go build -o /rlarkadm           ./cmd/rlarkadm && \
    CGO_ENABLED=0 GOOS=linux go build -o /rlarkctl           ./cmd/rlarkctl && \
    CGO_ENABLED=0 GOOS=linux go build -o /rlark-tools        ./cmd/rlark-tools && \
    CGO_ENABLED=0 GOOS=linux go build -o /csi-nodepath       ./astribot/cmd/csi-nodepath && \
    CGO_ENABLED=0 GOOS=linux go build -o /station-api        ./astribot/cmd/station-api && \
    CGO_ENABLED=0 GOOS=linux go build -o /astribot-iam       ./astribot/cmd/astribot-iam

# -- Runtime stage ---------------------------------------------------------
FROM ${RUNTIME_BASE_IMAGE}

# The CA certificates the programs trust: the Go image's bundle (Mozilla's, as Ubuntu's ca-certificates is), so the
# image is built without a package index.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /server            /usr/local/bin/server
COPY --from=builder /controller-manager /usr/local/bin/controller-manager
COPY --from=builder /gateway            /usr/local/bin/gateway
COPY --from=builder /agent              /usr/local/bin/agent
COPY --from=builder /network-sidecar    /usr/local/bin/network-sidecar
COPY --from=builder /rlarkadm           /usr/local/bin/rlarkadm
COPY --from=builder /rlarkctl           /usr/local/bin/rlarkctl
COPY --from=builder /rlark-tools        /usr/local/bin/rlark-tools
COPY --from=builder /csi-nodepath       /usr/local/bin/csi-nodepath
COPY --from=builder /station-api        /usr/local/bin/station-api
COPY --from=builder /astribot-iam       /usr/local/bin/astribot-iam
