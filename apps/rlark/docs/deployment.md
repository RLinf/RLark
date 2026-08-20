# Deployment Guide

## 1. Deployment Architecture

RLark supports three deployment modes for different scenarios:

| Mode | Complexity | Use Case |
|------|-----------|----------|
| Docker Compose | Low | Local development, single-machine testing |
| Kubernetes | Medium | Production, cluster deployment |
| Raw Binary | High | Minimal deployment, embedded scenarios |

## 2. Deployment Tool: rlarkadm

`rlarkadm` is RLark's deployment CLI for installing and uninstalling control-plane and data-plane components. Installation waits up to 180 seconds for each Kubernetes workload to become ready; there is no separate `health` subcommand.

```bash
# Install
rlarkadm install -f <config-file>

# Uninstall
rlarkadm uninstall -f <config-file>
```

### Configuration File Structure

A file describes exactly one plane and exactly one environment (`kubernetes`, `docker`, or `raw`). Start from the maintained examples instead of combining both planes in one YAML document:

```yaml
# Control plane
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  kubeconfig: /path/to/control-kubeconfig
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  ui-image: rlark-ui:latest
  storage:
    type: pvc
    storage-class: ""
    size: 10Gi
```

```yaml
# Data plane
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://rlark.example.com:8443
cert:
  ca-cert: |                # Inline PEM or an existing file path
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  agent-cert: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  agent-key: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
kubernetes:
  kubeconfig: /path/to/data-kubeconfig
  agent-image: rlark:latest
  image: rlark:latest       # Optional; enables Pod networking and SSH support
```

## 3. Kubernetes Deployment

### 3.1 Control Plane Deployment

```bash
# 1. Prepare configuration file
cp apps/rlark/docs/examples/deploy-control-plane.yaml my-deploy.yaml

# 2. Modify image addresses and kubeconfig
# 3. Execute installation
rlarkadm install -f my-deploy.yaml

# 4. Verify
kubectl get pods -n rlark-system
```

Control plane components:

| Component | Replicas | Ports | Description |
|-----------|----------|-------|-------------|
| kcp | 1 | 6443 | API Server |
| etcd | 1 | 2379 | kcp storage; deployed only with `etcd-image` and no external address |
| postgresql | 1 | 5432 | RLark storage; deployed only when the top-level `db` block is set |
| server | 1 | 8443, 2222, 8888 | HTTPS, SSH, and health/metrics HTTP |
| controller-manager | 1 | 8080, 8081 | Metrics and health probes |
| gateway | 1 | 8090 | API Gateway in an `rlarkadm` deployment |
| ui | 1 | 80 | Web UI; proxies `/api/` to Gateway |

### 3.2 Data Plane Deployment

```bash
# 1. Obtain certificates from control plane (via Gateway API or manual signing)
# 2. Fill in configuration file
cp apps/rlark/docs/examples/deploy-data-plane.yaml my-data.yaml

# 3. Execute installation
rlarkadm install -f my-data.yaml

# 4. Verify
kubectl get pods -n rlark-system
```

Data plane components:

| Component | Replicas | Description |
|-----------|----------|-------------|
| agent | Deployment | Cluster-level synchronization (`--mode=cluster`) |
| agent-node | DaemonSet | Node networking and image pre-pull (`--mode=node`) |
| network-sidecar | Injected container | Added to eligible training Pods when `kubernetes.image` is set |

## 4. Docker Compose Deployment (Development)

Suitable for local development and testing:

```yaml
# docker-compose.yml
services:
  kcp:
    image: kcp:v0.30.0
    command: ["start", "--root-directory", "/data/kcp"]
    ports:
      - "6443:6443"
    volumes:
      - kcp-data:/data

  postgresql:
    image: postgres:15
    environment:
      POSTGRES_DB: rlark
      POSTGRES_USER: rlark
      POSTGRES_PASSWORD: CHANGE_ME
    ports:
      - "5432:5432"
    volumes:
      - pg-data:/var/lib/postgresql/data

volumes:
  kcp-data:
  pg-data:
```

## 5. Component Configuration

### 5.1 Server

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` | Control plane kubeconfig |
| `--https-port` | `8443` | HTTPS service port |
| `--ssh-port` | `2222` | SSH service port |
| `--unsafe-http-port` | `8888` | Unauthenticated HTTP port for `/healthz`, `/readyz`, and metrics |
| `--db-config` | `""` | Database config file path |
| `--auto-sign-tls-ca-cert` | `false` | Generate the TLS CA and server certificate in Kubernetes when absent |
| `--tls-domains` | `localhost` | Comma-separated DNS names in the generated server certificate |

### 5.2 Controller-Manager

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` | Control plane kubeconfig |
| `--server-address` | `https://rlark-server.rlark-system.svc:8443` | Server address |
| `--leader-elect` | `true` | Enable leader election |
| `--metrics-bind-address` | `:8080` | Metrics bind address |
| `--health-probe-bind-address` | `:8081` | `/healthz` and `/readyz` bind address |

### 5.3 Gateway

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` | Control plane kubeconfig |
| `--addr` | `:8080` | Standalone default; `rlarkadm` overrides it to `:8090` |
| `--db-config` | `""` | Database config file path |
| `--server-address` | `https://rlark-server.rlark-system.svc:8443` | Server address for certificate signing |

### 5.4 Agent

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` | Data plane kubeconfig |
| `--server-address` | `https://localhost:8443` | Control plane Server address |
| `--client-cert` | `""` | Agent certificate path |
| `--client-key` | `""` | Agent private key path |
| `--ca-cert` | `""` | CA certificate path |
| `--mode` | `cluster` | Run mode: `cluster` / `node` / `both` |
| `--rlark-server-ssh-address` | `""` | Server SSH address (`user@host:port`) |
| `--rlark-server-ssh-host-key` | `""` | Server SSH host public key |
| `--agent-type` | `Kubernetes` | Agent type: `Kubernetes` / `Docker` / `Raw` |
| `--leader-election` | `false` | `rlarkadm` enables it for multi-replica cluster agents |
| `--image` | `""` | RLark container image (network sidecar, SSH server, etc.) |
| `--in-cluster` | `false` | `rlarkadm` sets it in Kubernetes mode |
| `--insecure-skip-tls-verify` | `false` | Skip TLS certificate verification |

### 5.5 Env Mode

RLark supports three deployment environment modes, configured in the DeployConfig YAML:

| Mode | Description | Use Case |
|------|-------------|----------|
| `kubernetes` | Deploy to K8s cluster | Production, GPU cluster |
| `docker` | Manage via Docker API (TODO) | Edge devices, single-node |
| `raw` | Download artifact and execute (TODO) | Bare metal, embedded |

In the config file, specify exactly one of `kubernetes`, `docker`, or `raw` env blocks.

### 5.6 Agent Modes

Agent supports three running modes via `--mode`:

| Mode | Description | Component |
|------|-------------|-----------|
| `cluster` | Cluster-level workload management | Deployment (agent) |
| `node` | Node-level network operations | DaemonSet (agent-node) |
| `both` | Run both cluster and node controllers | Combined |

When `--mode=node`, the agent runs as a DaemonSet on each node, handling SSH tunnel setup for cross-cluster networking.

## 6. Addon Configuration

rlark supports declarative Addon management across data plane clusters. Addons are defined in the addon catalog (`../pkg/addons/catalog/`) and installed via the Addon API.

### 6.1 Addon Catalog Structure

```
pkg/addons/catalog/
├── embodied-runtime-device-plugin/
│   ├── addon.yaml          # Addon metadata (name, version, description, configurable parameters)
│   └── manifests/
│       ├── daemonset.yaml  # K8s DaemonSet with template values
│       ├── configmap-template.yaml  # ConfigMap template (camera/ROS controller configs)
│       ├── headless-services.yaml   # Headless Services for camera/ROS controllers
│       └── rbac.yaml       # ClusterRole + ClusterRoleBinding
└── csi-driver-rclone/
    ├── addon.yaml          # Addon metadata (name, version, category: storage)
    └── manifests/
        ├── controller.yaml  # CSI Controller Deployment
        ├── node.yaml        # CSI Node DaemonSet
        ├── configmap.yaml   # RClone configuration
        ├── csidriver.yaml   # CSIDriver resource
        └── rbac.yaml        # RBAC permissions
```

Key configurable parameters for `embodied-runtime-device-plugin`:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image` | Device plugin container image | `rlark/embodied-device-plugin:0.1.0` |
| `rendererImage` | Node-level config renderer initContainer image (yq) | `yq:4.53.2` |
| `cameraImage` | Camera controller container image | — |
| `rosImage` | ROS controller container image | — |
| `nodeSelector` | Node selector for DaemonSet scheduling | `nvidia.com/gpu=true` |
| `robotTolerationKey` | Toleration key for robot nodes | — |

The addon also deploys two headless Services (`camera-controller-headless` and `ros-controller-headless`) for stable DNS-based discovery of camera and ROS controllers within the cluster.

Key configurable parameters for `csi-driver-rclone`:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rcloneImage` | RClone CSI driver container image | `csi-driver-rclone:v0.2.0` |
| `csiProvisionerImage` | CSI provisioner sidecar image | `csi-provisioner:v6.2.0` |
| `livenessProbeImage` | Liveness probe sidecar image | `livenessprobe:v2.18.0` |
| `nodeDriverRegistrarImage` | Node driver registrar sidecar image | `csi-node-driver-registrar:v2.16.0` |
| `driverName` | CSI driver registration name | `rclone.csi.veloxpack.io` |
| `controllerReplicas` | Controller Deployment replicas | `1` |
| `controllerLogLevel` | Controller log level (0-10) | `5` |
| `nodeLogLevel` | Node DaemonSet log level (0-10) | `5` |

The RClone CSI driver enables dynamic provisioning of PersistentVolumes backed by remote storage (S3, GCS, Azure Blob, etc.) via RClone.

### 6.2 Installing an Addon

```bash
# Install an addon to a specific cluster
curl -X POST "http://localhost:8080/api/v1/clusters/agent-beijing/addons" \
  -H "Content-Type: application/json" \
  -d '{
    "addonName": "embodied-runtime-device-plugin",
    "version": "0.1.0",
    "values": {
      "image": "rlark/embodied-device-plugin:0.1.0"
    }
  }'
```

### 6.3 Managing Addons

```bash
# List available addons in catalog
curl "http://localhost:8080/api/v1/addons"

# List installed addons across all clusters
curl "http://localhost:8080/api/v1/installed-addons"

# List addons in a specific cluster
curl "http://localhost:8080/api/v1/clusters/agent-beijing/addons"

# Get addon details
curl "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin"

# Update addon configuration
curl -X PUT "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin" \
  -H "Content-Type: application/json" \
  -d '{
    "values": {
      "image": "rlark/embodied-device-plugin:0.2.0"
    }
  }'

# Uninstall an addon
curl -X DELETE "http://localhost:8080/api/v1/clusters/agent-beijing/addons/embodied-device-plugin"
```

## 7. Storage Configuration

### 7.1 Control Plane Storage

| Component | Data Content | Recommended Size |
|-----------|-------------|-----------------|
| kcp | All CRD objects | 10Gi |
| etcd | kcp metadata | 8Gi |
| postgresql | SSH keys, user data | 30Gi |

### 7.2 Storage Types

| Type | Description | Use Case |
|------|-------------|----------|
| `emptyDir` | Ephemeral storage, lost on Pod deletion | Testing |
| `hostPath` | Node local path | Single-node deployment |
| `pvc` | Persistent volume | Production |

## 8. Network Requirements

### 8.1 Port Exposure

| Component | Port | Protocol | Description |
|-----------|------|----------|-------------|
| UI | 80 | HTTP | Browser entry; proxies `/api/` to Gateway |
| Gateway | 8090 | HTTP | Internal API in `rlarkadm` (`8080` standalone default) |
| Server | 8443 | HTTPS/WSS | Agent tunnels, proxying, and certificates |
| Server | 2222 | SSH | User and cross-cluster SSH |
| Server | 8888 | HTTP | Internal health and metrics |
| kcp | 6443 | HTTPS | Internal control plane API |

### 8.2 Network Topology

```text
User / browser ──HTTP──▶ UI (:80) ──/api proxy──▶ Gateway (:8090)
User SSH client ───────▶ Server (:2222)
                                      ▲
Data plane cluster ──outbound WSS─────┤ Server (:8443)
  ├─ agent Deployment (cluster mode)  │
  └─ agent-node DaemonSet (node mode) ┘

Server / Gateway / Controller Manager ──HTTPS──▶ kcp (:6443)
```

Both data-plane agents initiate outbound connections to Server, so no inbound data-plane port is required. Expose UI port 80 and, when needed, Server ports 8443 and 2222; keep Gateway, kcp, and health/metrics ports internal.

## 9. Certificate Management

### 9.1 Certificate Types

| Certificate | Purpose | Issuance |
|-------------|---------|----------|
| CA Certificate | Root certificate, signs other certificates | Generated at deployment |
| Agent Certificate | Agent connects to control plane | Admin issues via Gateway API |
| Domain Certificate | Cross-cluster Pod communication | Controller-Manager auto-issues |
| SSH Certificate | User SSH login | Server issues on-the-fly during authentication |

### 9.2 Issuing Agent Certificates

```bash
# UI proxies /api/ to Gateway.
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80

curl -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"cluster_id": "beijing"}'
```

Returns certificate and private key for deployment to the data plane Agent.

### 9.3 UI Authentication

During deployment, `rlarkadm` automatically creates a `rlark-ui-auth` Secret in the kcp cluster's `default` namespace, containing randomly generated passwords for admin and user roles:

| Key | Purpose |
|-----|---------|
| `admin-password` | Admin role password (16 random characters) |
| `user-password` | User role password (16 random characters) |

Passwords are displayed in the install summary. The web UI uses `POST /api/v1/auth/login` to authenticate.

## 10. Production Deployment and High Availability

### 10.1 Current `rlarkadm` Scope

The maintained `rlarkadm` example deploys one replica of each enabled control-plane component. Although the configuration accepts global and component-level `replicas`, RLark does not currently document or validate a production HA topology for Gateway, Server, kcp, etcd, or PostgreSQL. Increasing replica counts alone must not be assumed to provide high availability.

For production, keep the maintained single-replica topology unless you have independently designed and tested the component topology, shared state, traffic routing, failure recovery, and storage behavior. Use externally managed highly available data services where required; `rlarkadm` does not configure PostgreSQL primary/standby replication.

### 10.2 External etcd

```yaml
kubernetes:
  etcd:
    address: https://my-etcd.example.com:2379
    # Do not deploy built-in etcd
```

### 10.3 External PostgreSQL

```yaml
db:
  host: pg-managed.example.com
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

## 11. Monitoring & Operations

### 11.1 Prometheus Metrics

Gateway and Server expose Prometheus metrics:

- `rlark_gateway_requests_total`: Gateway request count
- `rlark_gateway_request_duration_seconds`: Request latency
- `rlark_proxy_requests_total`: Server proxy request count
- `rlark_peer_connections`: Current peer connections
- `rlark_ssh_connections_total`: SSH connection count

### 11.2 Logging

Structured logging (zap), controlled via `LOG_LEVEL` environment variable:

```bash
LOG_LEVEL=debug ./bin/server --kubeconfig ...
```

### 11.3 Health Checks

`rlarkadm install` waits for each Deployment, StatefulSet, and DaemonSet to report all desired replicas ready. For later checks:

```bash
kubectl get deploy,statefulset,daemonset -n rlark-system
kubectl rollout status deployment/rlark-server -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system

# Server health (normally cluster-internal)
kubectl port-forward -n rlark-system svc/rlark-server 8888:8888
curl --fail http://localhost:8888/healthz
curl --fail http://localhost:8888/readyz

# Controller Manager health
kubectl port-forward -n rlark-system deployment/rlark-controller-manager 8081:8081
curl --fail http://localhost:8081/healthz
curl --fail http://localhost:8081/readyz
```

Gateway and Agent expose metrics but do not define dedicated HTTP health routes; use Kubernetes workload readiness and logs for them.

## 12. Upgrades

```bash
# 1. Update image versions
# 2. Rolling update (Kubernetes native support)
kubectl rollout restart deployment -n rlark-system

# 3. Verify
kubectl get pods -n rlark-system
```

## 13. Troubleshooting

### Agent Cannot Connect to Server

1. Check if Agent certificate is valid (not expired, signed by correct CA)
2. Check network connectivity: `curl -k https://<server>:8443`
3. Check Server logs: `kubectl logs -n rlark-system deployment/server`

### Training Job Cannot Start

1. Check if Node has sufficient resources: `kubectl get nodes -n rlark-system`
2. Check Task status: query the corresponding Task CR
3. Check Agent logs: `kubectl logs -n rlark-system daemonset/agent`

### Cross-Cluster Network Not Working

1. Check if DomainPeer has been created
2. Check if Domain certificate was signed successfully
3. Check if network-sidecar is injected into the Pod