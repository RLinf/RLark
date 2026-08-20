# Local Development and Debugging

## Prerequisites

- Go 1.26+
- Node.js 18+
- Docker and Docker Compose
- kind (for local Kubernetes clusters)
- jq (for JSON processing in scripts)

## Quick Start Environment

The one-click [Quick Start](../quickstart.md) script provides a complete development environment with Docker Compose (kcp + PostgreSQL) and two kind clusters. This is useful for integration testing but is not a production deployment.

```bash
bash apps/rlark/docs/examples/quickstart.sh
```

## Building Go Components

Build all components:

```bash
make build
```

Individual components:

```bash
# Control plane
go build -o bin/rlark-server ./apps/rlark/cmd/server
go build -o bin/rlark-gateway ./apps/rlark/cmd/gateway
go build -o bin/rlark-controller-manager ./apps/rlark/cmd/controller-manager

# Data plane
go build -o bin/rlark-agent ./apps/rlark/cmd/agent
go build -o bin/rlark-network-sidecar ./apps/rlark/cmd/network-sidecar

# Tools
go build -o bin/rlarkadm ./apps/rlark/cmd/rlarkadm
go build -o bin/rlarkctl ./apps/rlark/cmd/rlarkctl
```

## Running Components Locally

### Server

```bash
rlark-server \
  --auto-sign-tls-ca-cert \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --kubeconfig=/tmp/rlark/admin.kubeconfig
```

### Gateway

```bash
rlark-gateway \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --server-address=https://localhost:8443
```

### Controller Manager

```bash
rlark-controller-manager \
  --server-address=https://localhost:8443 \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --leader-elect=false \
  --metrics-bind-address=:0 \
  --health-probe-bind-address=:0
```

### Agent

```bash
rlark-agent \
  --mode=both \
  --server-address=https://localhost:8443 \
  --client-cert=/tmp/rlark/agent-certs/cert.pem \
  --client-key=/tmp/rlark/agent-certs/key.pem \
  --ca-cert=/tmp/rlark/agent-certs/ca-cert.pem \
  --image=rlark:latest
```

## Web UI Development

```bash
cd apps/rlark-ui
npm install
npm run dev
```

The UI runs on `http://localhost:5173` by default.

### Data Modes

The Web UI uses `VITE_DATA_MODE` to select its data source:

| Mode | Value | Description |
|------|-------|-------------|
| Mock | `mock` (default) | Uses mock data for UI development and screenshots |
| Backend | `backend` | Connects to the real Gateway API |

```bash
# Development with mock data
npm run dev

# Development with real backend
VITE_DATA_MODE=backend npm run dev
```

!!! warning "Mock data limitations"
    Mock data is only for UI development and documentation screenshots. It must not be treated as real resource state. Always use backend mode when validating clusters, nodes, Jobs, storage, or capacity.

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./apps/rlark/pkg/... -v

# Run a single test
go test ./apps/rlark/pkg/... -run TestName -v

# Run with race detection
go test -race ./apps/rlark/pkg/...
```

### Linting

```bash
# Go linting
make lint-go

# Frontend linting
make lint-web

# All linting
make lint
```

## Debugging

### Structured Logging

All components use structured logging. Increase log verbosity:

```bash
# Most components use the standard Go log package
# Check component-specific flags for log level configuration
```

### Kubernetes Events

Controller-manager and agent emit Kubernetes events for reconciliation. Check events:

```bash
kubectl get events --sort-by='.lastTimestamp'
```

### CR Status Transitions

Monitor CR status transitions to trace reconciliation flow:

```bash
kubectl get job <name> -o yaml | grep -A20 status
kubectl get task <name> -o yaml | grep -A20 status
```

### Common Issues

| Issue | Check |
|-------|-------|
| Agent not connecting | Verify TLS certificates, server address, and network connectivity |
| Tasks not created | Check controller-manager logs, verify Job namespace matches Node namespace |
| Cross-cluster network fails | Verify Domain CRD, NodeServer socket, and SSH tunnel configuration |
| UI shows no data | Confirm `VITE_DATA_MODE=backend` and Gateway is accessible |
| Database connection errors | Verify `db-config.yaml` credentials and PostgreSQL is running |

### Frontend Debugging

Language, theme, and sidebar state are stored in the browser. Lists and details use stable URLs for refresh, sharing, and back navigation.

```bash
# Clear browser state
localStorage.clear()
```

## Keeping Generated Code in Sync

After changing CRD types, regenerate API clients:

```bash
# Regenerate CRD manifests
make generate

# Regenerate typed clients, informers, and listers
make -C api generate-clients
```

Always test user-facing changes in both the API and Web UI.

## IDE Setup

Recommended VS Code extensions:

- Go
- ESLint
- Prettier
- YAML