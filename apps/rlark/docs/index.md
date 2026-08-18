---
hide:
  - navigation
  - toc
---

<div align="center">
  <img src="images/logo-en.png" alt="RLark Logo" width="400" />
</div>

<div align="center" markdown>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&style=flat-square" alt="Go Version" />
  <img src="https://img.shields.io/badge/TypeScript-React-3178C6?logo=typescript&style=flat-square" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Kubernetes-kcp-326CE5?logo=kubernetes&style=flat-square" alt="Kubernetes" />
</div>

<h1 align="center">
  <sub>RLark — Cross-Cluster Embodied Intelligence Cloud-Native Platform</sub>
</h1>

Manage cross-cluster embodied intelligence workloads natively with Kubernetes, from cloud GPU training to edge device deployment. Through unified job scheduling, cross-cluster Pod-to-Pod networking, and multi-runtime support, RLark enables seamless collaboration between GPU clusters, robot arms, sensors, and other heterogeneous devices.

## What's NEW!

- [2026/08] RLark is now open-source.

## Key Capabilities

- **Embodied AI Workload Orchestration**: From cloud GPU training (RL/LLM) to edge deployment, unified declarative Job/Workflow/Task abstraction across the full pipeline
- **Multi-Runtime Data Plane**: Native support for Kubernetes runtime, with Docker and Raw runtimes in experimental status
- **Cross-Cluster Resource Abstraction**: Unify multi-site GPU clusters and edge devices via Domain and Node CRDs, with the control plane running on kcp
- **Declarative Training Jobs**: Multi-layer abstraction with DAG-based training pipelines and declarative Ray cluster definition
- **Cross-Cluster Pod Networking**: Virtual network based on TUN devices + gVisor netstack + SSH tunnels, enabling Pod-to-Pod communication without NAT traversal
- **Certificate System**: Dual-layer X.509 + SSH certificates for Agent access, Domain isolation, and user SSH authentication
- **Observability**: Prometheus metrics, real-time Pod log streaming, and web management UI

## Architecture Overview

![System Architecture](images/architecture.png)

## Quick Start

```bash
# One-click deploy: control plane (Docker Compose) + 2 kind clusters + cross-cluster network test
bash apps/rlark/docs/examples/quickstart.sh
```

See the [Quick Start Guide](quickstart.md) for prerequisite requirements and detailed instructions.

## Try the Web Console

See [https://rlark-docs.pages.dev/](https://rlark-docs.pages.dev/) for the full Web Console guide.

## Tech Stack

- **Language**: Go (control plane/agent) + TypeScript (frontend)
- **Orchestration**: Kubernetes (kcp + kind)
- **Networking**: TUN device + gVisor netstack + SSH tunnel
- **Certificates**: X.509 mTLS + SSH certificates
- **Database**: PostgreSQL (Bun ORM)
- **Monitoring**: Prometheus
- **Frontend**: React + Vite + TypeScript