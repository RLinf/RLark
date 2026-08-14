# embodied-runtime Python SDK

English | [简体中文](./README.zh-CN.md)

A Python client for the [embodied-runtime][repo] robot (ROS) and camera gRPC services. Both controllers are exposed over **Unix domain sockets** inside a task pod; this SDK mirrors the `rosctr` / `camctr` Go CLIs and handles the socket plumbing for you.

[repo]: https://github.com/RLinf/RLark/tree/main/apps/embodied-runtime

Generated gRPC stubs live under `embodied_runtime/gen/`; the wrapper clients live in `embodied_runtime.robot` and `embodied_runtime.camera`.

## Install

```bash
pip install -e .            # from sdk/python/
# or, once published:
pip install embodied-runtime
```

Runtime dependencies: `grpcio`, `protobuf`.

## Quick start

```python
from embodied_runtime import RobotClient, ModeConfig
from embodied_runtime.robot import state_name

# Default socket: /var/run/rlark/ros-ctrl.sock
with RobotClient() as robot:
    # preset mode + extra arg override
    robot.start_robot("franka-0", mode="impedance", args={"robot_ip": "172.16.0.2"})

    # ad-hoc custom mode
    robot.start_robot("franka-0", mode_config=ModeConfig(
        package="serl_franka_controllers",
        launch_file="impedance.launch",
        passthrough_robot_args=True,
    ))

    for info in robot.list_robots().robots:
        print(info.robot_id, info.mode, state_name(info.state))

    # ROS package introspection
    for pkg in robot.list_packages().packages:
        print(pkg)
```

```python
from embodied_runtime import CameraClient

with CameraClient() as cam:                       # /var/run/rlark/camera-ctrl.sock
    cam.open_camera("camera-0", encoding="h264")
    for frame in cam.watch_frames("camera-0"):
        # jpeg/png/bmp/tiff -> one complete, independently decodable frame per message
        # h264/h265          -> Annex B chunks; concatenate in sequence order
        print(frame.sequence, frame.encoding, len(frame.data), frame.keyframe)
```

## Clients

### `RobotClient` — `ros.controller.v1.RobotController`

| Method | RPC |
|--------|-----|
| `start_robot(robot_id, mode="", *, mode_config=None, args=None, env=None)` | `StartRobot` |
| `stop_robot(robot_id)` | `StopRobot` |
| `get_robot_status(robot_id)` | `GetRobotStatus` |
| `switch_mode(robot_id, mode="", *, mode_config=None, args=None, env=None)` | `SwitchMode` |
| `reset_robot(robot_id)` | `ResetRobot` |
| `list_robots()` | `ListRobots` |
| `list_modes(robot_id)` | `ListModes` |
| `get_robot_logs(robot_id, tail=0)` | `GetRobotLogs` |
| `list_packages()` | `ListPackages` |
| `get_package_info(name)` | `GetPackageInfo` |
| `get_package_launch_files(name)` | `GetPackageLaunchFiles` |
| `get_launch_file_args(package, launch_file)` | `GetLaunchFileArgs` |

**Mode selection** mirrors `rosctr`:

- Preset: pass a `mode` name; optional `args` / `env` are merged into the preset.
- Custom: pass a `ModeConfig(package=..., launch_file=...)` with no `mode`.
- The two are mutually exclusive (a `ValueError` is raised otherwise).

### `CameraClient` — `camera.controller.v1.CameraController`

| Method | RPC |
|--------|-----|
| `list_cameras()` | `ListCameras` |
| `get_camera_info(camera_id)` | `GetCameraInfo` |
| `open_camera(camera_id, *, width=None, height=None, fps=None, encoding=None)` | `OpenCamera` |
| `close_camera(camera_id)` | `CloseCamera` |
| `capture_frame(camera_id, *, wait=None)` | `CaptureFrame` |
| `capture_frames(camera_ids, *, wait=None)` → `CaptureFramesResponse` | `CaptureFrames` |
| `watch_frames(camera_id)` → iterator of `VideoFrame` | `WatchFrames` (stream) |

`watch_frames` returns an iterator — simply stop iterating (or exit the `with` block) to cancel the stream.

`capture_frames` fetches the latest frame from multiple cameras in a single RPC (each camera is read concurrently server-side, so the latency is bounded by the slowest camera). It's intended for use cases such as pairing an RGB frame with its depth map where splitting the capture across multiple requests would hurt real-time performance. Per-camera failures are reported on each `CapturedFrame` (`error_code` / `error`) rather than raising — inspect them to detect partial failures:

```python
from embodied_runtime import CameraClient

with CameraClient() as cam:
    cam.open_camera("wrist-rgb", encoding="jpeg")
    cam.open_camera("wrist-depth", encoding="jpeg")

    resp = cam.capture_frames(["wrist-rgb", "wrist-depth"])
    for f in resp.frames:
        if f.error_code:                      # 0 == OK
            print(f.camera_id, "failed:", f.error)
        else:
            print(f.camera_id, f.encoding, len(f.data), f.timestamp_ns)
```

## Connecting

Both clients default to the node-local Unix sockets. To reach a remote TCP server, pass `address="host:port"`:

```python
RobotClient(address="10.0.0.5:50051")
```

## Environment variables

The device plugin injects `RLINF_EMBODIED_ROS_SOCKET_PATH` (ROS 1), `RLINF_EMBODIED_ROS2_SOCKET_PATH` (ROS 2), and `RLINF_EMBODIED_CAMERA_SOCKET_PATH` into task pods; both clients read them automatically (an explicit `socket_path` / `address` argument always wins):

| Env var | Used by | Default |
|---------|---------|---------|
| `RLINF_EMBODIED_ROS_SOCKET_PATH` | `RobotClient` | `/var/run/rlark/ros-ctrl.sock` |
| `RLINF_EMBODIED_ROS2_SOCKET_PATH` | `RobotClient` | `/var/run/rlark/ros2-ctrl.sock` |
| `RLINF_EMBODIED_CAMERA_SOCKET_PATH` | `CameraClient` | `/var/run/rlark/camera-ctrl.sock` |

## Regenerate stubs

Stubs are generated from [proto/embodied-runtime](../../proto/embodied-runtime):

```bash
# From repo root — generates all SDK stubs (Go + Python)
make proto

# Or from the proto directory — generate only Python stubs
make -C proto/embodied-runtime proto-python
# or directly:
python3 sdks/embodied-runtime-python/scripts/gen_proto.py
```

The generated `*_pb2.py` / `*_pb2_grpc.py` files are checked in; regenerate only when the `.proto` definitions change.

## Lint & format

[Ruff](https://docs.astral.sh/ruff/) is the linter and formatter (configured in `pyproject.toml`; the generated `embodied_runtime/gen/` tree is excluded):

```bash
make lint-py          # ruff check — lint the SDK
make fmt-py           # ruff format — format the SDK
# or directly, from sdk/python/:
ruff check embodied_runtime scripts examples
ruff format embodied_runtime scripts examples
```

Install dev deps: `pip install -r requirements-dev.txt`.
