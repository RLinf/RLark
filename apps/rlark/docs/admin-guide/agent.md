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

The three certificate fields accept either inline PEM or an existing file path. Set a non-empty RLark tools/sidecar image using `kubernetes.image`; Kubernetes cluster-mode Task assembly rejects a missing image at startup. This shared image field can also serve as the component-image fallback, but `kubernetes.agent-image` alone does not supply the Task tools image. Label every Node that should run the node Agent before installation:

```bash
kubectl label node <node-name> rlark.io/enabled=true
```

Start from the maintained example:

```bash
cp apps/rlark/docs/examples/deploy-data-plane.yaml deploy-data-plane.yaml
rlarkadm install -f deploy-data-plane.yaml
```

Do not combine cluster and node modes in one Deployment for a multi-node Kubernetes data plane. `rlarkadm` creates the correct Deployment and DaemonSet, certificate Secret, RBAC, socket mounts, and container-runtime mount.

For a manually supervised local process, `--mode=cluster --node-startup=embedded` adds this machine's node services, not remote deployment. Supply both roles' dependencies and `--node-name`; cluster metrics use `:8081`, embedded node health/metrics/pprof use `:8082` by default. Both roles share the certificate-derived management scope and local Kubernetes API, but retain separate cluster and `node-agent:<node-name>` tunnel roles. Node startup is not gated by cluster leadership. Any fatal role error cancels and joins the whole process (including HTTP shutdown and node informers); stopping does not delete workloads or release Task finalizers. Embedded health reflects local node readiness, not a proof that every cluster controller is ready. Avoid also starting another node agent on the same machine/socket. The legacy combined mode is removed; external remains default and `managed` is not implemented. This is not a new HA/backend-identity guarantee: replicas must still use the same Kubernetes data plane.

Agent supports a startup-only `--config` file and the `check-config` and redacted `print-config` subcommands. These do not accept DeployConfig; see [startup schema, environment compatibility and validation](../reference/configuration.md#agent-startup-file-and-validation). Invalid initial configuration fails before clients/workers. Correct it and restart; no file reload or managed rollout is available in MR 4.1.

Normal startup checks local Kubernetes `/version` and, for node/embedded, local Node existence under a 30-second deadline. It then starts a supervised bootstrap tunnel and waits up to 30 seconds for its WebSocket handshake: Server completes first registration (management namespace, ServiceAccount and RBAC) before accepting the upgrade. Only then does a separate 30-second management preflight read Tasks for cluster and DomainPeers/Pods (plus Tasks when pre-pull is enabled) for node/embedded. Forbidden/NotFound and API timeout/unavailability are retried within that deadline to allow registration permissions to propagate; persistent denial still fails closed. All selected roles finish Build/Setup before business workers and the local proxy HTTP server start. Embedded adds its node-role tunnel after preparation, without duplicating the bootstrap cluster tunnel. Node informer initial sync has its own 30-second deadline. Registration, preflight or Build failure cancels and joins the bootstrap tunnel; registered management objects are not rolled back. Restore access/configuration and restart to recover. These checks do not replace future-operation authorization or prove backend identity.

Shutdown and failed listener startup close/join NodeServer's private in-memory HTTP service as well as its socket cancellation worker. At shutdown, private HTTP admission, keepalive connections, and host watches stop before waiting for workload connections to drain; concurrent stop calls are safe. Private dials wait for HTTP acceptance instead of accumulating unaccepted buffered peers. The image monitor closes its process-owned containerd connection after workers stop. Existing connection drain timeout and workload ownership semantics are unchanged.

For bidirectional connection relays, the 60-second drain budget starts only after one copy direction ends; it does not limit ongoing traffic. If a connection rejects read deadlines (for example, an SSH channel), expiry closes only the two relayed stream connections, not the shared SSH session or unrelated channels. If both directions finish earlier, the fallback timer is canceled or joined before the relay returns. This does not change the separate NodeServer workload drain budget or provide an upgrade continuity guarantee.

Agent's internal proxy HTTP listener also uses unbuffered Pipe admission so pending dials cannot leave unaccepted peers behind on shutdown. Its existing HTTP shutdown budget remains three seconds, followed by forced connection closure if graceful shutdown times out; the shutdown worker is joined even when serving fails.

NodeServer's private HTTP dial waits at most 30 seconds for acceptance, matching the remote dial timeout. This bounds connection admission only, not established traffic or host watches.

The separate `RelayStreams` helper used by SSH services does not have this forced-close fallback. When a stream does not implement read deadlines, it emits a verbosity-1 diagnostic with the copy direction; drain still relies on available half-close operations and peer completion. This diagnostic does not impose a shutdown bound.

`rlarkadm` assigns the node Agent a dedicated `rlark-agent-node` ServiceAccount. Its local-cluster RBAC is limited to reading the current Node and listing/watching Node events; it does not inherit the cluster Agent's resource-management permissions.

Cluster startup (external or embedded) also requires management `ClusterUpgradeJob` read access. This dependency is checked after bootstrap registration under the management preflight deadline; persistent denial or a missing CRD prevents business workers from starting. Node-only startup does not require this resource. Upgrade controller setup belongs to cluster Build and starts only with its manager.

### Task Runtime startup and shutdown

Inspect admission through the management API, for example `kubectl --kubeconfig <management-kubeconfig> get tasks.rlinf.io <task-name> -n <management-namespace> -o yaml`. `status.conditions[type=ExecutionAdmitted]` reports permanent rejection as `False` / `ExecutionAdmissionFailed`, with the reason detail and observed generation. Correct the Task configuration or restore required Agent capabilities; relevant changes and periodic resync retry admission without ordinary error backoff. Passing admission changes the Condition to `True`, not workload Ready. Rejection does not change Phase or destroy existing workloads/claims; deletion and suspension still perform identity-only cleanup. Completed suspension/reset removes the admission Condition. Transient API/transport or Status publication failures remain retryable errors.

The assembled Runtime checks inventory advertisements/interfaces at startup. Normal Task execution also validates compiled/bound requirements before lifecycle/workload writes; unsupported required Environment, units, projections or profiles fail closed without claiming a new Task. Invalid configuration does not release existing claims. Deletion/suspension and Node Drain cleanup still use ownership-proven inventory, not current Spec compilation. See [admission and recovery boundaries](../developer-guide/runtime-provider.md#assembly-admission-and-recovery-conformance).

The Kubernetes cluster Agent builds one complete Task execution assembly before installing its pull, observation and resync components. Invalid dependencies, observation mappings or denied/indeterminate observation permissions abort startup before managers run. Installation failures are fatal too: restart with corrected configuration rather than running a partially installed Agent. Build's metadata/permission preflight has a 30-second deadline and starts no workload watches or writes.

Task execution replicas sharing a management namespace and Agent Kind must connect to the same Kubernetes data plane. Do not use the same logical Agent scope for different kubeconfigs pointing to different clusters; backend identity is not automatically checked. Node Agents remain separate node services, not independent Task owners.

On shutdown, manager context cancellation stops and joins Task observation workers together with the other manager-owned components. Shutdown itself does not delete Task workloads or release finalizers; the next Agent resumes reconciliation from management Tasks and current Runtime state. This assembly change adds no flags, Task schema changes, or Docker/Raw workload support.

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

The Nodes and Clusters views display the connection state, the node Lease-derived last heartbeat, and the last connection-state transition time. Agents renew heartbeat Leases every 15 seconds, and the default Lease duration is 45 seconds; a Node is marked `Offline` when its node Lease expires. A reconnecting Agent resumes renewal immediately, so a completed Agent restart does not wait for the previous Lease to expire before the Node becomes online again. The cluster detail view keeps every cluster Node visible, including offline and non-worker Nodes; an information control next to an offline status shows the last online time.

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
| `--mode` | `cluster` | `cluster` or `node`; Kubernetes `rlarkadm` uses separate workloads; local composition uses `--node-startup=embedded` |
| `--agent-type` | `kubernetes` | Case-insensitive startup selector mapped to unchanged API Kind; Docker/Raw explicitly unsupported |
| `--server-address` | `https://localhost:8443` | Server HTTPS/WSS address |
| `--metrics-bind-address` | `:8081` | Metrics listen address |
| `--image` | `""` | RLark tools/network/SSH image; non-empty required by Kubernetes cluster-mode Task assembly |
| `--leader-election` | `false` | Legacy accepted flag; installer may set it, but current Agent managers do not consume it or enforce leadership |
| `--task-runtime-resync-period` | `15m` | Base period for the bounded cluster-mode Task Runtime repair scan; intervals use 80–120% jitter, `0` disables it, and negative values are invalid in cluster mode; node-only mode accepts the shared flag but does not run the scanner |
| `--enable-cross-cluster-direct` | `true` | Not implemented; cross-cluster traffic always uses the SSH relay |
| `--containerd-socket` | `/run/containerd/containerd.sock` | Node Agent containerd socket |

See [Configuration Reference](../reference/configuration.md#rlark-agent) for the complete list.

The Runtime/Workload coordinator is the production writer for Deployment and StatefulSet Tasks. It executes ordered Runtime operations. The Agent creates Runtime push controllers from the trusted workload-root registry; they only enqueue reconciliation hints, while verified targeted observation and Task Status updates run through the pull controller. `--task-runtime-push-controller-workers` configures concurrency for each registered root watch.

Task root and secondary Pod watches use a dedicated cache restricted to the historical workload namespace `rlark-system`. This does not restrict the separate cluster-wide Node/Pod reporting controllers. Before observer startup, the Agent checks root `get/list/watch`, Pod `get/list/watch` for the declared instance profiles, and ReplicaSet `get` for the Deployment profile via `SelfSubjectAccessReview` (30-second preflight timeout). Denied, indeterminate, or failed reviews stop startup; they are not interpreted as empty inventory. The credentials must also permit creating self-access reviews. If startup fails, repair the applicable RBAC or API availability and restart; no partial Task observer starts. Runtime roots never require `/status` write permission for observation. Root deletion with verified Task UID requests full pull reconciliation; missing legacy UID metadata relies on periodic resync.

**Worker flag upgrade/rollback:** remove `--task-deployment-push-controller-workers` and `--task-statefulset-push-controller-workers` from deployment arguments and use `--task-runtime-push-controller-workers` (default `8`). Old names are not aliases and are rejected. If their values differed, choose an explicit shared concurrency budget; using the larger value preserves the previous upper per-root allowance, but may increase total concurrency. When rolling back to an Agent that predates the shared flag, restore its two supported flags. Do not run old and new Agents concurrently as workload/Task Status writers.

The Ray-on-Kubernetes Workload Adapter supports Deployment and StatefulSet carriers.

The cluster Agent runs one repair scan immediately after startup and repeats it at the configured jittered interval. Each scan pages through management Tasks assigned or eligible for the same Host with a 10,000-Task budget and emits full-reconciliation hints; it does not mutate Tasks or Runtime objects directly. Transient management API list failures are logged and retried in a later round without stopping the Agent. Trigger-bridge pressure is summarized but does not stop the scan from visiting later Tasks and pages. Disabling the scan removes this recovery path for lost watch events and should be done only when another operational mechanism guarantees convergence.

During an Agent upgrade, the coordinator recognizes primary workloads created by the previous Task controller even when they do not yet carry Task UID annotations. It adopts only a same-name Deployment or StatefulSet whose existing metadata does not conflict with the Task; this compatibility rule does not apply to arbitrary auxiliary resources. Initial adoption and subsequent reconciliation while the workload remains at that legacy adopted generation update identity and revision metadata only, preserving the running Pod template and avoiding an upgrade-triggered rollout. After a later Task generation establishes the workload as canonical-owned, full reconciliation repairs out-of-band drift in Agent-managed replicas and Pod-template fields even when the current Task generation has not changed.

Do not change `spec.runtime.kind` or `spec.runtime.name` to migrate an already claimed Task. The Agent freezes the claimed Runtime: such a change leaves the old workload in place, does not apply any other Spec changes from that generation, and waits for the original value to be restored or for the Task to be deleted and recreated. A decode, validation, or implementation-registration failure on a claimed, non-deleting Task also preserves the claim and existing workload instead of cleaning it up. Deletion still cleans up through the claimed Runtime and Task identity. During upgrade, a legacy Task that has the cleanup finalizer but no claimant annotation is adopted only when it is deleting or the current Agent can realize it; a non-deleting Task selected for another Agent is left untouched. See [Runtime, Workload, and Environment Boundary](../developer-guide/runtime-provider.md) for the ownership contract.

Upper-layer services should not change `spec.workload.kind` on an already claimed Kubernetes Task as their normal workflow. Replace the Task with a new UID—or use the Job restart/replacement workflow—to change carrier kind. If a direct edit nevertheless reaches the Agent, it invokes a narrow, recoverable compatibility path between Deployment and StatefulSet: the Agent first cleans up and confirms absence of the old carrier and every old `DeleteOnTaskCleanup` auxiliary object, including the Ray head Service, while retaining shared Ray ConfigMaps. Referenced external PVCs are outside RLark ownership. It then switches the frozen claimed Workload selector and creates the new carrier on a later reconciliation. Ownership conflicts or incomplete evidence block migration. This fallback can interrupt the workload and provides neither cross-Runtime migration nor transactional rollback; it exists to prevent leaked resources and stale claim metadata, not to replace Task replacement.

Task deletion, stop, and restart remain pending until every task-owned `DeleteOnTaskCleanup` object is confirmed absent. This includes the primary Deployment or StatefulSet and a Ray head Task's UID-owned head Service. Shared Ray bootstrap ConfigMaps are retained; referenced external PVCs are not part of cleanup inventory. A snapshot or deletion failure keeps the Task finalizer so reconciliation can recover safely.

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
