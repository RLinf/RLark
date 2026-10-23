# Storage API Documentation

## Overview

The Storage API manages RLark StorageClass declarations across multiple clusters. The Gateway stores each declaration in the management plane, and the Replication and Delivery controllers asynchronously distribute the generated rclone CSI StorageClass and its Secret to the selected data-plane clusters.

## API Endpoints

### 1. List StorageClasses
**GET** `/api/v1/storage/storageclass`

Lists StorageClass declarations managed by RLark from the management plane. It does not query Agents or verify the live state of data-plane resources.

#### Query Parameters
- `clusters` (optional): A comma-separated list of cluster IDs, for example `?clusters=agent-beijing,agent-shanghai`. When supplied, the response includes only declarations whose configured target clusters intersect with the specified IDs. If omitted, all managed declarations are returned.

#### Example Response
```json
{
  "data": {
    "ceph-rbd": {
      "name": "ceph-rbd",
      "clusters": ["agent-beijing", "agent-shanghai"],
      "description": "Ceph RBD StorageClass",
      "bucket": "",
      "provider": "s3",
      "endpoint": "https://s3.amazonaws.com",
      "region": "us-east-1",
      "pathStyle": false,
      "accessKeyId": "AKIA..."
    }
  },
  "success": true
}
```

| Field | Type | Description |
|------|------|------|
| `name` | `string` | StorageClass name |
| `clusters` | `[]string` | Configured target cluster IDs for this StorageClass |
| `description` | `string` | Human-readable description |
| `bucket` | `string` | Bucket name |
| `provider` | `string` | Storage provider type, such as s3, gcs, or azureblob |
| `endpoint` | `string` | Storage service endpoint |
| `region` | `string` | Storage region |
| `pathStyle` | `bool` | Whether path-style addressing is used |
| `accessKeyId` | `string` | Access Key ID; the Access Key Secret is never returned |

### 2. List Storage Providers
**GET** `/api/v1/storage/storageclass/provider`

Lists the supported storage providers, including 31 providers such as AWS S3, Alibaba Cloud OSS, MinIO, and Ceph.

#### Example Response
```json
{
  "data": [
    { "name": "AWS S3", "value": "AWS" },
    { "name": "Alibaba Cloud OSS", "value": "Alibaba" },
    { "name": "MinIO", "value": "MinIO" }
  ],
  "success": true
}
```

### 3. Create a StorageClass
**POST** `/api/v1/storage/storageclass`

Creates a StorageClass declaration in the management plane. Replication and Delivery controllers asynchronously distribute the generated rclone CSI StorageClass and its Secret to the requested target clusters.

### 4. Update a StorageClass
**PUT** `/api/v1/storage/storageclass/{name}`

Updates the object storage configuration and target clusters for the specified StorageClass declaration. The `clusters` field is the desired final target-cluster set; the distribution workflow cleans up resources from clusters omitted from that set. During an update, `access_key_secret` may be left empty, and the Gateway reuses the existing key.

### 5. Delete a StorageClass
**DELETE** `/api/v1/storage/storageclass/{name}`

Deletes the StorageClass declaration. Distribution finalizers asynchronously remove its Delivery declarations and generated StorageClass and Secret from all associated clusters.

### 6. List Bucket Files
**GET** `/api/v1/storage/storageclass/{name}/{cluster}/list`

Lists files in the StorageClass bucket in the specified cluster.

#### Path Parameters
- `name`: StorageClass name
- `cluster`: Cluster ID, such as `agent-beijing`

#### Example Response
```json
{
  "data": [
    {"name": "model-checkpoint.pt", "size": 1048576, "modified": "2026-07-30T10:00:00Z"},
    {"name": "logs/training.log", "size": 2048, "modified": "2026-07-30T09:30:00Z"}
  ],
  "success": true
}
```

### 7. Upload a File
**POST** `/api/v1/storage/storageclass/{name}/{cluster}/upload`

Uploads a file to the specified StorageClass bucket using multipart/form-data.

#### Path Parameters
- `name`: StorageClass name
- `cluster`: Cluster ID

#### Request Body
Use multipart/form-data with the uploaded file in the `file` field.

#### Example Response
```json
{
  "data": {"key": "model-checkpoint.pt", "size": 1048576},
  "success": true
}
```

### 8. Download a File
**GET** `/api/v1/storage/storageclass/{name}/{cluster}/object/*key`

Downloads an object from the specified bucket and returns the raw file content.

#### Path Parameters
- `name`: StorageClass name
- `cluster`: Cluster ID
- `key`: Object path, such as `model-checkpoint.pt` or `logs/training.log`

#### Response
- `200`: File content as a binary stream
- `404`: File not found

### 9. Delete a File
**DELETE** `/api/v1/storage/storageclass/{name}/{cluster}/object/*key`

Deletes an object from the specified bucket.

#### Path Parameters
- `name`: StorageClass name
- `cluster`: Cluster ID
- `key`: Object path

#### Example Response
```json
{
  "data": {"deleted": true},
  "success": true
}
```

## Implementation Notes

### Control-plane Distribution Architecture

```
Gateway ──▶ Management-plane StorageClass declaration
                         │
                         ▼
              Replication Controller ──▶ Delivery Controller ──▶ K8s API
```

1. The Gateway validates a StorageClass request and stores a replication declaration in the management plane.
2. The Replication Controller creates Delivery declarations for the configured target clusters.
3. Each Delivery Controller applies the generated rclone CSI Secret and cluster-scoped StorageClass to its local Kubernetes cluster.
4. Updates replace the desired target-cluster set; deletion triggers asynchronous cleanup through distribution finalizers.

A listed declaration represents configured distribution intent. Before provisioning a PVC, ensure delivery has completed and the target cluster has a working rclone CSI driver, valid object-storage credentials, and network connectivity to the storage service.

### File Structure
- **`../pkg/gateway/storage_handler.go`** - Storage API handlers and generated manifests
- **`../pkg/controllermanager/replication/controller.go`** - Control-plane replication controller
- **`../pkg/agent/controllers/delivery/controller.go`** - Data-plane delivery controller
- **`../pkg/gateway/router.go`** - Route registration

### Data Structures
```go
type StorageClassData struct {
    Name        string   `json:"name"`
    Clusters    []string `json:"clusters"`
    Description string   `json:"description"`
    Bucket      string   `json:"bucket"`
    Provider    string   `json:"provider"`
    Endpoint    string   `json:"endpoint"`
    Region      string   `json:"region"`
    PathStyle   bool     `json:"pathStyle"`
}

type Provider struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}
```

## Usage Examples

### List All Managed StorageClasses
```bash
curl "http://localhost:8080/api/v1/storage/storageclass"
```

### List StorageClasses Targeting a Specific Cluster
```bash
curl "http://localhost:8080/api/v1/storage/storageclass?clusters=agent-beijing"
```

### List Storage Providers
```bash
curl "http://localhost:8080/api/v1/storage/storageclass/provider"
```

## Integration with Task PVC Mounts

A Task declares remote storage through a Kubernetes generic ephemeral volume:

```yaml
workload:
  template:
    spec:
      volumes:
        - name: data
          ephemeral:
            volumeClaimTemplate:
              spec:
                accessModes: [ReadWriteOnce]
                storageClassName: ceph-rbd
                resources:
                  requests:
                    storage: 10Gi
```

Kubernetes creates a PVC for each Pod from `volumeClaimTemplate` and removes it with the Pod. The current Web UI writes this generic ephemeral-volume form and uses the Storage API to retrieve RLark-managed StorageClass declarations for the selected cluster. `pvcStorageMap` and `pvcSizeGbMap` are not part of the canonical Task schema; old resources that depend on those fields must be replaced during a breaking upgrade.
