# Storage API 文档

## 概述

Storage API 用于管理跨多个集群的 RLark StorageClass 声明。Gateway 将每个声明保存在控制面，Replication 和 Delivery 控制器再将生成的 rclone CSI StorageClass 及其 Secret 异步分发到所选数据面集群。

## API 端点

### 1. 获取 StorageClass 列表
**GET** `/api/v1/storage/storageclass`

从控制面列出由 RLark 管理的 StorageClass 声明。该接口不会查询 Agent，也不会校验数据面资源的实时状态。

#### 查询参数
- `clusters`（可选）：逗号分隔的集群 ID 列表，例如 `?clusters=agent-beijing,agent-shanghai`。提供该参数时，响应仅包含其已配置目标集群与指定 ID 有交集的声明；不传则返回全部已管理声明。

#### 响应示例
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

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | StorageClass 名称 |
| `clusters` | `[]string` | 该 StorageClass 已配置的目标集群 ID 列表 |
| `description` | `string` | 可读描述 |
| `bucket` | `string` | 存储桶名称 |
| `provider` | `string` | 存储提供商类型（s3、gcs、azureblob 等） |
| `endpoint` | `string` | 存储服务端点地址 |
| `region` | `string` | 存储区域 |
| `pathStyle` | `bool` | 是否使用 path-style 寻址 |
| `accessKeyId` | `string` | Access Key ID；不会返回 Access Key Secret |

### 2. 获取存储提供商列表
**GET** `/api/v1/storage/storageclass/provider`

列出支持的存储提供商列表（AWS S3、阿里云 OSS、MinIO、Ceph 等共 31 个提供商）。

#### 响应示例
```json
{
  "data": [
    { "name": "AWS S3", "value": "AWS" },
    { "name": "阿里云 OSS", "value": "Alibaba" },
    { "name": "MinIO", "value": "MinIO" }
  ],
  "success": true
}
```

### 3. 创建 StorageClass
**POST** `/api/v1/storage/storageclass`

在控制面创建 StorageClass 声明。Replication 和 Delivery 控制器会将生成的 rclone CSI StorageClass 及其 Secret 异步分发到请求指定的目标集群。

### 4. 更新 StorageClass
**PUT** `/api/v1/storage/storageclass/{name}`

更新指定 StorageClass 声明的对象存储配置和目标集群。请求中的 `clusters` 是期望的最终目标集群集合；分发流程会从未包含的集群清理资源。更新时 `access_key_secret` 可留空，Gateway 会复用已有密钥。

### 5. 删除 StorageClass
**DELETE** `/api/v1/storage/storageclass/{name}`

删除 StorageClass 声明。分发 finalizer 会异步移除其 Delivery 声明，以及所有关联集群中的生成 StorageClass 和 Secret。

### 6. 列出存储桶文件
**GET** `/api/v1/storage/storageclass/{name}/{cluster}/list`

列出指定集群中 StorageClass 存储桶下的文件列表。

#### 路径参数
- `name`：StorageClass 名称
- `cluster`：集群 ID（如 `agent-beijing`）

#### 响应示例
```json
{
  "data": [
    {"name": "model-checkpoint.pt", "size": 1048576, "modified": "2026-07-30T10:00:00Z"},
    {"name": "logs/training.log", "size": 2048, "modified": "2026-07-30T09:30:00Z"}
  ],
  "success": true
}
```

### 7. 上传文件
**POST** `/api/v1/storage/storageclass/{name}/{cluster}/upload`

向指定 StorageClass 存储桶上传文件，使用 multipart/form-data 格式。

#### 路径参数
- `name`：StorageClass 名称
- `cluster`：集群 ID

#### 请求体
multipart/form-data，字段 `file` 为上传的文件。

#### 响应示例
```json
{
  "data": {"key": "model-checkpoint.pt", "size": 1048576},
  "success": true
}
```

### 8. 下载文件
**GET** `/api/v1/storage/storageclass/{name}/{cluster}/object/*key`

下载指定存储桶中的对象，返回原始文件内容。

#### 路径参数
- `name`：StorageClass 名称
- `cluster`：集群 ID
- `key`：对象路径（如 `model-checkpoint.pt` 或 `logs/training.log`）

#### 响应
- `200`：文件内容（二进制流）
- `404`：文件不存在

### 9. 删除文件
**DELETE** `/api/v1/storage/storageclass/{name}/{cluster}/object/*key`

删除指定存储桶中的对象。

#### 路径参数
- `name`：StorageClass 名称
- `cluster`：集群 ID
- `key`：对象路径

#### 响应示例
```json
{
  "data": {"deleted": true},
  "success": true
}
```

## 实现说明

### 控制面分发架构

```
Gateway ──▶ 控制面 StorageClass 声明
                    │
                    ▼
          Replication Controller ──▶ Delivery Controller ──▶ K8s API
```

1. Gateway 校验 StorageClass 请求，并在控制面保存复制声明。
2. Replication Controller 为已配置目标集群创建 Delivery 声明。
3. 各 Delivery Controller 将生成的 rclone CSI Secret 和集群级 StorageClass 应用到本地 Kubernetes 集群。
4. 更新会替换期望目标集群集合；删除通过分发 finalizer 触发异步清理。

列表中的声明表示已配置的分发意图。创建 PVC 前，请确认下发已完成，且目标集群中的 rclone CSI 驱动正常、对象存储凭据有效，并且可连通存储服务。

### 文件结构
- **`../pkg/gateway/storage_handler.go`** - 存储 API 处理函数和生成的清单
- **`../pkg/controllermanager/replication/controller.go`** - 控制面复制控制器
- **`../pkg/agent/controllers/delivery/controller.go`** - 数据面分发控制器
- **`../pkg/gateway/router.go`** - 路由注册

### 数据结构
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

## 使用示例

### 获取全部已管理 StorageClass
```bash
curl "http://localhost:8080/api/v1/storage/storageclass"
```

### 获取以指定集群为目标的 StorageClass
```bash
curl "http://localhost:8080/api/v1/storage/storageclass?clusters=agent-beijing"
```

### 获取存储提供商列表
```bash
curl "http://localhost:8080/api/v1/storage/storageclass/provider"
```

## 与 Task PVC 挂载的集成

Task 通过 Kubernetes 通用临时卷声明远程存储：

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

Kubernetes 根据 `volumeClaimTemplate` 为每个 Pod 创建 PVC，并随 Pod 删除。当前 Web UI 写入这种通用临时卷形式，并通过 Storage API 获取已为所选集群配置的 RLark 管理 StorageClass 声明。`pvcStorageMap` 和 `pvcSizeGbMap` 不属于 canonical Task Schema；破坏性升级时必须替换依赖这些字段的旧资源。
