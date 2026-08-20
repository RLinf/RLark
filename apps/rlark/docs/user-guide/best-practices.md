# Best Practices

This guide walks through a complete end-to-end workflow for submitting a reinforcement learning training job on RLark, covering cluster selection, SSH key setup, domain creation, and job submission.

## Overview

A typical RL training workflow on RLark involves:

1. **Browse available clusters and nodes** to understand your compute environment
2. **Set up SSH keys** for remote access to Workers
3. **Create a network domain** if your job needs cross-cluster communication
4. **Create and submit a training job** with proper resource configuration
5. **Monitor and verify** that the job is running correctly

---

## Step 1: Browse Clusters and Nodes

Before creating a job, identify the cluster and nodes that will run your workload.

### Check Available Clusters

Navigate to the administrator console or the clusters page to see available clusters:

![Cluster list](../images/ui/first-login-cluster-list.png)

Note the cluster name and the labels applied to nodes in each cluster. You will need these when configuring node selectors in your job.

### Inspect Node Resources

Click on a cluster to view its nodes:

![Cluster detail](../images/ui/first-login-cluster-detail.png)

For each node, check:

- **Online status**: Is the node ready to accept work?
- **Scheduling status**: Is the node schedulable or cordoned?
- **GPU count and model**: Does the node have the GPUs your job needs?
- **Labels**: What labels are applied (e.g., `rlark.io/node-category`, `rlark.io/gpu-model`)?

Click on a node to see detailed resource usage:

![Node detail](../images/ui/first-login-node-detail.png)

!!! tip "Record your node selectors"
    Write down the exact label keys and values you will use as node selectors. For example: `rlark.io/node-category=cloud`, `rlark.io/gpu-model=A100`.

---

## Step 2: Set Up SSH Keys

SSH keys allow you to connect to your Workers for debugging and inspection.

### Add an SSH Public Key

1. Navigate to **SSH Keys** in the user menu
2. Click **Add SSH Key**
3. Provide a memorable name (e.g., `my-workstation`)
4. Paste your public key (e.g., `~/.ssh/id_ed25519.pub`)

```bash
# Generate a key if you don't have one
ssh-keygen -t ed25519 -C "rlark-training"
```

The key will be available for injection into your Workers. When selected in a job, the public key is added to `~/.ssh/authorized_keys` inside every Worker container.

### Rotate Keys

Regularly rotate your SSH keys for security:

1. Add a new key in the SSH Keys page
2. Update your jobs to use the new key
3. Remove the old key when no longer needed

---

## Step 3: Create a Network Domain (Optional)

Network domains enable cross-cluster communication between Workers. Skip this step if all Workers run in the same cluster.

### When to Create a Domain

- Workers span multiple clusters
- Workers need to communicate over a virtual network
- Ray requires cross-cluster node discovery

### Create a Domain

1. Navigate to **Administration** → **Domains**
2. Click **Create Domain**
3. Configure:
   - **CIDR**: Choose a private IP range not used by any cluster (e.g., `10.100.0.0/16`)
   - **Name**: Give the domain a descriptive name
4. Click **Create**

The domain will allocate virtual IPs to Workers that join it, establishing SSH tunnels for cross-cluster traffic.

!!! warning "CIDR must not overlap"
    The domain CIDR must not overlap with any cluster's pod or service CIDR, or with any other domain.

---

## Step 4: Create and Submit a Training Job

### Prepare Your RLinf Configuration

Before creating the job, gather the following from your existing RLinf setup:

| Item | Example |
|------|---------|
| Container image | `registry.example.com/rlinf:v0.3.0-cuda12.4` |
| GPU count per Worker | `8` |
| RLinf config | `cluster.num_nodes: 1` |
| Start command | `python -m rlinf.run config.yaml` |
| Data paths | `/data/inputs`, `/data/outputs` |

### Create the Job

1. Navigate to **Jobs** → **Create Job**

2. **Fill in basic info**:
   - Job name: lowercase, alphanumeric, hyphens (e.g., `my-training-run-001`)
   - Job type: **Custom** for single-node, or pick a template for multi-role

3. **Configure Worker roles**:

   For a single-node job:
   - Add a role named `worker`
   - Set it as **Header** (this Worker starts Ray Head)
   - Set **Node count** to `1`

   ![Worker configuration](../images/ui/create-job-worker-configuration.png)

4. **Configure each Worker**:

   | Field | Description |
   |-------|-------------|
   | **Cluster** | Select the cluster for this role |
   | **Node Selector** | Add labels to target specific nodes (e.g., `rlark.io/gpu-model=A100`) |
   | **Selected Nodes** | Must match the expected Worker count for this role |
   | **GPU** | Number of GPUs per Worker |
   | **Image** | Full image path with version tag or digest |
   | **Prep Script** | Commands to run before Ray starts (env activation, directory setup) |
   | **Storage Mounts** | Host paths or object storage to mount |

5. **Configure common settings**:

   | Setting | Description |
   |----------|-------------|
   | **Header Role** | Must be the role with exactly 1 Worker |
   | **Network Domain** | Select if cross-cluster communication is needed |
   | **SSH Key** | Select a key to inject into Workers |
   | **Run Script** | The main command executed after Ray is ready |
   | **TensorBoard Dir** | Path inside the container for TensorBoard logs |

6. **Review and submit**:

   Inspect the YAML preview carefully:

   - Verify `spec.tasks[]` count matches your role count
   - Verify `nodeSelector` only targets the expected cluster
   - Verify the Header role has exactly 1 replica
   - Verify the image uses an explicit version tag, not `latest`
   - Verify no secrets or tokens in environment variables or scripts

   Click **Submit** when ready.

!!! danger "Protect sensitive information"
    Never include passwords, private keys, access tokens, or object storage credentials in environment variables, prep scripts, run scripts, or YAML.

---

## Step 5: Monitor and Verify

After submission, verify that the job is running correctly.

### Check Job Status

1. Navigate to **Jobs** and find your job in the list
2. Click the job to open its details

![Job details](../images/ui/job-details-worker-and-pod.png)

### Verify Worker Pods

- Confirm the expected number of Workers are running
- Check that Workers are scheduled on the expected nodes
- Verify each Worker's resource allocation (GPU, device)

### Check Logs

Open the Worker logs to verify:

- Ray Head has started successfully
- All Ray nodes have joined the cluster
- The run script has started executing
- Training output appears as expected

![Job logs](../images/ui/first-login-job-logs.png)

### Verify Persistent Output

Check that checkpoints and training outputs are written to the configured storage paths.

### Common Issues

| Symptom | Check |
|---------|-------|
| Job stuck in Pending | Verify node selectors match available nodes |
| Workers not created | Check if node count matches the node selector match count |
| Run script fails | Review logs for environment or dependency errors |
| Cross-cluster network fails | Verify domain is created and CIDR doesn't overlap |

---

## Next Steps

- [Create a Training Job](jobs.md) — detailed field reference
- [Plan Multi-Node Jobs](workflows.md) — multi-role and heterogeneous Workers
- [Use Storage in Jobs](storage.md) — hostPath and object storage configuration
- [Connect via SSH](ssh-keys.md) — SSH into running Workers