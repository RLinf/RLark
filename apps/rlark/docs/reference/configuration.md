# Configuration Reference

## Database Configuration

PostgreSQL connection configuration loaded via `--db-config` flag. Used by rlark-server, rlark-gateway, and rlark-controller-manager.

| Field | Type | Default | Description |
| ------- | ------ | --------- | ------------- |
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
| ------ | ------ | --------- | ------------- |
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
| ------ | ------ | --------- | ------------- |
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

Controller manager. Reconciles Jobs and Domain resources.

| Flag | Type | Default | Description |
| ------ | ------ | --------- | ------------- |
| `--server-address` | string | `https://rlark-server.rlark-system.svc:8443` | RLark server address |
| `--db-config` | string | `""` | Database configuration file path |
| `--leader-election` | bool | `true` | Enable leader election for HA |
| `--leader-election-key` | string | `rlark-controller-manager` | Leader election lock key (`name` or `namespace/name`) |
| `--leader-election-id` | string | `""` | Reserved participant identity; controller-runtime currently generates its own identity |
| `--metrics-bind-address` | string | `:8080` | Metrics endpoint bind address |
| `--health-probe-bind-address` | string | `:8081` | Health probe endpoint bind address |
| `--job-controller-workers` | int | `8` | Maximum concurrent Job reconciles |
| `--task-controller-workers` | int | `8` | Maximum concurrent Task reconciles |
| `--workflow-controller-workers` | int | `8` | Maximum concurrent Workflow reconciles |
| `--node-controller-workers` | int | `8` | Maximum concurrent Node reconciles |
| `--domain-controller-workers` | int | `8` | Maximum concurrent Domain reconciles |
| `--job-sync-controller-workers` | int | `8` | Maximum concurrent Job database sync reconciles |
| `--task-sync-controller-workers` | int | `8` | Maximum concurrent Task database sync reconciles |
| `--workflow-sync-controller-workers` | int | `8` | Maximum concurrent Workflow database sync reconciles |
| `--node-sync-controller-workers` | int | `8` | Maximum concurrent Node database sync reconciles |
| `--kubeconfig` | string | `$KUBECONFIG` | kubeconfig file path |
| `--master` | string | `""` | Kubernetes API server address |
| `--in-cluster` | bool | `false` | Use in-cluster Kubernetes config |
| `--kube-namespace` | string | `""` | Kubernetes namespace |
| `--kube-qps` | float32 | `5000` | Kubernetes client QPS |
| `--kube-burst` | int | `8000` | Kubernetes client burst |
| `--kube-timeout` | duration | `0` | Kubernetes client request timeout |

!!! note "Single-instance deployment"
    Set `--leader-election=false` for single-instance deployments to avoid unnecessary election overhead.

**Example:**

```bash
rlark-controller-manager \
  --server-address=https://rlark-server:8443 \
  --db-config=/etc/rlark/db-config.yaml \
  --leader-election=false \
  --metrics-bind-address=:8080 \
  --health-probe-bind-address=:8081
```

## rlark-agent

Data plane agent. Deployed on each cluster or node. Manages node registration, Task execution, and cross-cluster networking.

| Flag | Type | Default | Description |
| ------ | ------ | --------- | ------------- |
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
| `--task-pull-controller-workers` | int | `8` | Maximum concurrent Task pull reconciles |
| `--addon-pull-controller-workers` | int | `8` | Maximum concurrent Addon pull reconciles |
| `--task-deployment-push-controller-workers` | int | `8` | Maximum concurrent Task Deployment push reconciles |
| `--task-daemonset-push-controller-workers` | int | `8` | Maximum concurrent Task DaemonSet push reconciles |
| `--task-statefulset-push-controller-workers` | int | `8` | Maximum concurrent Task StatefulSet push reconciles |
| `--node-push-controller-workers` | int | `8` | Maximum concurrent Node push reconciles |
| `--pod-push-controller-workers` | int | `8` | Maximum concurrent Pod push reconciles |
| `--pod-orphan-sweep-interval` | duration | `5m` | Interval between agent-scoped management Pod orphan sweeps |
| `--pod-orphan-sweep-page-size` | int | `200` | Management Pods processed per orphan sweep page |
| `--pod-stale-ttl` | duration | `15m` | Time a missing local Pod is retained as `Unknown`/stale before its management Pod is deleted |

The Pod orphan sweep is a fallback for missed local delete events. It deletes only agent-scoped mirrors whose local Pod UID or verified management Task UID is no longer current. Legacy mirrors are adopted only when the UID-named mirror, live local Pod annotations, management namespace, Task UID, and available domain all agree; ambiguous legacy objects remain untouched and require manual cleanup. A delayed delete intentionally preserves a same-name replacement, so stale mirrors may remain until the next sweep interval.
| `--rlark-server-ssh-address` | string | `""` | RLark server SSH address (user@host:port) |
| `--rlark-server-ssh-host-key` | string | `""` | RLark server SSH host key |
| `--ssh-max-connections-per-domain` | int | `4` | Maximum adaptive physical SSH connections per Domain |
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
| ------ | ------ | --------- | ------------- |
| `--sidecar-unix-socket` | string | `/var/run/rlark/nodeserver.sock` | NodeServer Unix socket path |
| `--sidecar-tun-name` | string | `gnet0` | TUN device name |
| `--sidecar-tun-mtu` | int | `1500` | TUN device MTU |
| `--sidecar-proxy-listen` | string | `:5700` | Proxy TCP listen address |
| `--sidecar-metrics-listen` | string | `:5790` | Metrics and pprof HTTP listen address; set to an empty value to disable |
| `--sidecar-hosts-sync-enabled` | bool | `true` | Enable hosts file synchronization |
| `--sidecar-hosts-sync-interval` | duration | `30s` | Fallback polling interval for NodeServers without the hosts watch API |
| `--sidecar-hosts-file` | string | `/etc/hosts` | Hosts file path |

New sidecars use the NodeServer `/watch_hosts` long-poll endpoint to receive host changes within approximately one second. If the endpoint is unavailable, they automatically fall back to the configured polling interval, preserving compatibility with older NodeServers.

**Example:**

```bash
rlark-network-sidecar \
  --sidecar-unix-socket=/var/run/rlark/nodeserver.sock \
  --sidecar-tun-name=gnet0 \
  --sidecar-tun-mtu=1500
```

## rlark-tools sshd

The `sshd` subcommand provides SSH access to running Task Pods. The agent always injects the `rlark-tools` binary at `/rlark-tools/rlark-tools`; workloads that need SSH start it with `rlark-tools sshd`.

| Flag | Type | Default | Description |
| ------ | ------ | --------- | ------------- |
| `--port` | string | `22` | SSH listen port |
| `--shell` | string | `""` | Shell binary path (default: /bin/bash) |

**Environment variables:**

| Variable | Description |
| ---------- | ------------- |
| `RLARK_SSH_PUBLIC_KEY` | SSH public key for authorized_keys |

The Agent environment variable `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES=true` enables the legacy task mode that grants all task containers privileged access and enables host networking for tasks other than Ray heads. It is disabled by default and should only be used in trusted clusters.

## Storage Provider Configuration

Object storage backend configuration used by the gateway.

| Field | Type | Default | Description |
| ------- | ------ | --------- | ------------- |
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
| ------- | ------ | --------- | ------------- |
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

!!! warning "Runtime support"
    The configuration schema retains `kubernetes`, `docker`, and `raw`, but the current supported workload path is Kubernetes only. Do not use Docker or Raw for current deployments; they are not recommended or supported workload paths. For a Kubernetes data plane, also provide `control-plane-address` and `cert`.

### DBConfig

| Field | Type | Default | Description |
| ------- | ------ | --------- | ------------- |
| `host` | string | `postgresql` | PostgreSQL host |
| `port` | int | `5432` | PostgreSQL port |
| `database` | string | `rlark` | Database name |
| `user` | string | `rlark` | Database user |
| `password` | string | `rlark` | Database password |

### KubernetesEnv

| Field | Type | Default | Description |
| ------- | ------ | --------- | ------------- |
| `management-api` | string | `kcp` | Management API mode: `kcp` deploys kcp/optional etcd; `kubernetes` stores RLark resources in the target cluster and deploys neither kcp nor etcd |
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
| `image-pull-secrets` | string list | empty | Names of existing image pull Secrets in the `rlark-system` namespace, applied to all component Pods |
| `replicas` | int | `0` (resolved to `1`) | Default component replicas |
| `storage` | StorageConfig | unset | Default storage configuration |
| `kcp` | ComponentConfig | unset | kcp component config. kcp is currently limited to one replica. Without `etcd`, it uses a StatefulSet and supports persistent storage; with deployed or external etcd, it uses a Deployment |
| `etcd` | EtcdConfig | unset | etcd component config. An empty `address` deploys etcd; a non-empty value selects external etcd |
| `postgresql` | ComponentConfig | unset | PostgreSQL component config |
| `containerd-socket` | string | `/run/containerd/containerd.sock` | Node Agent containerd socket path |

!!! tip "Image priority"
    A component-specific image such as `gateway-image` takes priority over `image`.

### DockerEnv

!!! warning "Not currently supported"
    These fields remain in the schema for compatibility, but Docker is not a supported workload path and is not recommended for deployment.

| Field | Type | Description |
| ------- | ------ | ------------- |
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

!!! warning "Not currently supported"
    These fields remain in the schema for compatibility, but Raw is not a supported workload path and is not recommended for deployment. Use Kubernetes.

| Field | Type | Description |
| ------- | ------ | ------------- |
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
| ------- | ------ | ------------- |
| `ca-cert` | string | Inline CA PEM or an existing file path |
| `agent-cert` | string | Inline Agent certificate PEM or an existing file path |
| `agent-key` | string | Inline Agent private key PEM or an existing file path |

### StorageConfig

| Field | Type | Default | Description |
| ------- | ------ | --------- | ------------- |
| `type` | string | | Storage type: emptyDir, hostPath, pvc |
| `host-path` | string | `""` | Host path for hostPath type |
| `storage-class` | string | `""` | StorageClass for PVC type; empty uses the cluster default |
| `size` | string | `""` | Storage size |
| `node-selector` | map | empty | Node selector for the workload |

### ComponentConfig

| Field | Type | Description |
| ------- | ------ | ------------- |
| `replicas` | int | Number of replicas |
| `storage` | StorageConfig | Storage configuration |

### EtcdConfig

| Field | Type | Description |
| ------- | ------ | ------------- |
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
