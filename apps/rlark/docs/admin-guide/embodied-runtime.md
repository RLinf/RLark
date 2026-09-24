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
| Webhook | Mutating Webhook | Optionally injects a `devinit` init container for macvlan networking |

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

Direct deployment with the Embodied Runtime Helm chart supports ROS 2 through `config.ros2`. The RLark addon catalog currently does not expose ROS 2 configuration, so use the chart directly when ROS 2 is required.

```bash
helm install embodied-runtime ./apps/embodied-runtime/charts/embodied-runtime \
  --namespace rlark-system \
  --create-namespace \
  --set config.ros.enabled=true \
  --set config.camera.enabled=true
```

### Device Plugin Configuration

Configure the device plugin with the devices available on your nodes:

```yaml
# device-plugin-config.yaml
device_count: 1

host_devices:
  - host_path: /dev/video0
  - host_path: /dev/ttyUSB0
    permissions: rw

host_macvlans:
  - host_nic: eno1
    name: macvlan0
    ip: 172.16.0.0/24
    # gateway: 172.16.0.1

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
  - host_path: /dev/video0
    # container_path: /dev/video0
    # permissions: rwm
```

### Macvlan for Network Robots

For robots with fixed IP addresses on the network, use `host_macvlans`:

```yaml
host_macvlans:
  - host_nic: eno1
    name: macvlan0
    ip: 172.16.0.0/24
    # gateway: 172.16.0.1
```

The webhook is disabled by default. Set `webhook.enabled=true` and provide a non-empty `config.hostMacvlans` list to render it. It then injects a `devinit` init container that creates the macvlan interface in the Worker pod's network namespace.

### Controller Pods

For full controller pod specs, manager modes, and networking details, see [Embodied Runtime Reference](../developer-guide/embodied-runtime-reference.md).

## Verification

After deployment, verify the Embodied Runtime is working:

```bash
# 1. Check device plugin is running
kubectl get pods -n rlark-system -l app.kubernetes.io/name=embodied-runtime,app.kubernetes.io/component=device-plugin

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