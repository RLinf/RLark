# 具身设备接入

## GPU 集群 vs. 具身设备

RLark 可以管理两类计算资源。标准 GPU 集群接入请参见 [GPU 集群接入](data-plane.md)。

| | GPU 集群接入 | 具身设备接入 |
|---|---|---|
| **目标** | 云端 / 自建 GPU 集群 | 带有机器人、摄像头等物理设备的边缘集群 |
| **Agent** | `rlark-agent` 连接集群到控制面 | 同一个 Agent，额外需要 **Device Plugin** 注册设备 |
| **资源类型** | `nvidia.com/gpu`、CPU、内存 | `rlinf.io/device-*`（机器人、摄像头等） |
| **工作负载** | 标准 Ray 强化学习训练 | 需要与真实硬件交互的 RL 训练（机器人、摄像头） |
| **网络** | 集群内或通过 Domain 跨集群 | 可能需要宿主设备透传或 macvlan 连接固定 IP 机器人 |

两者使用相同的[集群证书流程](data-plane.md)。关键区别在于**接入后集群上运行什么**：GPU 集群只需要 Agent，而具身设备集群还需要 **Embodied Runtime** 来发现和管理物理设备。

## 什么是 Embodied Runtime？

Embodied Runtime（`apps/embodied-runtime`）使机器人和摄像头能够作为可调度设备参与 RLark 训练任务。它由 Kubernetes Device Plugin、基于 gRPC 的 ROS 1/ROS 2/摄像头控制器以及 CLI 工具组成。深入了解内部细节请参见 [Embodied Runtime 参考](../developer-guide/embodied-runtime-reference.md)。

## 架构

Embodied Runtime 有三层结构：

| 层次 | 组件 | 说明 |
|------|------|------|
| Device Plugin | `device-plugin` | 向 Kubernetes 注册设备资源（`rlinf.io/device-*`） |
| 控制器 | `ros-controller`, `ros2-controller`, `camera-controller` | 管理设备生命周期的 gRPC 服务 |
| Webhook | Mutating Webhook | 自动注入 `devinit` sidecar，用于 macvlan 网络 |

### 工作原理

1. **Device Plugin** 向 kubelet 注册并申报 `rlinf.io/device[-<model>]` 资源。
2. 收到 `Allocate` 请求时，把 socket 目录（`/var/run/rlark`）与 CLI 目录（`/opt/rlinf/bin`）注入业务 Pod，并通过 `RLINF_EMBODIED_*` 环境变量告知可用运行时。
3. 业务 Pod 用挂载的 CLI（或直接走 gRPC）驱动硬件。

## 前置条件

- 具有兼容边缘节点的 Kubernetes 集群
- Go 1.26+（从源码构建时需要）
- Docker（用于容器镜像）
- 目标节点上可访问的宿主设备
- 机器人控制器需要 `hostPID: true` 和 `privileged: true`（ROS 1 需要 PID 命名空间访问）

## 部署

### Helm（推荐）

```bash
helm install embodied-runtime ./charts/embodied-runtime \
  --namespace rlark-system \
  --set config.ros.enabled=true \
  --set config.camera.enabled=true
```

### Device Plugin 配置

配置设备插件，声明节点上可用的设备：

```yaml
# device-plugin-config.yaml
host_devices:
  - name: rlinf.io/device-webcam
    count: 2
    devices:
      - /dev/video0
      - /dev/video1

host_macvlans:
  - name: rlinf.io/device-franka
    count: 1
    parent_interface: eth0
    robot_ip: 192.168.1.100

camera:
  enabled: true
  socket: /var/run/rlark/camera-ctrl.sock

ros:
  enabled: true
  socket: /var/run/rlark/ros-ctrl.sock

ros2:
  enabled: false
  socket: /var/run/rlark/ros2-ctrl.sock
```

### 宿主设备透传

对于 USB 摄像头等简单设备，使用 `host_devices` 直接透传设备文件：

```yaml
host_devices:
  - name: rlinf.io/device-webcam
    count: 1
    devices:
      - /dev/video0
```

### Macvlan 网络机器人

对于具有固定 IP 的网络机器人，使用 `host_macvlans`：

```yaml
host_macvlans:
  - name: rlinf.io/device-franka
    count: 1
    parent_interface: eth0
    robot_ip: 192.168.1.100
```

Mutating Webhook 会自动注入 `devinit` sidecar，在 Worker 容器中创建 macvlan 接口。

### 控制器 Pod

完整的控制器 Pod 规格、Manager 模式和组网细节请参见 [Embodied Runtime 参考](../developer-guide/embodied-runtime-reference.md)。

## 验证

部署后验证 Embodied Runtime 是否正常工作：

```bash
# 1. 检查 Device Plugin 运行状态
kubectl get pods -n rlark-system -l app=device-plugin

# 2. 验证设备资源已注册
kubectl describe node <node-name> | grep rlinf.io/device

# 3. 测试机器人发现（在 Worker 内）
rosctr list

# 4. 测试摄像头发现（在 Worker 内）
camctr list
```

## 安全

部署真实机器人时：

- 仅在兼容的边缘集群部署控制器
- 先用非生产设备验证发现与分配流程
- 确认宿主机运行时依赖和设备访问权限
- 操作真实硬件前遵循 [安全清单](../developer-guide/device-integration.md) 中的真实设备安全要求

## 参考

| 资源 | 路径 |
|------|------|
| Embodied Runtime 参考 | [完整技术参考](../developer-guide/embodied-runtime-reference.md) |
| Embodied Runtime README | `apps/embodied-runtime/README.zh-CN.md` |
| 部署示例 | `apps/embodied-runtime/docs/examples.zh-CN.md` |
| gRPC API 参考 | `apps/embodied-runtime/docs/proto-api.zh-CN.md` |
| Helm Chart | `apps/embodied-runtime/charts/embodied-runtime/` |
| 设备集成指南 | [新设备适配](../developer-guide/device-integration.md) |