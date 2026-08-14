# embodied-runtime gRPC API

English | [简体中文](./proto-api.zh-CN.md)

This document describes the two gRPC services exposed by embodied-runtime:

- **`RobotController`** — robot (ROS) lifecycle and control-mode management.
- **`CameraController`** — camera capture, single-frame grab, and live streaming.

Both are defined in Protocol Buffers v3 under [`proto/embodied-runtime/`](../../../proto/embodied-runtime) and served over Unix domain sockets. This is a functional reference; the `.proto` files are the authoritative source for field numbers and wire types.

## Overview

| Service | Package | Proto | Default socket |
|---------|---------|-------|----------------|
| `RobotController` | `ros.controller.v1` | `proto/roscontroller/v1/robot.proto` | `/var/run/rlark/ros-ctrl.sock` |
| `CameraController` | `camera.controller.v1` | `proto/cameracontroller/v1/camera.proto` | `/var/run/rlark/camera-ctrl.sock` |

### Transport

- Both services listen on a Unix domain socket under the node-local `/var/run/rlark` directory, which the device plugin mounts (read-only) into task pods.
- The device plugin injects `RLINF_EMBODIED_ROS_SOCKET_PATH` (ROS 1), `RLINF_EMBODIED_ROS2_SOCKET_PATH` (ROS 2), and `RLINF_EMBODIED_CAMERA_SOCKET_PATH`; the `rosctr` / `camctr` CLIs read them automatically (an explicit `--socket-path` argument always wins). For `rosctr`, the ROS 1 socket path takes priority; when it is unset the ROS 2 socket path is used.

### HTTP gateway

Each controller optionally mirrors its gRPC API over HTTP/JSON so non-gRPC clients (curl, browsers, `fetch`) can drive it. Enable with the `--http-addr` flag (`camera-controller --http-addr :8080`, `ros-controller --http-addr :8080`) or the `http_addr` field in the controller's config file. In the device-plugin config, set `camera.http_addr` / `ros.http_addr` — it is written into the controller's config file and applies in **both local and pod mode** (it is not a pod-only setting). Empty disables HTTP; only the gRPC Unix socket is exposed.

Responses use canonical proto JSON (lowerCamelCase, `EmitUnpopulated`), matching `camctr -o json` / `rosctr -o json`. gRPC status errors map to the closest HTTP code (`NotFound`→404, `InvalidArgument`→400, `PermissionDenied`→403, `FailedPrecondition`→412, `Unimplemented`→501, …).

**CameraController** (`/v1/`):

| Method | Path | RPC |
|--------|------|-----|
| GET  | `/v1/cameras` | `ListCameras` |
| GET  | `/v1/cameras/{camera_id}` | `GetCameraInfo` |
| POST | `/v1/cameras/{camera_id}/open` | `OpenCamera` |
| POST | `/v1/cameras/{camera_id}/close` | `CloseCamera` |
| GET  | `/v1/cameras/{camera_id}/frame` | `CaptureFrame` (raw bytes + metadata headers by default; `Accept: application/json` → JSON) |
| POST | `/v1/cameras:captureFrames` | `CaptureFrames` |
| GET  | `/v1/cameras/{camera_id}/watch` | `WatchFrames` (Server-Sent Events; one `VideoFrame` per event) |

**RobotController** (`/v1/`):

| Method | Path | RPC |
|--------|------|-----|
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
| ANY  | `/v1/robots/{robot_id}/proxy/*` | reverse proxy → robot's `web_service` URL |

For `StartRobot` / `SwitchMode`, the request body is the proto message minus the `robot_id` (which comes from the path); path-derived IDs always win over any value in the JSON body. The per-robot web proxy (`/v1/robots/{robot_id}/proxy/*`) reverse-proxies to each robot's configured `web_service`; `Location` headers in the response are rewritten back under the same `/v1/robots/{robot_id}/proxy` prefix so redirects stay inside the gateway.

### Clients

- **Go** — `rosctr` / `camctr` CLIs (`cmd/rosctr`, `cmd/camctr`) or import the stubs directly.

---

## RobotController

`ros.controller.v1.RobotController` manages robot nodes on the host: lifecycle (start / stop / reset), control-mode switching, status, logs, and ROS package introspection. The device plugin calls it to drive robot nodes (e.g. Franka) that run on the host network.

The service is implemented by two independent controllers that register on separate Unix sockets and honour the same RPC contract:

- **ROS 1** (`ros-controller` on `ros-ctrl.sock`): starts a per-robot `roscore`; fills `ros_master_uri` in responses.
- **ROS 2** (`ros2-controller` on `ros2-ctrl.sock`): no master; assigns a per-robot `ROS_DOMAIN_ID` for DDS isolation; fills `ros_domain_id` in responses.

> **Multicast requirement (ROS 2 only).** ROS 2 DDS uses IP multicast for node discovery by default. The cluster's network layer (CNI plugin, node / underlay switches) **must support multicast routing** for ROS 2 nodes in different pods / nodes to discover each other. If multicast is blocked, configure unicast discovery via `CYCLONEDDS_URI` in each robot's mode-level `env`, or deploy a DDS Discovery Server.

### RPCs

| RPC | Request | Response | Description |
|-----|---------|----------|-------------|
| `StartRobot` | `StartRobotRequest` | `StartRobotResponse` | Launch a robot node in a control mode. |
| `StopRobot` | `StopRobotRequest` | `StopRobotResponse` | Stop a running robot node. |
| `GetRobotStatus` | `GetRobotStatusRequest` | `GetRobotStatusResponse` | Current status of a robot node. |
| `SwitchMode` | `SwitchModeRequest` | `SwitchModeResponse` | Change the control mode of a running robot. |
| `ListRobots` | `ListRobotsRequest` | `ListRobotsResponse` | All managed robots and their status. |
| `ListModes` | `ListModesRequest` | `ListModesResponse` | Supported control modes for a robot. |
| `GetRobotLogs` | `GetRobotLogsRequest` | `GetRobotLogsResponse` | Recent launch process log lines. |
| `ListPackages` | `ListPackagesRequest` | `ListPackagesResponse` | Whitelisted ROS packages. |
| `GetPackageInfo` | `GetPackageInfoRequest` | `GetPackageInfoResponse` | Metadata for a ROS package. |
| `GetPackageLaunchFiles` | `GetPackageLaunchFilesRequest` | `GetPackageLaunchFilesResponse` | Launch files in a ROS package. |
| `GetLaunchFileArgs` | `GetLaunchFileArgsRequest` | `GetLaunchFileArgsResponse` | Arguments supported by a launch file. |
| `ResetRobot` | `ResetRobotRequest` | `ResetRobotResponse` | Stop, restart ROS middleware (roscore for ROS 1; launch process for ROS 2), reset state to STOPPED. |

### Enums

#### `RobotState`

Lifecycle state of a robot node.

| Value | # | Meaning |
|-------|---|---------|
| `ROBOT_STATE_UNSPECIFIED` | 0 | Unknown / not set. |
| `ROBOT_STATE_STARTING` | 1 | Launching. |
| `ROBOT_STATE_RUNNING` | 2 | Running. |
| `ROBOT_STATE_STOPPING` | 3 | Stopping. |
| `ROBOT_STATE_STOPPED` | 4 | Stopped. |
| `ROBOT_STATE_ERROR` | 5 | Error. |

### Messages — lifecycle

#### `StartRobotRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Unique robot id (e.g. `franka-0`). |
| `mode` | `string` | Initial control mode (e.g. `impedance`, `joint`). Acts as a label when `mode_config` is set. |
| `mode_config` | `ModeConfig` | Optional custom mode config; bypasses preset lookup by name. |

#### `StartRobotResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `state` | `RobotState` | Resulting state. |
| `current_mode` | `ModeInfo` | Resolved mode info. |
| `ros_master_uri` | `string` | ROS master URI the robot listens on (ROS 1 only). |
| `params` | `map<string,string>` | Robot-level params referenceable via `arg_from`. |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID` for DDS isolation (ROS 2 only; 0 for ROS 1). |

#### `StopRobotRequest` / `StopRobotResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id (both request and response). |

#### `GetRobotStatusRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |

#### `GetRobotStatusResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `mode` | `string` | Current mode name. |
| `state` | `RobotState` | Lifecycle state. |
| `current_mode` | `ModeInfo` | Resolved mode info. |
| `ros_master_uri` | `string` | ROS master URI (ROS 1 only). |
| `params` | `map<string,string>` | Robot-level params. |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID` for DDS isolation (ROS 2 only; 0 for ROS 1). |

#### `SwitchModeRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `mode` | `string` | Target mode. Label only when `mode_config` is set. |
| `mode_config` | `ModeConfig` | Optional custom mode config. |

#### `SwitchModeResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `mode` | `string` | Active mode. |
| `state` | `RobotState` | Lifecycle state. |
| `current_mode` | `ModeInfo` | Resolved mode info. |
| `ros_master_uri` | `string` | ROS master URI (ROS 1 only). |
| `params` | `map<string,string>` | Robot-level params. |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID` for DDS isolation (ROS 2 only; 0 for ROS 1). |

#### `ResetRobotRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |

#### `ResetRobotResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `state` | `RobotState` | State after reset (STOPPED). |
| `ros_master_uri` | `string` | ROS master URI (ROS 1; may change after roscore restart). |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID` (ROS 2; preserved across reset). |

### Messages — inventory & modes

#### `ListRobotsRequest`

Empty.

#### `ListRobotsResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robots` | `repeated RobotInfo` | All managed robots. |

#### `RobotInfo`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `mode` | `string` | Current mode name. |
| `state` | `RobotState` | Lifecycle state. |
| `current_mode` | `ModeInfo` | Resolved mode info. |
| `ros_master_uri` | `string` | ROS master URI (ROS 1 only). |
| `params` | `map<string,string>` | Robot-level params. |
| `ros_domain_id` | `int32` | `ROS_DOMAIN_ID` for DDS isolation (ROS 2 only; 0 for ROS 1). |

#### `ListModesRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |

#### `ListModesResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `modes` | `repeated ModeInfo` | Available control modes with full config. |

#### `ModeInfo`

Full configuration of a preset control mode.

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Mode name (e.g. `impedance`, `joint`). |
| `package` | `string` | ROS package containing the launch file. |
| `launch_file` | `string` | Launch file name (e.g. `impedance.launch`). |
| `args` | `map<string,string>` | Default key=value args passed to roslaunch. |
| `env` | `map<string,string>` | Extra env vars for the roslaunch process. |
| `arg_from` | `map<string,string>` | Maps roslaunch arg names → robot param names. |
| `passthrough_robot_args` | `bool` | Merge all robot params with identity mapping. |

#### `ModeConfig`

Ad-hoc custom mode passed inline in `StartRobot` / `SwitchMode` to bypass preset modes. Mirrors the server's ModeConfig struct.

| Field | Type | Description |
|-------|------|-------------|
| `package` | `string` | ROS package containing the launch file. |
| `launch_file` | `string` | Launch file name. |
| `args` | `map<string,string>` | key=value args passed to roslaunch. |
| `passthrough_robot_args` | `bool` | Merge all robot params (identity). Mutually exclusive with `arg_from`. |
| `arg_from` | `map<string,string>` | arg-name → param-name mapping. Mutually exclusive with `passthrough_robot_args`. |
| `env` | `map<string,string>` | Extra env vars for the roslaunch process. |

### Messages — logs

#### `GetRobotLogsRequest`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `tail` | `int32` | Number of recent lines; 0 = all buffered lines. |

#### `GetRobotLogsResponse`

| Field | Type | Description |
|-------|------|-------------|
| `robot_id` | `string` | Robot id. |
| `lines` | `repeated string` | Recent log lines (empty if never started or cleared). |

### Messages — ROS package introspection

#### `ListPackagesRequest`

Empty.

#### `ListPackagesResponse`

| Field | Type | Description |
|-------|------|-------------|
| `packages` | `repeated string` | Whitelisted ROS package names. |

#### `GetPackageInfoRequest`

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | ROS package name (e.g. `franka_ros`). |

#### `PackageInfo`

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Package name. |
| `version` | `string` | Version from package.xml. |
| `description` | `string` | Description from package.xml. |
| `maintainer` | `string` | Maintainer name + email. |
| `allowed` | `bool` | Whether in the allowed launch packages whitelist. |

#### `GetPackageInfoResponse`

| Field | Type | Description |
|-------|------|-------------|
| `info` | `PackageInfo` | Package metadata. |

#### `GetPackageLaunchFilesRequest`

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | ROS package name. |

#### `GetPackageLaunchFilesResponse`

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Package name. |
| `launch_files` | `repeated string` | Launch file names. |

#### `GetLaunchFileArgsRequest`

| Field | Type | Description |
|-------|------|-------------|
| `package` | `string` | ROS package name. |
| `launch_file` | `string` | Launch file name. |

#### `LaunchArg`

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Argument name. |
| `required` | `bool` | Whether required (no default). |
| `default` | `string` | Default value, if any. |
| `description` | `string` | Description from the arg tag or comment. |

#### `GetLaunchFileArgsResponse`

| Field | Type | Description |
|-------|------|-------------|
| `package` | `string` | Package name. |
| `launch_file` | `string` | Launch file name. |
| `args` | `repeated LaunchArg` | Supported arguments. |

---

## CameraController

`camera.controller.v1.CameraController` manages camera devices: list, open / close, single-frame capture, multi-camera capture, and live frame streaming. The camera must be open before `CaptureFrame` / `CaptureFrames` / `WatchFrames`.

### RPCs

| RPC | Request | Response | Description |
|-----|---------|----------|-------------|
| `ListCameras` | `ListCamerasRequest` | `ListCamerasResponse` | All managed cameras + state. |
| `GetCameraInfo` | `GetCameraInfoRequest` | `GetCameraInfoResponse` | Detailed info for one camera. |
| `OpenCamera` | `OpenCameraRequest` | `OpenCameraResponse` | Start frame capture. |
| `CloseCamera` | `CloseCameraRequest` | `CloseCameraResponse` | Stop frame capture. |
| `CaptureFrame` | `CaptureFrameRequest` | `CaptureFrameResponse` | Grab the latest frame. |
| `CaptureFrames` | `CaptureFramesRequest` | `CaptureFramesResponse` | Grab the latest frame from multiple cameras in one request (concurrent). |
| `WatchFrames` | `WatchFramesRequest` | `stream VideoFrame` | Stream frames continuously. |

### Enums

#### `CameraState`

Lifecycle state of a camera device.

| Value | # | Meaning |
|-------|---|---------|
| `CAMERA_STATE_UNSPECIFIED` | 0 | Unknown / not set. |
| `CAMERA_STATE_CLOSED` | 1 | Closed. |
| `CAMERA_STATE_OPEN` | 2 | Open (capture running). |
| `CAMERA_STATE_ERROR` | 3 | Error. |

### Messages

#### `ListCamerasRequest`

Empty.

#### `CameraDescriptor`

Full description of a camera device.

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Unique id (e.g. `camera-0`, `wrist-cam`). |
| `name` | `string` | Human-readable name. |
| `camera_type` | `string` | Type (e.g. `realsense`, `usb_cam`, `opencv_grabber`). |
| `serial_number` | `string` | Physical device serial. |
| `width` | `int32` | Capture width. |
| `height` | `int32` | Capture height. |
| `fps` | `int32` | Target frame rate. |
| `enable_depth` | `bool` | Whether depth stream is enabled. |
| `state` | `CameraState` | Current lifecycle state. |
| `supported_resolutions` | `repeated string` | Supported resolutions as `WxH` (ascending). |
| `supported_fps` | `repeated int32` | Supported frame rates (ascending). |
| `pixel_format` | `string` | Pixel format the camera captures in (e.g. `mjpeg`, `h264`, `yuyv`). Populated from autodetect; empty when not enumerated. |

#### `ListCamerasResponse`

| Field | Type | Description |
|-------|------|-------------|
| `cameras` | `repeated CameraDescriptor` | All managed cameras. |

#### `GetCameraInfoRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |

#### `GetCameraInfoResponse`

| Field | Type | Description |
|-------|------|-------------|
| `camera` | `CameraDescriptor` | Camera info. |

#### `OpenCameraRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |
| `width` | `int32` (optional) | Per-open width override. |
| `height` | `int32` (optional) | Per-open height override. |
| `fps` | `int32` (optional) | Per-open fps override. |
| `encoding` | `string` (optional) | Encoding hint. Frame-mode still-image: `jpeg` (default) / `png` / `bmp` / `tiff` (one complete, independently decodable frame per message). Bitstream-mode: `h264` / `h265` (Annex B elementary-stream chunks). |

#### `OpenCameraResponse`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |
| `state` | `CameraState` | Resulting state. |
| `encoding` | `string` | Actual encoding (may differ from request on hardware fallback). |

#### `CloseCameraRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |

#### `CloseCameraResponse`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |
| `state` | `CameraState` | Resulting state. |

#### `CaptureFrameRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |
| `timeout` | `int32` (optional) | Seconds to wait for a frame (default 5). |

#### `CaptureFrameResponse`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id. |
| `data` | `bytes` | Encoded image data (per the open encoding). |
| `width` | `int32` | Actual frame width. |
| `height` | `int32` | Actual frame height. |
| `encoding` | `string` | Data encoding: one of `jpeg` / `png` / `bmp` / `tiff` / `h264` / `h265` (see `OpenCameraRequest`). |
| `timestamp_ns` | `int64` | Monotonic capture timestamp (ns). |

#### `CapturedFrame`

A single camera's result within a `CaptureFramesResponse`. Mirrors `CaptureFrameResponse` plus per-camera error fields so partial failures can be reported without failing the whole batch.

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id (always set). |
| `data` | `bytes` | Encoded image data. Empty when `error_code != 0`. |
| `width` | `int32` | Actual frame width. |
| `height` | `int32` | Actual frame height. |
| `encoding` | `string` | Data encoding. |
| `timestamp_ns` | `int64` | Monotonic capture timestamp (ns). |
| `error_code` | `int32` | gRPC status code (`google.rpc.Code`); `0` = OK. |
| `error` | `string` | Human-readable error message when `error_code != 0`. |

#### `CaptureFramesRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_ids` | `repeated string` | Camera ids to capture from (duplicates de-duplicated; order preserved). Empty is invalid. |
| `timeout` | `int32` (optional) | Seconds to wait for a frame, per camera (default 5). |

#### `CaptureFramesResponse`

| Field | Type | Description |
|-------|------|-------------|
| `frames` | `repeated CapturedFrame` | One result per requested camera (request order). Failed cameras included with `error_code` set. |

### Video streaming

#### `WatchFramesRequest`

| Field | Type | Description |
|-------|------|-------------|
| `camera_id` | `string` | Camera id (must be open). |

#### `VideoFrame`

A single message in the `WatchFrames` stream. Interpretation depends on the **encoding the camera was opened with** (`OpenCameraResponse.encoding`):

| Field | Type | Description |
|-------|------|-------------|
| `data` | `bytes` | Encoded data — see modes below. |
| `width` | `int32` | Frame/chunk width (px). |
| `height` | `int32` | Frame/chunk height (px). |
| `encoding` | `string` | `jpeg` / `png` / `bmp` / `tiff` / `h264` / `h265`. |
| `timestamp_ns` | `int64` | Monotonic capture timestamp (ns); bitstream mode = PTS of first frame in chunk. |
| `sequence` | `uint64` | Frame mode: +1 per frame. Bitstream mode: byte-stream position. |
| `keyframe` | `bool` | Frame mode: always true. Bitstream mode: true when chunk has an IDR frame. |

**Frame mode (`jpeg` / `png` / `bmp` / `tiff`)** — `data` holds exactly one complete, independently decodable frame. `width`/`height` are constant within a stream; `sequence` increments by 1 per frame; `keyframe` is always true.

**Bitstream mode (`h264` / `h265`)** — `data` holds one or more NAL units of the Annex B elementary stream (`0x00000001` start codes). The client **must concatenate all chunks in `sequence` order** to rebuild a valid stream. `width`/`height` are set on the first message and may change on keyframes (resolution change). `sequence` is a byte-stream position hint, not a frame counter. `keyframe` is true when the chunk contains an IDR frame — the client can safely begin decoding from there.

Prefer `WatchFrames` over polling `CaptureFrame` for real-time consumption (display, ML inference): it avoids request/response overhead and delivers frames at the native camera rate. The stream ends when the client cancels the RPC context or the camera is closed.
