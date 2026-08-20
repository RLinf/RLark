<div align="center">
  <img src="apps/rlark/docs/images/logo-zh.png" alt="RLark Logo" width="400" />
</div>

<div align="center">
  <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue.svg" /></a>
  <a href="README.zh-CN.md"><img src="https://img.shields.io/badge/语言-简体中文-red.svg" /></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&style=flat-square" alt="Go Version" />
  <img src="https://img.shields.io/badge/TypeScript-React-3178C6?logo=typescript&style=flat-square" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Kubernetes-kcp-326CE5?logo=kubernetes&style=flat-square" alt="Kubernetes" />
</div>

<h1 align="center">
  <sub>RLark 具身智能云原生纳管平台</sub>
</h1>

以云原生方式统一纳管跨集群具身智能工作负载，覆盖云端 GPU 训练、跨集群协同与端侧设备部署，连接 GPU 集群、机械臂、传感器和摄像头等异构资源。

## 最新动态

- [2026/08] RLark 现已开源。

## 核心能力

- **具身智能工作负载编排**：从云端 GPU 训练（RL/LLM）到端侧部署（机械臂、传感器、摄像头），统一的声明式 Job/Workflow/Task 抽象覆盖全链路
- **多运行时数据面**：基于 Kubernetes 统一纳管云端 GPU 集群与端侧设备，覆盖训练到具身设备部署的完整链路；面向不适合部署 Kubernetes 的轻量端侧场景，后续将扩展 Docker 和 Raw 运行时支持
- **跨集群资源抽象**：通过 Domain（虚拟网络域）和 Node（计算节点）CRD 统一管理多地 GPU 集群和端侧设备，控制面运行在 kcp 之上
- **声明式训练任务**：Job/Workflow/Task 多层抽象，支持 DAG 编排的训练流水线，声明式定义 Ray 集群
- **跨集群 Pod 网络**：基于 TUN 设备 + gVisor 协议栈 + SSH 隧道的虚拟网络，Pod 跨集群通信无需 NAT 穿透 — 云端 GPU 与端侧机器人直接通信
- **证书体系**：X.509 + SSH 双层证书，支持 Agent 接入、Domain 范围的跨集群转发鉴权、用户 SSH 登录鉴权
- **可观测性**：Prometheus 指标暴露、Pod 日志实时查询、Web 管理界面

## 架构概览

![系统架构](apps/rlark/docs/images/architecture.png)

## 快速开始

请参阅 [快速开始指南](apps/rlark/docs/zh/quickstart.md) 了解如何在本地搭建开发环境并运行第一个训练任务。

```bash
# 1. 编译
git clone https://github.com/RLinf/RLark
cd RLark && make build

# 2. 启动控制面（Docker Compose）
docker compose -f apps/rlark/docs/examples/docker-compose.yml up -d

# 3. 启动数据面（kind 集群）
kind create cluster --name rlark-data

# 然后按照快速开始指南启动各组件并创建任务
```

## 文档索引

| 文档 | 说明 |
|------|------|
| [架构设计](apps/rlark/docs/zh/architecture.md) | RLark 核心：完整技术架构、组件交互、数据流 |
| [核心概念](apps/rlark/docs/zh/concepts.md) | Domain、Job、Task、Workflow 等概念解释 |
| [快速开始](apps/rlark/docs/zh/quickstart.md) | 本地开发环境搭建与第一个训练任务 |
| [部署指南](apps/rlark/docs/zh/deployment.md) | 生产环境部署、配置说明 |
| [API 参考](apps/rlark/docs/api/reference.md) | 完整的 REST API 参考 |
| [API 示例](apps/rlark/docs/api/examples.md) | 端到端 API 调用示例 |
| [Embodied Runtime](apps/embodied-runtime/README.md) | 端侧机器人/摄像头运行时管理 |
| [Web UI](apps/rlark-ui/README.md) | 前端管理界面说明 |
| [Python SDK](sdks/embodied-runtime-python/README.md) | 机器人/摄像头 gRPC 客户端 |
| [Go SDK](sdks/embodied-runtime-go/README.md) | Go 语言 gRPC 客户端 |
| [Proto 定义](proto/embodied-runtime/README.md) | gRPC 服务接口定义 |

> [English Documentation](apps/rlark/docs/README.md)

## 技术栈

- **语言**：Go (控制面/Agent) + TypeScript (前端)
- **编排**：Kubernetes (kcp + kind)
- **网络**：TUN 设备 + gVisor netstack + SSH 隧道
- **证书**：X.509 mTLS + SSH 证书
- **数据库**：PostgreSQL (Bun ORM)
- **监控**：Prometheus
- **前端**：React + Vite + TypeScript

## 参与贡献

欢迎贡献！请参阅 [CONTRIBUTING.md](CONTRIBUTING.md) 了解贡献指南，以及 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) 了解社区行为准则。

## 开源协议

RLark 基于 [Apache License 2.0](LICENSE) 开源。
