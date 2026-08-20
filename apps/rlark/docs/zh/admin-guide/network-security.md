# 网络与安全

## 管理网络域

Domain 用于虚拟地址分组并限定跨集群转发范围。每个 Domain 有一个 CIDR 范围用于 IP 分配，但它不是底层基础设施或普通 Kubernetes 网络的隔离边界。

### 创建 Domain

![Domain 管理](../../images/ui/domain-ui.png)

- 管理后台 → Domain 管理 → 创建 Domain
- 输入唯一名称和 CIDR 范围（如 `10.200.0.0/24`）
- Domain CRD 在 kcp 中创建
- DomainPeer 资源自动创建用于跨集群连接

### 查看 IP 分配
- Domain 详情显示已分配的 IP 及关联的 Pod
- 确认 IP 范围在 Domain 之间不重叠
- 跨集群 Pod 通过 Domain IP 经 SSH 隧道通信

### 删除 Domain
- 确认没有活跃的 Job 引用该 Domain
- 删除 Domain CRD
- DomainPeer 资源被垃圾回收

### 通过 API 操作
```bash
# 创建 Domain
kubectl apply -f domain.yaml

# 列出 Domain
kubectl get domains -A

# 删除 Domain
kubectl delete domain <name>
```

## 跨集群网络架构

```
客户端 Pod (cluster-B)                    服务端 Pod (cluster-A)
  ├── wget → Domain IP (10.200.0.x)        ├── nc -l -p 8000
  ├── gVisor netstack 拦截                  │
  ├── TUN 设备 → NodeServer socket         │
  └── NodeServer → SSH 隧道 → ────────────→ Proxy → localhost:8000
```

## 安全最佳实践

- 为不同安全区域使用独立的 Domain
- 通过 `rlarkadm` 定期轮换 TLS 证书
- 检查 DomainPeer 资源是否有意外的跨集群连接
- Worker 访问的 SSH 密钥应通过平台管理，不直接在节点上操作
- 控制面证书自动生成，但应监控过期时间