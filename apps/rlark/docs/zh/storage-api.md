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
      "storageClassName": "sc-0123456789abcdef",
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
| `name` | `string` | 供人阅读的 StorageClass 声明名称 |
| `storageClassName` | `string` | 已下发到目标数据面集群的生成式 `sc-<id>` Kubernetes StorageClass 名称；PVC 的 `spec.storageClassName` 必须使用该值 |
| `clusters` | `[]string` | 该 StorageClass 已配置的目标集群 ID 列表 |
| `description` | `string` | 可读描述 |
| `bucket` | `string` | 存储桶名称 |
| `provider` | `string` | 存储提供商类型（s3、gcs、azureblob 等） |
| `endpoint` | `string` | 存储服务端点地址 |
| `region` | `string` | 存储区域 |
| `pathStyle` | `bool` | 是否使用 path-style 寻址 |
| `accessKeyId` | `string` | 已掩码的 Access Key ID（`<前两个字符>*****<后两个字符>`）；不会返回 Access Key Secret |

### 2. 获取存储提供商列表
**GET** `/api/v1/storage/storageclass/provider`

列出支持的存储提供商。所有已认证用户均可读取该目录，以及创建、更新和删除 StorageClass。该目录是创建、更新 StorageClass 时 `provider` 合法取值的唯一来源：提交 `value`，`name` 仅用于展示。rclone 遇到无法识别的 provider 不会报错，只会打印告警并回退到通用配置，因此 Gateway 会直接拒绝不在该目录中的 `provider`。

#### 响应示例
```json
{
  "data": [
    { "name": "AWS S3", "value": "AWS" },
    { "name": "阿里云 OSS", "value": "Alibaba" },
    { "name": "MinIO", "value": "Minio" }
  ],
  "success": true
}
```

### 3. 创建 StorageClass
**POST** `/api/v1/storage/storageclass`

在控制面创建 StorageClass 声明。Replication 和 Delivery 控制器会将生成的 rclone CSI StorageClass 及其 Secret 异步分发到请求指定的目标集群。

Gateway 在存储前会对 S3 配置做规范化和校验：

- `provider` 必须命中提供商目录。canonical 取值、展示名以及旧客户端写入的历史拼写都会被接受，但记录中始终保存 canonical 取值；无法识别的取值返回 `400`。
- `endpoint` 必须是不含存储桶名的 `http`/`https` URL，缺少 scheme 时默认补 `https`。存储桶名绝不会拼进 endpoint，因为 Gateway 的 S3 客户端和 rclone 都会自行拼接 virtual-hosted URL。
- 只有发布了区域 endpoint 格式的提供商（如阿里云 OSS）才允许 `endpoint` 留空，此时必须提供 `region`，endpoint 由 region 推导得出。
- 对仅支持 virtual-hosted 寻址的提供商（如阿里云 OSS、腾讯云 COS、华为云 OBS），`path_style` 会被强制置为 `false`；其他提供商保留提交的取值。
- 完成结构校验后，Gateway 会在有界超时时间内发起带认证的 `HeadBucket` 请求。若配置的凭据无法访问存储桶，创建和更新请求会返回 `400 INVALID_REQUEST`；响应不会包含提供商错误详情或任何凭据。

该校验上线前写入的历史记录会在读取时做 canonical 化，确保 Gateway 的 S3 客户端与下发到数据面的 rclone 配置在 provider 和寻址方式上保持一致。

校验失败会使用稳定的机器可读错误码和不含敏感信息的结构化详情。缺少必填字段时，接口返回 `400`、错误码 `STORAGE_CLASS_REQUIRED_FIELDS` 以及 `details.fields`，例如：

```json
{
  "code": "STORAGE_CLASS_REQUIRED_FIELDS",
  "message": "bucket are required",
  "details": { "fields": ["bucket"] }
}
```

客户端必须使用 `code` 和 `details.fields` 生成本地化字段提示，而不是翻译面向人类的 `message` 文本。

### 4. 更新 StorageClass
**PUT** `/api/v1/storage/storageclass/{name}`

更新指定 StorageClass 声明的对象存储配置和目标集群。请求中的 `clusters` 是期望的最终目标集群集合；分发流程会从未包含的集群清理资源。更新时 `access_key_id` 和 `access_key_secret` 均可留空；Gateway 会读取并复用相应的已存储值，不会将凭据替换为空值。列表返回的 `accessKeyId` 使用五个星号掩码；客户端提交的 `access_key_id` 只要包含 `*****`，Gateway 就会将其视为未变更，而不是替换值。保留凭据时，Gateway 会使用之前存储的 Access Key ID 和 Secret 校验更新后的连接。

### 5. 删除 StorageClass
**DELETE** `/api/v1/storage/storageclass/{name}`

删除 StorageClass 声明。分发 finalizer 会异步移除其 Delivery 声明，以及所有关联集群中的生成 StorageClass 和 Secret。

### 6. 列出存储桶文件
**GET** `/api/v1/storage/storageclass/{name}/list`

列出 StorageClass 存储桶中的文件。Gateway 会直接从控制面 StorageClass 声明读取存储桶。

#### 路径参数
- `name`：StorageClass 名称

#### 查询参数
- `prefix`（可选）：要列出的对象键前缀。非根目录前缀使用 `/` 分隔且以 `/` 结尾，例如 `a/b/c/`。
- `delimiter`（可选）：对象键分隔符。默认值为 `/`；使用该分隔符时，直属子目录会在 `common_prefixes` 中返回。
- `maxKeys`（可选）：返回的对象和公共前缀的最大数量。

#### 目录导航

对象存储使用前缀而不是真实目录。文件浏览器将非空目录前缀视为以 `/` 结尾的 canonical 值。例如，目录路径 `a/b/c` 表示为 `a/b/c/`；其上级目录为 `a/b/`，面包屑目标依次为 `a/`、`a/b/` 和 `a/b/c/`。即使某个前缀不包含直属对象或子前缀，上述导航仍然可用。

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
**POST** `/api/v1/storage/storageclass/{name}/upload`

向指定 StorageClass 存储桶上传文件，使用 multipart/form-data 格式。

#### 路径参数
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
**GET** `/api/v1/storage/storageclass/{name}/object/*key`

下载指定存储桶中的对象，返回原始文件内容。

#### 路径参数
- `name`：StorageClass 名称
- `key`：对象路径（如 `model-checkpoint.pt` 或 `logs/training.log`）

#### 响应
- `200`：文件内容（二进制流）
- `404`：文件不存在

### 9. 删除文件
**DELETE** `/api/v1/storage/storageclass/{name}/object/*key`

删除指定存储桶中的对象。

#### 路径参数
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
