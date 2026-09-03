# Storage

## Storage Types

RLark supports two storage types for training jobs:

| Type | Use Case | Lifecycle |
|------|----------|-----------|
| Host Directory | Data already on the node, high I/O | Job lifecycle actions do not delete data |
| Object Storage (PVC) | Shared data within a Job run | Stop/restart/delete removes task PVCs; start/restart creates empty PVCs |

## Host Directory

- Administrator must confirm the path exists on the target node with correct permissions
- Specify source path (node filesystem) and mount path (container filesystem)
- Data persists on the node after job deletion

## Object Storage

- Uses Kubernetes StorageClass and PVC
- PVC is auto-created with 10Gi request
- Select cluster first, then available StorageClasses appear in the dropdown
- Multiple workers can share the same PVC (be aware of RWO access mode limitations)

## Using Storage in a Training Job

When creating a job, in the Worker configuration step:
1. Select the storage type (hostPath or PVC)
2. Enter the source path (for hostPath) or select StorageClass (for PVC)
3. Enter the container mount path
4. Your training code reads/writes to the mount path

![Storage file browser](../images/ui/storage-file-browser.png)

## Checking Read/Write

Verify the storage chain:
1. Source location is accessible
2. Container mount is correct
3. Application can read input data
4. Application can write output data
5. Copy required PVC output elsewhere before stopping or restarting the Job

## Lifecycle

- Stop or restart: task PVCs are deleted; hostPath data is preserved
- Start or restart: new empty task PVCs are created
- Delete: task PVCs are deleted; hostPath data is preserved

## API Equivalent

Use StorageClass, provider, and object-file endpoints described in the [Storage API](../storage-api.md).