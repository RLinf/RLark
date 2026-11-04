# 迁移 Cluster 下架状态

RLark 现在使用 `Cluster.spec.delisted` 表示管理员已下架集群。早期版本使用 `Cluster.spec.deleted` 表示相同的持续意图。

## 升级顺序

1. 先部署同时接受 `spec.deleted` 与 `spec.delisted` 的过渡版 CRD。
2. 在从 CRD 移除 `spec.deleted` 前，针对承载 kcp 的 Kubernetes 集群执行迁移脚本：

```bash
KUBECONFIG=~/.kube/host.kubeconfig \
  ./hack/migrate-cluster-deleted-to-delisted.sh rlark-system kcp-0
```

3. 使用 dry-run 确认不存在遗留资源：

```bash
KUBECONFIG=~/.kube/host.kubeconfig \
  ./hack/migrate-cluster-deleted-to-delisted.sh rlark-system kcp-0 --dry-run
```

4. 部署移除 `spec.deleted` 并且只使用 `spec.delisted` 的版本。

脚本会从 `rlark-kcp-kubeconfig` 读取 kcp 管理员 kubeconfig，并对所选 kcp Pod 建立端口转发，因此实际操作的是 kcp API，而不是宿主 Kubernetes API。脚本仅处理 `spec.deleted=true` 的 Cluster 资源，将 `spec.delisted` 设置为 `true`，并通过 merge patch 删除旧字段。脚本可重复执行；成功迁移后再次运行不会产生变更。

迁移完成前，请勿部署会拒绝或裁剪 `spec.deleted` 的 CRD，否则旧字段可能会在其语义转移到 `spec.delisted` 前被移除。
