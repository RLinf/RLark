# Storage API 文档

## 概述

Storage API 提供多集群 StorageClass 管理能力。Gateway 通过 Server 代理向各数据面 Agent 查询 StorageClass 信息，聚合后返回。

## API 端点

### 1. 获取 StorageClass 列表
**GET** `/api/v1/storage/storageclass`

通过 Server 代理，从指定集群的 Agent 查询 StorageClass 列表。过滤掉默认 StorageClass（`default`、`local-path`、`hostpath`），按名称分组聚合。

#### 查询参数
- `clusters` (可选): 逗号分隔的集群 ID 列表，如 `?clusters=agent-beijing,agent-shanghai`。不传则查询所有集群。

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
| `clusters` | `[]string` | 该 StorageClass 可用的集群 ID 列表 |
| `description` | `string` | 可读描述 |
| `bucket` | `string` | 存储桶名称 |
| `provider` | `string` | 存储提供商类型（s3, gcs, azureblob 等） |
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

在一个或多个集群中创建新的 rclone CSI StorageClass 资源和配套 Secret。

### 4. 更新 StorageClass
**PUT** `/api/v1/storage/storageclass/{name}`

更新指定 StorageClass 的对象存储配置和关联集群。请求中的 `clusters` 是期望的最终集群集合；已不在集合内的集群会移除该 StorageClass。更新时 `access_key_secret` 可留空，Gateway 会尽量复用已有 Secret 中的密钥。

### 5. 删除 StorageClass
**DELETE** `/api/v1/storage/storageclass/{name}`

从所有已关联集群删除指定 StorageClass 和配套 Secret。可通过查询参数 `clusters=agent-a,agent-b` 限制删除范围。

### 6. 列出存储桶文件
**GET** `/api/v1/storage/storageclass/{cluster}/{name}/list`

列出指定集群中 StorageClass 存储桶下的文件列表。

#### 路径参数
- `cluster`：集群 ID（如 `agent-beijing`）
- `name`：StorageClass 名称

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
**POST** `/api/v1/storage/storageclass/{cluster}/{name}/upload`

向指定 StorageClass 存储桶上传文件，使用 multipart/form-data 格式。

#### 路径参数
- `cluster`：集群 ID
- `name`：StorageClass 名称

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
**GET** `/api/v1/storage/storageclass/{cluster}/{name}/object/*key`

下载指定存储桶中的对象，返回原始文件内容。

#### 路径参数
- `cluster`：集群 ID
- `name`：StorageClass 名称
- `key`：对象路径（如 `model-checkpoint.pt` 或 `logs/training.log`）

#### 响应
- `200`：文件内容（二进制流）
- `404`：文件不存在

### 7. 删除文件
**DELETE** `/api/v1/storage/storageclass/{cluster}/{name}/object/*key`

删除指定存储桶中的对象。

#### 路径参数
- `cluster`：集群 ID
- `name`：StorageClass 名称
- `key`：对象路径

#### 响应示例
```json
{
  "data": {"deleted": true},
  "success": true
}
```

## 实现说明

### 多集群代理架构

```
Gateway ──▶ Server ──▶ Agent (cluster A) ──▶ K8s API (list StorageClasses)
                  ──▶ Agent (cluster B) ──▶ K8s API (list StorageClasses)
```

1. Gateway 接收请求后，解析 `clusters` 参数获取目标集群列表
2. 通过 Server 的代理接口 (`/api/proxy/{agentID}/api/kubernetes/apis/storage.k8s.io/v1/storageclasses`) 向各 Agent 发起查询
3. Agent 查询本地 K8s 集群的 StorageClass 资源
4. Gateway 聚合结果：过滤默认 StorageClass，按名称分组，汇总各集群信息

### 文件结构
- **`../pkg/gateway/storage_handler.go`** - 存储 API 处理函数
- **`../pkg/gateway/clusters_handler.go`** - 集群列表聚合
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

### Agent RBAC 要求

Agent 需要 `storage.k8s.io` API 组的 `storageclasses` 资源读取权限，`rlarkadm` 部署时自动配置。

## 使用示例

### 获取所有集群的 StorageClass
```bash
curl "http://localhost:8080/api/v1/storage/storageclass"
```

### 获取指定集群的 StorageClass
```bash
curl "http://localhost:8080/api/v1/storage/storageclass?clusters=agent-beijing"
```

### 获取存储提供商列表
```bash
curl "http://localhost:8080/api/v1/storage/storageclass/provider"
```

## 与 Task PVC 挂载的集成

Task 通过 `pvcStorageMap` 声明需要挂载的 PVC：

```yaml
kubernetes:
  workload:
    pvcStorageMap:
      my-data-pvc: "ceph-rbd"
```

Agent 的 Pull 控制器在创建 workload 前，调用 `ensurePVCs` 根据 `pvcStorageMap` 创建对应的 PVC。前端通过 Storage API 获取可用 StorageClass 列表供用户选择。
