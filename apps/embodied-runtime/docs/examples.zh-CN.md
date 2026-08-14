# 部署与使用样例

[English](./examples.md) | 简体中文

embodied-runtime 最常见场景的端到端部署与使用样例。每个场景演示如何通过 Helm chart 配置 device plugin，再启动一个申请 `rlinf.io/device` 资源的业务 Pod，通过注入的 CLI / SDK 驱动硬件。

> 以下场景是相互独立的构建块。摄像头与机器人场景可在同一节点自由组合——见 [组合场景](#组合--同一节点上的机器人--摄像头)。

---

## 目录

- [前置条件](#前置条件)
- [场景总览](#场景总览)
- [摄像头场景](#摄像头场景)
  - [C1 — V4L2 摄像头宿主设备透传](#c1--v4l2-摄像头宿主设备透传)
  - [C2 — 通过 camera-controller 统一管理摄像头](#c2--通过-camera-controller-统一管理摄像头)
- [机器人场景](#机器人场景)
  - [R1 — USB 机器人宿主设备透传](#r1--usb-机器人宿主设备透传)
  - [R2 — 通过宿主 macvlan 接入网络机器人](#r2--通过宿主-macvlan-接入网络机器人)
  - [R3 — ROS 托管机器人](#r3--ros-托管机器人)
- [组合 — 同一节点上的机器人 + 摄像头](#组合--同一节点上的机器人--摄像头)
- [注意事项](#注意事项)
  - [ROS 隔离](#ros-隔离)
  - [ROS 2 组播](#ros-2-组播)

---

## 前置条件

1. **集群** —— 一个 Kubernetes 集群，至少一个节点挂载了硬件。
2. **Helm 3** —— `helm version` 应为 v3。
3. **镜像** —— 构建并推送运行时镜像（见主 README 的 *Build* 章节），或使用预构建镜像：
   - `rlinf/embodied-runtime:v0.1.0` —— device plugin + 全部二进制 + `devinit`。
   - `rlinf/camera-base:v0.1.0` —— Alpine + `ffmpeg`（camera-controller 用）。
   - `rlinf/serl_franka_controllers:v0.1.0-...` —— 支持 Franka 的 ROS 1 workspace 镜像，基于 `ros-base` 构建（R3a 的 ros-controller 用）。
   - `rlinf/ros2-base:v0.1.0` —— ROS 2 Humble **基础**镜像，与 `ros-base` 对应（ros2-controller 需在其上构建机器人 workspace —— 见 R3b）。
4. **节点标签 / 污点** —— 为硬件节点打标签并（可选地）打污点，使 device-plugin DaemonSet 调度到该节点、业务 Pod 能容忍：

   ```bash
   NODE=worker-1
   kubectl label nodes "$NODE" rlinf.io/robot=true
   kubectl taint nodes "$NODE" rlinf.io/robot=true:NoSchedule   # 可选，隔离该节点
   ```

   chart 默认容忍 `rlinf.io/robot`，因此 device-plugin 会自动调度到带污点的节点。业务 Pod 需重复该 toleration 才能落到该节点。

5. **安装 chart** —— 每个场景用对应的 values 文件安装：

   ```bash
   helm install embodied-runtime ./charts/embodied-runtime \
     -f <scenario-values.yaml> \
     -n rlark-system --create-namespace
   ```

   切换场景时先 `helm uninstall embodied-runtime -n rlark-system`，或为每个节点用独立的 release 名。

---

## 场景总览

| 编号 | 场景 | device-plugin 特性 | 控制器 | 业务 Pod 访问方式 |
|------|------|---------------------|--------|--------------------|
| C1 | V4L2 USB 摄像头直连 | `host_devices` | 无 | 直接打开 `/dev/videoN` |
| C2 | 摄像头统一管理 | `camera`（pod 模式） | camera-controller | `camctr` CLI / `CameraClient` SDK / REST |
| R1 | USB 机器人直连 | `host_devices` | 无 | 直接打开 `/dev/ttyUSBx` |
| R2 | 通过 macvlan 接入网络机器人 | `host_macvlans` + webhook | 无 | 通过 macvlan 用 IP 访问机器人 |
| R3 | ROS 托管机器人 | `ros` / `ros2`（pod 模式） | ros[-2]-controller | `rosctr` CLI / `RobotClient` SDK / REST |

每个场景中，业务 Pod 都申请 `rlinf.io/device`（设置了 `config.model` 时为 `rlinf.io/device-<model>`）。device plugin 的 `Allocate` 随后注入：

- socket 目录 `/var/run/rlark`（只读）—— 控制器 gRPC socket。
- CLI 目录 `/opt/rlinf/bin`（只读）—— `rosctr`、`camctr`。
- `RLINF_EMBODIED_*` 环境变量，告知 Pod 哪些运行时可用及其 socket 路径。CLI 与 SDK 会自动读取，因此通常无需 `--socket-path`。

> **调用 CLI：** 二进制目录会被挂载，但不会加入 `PATH`。请用全路径调用（`/opt/rlinf/bin/rosctr ...`），或先 `export PATH=$PATH:/opt/rlinf/bin`。

---

## 摄像头场景

### C1 — V4L2 摄像头宿主设备透传

**适用场景。** USB 摄像头被内核识别为 `/dev/videoN`，希望容器直接打开设备节点（OpenCV / `ffmpeg` / GStreamer）。无需生命周期管理、无控制器——设备节点直接透传。

**部署。** `values-c1.yaml`：

```yaml
namespace: rlark-system

config:
  device_count: 1
  hostDevices:
    - host_path: /dev/video0
      permissions: rwm
  # 不启用任何控制器。
  camera: {enabled: false}
  ros:    {enabled: false}
  ros2:   {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
helm install embodied-runtime ./charts/embodied-runtime -f values-c1.yaml -n rlark-system --create-namespace
```

**运行业务 Pod。**

```yaml
# v4l2-capture-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: v4l2-capture
  namespace: default
spec:
  containers:
    - name: app
      image: jrotten/ffmpeg:4.4-alpine      # 任何带 ffmpeg / OpenCV 的镜像
      command: ["sh", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device: 1               # → 挂载 /dev/video0 + 注入 RLINF_EMBODIED_HOST_DEVICES_ENABLED=1
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f v4l2-capture-pod.yaml
kubectl exec -it v4l2-capture -- sh
# Pod 内：/dev/video0 可读写。
ffmpeg -f v4l2 -video_size 640x480 -framerate 30 -i /dev/video0 -frames:v 1 /tmp/shot.jpg
# 或在带 OpenCV 的镜像中：  cv2.VideoCapture("/dev/video0")
```

---

### C2 — 通过 camera-controller 统一管理摄像头

**适用场景。** 希望用统一的 gRPC / REST API 打开、采集、推流、转码多个摄像头（V4L2、RTSP、RealSense），由 `camctr` CLI 或 Python `CameraClient` 驱动。camera-controller 会代为拉起 `ffmpeg`。

**部署。** `values-c2.yaml`：

```yaml
namespace: rlark-system

config:
  device_count: 1
  camera:
    enabled: true                 # pod 模式 camera-controller
    httpPort: 8080
    pod:
      image: rlinf/camera-base:v0.1.0
      pod_generate_name: camera-controller
      subdomain: camera-controller-headless
      labels:
        app.kubernetes.io/name: camera-controller
    auto_detect_v4l2: true        # 自动注册节点上所有 /dev/videoN
    cameras:                      # 额外 / 非 v4l2 摄像头（与自动探测结果合并）
      - id: ptz-cam
        name: "RTSP PTZ camera"
        camera_type: rtsp
        params:
          url: rtsp://10.0.1.50/stream1
          transport: tcp
  ros:  {enabled: false}
  ros2: {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

chart 会渲染 headless Service `camera-controller-headless`，使控制器的 HTTP/JSON 网关在集群内可达。

**运行业务 Pod。**

```yaml
# camera-task-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: camera-task
  namespace: default
spec:
  containers:
    - name: app
      image: python:3.11-slim
      command: ["sh", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device: 1       # → 注入摄像头 socket + camctr
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f camera-task-pod.yaml
```

**用法 — CLI**（`camctr` 自动读取 `RLINF_EMBODIED_CAMERA_SOCKET_PATH`）：

> 自动探测的 V4L2 摄像头以 sysfs 名作为 ID 注册——例如 `/dev/video0` 对应 `video0`。用 `camctr list` 查看节点上的实际 ID；手动声明的 `ptz-cam` 会与它们并列出现。

```bash
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr list
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr open video0 --encoding h264
# 推流到文件，或管道喂给你工作机上的 ffplay：
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr watch video0 > stream.h264
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr frame video0 -o json   # 单帧采集
```

**用法 — Python SDK**：

```bash
kubectl exec -it camera-task -- pip install embodied-runtime
kubectl exec -it camera-task -- python - <<'PY'
from embodied_runtime import CameraClient
with CameraClient() as cam:                       # 读取 RLINF_EMBODIED_CAMERA_SOCKET_PATH
    cam.open_camera("video0", encoding="h264")
    for f in cam.watch_frames("video0"):
        print(f.sequence, f.encoding, len(f.data), "keyframe" if f.keyframe else "")
        if f.sequence >= 10:
            break
PY
```

**用法 — REST**（经 headless Service 的 HTTP/JSON 网关）：

```bash
NODE=worker-1
curl "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras"
curl -XPOST "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras/video0/open"
curl "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras/video0/watch"
```

---

## 机器人场景

### R1 — USB 机器人宿主设备透传

**适用场景。** 机器人或设备暴露为串口 / 字符设备（如 `/dev/ttyUSB0`、`/dev/ttyACM0`）。容器直接打开设备——无需 ROS、无需控制器。

**部署。** `values-r1.yaml`：

```yaml
namespace: rlark-system

config:
  device_count: 1
  hostDevices:
    - host_path: /dev/ttyUSB0
      permissions: rw
  camera: {enabled: false}
  ros:    {enabled: false}
  ros2:   {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

**运行业务 Pod** 并与串口设备通信：

```bash
kubectl exec -it usb-robot-task -- sh
# /dev/ttyUSB0 在 Pod 内以读写方式挂载。
python - <<'PY'
import serial
s = serial.Serial("/dev/ttyUSB0", 115200, timeout=1)
print(s.read(100))
PY
```

---

### R2 — 通过宿主 macvlan 接入网络机器人

**适用场景。** 通过以太网可达的机器人（例如机器人子网 `172.16.0.0/24` 上的 Franka 机械臂 `172.16.0.2`）。Pod 需要一个该子网上的网络接口才能用 IP 访问机器人。不涉及 ROS 控制器——Pod 直接用 TCP/HTTP 与机器人通信。

macvlan 无法通过 Allocate 挂载（必须在 Pod 网络命名空间内创建），因此 device plugin 提供按需 gRPC 服务，并由 **mutating webhook 自动向每个申请资源的 Pod 注入 `devinit` init 容器**。init 容器请求插件把已配置的 macvlan 注入到 Pod 网络命名空间；主容器随后使用该接口。

**部署。** `values-r2.yaml`：

```yaml
namespace: rlark-system

config:
  device_count: 1
  hostMacvlans:
    - host_nic: eno1              # 接入机器人子网的宿主网卡（ip 为空时按子网自动探测）
      name: macvlan0              # Pod 内的接口名
      ip: 172.16.0.0/24           # 网络地址 → 插件自动挑选未用主机 IP
      gateway: 172.16.0.1        # 可选，机器人子网默认网关
  camera: {enabled: false}
  ros:    {enabled: false}
  ros2:   {enabled: false}

webhook:
  enabled: true                   # 自动注入 devinit init 容器
  failurePolicy: Ignore           # Ignore = webhook 不可用时也放行 Pod（无 macvlan）
  caSecret:
    name: devinit-ca
    namespace: rlark-system

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
helm install embodied-runtime ./charts/embodied-runtime -f values-r2.yaml -n rlark-system --create-namespace
```

> chart 仅在 `webhook.enabled` 与 `config.hostMacvlans` **同时**设置时才启用 webhook。`hostMacvlans` 非空时 DaemonSet 还会设置 `hostPID: true`（插件需通过 socket 对端凭证读取调用方 PID 所必需）。

**运行业务 Pod。** webhook 会注入 `devinit` init 容器——你只需编写主容器：

```yaml
# network-robot-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: network-robot-task
  namespace: default
spec:
  containers:
    - name: app
      image: curlimages/curl:8.5.0
      command: ["sh", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device: 1       # → 挂载 RunDir + webhook 自动注入 devinit
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f network-robot-pod.yaml
kubectl exec -it network-robot-task -- sh
ip addr show macvlan0            # devinit 已注入的接口
ip route
curl -k https://172.16.0.2/      # 直接在机器人子网上访问机器人
```

若不想用 webhook，可自行编写 init 容器（只需申请资源并运行 `devinit setup`）：

```yaml
initContainers:
  - name: devinit
    image: rlinf/embodied-runtime:v0.1.0
    command: ["devinit", "setup"]   # 读取 RLINF_EMBODIED_DEVINIT_SOCKET_PATH
    resources:
      requests:
        rlinf.io/device: 1          # 触发 Allocate → RunDir 挂载 + 环境变量
      limits:                        # 必填：LimitRanger/ResourceQuota 会拒绝未设置 limits 的 init 容器
        rlinf.io/device: 1           # 扩展资源的 limits 必须等于 requests
```

---

### R3 — ROS 托管机器人

**适用场景。** 处于 ROS 生态的机器人（如 Franka），希望通过 ROS 驱动——启停控制模式、launch 包、在 impedance / joint 控制间切换。ros-controller（ROS 1）或 ros2-controller（ROS 2）管理 `roscore` / `ros2 launch` 生命周期；业务 Pod 通过 `rosctr` 或 `RobotClient` SDK 驱动。

控制器在自己的 Pod（pod 模式）中用 ROS workspace 镜像运行，并自行创建 macvlan 以接入机器人子网。业务 Pod 通过注入的 Unix socket 与控制器通信——**无需**自带 macvlan。

设置 `config.model: franka`，使申报的资源变为 `rlinf.io/device-franka`（区分机器人节点与纯摄像头节点），socket 变为 `rlinf-device-franka.sock`。

#### R3a — ROS 1（Noetic）

**部署。** `values-r3a.yaml`：

```yaml
namespace: rlark-system

config:
  model: franka                  # → 资源 rlinf.io/device-franka
  device_count: 1
  ros:
    enabled: true                # pod 模式 ros-controller
    httpPort: 8080
    pod:
      image: rlinf/serl_franka_controllers:v0.1.0-libfranka-0.19.0-frankaros-0.10.2
      pod_generate_name: ros-controller
      subdomain: ros-controller-headless
      labels:
        app.kubernetes.io/name: ros-controller
    macvlans:                     # 控制器侧 macvlan，用于接入机器人
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
  camera: {enabled: false}
  ros2:   {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

**运行业务 Pod。** 注意设置了 `model`，资源名带 `-franka` 后缀：

```yaml
# ros-task-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: ros-task
  namespace: default
spec:
  containers:
    - name: app
      image: python:3.11-slim
      command: ["sh", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device-franka: 1   # → 注入 ROS socket + rosctr；单机器人时注入 ROS_MASTER_URI
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f ros-task-pod.yaml
```

**用法 — CLI**（`rosctr` 自动读取 `RLINF_EMBODIED_ROS_SOCKET_PATH`）：

```bash
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr list
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr modes franka-robot-1
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr status franka-robot-1
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr switch franka-robot-1 joint
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr stop franka-robot-1
```

**用法 — Python SDK**：

```bash
kubectl exec -it ros-task -- pip install embodied-runtime
kubectl exec -it ros-task -- python - <<'PY'
from embodied_runtime import RobotClient
with RobotClient() as robot:                   # 读取 RLINF_EMBODIED_ROS_SOCKET_PATH
    robot.start_robot("franka-robot-1", mode="impedance")
    s = robot.get_robot_status("franka-robot-1")
    print(s.robot_id, s.mode, s.ros_master_uri)
PY
```

**直接使用 ROS（Franka CLI）。** 控制器在自身 Pod IP 上运行 `roscore`，端口从 `11311` 起递增，因此带 ROS 的工作负载 Pod 像普通 ROS 客户端一样接入机器人——只需配好环境变量与 `franka_msgs` workspace。当你想用 `rostopic` / `rviz` / rospy 节点而非 gRPC SDK 时走这条路。

**1. 使用 ROS workspace 镜像。** 控制器所用的同一镜像已内置 ROS Noetic + `franka_msgs` + 控制器包，工作负载 Pod 直接复用即可：

```yaml
# ros-franka-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: ros-franka-task
  namespace: default
spec:
  containers:
    - name: app
      image: rlinf/serl_franka_controllers:v0.1.0-libfranka-0.19.0-frankaros-0.10.2
      command: ["bash", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device-franka: 1   # → 注入 ROS socket + rosctr；单机器人时注入 ROS_MASTER_URI
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f ros-franka-pod.yaml
```

**2. 以某控制模式启动机器人。** impedance 模式会拉起 `cartesian_impedance_controller`，暴露出你用于下发运动目标的 equilibrium-pose topic：

```bash
kubectl exec -it ros-franka-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
```

**3. 接入 —— 设置 ROS 环境变量。** 单机器人时 device plugin 已注入 `ROS_MASTER_URI`，但你仍需 source workspace 并设置 `ROS_IP`（本 Pod 的 IP，供机器人回连）。`rosctr env` 打印精确的 `ROS_MASTER_URI` 供 source，多机器人 Pod 也用它：

```bash
kubectl exec -it ros-franka-task -- bash
# Pod 内：
source /opt/ros/noetic/setup.bash
source /catkin_ws/devel_isolated/setup.bash          # 提供 franka_msgs / geometry_msgs
export ROS_IP=$(hostname -I | awk '{print $1}')      # 本 Pod IP —— 双向 ROS 通信必需
. <(/opt/rlinf/bin/rosctr env franka-robot-1)          # export ROS_MASTER_URI=http://<controller-pod-ip>:11311
echo "$ROS_MASTER_URI"
rostopic list                                         # /cartesian_impedance_controller/equilibrium_pose, ...
```

> 若省略 `rosctr env`，单机器人下 `ROS_MASTER_URI` 仍可用（Allocate 时自动注入）。但 `ROS_IP` **不会**被注入——只要工作负载有发布/订阅，就需自行设置，否则机器人节点无法回连你的 Pod。

**4. 通过 ROS topic 操控机器人。** impedance 控制器的接口如下：

| 动作 | Topic | 消息类型 |
|------|-------|----------|
| 读取机器人状态 | `franka_state_controller/franka_states` | `franka_msgs/FrankaState` |
| 移动 TCP（equilibrium pose） | `/cartesian_impedance_controller/equilibrium_pose` | `geometry_msgs/PoseStamped` |
| 错误恢复 | `/franka_control/error_recovery/goal` | `franka_msgs/ErrorRecoveryActionGoal` |

把这些 topic 包一层薄薄的 rospy CLI，即可用 `status` / `move` / `clear_errors`（改编自一个 Franka 辅助脚本）：

```bash
python frankacli.py status                                   # TCP 位姿、关节、力、错误
python frankacli.py move 0.3 0.0 0.4 0.0 0.0 0.0 1.0          # x y z qx qy qz qw @ 100 Hz
python frankacli.py clear_errors
```

等价的 `rostopic` 单行命令（无需额外代码）：

```bash
# 当前状态
rostopic echo -n1 franka_state_controller/franka_states

# 以 100 Hz 发布运动目标
rostopic pub -r 100 /cartesian_impedance_controller/equilibrium_pose \
  geometry_msgs/PoseStamped \
  "{header: {frame_id: '0'}, pose: {position: {x: 0.3, y: 0.0, z: 0.4}, orientation: {x: 0, y: 0, z: 0, w: 1}}}"

# 发布错误恢复 goal
rostopic pub -1 /franka_control/error_recovery/goal franka_msgs/ErrorRecoveryActionGoal "{}"
```

一个最小化的 rospy 状态读取器（即 `frankacli.py status` 的核心），使片段自包含：

```python
#!/usr/bin/env python3
import numpy as np, rospy
from franka_msgs.msg import FrankaState

rospy.init_node("franka_status", anonymous=True)
msg = rospy.wait_for_message("franka_state_controller/franka_states", FrankaState, timeout=5.0)

T = np.array(msg.O_T_EE).reshape(4, 4).T            # TCP 位姿
print("TCP position:", T[:3, -1])
print("joints:      ", list(msg.q))
print("mode:        ", msg.robot_mode, " success:", msg.control_command_success_rate)
```

#### R3b — ROS 2（Humble）

把 `ros` 换成 `ros2`，用 `.launch.py` 文件，可按机器人固定 `domain_id`。单机器人时插件注入 `ROS_DOMAIN_ID`，使 Pod 内的 ROS 2 工具加入正确的 DDS 域。

> **镜像层级 —— `ros2-base` 是基础镜像，不是 Franka 镜像。** `ros2-base` 是 `ros-base` 的 ROS 2 对应（二者都是带 ROS + 控制栈的基础镜像，与具体机器人无关），它**不是** R3a 中 `serl_franka_controllers` 的替代——后者是基于 `ros-base` 之上构建的 Franka ROS 1 workspace。要在 ROS 2 下驱动 Franka，需在 `ros2-base` 之上构建/使用 Franka ROS 2 workspace 镜像（即 `serl_franka_controllers` 的 ROS 2 对应），并设为 `config.ros2.pod.image`。下方引用的 `moveit_servo` 包必须存在于该 workspace 镜像中——裸 `ros2-base` 并不含它。

```yaml
namespace: rlark-system

config:
  model: franka
  device_count: 1
  ros2:
    enabled: true
    httpPort: 8080
    pod:
      # 请替换为基于 ros2-base 构建的 Franka ROS 2 workspace 镜像
      # （serl_franka_controllers 的 ROS 2 对应）。它须包含各模式
      # 要 launch 的包（如 moveit_servo）。
      image: rlinf/ros2-base:v0.1.0
      pod_generate_name: ros2-controller
      subdomain: ros2-controller-headless
      labels:
        app.kubernetes.io/name: ros2-controller
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
        domain_id: 5            # 显式 ROS_DOMAIN_ID（为 0 时自动分配）
    allowed_launch_packages:
      - moveit_servo
  camera: {enabled: false}
  ros:    {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

业务 Pod 与 R3a 完全一致（`rlinf.io/device-franka: 1`）。当 ROS 1 socket 不存在时，`rosctr` 自动通过 `RLINF_EMBODIED_ROS2_SOCKET_PATH` 指向 ROS 2 控制器。同一套 RPC 均可用：

```bash
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr list
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr env franka-robot-1   # 显示 ROS_DOMAIN_ID / RMW_IMPLEMENTATION
```

> ROS 2 DDS 发现依赖 IP 组播。跨 Pod / 跨节点运行 ROS 2 前，请先阅读 [ROS 2 组播](#ros-2-组播)。

---

## 组合 — 同一节点上的机器人 + 摄像头

不同类型的机器人与摄像头可自由组合。典型的具身智能数据采集节点：一台 Franka 机械臂 **加** 一或多台摄像头，外加一个训练 / 遥操作 Pod，边驱动机器人边推流腕部摄像头。在同一 release 中同时启用 ROS 控制器与摄像头控制器：

```yaml
namespace: rlark-system

config:
  model: franka
  device_count: 1
  ros:
    enabled: true
    httpPort: 8080
    pod:
      image: rlinf/serl_franka_controllers:v0.1.0-libfranka-0.19.0-frankaros-0.10.2
      pod_generate_name: ros-controller
      subdomain: ros-controller-headless
      labels:
        app.kubernetes.io/name: ros-controller
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
    robots:
      - id: franka-robot-1
        type: franka
        params:
          robot_ip: 172.16.0.2
        web_service: https://172.16.0.2/
    allowed_launch_packages:
      - serl_franka_controllers
  camera:
    enabled: true
    httpPort: 8080
    pod:
      image: rlinf/camera-base:v0.1.0
      pod_generate_name: camera-controller
      subdomain: camera-controller-headless
      labels:
        app.kubernetes.io/name: camera-controller
    auto_detect_v4l2: true
  ros2: {enabled: false}

devicePlugin:
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

单个业务 Pod 申请 `rlinf.io/device-franka: 1`，即可同时获得 ROS socket（`ros-ctrl.sock`）与摄像头 socket（`camera-ctrl.sock`），以及两套 CLI 与单机器人的 `ROS_MASTER_URI`。在一个进程内同时驱动两者：

```python
# teleop.py —— 在申请 rlinf.io/device-franka 的 Pod 中运行
from embodied_runtime import RobotClient, CameraClient

with RobotClient() as robot, CameraClient() as cam:
    robot.start_robot("franka-robot-1", mode="impedance")
    cam.open_camera("video0", encoding="jpeg")

    for frame in cam.watch_frames("video0"):
        # frame.data 是一帧完整 JPEG；喂给你的策略模型 / 数据记录器
        record_step(robot.get_robot_status("franka-robot-1"), frame.data)
```

自由搭配：把 ROS 1 控制器换成 ROS 2、在 `config.camera.cameras` 下加 RTSP 摄像头，或为同一节点加 `hostDevices` 额外透传一个 USB 设备——全部在同一个 release 中完成。

---

## 注意事项

### ROS 隔离

- **ROS 1。** 每个机器人独占一个 `roscore`（端口从 `11311` 起递增），同一控制器内的多机器人不会在 topic / 节点名上冲突。仅当恰好一个机器人时插件才注入 `ROS_MASTER_URI`；多个时由调用方通过控制器区分。如需更严格的跨租户隔离，为每个命名空间部署独立 device-plugin 实例，并用 Kubernetes `NetworkPolicy` 限制哪些 Pod 能访问某控制器的 ROS master 端口。

- **ROS 2。** DDS 级隔离来自每机器人独占的 `ROS_DOMAIN_ID`（自动分配或通过 `domain_id` 固定）；不同域 ID 的机器人互不发现。如需更强隔离，可再用命名空间与 `NetworkPolicy` 划分集群网络，并/或使用单播发现（见下）。

### ROS 2 组播

ROS 2 DDS 默认使用 IP 组播进行节点发现。集群的网络层（CNI 插件、节点 / 底层交换机）**必须支持组播路由**，跨 Pod / 跨节点的 ROS 2 节点才能互相发现。若集群屏蔽组播（云厂商 CNI 常见），可任选其一：

- **单播发现** —— 在每个 robot 的 mode 级 `env` 中设置 `CYCLONEDDS_URI`（或 Fast DDS 等价配置）为 peer-list XML profile，让节点显式发现对端而非依赖组播：

  ```yaml
  types:
    - type: franka
      modes:
        impedance:
          package: moveit_servo
          launch_file: servo.launch.py
          passthrough_robot_args: true
          env:
            RMW_IMPLEMENTATION: rmw_cyclonedds_cpp
            CYCLONEDDS_URI: |
              <CycloneDDS><Domain><General><Interfaces><NetworkInterface name="macvlan0" /></Interfaces></General></Domain></CycloneDDS>
  ```

- **DDS Discovery Server** —— 部署一个所有 ROS 2 Pod 可达的 Discovery Server，让每个机器人的 DDS 配置指向它。

控制器的 MACVLAN L2 接口可直接接入机器人物理子网（该子网内组播通常可用），但 Pod 间 / 节点间的发现仍取决于集群网络。

> **提示。** 若从零搭建集群，[`ros_k8s`](https://github.com/fujitatomoya/ros_k8s) 涵盖了 Kubernetes 与 KubeEdge 上的 ROS / ROS 2，包括上述组播 / 发现决策所依赖的容器与网络配置。
