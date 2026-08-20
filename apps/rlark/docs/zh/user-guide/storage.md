# 存储

## 存储类型

RLark 支持两种存储类型：

| 类型 | 适用场景 | 持久性 |
|------|----------|--------|
| 主机目录 | 数据已在节点上，高 I/O | Pod 重启后保留 |
| 对象存储（PVC） | 共享数据、checkpoint 持久化 | Pod 删除后保留 |

## 主机目录

- 管理员需确认目标节点上路径存在且权限正确
- 填写源路径（节点文件系统）和挂载路径（容器文件系统）
- 任务删除后数据保留在节点上

## 对象存储

- 使用 Kubernetes StorageClass 和 PVC
- PVC 自动创建，请求 10Gi
- 先选择集群，可用 StorageClass 才会出现在下拉框中
- 多个 Worker 可共享同一 PVC（注意 RWO 访问模式限制）

## 在训练任务中使用存储

创建任务时，在 Worker 配置步骤中：
1. 选择存储类型（hostPath 或 PVC）
2. 输入源路径（hostPath）或选择 StorageClass（PVC）
3. 输入容器挂载路径
4. 训练代码读写挂载路径

![存储文件浏览器](../../images/ui/storage-file-browser.png)

## 检查读写

验证存储链路：
1. 源位置可访问
2. 容器挂载正确
3. 应用可读取输入数据
4. 应用可写入输出数据
5. 任务停止后输出持久存在

## 清理

- 停止任务：PVC 保留，hostPath 数据保留
- 删除任务：PVC 清理，hostPath 数据不清理

## API 等效操作

使用 StorageClass、provider 和对象文件接口，详见 [Storage API](../storage-api.md)。