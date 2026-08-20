---
hide:
  - toc
---

<div align="center">
  <img src="../images/logo-zh.png" alt="RLark Logo" width="400" />
</div>

<div align="center" markdown>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&style=flat-square" alt="Go Version" />
  <img src="https://img.shields.io/badge/TypeScript-React-3178C6?logo=typescript&style=flat-square" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Kubernetes-kcp-326CE5?logo=kubernetes&style=flat-square" alt="Kubernetes" />
</div>

<div align="center" markdown>
  <a href="https://rlark.readthedocs.io/en/latest/"><img src="https://img.shields.io/badge/English-EN-4051b5?style=flat-square" alt="English" /></a>
  <img src="https://img.shields.io/badge/中文-中文-e91e63?style=flat-square" alt="中文" />
</div>

<h1 align="center">
  <sub>RLark 具身智能云原生纳管平台</sub>
</h1>

以 Kubernetes 原生方式管理跨集群具身智能工作负载，从云端 GPU 训练到端侧设备部署。通过统一的任务调度、跨集群 Pod 网络互通和多运行时支持，实现 GPU 集群、机械臂、传感器等异构设备间的无缝协同。

## 最新动态

- [2026/08] RLark 现已开源。

## 核心能力

- **具身智能工作负载编排**：从云端 GPU 训练（RL/LLM）到端侧部署，统一的声明式 Job/Workflow/Task 抽象覆盖全链路
- **多运行时数据面**：基于 Kubernetes 统一纳管云端 GPU 集群与端侧设备，覆盖训练到具身设备部署的完整链路；面向不适合部署 Kubernetes 的轻量端侧场景，后续将扩展 Docker 和 Raw 运行时支持
- **跨集群资源抽象**：通过 Domain 和 Node CRD 统一管理多地 GPU 集群和端侧设备，控制面运行在 kcp 之上
- **声明式训练任务**：多层抽象，支持 DAG 编排的训练流水线，声明式定义 Ray 集群
- **跨集群 Pod 网络**：基于 TUN 设备 + gVisor 协议栈 + SSH 隧道的虚拟网络，Pod 跨集群通信无需 NAT 穿透
- **证书体系**：X.509 + SSH 双层证书，支持 Agent 接入、Domain 范围的跨集群转发鉴权、用户 SSH 登录鉴权
- **可观测性**：Prometheus 指标暴露、Pod 日志实时查询、Web 管理界面

## 架构概览

<div style="max-width: 85%; margin: 0 auto;">
  <img src="../images/architecture.png" alt="系统架构" style="width: 100%;">
</div>

## 快速开始

```bash
# 克隆仓库并进入项目目录
git clone https://github.com/RLinf/RLark.git
cd RLark

# 一键部署：控制面（Docker Compose）+ 2 个 kind 集群 + 跨集群网络验证
bash apps/rlark/docs/examples/quickstart.sh
```

!!! tip "系统要求"
    - 操作系统：Linux（推荐）或 macOS
    - CPU 架构：amd64 / arm64
    - 内存：建议 ≥ 16 GB
    - 磁盘：建议 ≥ 20 GB 可用空间
    - 依赖：Docker ≥ 24.0、kind ≥ 0.20、kubectl ≥ 1.28、jq、python3
    - 中国大陆用户：如 Docker Hub 拉取镜像失败，脚本会自动尝试国内镜像源

详见 [快速开始指南](quickstart.md) 了解完整前置依赖和分步说明。

## 技术栈

- **语言**：Go (控制面/Agent) + TypeScript (前端)
- **编排**：Kubernetes (kcp + kind)
- **网络**：TUN 设备 + gVisor netstack + SSH 隧道
- **证书**：X.509 mTLS + SSH 证书
- **数据库**：PostgreSQL (Bun ORM)
- **监控**：Prometheus
- **前端**：React + Vite + TypeScript
