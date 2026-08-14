# Deployment & usage examples

English | [简体中文](./examples.zh-CN.md)

End-to-end deployment and usage walkthroughs for the most common embodied-runtime scenarios. Each scenario shows how to configure the device plugin (via the Helm chart), then run a workload pod that requests the `rlinf.io/device` resource and drives the hardware through the injected CLIs / SDKs.

> The scenarios below are independent building blocks. Camera and robot scenarios can be freely combined on the same node — see the [combined scenario](#combined--robot--camera-on-one-node).

---

## Contents

- [Prerequisites](#prerequisites)
- [Scenario overview](#scenario-overview)
- [Camera scenarios](#camera-scenarios)
  - [C1 — V4L2 camera via host device passthrough](#c1--v4l2-camera-via-host-device-passthrough)
  - [C2 — Camera via the camera-controller (unified management)](#c2--camera-via-the-camera-controller-unified-management)
- [Robot scenarios](#robot-scenarios)
  - [R1 — USB robot via host device passthrough](#r1--usb-robot-via-host-device-passthrough)
  - [R2 — Network robot via host macvlan](#r2--network-robot-via-host-macvlan)
  - [R3 — ROS-managed robot](#r3--ros-managed-robot)
- [Combined — robot + camera on one node](#combined--robot--camera-on-one-node)
- [Notes](#notes)
  - [ROS isolation](#ros-isolation)
  - [ROS 2 multicast](#ros-2-multicast)

---

## Prerequisites

1. **Cluster** — a Kubernetes cluster with at least one node hosting hardware.
2. **Helm 3** — `helm version` should report v3.
3. **Images** — build & push the runtime images (see the main README's *Build* section), or use prebuilt ones:
   - `rlinf/embodied-runtime:v0.1.0` — device plugin + all binaries + `devinit`.
   - `rlinf/camera-base:v0.1.0` — Alpine + `ffmpeg` (for the camera-controller).
   - `rlinf/serl_franka_controllers:v0.1.0-...` — Franka ROS 1 workspace image, built on `ros-base` (for the ros-controller in R3a).
   - `rlinf/ros2-base:v0.1.0` — ROS 2 Humble **base** image, the counterpart of `ros-base` (the ros2-controller needs a robot workspace built on top — see R3b).
4. **Node labels / taints** — label and (optionally) taint the hardware node so the device-plugin DaemonSet lands there and workload pods tolerate it:

   ```bash
   NODE=worker-1
   kubectl label nodes "$NODE" rlinf.io/robot=true
   kubectl taint nodes "$NODE" rlinf.io/robot=true:NoSchedule   # optional, isolates the node
   ```

   The chart tolerates `rlinf.io/robot` by default, so the device-plugin schedules onto tainted nodes automatically. Workload pods must repeat the toleration to land there.

5. **Install the chart** once per scenario using the values file shown in that scenario:

   ```bash
   helm install embodied-runtime ./charts/embodied-runtime \
     -f <scenario-values.yaml> \
     -n rlark-system --create-namespace
   ```

   To switch scenarios, `helm uninstall embodied-runtime -n rlark-system` first, or use a separate release name per node.

---

## Scenario overview

| ID | Scenario | Device-plugin feature | Controller | Workload access |
|----|----------|-----------------------|------------|-----------------|
| C1 | V4L2 USB camera direct | `host_devices` | none | open `/dev/videoN` directly |
| C2 | Camera unified management | `camera` (pod mode) | camera-controller | `camctr` CLI / `CameraClient` SDK / REST |
| R1 | USB robot direct | `host_devices` | none | open `/dev/ttyUSBx` directly |
| R2 | Network robot via macvlan | `host_macvlans` + webhook | none | reach robot by IP over macvlan |
| R3 | ROS-managed robot | `ros` / `ros2` (pod mode) | ros[-2]-controller | `rosctr` CLI / `RobotClient` SDK / REST |

In every scenario the workload pod requests `rlinf.io/device` (or `rlinf.io/device-<model>` when `config.model` is set). The device plugin's `Allocate` then injects:

- The socket directory `/var/run/rlark` (read-only) — controller gRPC sockets.
- The CLI directory `/opt/rlinf/bin` (read-only) — `rosctr`, `camctr`.
- `RLINF_EMBODIED_*` env vars telling the pod which runtimes are available and where their sockets live. The CLIs and SDKs read them automatically, so you usually don't need `--socket-path`.

> **Calling the CLIs:** the binary dir is mounted but not added to `PATH`. Either call them by full path (`/opt/rlinf/bin/rosctr ...`) or `export PATH=$PATH:/opt/rlinf/bin` first.

---

## Camera scenarios

### C1 — V4L2 camera via host device passthrough

**When to use.** A USB camera that the kernel exposes as `/dev/videoN`. You want the container to open the device node directly (OpenCV / `ffmpeg` / GStreamer). No lifecycle management, no controller — the device node is simply passed through.

**Deploy.** `values-c1.yaml`:

```yaml
namespace: rlark-system

config:
  device_count: 1
  hostDevices:
    - host_path: /dev/video0
      permissions: rwm
  # No controllers.
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

**Run a workload pod.**

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
      image: jrotten/ffmpeg:4.4-alpine      # any image with ffmpeg / OpenCV
      command: ["sh", "-c", "sleep 3600"]
      resources:
        requests:
          rlinf.io/device: 1               # → /dev/video0 mounted + RLINF_EMBODIED_HOST_DEVICES_ENABLED=1
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f v4l2-capture-pod.yaml
kubectl exec -it v4l2-capture -- sh
# Inside the pod: /dev/video0 is available read/write.
ffmpeg -f v4l2 -video_size 640x480 -framerate 30 -i /dev/video0 -frames:v 1 /tmp/shot.jpg
# or, with an OpenCV image:  cv2.VideoCapture("/dev/video0")
```

---

### C2 — Camera via the camera-controller (unified management)

**When to use.** You want a single gRPC / REST API to open, capture, stream, and transcode multiple cameras (V4L2, RTSP, RealSense), driven by the `camctr` CLI or the Python `CameraClient`. The camera-controller spawns `ffmpeg` for you.

**Deploy.** `values-c2.yaml`:

```yaml
namespace: rlark-system

config:
  device_count: 1
  camera:
    enabled: true                 # pod-mode camera-controller
    httpPort: 8080
    pod:
      image: rlinf/camera-base:v0.1.0
      pod_generate_name: camera-controller
      subdomain: camera-controller-headless
      labels:
        app.kubernetes.io/name: camera-controller
    auto_detect_v4l2: true        # auto-register every /dev/videoN on the node
    cameras:                      # extra / non-v4l2 cameras (merged with auto-detected)
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

The chart renders a headless Service `camera-controller-headless` so the controller's HTTP/JSON gateway is reachable cluster-wide.

**Run a workload pod.**

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
          rlinf.io/device: 1       # → camera socket + camctr injected
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f camera-task-pod.yaml
```

**Usage — CLI** (`camctr` auto-detects `RLINF_EMBODIED_CAMERA_SOCKET_PATH`):

> Auto-detected V4L2 cameras are registered with their sysfs name as the ID — e.g. `video0` for `/dev/video0`. Use `camctr list` to see the exact IDs on your node; the manually-declared `ptz-cam` appears alongside them.

```bash
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr list
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr open video0 --encoding h264
# stream to a file, or pipe to ffplay on your workstation:
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr watch video0 > stream.h264
kubectl exec -it camera-task -- /opt/rlinf/bin/camctr frame video0 -o json   # single capture
```

**Usage — Python SDK**:

```bash
kubectl exec -it camera-task -- pip install embodied-runtime
kubectl exec -it camera-task -- python - <<'PY'
from embodied_runtime import CameraClient
with CameraClient() as cam:                       # reads RLINF_EMBODIED_CAMERA_SOCKET_PATH
    cam.open_camera("video0", encoding="h264")
    for f in cam.watch_frames("video0"):
        print(f.sequence, f.encoding, len(f.data), "keyframe" if f.keyframe else "")
        if f.sequence >= 10:
            break
PY
```

**Usage — REST** (HTTP/JSON gateway over the headless Service):

```bash
NODE=worker-1
curl "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras"
curl -XPOST "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras/video0/open"
curl "http://camera-controller-$NODE.camera-controller-headless.rlark-system.svc:8080/v1/cameras/video0/watch"
```

---

## Robot scenarios

### R1 — USB robot via host device passthrough

**When to use.** A robot or device that exposes a serial / character device (e.g. `/dev/ttyUSB0`, `/dev/ttyACM0`). The container opens the device directly — no ROS, no controller.

**Deploy.** `values-r1.yaml`:

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

**Run a workload pod** and talk to the serial device:

```bash
kubectl exec -it usb-robot-task -- sh
# /dev/ttyUSB0 is mounted read/write inside the pod.
python - <<'PY'
import serial
s = serial.Serial("/dev/ttyUSB0", 115200, timeout=1)
print(s.read(100))
PY
```

---

### R2 — Network robot via host macvlan

**When to use.** A robot reachable over Ethernet (e.g. a Franka arm at `172.16.0.2` on the robot subnet). The pod needs a network interface on that subnet so it can reach the robot by IP. No ROS controller is involved — the pod talks raw TCP/HTTP to the robot.

A macvlan cannot be Allocate-mounted (it must be created inside the pod's network namespace), so the device plugin runs an on-demand gRPC service and a **mutating webhook auto-injects a `devinit` init container** into any pod requesting the resource. The init container asks the plugin to attach the configured macvlan into the pod netns; the main container then uses that interface.

**Deploy.** `values-r2.yaml`:

```yaml
namespace: rlark-system

config:
  device_count: 1
  hostMacvlans:
    - host_nic: eno1              # host NIC wired to the robot subnet (auto-detected from ip if empty)
      name: macvlan0              # interface name inside the pod
      ip: 172.16.0.0/24           # network addr → the plugin auto-picks an unused host IP
      gateway: 172.16.0.1        # optional default gateway for the robot subnet
  camera: {enabled: false}
  ros:    {enabled: false}
  ros2:   {enabled: false}

webhook:
  enabled: true                   # auto-inject the devinit init container
  failurePolicy: Ignore           # Ignore = admit the pod even if the webhook is down (no macvlan)
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

> The chart enables the webhook only when **both** `webhook.enabled` and `config.hostMacvlans` are set. When `hostMacvlans` is non-empty the DaemonSet also sets `hostPID: true` (required for the plugin to read the caller's PID via socket peer credentials).

**Run a workload pod.** The webhook injects the `devinit` init container — you only author the main container:

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
          rlinf.io/device: 1       # → RunDir mount + devinit auto-injected by the webhook
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f network-robot-pod.yaml
kubectl exec -it network-robot-task -- sh
ip addr show macvlan0            # interface attached by devinit
ip route
curl -k https://172.16.0.2/      # reach the robot directly on its subnet
```

If you prefer not to use the webhook, author the init container yourself (it just requests the resource and runs `devinit setup`):

```yaml
initContainers:
  - name: devinit
    image: rlinf/embodied-runtime:v0.1.0
    command: ["devinit", "setup"]   # reads RLINF_EMBODIED_DEVINIT_SOCKET_PATH
    resources:
      requests:
        rlinf.io/device: 1          # triggers Allocate → RunDir mount + env vars
      limits:                        # required: LimitRanger/ResourceQuota rejects init containers without limits
        rlinf.io/device: 1           # extended-resource limits must equal requests
```

---

### R3 — ROS-managed robot

**When to use.** A robot in the ROS ecosystem (e.g. Franka) that you want to drive through ROS — start/stop control modes, launch packages, switch between impedance / joint control. The ros-controller (ROS 1) or ros2-controller (ROS 2) manages `roscore` / `ros2 launch` lifecycle; workload pods drive it via `rosctr` or the `RobotClient` SDK.

The controller runs in its own Pod (pod mode) using a ROS workspace image; it creates its own macvlan to reach the robot on the robot subnet. Workload pods talk to the controller over the injected Unix socket — they do **not** need a macvlan themselves.

Set `config.model: franka` so the advertised resource becomes `rlinf.io/device-franka` (distinguishing robot nodes from camera-only nodes) and the socket becomes `rlinf-device-franka.sock`.

#### R3a — ROS 1 (Noetic)

**Deploy.** `values-r3a.yaml`:

```yaml
namespace: rlark-system

config:
  model: franka                  # → resource rlinf.io/device-franka
  device_count: 1
  ros:
    enabled: true                # pod-mode ros-controller
    httpPort: 8080
    pod:
      image: rlinf/serl_franka_controllers:v0.1.0-libfranka-0.19.0-frankaros-0.10.2
      pod_generate_name: ros-controller
      subdomain: ros-controller-headless
      labels:
        app.kubernetes.io/name: ros-controller
    macvlans:                     # controller-side macvlan to reach the robot
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

**Run a workload pod.** Note the resource name carries the `-franka` suffix because `model` is set:

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
          rlinf.io/device-franka: 1   # → ROS socket + rosctr injected; single-robot ROS_MASTER_URI injected
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f ros-task-pod.yaml
```

**Usage — CLI** (`rosctr` auto-detects `RLINF_EMBODIED_ROS_SOCKET_PATH`):

```bash
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr list
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr modes franka-robot-1
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr status franka-robot-1
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr switch franka-robot-1 joint
kubectl exec -it ros-task -- /opt/rlinf/bin/rosctr stop franka-robot-1
```

**Usage — Python SDK**:

```bash
kubectl exec -it ros-task -- pip install embodied-runtime
kubectl exec -it ros-task -- python - <<'PY'
from embodied_runtime import RobotClient
with RobotClient() as robot:                   # reads RLINF_EMBODIED_ROS_SOCKET_PATH
    robot.start_robot("franka-robot-1", mode="impedance")
    s = robot.get_robot_status("franka-robot-1")
    print(s.robot_id, s.mode, s.ros_master_uri)
PY
```

**Using ROS directly (Franka CLI).** The controller runs `roscore` on its own pod IP and a unique port (from `11311`), so a ROS-equipped workload pod connects to the robot as an ordinary ROS client — it just needs the right environment and the `franka_msgs` workspace. This is the path to take when you want `rostopic` / `rviz` / a rospy node rather than the gRPC SDK.

**1. Use a ROS workspace image.** The same image the controller uses already has ROS Noetic + `franka_msgs` + the controller packages, so reuse it for the workload pod:

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
          rlinf.io/device-franka: 1   # → ROS socket + rosctr; single-robot ROS_MASTER_URI injected
  tolerations:
    - key: rlinf.io/robot
      operator: Exists
      effect: NoSchedule
```

```bash
kubectl apply -f ros-franka-pod.yaml
```

**2. Start the robot in a control mode.** The impedance mode launches `cartesian_impedance_controller`, which exposes the equilibrium-pose topic you publish motion targets to:

```bash
kubectl exec -it ros-franka-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
```

**3. Connect — set the ROS environment.** With a single robot the device plugin already injects `ROS_MASTER_URI`, but you still need to source the workspace and set `ROS_IP` (your pod's own IP, so the robot can call you back). `rosctr env` prints the exact `ROS_MASTER_URI` to source and is also the right tool for multi-robot pods:

```bash
kubectl exec -it ros-franka-task -- bash
# inside the pod:
source /opt/ros/noetic/setup.bash
source /catkin_ws/devel_isolated/setup.bash          # provides franka_msgs / geometry_msgs
export ROS_IP=$(hostname -I | awk '{print $1}')      # this pod's IP — needed for bidirectional ROS traffic
. <(/opt/rlinf/bin/rosctr env franka-robot-1)          # export ROS_MASTER_URI=http://<controller-pod-ip>:11311
echo "$ROS_MASTER_URI"
rostopic list                                         # /cartesian_impedance_controller/equilibrium_pose, ...
```

> If `rosctr env` is omitted, `ROS_MASTER_URI` still works for a single robot (auto-injected at Allocate). `ROS_IP` is *not* injected — set it yourself whenever the workload publishes or subscribes, otherwise the robot's nodes cannot resolve a return path to your pod.

**4. Drive the robot over ROS topics.** The impedance controller's interface is:

| Action | Topic | Message type |
|--------|-------|--------------|
| Read robot state | `franka_state_controller/franka_states` | `franka_msgs/FrankaState` |
| Move TCP (equilibrium pose) | `/cartesian_impedance_controller/equilibrium_pose` | `geometry_msgs/PoseStamped` |
| Recover from errors | `/franka_control/error_recovery/goal` | `franka_msgs/ErrorRecoveryActionGoal` |

A thin rospy CLI around those topics gives you `status` / `move` / `clear_errors` (adapted from a Franka helper script):

```bash
python frankacli.py status                                   # TCP pose, joints, forces, errors
python frankacli.py move 0.3 0.0 0.4 0.0 0.0 0.0 1.0          # x y z qx qy qz qw @ 100 Hz
python frankacli.py clear_errors
```

The equivalent `rostopic` one-liners (no extra code):

```bash
# current state
rostopic echo -n1 franka_state_controller/franka_states

# publish a motion target at 100 Hz
rostopic pub -r 100 /cartesian_impedance_controller/equilibrium_pose \
  geometry_msgs/PoseStamped \
  "{header: {frame_id: '0'}, pose: {position: {x: 0.3, y: 0.0, z: 0.4}, orientation: {x: 0, y: 0, z: 0, w: 1}}}"

# publish an error-recovery goal
rostopic pub -1 /franka_control/error_recovery/goal franka_msgs/ErrorRecoveryActionGoal "{}"
```

A minimal rospy status reader (the core of `frankacli.py status`) so the snippet is self-contained:

```python
#!/usr/bin/env python3
import numpy as np, rospy
from franka_msgs.msg import FrankaState

rospy.init_node("franka_status", anonymous=True)
msg = rospy.wait_for_message("franka_state_controller/franka_states", FrankaState, timeout=5.0)

T = np.array(msg.O_T_EE).reshape(4, 4).T            # TCP pose
print("TCP position:", T[:3, -1])
print("joints:      ", list(msg.q))
print("mode:        ", msg.robot_mode, " success:", msg.control_command_success_rate)
```

#### R3b — ROS 2 (Humble)

Swap `ros` for `ros2`, use `.launch.py` files, and optionally pin `domain_id` per robot. With a single robot the plugin injects `ROS_DOMAIN_ID` so ROS 2 tools in the pod join the correct DDS domain.

> **Image hierarchy — `ros2-base` is a base image, not a Franka image.** `ros2-base` is the ROS 2 counterpart of `ros-base` (both are base images with the ROS + control stack, not robot-specific). It is **not** a drop-in replacement for the `serl_franka_controllers` image used in R3a — that one is a Franka ROS 1 workspace built *on top of* `ros-base`. To drive a Franka in ROS 2, build/use a Franka ROS 2 workspace image on top of `ros2-base` (the ROS 2 analogue of `serl_franka_controllers`) and set it as `config.ros2.pod.image`. The `moveit_servo` package referenced below must exist in that workspace image — bare `ros2-base` does not ship it.

```yaml
namespace: rlark-system

config:
  model: franka
  device_count: 1
  ros2:
    enabled: true
    httpPort: 8080
    pod:
      # Replace with your Franka ROS 2 workspace image built on ros2-base
      # (the ROS 2 analogue of serl_franka_controllers). It must contain
      # the packages your modes launch (e.g. moveit_servo).
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
        domain_id: 5            # explicit ROS_DOMAIN_ID (auto-assigned when 0)
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

The workload pod is identical to R3a (`rlinf.io/device-franka: 1`). `rosctr` targets the ROS 2 controller via `RLINF_EMBODIED_ROS2_SOCKET_PATH` automatically when the ROS 1 socket is absent. The same RPCs work:

```bash
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr list
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr start franka-robot-1 impedance
kubectl exec -it ros2-task -- /opt/rlinf/bin/rosctr env franka-robot-1   # shows ROS_DOMAIN_ID / RMW_IMPLEMENTATION
```

> ROS 2 DDS discovery relies on IP multicast. See [ROS 2 multicast](#ros-2-multicast) before running ROS 2 across multiple pods or nodes.

---

## Combined — robot + camera on one node

Different robot and camera types can be freely combined. The canonical embodied-AI data-collection node has a Franka arm **and** one or more cameras, and a training / teleop pod that drives the robot while streaming the wrist camera. Enable the ROS controller and the camera controller in the same release:

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

A single workload pod requests `rlinf.io/device-franka: 1` and gets **both** the ROS socket (`ros-ctrl.sock`) and the camera socket (`camera-ctrl.sock`) plus both CLIs and the single-robot `ROS_MASTER_URI`. Drive both in one process:

```python
# teleop.py — run inside a pod requesting rlinf.io/device-franka
from embodied_runtime import RobotClient, CameraClient

with RobotClient() as robot, CameraClient() as cam:
    robot.start_robot("franka-robot-1", mode="impedance")
    cam.open_camera("video0", encoding="jpeg")

    for frame in cam.watch_frames("video0"):
        # frame.data is a complete JPEG; feed it to your policy / logger
        record_step(robot.get_robot_status("franka-robot-1"), frame.data)
```

Mix and match: replace the ROS 1 controller with ROS 2, add an RTSP camera under `config.camera.cameras`, or add `hostDevices` for an extra USB device on the same node — all in the same release.

---

## Notes

### ROS isolation

- **ROS 1.** Each robot gets its own `roscore` on a unique port (from `11311`), so multiple robots in one controller pod never clash on topics or node names. The plugin injects the per-robot `ROS_MASTER_URI` only when there is exactly one robot; with several, the caller disambiguates via the controller. For stricter multi-tenant isolation across the cluster, deploy separate device-plugin instances (one per namespace) and back them with Kubernetes `NetworkPolicy` to restrict which pods can reach a controller's ROS master ports.

- **ROS 2.** DDS-level isolation comes from a per-robot `ROS_DOMAIN_ID` (auto-assigned or pinned via `domain_id`); robots on different domain IDs never discover each other. If you need stronger isolation, also partition the cluster network with namespaces and `NetworkPolicy`, and/or use unicast discovery profiles (see below).

### ROS 2 multicast

ROS 2 DDS uses IP multicast for node discovery by default. The cluster's network layer (CNI plugin, node / underlay switches) **must support multicast routing** for ROS 2 nodes in different pods / nodes to find each other. If the cluster blocks multicast (common with cloud-provider CNIs), pick one:

- **Unicast discovery** — set `CYCLONEDDS_URI` (or the Fast DDS equivalent) to a peer-list XML profile in each robot's mode-level `env`, so nodes discover peers explicitly instead of via multicast:

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

- **DDS Discovery Server** — run a Discovery Server reachable from all ROS 2 pods and point each robot's DDS config at it.

The controller's MACVLAN L2 interface gives direct access to the robot's physical subnet (where multicast typically works), but inter-pod / inter-node discovery still depends on the cluster network.

> **Tip.** If you are building the cluster from scratch, [`ros_k8s`](https://github.com/fujitatomoya/ros_k8s) covers ROS / ROS 2 on Kubernetes and KubeEdge, including the container and networking setup these multicast / discovery decisions depend on.
