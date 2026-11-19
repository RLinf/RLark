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

For repeatable tests against the project's long-running multi-cluster Kind
topology, see [End-to-end testing](e2e-testing.md). The E2E framework keeps
cluster paths, images, timeouts, and data-plane allowlists configurable.

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
  --leader-election=false \
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

After changing CRD types, regenerate the CRD manifests, Go clients, schema
reference, and frontend TypeScript models:

```bash
# Regenerate all CRD-derived artifacts
make generate

# Regenerate typed clients, informers, and listers
make -C api generate-clients
```

Always test user-facing changes in both the API and Web UI.

`make verify-generated` runs the same generators and fails when the committed
CRD manifests, deepcopy functions, typed Go clients, frontend TypeScript models,
or `apps/rlark/docs/reference/crd.md` differ from the regenerated output, or when
generation produces untracked files.
CI runs it on every change, so run it locally before pushing API changes.

Frontend models are generated from the checked-in manifests in
`api/config/crd/bases` into `apps/rlark-ui/src/generated/rlark`. The generated
files are committed and must not be edited manually. Frontend code should import
CRD interfaces through `apps/rlark-ui/src/generated/rlark-types.ts`; keep page and
domain view models separate from these wire types. The generator follows the CRD
OpenAPI schema as emitted by `controller-gen`, including expanded Kubernetes
types such as `PodTemplateSpec`, `IntOrString`, and resource quantities.

The generator and its runtime packages are pinned in
`apps/rlark-ui/package.json` and require Node.js 22. The repository wrapper calls
the generator API directly and reads CRD files in sorted order:

```bash
npm ci --prefix apps/rlark-ui
npm run generate:crd-models --prefix apps/rlark-ui
```

### CRD generators and the pinned toolchain

`controller-gen` is pinned by `CONTROLLER_TOOLS_VERSION` in `api/Makefile` and
installed into the gitignored, repository-local `api/bin` under a
version-stamped file name. The client generators invoked by
`api/hack/generate-clients.sh` are pinned by `KUBE_CODEGEN_TAG` in that script
and installed into the same directory. Scripts that need the generator directly
run `make -C api controller-gen` to install it and
`make -s -C api print-controller-gen` to resolve its path.

Do not point `CONTROLLER_GEN` at a generator in a shared `GOPATH/bin`. That
directory is shared across jobs and projects in CI, so a file named
`controller-gen` there may be stale or not even executable while make still
treats the install as done and skips it, which fails mid-generation with exit
code 127. The install rule runs `controller-gen --version` after installing, so
an unusable binary fails at install time instead.

Bumping `CONTROLLER_TOOLS_VERSION` is a deliberate change: the version is
recorded in the header of every generated CRD manifest, so regenerate all
artifacts and commit them together with the pin.

### Protobuf stubs and the pinned toolchain

The gRPC stubs under `sdks/embodied-runtime-go/gen`, `sdks/embodied-runtime-python/embodied_runtime/gen`, and `sdks/execution-go/gen` are checked in, and every generated file records the generator version in its header. Generating with a different `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`, or `grpcio-tools` rewrites all of them, so the whole repository uses one pinned toolchain:

- `proto/toolchain.mk` is the single source of truth for `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`, and `buf`. Both proto trees (`proto/embodied-runtime` and `proto/execution`) include it.
- `sdks/embodied-runtime-python/requirements-dev.txt` pins `grpcio-tools` for the Python stubs.

Do not generate with whatever `protoc` happens to be on `PATH`. Install the pinned release into the gitignored `proto/bin` directory first; the Go plugins and `buf` are installed there automatically, with version-stamped file names:

```bash
# Download the pinned protoc release into proto/bin
make protoc-install

# Regenerate every stub tree
make proto

# Fail if the checked-in stubs do not match the .proto sources
make verify-proto-generated
```

`make proto` runs `protoc-check` first and fails when the resolved `protoc` does not report the pinned version, so a mismatched system install can never silently rewrite the stubs. `proto/bin/protoc` takes precedence over `PATH` once it exists.

If `python3` is not the interpreter that has the pinned `grpcio-tools` installed, point the Python generator at the right one instead of installing the pin globally:

```bash
python3 -m venv /tmp/rlark-protogen
/tmp/rlark-protogen/bin/pip install --requirement sdks/embodied-runtime-python/requirements-dev.txt
make proto PYTHON=/tmp/rlark-protogen/bin/python
```

!!! warning "For coding agents"
    Never regenerate protobuf stubs with an unpinned `protoc`, and never relax the pins in `proto/toolchain.mk` to match a locally installed version. Run `make protoc-install` to place the pinned release in the repository-local `proto/bin` directory, and use `PYTHON=<interpreter>` for the Python generator when needed. A diff that rewrites every generated file is a toolchain mismatch, not a real change. Bumping a pin is a deliberate change that must regenerate all stub trees in the same commit.

## IDE Setup

Recommended VS Code extensions:

- Go
- ESLint
- Prettier
- YAML
