# RLark Core

RLark core is the control plane and data plane platform for cross-cluster embodied intelligence workloads.

## Directory Structure

```text
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
│   ├── controllermanager/   # Controllers: job, domain, task, node
│   ├── network/             # Network: sidecar, nodeserver, SSH dialer
│   ├── addons/              # Addon catalog and management
│   ├── auth/                # Authentication
│   ├── db/                  # Database (PostgreSQL)
│   ├── log/                 # Structured logging
│   ├── metrics/             # Prometheus metrics
│   ├── apis/                # API types
│   ├── configs/             # Configuration
│   └── utils/               # Shared utilities
└── docs/         # RLark core documentation and Read the Docs source
    ├── overview/             # Product capabilities
    ├── user-guide/           # Platform user workflows
    ├── admin-guide/          # Deployment, onboarding, security, operations
    ├── developer-guide/      # Development, debugging, and extensions
    ├── reference/            # CLI, CRD, configuration, and changelog
    ├── api/                  # API reference and examples
    ├── zh/                   # Chinese documentation mirroring the English structure
    ├── architecture.md       # Technical architecture
    ├── concepts.md           # Core concepts (Domain, Job, Task)
    ├── quickstart.md         # One-click and UI-based evaluation flows
    ├── deployment.md         # Complete deployment reference
    └── storage-api.md        # Storage API documentation
```

## Documentation

The complete English and Chinese documentation is published on [Read the Docs](https://rlark.readthedocs.io/en/latest/). Its audience-oriented source structure is described in [docs/README.md](docs/README.md).

- [Quick Start](docs/quickstart.md) — one-click CLI and UI-based evaluation flows
- [Platform User Guide](docs/user-guide/index.md) — console, compute resources, jobs, storage, and SSH keys
- [Administrator Guide](docs/admin-guide/index.md) — control plane, data plane, devices, networking, and operations
- [Developer Guide](docs/developer-guide/index.md) — local development, project layout, debugging, and extensions
- [Reference](docs/reference/index.md) — CLI, API, CRD, configuration, and changelog
- [Architecture](docs/architecture.md) — complete technical architecture
- [Core Concepts](docs/concepts.md) — Domain, Job, Task, and related concepts

## Build

```bash
make build
```
