# embodied-runtime Go SDK

English | [简体中文](./README.zh-CN.md)

A Go client for the [embodied-runtime][repo] robot (ROS) and camera gRPC services. Both controllers are exposed over **Unix domain sockets** inside a task pod; this package provides the generated stubs and a `transport` helper for socket plumbing.

[repo]: https://github.com/rlinf/rlark/apps/embodied-runtime

Generated gRPC stubs live under `gen/`; the `embodiedruntime` package in `transport.go` provides convenience dial functions.

## Install

```bash
go get github.com/rlinf/rlark/sdks/embodied-runtime-go
```

## Quick start

### Robot client

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

    // List robots
    resp, err := client.ListRobots(ctx, &rosctrl.ListRobotsRequest{})
    if err != nil {
        log.Fatal(err)
    }
    for _, r := range resp.Robots {
        log.Printf("robot=%s mode=%s state=%v", r.RobotId, r.Mode, r.State)
    }

    // Start a robot
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

### Camera client

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

    // Open camera
    _, err = client.OpenCamera(ctx, &camctrl.OpenCameraRequest{
        CameraId: "camera-0",
        Encoding: "h264",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Watch frames
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

## Client API

### RobotController — `ros.controller.v1.RobotController`

| RPC | Request | Description |
|-----|---------|-------------|
| `StartRobot` | `StartRobotRequest` | Start a robot in a preset or custom mode |
| `StopRobot` | `StopRobotRequest` | Stop a running robot |
| `GetRobotStatus` | `GetRobotStatusRequest` | Get robot status |
| `SwitchMode` | `SwitchModeRequest` | Switch a running robot's control mode |
| `ResetRobot` | `ResetRobotRequest` | Restart roscore + state |
| `ListRobots` | `ListRobotsRequest` | List all robots and their status |
| `ListModes` | `ListModesRequest` | List available control modes for a robot |
| `GetRobotLogs` | `GetRobotLogsRequest` | Get robot logs (optional tail) |
| `ListPackages` | `ListPackagesRequest` | List whitelisted ROS packages |
| `GetPackageInfo` | `GetPackageInfoRequest` | Get package details |
| `GetPackageLaunchFiles` | `GetPackageLaunchFilesRequest` | List launch files in a package |
| `GetLaunchFileArgs` | `GetLaunchFileArgsRequest` | List arguments for a launch file |

### CameraController — `camera.controller.v1.CameraController`

| RPC | Request | Description |
|-----|---------|-------------|
| `ListCameras` | `ListCamerasRequest` | List all cameras and their state |
| `GetCameraInfo` | `GetCameraInfoRequest` | Get camera details |
| `OpenCamera` | `OpenCameraRequest` | Open a camera with optional width/height/fps/encoding |
| `CloseCamera` | `CloseCameraRequest` | Close a camera |
| `CaptureFrame` | `CaptureFrameRequest` | Capture a single frame |
| `CaptureFrames` | `CaptureFramesRequest` | Capture frames from multiple cameras concurrently |
| `WatchFrames` | `WatchFramesRequest` | Stream frames (server-side streaming) |

## Connection

The `embodiedruntime` package provides convenience dial functions:

```go
// Default: Unix socket at /var/run/rlark/ros-ctrl.sock
conn, err := embodiedruntime.DialRobot("")

// Default: Unix socket at /var/run/rlark/camera-ctrl.sock
conn, err := embodiedruntime.DialCamera("")

// Remote TCP server
conn, err := embodiedruntime.DialRobot("10.0.0.5:50051")

// Manual dial with custom target
target := embodiedruntime.UnixTarget("/custom/path.sock")
conn, err := embodiedruntime.Dial(target)
```

## Environment variables

The device plugin injects these into task pods; the `embodiedruntime` package reads them automatically:

| Env var | Used by | Default |
|---------|---------|---------|
| `RLINF_EMBODIED_ROS_SOCKET_PATH` | `DialRobot()` | `/var/run/rlark/ros-ctrl.sock` |
| `RLINF_EMBODIED_ROS2_SOCKET_PATH` | `DialRobot2()` | `/var/run/rlark/ros2-ctrl.sock` |
| `RLINF_EMBODIED_CAMERA_SOCKET_PATH` | `DialCamera()` | `/var/run/rlark/camera-ctrl.sock` |

## Regenerate stubs

Stubs are generated from [proto/embodied-runtime](../../proto/embodied-runtime):

```bash
# From repo root — generates all SDK stubs (Go + Python)
make proto

# Or from the proto directory — generate only Go stubs
make -C proto/embodied-runtime proto-go
```

## Lint & format

```bash
make lint          # golangci-lint
make fmt           # gofmt
```

## Project structure

```
sdks/embodied-runtime-go/
├── gen/
│   ├── roscontroller/v1/     # Robot gRPC stubs (generated)
│   └── cameracontroller/v1/  # Camera gRPC stubs (generated)
├── transport.go              # Dial helpers + socket resolution
├── go.mod / go.sum
├── Makefile
└── README.md
```
