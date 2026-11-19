# 运维与故障排查

## 健康检查流程

按以下顺序从控制面检查到数据面：

1. **服务健康**：确认所有组件正常运行
2. **数据库与 kcp**：检查数据库连接和 kcp 可用性
3. **Gateway 鉴权**：验证 Gateway API 可访问且正常接受请求
4. **Agent 心跳**：确认 Agent 已连接并上报
5. **资源同步**：验证节点、资源和标签已同步
6. **Kubernetes 事件**：检查错误和警告
7. **工作负载日志**：查看 Task 和 Worker 日志
8. **SSH 隧道连通性**：验证跨集群网络

## 常见问题

### 控制面

| 症状 | 排查方向 | 解决方案 |
|------|---------|---------|
| Gateway 返回 503 | `kubectl get pods -n rlark-system` | 重启不健康的 Pod |
| 数据库连接错误 | `kubectl logs -n rlark-system deploy/rlark-server` | 验证 db-config.yaml 凭据 |
| kcp 无响应 | `kubectl get pods -n rlark-system -l app=kcp` | 检查 kcp Pod 日志，必要时重启 |

### Agent

| 症状 | 排查方向 | 解决方案 |
|------|---------|---------|
| Agent 无法连接 | `kubectl logs -n rlark-system deploy/rlark-agent` | 验证 TLS 证书和 Server 地址 |
| Agent 心跳缺失 | 检查到控制面的网络连通性 | 验证防火墙规则、DNS 解析 |
| 节点未出现 | `kubectl get nodes -l rlark.io/cluster-id` | 确认节点标签已应用 |

### 任务

| 症状 | 排查方向 | 解决方案 |
|------|---------|---------|
| RLark Job 卡在 Pending | `kubectl describe jobs.rlinf.io <name>` | 检查 nodeSelector 匹配可用节点 |
| Task 未创建 | 检查 Controller Manager 日志 | 验证 Job namespace 与节点 namespace 匹配 |
| Worker 无法启动 | `kubectl describe pod <worker-name>` | 检查镜像拉取、资源可用性 |
| 跨集群网络失败 | 检查 Domain CRD 和 SSH 隧道 | 验证 DomainPeer 资源，重启 network-sidecar |

### 存储

| 症状 | 排查方向 | 解决方案 |
|------|---------|---------|
| PVC 卡在 Pending | `kubectl describe pvc <name>` | 检查 StorageClass 是否存在且 provisioner 运行中 |
| 挂载失败 | 检查 hostPath 在节点上是否存在 | 验证路径权限和节点可用性 |
| 对象存储不可达 | 检查存储 Provider 配置 | 验证端点、凭据和网络 |

### 诊断信息收集

收集问题报告所需的诊断信息：

```bash
# 组件版本
rlark-server --version
rlark-agent --version

# Pod 状态
kubectl get pods -n rlark-system -o wide

# 近期事件
kubectl get events -n rlark-system --sort-by='.lastTimestamp' | tail -50

# 组件日志
kubectl logs -n rlark-system deploy/rlark-server --tail=100
kubectl logs -n rlark-system deploy/rlark-agent --tail=100
kubectl logs -n rlark-system deploy/rlark-controller-manager --tail=100

# 资源状态
kubectl get nodes -o wide
kubectl get jobs.rlinf.io -A
kubectl get domains -A
```

!!! warning "诊断数据安全"
    收集诊断信息时记录组件版本与时间。禁止在问题报告中包含令牌、私钥或生成的集群凭据。分享前对配置文件中的密码进行脱敏处理。