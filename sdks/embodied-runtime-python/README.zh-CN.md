# embodied-runtime Python SDK

[English](./README.md) | 简体中文

[embodied-runtime][repo] 机器人（ROS）与摄像头 gRPC 服务的 Python 客户端。两个控制器在 task pod 内通过 **Unix domain socket** 暴露；本 SDK 镜像 `rosctr` / `camctr` Go CLI，并代为处理 socket 连接。

[repo]: https://github.com/RLinf/RLark/tree/main/apps/embodied-runtime

生成的 gRPC stub 位于 `embodied_runtime/gen/`；封装客户端位于 `embodied_runtime.robot` 与 `embodied_runtime.camera`。

## 安装

```bash
pip install -e .            # 在 sdk/python/ 下
# 发布后可用：
pip install embodied-runtime
```

运行依赖：`grpcio`、`protobuf`。

## 快速上手

```python
from embodied_runtime import RobotClient, ModeConfig
from embodied_runtime.robot import state_name

# 默认 socket: /var/run/rlark/ros-ctrl.sock
with RobotClient() as robot:
    # 预置模式 + 额外参数覆盖
    robot.start_robot("franka-0", mode="impedance", args={"robot_ip": "172.16.0.2"})

    # 临时自定义模式
    robot.start_robot("franka-0", mode_config=ModeConfig(
        package="serl_franka_controllers",
        launch_file="impedance.launch",
        passthrough_robot_args=True,
    ))

    for info in robot.list_robots().robots:
        print(info.robot_id, info.mode, state_name(info.state))

    # ROS 包查询
    for pkg in robot.list_packages().packages:
        print(pkg)
```

```python
from embodied_runtime import CameraClient

with CameraClient() as cam:                       # /var/run/rlark/camera-ctrl.sock
    cam.open_camera("camera-0", encoding="h264")
    for frame in cam.watch_frames("camera-0"):
        # jpeg/png/bmp/tiff -> 每条消息一帧完整、可独立解码的静图
        # h264/h265          -> Annex B 分片，按 sequence 顺序拼接
        print(frame.sequence, frame.encoding, len(frame.data), frame.keyframe)
```

## 客户端

### `RobotClient` — `ros.controller.v1.RobotController`

| 方法 | RPC |
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

**模式选择** 与 `rosctr` 一致：

- 预置模式：传 `mode` 名称；可选 `args` / `env` 会合并进预置。
- 自定义模式：传 `ModeConfig(package=..., launch_file=...)`，不传 `mode`。
- 二者互斥（否则抛出 `ValueError`）。

### `CameraClient` — `camera.controller.v1.CameraController`

| 方法 | RPC |
|--------|-----|
| `list_cameras()` | `ListCameras` |
| `get_camera_info(camera_id)` | `GetCameraInfo` |
| `open_camera(camera_id, *, width=None, height=None, fps=None, encoding=None)` | `OpenCamera` |
| `close_camera(camera_id)` | `CloseCamera` |
| `capture_frame(camera_id, *, wait=None)` | `CaptureFrame` |
| `capture_frames(camera_ids, *, wait=None)` → `CaptureFramesResponse` | `CaptureFrames` |
| `watch_frames(camera_id)` → `VideoFrame` 迭代器 | `WatchFrames`（流） |

`watch_frames` 返回迭代器 —— 停止迭代（或退出 `with` 块）即可取消流。

`capture_frames` 在单次 RPC 内抓取多个摄像头的最新帧（服务端并发读取各摄像头，延迟取决于最慢的摄像头）。适用于 RGB + 深度图配对等对实时性要求高、不宜拆分多次请求的场景。每摄像头的失败以 `CapturedFrame` 的 `error_code` / `error` 字段返回而非抛异常 —— 需自行检查以发现部分失败：

```python
from embodied_runtime import CameraClient

with CameraClient() as cam:
    cam.open_camera("wrist-rgb", encoding="jpeg")
    cam.open_camera("wrist-depth", encoding="jpeg")

    resp = cam.capture_frames(["wrist-rgb", "wrist-depth"])
    for f in resp.frames:
        if f.error_code:                      # 0 == OK
            print(f.camera_id, "失败:", f.error)
        else:
            print(f.camera_id, f.encoding, len(f.data), f.timestamp_ns)
```

## 连接

两个客户端默认连接节点本地 Unix socket。如需连接远端 TCP 服务，传 `address="host:port"`：

```python
RobotClient(address="10.0.0.5:50051")
```

## 环境变量

device plugin 会向 task pod 注入 `RLINF_EMBODIED_ROS_SOCKET_PATH`（ROS 1）、`RLINF_EMBODIED_ROS2_SOCKET_PATH`（ROS 2）与 `RLINF_EMBODIED_CAMERA_SOCKET_PATH`；两个客户端会自动读取（显式传入的 `socket_path` / `address` 参数始终优先）：

| 环境变量 | 使用者 | 默认值 |
|---------|---------|---------|
| `RLINF_EMBODIED_ROS_SOCKET_PATH` | `RobotClient` | `/var/run/rlark/ros-ctrl.sock` |
| `RLINF_EMBODIED_ROS2_SOCKET_PATH` | `RobotClient` | `/var/run/rlark/ros2-ctrl.sock` |
| `RLINF_EMBODIED_CAMERA_SOCKET_PATH` | `CameraClient` | `/var/run/rlark/camera-ctrl.sock` |

## 重新生成 stub

Stub 从 [proto/embodied-runtime](../../proto/embodied-runtime) 生成：

```bash
# 在仓库根目录执行 — 生成所有 SDK stub（Go + Python）
make proto

# 或在 proto 目录下执行 — 仅生成 Python stub
make -C proto/embodied-runtime proto-python
# 或直接运行：
python3 sdks/embodied-runtime-python/scripts/gen_proto.py
```

生成的 `*_pb2.py` / `*_pb2_grpc.py` 已入库；仅在 `.proto` 定义变更时重新生成。

## 检查与格式化

[Ruff](https://docs.astral.sh/ruff/) 是 lint 与 format 工具（配置在 `pyproject.toml`；生成的 `embodied_runtime/gen/` 目录已排除）：

```bash
make lint-py          # ruff check —— 检查 SDK
make fmt-py           # ruff format —— 格式化 SDK
# 或在 sdk/python/ 下直接运行：
ruff check embodied_runtime scripts examples
ruff format embodied_runtime scripts examples
```

安装开发依赖：`pip install -r requirements-dev.txt`。
