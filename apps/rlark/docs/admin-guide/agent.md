# Agent Installation and Upgrade

## Installation

The Agent runs in each data-plane cluster. A Kubernetes installation created by `rlarkadm` uses:

- `rlark-agent` Deployment with `--mode=cluster` for cluster-wide synchronization.
- `rlark-agent-node` DaemonSet with `--mode=node` for node networking and image pre-pull. It is scheduled only on Nodes labeled `rlark.io/enabled=true`; only those Nodes are reported as active to the management plane. RLark-created Kubernetes Task workloads also require this label, so a user-provided node selector cannot place a Task on another matching Kubernetes Node. If the label is later removed or the local Node disappears, the cluster Agent retains the management Node record and marks it `Offline`.

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

The three certificate fields accept either inline PEM or an existing file path. `kubernetes.image` is optional; when set, it enables the network sidecar and SSH support. Label every Node that should run the node Agent before installation:

```bash
kubectl label node <node-name> rlark.io/enabled=true
```

Start from the maintained example:

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

Do not combine cluster and node modes in one Deployment for a multi-node Kubernetes data plane. `rlarkadm` creates the correct Deployment and DaemonSet, certificate Secret, RBAC, socket mounts, and container-runtime mount.

`rlarkadm` assigns the node Agent a dedicated `rlark-agent-node` ServiceAccount. Its local-cluster RBAC is limited to reading the current Node and listing/watching Node events; it does not inherit the cluster Agent's resource-management permissions.

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

### Connection Status and Heartbeats

The Server maintains the cluster Agent's `heartbeat` Lease in the registered cluster namespace for cluster connectivity. Each node Agent maintains a dedicated `node-heartbeat-<node-name>` Lease in that same namespace. The Node controller treats only the node-specific Lease as authoritative for that Node: a fresh renewal marks it online and an expired Lease marks it offline with `AgentLeaseExpired`.

A registered Node without its dedicated Lease is marked `Offline` with `NodeAgentLeaseMissing`; a cluster Agent heartbeat never makes such a Node online. This means a Node must run `rlark-agent-node` and retain a live tunnel to be online, even when the cluster Agent remains connected.

The Nodes and Clusters views display the connection state, the node Lease-derived last heartbeat, and the last connection-state transition time. Agents renew heartbeat Leases every 15 seconds, and the default Lease duration is 45 seconds; a Node is marked `Offline` when its node Lease expires. The cluster detail view keeps every cluster Node visible, including offline and non-worker Nodes; an information control next to an offline status shows the last online time.

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

### Cluster Resource Migration

After the control plane and Agent are upgraded to a release containing the `Cluster` CRD, the Server creates one cluster-scoped `Cluster` object when each previously registered cluster Agent next connects. The object name and `spec.agentNamespace` retain the existing `rlark-<cluster-id>` value. `spec.agentNamespace` is the authoritative management placement for that Agent's heartbeat Lease and Node CRs; Controllers aggregate only Nodes in that namespace, not Nodes selected by a mutable label. During Agent registration, the Server reconciles the legacy `rlark.io/cluster-id` label for Nodes in `spec.agentNamespace` as a compatibility lookup value while retaining legacy cluster IDs; it does not rename or copy the Node CRs or heartbeat Leases.

The migration is idempotent: reconnecting an Agent does not overwrite an existing `Cluster` specification, and Nodes already in the Agent namespace remain associated with that Cluster. The Agent reports Kubernetes-derived node capacity and health fields only; the control plane owns the Node connection phase, heartbeat timestamps, and `Ready` condition from the node Agent Lease. Upgrade the control plane before the data-plane Agents, then verify the backfill after every legacy cluster Agent has reconnected:

```bash
kubectl get clusters.rlinf.io
kubectl get cluster <cluster-id> -o jsonpath='{.spec.agentNamespace}{"\n"}'
```

A legacy cluster that has not reconnected is not guessed or synthesized. Keep its Agent credentials and configuration intact, restore connectivity, and wait for its next successful Server connection before treating migration as complete.

## Configuration Reference

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `cluster` | `cluster`, `node`, or `both`; Kubernetes `rlarkadm` uses separate cluster/node workloads |
| `--agent-type` | `Kubernetes` | Agent type selector. Only Kubernetes has a workload execution path; Docker and Raw are scaffolding and cannot execute Tasks |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS address |
| `--metrics-bind-address` | `:8081` | Metrics listen address |
| `--image` | `""` | Network sidecar and SSH image |
| `--leader-election` | `false` | Leader election; `rlarkadm` enables it for the cluster Agent when replicas require it |
| `--task-runtime-resync-period` | `15m` | Base period for the bounded cluster-mode Task Runtime repair scan; intervals use 80–120% jitter, `0` disables it, and negative values are invalid in `cluster`/`both` mode; node-only mode accepts the shared flag but does not run the scanner |
| `--enable-cross-cluster-direct` | `true` | Permit direct cross-cluster Pod routes |
| `--containerd-socket` | `/run/containerd/containerd.sock` | Node Agent containerd socket |

See [Configuration Reference](../reference/configuration.md#rlark-agent) for the complete list.

The Runtime/Workload coordinator is the production writer for Deployment, DaemonSet, and StatefulSet Tasks. It executes ordered Runtime operations. Runtime push controllers only enqueue reconciliation hints; verified targeted observation and Task Status updates run through the pull controller.

The cluster Agent runs one repair scan immediately after startup and repeats it at the configured jittered interval. Each scan pages through management Tasks assigned or eligible for the same Host with a 10,000-Task budget and emits full-reconciliation hints; it does not mutate Tasks or Runtime objects directly. Transient management API list failures are logged and retried in a later round without stopping the Agent. Trigger-bridge pressure is summarized but does not stop the scan from visiting later Tasks and pages. Disabling the scan removes this recovery path for lost watch events and should be done only when another operational mechanism guarantees convergence.

During an Agent upgrade, the coordinator recognizes primary workloads created by the previous Task controller even when they do not yet carry Task UID annotations. It adopts only a same-name Deployment, DaemonSet, or StatefulSet whose existing metadata does not conflict with the Task; this compatibility rule does not apply to arbitrary auxiliary resources. Initial adoption and subsequent reconciliation while the workload remains at that legacy adopted generation update identity and revision metadata only, preserving the running Pod template and avoiding an upgrade-triggered rollout. After a later Task generation establishes the workload as canonical-owned, full reconciliation repairs out-of-band drift in Agent-managed replicas and Pod-template fields even when the current Task generation has not changed.

Do not change `spec.runtime.kind` or `spec.runtime.name` to migrate an already claimed Task. The Agent freezes the claimed Runtime: such a change leaves the old workload in place, does not apply any other Spec changes from that generation, and waits for the original value to be restored or for the Task to be deleted and recreated. A decode, validation, or implementation-registration failure on a claimed, non-deleting Task also preserves the claim and existing workload instead of cleaning it up. Deletion still cleans up through the claimed Runtime and Task identity. During upgrade, a legacy Task that has the cleanup finalizer but no claimant annotation is adopted only when it is deleting or the current Agent can realize it; a non-deleting Task selected for another Agent is left untouched. See [Runtime, Workload, and Environment Boundary](../developer-guide/runtime-provider.md) for the ownership contract.

Upper-layer services should not change `spec.workload.kind` on an already claimed Kubernetes Task as their normal workflow. Replace the Task with a new UID—or use the Job restart/replacement workflow—to change carrier kind. If a direct edit nevertheless reaches the Agent, it invokes a narrow, recoverable compatibility path among Deployment, DaemonSet, and StatefulSet: the Agent first cleans up and confirms absence of the old carrier and every old `DeleteOnTaskCleanup` auxiliary object, including the Ray head Service, while retaining shared Ray ConfigMaps. Referenced external PVCs are outside RLark ownership. It then switches the frozen claimed Workload selector and creates the new carrier on a later reconciliation. Ownership conflicts or incomplete evidence block migration. This fallback can interrupt the workload and provides neither cross-Runtime migration nor transactional rollback; it exists to prevent leaked resources and stale claim metadata, not to replace Task replacement.

Task deletion, stop, and restart remain pending until every task-owned `DeleteOnTaskCleanup` object is confirmed absent. This includes the primary Deployment, DaemonSet, or StatefulSet and a Ray head Task's UID-owned head Service. Shared Ray bootstrap ConfigMaps are retained; referenced external PVCs are not part of cleanup inventory. A snapshot or deletion failure keeps the Task finalizer so reconciliation can recover safely.

The Ray head Service uses replacement semantics when an RLark-owned selector, port, type, readiness-publication setting, or canonical metadata field drifts. The Agent UID-fences and deletes the current Service, then recreates it on a later reconciliation after absence is observed. This may briefly interrupt access and may allocate different Service addresses or ports. Kubernetes-materialized defaults and allocations, including ClusterIP, IP family fields, NodePort, default protocol and target port, and unrelated metadata, are excluded from the drift projection and do not by themselves trigger replacement.

When a Job is deleted, an owned Task without the Agent cleanup finalizer was never claimed and has no Agent-created Runtime effect to stop. The Job controller deletes that Task directly instead of waiting for its Status to become `Stopped`; this allows unsupported Pending Tasks to disappear with their Job. Tasks carrying the Agent cleanup finalizer still follow the normal stop and Agent cleanup protocol.

Cleanup uses the previously claimed Task identity and the Runtime object snapshot, rather than requiring the current workload Spec to remain decodable. A claimed Task that becomes unsupported or loses its Kubernetes workload Spec can therefore still remove its provably owned primary workload and Ray head Service and release its finalizer while retaining shared Ray ConfigMaps. Referenced external PVCs remain untouched. Workload status events use targeted observation of the changed workload and its Pods instead of running a full managed-resource scan.

Each successful Task pull reconciliation emits one structured `Task reconciliation completed` summary with `task`, `outcome`, `retryClass`, effective `retryAfter`, and a stable `reason` such as `RuntimeSelectorChanged`, `WorkloadSelectorChanged`, `CarrierSelectorChanged`, or `ClaimedByOtherHost`. Repeated two-second in-progress reconciliations with the same outcome, retry class, and reason are summarized at most once per minute; a reason or outcome change is logged immediately. Failed reconciliations return the error to controller-runtime and do not emit this summary, avoiding a duplicate error log.

### Legacy PVC Upgrade Note

The Agent no longer provisions, adopts, validates, inventories, or deletes PVCs from Ray Task declarations. Existing claims created by an older Agent are left untouched during upgrade. Inventory them before removing legacy metadata or deleting claims:

```bash
# Legacy compatibility PVCs carrying the old Task label:
kubectl get pvc -n rlark-system -l rlark.io/task
kubectl get pvc -n rlark-system -l rlark.io/task=<task-name> \
  -o custom-columns=NAME:.metadata.name,TASK:.metadata.labels.rlark\.io/task,OWNER:.metadata.annotations.rlark\.io/pvc-owner-task,STATUS:.status.phase,CAPACITY:.status.capacity.storage

# Include legacy annotation-only PVCs in a manual inventory:
kubectl get pvc -n rlark-system \
  -o custom-columns=NAME:.metadata.name,TASK_LABEL:.metadata.labels.rlark\.io/task,OWNER_ANNOTATION:.metadata.annotations.rlark\.io/pvc-owner-task,STATUS:.status.phase,CAPACITY:.status.capacity.storage
```

The label-selecting commands do not find legacy PVCs that carry only `rlark.io/pvc-owner-task`; use the namespace inventory for those claims. New `persistentVolumeClaim.claimName` volumes are reference-only and require an existing claim in the workload namespace. RLark does not infer ownership from the old labels or annotations. Delete a legacy or externally managed PVC only after checking all workload references and the bound PV's reclaim policy. Pod-scoped PVCs created from `ephemeral.volumeClaimTemplate` remain Kubernetes-owned and are deleted with their Pods.

Shared Ray bootstrap ConfigMaps also survive Task cleanup. Their fixed names are reserved for RLark. The Agent preserves unrelated metadata but repairs `data`, `binaryData`, and the Ray-script identity annotation when they differ from the built-in scripts. Do not edit those script fields or let another controller manage them. Conflicting Ray-role ownership metadata remains a conflict and requires operator investigation.
