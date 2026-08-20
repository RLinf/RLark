# Configuration Reference

## Database Configuration

PostgreSQL connection configuration loaded via `--db-config` flag. Used by rlark-server, rlark-gateway, and rlark-controller-manager.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `localhost` | PostgreSQL host |
| `port` | int | `5432` | PostgreSQL port |
| `database` | string | `rlark` | Database name |
| `user` | string | `rlark` | Database user |
| `password` | string | `rlark` | Database password |
| `maxOpenConns` | int | `25` | Maximum open connections |
| `maxIdleConns` | int | `5` | Maximum idle connections |
| `connMaxLifetime` | duration | `30m` | Connection maximum lifetime |
| `connMaxIdleTime` | duration | `5m` | Connection maximum idle time |
| `debug` | bool | `false` | Enable query logging |

!!! note "Connection pooling"
    Adjust `maxOpenConns` and `maxIdleConns` based on actual load. Increase for high-concurrency scenarios.

**Example:**
```yaml
host: postgresql
port: 5432
database: rlark
user: rlark
password: CHANGE_ME
maxOpenConns: 25
maxIdleConns: 5
connMaxLifetime: 30m
connMaxIdleTime: 5m
debug: false
```

## rlark-server

Control plane server. Manages TLS/SSH certificates, agent registration, and the Gateway API.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--https-port` | int | `8443` | HTTPS listen port |
| `--ssh-port` | int | `2222` | SSH listen port |
| `--unsafe-http-port` | int | `8888` | Internal HTTP for `/healthz`, `/readyz`, `/livez`, `/metrics`, and peer proxying |
| `--auto-sign-tls-ca-cert` | bool | `false` | Auto-sign TLS CA certificate if not present |
| `--tls-domains` | strings | `["localhost"]` | TLS certificate domain list |
| `--db-config` | string | `""` | Database configuration file path |
| `--peer-service` | string | `""` | Cluster peer service DNS name |
| `--peers` | strings | `[]` | Cluster peer server addresses |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig file path |
| `--master` | string | `""` | Kubernetes API server address |
| `--in-cluster` | bool | `false` | Use in-cluster Kubernetes config |
| `--kube-namespace` | string | `""` | Kubernetes namespace |
| `--kube-qps` | float32 | `5000` | Kubernetes client QPS |
| `--kube-burst` | int | `8000` | Kubernetes client burst |
| `--kube-timeout` | duration | `0` | Kubernetes client request timeout |

!!! tip "`--unsafe-http-port`"
    Agents use this port for certificate signing. In production, only expose to internal networks.

**Example:**
```bash
rlark-server \
  --https-port=8443 \
  --ssh-port=2222 \
  --auto-sign-tls-ca-cert \
  --tls-domains=localhost,rlark.example.com \
  --db-config=/etc/rlark/db-config.yaml
```

## rlark-gateway

API gateway. Handles all REST API requests including cluster management, job management, and storage operations.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--addr` | string | `:8080` | API gateway bind address; `rlarkadm` overrides it to `:8090` |
| `--db-config` | string | `""` | Database configuration file path |
| `--server-address` | string | `https://rlark-server.rlark-system.svc:8443` | RLark server address for certificate signing |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig file path |
| `--master` | string | `""` | Kubernetes API server address |
| `--in-cluster` | bool | `false` | Use in-cluster Kubernetes config |
| `--kube-namespace` | string | `""` | Kubernetes namespace |
| `--kube-qps` | float32 | `5000` | Kubernetes client QPS |
| `--kube-burst` | int | `8000` | Kubernetes client burst |
| `--kube-timeout` | duration | `0` | Kubernetes client request timeout |

**Example:**
```bash
rlark-gateway \
  --addr=:8080 \
  --db-config=/etc/rlark/db-config.yaml \
  --server-address=https://rlark-server:8443
```

## rlark-controller-manager

Controller manager. Reconciles Jobs, Workflows, and Domain resources.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server-address` | string | `https://rlark-server.rlark-system.svc:8443` | RLark server address |
| `--db-config` | string | `""` | Database configuration file path |
| `--leader-elect` | bool | `true` | Enable leader election for HA |
| `--leader-election-id` | string | `rlark-controller-manager` | Leader election identity |
| `--metrics-bind-address` | string | `:8080` | Metrics endpoint bind address |
| `--health-probe-bind-address` | string | `:8081` | Health probe endpoint bind address |
| `--sync-workers` | int | `5` | Number of concurrent sync workers |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig file path |
| `--master` | string | `""` | Kubernetes API server address |
| `--in-cluster` | bool | `false` | Use in-cluster Kubernetes config |
| `--kube-namespace` | string | `""` | Kubernetes namespace |
| `--kube-qps` | float32 | `5000` | Kubernetes client QPS |
| `--kube-burst` | int | `8000` | Kubernetes client burst |
| `--kube-timeout` | duration | `0` | Kubernetes client request timeout |

!!! note "Single-instance deployment"
    Set `--leader-elect=false` for single-instance deployments to avoid unnecessary election overhead.

**Example:**
```bash
rlark-controller-manager \
  --server-address=https://rlark-server:8443 \
  --db-config=/etc/rlark/db-config.yaml \
  --leader-elect=false \
  --metrics-bind-address=:8080 \
  --health-probe-bind-address=:8081
```

## rlark-agent

Data plane agent. Deployed on each cluster or node. Manages node registration, Task execution, and cross-cluster networking.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server-address` | string | `https://localhost:8443` | RLark server address |
| `--server-hostname` | string | `""` | Expected server TLS hostname |
| `--client-cert` | string | `""` | Client TLS certificate path |
| `--client-key` | string | `""` | Client TLS private key path |
| `--ca-cert` | string | `""` | CA certificate path |
| `--insecure-skip-tls-verify` | bool | `false` | Skip TLS certificate verification |
| `--agent-type` | string | `Kubernetes` | Agent type: Kubernetes, Docker, Raw |
| `--mode` | string | `cluster` | Agent mode: cluster, node, both |
| `--leader-election` | bool | `false` | Enable agent leader election |
| `--leader-election-key` | string | `default/rlark-agent` | Leader election key (namespace/name) |
| `--leader-election-id` | string | `hostname-pid` | Leader election identity |
| `--metrics-bind-address` | string | `:8081` | Metrics endpoint bind address |
| `--rlark-server-ssh-address` | string | `""` | RLark server SSH address (user@host:port) |
| `--rlark-server-ssh-host-key` | string | `""` | RLark server SSH host key |
| `--image` | string | `""` | RLark network sidecar image |
| `--enable-same-cluster-direct` | bool | `true` | Enable same-cluster direct Pod access |
| `--enable-cross-cluster-direct` | bool | `true` | Enable cross-cluster direct Pod access |
| `--kubelet-dir` | string | `""` | Kubelet directory for pod UID discovery |
| `--image-pull-enabled` | bool | `true` | Enable image pre-pull |
| `--containerd-socket` | string | `/run/containerd/containerd.sock` | Containerd socket path |
| `--containerd-namespace` | string | `k8s.io` | Containerd namespace |
| `--node-name` | string | `$NODE_NAME` | Local node name |
| `--nodeserver-unix-socket` | string | `/var/run/rlark/nodeserver.sock` | NodeServer Unix socket path |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig file path |
| `--master` | string | `""` | Kubernetes API server address |
| `--in-cluster` | bool | `false` | Use in-cluster Kubernetes config |
| `--kube-namespace` | string | `""` | Kubernetes namespace |
| `--kube-qps` | float32 | `5000` | Kubernetes client QPS |
| `--kube-burst` | int | `8000` | Kubernetes client burst |
| `--kube-timeout` | duration | `0` | Kubernetes client request timeout |

!!! tip "`--mode` values"
    - `cluster`: Cluster-level agent only, manages cluster-wide resources
    - `node`: Node-level agent only, manages Tasks on a single node
    - `both`: Runs both cluster and node-level agents

**Example:**
```bash
rlark-agent \
  --mode=both \
  --server-address=https://rlark-server:8443 \
  --client-cert=/etc/rlark/agent-cert.pem \
  --client-key=/etc/rlark/agent-key.pem \
  --ca-cert=/etc/rlark/ca-cert.pem \
  --image=rlark:latest \
  --node-name=worker-01
```

## rlark-network-sidecar

Network sidecar. Runs alongside each Task Pod to provide cross-cluster Pod-to-Pod networking via TUN device and gVisor netstack.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--sidecar-unix-socket` | string | `/var/run/rlark/nodeserver.sock` | NodeServer Unix socket path |
| `--sidecar-tun-name` | string | `gnet0` | TUN device name |
| `--sidecar-tun-mtu` | int | `1500` | TUN device MTU |
| `--sidecar-proxy-listen` | string | `:5700` | Proxy TCP listen address |
| `--sidecar-hosts-sync-enabled` | bool | `true` | Enable periodic hosts file sync |
| `--sidecar-hosts-sync-interval` | duration | `30s` | Hosts sync interval |
| `--sidecar-hosts-file` | string | `/etc/hosts` | Hosts file path |

**Example:**
```bash
rlark-network-sidecar \
  --sidecar-unix-socket=/var/run/rlark/nodeserver.sock \
  --sidecar-tun-name=gnet0 \
  --sidecar-tun-mtu=1500
```

## sshd

SSH daemon. Provides SSH access to running Task Pods. Integrated into rlark-server via `--ssh-port`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | string | `22` | SSH listen port |
| `--shell` | string | `""` | Shell binary path (default: /bin/bash) |

**Environment variables:**

| Variable | Description |
|----------|-------------|
| `RLARK_SSH_PUBLIC_KEY` | SSH public key for authorized_keys |
| `RLARK_SSH_AUTHORIZED_KEYS_FILE` | Path to authorized_keys file |

## Storage Provider Configuration

Object storage backend configuration used by the gateway.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `accessKeyId` | string | `""` | Access key ID |
| `secretAccessKey` | string | `""` | Secret access key |
| `bucket` | string | `""` | Bucket name |
| `endpoint` | string | `http://localhost:9000` | Storage endpoint |
| `region` | string | `us-east-1` | Region |
| `usePathStyle` | bool | `false` | Use path-style addressing |
| `provider` | string | `""` | Storage provider name |

!!! tip "MinIO vs AWS S3"
    Set `usePathStyle: true` for MinIO. Leave `false` for AWS S3.

## rlarkadm Deploy Configuration

The YAML file passed to `rlarkadm install -f`. See [CLI Reference](cli.md#rlarkadm) for usage.

### Top-level Fields

These names are the exact YAML keys accepted by `rlarkadm`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `apiVersion` | string | — | API version (required); maintained examples use `rlark.io/v1alpha1` |
| `kind` | string | — | Must be `DeployConfig` |
| `plane` | string | — | Required: `control` or `data` |
| `control-plane-address` | string | `""` | Server HTTPS/WSS address; required for the data plane |
| `db` | DBConfig | unset | Database configuration; enables PostgreSQL components in `rlarkadm` deployments |
| `kubernetes` | KubernetesEnv | unset | Kubernetes deployment environment |
| `docker` | DockerEnv | unset | Docker deployment environment |
| `raw` | RawEnv | unset | Raw deployment environment |
| `cert` | CertConfig | unset | Certificate configuration; required for the data plane |
| `insecure-skip-tls-verify` | bool | `false` | Skip Server TLS verification |

!!! note "Environment selection"
    Choose exactly one of `kubernetes`, `docker`, or `raw`. The data plane also requires `control-plane-address` and `cert`.

### DBConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `postgresql` | PostgreSQL host |
| `port` | int | `5432` | PostgreSQL port |
| `database` | string | `rlark` | Database name |
| `user` | string | `rlark` | Database user |
| `password` | string | `rlark` | Database password |

### KubernetesEnv

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `kubeconfig` | string | `""` | kubeconfig file path; an empty value uses the normal client-go loading rules |
| `gateway-image` | string | `""` | Gateway image |
| `controller-manager-image` | string | `""` | Controller Manager image |
| `server-image` | string | `""` | Server image |
| `agent-image` | string | `""` | Agent image |
| `image` | string | `""` | Shared RLark image fallback; on the data plane also enables network sidecar and SSH support |
| `kcp-image` | string | `""` | kcp image |
| `etcd-image` | string | `""` | Built-in etcd image; built-in etcd is enabled only when set and no external address is configured |
| `postgresql-image` | string | `""` | PostgreSQL image; PostgreSQL is enabled only when the top-level `db` block is set |
| `ui-image` | string | `""` | UI image |
| `replicas` | int | `0` (resolved to `1`) | Default component replicas |
| `storage` | StorageConfig | unset | Default storage configuration |
| `kcp` | ComponentConfig | unset | kcp component config |
| `etcd` | EtcdConfig | unset | etcd component config |
| `postgresql` | ComponentConfig | unset | PostgreSQL component config |
| `containerd-socket` | string | `/run/containerd/containerd.sock` | Node Agent containerd socket path |

!!! tip "Image priority"
    A component-specific image such as `gateway-image` takes priority over `image`.

### DockerEnv

| Field | Type | Description |
|-------|------|-------------|
| `gateway-image` | string | Gateway image |
| `controller-manager-image` | string | Controller Manager image |
| `server-image` | string | Server image |
| `agent-image` | string | Agent image |
| `image` | string | Shared RLark image fallback |
| `kcp-image` | string | kcp image |
| `etcd-image` | string | etcd image |
| `postgresql-image` | string | PostgreSQL image |
| `ui-image` | string | UI image |

### RawEnv

!!! warning "Experimental"
    Raw deployment is experimental. Prefer Kubernetes or Docker.

| Field | Type | Description |
|-------|------|-------------|
| `gateway-artifact` | string | Gateway binary path |
| `controller-manager-artifact` | string | Controller Manager binary path |
| `server-artifact` | string | Server binary path |
| `agent-artifact` | string | Agent binary path |
| `network-sidecar-artifact` | string | Network sidecar binary path |
| `kcp-artifact` | string | kcp binary path |
| `etcd-artifact` | string | etcd binary path |
| `postgresql-artifact` | string | PostgreSQL binary path |

### CertConfig

Required for data plane deployment.

| Field | Type | Description |
|-------|------|-------------|
| `ca-cert` | string | Inline CA PEM or an existing file path |
| `agent-cert` | string | Inline Agent certificate PEM or an existing file path |
| `agent-key` | string | Inline Agent private key PEM or an existing file path |

### StorageConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | | Storage type: emptyDir, hostPath, pvc |
| `host-path` | string | `""` | Host path for hostPath type |
| `storage-class` | string | `""` | StorageClass for PVC type; empty uses the cluster default |
| `size` | string | `""` | Storage size |
| `node-selector` | map | empty | Node selector for the workload |

### ComponentConfig

| Field | Type | Description |
|-------|------|-------------|
| `replicas` | int | Number of replicas |
| `storage` | StorageConfig | Storage configuration |

### EtcdConfig

| Field | Type | Description |
|-------|------|-------------|
| `address` | string | etcd address |
| `replicas` | int | Number of replicas |
| `storage` | StorageConfig | Storage configuration |

**Example (control plane):**
```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  postgresql-image: postgres:15
  ui-image: rlark-ui:latest
  replicas: 1
db:
  host: postgresql
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

**Example (data plane):**
```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://rlark.example.com:8443
cert:
  ca-cert: /path/to/ca-cert.pem
  agent-cert: /path/to/agent-cert.pem
  agent-key: /path/to/agent-key.pem
kubernetes:
  kubeconfig: ~/.kube/config
  agent-image: rlark:latest
  image: rlark:latest
```
