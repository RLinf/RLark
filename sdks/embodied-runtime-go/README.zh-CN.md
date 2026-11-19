# embodied-runtime Go SDK

[English](./README.md) | 简体中文

[embodied-runtime][repo] 机器人（ROS）与摄像头 gRPC 服务的 Go 客户端。两个控制器在 task pod 内通过 **Unix domain socket** 暴露；本包提供生成的 gRPC stub 和 `transport` 辅助包处理 socket 连接。

[repo]: https://github.com/rlinf/rlark/apps/embodied-runtime

生成的 gRPC stub 位于 `gen/`；`embodiedruntime` 包（`transport.go`）提供便捷的连接函数。

## 安装

```bash
go get github.com/rlinf/rlark/sdks/embodied-runtime-go
```

## 快速上手

### Robot 客户端

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/rlinf/rlark/sdks/embodied-runtime-go"
    rosctrl "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    conn, err := embodiedruntime.DialRobot("")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := rosctrl.NewRobotControllerClient(conn)

    // 列出机器人
    resp, err := client.ListRobots(ctx, &rosctrl.ListRobotsRequest{})
    if err != nil {
        log.Fatal(err)
    }
    for _, r := range resp.Robots {
        log.Printf("robot=%s mode=%s state=%v", r.RobotId, r.Mode, r.State)
    }

    // 启动机器人
    _, err = client.StartRobot(ctx, &rosctrl.StartRobotRequest{
        RobotId: "franka-0",
        Mode:    "impedance",
        Args:    map[string]string{"robot_ip": "172.16.0.2"},
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

### Camera 客户端

```go
package main

import (
    "context"
    "io"
    "log"
    "time"

    "github.com/rlinf/rlark/sdks/embodied-runtime-go"
    camctrl "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    conn, err := embodiedruntime.DialCamera("")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := camctrl.NewCameraControllerClient(conn)

    // 打开摄像头
    _, err = client.OpenCamera(ctx, &camctrl.OpenCameraRequest{
        CameraId: "camera-0",
        Encoding: "h264",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 流式获取帧
    stream, err := client.WatchFrames(ctx, &camctrl.WatchFramesRequest{
        CameraId: "camera-0",
    })
    if err != nil {
        log.Fatal(err)
    }
    for {
        frame, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Fatal(err)
        }
        log.Printf("seq=%d encoding=%s size=%d keyframe=%v",
            frame.Sequence, frame.Encoding, len(frame.Data), frame.Keyframe)
    }
}
```

## 客户端 API

### RobotController — `ros.controller.v1.RobotController`

| RPC | 请求 | 说明 |
|-----|------|------|
| `StartRobot` | `StartRobotRequest` | 以预置或自定义模式启动机器人 |
| `StopRobot` | `StopRobotRequest` | 停止正在运行的机器人 |
| `GetRobotStatus` | `GetRobotStatusRequest` | 获取机器人状态 |
| `SwitchMode` | `SwitchModeRequest` | 切换运行中机器人的控制模式 |
| `ResetRobot` | `ResetRobotRequest` | 重启 roscore 和状态 |
| `ListRobots` | `ListRobotsRequest` | 列出所有机器人及其状态 |
| `ListModes` | `ListModesRequest` | 列出机器人可用的控制模式 |
| `GetRobotLogs` | `GetRobotLogsRequest` | 获取机器人日志（可选 tail） |
| `ListPackages` | `ListPackagesRequest` | 列出白名单 ROS 包 |
| `GetPackageInfo` | `GetPackageInfoRequest` | 获取包详情 |
| `GetPackageLaunchFiles` | `GetPackageLaunchFilesRequest` | 列出包中的 launch 文件 |
| `GetLaunchFileArgs` | `GetLaunchFileArgsRequest` | 列出 launch 文件的参数 |

### CameraController — `camera.controller.v1.CameraController`

| RPC | 请求 | 说明 |
|-----|------|------|
| `ListCameras` | `ListCamerasRequest` | 列出所有摄像头及其状态 |
| `GetCameraInfo` | `GetCameraInfoRequest` | 获取摄像头详细信息 |
| `OpenCamera` | `OpenCameraRequest` | 打开摄像头（可指定宽/高/帧率/编码） |
| `CloseCamera` | `CloseCameraRequest` | 关闭摄像头 |
| `CaptureFrame` | `CaptureFrameRequest` | 捕获单帧 |
| `CaptureFrames` | `CaptureFramesRequest` | 并发捕获多摄像头帧 |
| `WatchFrames` | `WatchFramesRequest` | 流式获取帧（服务端流） |

## 连接

`embodiedruntime` 包提供便捷的连接函数：

```go
// 默认：Unix socket /var/run/rlark/ros-ctrl.sock
conn, err := embodiedruntime.DialRobot("")

// 默认：Unix socket /var/run/rlark/camera-ctrl.sock
conn, err := embodiedruntime.DialCamera("")

// 远程 TCP 服务
conn, err := embodiedruntime.DialRobot("10.0.0.5:50051")

// 手动指定自定义 target
target := embodiedruntime.UnixTarget("/custom/path.sock")
conn, err := embodiedruntime.Dial(target)
```

## 环境变量

device plugin 会向 task pod 注入以下环境变量，`embodiedruntime` 包会自动读取：

| 环境变量 | 使用者 | 默认值 |
|---------|--------|--------|
| `RLINF_EMBODIED_ROS_SOCKET_PATH` | `DialRobot()` | `/var/run/rlark/ros-ctrl.sock` |
| `RLINF_EMBODIED_ROS2_SOCKET_PATH` | `DialRobot2()` | `/var/run/rlark/ros2-ctrl.sock` |
| `RLINF_EMBODIED_CAMERA_SOCKET_PATH` | `DialCamera()` | `/var/run/rlark/camera-ctrl.sock` |

## 重新生成 stub

Stub 从 [proto/embodied-runtime](../../proto/embodied-runtime) 生成：

```bash
# 在仓库根目录执行 — 生成所有 SDK stub（Go + Python）
make proto

# 或在 proto 目录下执行 — 仅生成 Go stub
make -C proto/embodied-runtime proto-go
```

## 检查与格式化

```bash
make lint          # golangci-lint
make fmt           # gofmt
```

## 项目结构

```
sdks/embodied-runtime-go/
├── gen/
│   ├── roscontroller/v1/     # Robot gRPC stub（生成）
│   └── cameracontroller/v1/  # Camera gRPC stub（生成）
├── transport.go              # Dial 辅助函数 + socket 解析
├── go.mod / go.sum
├── Makefile
└── README.md
```
