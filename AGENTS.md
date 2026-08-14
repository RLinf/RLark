# AGENTS.md

Brief for AI coding agents working on RLark. For full contribution flow, code style, and PR process see [CONTRIBUTING.md](CONTRIBUTING.md).

**Quick orientation:** RLark is a cross-cluster embodied intelligence platform built with **Go** (control plane + agents) and **TypeScript/React** (web UI). It uses **kcp** as a lightweight Kubernetes API server and operates across multi-runtime data planes (Kubernetes / Docker / Raw). The control plane manages CRDs (Domain, Node, Job, Task, Workflow) and the data plane agents handle workload orchestration and cross-cluster Pod networking via TUN + gVisor + SSH tunnels. All user-facing changes need tests and docs.

---

## Code structure

- **`api/`** -- CRD type definitions (`rlinf.io/v1alpha1`) + generated Kubernetes client code (`kubeclients/`) + code generation scripts (`hack/`).
- **`apps/rlark/`** -- Main Go project (control plane + data plane agents):
  - `cmd/` -- Entry points: `server/`, `gateway/`, `controller-manager/`, `agent/`, `network-sidecar/`, `rlarkadm/`, `rlarkctl/`, `crd-api-docgen/`.
  - `pkg/` -- Core logic: `server/`, `gateway/`, `agent/`, `controllermanager/`, `network/`, `auth/`, `db/`, `log/`, `metrics/`, `rlarkadm/`, `rlarkctl/`, `addons/`, `apis/`, `configs/`, `utils/`.
- **`apps/embodied-runtime/`** -- Embodied runtime: manages robot (ROS) and camera hardware on edge nodes via Kubernetes Device Plugin.
  - `cmd/` -- `device-plugin/`, `ros-controller/`, `camera-controller/`, `rosctr/`, `camctr/`.
  - `pkg/` -- `deviceplugin/`, `roscontroller/`, `cameracontroller/`, `cli/`.
- **`apps/rlark-ui/`** -- Frontend management UI: React + TypeScript + Vite. Nginx serves static files and proxies `/api/` to Gateway.
- **`sdks/embodied-runtime-go/`** -- Go SDK for embodied-runtime gRPC stubs.
- **`sdks/embodied-runtime-python/`** -- Python SDK for embodied-runtime (RobotClient / CameraClient).
- **`proto/embodied-runtime/`** -- Proto definitions for embodied-runtime gRPC services.
- **`apps/rlark/docs/`** -- RLark core documentation (EN + CN): architecture, concepts, quickstart, deployment, API reference, examples.

---

## How RLark works

### Control Plane

The control plane runs on **kcp** (a lightweight Kubernetes API server) and consists of:

1. **Server** -- Central hub: HTTPS API for Gateway, SSH server for Agent tunnels, reverse proxy to agents, X.509 + SSH certificate management.
2. **Gateway** -- REST API gateway: exposes CRD operations, auth, storage, cluster, and job log endpoints. Proxies requests to agents via Server.
3. **Controller Manager** -- Reconciles CRDs: Job controller splits Jobs into Tasks and drives state machines; Domain/Node/Workflow controllers manage resource lifecycle.
4. **Web UI (Nginx)** -- Serves the React frontend, proxies `/api/` requests to Gateway.

### Data Plane

Each data plane cluster runs an **Agent** with two modes:

- **Cluster Agent** (Deployment) -- Pull controllers: syncs control plane CRs to local K8s resources (Jobs → Pods, ConfigMaps, PVCs). Push controllers: reports local K8s state back to control plane.
- **Node Agent** (DaemonSet) -- Node-level network operations: SSH tunnel setup for cross-cluster Pod networking.

### Cross-Cluster Networking

Pods communicate across clusters via a virtual network:
1. Sidecar (TUN device + gVisor netstack) intercepts Pod traffic.
2. NodeServer makes routing decisions based on DomainPeer CRs.
3. SSHDialer establishes SSH tunnels between clusters.
4. Traffic flows through the tunnel without NAT traversal.

---

## Build and run

```bash
# Build all rlark binaries
make build

# Build specific components
make build-server
make build-gateway
make build-controller-manager
make build-agent
make build-network-sidecar
make build-rlarkadm

# Build embodied-runtime
make -C apps/embodied-runtime build

# Generate proto code
make proto

# Build Docker images
make docker-build
make docker-build-ui

# Lint
make lint
make -C apps/rlark-ui lint
```

### Running locally

```bash
# Start control plane
./bin/server --kubeconfig ~/.kube/config --port 8443
./bin/gateway --port 8080 --server-address localhost:8443
./bin/controller-manager --kubeconfig ~/.kube/config

# Start data plane agent
./bin/agent --kubeconfig ~/.kube/config --server-url https://localhost:8443
```

### Deployment

```bash
# Deploy control plane to K8s cluster
./bin/rlarkadm install -f apps/rlark/docs/examples/deploy-control-plane.yaml

# Deploy data plane agent
./bin/rlarkadm install -f apps/rlark/docs/examples/deploy-data-plane.yaml
```

---

## Key concepts

- **Domain** -- A security domain representing a physical or logical cluster boundary. Each domain has its own X.509 certificate.
- **Node** -- A compute node (GPU server, edge device, robot) within a Domain. Supports `unschedulable` for cordon/uncordon.
- **Job** -- A training job composed of multiple Tasks with DAG dependencies.
- **Task** -- A single workload unit (e.g., a Ray head or worker). Supports PVC mounting via `pvcStorageMap`.
- **Workflow** -- A DAG of Job templates for complex training pipelines.
- **DomainPeer** -- Defines cross-cluster network connectivity between two Domains.

---

## Style and contributing

- **Go**: Follow [Effective Go](https://go.dev/doc/effective_go) and standard Go conventions. Run `make lint` before committing.
- **TypeScript/React**: Follow standard React conventions. Run `make -C apps/rlark-ui lint` before committing.
- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/) format. Every commit must include `Signed-off-by:` (use `git commit -s`).
- **PRs**: Same title format as commits. Fill in the PR template. Link related issues.
- **Tests**: New features should include tests where applicable. Go tests use standard `go test`.
- **Docs**: All user-facing changes must include documentation updates. Both EN and CN versions should be kept in sync.

Full details: [CONTRIBUTING.md](CONTRIBUTING.md).
