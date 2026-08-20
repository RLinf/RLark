# GPU Cluster Onboarding

## Overview

Onboarding a Kubernetes data plane requires an Agent certificate and a `DeployConfig` for `rlarkadm`. The current admin form signs certificates; it does not ask for a cluster type or region and does not generate a shell installation command.

## UI-Based Certificate Flow

### Step 1: Sign the Cluster Certificate

1. Sign in to the administrator console at `http://<host>:5173/admin`.
2. Open **Cluster Management** → **Create Cluster**.
3. Enter only the cluster name, for example `my-cluster-01`.
4. Choose **Sign Certificate**.

![Create Cluster](../images/ui/admin-create-cluster.jpg)

After signing, the page displays the cluster name, Server address, and a complete deploy YAML. It also adds the name to **Signed Clusters**, where the YAML can be opened and copied again.

!!! warning "Protect the YAML"
    The displayed `agent-key` is a private key. Store the copied YAML securely and never reuse it for another cluster.

### Step 2: Complete the Deploy YAML

The UI output matches the `DeployConfig` accepted by `rlarkadm`:

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://<control-plane>:8443

cert:
  ca-cert: |
    -----BEGIN CERTIFICATE-----
    <CA certificate>
    -----END CERTIFICATE-----
  agent-cert: |
    -----BEGIN CERTIFICATE-----
    <Agent certificate>
    -----END CERTIFICATE-----
  agent-key: |
    -----BEGIN PRIVATE KEY-----
    <Agent private key>
    -----END PRIVATE KEY-----

kubernetes:
  kubeconfig: /path/to/kubeconfig.yaml
  agent-image: rlark-agent:latest
```

Replace `kubernetes.kubeconfig` with a kubeconfig that can deploy to the target cluster and set an available Agent image. Add the optional `kubernetes.image` only when enabling components that require the shared RLark image. See [Configuration Reference](../reference/configuration.md) for all accepted keys.

### Step 3: Install the Data Plane

Save the completed YAML as `deploy-data-plane.yaml`, then run it from a trusted administration host:

```bash
rlarkadm install -f deploy-data-plane.yaml
```

The UI does not generate this command or execute it for you.

### Step 4: Verify Registration

1. Return to **Cluster Management** → **Cluster List** or **Node Management**.
2. Wait for the Agent connection and node synchronization.
3. Confirm that the cluster appears online and expected Worker nodes are present.

![Verify Cluster Nodes](../images/ui/admin-clusters-nodes.jpg)

## CLI-Based Certificate Flow

For automation, sign an Agent certificate and use the maintained deployment example:

```bash
rlarkctl sign \
  --role=agent \
  --client-id=agent-my-cluster-1 \
  --output=/tmp/agent-certs

rlarkadm install -f apps/rlark/docs/examples/deploy-data-plane.yaml
```

Update the example's Server address, certificate values or paths, kubeconfig, and images before installation.

## Add Scheduling Metadata

Use the Node Management batch editor to set city, one or more cloud/edge/robot categories, GPU model, or device model. You can also cordon or uncordon selected nodes. These fields are stored on the control-plane Node CR and preserved when the Agent refreshes discovered Kubernetes state.

## Run a Smoke Test

Create a small Job from the platform console, choose the onboarded cluster for its Worker, use an available image, and submit it. Verify that the Worker reaches Running, logs are available, and WebTerminal opens if the image supplies a shell.

## Troubleshooting

| Symptom | Check |
|---------|-------|
| Agent not connecting | Server address, CA/Agent certificate pair, outbound connectivity |
| Cluster shows offline | Agent deployment logs and certificate validity |
| Nodes not appearing | Agent mode, local RBAC, and node-controller logs |
| Job does not schedule | Selected cluster, node selectors, resource requests, image pull status |

## Registration Management

- Sign a separate Agent certificate for each data-plane cluster.
- Treat copied deploy YAML as a secret because it contains `agent-key`.
- Rotate certificates with `rlarkctl sign` and redeploy the affected Agent.
