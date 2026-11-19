# embodied-runtime gRPC API

[English](./proto-api.md) | 简体中文

本文档描述 embodied-runtime 暴露的两个 gRPC 服务：

- **`RobotController`** —— 机器人（ROS）生命周期与控制模式管理。
- **`CameraController`** —— 摄像头采集、单帧抓取与实时推流。

二者均以 Protocol Buffers v3 定义于 [`proto/embodied-runtime/`](../../../proto/embodied-runtime)，并通过 Unix domain socket 提供服务。本文为功能参考；字段编号与线缆类型以 `.proto` 源文件为准。

## 概览

| 服务 | 包 | Proto | 默认 socket |
|---------|---------|-------|----------------|
| `RobotController` | `ros.controller.v1` | `proto/roscontroller/v1/robot.proto` | `/var/run/rlark/ros-ctrl.sock` |
| `CameraController` | `camera.controller.v1` | `proto/cameracontroller/v1/camera.proto` | `/var/run/rlark/camera-ctrl.sock` |

### 传输

- 两个服务均监听节点本地 `/var/run/rlark` 目录下的 Unix domain socket；device plugin 会将该目录以只读方式挂载进 task pod。
- device plugin 会注入 `RLINF_EMBODIED_ROS_SOCKET_PATH`（ROS 1）、`RLINF_EMBODIED_ROS2_SOCKET_PATH`（ROS 2）与 `RLINF_EMBODIED_CAMERA_SOCKET_PATH`；`rosctr` / `camctr` CLI 会自动读取（显式传入的 `--socket-path` 参数始终优先）。对 `rosctr` 而言，ROS 1 的 socket 路径优先；未设置时回退至 ROS 2 的 socket 路径。

### HTTP 网关

每个控制器可选地通过 HTTP/JSON 镜像其 gRPC API，便于非 gRPC 客户端（curl、浏览器、`fetch`）调用。通过 `--http-addr` 标志（`camera-controller --http-addr :8080`、`ros-controller --http-addr :8080`）或配置文件中的 `http_addr` 字段开启；在 device-plugin 配置里设置 `camera.http_addr` / `ros.http_addr`（写入控制器配置文件，**local 与 pod 模式均生效**，并非 pod 专属）。留空则禁用 HTTP，仅暴露 gRPC Unix socket。

响应使用标准 proto JSON（lowerCamelCase、`EmitUnpopulated`），与 `camctr -o json` / `rosctr -o json` 输出一致。gRPC status 错误映射到最接近的 HTTP 状态码（`NotFound`→404、`InvalidArgument`→400、`PermissionDenied`→403、`FailedPrecondition`→412、`Unimplemented`→501……）。

**CameraController**（`/v1/`）：

| 方法 | 路径 | RPC |
|------|------|-----|
| GET  | `/v1/cameras` | `ListCameras` |
| GET  | `/v1/cameras/{camera_id}` | `GetCameraInfo` |
| POST | `/v1/cameras/{camera_id}/open` | `OpenCamera` |
| POST | `/v1/cameras/{camera_id}/close` | `CloseCamera` |
| GET  | `/v1/cameras/{camera_id}/frame` | `CaptureFrame`（默认返回原始字节 + 元数据响应头；`Accept: application/json` 返回 JSON） |
| POST | `/v1/cameras:captureFrames` | `CaptureFrames` |
| GET  | `/v1/cameras/{camera_id}/watch` | `WatchFrames`（Server-Sent Events，每事件一条 `VideoFrame`） |

**RobotController**（`/v1/`）：

| 方法 | 路径 | RPC |
|------|------|-----|
| GET  | `/v1/robots` | `ListRobots` |
| GET  | `/v1/robots/{robot_id}` | `GetRobotStatus` |
| POST | `/v1/robots/{robot_id}/start` | `StartRobot` |
| POST | `/v1/robots/{robot_id}/stop` | `StopRobot` |
| POST | `/v1/robots/{robot_id}/mode` | `SwitchMode` |
| POST | `/v1/robots/{robot_id}/reset` | `ResetRobot` |
| GET  | `/v1/robots/{robot_id}/modes` | `ListModes` |
| GET  | `/v1/robots/{robot_id}/logs?tail=N` | `GetRobotLogs` |
| GET  | `/v1/packages` | `ListPackages` |
| GET  | `/v1/packages/{name}` | `GetPackageInfo` |
| GET  | `/v1/packages/{name}/launch-files` | `GetPackageLaunchFiles` |
| GET  | `/v1/packages/{name}/launch-files/{launch_file}/args` | `GetLaunchFileArgs` |
| ANY  | `/v1/robots/{robot_id}/proxy/*` | 反向代理 → 机器人的 `web_service` URL |

`StartRobot` / `SwitchMode` 的请求体是去掉 `robot_id` 的 proto 消息（`robot_id` 取自路径）；路径中的 ID 始终优先于 JSON body 中的同名字段。每机器人的 web 代理（`/v1/robots/{robot_id}/proxy/*`）反向代理到该机器人配置的 `web_service`；响应中的 `Location` 头会改写回同一 `/v1/robots/{robot_id}/proxy` 前缀下，使重定向留在网关内。

### 客户端

- **Go** —— `rosctr` / `camctr` CLI（`cmd/rosctr`、`cmd/camctr`），或直接导入 stub。

---

## RobotController

`ros.controller.v1.RobotController` 管理主机上的机器人节点：生命周期（启动 / 停止 / 重置）、控制模式切换、状态、日志，以及 ROS 包查询。device plugin 通过它驱动运行在主机网络上的机器人节点（如 Franka）。

该服务由两个独立控制器实现，注册在不同 Unix socket 上，遵循同一套 RPC 契约：

- **ROS 1**（`ros-controller`，`ros-ctrl.sock`）：启动每机器人 `roscore`；响应中填充 `ros_master_uri`。
- **ROS 2**（`ros2-controller`，`ros2-ctrl.sock`）：无 master；为每机器人分配 `ROS_DOMAIN_ID` 实现 DDS 隔离；响应中填充 `ros_domain_id`。

> **组播要求（仅 ROS 2）。** ROS 2 DDS 默认使用 IP 组播进行节点发现。集群的网络层（CNI 插件、节点 / 底层交换机）**必须支持组播路由**，跨 Pod / 跨节点的 ROS 2 节点才能互相发现。若组播被屏蔽，需在各 robot 的 mode 级 `env` 中设置 `CYCLONEDDS_URI` 改为单播发现，或部署 DDS Discovery Server。

### RPC

| RPC | 请求 | 响应 | 说明 |
|-----|---------|----------|-------------|
| `StartRobot` | `StartRobotRequest` | `StartRobotResponse` | 以指定控制模式启动机器人节点。 |
| `StopRobot` | `StopRobotRequest` | `StopRobotResponse` | 停止运行中的机器人节点。 |
| `GetRobotStatus` | `GetRobotStatusRequest` | `GetRobotStatusResponse` | 机器人节点当前状态。 |
| `SwitchMode` | `SwitchModeRequest` | `SwitchModeResponse` | 切换运行中机器人的控制模式。 |
| `ListRobots` | `ListRobotsRequest` | `ListRobotsResponse` | 所有受管机器人及状态。 |
| `ListModes` | `ListModesRequest` | `ListModesResponse` | 机器人支持的控制模式。 |
| `GetRobotLogs` | `GetRobotLogsRequest` | `GetRobotLogsResponse` | 最近的 launch 进程日志行。 |
| `ListPackages` | `ListPackagesRequest` | `ListPackagesResponse` | 白名单内的 ROS 包。 |
| `GetPackageInfo` | `GetPackageInfoRequest` | `GetPackageInfoResponse` | 某个 ROS 包的元信息。 |
| `GetPackageLaunchFiles` | `GetPackageLaunchFilesRequest` | `GetPackageLaunchFilesResponse` | 包内的 launch 文件。 |
| `GetLaunchFileArgs` | `GetLaunchFileArgsRequest` | `GetLaunchFileArgsResponse` | launch 文件支持的参数。 |
| `ResetRobot` | `ResetRobotRequest` | `ResetRobotResponse` | 停止、重启 ROS 中间件（ROS 1 重启 roscore；ROS 2 重启 launch 进程）、状态重置为 STOPPED。 |

### 枚举

#### `RobotState`

机器人节点的生命周期状态。

| 值 | # | 含义 |
|-------|---|---------|
| `ROBOT_STATE_UNSPECIFIED` | 0 | 未知 / 未设置。 |
| `ROBOT_STATE_STARTING` | 1 | 启动中。 |
| `ROBOT_STATE_RUNNING` | 2 | 运行中。 |
| `ROBOT_STATE_STOPPING` | 3 | 停止中。 |
| `ROBOT_STATE_STOPPED` | 4 | 已停止。 |
| `ROBOT_STATE_ERROR` | 5 | 错误。 |

### 消息 —— 生命周期

#### `StartRobotRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人唯一 id（如 `franka-0`）。 |
| `mode` | `string` | 初始控制模式（如 `impedance`、`joint`）。设置 `mode_config` 时仅作标签。 |
| `mode_config` | `ModeConfig` | 可选的自定义模式配置；设置后绕过按名称查找预置模式。 |

#### `StartRobotResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `state` | `RobotState` | 结果状态。 |
| `current_mode` | `ModeInfo` | 解析后的模式信息。 |
| `ros_master_uri` | `string` | 机器人监听的 ROS master URI（仅 ROS 1）。 |
| `params` | `map<string,string>` | 机器人级参数，可经 `arg_from` 引用。 |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID`，用于 DDS 隔离（仅 ROS 2；ROS 1 为 0）。 |

#### `StopRobotRequest` / `StopRobotResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id（请求与响应均含）。 |

#### `GetRobotStatusRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |

#### `GetRobotStatusResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `mode` | `string` | 当前模式名。 |
| `state` | `RobotState` | 生命周期状态。 |
| `current_mode` | `ModeInfo` | 解析后的模式信息。 |
| `ros_master_uri` | `string` | ROS master URI（仅 ROS 1）。 |
| `params` | `map<string,string>` | 机器人级参数。 |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID`（仅 ROS 2；ROS 1 为 0）。 |

#### `SwitchModeRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `mode` | `string` | 目标模式。设置 `mode_config` 时仅作标签。 |
| `mode_config` | `ModeConfig` | 可选的自定义模式配置。 |

#### `SwitchModeResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `mode` | `string` | 当前模式。 |
| `state` | `RobotState` | 生命周期状态。 |
| `current_mode` | `ModeInfo` | 解析后的模式信息。 |
| `ros_master_uri` | `string` | ROS master URI（仅 ROS 1）。 |
| `params` | `map<string,string>` | 机器人级参数。 |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID`（仅 ROS 2；ROS 1 为 0）。 |

#### `ResetRobotRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |

#### `ResetRobotResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `state` | `RobotState` | 重置后状态（STOPPED）。 |
| `ros_master_uri` | `string` | ROS master URI（ROS 1；roscore 重启后可能变化）。 |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID`（ROS 2；重置后保持不变）。 |

### 消息 —— 清单与模式

#### `ListRobotsRequest`

空。

#### `ListRobotsResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robots` | `repeated RobotInfo` | 所有受管机器人。 |

#### `RobotInfo`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `mode` | `string` | 当前模式名。 |
| `state` | `RobotState` | 生命周期状态。 |
| `current_mode` | `ModeInfo` | 解析后的模式信息。 |
| `ros_master_uri` | `string` | ROS master URI（仅 ROS 1）。 |
| `params` | `map<string,string>` | 机器人级参数。 |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID`（仅 ROS 2；ROS 1 为 0）。 |

#### `ListModesRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |

#### `ListModesResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `modes` | `repeated ModeInfo` | 可用控制模式及完整配置。 |

#### `ModeInfo`

预置控制模式的完整配置。

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | 模式名（如 `impedance`、`joint`）。 |
| `package` | `string` | 含 launch 文件的 ROS 包。 |
| `launch_file` | `string` | launch 文件名（如 `impedance.launch`）。 |
| `args` | `map<string,string>` | 传给 roslaunch 的默认 key=value 参数。 |
| `env` | `map<string,string>` | roslaunch 进程的额外环境变量。 |
| `arg_from` | `map<string,string>` | roslaunch 参数名 → 机器人参数名的映射。 |
| `passthrough_robot_args` | `bool` | 以恒等映射合并所有机器人参数。 |

#### `ModeConfig`

在 `StartRobot` / `SwitchMode` 中内联传入的临时自定义模式，绕过预置模式。镜像服务端 ModeConfig 结构。

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `package` | `string` | 含 launch 文件的 ROS 包。 |
| `launch_file` | `string` | launch 文件名。 |
| `args` | `map<string,string>` | 传给 roslaunch 的 key=value 参数。 |
| `passthrough_robot_args` | `bool` | 以恒等映射合并所有机器人参数。与 `arg_from` 互斥。 |
| `arg_from` | `map<string,string>` | 参数名 → 参数名映射。与 `passthrough_robot_args` 互斥。 |
| `env` | `map<string,string>` | roslaunch 进程的额外环境变量。 |

### 消息 —— 日志

#### `GetRobotLogsRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `tail` | `int32` | 返回最近行数；0 = 全部缓冲行。 |

#### `GetRobotLogsResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `robot_id` | `string` | 机器人 id。 |
| `lines` | `repeated string` | 最近日志行（从未启动或已清空则为空）。 |

### 消息 —— ROS 包查询

#### `ListPackagesRequest`

空。

#### `ListPackagesResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `packages` | `repeated string` | 白名单内的 ROS 包名。 |

#### `GetPackageInfoRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | ROS 包名（如 `franka_ros`）。 |

#### `PackageInfo`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | 包名。 |
| `version` | `string` | package.xml 中的版本。 |
| `description` | `string` | package.xml 中的描述。 |
| `maintainer` | `string` | 维护者姓名 + 邮箱。 |
| `allowed` | `bool` | 是否在 launch 包白名单内。 |

#### `GetPackageInfoResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `info` | `PackageInfo` | 包元信息。 |

#### `GetPackageLaunchFilesRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | ROS 包名。 |

#### `GetPackageLaunchFilesResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | 包名。 |
| `launch_files` | `repeated string` | launch 文件名。 |

#### `GetLaunchFileArgsRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `package` | `string` | ROS 包名。 |
| `launch_file` | `string` | launch 文件名。 |

#### `LaunchArg`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `name` | `string` | 参数名。 |
| `required` | `bool` | 是否必填（无默认值）。 |
| `default` | `string` | 默认值（如有）。 |
| `description` | `string` | 来自 arg 标签或注释的描述。 |

#### `GetLaunchFileArgsResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `package` | `string` | 包名。 |
| `launch_file` | `string` | launch 文件名。 |
| `args` | `repeated LaunchArg` | 支持的参数。 |

---

## CameraController

`camera.controller.v1.CameraController` 管理摄像头设备：列表、打开 / 关闭、单帧抓取、多摄像头抓取与实时推流。调用 `CaptureFrame` / `CaptureFrames` / `WatchFrames` 前须先 open。

### RPC

| RPC | 请求 | 响应 | 说明 |
|-----|---------|----------|-------------|
| `ListCameras` | `ListCamerasRequest` | `ListCamerasResponse` | 所有受管摄像头及状态。 |
| `GetCameraInfo` | `GetCameraInfoRequest` | `GetCameraInfoResponse` | 单个摄像头详情。 |
| `OpenCamera` | `OpenCameraRequest` | `OpenCameraResponse` | 开始采集。 |
| `CloseCamera` | `CloseCameraRequest` | `CloseCameraResponse` | 停止采集。 |
| `CaptureFrame` | `CaptureFrameRequest` | `CaptureFrameResponse` | 抓取最新一帧。 |
| `CaptureFrames` | `CaptureFramesRequest` | `CaptureFramesResponse` | 单次请求并发抓取多摄像头最新帧。 |
| `WatchFrames` | `WatchFramesRequest` | `stream VideoFrame` | 持续推流。 |

### 枚举

#### `CameraState`

摄像头设备的生命周期状态。

| 值 | # | 含义 |
|-------|---|---------|
| `CAMERA_STATE_UNSPECIFIED` | 0 | 未知 / 未设置。 |
| `CAMERA_STATE_CLOSED` | 1 | 已关闭。 |
| `CAMERA_STATE_OPEN` | 2 | 已打开（采集中）。 |
| `CAMERA_STATE_ERROR` | 3 | 错误。 |

### 消息

#### `ListCamerasRequest`

空。

#### `CameraDescriptor`

摄像头设备的完整描述。

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 唯一 id（如 `camera-0`、`wrist-cam`）。 |
| `name` | `string` | 人类可读名称。 |
| `camera_type` | `string` | 类型（如 `realsense`、`usb_cam`、`opencv_grabber`）。 |
| `serial_number` | `string` | 物理设备序列号。 |
| `width` | `int32` | 采集宽度。 |
| `height` | `int32` | 采集高度。 |
| `fps` | `int32` | 目标帧率。 |
| `enable_depth` | `bool` | 是否启用深度流。 |
| `state` | `CameraState` | 当前生命周期状态。 |
| `supported_resolutions` | `repeated string` | 支持分辨率（`WxH`，升序）。 |
| `supported_fps` | `repeated int32` | 支持帧率（升序）。 |
| `pixel_format` | `string` | 摄像头采集的像素格式（如 `mjpeg`、`h264`、`yuyv`）。由自动检测填充；未枚举时为空。 |

#### `ListCamerasResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `cameras` | `repeated CameraDescriptor` | 所有受管摄像头。 |

#### `GetCameraInfoRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |

#### `GetCameraInfoResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera` | `CameraDescriptor` | 摄像头信息。 |

#### `OpenCameraRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |
| `width` | `int32`（可选） | 单次打开的宽度覆盖。 |
| `height` | `int32`（可选） | 单次打开的高度覆盖。 |
| `fps` | `int32`（可选） | 单次打开的帧率覆盖。 |
| `encoding` | `string`（可选） | 编码提示。帧模式静图：`jpeg`（默认）/ `png` / `bmp` / `tiff`（每条消息一帧完整、可独立解码的图像）。码流模式：`h264` / `h265`（Annex B 基础码流分片）。 |

#### `OpenCameraResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |
| `state` | `CameraState` | 结果状态。 |
| `encoding` | `string` | 实际编码（硬件回退时可能与请求不同）。 |

#### `CloseCameraRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |

#### `CloseCameraResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |
| `state` | `CameraState` | 结果状态。 |

#### `CaptureFrameRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |
| `timeout` | `int32`（可选） | 等待帧的秒数（默认 5）。 |

#### `CaptureFrameResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id。 |
| `data` | `bytes` | 编码后的图像数据（按 open 时的编码）。 |
| `width` | `int32` | 实际帧宽。 |
| `height` | `int32` | 实际帧高。 |
| `encoding` | `string` | 数据编码：`jpeg` / `png` / `bmp` / `tiff` / `h264` / `h265` 之一（见 `OpenCameraRequest`）。 |
| `timestamp_ns` | `int64` | 单调采集时间戳（ns）。 |

#### `CapturedFrame`

`CaptureFramesResponse` 中单个摄像头的结果。与 `CaptureFrameResponse` 字段一致，额外带每摄像头错误字段，以便部分失败时不影响整批。

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id（始终设置）。 |
| `data` | `bytes` | 编码后的图像数据。`error_code != 0` 时为空。 |
| `width` | `int32` | 实际帧宽。 |
| `height` | `int32` | 实际帧高。 |
| `encoding` | `string` | 数据编码。 |
| `timestamp_ns` | `int64` | 单调采集时间戳（ns）。 |
| `error_code` | `int32` | gRPC 状态码（`google.rpc.Code`），`0` = OK。 |
| `error` | `string` | `error_code != 0` 时的人类可读错误信息。 |

#### `CaptureFramesRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_ids` | `repeated string` | 要抓取的摄像头 id（去重、保留首次出现顺序）。空为非法。 |
| `timeout` | `int32`（可选） | 每摄像头等待帧的秒数（默认 5）。 |

#### `CaptureFramesResponse`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `frames` | `repeated CapturedFrame` | 每个请求摄像头一个结果（请求顺序）。失败摄像头以 `error_code` 标记并包含。 |

### 视频推流

#### `WatchFramesRequest`

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `camera_id` | `string` | 摄像头 id（须已 open）。 |

#### `VideoFrame`

`WatchFrames` 流中的单条消息。解读方式取决于**打开摄像头时使用的编码**（`OpenCameraResponse.encoding`）：

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `data` | `bytes` | 编码数据 —— 见下方两种模式。 |
| `width` | `int32` | 帧/分片宽度（px）。 |
| `height` | `int32` | 帧/分片高度（px）。 |
| `encoding` | `string` | `jpeg` / `png` / `bmp` / `tiff` / `h264` / `h265`。 |
| `timestamp_ns` | `int64` | 单调采集时间戳（ns）；码流模式为分片首帧 PTS。 |
| `sequence` | `uint64` | 帧模式：每帧 +1。码流模式：字节流位置。 |
| `keyframe` | `bool` | 帧模式：恒为 true。码流模式：分片含 IDR 帧时为 true。 |

**帧模式（`jpeg` / `png` / `bmp` / `tiff`）** —— `data` 恰好为一帧完整、可独立解码的图像。流内 `width`/`height` 恒定；`sequence` 每帧 +1；`keyframe` 恒为 true。

**码流模式（`h264` / `h265`）** —— `data` 含一条或多条 Annex B 基础码流的 NAL 单元（`0x00000001` 起始码）。客户端**必须按 `sequence` 顺序拼接所有分片**才能得到完整码流。`width`/`height` 在首条消息设置，可能在关键帧处变化（分辨率切换）。`sequence` 是字节流位置提示，非帧计数。`keyframe` 在分片含 IDR 帧时为 true —— 客户端可从该分片安全开始解码。

实时消费（显示、ML 推理）建议用 `WatchFrames` 而非轮询 `CaptureFrame`：避免请求/响应开销，按摄像头原生帧率推送。客户端取消 RPC context 或摄像头关闭时流结束。
