# Changelog

All notable changes to RLark will be documented in this file.

This file is maintained by hand. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-11-25

First public release of RLark, a cross-cluster embodied intelligence cloud-native platform. RLark spans
cloud GPU training, cross-cluster collaboration, and edge device deployment through a single
Kubernetes-compatible management API.

### Added

#### Control plane

- **Server** — central hub for the control plane. Serves an HTTPS API, terminates data-plane Agent tunnels
  over WebSocket (remotedialer) and acts as a reverse proxy to Agent-local services, runs an SSH server with
  two-phase user authentication and `direct-tcpip` channel forwarding, proxies Kubernetes API requests to
  data-plane clusters through Agent tunnels, manages X.509 and SSH certificate issuance and revocation for
  certificate roles such as `agent`, `domain`, and `ssh-guest`, and maintains an Informer-backed in-memory
  Pod cache for SSH Pod lookup. Server-to-Server peer interconnection is included.
- **Gateway** — user-facing REST API gateway. Exposes CRD create/read/update/patch/delete operations under
  `/api/v1/rlinf.io/v1alpha1/{resource}`, together with endpoints including agent certificate issuance, SSH
  user key management, storage and cluster endpoints, Job/Pod log retrieval and streaming, and Prometheus
  metrics middleware.
- **Controller Manager** — runs the control-plane reconcilers, including Job (splits a Job into Tasks and
  drives the `Pending → Running → Succeeded/Failed` state machine), Task, Node, Domain (allocates virtual IP
  subnets and signs DomainPeer certificates), Workflow, and Cluster, together with cross-namespace
  replication, archive, and optional persistence synchronization. Leader election is supported.
- **Web UI** — React + TypeScript + Vite management interface for Domains, Nodes, Jobs, Tasks, Workflows,
  storage, and log viewing. Shipped as an Nginx image that serves the static bundle and proxies `/api/`
  to Gateway.

#### Data plane Agent

A single `agent` binary whose role is selected with `--mode`:

- **`cluster` mode** — reconciles control-plane Tasks into local Kubernetes workloads (Deployment and
  StatefulSet carriers are selected by the Task's workload declaration; DaemonSet delivery is used for
  Addon/delivery resources), with finalizer-protected, UID-fenced, idempotent apply/delete planning and
  recovery of claimed workloads across restarts. Synchronizes Addon and delivery resources, and pushes
  observed Pod, workload, and Node state (addresses, capacity, GPU count) back to the management plane.
- **`node` mode** — runs NodeServer for cross-cluster Pod traffic routing, node-level network support,
  image pre-pull into the node container runtime with event reporting, and health and metrics endpoints.

#### Cross-cluster Pod networking

- A privileged **network sidecar**, injected as a restartable init container into the Pod template authorized
  by the selected Workload Adapter, creates a TUN device and a gVisor netstack to intercept Pod outbound
  traffic, and exposes an inbound proxy listener on port `5700`. When
  `KUBERNETES_SUPPORT_SIDECAR_FEATURE` is `false`, restartable init containers are lowered to regular
  containers. Missing trusted image configuration or conflicting container/volume declarations fail closed
  before any runtime effect.
- **NodeServer** makes routing decisions from Domain and DomainPeer information: same-cluster targets use a
  direct TCP connection to the target Pod's sidecar proxy; cross-cluster targets are relayed through a
  multiplexed SSH connection to the control-plane Server and on to the target Agent's NodeServer.
- **SSH dialer manager** with one shared multiplexed transport per Domain, least-loaded channel placement,
  bounded handshakes, exponential reconnect backoff (1s → 30s), keepalive-aware draining, generation-based
  transport invalidation on credential or endpoint change, and background GC of idle connections.

#### Core API resources

Kubernetes-compatible management API in group/version `rlinf.io/v1alpha1`, with generated typed clients
(`api/kubeclients`) and CRD manifests. The main resources include **Addon**, **Cluster**, **Domain**,
**DomainPeer**, **Job**, **Node**, **Pod**, **Task**, and **Workflow**. This list is not the complete set of
registered types; treat `api/rlark.io/v1alpha1/groupversion_info.go` as the authoritative registered type
list.

#### embodied-runtime

- Node-level **Device Plugin** that registers the `rlinf.io/device` extended resource with kubelet, detects
  node-local robot and camera hardware, and injects Unix sockets and CLI binaries into Task Pods on Allocate.
- **Mutating webhook** that injects a `devinit` init container into Pods requesting `rlinf.io/device`, with
  CA and serving certificate management.
- **ros-controller** (ROS 1 `roscore` + `roslaunch`), **ros2-controller**, and **camera-controller**
  (V4L2 / RTSP / RealSense with ffmpeg transcoding), each exposing a gRPC API over a Unix socket and able to
  run as a local subprocess, a dedicated Pod, or stay disabled.
- **`rosctr`** and **`camctr`** CLI tools mounted into Task Pods for direct robot and camera control.

#### SDKs and protocol definitions

- **Go SDK** (`sdks/embodied-runtime-go`) — generated gRPC stubs for the embodied-runtime services.
- **Python SDK** (`sdks/embodied-runtime-python`) — `RobotClient` and `CameraClient` over the generated
  gRPC stubs.
- **Go execution SDK** (`sdks/execution-go`) — generated stubs for the versioned Runtime Provider and
  Workload Adapter contracts.
- Protobuf definitions under `proto/` with a pinned generator toolchain so checked-in stubs stay reproducible.

#### Deployment tooling

- **`rlarkadm`** installs and removes the control plane and the data plane from a declarative deploy config:

  ```bash
  rlarkadm install -f apps/rlark/docs/examples/deploy-control-plane.yaml
  rlarkadm install -f apps/rlark/docs/examples/deploy-data-plane.yaml
  rlarkadm uninstall -f <deploy-config>
  ```

- A quickstart script (`apps/rlark/docs/examples/quickstart.sh`) brings up a Docker Compose control plane,
  two kind data-plane clusters, and a cross-cluster network check.

#### Documentation

- English and Chinese documentation covering overview, core concepts, technical architecture, quickstart,
  deployment, user and admin guides, developer guide, storage API, and generated CRD API reference.

### Known limitations and supported scope

Please read this section before planning a production deployment. Capabilities not listed here as available
must not be assumed to be supported.

- **The Kubernetes workload runtime is at preview maturity.** It is the only implemented production workload
  path. Treat its lifecycle, failure, and recovery semantics as subject to change and validate them against
  your own workloads.
- **The Docker and Raw workload runtimes are not implemented.** An `--agent-type` flag that names the
  Kubernetes, Docker, and Raw types, a deliberately unconstrained `Task.spec.runtime.kind` string, and
  deployment scaffolding all exist, but no Docker or Raw runtime path is implemented. Do not infer support
  from the presence of a type, a flag, or a deployment option. The Agent rejects unknown or unsupported
  runtime kinds before producing any external effect.
- **Versioned Runtime Provider and Workload Adapter gRPC contracts are not a supported extension point
  yet.** They exist for future external implementations, but the default Task controller does not select
  remote implementations; only the in-process Kubernetes implementation is used.
- **Agent modes.** `--mode` selects the `cluster` or the `node` role. There is no `managed` mode.
- **`pvcStorageMap` and `pvcSizeGbMap` are not part of the canonical Task schema.** For Kubernetes Tasks, use
  the Pod template together with Kubernetes-native volume APIs such as
  `ephemeral.volumeClaimTemplate`. Resources that depend on those removed fields must be replaced during a
  breaking upgrade; they must not be used in new APIs or new examples.
- **The cross-cluster SSH relay exists to avoid requiring direct inbound reachability between data-plane
  clusters.** It is not a general-purpose NAT traversal mechanism, and it must not be assumed to be available
  for every runtime or every workload.
- **Domain is a logical grouping and addressing boundary.** Pods in the same Domain share a virtual address
  space and a cross-cluster forwarding scope. A Domain does not by itself constitute a complete security
  isolation boundary, and it does not isolate the underlying clusters, nodes, runtimes, storage, or ordinary
  Kubernetes networking. Use your infrastructure's network policies and security controls where isolation is
  required.
- **Authorization and tenancy are incomplete.** Per-Pod authorization for the `ssh-guest` certificate role is
  not implemented, and Gateway-side certificate revocation for the `admin` role is not implemented. More
  complete account, role, and permission management is planned.
- **The API is pre-1.0.** `rlinf.io/v1alpha1` may change incompatibly in any release before 1.0, including
  field removals and changed status or condition semantics. Pin versions and review this file before
  upgrading.

### Installation

#### Binary archives

Each archive contains the seven `rlark` binaries: `server`, `controller-manager`, `gateway`, `agent`,
`network-sidecar`, `rlarkadm`, and `rlark-tools`.

| Archive                        | Platform      |
| ------------------------------ | ------------- |
| `rlark_linux_amd64.tar.gz`     | Linux x86_64  |
| `rlark_linux_arm64.tar.gz`     | Linux arm64   |
| `rlark_darwin_amd64.tar.gz`    | macOS x86_64  |
| `rlark_darwin_arm64.tar.gz`    | macOS arm64   |

#### Verifying downloads

The release includes a `checksums.txt` file. Download it next to the archives and verify them:

```bash
# Linux
sha256sum -c checksums.txt

# macOS
shasum -a 256 -c checksums.txt
```

#### Container images

```text
ghcr.io/rlinf/rlark:v0.1.0
ghcr.io/rlinf/rlark-ui:v0.1.0
ghcr.io/rlinf/embodied-runtime:v0.1.0
```

Each image manifest includes `linux/amd64` and `linux/arm64`.

#### Not included in this release

- No separately published Helm chart artifact. Use `rlarkadm` with a deploy config, or the example manifests
  under `apps/rlark/docs/examples/`.
- No standalone binary archive for embodied-runtime. Consume it through the
  `ghcr.io/rlinf/embodied-runtime:v0.1.0` container image.
