# Agent Installation and Upgrade

## Installation

The Agent runs in each data-plane cluster. A Kubernetes installation created by `rlarkadm` uses:

- `rlark-agent` Deployment with `--mode=cluster` for cluster-wide synchronization.
- `rlark-agent-node` DaemonSet with `--mode=node` for node networking and image pre-pull.

### Prerequisites

- A Kubernetes cluster with `kubectl` access.
- Outbound connectivity to the Server HTTPS/WSS port, normally 8443.
- A CA certificate, Agent certificate, and Agent private key issued for the cluster.
- The Server hostname or IP included in its TLS certificate.

### Install with rlarkadm (Recommended)

There is no `rlarkagent` command. Save the certificate values from the admin console or Gateway API in a `DeployConfig`, then run `rlarkadm install`:

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
  # containerd-socket: /run/k3s/containerd/containerd.sock
```

The three certificate fields accept either inline PEM or an existing file path. `kubernetes.image` is optional; when set, it enables the network sidecar and SSH support. Start from the maintained example:

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

Do not combine cluster and node modes in one Deployment for a multi-node Kubernetes data plane. `rlarkadm` creates the correct Deployment and DaemonSet, certificate Secret, RBAC, socket mounts, and container-runtime mount.

### Verification

```bash
kubectl get deployment/rlark-agent daemonset/rlark-agent-node -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system

kubectl logs -n rlark-system deployment/rlark-agent --tail=100
kubectl logs -n rlark-system daemonset/rlark-agent-node --tail=100

# Verify registered resources through the UI proxy to Gateway.
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80
curl --fail http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes
```

Port 8443 belongs to Server tunnel and proxy traffic; it is not the Gateway REST endpoint. Agent exposes metrics on `:8081` but has no dedicated HTTP health route, so use rollout status, logs, and resource registration.

### Verification Checklist

| Check | Expected Result |
|-------|-----------------|
| Cluster Agent | `rlark-agent` Deployment is available |
| Node Agents | `rlark-agent-node` desired and ready counts match |
| Connection | Logs show successful Server connection without repeated TLS errors |
| Cluster status | Cluster is online in the admin console |
| Node registration | Worker nodes appear with the expected labels and capacity |
| Task creation | A test Job creates and runs its Task workload |
| Networking | Network sidecar and SSH work when `kubernetes.image` is configured |

## Upgrade

Review the [Release Notes](../reference/changelog.md), back up manifests and certificates, and test on a non-critical cluster. Update both Agent workloads to the same release as the control plane:

```bash
kubectl set image deployment/rlark-agent agent=<new-image> -n rlark-system
kubectl set image daemonset/rlark-agent-node agent=<new-image> -n rlark-system
kubectl rollout status deployment/rlark-agent -n rlark-system
kubectl rollout status daemonset/rlark-agent-node -n rlark-system
```

Alternatively, update `agent-image` in the deployment configuration and run `rlarkadm install -f deploy-data-plane.yaml`. There is no `rlarkagent upgrade` command.

## Configuration Reference

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `cluster` | `cluster`, `node`, or `both`; Kubernetes `rlarkadm` uses separate cluster/node workloads |
| `--agent-type` | `Kubernetes` | Runtime type: Kubernetes, Docker, or Raw |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS address |
| `--metrics-bind-address` | `:8081` | Metrics listen address |
| `--image` | `""` | Network sidecar and SSH image |
| `--leader-election` | `false` | Leader election; `rlarkadm` enables it for the cluster Agent when replicas require it |
| `--enable-cross-cluster-direct` | `true` | Permit direct cross-cluster Pod routes |
| `--containerd-socket` | `/run/containerd/containerd.sock` | Node Agent containerd socket |

See [Configuration Reference](../reference/configuration.md#rlark-agent) for the complete list.
