<div align="center">
  <img src="apps/rlark/docs/images/logo-en.png" alt="RLark Logo" width="400" />
</div>

<div align="center">
  <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue.svg" /></a>
  <a href="README.zh-CN.md"><img src="https://img.shields.io/badge/语言-简体中文-red.svg" /></a>
  <a href="https://rlark-ci-test.readthedocs.io/en/latest/"><img src="https://img.shields.io/badge/Documentation-Read%20the%20Docs-8A2BE2?logo=readthedocs&logoColor=white" alt="Documentation" /></a>
  <a href="https://rlark-ci-test.readthedocs.io/zh-cn/latest/"><img src="https://img.shields.io/badge/中文文档-Read%20the%20Docs-red?logo=readthedocs&logoColor=white" alt="中文文档" /></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&style=flat-square" alt="Go Version" />
  <img src="https://img.shields.io/badge/TypeScript-React-3178C6?logo=typescript&style=flat-square" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Kubernetes-kcp-326CE5?logo=kubernetes&style=flat-square" alt="Kubernetes" />
</div>

<h1 align="center">
  <sub>RLark — Cross-Cluster Embodied Intelligence Cloud-Native Platform</sub>
</h1>

Manage cross-cluster embodied intelligence workloads through a unified cloud-native platform spanning cloud GPU training, cross-cluster collaboration, and edge device deployment across heterogeneous resources such as GPU clusters, robot arms, sensors, and cameras.

> **Explore the complete documentation on [Read the Docs](https://rlark-ci-test.readthedocs.io/en/latest/)** — start with the [Quick Start](https://rlark-ci-test.readthedocs.io/en/latest/quickstart/), then continue with the [platform user guide](https://rlark-ci-test.readthedocs.io/en/latest/user-guide/) or [administrator guide](https://rlark-ci-test.readthedocs.io/en/latest/admin-guide/).

## What's NEW!

- [2026/08] RLark is now open-source.

## Key Capabilities

- **Embodied AI Workload Orchestration**: From cloud GPU training (RL/LLM) to edge deployment (robot arm, sensor, camera), unified declarative Job/Workflow/Task abstraction across the full pipeline
- **Multi-Runtime Data Plane**: Kubernetes provides unified management for cloud GPU clusters and edge devices across the complete training-to-deployment lifecycle; Docker and Raw runtime support will extend coverage to lightweight edge scenarios where Kubernetes is not suitable
- **Cross-Cluster Resource Abstraction**: Unify multi-site GPU clusters and edge devices via Domain (virtual network domain) and Node (compute node) CRDs, with the control plane running on kcp
- **Declarative Training Jobs**: Multi-layer abstraction (Job/Workflow/Task) with DAG-based training pipelines and declarative Ray cluster definition
- **Cross-Cluster Pod Networking**: Virtual network based on TUN devices + gVisor netstack + SSH tunnels, enabling Pod-to-Pod communication without NAT traversal — cloud GPUs and edge robots communicate directly
- **Certificate System**: Dual-layer X.509 + SSH certificates for Agent access, Domain-scoped cross-cluster forwarding authentication, and user SSH authentication
- **Observability**: Prometheus metrics, real-time Pod log streaming, and web management UI

## Architecture Overview

![System Architecture](apps/rlark/docs/images/architecture.png)

## Quick Start

Follow the [Quick Start Guide on Read the Docs](https://rlark-ci-test.readthedocs.io/en/latest/quickstart/) to choose one of the verified flows:

- **One-click CLI**: deploy the control plane and two kind data-plane clusters, then verify cross-cluster Pod networking.
- **UI-based flow**: create clusters and a Domain in the web console, deploy two kind data planes, schedule a Job across them, and verify connectivity.

## Documentation

The complete, searchable, and versioned documentation is published on **[Read the Docs](https://rlark-ci-test.readthedocs.io/en/latest/)**. Use these rendered guides as the primary entry points:

| Guide | Description |
|-------|-------------|
| [Quick Start](https://rlark-ci-test.readthedocs.io/en/latest/quickstart/) | Verified one-click and UI-based local deployment flows |
| [Core Concepts](https://rlark-ci-test.readthedocs.io/en/latest/concepts/) | Domain, Job, Task, Workflow, and other concepts |
| [Platform User Guide](https://rlark-ci-test.readthedocs.io/en/latest/user-guide/) | Web console, clusters, jobs, workflows, storage, and SSH keys |
| [Administrator Guide](https://rlark-ci-test.readthedocs.io/en/latest/admin-guide/) | Control plane, data plane, networking, security, and operations |
| [Developer Guide](https://rlark-ci-test.readthedocs.io/en/latest/developer-guide/) | Local development, project layout, debugging, and extensions |
| [API Reference](https://rlark-ci-test.readthedocs.io/en/latest/api/reference/) | Gateway REST API routes and behavior |
| [Architecture](https://rlark-ci-test.readthedocs.io/en/latest/architecture/) | Components, interactions, and data flows |

Repository-specific references remain available alongside the code:

| Reference | Description |
|-----------|-------------|
| [Embodied Runtime](apps/embodied-runtime/README.md) | Robot (ROS) and camera hardware management on edge nodes |
| [Web UI](apps/rlark-ui/README.md) | Frontend management console |
| [Python SDK](sdks/embodied-runtime-python/README.md) | Python client for robot/camera gRPC services |
| [Go SDK](sdks/embodied-runtime-go/README.md) | Go client for embodied-runtime gRPC stubs |
| [Proto Definitions](proto/embodied-runtime/README.md) | gRPC service definitions for embodied-runtime |

> Prefer Chinese? Visit the **[中文 Read the Docs 站点](https://rlark-ci-test.readthedocs.io/zh-cn/latest/)**.

## Tech Stack

- **Language**: Go (control plane/agent) + TypeScript (frontend)
- **Orchestration**: Kubernetes (kcp + kind)
- **Networking**: TUN device + gVisor netstack + SSH tunnel
- **Certificates**: X.509 mTLS + SSH certificates
- **Database**: PostgreSQL (Bun ORM)
- **Monitoring**: Prometheus
- **Frontend**: React + Vite + TypeScript

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for our community standards.

## License

RLark is licensed under the [Apache License 2.0](LICENSE).
