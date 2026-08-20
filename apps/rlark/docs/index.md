---
hide:
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

<div align="center" markdown>
  <img src="https://img.shields.io/badge/English-EN-4051b5?style=flat-square" alt="English" />
  <a href="https://rlark.readthedocs.io/zh-cn/latest/"><img src="https://img.shields.io/badge/中文-中文-e91e63?style=flat-square" alt="中文" /></a>
</div>

<h1 align="center">
  <sub>RLark — Cross-Cluster Embodied Intelligence Cloud-Native Platform</sub>
</h1>

Manage cross-cluster embodied intelligence workloads natively with Kubernetes, from cloud GPU training to edge device deployment. Through unified job scheduling, cross-cluster Pod-to-Pod networking, and multi-runtime support, RLark enables seamless collaboration between GPU clusters, robot arms, sensors, and other heterogeneous devices.

## What's NEW!

- [2026/08] RLark is now open-source.

## Key Capabilities

- **Embodied AI Workload Orchestration**: From cloud GPU training (RL/LLM) to edge deployment, unified declarative Job/Workflow/Task abstraction across the full pipeline
- **Multi-Runtime Data Plane**: Kubernetes provides unified management for cloud GPU clusters and edge devices across the complete training-to-deployment lifecycle; Docker and Raw runtime support will extend coverage to lightweight edge scenarios where Kubernetes is not suitable
- **Cross-Cluster Resource Abstraction**: Unify multi-site GPU clusters and edge devices via Domain and Node CRDs, with the control plane running on kcp
- **Declarative Training Jobs**: Multi-layer abstraction with DAG-based training pipelines and declarative Ray cluster definition
- **Cross-Cluster Pod Networking**: Virtual network based on TUN devices + gVisor netstack + SSH tunnels, enabling Pod-to-Pod communication without NAT traversal
- **Certificate System**: Dual-layer X.509 + SSH certificates for Agent access, Domain-scoped cross-cluster forwarding authentication, and user SSH authentication
- **Observability**: Prometheus metrics, real-time Pod log streaming, and web management UI

## Architecture Overview

<div style="max-width: 85%; margin: 0 auto;">
  <img src="images/architecture.png" alt="System Architecture" style="width: 100%;">
</div>

## Quick Start

```bash
# Clone the repository and enter the project directory
git clone https://github.com/RLinf/RLark.git
cd RLark

# One-click deploy: control plane (Docker Compose) + 2 kind clusters + cross-cluster network test
bash apps/rlark/docs/examples/quickstart.sh
```

!!! tip "System Requirements"
    - OS: Linux (recommended) or macOS
    - CPU Architecture: amd64 / arm64
    - Memory: ≥ 16 GB recommended
    - Disk: ≥ 20 GB free space recommended
    - Dependencies: Docker ≥ 24.0, kind ≥ 0.20, kubectl ≥ 1.28, jq, python3
    - Users in China: the script automatically falls back to domestic mirror if Docker Hub is unreachable

See the [Quick Start Guide](quickstart.md) for full prerequisites and step-by-step instructions.

## Tech Stack

- **Language**: Go (control plane/agent) + TypeScript (frontend)
- **Orchestration**: Kubernetes (kcp + kind)
- **Networking**: TUN device + gVisor netstack + SSH tunnel
- **Certificates**: X.509 mTLS + SSH certificates
- **Database**: PostgreSQL (Bun ORM)
- **Monitoring**: Prometheus
- **Frontend**: React + Vite + TypeScript
