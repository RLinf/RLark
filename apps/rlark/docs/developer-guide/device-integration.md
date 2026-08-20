# New Device Integration

## Overview

RLark's embodied runtime (`apps/embodied-runtime`) enables robots and cameras to participate in training jobs as resource-schedulable devices. The runtime consists of a device plugin for Kubernetes, gRPC-based device controllers, and multi-language SDKs.

## Architecture

![Embodied Runtime Architecture](../images/embodied-runtime-architecture.svg)

For a deep dive into the Embodied Runtime internals, see [Embodied Runtime](embodied-runtime-reference.md).

## Device Resource Concept

RLark distinguishes between two concepts:

| Concept | Example | Description |
|---------|---------|-------------|
| Device Resource Name | `rlinf.io/device-franka` | Used for scheduling in Job creation; identifies the device type |
| Device ID | `franka-robot-1` | Physical device identifier; obtained inside the Worker after allocation |

When creating a training job, users specify device resource names and quantities. The scheduler allocates a Worker to a node that has the requested device type available. The actual device ID is discovered inside the Worker using `rosctr list` or `camctr list`.

## Adding a New Device

### Step 1: Define the Device Resource

Register the device in the device plugin configuration:

```yaml
# Example: Franka robot device
resources:
  - name: rlinf.io/device-franka
    count: 2  # Number of devices on this node
```

### Step 2: Implement the Device Controller

Create a gRPC service that implements the device controller interface (`proto/embodied-runtime/`):

```go
// Robot controller
service RobotController {
  rpc ListRobots(ListRobotsRequest) returns (ListRobotsResponse);
  rpc GetRobotStatus(GetRobotStatusRequest) returns (RobotStatus);
  rpc ExecuteAction(ExecuteActionRequest) returns (ExecuteActionResponse);
}

// Camera controller
service CameraController {
  rpc ListCameras(ListCamerasRequest) returns (ListCamerasResponse);
  rpc GetCameraInfo(GetCameraInfoRequest) returns (CameraInfo);
  rpc CaptureFrame(CaptureFrameRequest) returns (CaptureFrameResponse);
}
```

### Step 3: Deploy the Controller

Deploy the device controller alongside the RLark agent on the target node:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: embodied-runtime-controller
spec:
  selector:
    matchLabels:
      app: embodied-runtime
  template:
    spec:
      containers:
      - name: controller
        image: rlark-embodied-runtime:latest
        volumeMounts:
        - name: device-socket
          mountPath: /var/run/rlark
      volumes:
      - name: device-socket
        hostPath:
          path: /var/run/rlark
```

### Step 4: Test Discovery

After deployment, verify the device is discoverable:

```bash
# Inside a Worker container
rosctr list
# Expected output:
# franka-robot-1  (rlinf.io/device-franka)  [READY]

camctr list
# Expected output:
# video0  (rlinf.io/device-realsense)  [ACTIVE]
```

### Step 5: Verify in a Job

Create a test Job that requests the new device resource and verify the Worker can access it.

## Safety Requirements for Real Devices

When working with physical robots, the following safety measures are mandatory:

| Measure | Description |
|---------|-------------|
| Device Exclusivity | Real-device tasks must be scheduled exclusively; no sharing with other tasks |
| Arming | Safety approval and device armed status must be confirmed before task start |
| Motion Limits | Set safety boundaries for joint positions, velocities, and torques |
| Hardware E-Stop | Confirm emergency stop buttons are functional and accessible |
| Heartbeat Timeout | Configure heartbeat timeout; auto-enter safe state on disconnect |
| Audit Logging | Record operation logs for post-incident review |

!!! warning "Do not operate real robots"
    Before completing device exclusivity, human confirmation, motion boundaries, E-stop, disconnect safety state, and recovery authorization, do not issue real-device motion commands.

## Pre-submission Checklist

- [ ] Device exclusivity confirmed
- [ ] Safety approval obtained
- [ ] Motion boundaries configured
- [ ] E-stop functional
- [ ] Heartbeat timeout configured
- [ ] Audit logging enabled

## CLI Tools

### rosctr

Robot controller CLI tool:

```bash
rosctr list          # List available robots
rosctr status <id>   # Check robot status
rosctr arm <id>      # Arm the robot
rosctr disarm <id>   # Disarm the robot
```

### camctr

Camera controller CLI tool:

```bash
camctr list          # List available cameras
camctr info <id>     # Get camera information
camctr capture <id>  # Capture a frame
```

## SDK References

### Python SDK

```python
from embodied_runtime import RobotClient, CameraClient

# Connect to robot
robot = RobotClient("franka-robot-1")
status = robot.get_status()
robot.execute_action({"type": "move_joint", "target": [0.1, 0.2, 0.3]})

# Connect to camera
camera = CameraClient("video0")
info = camera.get_info()
frame = camera.capture()
```

See `sdks/embodied-runtime-python/` for full API documentation.

### Go SDK

```go
import "github.com/rlinf/rlark/sdks/embodied-runtime-go"

client := embodiedruntime.NewRobotClient("franka-robot-1")
status, err := client.GetStatus(ctx)
```

See `sdks/embodied-runtime-go/` for full API documentation.

## Node Categories

Nodes are labeled with `rlark.io/node-category` to indicate their type:

| Category | Description |
|----------|-------------|
| `cloud` | Cloud GPU compute nodes |
| `edge` | Edge compute nodes with embodied devices |
| `robot` | NUCs, industrial PCs, or mobile robot onboard computers bound to robot arms |

!!! note "Node category filtering"
    Use the node category label to filter nodes in the console and to target specific hardware types in Job nodeSelectors.

## Reference Materials

| Resource | Path |
|----------|------|
| Embodied Runtime Reference | [Embodied Runtime](embodied-runtime-reference.md) |
| Embodied Runtime CLI | `apps/embodied-runtime/docs/cli.md` |
| Deployment Examples | `apps/embodied-runtime/docs/examples.md` |
| gRPC API | `proto/embodied-runtime/` |
| Python SDK | `sdks/embodied-runtime-python/` |
| Go SDK | `sdks/embodied-runtime-go/` |