# Storage API Documentation

## Overview

The Storage API provides multi-cluster StorageClass management. The Gateway queries StorageClass information from each data-plane Agent through the Server proxy, aggregates the results, and returns them.

## API Endpoints

### 1. List StorageClasses
**GET** `/api/v1/storage/storageclass`

Queries the StorageClass list from Agents in the specified clusters through the Server proxy. Default StorageClasses (`default`, `local-path`, and `hostpath`) are filtered out, and the results are grouped by name.

#### Query Parameters
- `clusters` (optional): A comma-separated list of cluster IDs, for example `?clusters=agent-beijing,agent-shanghai`. If omitted, all clusters are queried.

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
| `clusters` | `[]string` | IDs of the clusters where this StorageClass is available |
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

Creates a new rclone CSI StorageClass resource and its corresponding Secret in one or more clusters.

### 4. Update a StorageClass
**PUT** `/api/v1/storage/storageclass/{name}`

Updates the object storage configuration and associated clusters for the specified StorageClass. The `clusters` field in the request is the desired final set of clusters; the StorageClass is removed from clusters no longer included in that set. During an update, `access_key_secret` may be left empty, and the Gateway will attempt to reuse the key from an existing Secret.

### 5. Delete a StorageClass
**DELETE** `/api/v1/storage/storageclass/{name}`

Deletes the specified StorageClass and its corresponding Secret from all associated clusters. Use the `clusters=agent-a,agent-b` query parameter to limit the deletion scope.

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

### Multi-cluster Proxy Architecture

```
Gateway ──▶ Server ──▶ Agent (cluster A) ──▶ K8s API (list StorageClasses)
                  ──▶ Agent (cluster B) ──▶ K8s API (list StorageClasses)
```

1. After receiving a request, the Gateway parses the `clusters` parameter to determine the target clusters.
2. It queries each Agent through the Server proxy endpoint (`/api/proxy/{agentID}/api/kubernetes/apis/storage.k8s.io/v1/storageclasses`).
3. Each Agent queries StorageClass resources in its local Kubernetes cluster.
4. The Gateway aggregates the results by filtering out default StorageClasses, grouping entries by name, and combining their cluster information.

### File Structure
- **`../pkg/gateway/storage_handler.go`** - Storage API handlers
- **`../pkg/gateway/clusters_handler.go`** - Cluster list aggregation
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

### Agent RBAC Requirements

The Agent requires read access to the `storageclasses` resource in the `storage.k8s.io` API group. `rlarkadm` configures this permission automatically during deployment.

## Usage Examples

### List StorageClasses in All Clusters
```bash
curl "http://localhost:8080/api/v1/storage/storageclass"
```

### List StorageClasses in a Specific Cluster
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
kubernetes:
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

Kubernetes creates a PVC for each Pod from `volumeClaimTemplate` and removes it with the Pod. The frontend uses the Storage API to retrieve available StorageClasses for users to select. `pvcStorageMap` and `pvcSizeGbMap` are deprecated and retained only for existing resources.
