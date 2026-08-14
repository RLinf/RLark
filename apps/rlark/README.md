# RLark Core

RLark core is the control plane and data plane platform for cross-cluster embodied intelligence workloads.

## Directory Structure

```
apps/rlark/
├── cmd/          # Entry points
│   ├── server/              # Control plane Server
│   ├── gateway/             # API Gateway
│   ├── controller-manager/  # Control plane controllers
│   ├── agent/               # Data plane Agent
│   ├── network-sidecar/     # Pod network Sidecar
│   ├── rlarkadm/            # Deployment CLI
│   ├── rlarkctl/            # Management CLI
│   └── crd-api-docgen/      # API doc generator
├── pkg/          # Core logic
│   ├── server/              # Server: tunnel, cert, SSH, peer, k8s proxy
│   ├── gateway/             # Gateway: CRD CRUD, cert, auth, storage
│   ├── agent/               # Agent: pull/push controllers, container adapters
│   ├── controllermanager/   # Controllers: job, domain, task, node, workflow
│   ├── network/             # Network: sidecar, nodeserver, SSH dialer
│   ├── addons/              # Addon catalog and management
│   ├── auth/                # Authentication
│   ├── db/                  # Database (PostgreSQL)
│   ├── log/                 # Structured logging
│   ├── metrics/             # Prometheus metrics
│   ├── apis/                # API types
│   ├── configs/             # Configuration
│   └── utils/               # Shared utilities
└── docs/         # RLark core documentation
    ├── architecture.md      # Technical architecture
    ├── concepts.md           # Core concepts (Domain, Job, Task, Workflow)
    ├── quickstart.md         # Local development setup
    ├── deployment.md         # Production deployment guide
    ├── storage-api.md        # Storage API documentation
    ├── ui-behavior.md        # Web console behavior
    └── api/                  # API reference and examples
```

## Documentation

- [Architecture](docs/architecture.md) — complete technical architecture
- [Core Concepts](docs/concepts.md) — Domain, Job, Task, Workflow, etc.
- [Quick Start](docs/quickstart.md) — local development setup
- [Deployment Guide](docs/deployment.md) — production deployment
- [API Reference](docs/api/reference.md) — REST API reference
- [API Examples](docs/api/examples.md) — end-to-end API usage

## Build

```bash
make build
```
