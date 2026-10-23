# Storage

## Storage Types

The Job editor exposes two storage choices: a host directory or a PVC-backed storage mount. Kubernetes-native PodTemplate volumes define their lifecycle:

| Type | Use Case | Lifecycle |
|------|----------|-----------|
| Host Directory | Data already on the node, high I/O | Job lifecycle actions do not delete data |
| Object Storage (Pod-scoped ephemeral PVC) | The form written by the current Web UI for a PVC-backed mount | Kubernetes creates and removes the PVC with its Pod |
| Existing persistent PVC reference | A Task volume uses `persistentVolumeClaim.claimName` | RLark neither creates nor deletes the referenced PVC; its administrator controls the lifecycle |

## Host Directory

- Administrator must confirm the path exists on the target node with correct permissions
- Specify source path (node filesystem) and mount path (container filesystem)
- Data persists on the node after job deletion

## Object Storage

- Uses a Kubernetes generic ephemeral volume and an RLark-managed StorageClass
- The current Web UI writes PVC-backed mounts as `ephemeral.volumeClaimTemplate`; newly created or edited Jobs therefore use Pod-scoped ephemeral PVCs
- Each PVC mount defaults to a 10Gi request; the configurable size range is 1–200Gi
- Select a cluster first; the dropdown lists StorageClass declarations configured to target that cluster
- Each worker Pod receives its own PVC

## Managing Storage Classes

Open **Storage** to review object-storage configurations, target clusters, providers, and buckets. Creating or updating a storage class stores its declaration in the control plane; the generated rclone CSI StorageClass and Secret are then distributed asynchronously to target clusters. Create the required storage class and wait for delivery before configuring a PVC mount for a Worker.

Before using a storage class, confirm the target cluster has the rclone CSI driver installed and can reach the object storage service. If a PVC remains `Pending`, check the distribution status, driver, credentials, and network connectivity.

> **Screenshot note:** Screenshots are from an example environment. Resource names and data are illustrative; your environment will differ.

![Storage management](../images/ui/storage-file-browser.png)

## Using Storage in a Training Job

When creating a job, in the Worker configuration step:
1. Select the storage type (hostPath or PVC)
2. Enter the source path (for hostPath) or select StorageClass (for PVC)
3. Enter the container mount path
4. Your training code reads/writes to the mount path

## Checking Read/Write

Verify the storage chain:
1. Source location is accessible
2. Container mount is correct
3. Application can read input data
4. Application can write output data
5. For a current Web UI Job, treat the PVC as Pod-scoped ephemeral storage and copy required output elsewhere before stopping or restarting. In `spec.workload.template.spec.volumes`, `ephemeral.volumeClaimTemplate` is Pod-scoped, while `persistentVolumeClaim.claimName` references an existing, externally managed PVC.

## Lifecycle

- Existing `persistentVolumeClaim`: RLark does not create, adopt, mutate, or delete the referenced PVC. Its lifecycle and data retention are external to the Task.
- Pod-scoped `ephemeral.volumeClaimTemplate`: deleting a Worker Pod deletes its ephemeral PVC; a new Pod receives a new PVC.
- HostPath: lifecycle actions do not delete node data.

An existing PVC reference must include `claimName`, and the claim must already exist in the workload namespace. Kubernetes reports mount or binding failures through the workload and Pod status. Deleting a referenced PVC is an external administrative action; confirm that no workload uses it and account for the bound PV's reclaim policy.

`pvcStorageMap` and `pvcSizeGbMap` are not part of the canonical Task schema. The current Web UI and Kubernetes-native examples write `ephemeral.volumeClaimTemplate`; old resources that depend on removed fields must be replaced during a breaking upgrade.

## API Equivalent

Use StorageClass, provider, and object-file endpoints described in the [Storage API](../storage-api.md).
