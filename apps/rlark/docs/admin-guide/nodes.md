# Node and Resource Management

## Managing Node Scheduling

Administrators can control whether a node accepts new workload by toggling its scheduling status.

### Pause Scheduling (Stop Scheduling)

- PATCH the Node resource: set `spec.unschedulable: true`
- Existing running Workers on the node are NOT affected and are not automatically marked failed
- New jobs will not be scheduled to this node
- Pause Scheduling is scoped to RLark. The Agent only sets the `rlark.io/unschedulable=true` label on the local Kubernetes Node, and every RLark workload template excludes that label with a required node affinity. RLark never writes the Kubernetes Node's `spec.unschedulable`, so Pods from workloads it does not manage keep being scheduled to the node
- This does not change `AgentConnected` or `Ready`: a Node with scheduling paused can remain connected and ready
- The Node list reports the Node as **Unavailable** with a **Cordoned** scheduling reason while the lifecycle phase remains `Cordoned`, because a node with scheduling paused cannot accept new work; existing workloads keep running
- Use this before maintenance, upgrades, or when the node is misbehaving
- A node cordoned outside RLark is reported as `KubernetesCordoned=True` in the Node's status and flagged in the node list. RLark does not manage that field, but a cordoned node cannot accept any Pod, so the Node Controller reports `Schedulable=False` with reason `KubernetesCordoned` and the Job scheduler stops placing new Tasks there
- Migration: nodes paused from scheduling by an earlier RLark release that also set the Kubernetes Node's `spec.unschedulable` stay cordoned, because RLark no longer manages that field. Release them explicitly with `kubectl uncordon <node>` once you confirm the node is healthy

## Draining a Node

Drain is a one-shot node maintenance operation. Use it when a node needs hardware repair, a reboot, an upgrade, or decommissioning and Tasks are still running on it, so those Tasks stop in an orderly way. Drain also blocks new placement. If you only need to block new placement while existing workloads keep running, use **Pause Scheduling** in the previous section.

### When to use it

- The node needs hardware repair, an OS upgrade, a reboot, or removal, and continuing to run workloads is unacceptable
- You need to stop RLark workloads on the node before maintenance instead of deleting the node or stopping Tasks one by one
- Do not use Drain for temporary scheduling control: use Pause Scheduling when you only need to block new placement, because Drain also stops existing Tasks on the node

### Difference from Pause Scheduling

| Operation | Blocks new placement | Stops existing Tasks on the node | Desired state written | Recovery |
|-----------|----------------------|----------------------------------|-----------------------|----------|
| Pause Scheduling | Yes | No | `spec.unschedulable=true` | Resume Scheduling |
| Drain | Yes | Yes | `spec.unschedulable=true`, plus a Drain operation; the Node Agent then writes `spec.drainRequests` on Tasks | undrain to end the drain, then Resume Scheduling or recover to restore node scheduling; Tasks stopped by the drain resume only when their Job is started explicitly |

Drain does not call the Kubernetes eviction API to delete Pods directly. The Node Agent identifies Tasks whose Pods are actually scheduled to the node and whose ownership can be proven, writes a one-shot `spec.drainRequests` entry on each, and the Task Agent stops that Task's root workload and reports the observation.

### Requesting a drain

- REST endpoint: `POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/drain?namespace=<ns>` (admin-only)
- Requests a `Drain` operation and sets `spec.unschedulable = true`. If an unfinished Drain already exists, the endpoint reuses its operation ID and returns it as `operationId` without appending another operation, so UI retries and concurrent duplicate requests converge on the same action. A new operation is created only when every existing Drain is complete
- The Gateway bounds `spec.operations` to 32 entries. At that limit, a request that would create another operation fails with `409 Conflict`; an idempotent retry that reuses an unfinished Drain still succeeds. Use **undrain** or **recover** to remove completed operations before requesting a new Drain
- Every entry in `spec.operations` must carry a non-empty `id` and a `type` of either `Drain` or `EnterMaintenance`. The API server rejects anything else, so a malformed operation never reaches the controllers to be silently ignored
- For every Task with a Pod on the Node, the Agent records a `spec.drainRequests` entry. The Task Agent stops that Task's root workload and reports `WaitingForReschedule`. The Agent writes at most 32 DrainRequest entries per Task; recording an already-present request remains a successful no-op at the limit
- `spec.drainRequests` is a projection of the Node's current Drain operations, not a permanent user intent. When a Drain operation leaves `spec.operations` through **undrain** or **recover**, the Agent removes the corresponding request, but a Task stopped by the drain does not resume on its own: the stop markers written by the Node Controller keep it suspended until an operator starts its Job explicitly (or unsuspends that Task). Deleting the Node releases the requests and clears the stop markers together, so an offline Agent or a deleted Node cannot leave Tasks permanently stopped
- Deleting the Node (`DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}`) also releases its `spec.drainRequests` from affected Tasks before deleting the object, so deleting a Node while its Agent is offline cannot leave Tasks permanently stopped
- `spec.drainRequests` is owned exclusively by the Node Agent. Entries present in a Job or Workflow Task template are ignored when the Job controller materializes Tasks, so cloning or restoring a Job never carries a stale Drain request into new Tasks
- A completed Drain remains in `spec.operations` until an operator removes it, so affected Tasks stay stopped while the Node is still drained. While it remains in `spec.operations`, a completed operation stays completed: resuming scheduling does not recompute it as pending and restart the drain

### Observing drain progress

The Node Controller publishes bounded progress for each Drain operation in `status.drainOperations`:

| Field | Meaning |
|-------|---------|
| `operationID` | ID of the Drain operation, matching `spec.operations[].id` |
| `waiting` | Number of Tasks that reported `WaitingForReschedule` for the current generation |
| `blocked` | Number of Tasks with no current-generation observation, or with state `Blocked` |
| `completed` | Whether the operation can be considered complete |

- When at least one Task has received a DrainRequest for the operation, `completed` becomes true only when `blocked=0`; `waiting` does not block completion because the corresponding root workload has already stopped
- When no Task has received a DrainRequest yet, the Node Controller consults the Agent-reported `status.activeWorkloads`: the operation is not complete while the Agent has not reported a workload snapshot; `RLarkManaged=0` means the node genuinely has no RLark workloads; `RLarkManaged>0` means the Agent has not dispatched requests yet
- The `Draining` condition reason summarizes the overall state: `DrainRequested` (True, waiting for Task observations), `DrainBlocked` (True, Tasks are still blocked), `DrainComplete` (False, all complete), and `NoDrainRequest` (False, no Drain operation)
- `status.phase` is `Draining` while an incomplete Drain operation exists; after completion it falls back to `Cordoned` because Drain also sets `spec.unschedulable=true`
- On the Task side, `spec.drainRequests` is the one-shot request written by the Node Agent (with `operationID`, `nodeUID`, and `nodeName`), and `status.drainRequests` is the Task Host's observation. `WaitingForReschedule` means the root workload has stopped and is waiting to be placed again; it does not mean the Task has moved to another node

```bash
# Drain progress on the Node
kubectl -n <agent-namespace> get nodes.rlinf.io <name> \
  -o jsonpath='{.status.drainOperations}'

# Draining condition
kubectl -n <agent-namespace> get nodes.rlinf.io <name> \
  -o jsonpath='{.status.conditions[?(@.type=="Draining")]}'

# Drain requests and observations on the Task
kubectl -n <agent-namespace> get tasks.rlinf.io <task> \
  -o jsonpath='{.spec.drainRequests}{"\n"}{.status.drainRequests}'
```

### Ending a drain (undrain)

- REST endpoint: `POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/undrain?namespace=<ns>` (admin-only)
- Removes all Drain operations from `spec.operations` while preserving `EnterMaintenance`; it does not set `spec.unschedulable` to false
- Idempotent: when the Node has no Drain operation, it returns the current object without mutating it
- Actual effect: undrain only removes the management-plane Drain desired state and releases `spec.drainRequests`. It does not resume drained Tasks and does not trigger automatic rescheduling. The `NodeDrainStoppedAnnotation` / `NodeDrainSuspendedAnnotation` markers written by the Node Controller stay in place, so the Tasks remain suspended until an operator starts their Job explicitly (or unsuspends that Task)
- After the requests are released, the node still has `spec.unschedulable=true`. To bring Tasks back to that node, follow with **uncordon** or use **recover** directly, then start the corresponding Job; to move Tasks to other nodes, first confirm cluster capacity, node selectors, and taints, then start the Job

### Restoring the node after a drain

#### Resume Scheduling

- REST endpoint: `POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/uncordon?namespace=<ns>` (admin-only)
- Equivalent kubectl: `kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge -p '{"spec":{"unschedulable":false}}'`
- Sets `spec.unschedulable = false` so new workloads can be placed again
- Removes all Drain operations from `spec.operations` (preserving `EnterMaintenance`) and releases the corresponding `spec.drainRequests`, but Tasks stopped by the drain do not resume automatically: their `NodeDrainStoppedAnnotation` / `NodeDrainSuspendedAnnotation` markers stay in place until the Job is started explicitly
- Use **Recover** below to clear the cordon flag and maintenance operations together
- Verify the node is healthy before resuming scheduling

#### Fault Recovery after Pause Scheduling or Drain

- REST endpoint: `POST /api/v1/rlinf.io/v1alpha1/nodes/{name}/recover?namespace=<ns>` (admin-only)
- One-shot recovery: removes all `Drain` and `EnterMaintenance` operations from `spec.operations` and sets `spec.unschedulable = false`
- Tasks stopped by a drain do not resume automatically: their stop markers stay in place until their Job is started explicitly
- Idempotent: a node that is already schedulable with no drain operations returns successfully without mutating the resource
- Typical workflow: pause scheduling → drain → repair hardware/software → **recover**

## Unmanaging a Node

Unmanaging (取消纳管) is rejected with `409 Conflict` while any Task's `status.observedNodes` still contains the node, regardless of which entry point is used:

- `DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}?namespace=<ns>` (admin-only) — deletes the management-plane Node CR
- `POST /api/v1/data-plane-nodes/{agent_id}/nodes/{node_name}/unmanage` (admin-only) — removes the `rlark.io/enabled` label from the data-plane Kubernetes Node so the Agent stops reporting it

Drain the node first and wait until all tasks are rescheduled or stopped (the `Draining` condition reaches reason `DrainComplete`). A node with no observed tasks can be unmanaged directly. The rejection response lists the offending Task names. The node must also be unable to receive new tasks: a connected node has to be cordoned (`spec.unschedulable=true`) first, while a node whose Agent is not connected is exempt because the controller reports it `Offline` with `Schedulable=False`, so no new tasks can be placed on it and an unreachable node stays removable.

The administrator console applies the same two prerequisites before enabling its Unmanage action and reads the Agent-reported `status.activeWorkloads.rlarkManaged` to decide whether the node still carries RLark workloads. That counter excludes non-RLark system Pods, so a node running only platform workloads remains eligible once cordoned. The server-side Task check is still authoritative: if the console enables the action but the node is referenced by a Task, the request is rejected with `409 Conflict`.

The delete entry point releases every `spec.drainRequests` entry that targets the node from affected Tasks before deleting the CR, so those Tasks resume even when the Node Agent is offline and can no longer reconcile the node. As a last-resort fallback, the control plane adds the `rlark.io/node-drain-request-cleanup` finalizer to every Node: if the object is deleted through another path, such as `kubectl delete`, the finalizer performs the same release before deletion completes. This fallback does not depend on the Gateway or the Node Agent being online.

When the label-removal entry point is used, the Agent stops reporting the node and deletes the management-plane Node CR after one confirmation sweep (about 30–36 seconds; the sweep interval is jittered by up to 20% so Agents restarted together do not list management Nodes in lockstep). If the local Kubernetes Node disappears entirely, the Agent waits about two minutes before deleting the CR so that a Node re-created with the same name is not treated as removed. Only CRs the Agent has reported at least once are eligible, and the Agent never clears finalizers owned by other controllers. If such a controller is not running, the CR remains in `Terminating` until that controller completes its cleanup.

## Node Labels and Managed Metadata

- Administrators can add custom labels to nodes for organization; labels are data-plane owned and used by users for nodeSelector in job configuration
- Administrator metadata is stored as annotations on the RLark Node CR and is not overwritten by the data plane; annotations take precedence over legacy labels carrying the same key
- Common metadata: `rlark.io/node-category-{cloud,edge,robot}` (annotations), GPU model, device model, location

| Key | Values | Description |
|-----|--------|-------------|
| `rlark.io/node-category-{cloud,edge,robot}` | `true` | Node type category (annotation on the Node CR; multiple categories allowed) |
| `rlark.io/node-category` | `cloud`, `edge`, `robot`, `other` | Legacy comma-separated category value, read as a fallback from labels or annotations |
| `rlark.io/gpu-model` | `A100`, `V100`, `T4`, etc. | GPU model (cloud nodes, annotation on the Node CR) |
| `rlark.io/device-model` | `franka`, `ur5`, `realsense`, etc. | Embodied device model (annotation on the Node CR) |
| `rlark.io/city` | Any | Physical location (annotation on the Node CR) |

## Inspecting Node Resources

Open the administrator console → Nodes to view and manage nodes:

> **Screenshot note:** Screenshots are from an example environment. Resource names and data are illustrative; your environment will differ.

![Node detail](../images/ui/first-login-node-detail.png)

Node details include:
- Scheduling status (schedulable / scheduling paused), independent from connection and readiness
- Connection and readiness conditions, plus the Agent-reported active-workload summary
- Pause Scheduling operation time and operator when the management API/audit integration records them
- Node type, access mode, OS, architecture
- Agent version
- Resource usage: CPU, memory, disk, GPU, and embodied-device resources
- Disk warnings when usage reaches 90% or kubelet reports `DiskPressure=True`
- Associated jobs running on the node

## Using the UI

Administrator Console → Nodes → Select a node → Toggle scheduling or edit labels.

## Using the API

```bash
# Pause Scheduling on an RLark Node CR. Namespace placement is explicit; do not patch the
# Kubernetes built-in Node resource by accident.
kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge \
  -p '{"spec":{"unschedulable":true}}'

# Resume Scheduling
kubectl -n <agent-namespace> patch nodes.rlinf.io <name> --type merge \
  -p '{"spec":{"unschedulable":false}}'

# Set node categories (annotations on the RLark Node CR)
kubectl -n <agent-namespace> annotate nodes.rlinf.io <name> \
  rlark.io/node-category-cloud=true

# List RLark Node CRs by category (annotations cannot be used as a label selector)
kubectl -n <agent-namespace> get nodes.rlinf.io -o json \
  | jq -r '.items[] | select(.metadata.annotations["rlark.io/node-category-cloud"]=="true") | .metadata.name'
```

`status.phase` is an operational overview only. Before placing work, require the controller-written `Schedulable=True` condition and compatible `status.capabilities`; the Node Controller derives `Schedulable` from `spec.unschedulable`, `AgentConnected`, `Ready`, and `ResourcePressure`, and the Job scheduler and console consume that condition. Do not schedule from Phase or from individual conditions alone. A Node with scheduling paused can remain connected and ready. `Degraded=True` identifies resource pressure or a failed capability for operator review; resource pressure keeps the Node `Ready`, but `ResourcePressure=True` makes `Schedulable=False`, so the Node receives no new placement while existing workloads keep running. `KubernetesCordoned` is an observation rather than a decision: it reports the Agent-observed `spec.unschedulable` of the local Kubernetes Node, a field RLark never writes. `KubernetesCordoned=True` means the node was cordoned outside RLark, so the Kubernetes scheduler already excludes every Pod on it while `Schedulable` stays `True` because RLark's own placement intent is unchanged.