# Embodied Device Onboarding

## GPU Clusters vs. Embodied Devices

RLark can manage two types of compute resources. See [GPU Cluster Onboarding](data-plane.md) for standard GPU cluster onboarding.

| | GPU Cluster Onboarding | Embodied Device Onboarding |
|---|---|---|
| **Target** | Cloud / on-prem GPU clusters | Edge clusters with robots, cameras, or other physical devices |
| **Agent** | `rlark-agent` connects the cluster to the control plane | Same Agent, plus a **Device Plugin** to register devices |
| **Resources** | `nvidia.com/gpu`, CPU, memory | `rlinf.io/device-*` (robots, cameras, etc.) |
| **Workload** | Standard RL training with Ray | RL training that interacts with real hardware (robots, cameras) |
| **Networking** | Cluster-internal or cross-cluster via Domain | May require host device passthrough or macvlan for fixed-IP robots |

Both follow the same [cluster certificate flow](data-plane.md). The key difference is **what runs on the cluster after onboarding**: a GPU cluster only needs the Agent, while an embodied edge cluster also needs the **Embodied Runtime** to discover and manage physical devices.

## What is Embodied Runtime?

The Embodied Runtime (`apps/embodied-runtime`) enables robots and cameras to participate in RLark training jobs as schedulable devices. It consists of a Kubernetes Device Plugin, gRPC-based controllers for ROS 1, ROS 2, and cameras, and CLI tools for device interaction. For a deep dive into internals, see [Embodied Runtime Reference](../developer-guide/embodied-runtime-reference.md).

## Architecture

The Embodied Runtime has three layers:

| Layer | Component | Description |
|-------|-----------|-------------|
| Device Plugin | `device-plugin` | Registers device resources (`rlinf.io/device-*`) with Kubernetes |
| Controllers | `ros-controller`, `ros2-controller`, `camera-controller` | gRPC services that manage device lifecycle |
| Webhook | Mutating Webhook | Automatically injects `devinit` sidecar for macvlan networking |

### How It Works

1. **Device Plugin** registers with kubelet and advertises `rlinf.io/device[-<model>]` resources.
2. On `Allocate`, it injects the socket directory (`/var/run/rlark`) and CLI binary directory (`/opt/rlinf/bin`) into the requesting pod, along with `RLINF_EMBODIED_*` environment variables.
3. User pods use the mounted CLIs (or gRPC directly) to control hardware.

## Prerequisites

- Kubernetes cluster with compatible edge nodes
- Go 1.26+ (for building from source)
- Docker (for container images)
- Host devices accessible on the target nodes
- `hostPID: true` and `privileged: true` for robot controllers (ROS 1 requires PID namespace access)

## Deployment

### Helm (Recommended)

```bash
helm install embodied-runtime ./charts/embodied-runtime \
  --namespace rlark-system \
  --set config.ros.enabled=true \
  --set config.camera.enabled=true
```

### Device Plugin Configuration

Configure the device plugin with the devices available on your nodes:

```yaml
# device-plugin-config.yaml
host_devices:
  - name: rlinf.io/device-webcam
    count: 2
    devices:
      - /dev/video0
      - /dev/video1

host_macvlans:
  - name: rlinf.io/device-franka
    count: 1
    parent_interface: eth0
    robot_ip: 192.168.1.100

camera:
  enabled: true
  socket: /var/run/rlark/camera-ctrl.sock

ros:
  enabled: true
  socket: /var/run/rlark/ros-ctrl.sock

ros2:
  enabled: false
  socket: /var/run/rlark/ros2-ctrl.sock
```

### Host Device Passthrough

For simple devices like USB cameras, use `host_devices` to pass through device files directly:

```yaml
host_devices:
  - name: rlinf.io/device-webcam
    count: 1
    devices:
      - /dev/video0
```

### Macvlan for Network Robots

For robots with fixed IP addresses on the network, use `host_macvlans`:

```yaml
host_macvlans:
  - name: rlinf.io/device-franka
    count: 1
    parent_interface: eth0
    robot_ip: 192.168.1.100
```

The mutating webhook automatically injects a `devinit` sidecar that creates the macvlan interface in the Worker container.

### Controller Pods

For full controller pod specs, manager modes, and networking details, see [Embodied Runtime Reference](../developer-guide/embodied-runtime-reference.md).

## Verification

After deployment, verify the Embodied Runtime is working:

```bash
# 1. Check device plugin is running
kubectl get pods -n rlark-system -l app=device-plugin

# 2. Verify device resources are registered
kubectl describe node <node-name> | grep rlinf.io/device

# 3. Test robot discovery (inside a Worker)
rosctr list

# 4. Test camera discovery (inside a Worker)
camctr list
```

## Safety

When deploying with real robots:

- Deploy controllers only on compatible edge clusters
- Validate discovery and allocation with a non-production device first
- Confirm host runtime dependencies and device access permissions
- Use the [safety checklist](../developer-guide/device-integration.md#safety-requirements-for-real-devices) before operating real hardware

## Reference

| Resource | Path |
|----------|------|
| Embodied Runtime Reference | [Full technical reference](../developer-guide/embodied-runtime-reference.md) |
| Embodied Runtime README | `apps/embodied-runtime/README.md` |
| Deployment Examples | `apps/embodied-runtime/docs/examples.md` |
| gRPC API Reference | `apps/embodied-runtime/docs/proto-api.md` |
| Helm Chart | `apps/embodied-runtime/charts/embodied-runtime/` |
| Device Integration Guide | [New Device Integration](../developer-guide/device-integration.md) |