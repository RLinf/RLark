# Node and Resource Management

## Managing Node Scheduling

Administrators can control whether a node accepts new workload by toggling its scheduling status.

### Cordon a Node (Stop Scheduling)

- PATCH the Node resource: set `spec.unschedulable: true`
- Existing running Workers on the node are NOT affected
- New jobs will not be scheduled to this node
- Use this before maintenance, upgrades, or when the node is misbehaving

### Uncordon a Node (Resume Scheduling)

- PATCH the Node resource: set `spec.unschedulable: false`
- The node resumes accepting new workload
- Verify the node is healthy before uncordoning

## Node Labels

- Administrators can add custom labels to nodes for organization
- Labels are used by users for nodeSelector in job configuration
- Common labels: `rlark.io/node-category` (cloud/edge/robot), GPU model, location

| Label | Values | Description |
|-------|--------|-------------|
| `rlark.io/node-category` | `cloud`, `edge`, `robot`, `other` | Node type category |
| `rlark.io/gpu-model` | `A100`, `V100`, `T4`, etc. | GPU model (cloud nodes) |
| `rlark.io/device-model` | `franka`, `ur5`, `realsense`, etc. | Embodied device model |
| `rlark.io/city` | Any | Physical location (annotation) |

## Inspecting Node Resources

Open the administrator console → Nodes to view and manage nodes:

> **Screenshot note:** Screenshots are from an example environment. Resource names and data are illustrative; your environment will differ.

![Node detail](../images/ui/first-login-node-detail.png)

Node details include:
- Scheduling status (schedulable / cordoned)
- Node type, access mode, OS, architecture
- Agent version
- Resource usage: CPU, memory, disk, GPU, and embodied-device resources
- Disk warnings when usage reaches 90% or kubelet reports `DiskPressure=True`
- Associated jobs running on the node

## Removing a Node

Removing a node takes it out of its cluster and returns the machine to a state where it can join again. It is not the same as cordoning the node, and it is not the same as deleting a site-node record.

RLark records the removal as a `NodeOperation` with action `Remove`. The operation runs through these stages: check, power off a robot, drain, leave the tunnel, reset the machine, delete the Node, and verify. The control plane owns the operation, not the browser:

- Closing the confirmation drawer or navigating to another admin page does not stop the removal. A task card shows the current stage while it runs.
- The node list marks the node as removing, and the row and detail view show the stage. Reloading the page restores progress from the `NodeOperation` list.
- If a stage fails, the operation stops in `Blocked` with the reason from its `Progressing` condition. A `Blocked` operation is terminal: the controller does not retry it automatically. Retry starts a new attempt with a new request ID. After an uncertain request, the console reuses the same request ID, so retrying cannot create a duplicate operation.
- A member cluster API that cannot be reached is recorded as `Blocked` with reason `MemberClusterUnreachable` after the member API timeout; the operation does not stay `Running` forever.
- A node whose kubelet stopped reporting is treated as reset even when an earlier reset Pod is still `Pending` on it. The machine clears what it kept when it joins again, so the removal proceeds instead of waiting on a Pod that cannot run.
- Only one removal per node may be Pending or Running. A second request for the same node is rejected with `409 NODE_OPERATION_IN_PROGRESS` and the name of the operation already running. Once the operation reaches `Blocked` or `Failed`, or succeeds, a new removal or retry may be started.
- A control-plane node cannot be removed: its cluster would end with it. A removal may stop RLark tasks on the node; the owning Jobs restart them elsewhere.
- If the machine cannot be reached, its old installation is cleared when it joins again. After removal RLark may no longer reach it, so it may need to join from the machine itself.

The confirmation dialog lists the tasks that will stop and any site-node record the removal will affect. Deleting a site-node record is a separate action, available only when its node was removed, is in `Conflict`, or has no Node.

## Using the UI

Administrator Console → Nodes → Select a node → Toggle scheduling or edit labels.

## Using the API

```bash
# Cordon
kubectl patch node <name> --type merge -p '{"spec":{"unschedulable":true}}'

# Uncordon
kubectl patch node <name> --type merge -p '{"spec":{"unschedulable":false}}'

# List nodes by category
kubectl get nodes -l rlark.io/node-category=cloud
```