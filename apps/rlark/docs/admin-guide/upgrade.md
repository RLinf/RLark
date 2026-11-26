# Cluster Upgrade

RLark supports batch upgrades of data-plane clusters. By creating an upgrade job, you specify the target image and configuration, and the system automatically performs the upgrade on selected clusters with automatic rollback on failure.

## Create an Upgrade Job

Navigate to **Admin Console → Cluster Upgrades** and click **Create Upgrade Job**.

### Basic Configuration

| Field | Description | Required |
|-------|-------------|----------|
| Description | Purpose of this upgrade, for easy identification | No |
| Upgrade Image | rlark-upgrade image address, e.g. `registry/rlark-upgrade:v0.0.10` | Yes |
| Target Clusters | Select clusters to upgrade, supports multiple selection | Yes |

### Advanced Configuration

| Field | Description | Default |
|-------|-------------|---------|
| Concurrency | Number of clusters to upgrade simultaneously | 1 |
| Failure Tolerate | Fraction of clusters allowed to fail (0-1) before aborting | 0 |
| Timeout (seconds) | Timeout per cluster upgrade | 600 |
| Auto Rollback on Failure | Whether to automatically rollback to the previous version on failure | Enabled |

### Deploy Config

This is the `rlarkadm` **DeployConfig** contract, not the Agent startup file (`version: v1` + `options`). Upgrade regenerates the separate cluster Deployment and node DaemonSet from DeployConfig and restarts their processes; it does not reload Agent startup files, preserve custom embedded topology, or implement `node-startup=managed`. Manually added arguments or configuration-file mounts are not preserved by generated workload replacement. Keep custom startup-file/embedded deployments under their own rollout procedure. See [Agent startup configuration](../reference/configuration.md#agent-startup-file-and-validation).

To modify the agent image or other deployment configurations, fill in the complete YAML in **Deploy Config**.

You can think of the upgrade process as analogous to a Helm Chart deployment:
- The **Upgrade Image** (rlark-upgrade) is equivalent to the Chart package
- The **Deploy Config** is equivalent to `values.yaml` for customizing deployment parameters

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://rlark-server.rlark-system.svc:8443
kubernetes:
  agent-image: registry/rlark:v0.0.67
  image: registry/rlark:v0.0.67
  image-pull-policy: Always
```

**Notes**:
- Must be a **complete configuration** including `apiVersion`, `kind`, `plane`, and other required fields
- `control-plane-address` is required
- `cert` fields are not required (certificates already exist in the cluster during upgrade)
- Leave empty if no configuration changes are needed (uses current configuration)
- Non-Kubernetes scenarios (Docker/bare metal) use the same configuration format with fields adjusted per deployment mode

## View Upgrade Jobs

### Job List

The job list is sorted by creation time (newest first) and displays:

- **Name**: Auto-generated unique ID (e.g. `up-xxxxx`)
- **Phase**:
  - **Pending**: Job created, not yet started
  - **Upgrading**: Currently upgrading
  - **Succeeded**: All clusters upgraded successfully
  - **Rolled Back**: Some clusters failed but were rolled back successfully
  - **Failed**: Some clusters failed and could not be rolled back
- **Progress**: Shows succeeded/failed/rolled-back cluster counts, e.g. `2/4 (1 failed, 1 rolled back)`
- **Created**: Job creation time

### Filter and Search

- **Phase Filter**: Click the filter button in the Phase column to show jobs with specific phases
- **Search**: Search by job name or description

### Job Detail

Click a job name to enter the detail page:

- **Progress**: Bar chart showing cluster status distribution
- **Job Info**: Description, upgrade image, target clusters, concurrency, timeout, auto-rollback setting
- **Deploy Config**: Configuration content for this upgrade
- **Cluster Details**: Status, completion time, and details for each cluster

## Job Status Reference

### ClusterUpgradeJob (per-cluster execution status)

| Status | Description |
|--------|-------------|
| Pending | Waiting to execute |
| Upgrading | Currently upgrading |
| Succeeded | Upgrade succeeded |
| RolledBack | Upgrade failed but rollback succeeded |
| Failed | Upgrade failed and rollback failed (or rollback not enabled) |

### UpgradeJob (overall job status)

| Status | Description |
|--------|-------------|
| Pending | Waiting to execute |
| Upgrading | Some clusters are upgrading, or some completed but others are not yet created (concurrency limit) |
| Succeeded | All clusters upgraded successfully |
| RollingBack | All clusters completed; some failed but were rolled back successfully |
| Failed | All clusters completed; some failed and could not be rolled back; or failure tolerance exceeded |

**Status priority**:
1. Any cluster upgrading → **Upgrading** (even if some have failed)
2. All completed → **Succeeded** / **RollingBack** / **Failed** based on results
3. Not all created (concurrency limit) but some completed → **Upgrading**

See the cluster details in the detail page for per-cluster status.

## Copy and Create

To quickly create a new job based on a previous one:

1. Click **Copy & Create** in the job list or detail page
2. The form is pre-filled with the previous job's configuration
3. Modify the required fields and submit

## Delete a Job

Only completed jobs (not Upgrading/RollingBack) can be deleted:

1. Click the **Delete** button in the job row
2. Confirm the deletion

**Note**: Deleting a job will not rollback completed upgrades.

## Automatic Rollback

When **Auto Rollback on Failure** is enabled, the upgrade process is:

1. Upgrade Pod performs pre-upgrade checks
2. Upgrades components (agent, agent-node, etc.)
3. Health check (confirm components are running normally)
4. If any stage fails, automatically rollback to the previous version
5. Rollback succeeded → Marked as **Rolled Back**
6. Rollback failed → Marked as **Failed**

## FAQ

### Q: Upgrade failed, how to check the reason?

Enter the job detail page and check the **Message** field for failed clusters in **Cluster Details**. You can also check the upgrade Pod logs:

```bash
kubectl logs -n rlark-system <upgrade-pod-name>
```

### Q: Can I cancel an in-progress upgrade?

Cancellation is not currently supported. You can delete the UpgradeJob, but already started upgrades will continue to execute.

### Q: Will rollback restore the configuration?

Rollback uses the configuration recorded from the last successful upgrade, including images and deployment configuration.

### Q: How to change the control plane address?

Fill in the new address in **Deploy Config**:

```yaml
control-plane-address: https://new-server:8443
```

### Q: What if multiple clusters fail simultaneously?

Each cluster executes upgrade and rollback independently. Failed clusters will decide whether to rollback based on configuration. Check the detail page for specific status of each cluster.
