# Embodied Runtime

A Kubernetes-native runtime for managing robot (ROS) and camera hardware on edge nodes. It exposes robots and cameras as **schedulable Kubernetes resources** via the Device Plugin API. For the onboarding workflow, see [Embodied Device Onboarding](../admin-guide/embodied-runtime.md).

![Embodied Runtime Architecture](../images/embodied-runtime-architecture.svg)

## Manager Modes

Each controller (`camera`, `ros`, `ros2`) can be independently configured with one of three modes:

| Mode | Behavior |
|------|----------|
| `disabled` | No controller launched, no config generated (default). |
| `local` | Controller runs as a subprocess of the device plugin. |
| `pod` | Controller runs as a dedicated Pod, created and owned by the device plugin. |

In `pod` mode, the device plugin auto-discovers its own Pod (owner refs, tolerations, image, node) and reuses them to fill in blank controller-pod fields, so controller pods are garbage-collected with the device plugin and scheduled onto the same tainted node.

## ROS 1 Networking

Each robot gets its own `roscore` on a unique port starting at `11311`, so multiple robots coexist in one container without topic/name conflicts. `ROS_MASTER_URI` and `ROS_IP` are injected into each `roslaunch` process. MACVLAN interfaces are created at startup for robot-network access, and `roslaunch` runs directly in the container (no `nsenter`).

## ROS 2 Networking

Unlike ROS 1, there is no central master. Each robot gets its own `ROS_DOMAIN_ID` (auto-assigned starting at 0, or explicitly set via `domain_id` in the robot config) for DDS-level isolation, so multiple robots in the same container do not discover each other's topics/services. `ROS_DOMAIN_ID` (and optionally `RMW_IMPLEMENTATION`, `CYCLONEDDS_URI`) is injected into each `ros2 launch` process. The base image defaults to Cyclone DDS (`rmw_cyclonedds_cpp`) for better multi-robot and cross-subnet discovery behavior.

!!! warning "Multicast requirement"
    ROS 2 DDS uses IP multicast for node discovery by default. The cluster's network layer (CNI plugin, node/underlay switches) **must support multicast routing** for ROS 2 nodes across different pods/nodes to find each other. If the cluster does not support multicast (e.g. many cloud-provider CNIs block it), configure unicast discovery instead — set `CYCLONEDDS_URI` (or the Fast DDS equivalent) to a peer-list XML profile in each robot's mode-level `env`, or deploy a DDS Discovery Server.

## Full Configuration Reference

### device-plugin

Loaded from `--config <path>`; all fields optional.

```yaml
# model: franka          # → resource rlinf.io/device-franka,
                          #   socket rlinf-device-franka.sock
device_count: 1
skip_register: false

host_devices:
  - host_path: /dev/video0
    # container_path: /dev/video0  # defaults to host_path
    # permissions: rwm             # r|w|m; defaults to rwm
  - host_path: /dev/ttyUSB0
    permissions: rw

host_macvlans:
  - host_nic: eno1
    name: macvlan0
    ip: 172.16.0.0/24      # network addr → auto-pick an unused host IP
    # gateway: 172.16.0.1  # optional default gateway for the robot subnet

camera:
  manager_mode: local     # disabled | local | pod
  ctrl_config_path: /etc/rlinf/camera-controller.yaml
  ctrl_bin: /usr/local/bin/camera-controller
  ctr_cli: /opt/rlinf/bin/camctr
  auto_detect_v4l2: true  # auto-detect /dev/video* cameras
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
  ctr_cli: /opt/rlinf/bin/rosctr    # shared with ROS 1
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
      domain_id: 5            # explicit ROS_DOMAIN_ID override (auto-assigned if 0)
  allowed_launch_packages:
    - moveit_servo
```

## Controller Pod Specs

### ROS 1 Controller

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

The typical pattern: an `initContainer` copies `ros-controller` / `rosctr` out of the `embodied-runtime` image into a shared `emptyDir`; the main container runs the controller binary from there. Liveness / readiness probes use `rosctr list` against the Unix socket.

### ROS 2 Controller

Same as ROS 1, but uses a different socket:

```yaml
args: ["--socket=/var/run/rlark/ros2-ctrl.sock"]
```

### Camera Controller

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

### Camera Controller Details

Cameras are configured statically or auto-detected from `/sys/class/video4linux`. Capture is done by spawning `ffmpeg` subprocesses; the driver captures in whatever format the device provides and transcodes to the requested output — frame-mode still-image encodings (`jpeg` / `png` / `bmp` / `tiff`, one complete decodable frame per message) or bitstream encodings (`h264` / `h265`, Annex B elementary-stream chunks).

## CLI Tools

### rosctr — Robot Control (ROS 1 and ROS 2)

The same `rosctr` binary works with both the ROS 1 controller (`ros-ctrl.sock`) and the ROS 2 controller (`ros2-ctrl.sock`). Point it at the appropriate socket via `--socket-path` or the `RLINF_EMBODIED_ROS_SOCKET_PATH` / `RLINF_EMBODIED_ROS2_SOCKET_PATH` env vars.

```bash
rosctr list                                      # robots + status
rosctr status <robot-id>
rosctr modes <robot-id>                          # available control modes
rosctr start <robot-id> [mode]                   # preset mode
rosctr start <robot-id> --package pkg --launch-file f.launch   # custom mode
rosctr switch <robot-id> <mode>                  # switch running robot's mode
rosctr stop <robot-id>
rosctr reset <robot-id>                          # restart roscore + state
rosctr logs <robot-id> [--tail N]
rosctr packages                                  # whitelisted ROS packages
rosctr pkg info <package>
rosctr pkg launch-files <package>
rosctr pkg launch-args <package> <launch-file>
rosctr env <robot-id>                            # injected ROS env vars
```

Output format: `-o text|json|yaml`.

### camctr — Camera Control

```bash
camctr list                                      # cameras + state
camctr info <camera-id>
camctr open <camera-id> [--width W --height H --fps F --encoding jpeg|png|bmp|tiff|h264|h265]
camctr close <camera-id>
camctr frame <camera-id>                         # single capture
camctr watch <camera-id>                         # streaming frames
camctr watch <camera-id> --save-dir /tmp/frames  # save chunks to files
camctr watch <camera-id> > stream.h264           # raw bitstream to stdout
camctr watch <camera-id> | ffplay -i -           # pipe to ffplay
```

`watch` delivers frames in the encoding the camera was opened with: `jpeg` / `png` / `bmp` / `tiff` = one complete, independently decodable still-image frame per message; `h264` / `h265` = Annex B elementary-stream chunks that the client concatenates in order.

## gRPC API

The controllers expose gRPC services over **Unix sockets** (`/var/run/rlark/*.sock`):

| Service | RPCs | Description |
|---------|------|-------------|
| `RobotController` | StartRobot, StopRobot, GetRobotStatus, SwitchMode, ResetRobot, ListRobots, ListModes, GetRobotLogs | Robot lifecycle management |
| `CameraController` | ListCameras, OpenCamera, CloseCamera, CaptureFrame, CaptureFrames, WatchFrames | Camera management and frame capture |
| `DeviceService` | SetupDevices | On-demand macvlan setup (served by device plugin itself) |

The `RobotController` service is implemented by two independent controllers on separate sockets: `ros-controller` on `ros-ctrl.sock` (ROS 1) and `ros2-controller` on `ros2-ctrl.sock` (ROS 2). Both honour the same RPC contract and proto, so a single `rosctr` CLI and SDK client can target either by pointing at the right socket.

Full API reference: `proto/embodied-runtime/`.

## Mutating Webhook (Auto devinit Injection)

When `host_macvlans` is configured, the device plugin can optionally run a mutating webhook that automatically injects the `devinit` init container into workload pods.

**How it works:**

1. When `--webhook` is set and `host_macvlans` is configured, the device plugin starts an HTTPS webhook that intercepts Pod CREATE/UPDATE.
2. For any pod requesting `rlinf.io/device[-<model>]`, it appends an init container named `rlark-devinit` that runs `devinit setup`.
3. The injected container only declares the same device resource — the device plugin's `Allocate` then injects the RunDir socket mount and `RLINF_EMBODIED_DEVINIT_SOCKET_PATH` env var.

The webhook has **automatic CA management**: it loads the CA cert+key from a Secret (or generates a self-signed CA in memory), reads the `MutatingWebhookConfiguration`, and auto-patches the `caBundle` when empty.

**Device plugin CLI flags for webhook:**

| Flag | Purpose |
|------|---------|
| `--webhook` | Enable the webhook (requires `host_macvlans`). |
| `--webhook-addr` | HTTPS listen address (default `:9443`). |
| `--webhook-path` | Admission endpoint path (default `/mutate`). |
| `--webhook-mutating-config` | `MutatingWebhookConfiguration` name for auto CA management. |
| `--webhook-service-name` / `--webhook-service-namespace` | Service fronting the webhook. |
| `--webhook-ca-secret-name` / `--webhook-ca-secret-namespace` | Secret persisting the CA (empty = in-memory). |
| `--webhook-devinit-image` | Injected init container image (default: auto-discovered). |

Enable via Helm:

```yaml
webhook:
  enabled: true
  port: 9443
  failurePolicy: Ignore   # Ignore = admit pods even if webhook is down
  caSecret:
    name: devinit-ca
    namespace: rlark-system
```

The webhook only renders when **both** `webhook.enabled` and `config.hostMacvlans` are set.

## Build

Requires Go 1.26+.

```bash
make build        # just the binaries → bin/
make test         # unit tests
make vet          # go vet
```

Binaries are built with `CGO_ENABLED=0` (statically linked).

### Docker Images

```bash
# embodied-runtime — all 7 binaries (static, multi-arch)
make docker REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# camera-base — Alpine + ffmpeg (no binaries; injected at deploy via initContainer)
make docker-camera REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# ros-base — ROS Noetic + control packages (for ROS 1 controllers)
make docker-ros REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0

# ros2-base — ROS 2 Humble + control packages (for ROS 2 controllers)
make docker-ros2 REGISTRY=ghcr.io/your-org IMAGE_TAG=v0.1.0
```

Overridable variables: `REGISTRY`, `IMAGE_TAG` / `VERSION`, `BUILDX_PLATFORMS`, `GO_BASE_IMAGE`, `RUNTIME_BASE_IMAGE`, `APK_MIRROR`.

## Components

| Binary | Package | Role |
|--------|---------|------|
| `device-plugin` | `cmd/device-plugin` | Kubelet device plugin; supervises all controllers. |
| `ros-controller` | `cmd/ros-controller` | ROS 1 robot lifecycle: roscore, roslaunch, modes, web proxy. |
| `ros2-controller` | `cmd/ros2-controller` | ROS 2 robot lifecycle: ros2 launch, DOMAIN_ID, modes, proxy. |
| `camera-controller` | `cmd/camera-controller` | Camera lifecycle: open/close, capture, stream, transcode. |
| `rosctr` | `cmd/rosctr` | CLI for RobotController gRPC API (ROS 1 **and** ROS 2). |
| `camctr` | `cmd/camctr` | CLI for camera-controller gRPC API. |
| `devinit` | `cmd/devinit` | Init-container CLI: requests on-demand macvlan setup. |

## Project Structure

```
cmd/                       # one package per binary
  device-plugin/           # kubelet device plugin
  ros-controller/          # ROS 1 robot lifecycle
  ros2-controller/         # ROS 2 robot lifecycle
  camera-controller/       # camera lifecycle
  rosctr/                  # CLI (cobra) — shared ROS 1 + ROS 2
  camctr/                  # CLI (cobra)
  devinit/                 # init-container CLI
pkg/
  deviceplugin/            # kubelet plugin, config, hardware detect, pod/local managers
  roscontroller/           # roscore, roslaunch, modes, MACVLAN, web proxy, gRPC server
  ros2controller/          # ros2 launch, DOMAIN_ID, modes, gRPC server
  cameracontroller/        # ffmpeg/remote drivers, transcoder, gRPC server
  netmac/                  # shared MACVLAN interface management
examples/                  # sample ConfigMaps + Pod manifests
runtimes/                  # camera-base, ros-base, ros2-base dockerfiles
Dockerfile                 # multi-stage build → all binaries
Makefile                   # build / proto / docker / lint / test
```