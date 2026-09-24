# 存储

## 存储类型

RLark 支持两种存储类型：

| 类型 | 适用场景 | 生命周期 |
|------|----------|----------|
| 主机目录 | 数据已在节点上，高 I/O | 任务生命周期操作不删除数据 |
| 对象存储（临时 PVC） | 单个 Pod 运行期间的远程存储 | Kubernetes 随 Pod 创建和删除 PVC |

## 主机目录

- 管理员需确认目标节点上路径存在且权限正确
- 填写源路径（节点文件系统）和挂载路径（容器文件系统）
- 任务删除后数据保留在节点上

## 对象存储

- 使用 Kubernetes 通用临时卷和 StorageClass
- 每个 PVC 挂载默认申请 10Gi，可配置范围为 1–200Gi
- 先选择集群，可用 StorageClass 才会出现在下拉框中
- 每个 Worker Pod 使用独立的 PVC

## 管理存储类

进入 **存储** 页面可查看对象存储配置、关联集群、提供商和存储桶。为 Worker 配置 PVC 挂载前，需要先创建对应的存储类。

> **截图说明：** 截图来自示例环境，资源名称和数据仅供说明，实际环境会有所不同。

![存储管理](../../images/ui/storage-file-browser.png)

## 在训练任务中使用存储

创建任务时，在 Worker 配置步骤中：
1. 选择存储类型（hostPath 或 PVC）
2. 输入源路径（hostPath）或选择 StorageClass（PVC）
3. 输入容器挂载路径
4. 训练代码读写挂载路径

## 检查读写

验证存储链路：
1. 源位置可访问
2. 容器挂载正确
3. 应用可读取输入数据
4. 应用可写入输出数据
5. 停止或重启任务前，将需保留的 PVC 输出复制到其他位置

## 生命周期

- 停止或重启：删除 Worker Pod 时同时删除其临时 PVC，保留 hostPath 数据
- 启动或重启：Kubernetes 为新 Pod 创建新的临时 PVC
- 删除：删除 Worker Pod 时同时删除其临时 PVC，保留 hostPath 数据

## API 等效操作

使用 StorageClass、provider 和对象文件接口，详见 [Storage API](../storage-api.md)。
