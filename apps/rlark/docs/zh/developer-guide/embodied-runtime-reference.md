# Embodied Runtime

一个面向边缘节点的 Kubernetes 原生运行时，用于管理机器人（ROS）与摄像头硬件。它通过 Device Plugin API 将机器人和摄像头暴露为**可调度的 Kubernetes 资源**。接入流程请参见 [具身设备集群接入](../admin-guide/embodied-runtime.md)。

## Manager 模式

每个控制器（`camera`、`ros`、`ros2`）可独立配置三种模式：

| 模式 | 行为 |
|------|------|
| `disabled` | 不启动控制器、不生成配置（默认）。 |
| `local` | 控制器作为 device plugin 的子进程运行。 |
| `pod` | 控制器在独立 Pod 中运行，由 device plugin 创建并托管。 |

在 `pod` 模式下，device plugin 会自动发现自身 Pod 的属性（owner refs、tolerations、镜像、节点名）并复用填充控制器 Pod 的空字段，使控制器 Pod 随 device plugin Pod 一起被垃圾回收，并被调度到同一个带污点的节点上。

## ROS 1 组网

每个机器人拥有独立的 `roscore`（端口从 `11311` 起递增），多机器人可在同一容器内共存而不冲突 topic / 节点名。`ROS_MASTER_URI` 与 `ROS_IP` 会被注入每个 `roslaunch` 进程。启动时按配置创建 MACVLAN 接口以接入机器人物理子网，`roslaunch` 直接在容器内运行（不使用 `nsenter`）。

## ROS 2 组网

与 ROS 1 不同，ROS 2 没有中央 master。每个机器人拥有独立的 `ROS_DOMAIN_ID`（自动从 0 递增分配，或通过 robot 配置中的 `domain_id` 显式指定），实现 DDS 级隔离，使同一容器内的多机器人互不发现对方的 topic / service。`ROS_DOMAIN_ID`（及可选的 `RMW_IMPLEMENTATION`、`CYCLONEDDS_URI`）会注入每个 `ros2 launch` 进程。基础镜像默认使用 Cyclone DDS（`rmw_cyclonedds_cpp`），更适合多机器人与跨子网发现场景。

!!! warning "组播要求"
    ROS 2 DDS 默认使用 IP 组播进行节点发现。集群的网络层（CNI 插件、节点 / 底层交换机）**必须支持组播路由**，跨 Pod / 跨节点的 ROS 2 节点才能互相发现。如果集群不支持组播（许多云厂商 CNI 默认会屏蔽），需改为单播发现 —— 在 robot 的 mode 级 `env` 中设置 `CYCLONEDDS_URI`（或 Fast DDS 等价配置）为 peer-list XML profile，或部署 DDS Discovery Server。

## 完整配置参考

### device-plugin

通过 `--config <path>` 加载；所有字段均可省略。

```yaml
# model: franka          # → 资源 rlinf.io/device-franka，
                          #   socket rlinf-device-franka.sock
device_count: 1
skip_register: false

host_devices:
  - host_path: /dev/video0
    # container_path: /dev/video0  # 默认与 host_path 相同
    # permissions: rwm             # r|w|m 任意组合；默认 rwm
  - host_path: /dev/ttyUSB0
    permissions: rw

host_macvlans:
  - host_nic: eno1
    name: macvlan0
    ip: 172.16.0.0/24      # 网络地址 → 自动挑选未用主机 IP
    # gateway: 172.16.0.1  # 可选，机器人子网的默认网关

camera:
  manager_mode: local     # disabled | local | pod
  ctrl_config_path: /etc/rlinf/camera-controller.yaml
  ctrl_bin: /usr/local/bin/camera-controller
  ctr_cli: /opt/rlinf/bin/camctr
  auto_detect_v4l2: true  # 自动探测 /dev/video* 摄像头
  cameras: []

ros:
  manager_mode: local     # disabled | local | pod
  ctrl_config_path: /etc/rlinf/ros-controller.yaml
  ctrl_bin: /usr/local/bin/ros-controller
  ctr_cli: /opt/rlinf/bin/rosctr
  macvlans:
    - host_nic: eno1
      name: macvlan0
      ip: 172.16.0.101/24
  types:
    - type: franka
      modes:
        impedance:
          package: serl_franka_controllers
          launch_file: impedance.launch
          passthrough_robot_args: true
        joint:
          package: serl_franka_controllers
          launch_file: joint.launch
          passthrough_robot_args: true
  robots:
    - id: franka-robot-1
      type: franka
      params:
        robot_ip: 172.16.0.2
      web_service: https://172.16.0.2/
  allowed_launch_packages:
    - serl_franka_controllers

ros2:
  manager_mode: local     # disabled | local | pod
  ctrl_config_path: /etc/rlinf/ros2-controller.yaml
  ctrl_bin: /usr/local/bin/ros2-controller
  ctr_cli: /opt/rlinf/bin/rosctr    # 与 ROS 1 共用
  macvlans:
    - host_nic: eno1
      name: macvlan0
      ip: 172.16.0.101/24
  types:
    - type: franka
      modes:
        impedance:
          package: moveit_servo
          launch_file: servo.launch.py
          passthrough_robot_args: true
  robots:
    - id: franka-robot-1
      type: franka
      params:
        robot_ip: 172.16.0.2
      domain_id: 5            # 显式 ROS_DOMAIN_ID 覆盖（为 0 时自动分配）
  allowed_launch_packages:
    - moveit_servo
```

## 控制器 Pod 规格

### ROS 1 控制器

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: ros-controller
  namespace: rlark-system
spec:
  hostPID: true
  containers:
  - name: ros-controller
    image: rlark-embodied-runtime:latest
    command: ["/ros-controller"]
    args: ["--socket=/var/run/rlark/ros-ctrl.sock"]
    securityContext:
      privileged: true
    volumeMounts:
    - name: rlark-socket
      mountPath: /var/run/rlark
  volumes:
  - name: rlark-socket
    hostPath:
      path: /var/run/rlark
```

典型模式：`initContainer` 把 `ros-controller` / `rosctr` 从 `embodied-runtime` 镜像拷贝到共享的 `emptyDir`；主容器从该目录运行控制器二进制。存活 / 就绪探针通过 `rosctr list` 访问 Unix socket 来实现。

### ROS 2 控制器

与 ROS 1 类似，使用不同的 socket：

```yaml
args: ["--socket=/var/run/rlark/ros2-ctrl.sock"]
```

### 摄像头控制器

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: camera-controller
  namespace: rlark-system
spec:
  containers:
  - name: camera-controller
    image: rlark-embodied-runtime:latest
    command: ["/camera-controller"]
    args: ["--socket=/var/run/rlark/camera-ctrl.sock"]
    volumeMounts:
    - name: rlark-socket
      mountPath: /var/run/rlark
  volumes:
  - name: rlark-socket
    hostPath:
      path: /var/run/rlark
```

### 摄像头控制器详情

摄像头可静态配置，或从 `/sys/class/video4linux` 自动探测。采集通过拉起 `ffmpeg` 子进程完成；驱动按设备原生格式采集，并按需转码为目标输出 —— 帧模式静图编码（`jpeg` / `png` / `bmp` / `tiff`，每条消息一帧完整可解码图像）或码流编码（`h264` / `h265`，Annex B 基础码流分片）。

## CLI 工具

### rosctr —— 机器人控制（ROS 1 与 ROS 2 共用）

同一个 `rosctr` 二进制可同时操作 ROS 1 控制器（`ros-ctrl.sock`）和 ROS 2 控制器（`ros2-ctrl.sock`）。通过 `--socket-path` 或环境变量 `RLINF_EMBODIED_ROS_SOCKET_PATH` / `RLINF_EMBODIED_ROS2_SOCKET_PATH` 指向对应 socket。

```bash
rosctr list                                      # 机器人列表 + 状态
rosctr status <robot-id>
rosctr modes <robot-id>                          # 可用控制模式
rosctr start <robot-id> [mode]                   # 预置模式
rosctr start <robot-id> --package pkg --launch-file f.launch   # 自定义模式
rosctr switch <robot-id> <mode>                  # 切换运行中机器人的模式
rosctr stop <robot-id>
rosctr reset <robot-id>                          # 重启 roscore 并重置状态
rosctr logs <robot-id> [--tail N]
rosctr packages                                  # 白名单内的 ROS 包
rosctr pkg info <package>
rosctr pkg launch-files <package>
rosctr pkg launch-args <package> <launch-file>
rosctr env <robot-id>                            # 注入的 ROS 环境变量
```

输出格式：`-o text|json|yaml`。

### camctr —— 摄像头控制

```bash
camctr list                                      # 摄像头列表 + 状态
camctr info <camera-id>
camctr open <camera-id> [--width W --height H --fps F --encoding jpeg|png|bmp|tiff|h264|h265]
camctr close <camera-id>
camctr frame <camera-id>                         # 单帧采集
camctr watch <camera-id>                         # 持续推流
camctr watch <camera-id> --save-dir /tmp/frames  # 把帧/分片存为文件
camctr watch <camera-id> > stream.h264           # 原始码流写 stdout
camctr watch <camera-id> | ffplay -i -           # 管道喂给 ffplay
```

`watch` 按 open 时的编码推送：`jpeg` / `png` / `bmp` / `tiff` = 每条消息一帧完整、可独立解码的静图；`h264` / `h265` = Annex B 基础码流分片，客户端按顺序拼接即得完整码流。

## gRPC API

控制器通过 **Unix socket**（`/var/run/rlark/*.sock`）暴露 gRPC 服务：

| 服务 | RPC | 说明 |
|------|-----|------|
| `RobotController` | StartRobot, StopRobot, GetRobotStatus, SwitchMode, ResetRobot, ListRobots, ListModes, GetRobotLogs | 机器人生命周期管理 |
| `CameraController` | ListCameras, OpenCamera, CloseCamera, CaptureFrame, CaptureFrames, WatchFrames | 摄像头管理和帧捕获 |
| `DeviceService` | SetupDevices | 按需 macvlan 配置（由 device plugin 自身提供） |

`RobotController` 服务由两个独立控制器实现，注册在不同 socket 上：`ros-controller` 在 `ros-ctrl.sock`（ROS 1），`ros2-controller` 在 `ros2-ctrl.sock`（ROS 2）。两者遵循同一套 RPC 契约和 proto，因此单个 `rosctr` CLI 和 SDK 客户端只需指向对应 socket 即可操作任一控制器。

完整 API 参考：`proto/embodied-runtime/`。

## 准入 Webhook（自动注入 devinit）

当配置了 `host_macvlans` 时，device plugin 可启用 mutating webhook 自动向业务 Pod 注入 `devinit` init 容器。

**工作原理：**

1. 启用 `--webhook` 且配置了 `host_macvlans` 时，device plugin 启动 HTTPS webhook 拦截 Pod 的 CREATE/UPDATE。
2. 对任何申请 `rlinf.io/device[-<model>]` 的 Pod，追加一个名为 `rlark-devinit`、执行 `devinit setup` 的 init 容器。
3. 被注入的容器只声明同一设备资源 —— device plugin 的 `Allocate` 会为它注入 RunDir socket 挂载和 `RLINF_EMBODIED_DEVINIT_SOCKET_PATH` 环境变量。

该 webhook 具备**自动 CA 管理**：从 Secret 读取 CA 证书+私钥（或内存中生成自签名 CA），读取 `MutatingWebhookConfiguration`，`caBundle` 为空时自动 patch。

**device plugin webhook 参数：**

| 参数 | 用途 |
|------|------|
| `--webhook` | 启用 webhook（要求配置中存在 `host_macvlans`）。 |
| `--webhook-addr` | HTTPS 监听地址（默认 `:9443`）。 |
| `--webhook-path` | 准入端点路径（默认 `/mutate`）。 |
| `--webhook-mutating-config` | 待自动管理 `caBundle` 的 `MutatingWebhookConfiguration` 名称。 |
| `--webhook-service-name` / `--webhook-service-namespace` | 前置 webhook 的 Service。 |
| `--webhook-ca-secret-name` / `--webhook-ca-secret-namespace` | 持久化 CA 的 Secret（留空 = 内存中生成）。 |
| `--webhook-devinit-image` | 注入的 init 容器镜像（默认：自动发现）。 |

通过 Helm 启用：

```yaml
webhook:
  enabled: true
  port: 9443
  failurePolicy: Ignore   # Ignore = webhook 不可用时也放行 Pod
  caSecret:
    name: devinit-ca
    namespace: rlark-system
```

webhook 仅在 `webhook.enabled` 与 `config.hostMacvlans` **同时**设置时渲染。

## 构建

需要 Go 1.26+。

```bash
make build        # 仅编译二进制 → bin/
make test         # 单元测试
make vet          # go vet
```

二进制以 `CGO_ENABLED=0` 静态编译。

### Docker 镜像

```bash
# embodied-runtime —— 包含全部 7 个二进制（静态，多架构）
make docker REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# camera-base —— Alpine + ffmpeg 运行时依赖（不含二进制；部署时由 initContainer 注入）
make docker-camera REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# ros-base —— ROS Noetic + 控制包（用于 ROS 1 控制器）
make docker-ros REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# ros2-base —— ROS 2 Humble + 控制包（用于 ROS 2 控制器）
make docker-ros2 REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0
```

可覆盖变量：`REGISTRY`、`IMAGE_TAG` / `VERSION`、`BUILDX_PLATFORMS`、`GO_BASE_IMAGE`、`RUNTIME_BASE_IMAGE`、`APK_MIRROR`。

## 组件

| 二进制 | 包 | 职责 |
|--------|-----|------|
| `device-plugin` | `cmd/device-plugin` | kubelet 设备插件，托管所有控制器。 |
| `ros-controller` | `cmd/ros-controller` | ROS 1 机器人生命周期：roscore、roslaunch、模式切换、Web 代理。 |
| `ros2-controller` | `cmd/ros2-controller` | ROS 2 机器人生命周期：ros2 launch、DOMAIN_ID、模式切换、代理。 |
| `camera-controller` | `cmd/camera-controller` | 摄像头生命周期：打开/关闭、采集、流化、转码。 |
| `rosctr` | `cmd/rosctr` | RobotController gRPC CLI（ROS 1 **与** ROS 2 共用）。 |
| `camctr` | `cmd/camctr` | camera-controller 的 gRPC CLI。 |
| `devinit` | `cmd/devinit` | init 容器 CLI：向 device plugin 请求按需 macvlan 配置。 |

## 项目结构

```
cmd/                       # 每个二进制一个包
  device-plugin/           # kubelet 设备插件
  ros-controller/          # ROS 1 机器人生命周期
  ros2-controller/         # ROS 2 机器人生命周期
  camera-controller/       # 摄像头生命周期
  rosctr/                  # CLI（cobra）—— ROS 1 + ROS 2 共用
  camctr/                  # CLI（cobra）
  devinit/                 # init 容器 CLI
pkg/
  deviceplugin/            # kubelet 插件、配置、硬件探测、pod/local 管理器
  roscontroller/           # roscore、roslaunch、模式、MACVLAN、Web 代理、gRPC server
  ros2controller/          # ros2 launch、DOMAIN_ID、模式、gRPC server
  cameracontroller/        # ffmpeg/remote 驱动、转码器、gRPC server
  netmac/                  # 共享 MACVLAN 接口管理
examples/                  # 示例 ConfigMap + Pod 清单
runtimes/                  # camera-base、ros-base、ros2-base dockerfile
Dockerfile                 # 多阶段构建 → 全部二进制
Makefile                   # 构建 / proto / docker / lint / test
```