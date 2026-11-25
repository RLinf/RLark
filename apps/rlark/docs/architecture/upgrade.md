# Cluster Upgrade Architecture

## Overview

RLark supports batch upgrades of data-plane clusters across multiple deployment modes (Kubernetes, Docker, Raw Binary). Upgrades use a two-level CRD design (`UpgradeJob` and `ClusterUpgradeJob`): the control plane creates and monitors upgrade jobs, while data-plane agents execute the actual upgrade operations.

## Core Concepts

| Concept | Description |
|---------|-------------|
| UpgradeJob | Control plane CRD that defines an upgrade task (image, target clusters, strategy) |
| ClusterUpgradeJob | Per-cluster subtask executed by the data-plane agent |
| StateStore | Abstraction for persisting cluster state (image + config). Implementations: `ConfigMapStateStore` (K8s), `FileStateStore` (Docker/Raw) |
| UpgradeDriver | Abstraction for executing upgrades. Implementations: `K8sDriver` (K8s Job), `DockerDriver` (container), `RawDriver` (systemd) |
| rlark-upgrade | Upgrade tool image/binary that performs the actual upgrade |

## Deployment Mode Support

| Mode | StateStore | UpgradeDriver | Upgrade Executor |
|------|-----------|---------------|------------------|
| Kubernetes | ConfigMap in rlark-system namespace | K8sDriver creates batch/v1.Job | Job Pod runs rlark-upgrade |
| Docker | File on host (e.g. /var/lib/rlark/state.yaml) | DockerDriver runs container | Container runs rlark-upgrade |
| Raw Binary | File on host (e.g. /etc/rlark/state.yaml) | RawDriver runs systemd service | systemd service runs rlark-upgrade |

**Note**: Docker and Raw modes are not yet fully implemented. The architecture supports them through the StateStore and UpgradeDriver interfaces.

## Architecture

```
Control Plane (kcp)                 Data Plane Cluster              Upgrade Executor
  │                                    │                               │
  │─ UpgradeJob CRD                    │                               │
  │  (spec.image, clusterNames)        │                               │
  │                                    │                               │
  │─ ClusterUpgradeJob CRD             │                               │
  │  (per-cluster, namespaced)         │                               │
  │                                    │                               │
  │                                    │◄── Agent watch ───────────────│
  │                                    │    ClusterUpgradeJob          │
  │                                    │                               │
  │                                    │── Driver.StartUpgrade() ─────►│
  │                                    │    (K8s Job / Docker / Raw)   │
  │                                    │                               │
  │                                    │                               │── Pre-check
  │                                    │                               │── Upgrade components
  │                                    │                               │── Health check
  │                                    │                               │── Rollback on failure
  │                                    │                               │── Exit (0/1/2)
  │                                    │                               │
  │                                    │── Driver.CheckStatus() ──────►│
  │                                    │    StateStore.Save()          │
  │                                    │    Update status ────────────►│
  │◄── Aggregate status ───────────────│                               │
```

## Upgrade Flow

### Phase 1: Create UpgradeJob

An administrator creates an `UpgradeJob` via the UI or API, specifying:
- **image**: rlark-upgrade image (e.g. `registry/rlark-upgrade:v0.0.10`)
- **clusterNames**: list of target clusters
- **configPatch**: deployment configuration overrides (optional)
- **strategy**: concurrency, failure tolerance, timeout, auto-rollback

### Phase 2: Control Plane Creates ClusterUpgradeJobs

The `UpgradeJob` controller creates a `ClusterUpgradeJob` for each target cluster:
- Namespace is the cluster name (e.g. `rlark-ningxia-data`)
- Creation is throttled by `strategy.concurrency`
- `MaxConcurrentReconciles = 1` to avoid race conditions

### Phase 3: Data Plane Agent Executes Upgrade

The data-plane agent watches `ClusterUpgradeJob` in its own namespace:

1. **Read state**: Load current image and configuration from `StateStore` (used as rollback target)
2. **Prepare config**: Use `configPatch` if provided, otherwise use current configuration
3. **Start upgrade**: Call `UpgradeDriver.StartUpgrade()` to create an upgrade executor
4. **Monitor**: Poll `UpgradeDriver.CheckStatus()` for completion and exit code

### Phase 4: Handle Upgrade Result

Based on the exit code:

| Exit Code | Meaning | Action |
|-----------|---------|--------|
| 0 | Upgrade succeeded | `StateStore.Save()` new state, mark as `Succeeded` |
| 2 | Upgrade failed, rollback succeeded | Do not update state, mark as `RolledBack` |
| Other | Upgrade failed, rollback failed | Do not update state, mark as `Failed` |

## StateStore Interface

The `StateStore` interface abstracts how cluster state is persisted:

```go
type StateStore interface {
    // Get returns the current ClusterState.
    // Returns an error matching apierrors.IsNotFound when no state exists yet.
    Get(ctx context.Context) (*ClusterState, error)

    // Save persists the ClusterState.
    Save(ctx context.Context, state *ClusterState) error
}

type ClusterState struct {
    Image        string  // Current rlark-upgrade image
    DeployConfig string  // Full deployment configuration YAML
}
```

### Implementations

| Implementation | Storage | Use Case |
|---------------|---------|----------|
| ConfigMapStateStore | K8s ConfigMap `rlark-state` in `rlark-system` namespace | Kubernetes data planes |
| FileStateStore | Local file (e.g. `/var/lib/rlark/state.yaml`) | Docker, Raw Binary |

## UpgradeDriver Interface

The `UpgradeDriver` interface abstracts how upgrades are executed:

```go
type UpgradeDriver interface {
    // StateStore returns the StateStore for this driver.
    StateStore() StateStore

    // PrepareConfig persists the config for this upgrade attempt.
    PrepareConfig(ctx context.Context, job *ClusterUpgradeJob, config string) (configRef string, err error)

    // StartUpgrade launches the upgrade workload.
    StartUpgrade(ctx context.Context, job *ClusterUpgradeJob, rollbackImage, configRef string) (jobName string, err error)

    // CheckStatus returns the current state of the upgrade.
    CheckStatus(ctx context.Context, jobName string) (JobStatus, error)
}
```

### Implementations

| Implementation | Executor | Use Case |
|---------------|----------|----------|
| K8sDriver | `batch/v1.Job` running rlark-upgrade image | Kubernetes data planes |
| DockerDriver | Docker container running rlark-upgrade | Docker data planes |
| RawDriver | systemd service running rlark-upgrade | Raw Binary data planes |

## Automatic Rollback

When `strategy.autoRollback` is true, the upgrade executor handles rollback internally:

1. On upgrade failure, read the rollback image and configuration from `StateStore`
2. Re-run the upgrade using the old version (i.e., rollback)
3. Rollback succeeded → exit code 2
4. Rollback failed → exit code 1 or other non-zero value

**Note**: Rollback is handled inside the upgrade executor, not by the agent creating a new rollback Job. This is because the agent itself may be unhealthy during the upgrade.

## State Machine

### ClusterUpgradeJob

```
Pending → Upgrading → Succeeded
                     ↘ Failed
                     ↘ RolledBack
```

| State | Description |
|-------|-------------|
| Pending | Waiting to execute |
| Upgrading | Upgrade in progress |
| Succeeded | Upgrade succeeded |
| RolledBack | Upgrade failed but rollback succeeded |
| Failed | Upgrade failed and rollback failed |

### UpgradeJob

```
Pending → Upgrading → Succeeded
                     ↘ Failed
                     ↘ RollingBack (indicates some clusters RolledBack)
```

For multi-cluster upgrades, the overall status reflects the most "severe" level: Failed > RollingBack > Succeeded.

## Configuration Management

### State Persistence

Each data-plane cluster persists its state via `StateStore`:

**Kubernetes (ConfigMapStateStore)**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rlark-state
  namespace: rlark-system
data:
  image: "registry/rlark-upgrade:v0.0.10"
  deploy-config: |
    apiVersion: rlark.io/v1alpha1
    kind: DeployConfig
    ...
```

**Docker/Raw (FileStateStore)**:
```yaml
# /var/lib/rlark/state.yaml
image: "registry/rlark-upgrade:v0.0.10"
deployConfig: |
  apiVersion: rlark.io/v1alpha1
  kind: DeployConfig
  ...
```

- `image`: current rlark-upgrade image
- `deploy-config`: full deployment configuration

### Config Patch

Users can provide a new configuration in `UpgradeJob.spec.configPatch`:

```yaml
spec:
  configPatch: |
    apiVersion: rlark.io/v1alpha1
    kind: DeployConfig
    plane: data
    kubernetes:
      agent-image: registry/rlark:v0.0.67
```

- If `configPatch` is not empty, it is used as the new configuration
- If `configPatch` is empty, the current configuration from `StateStore` is used

## Concurrency Control

### Control Plane Concurrency

- `MaxConcurrentReconciles = 1`: only one `UpgradeJob` is processed at a time
- `strategy.concurrency`: limits how many `ClusterUpgradeJob`s are created concurrently

### Data Plane Concurrency

- Each `ClusterUpgradeJob` corresponds to a single upgrade executor
- Only one upgrade can be in progress per cluster at a time (determined by `ClusterUpgradeJob` status)

## CRD Definitions

### UpgradeJob

```yaml
apiVersion: rlinf.io/v1alpha1
kind: UpgradeJob
metadata:
  name: up-xxxxx
  annotations:
    rlark.io/description: "Upgrade to v0.0.67"
spec:
  image: registry/rlark-upgrade:v0.0.10
  clusterNames: ["cluster-1", "cluster-2"]
  configPatch: "..."
  strategy:
    concurrency: 2
    failureTolerate: 0
    timeoutSeconds: 600
    autoRollback: true
status:
  phase: Succeeded
  totalClusters: 2
  upgradedClusters: 2
  conditions:
    - cluster: cluster-1
      status: Succeeded
      targetImage: registry/rlark-upgrade:v0.0.10
```

### ClusterUpgradeJob

```yaml
apiVersion: rlinf.io/v1alpha1
kind: ClusterUpgradeJob
metadata:
  name: up-xxxxx-cluster-1
  namespace: cluster-1
spec:
  upgradeJobName: up-xxxxx
  image: registry/rlark-upgrade:v0.0.10
  configPatch: "..."
  timeoutSeconds: 600
  autoRollback: true
status:
  phase: Succeeded
  jobName: upgrade-up-xxxxx-cluster-1-1234567890
  startTime: "2026-11-16T08:38:32Z"
  completionTime: "2026-11-16T08:39:09Z"
```

## Upgrade Executor

The upgrade executor (Job Pod, container, or systemd service) runs `rlark-upgrade`:

1. **Pre-check**: `rlarkadm` checks cluster status
2. **Upgrade**: `rlarkadm upgrade` updates component images
3. **Health Check**: `rlark-upgrade` deep health check (3 consecutive passes required)
4. **Rollback**: On failure, use `rlarkadm rollback` to restore

### Configuration

**Environment Variables**:
- `TARGET_IMAGE`: target rlark-upgrade image
- `ROLLBACK_IMAGE`: rollback image (from StateStore)
- `AUTO_ROLLBACK`: whether to enable automatic rollback

**Mounted Files**:
- `deploy-config`: new deployment configuration
- `rollback-config`: previous deployment configuration (from StateStore)

## Frontend Interface

The admin console provides an upgrade job management UI:

- **Job List**: displays all upgrade jobs with sorting, filtering, and search
- **Create Job**: form for image, clusters, strategy, and config patch
- **Job Detail**: shows progress, cluster details, and configuration
- **Copy & Create**: quickly create a new job based on an existing one

## References

- [Administrator Guide - Cluster Upgrade](../admin-guide/upgrade.md)
- [rlarkadm CLI Reference](../reference/cli.md#rlarkadm)
