# Proto Definitions

gRPC service definitions for [embodied-runtime](../../apps/embodied-runtime).

## Services

| Service | Package | File |
|---------|---------|------|
| `RobotController` | `ros.controller.v1` | [roscontroller/v1/robot.proto](roscontroller/v1/robot.proto) |
| `CameraController` | `camera.controller.v1` | [cameracontroller/v1/camera.proto](cameracontroller/v1/camera.proto) |

## Generate stubs

```bash
make proto
```

Generated code:
- Go: `sdks/embodied-runtime-go/gen/`
- Python: `sdks/embodied-runtime-python/embodied_runtime/gen/`

## Proto API Reference

- [English](../../apps/embodied-runtime/docs/proto-api.md)
- [简体中文](../../apps/embodied-runtime/docs/proto-api.zh-CN.md)
