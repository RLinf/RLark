# Production Control Plane

## Architecture

The production control plane consists of the following components:

| Component | Role | `rlarkadm` port |
|-----------|------|-----------------|
| kcp | Multi-cluster control plane (Kubernetes API server) | 6443 |
| PostgreSQL | Optional persistent storage when the top-level `db` block is configured | 5432 |
| rlark-server | Certificate management, Agent tunnels, SSH, health, and metrics | 8443 (HTTPS/WSS), 2222 (SSH), 8888 (internal HTTP) |
| rlark-gateway | REST API gateway for the console and CLI | 8090 |
| rlark-controller-manager | Job/Workflow/Domain reconciliation | 8080 (metrics), 8081 (health) |
| rlark-ui | Web management console and `/api/` reverse proxy | 80 |

The standalone Gateway binary defaults to `:8080`; `rlarkadm` overrides it to `:8090`.

## Deployment Methods

### Docker Compose (Development/Testing)

```bash
docker compose -f apps/rlark/docs/examples/docker-compose.yml up -d
```

### Kubernetes

Start from the maintained example and use `rlarkadm`:

```bash
cp apps/rlark/docs/examples/deploy-control-plane.yaml deploy-control-plane.yaml
rlarkadm install -f deploy-control-plane.yaml
```

The configuration uses kebab-case YAML fields:

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: control
kubernetes:
  kubeconfig: ~/.kube/config
  gateway-image: rlark:latest
  controller-manager-image: rlark:latest
  server-image: rlark:latest
  kcp-image: kcp:v0.30.0
  postgresql-image: postgres:15
  ui-image: rlark-ui:latest
  storage:
    type: pvc
    storage-class: ""
    size: 10Gi
```

`postgresql-image` selects the image but does not by itself enable PostgreSQL. Add the top-level `db` block when RLark components should use a database, and replace all example credentials:

```yaml
db:
  host: postgresql
  port: 5432
  database: rlark
  user: rlark
  password: CHANGE_ME
```

See [Configuration Reference](../reference/configuration.md#rlarkadm-deploy-configuration) for all fields.

## Key Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| Server `--https-port` | `8443` | Agent tunnels, proxying, and certificate operations |
| Server `--ssh-port` | `2222` | User and cross-cluster SSH |
| Server `--unsafe-http-port` | `8888` | Internal `/healthz`, `/readyz`, `/livez`, `/metrics`, and peer proxy HTTP |
| Server `--auto-sign-tls-ca-cert` | `false` | Generate missing TLS CA and server certificates; `rlarkadm` enables it |
| Server `--tls-domains` | `localhost` | DNS names in generated server certificates; `rlarkadm` supplies service DNS names |
| Gateway `--addr` | `:8080` | Standalone default; `rlarkadm` uses `:8090` |
| Controller Manager `--metrics-bind-address` | `:8080` | Metrics endpoint |
| Controller Manager `--health-probe-bind-address` | `:8081` | `/healthz` and `/readyz` |

## Security

- Replace example database credentials and keep secrets outside source control.
- Use certificates valid for every Server access name; use a trusted CA for production ingress.
- Expose UI port 80 and Server ports 8443/2222 only as required. Keep Gateway, kcp, metrics, and health ports internal.
- Restrict Gateway access with a trusted authenticated ingress or reverse proxy.
- Configure persistent storage, backups, and an upgrade procedure before onboarding production clusters.

## Post-Deployment Verification

`rlarkadm install` waits up to 180 seconds for each Kubernetes workload. There is no `rlarkadm health` subcommand. Verify the workloads and the actual health endpoints:

```bash
kubectl get deploy,statefulset,daemonset -n rlark-system
kubectl rollout status deployment/rlark-server -n rlark-system
kubectl rollout status deployment/rlark-controller-manager -n rlark-system

# Server health is on the internal HTTP port, not HTTPS 8443.
kubectl port-forward -n rlark-system svc/rlark-server 8888:8888
curl --fail http://localhost:8888/healthz
curl --fail http://localhost:8888/readyz

# Run in a second terminal for Controller Manager health.
kubectl port-forward -n rlark-system deployment/rlark-controller-manager 8081:8081
curl --fail http://localhost:8081/healthz
curl --fail http://localhost:8081/readyz

# Verify the UI and its /api/ proxy to Gateway.
kubectl port-forward -n rlark-system svc/rlark-ui 8080:80
curl --fail http://localhost:8080/
```

Gateway has no dedicated health route; use its Deployment readiness and logs.
