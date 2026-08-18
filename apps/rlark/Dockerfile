# -- Base image arguments --------------------------------------------------
ARG GO_BASE_IMAGE=golang:1.26-alpine
ARG RUNTIME_BASE_IMAGE=ubuntu:22.04

# -- Build stage -----------------------------------------------------------
FROM ${GO_BASE_IMAGE} AS builder

WORKDIR /src
COPY api/go.mod api/go.sum ./api/
COPY apps/rlark/go.mod apps/rlark/go.sum ./apps/rlark/
RUN --mount=type=cache,target=/go/pkg/mod \
    GOMODCACHE=/go/pkg/mod go mod download -C apps/rlark

COPY api/ ./api/
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
    CGO_ENABLED=0 GOOS=linux go build -o /rlark-sshd          ./cmd/sshd

# -- Runtime stage ---------------------------------------------------------
FROM ${RUNTIME_BASE_IMAGE}

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /server            /usr/local/bin/server
COPY --from=builder /controller-manager /usr/local/bin/controller-manager
COPY --from=builder /gateway            /usr/local/bin/gateway
COPY --from=builder /agent              /usr/local/bin/agent
COPY --from=builder /network-sidecar    /usr/local/bin/network-sidecar
COPY --from=builder /rlarkadm           /usr/local/bin/rlarkadm
COPY --from=builder /rlark-sshd         /usr/local/bin/rlark-sshd
