# 本地开发与调试

## 前置条件

- Go 1.26+
- Node.js 18+
- Docker 和 Docker Compose
- kind（用于本地 Kubernetes 集群）
- jq（用于脚本中的 JSON 处理）

## 快速开始环境

一键 [Quick Start](../quickstart.md) 脚本提供完整的开发环境，包含 Docker Compose（kcp + PostgreSQL）和两个 kind 集群。适用于集成测试，但不是生产部署。

```bash
bash apps/rlark/docs/examples/quickstart.sh
```

## 构建 Go 组件

构建所有组件：

```bash
make build
```

单独构建各组件：

```bash
# 控制面
go build -o bin/rlark-server ./apps/rlark/cmd/server
go build -o bin/rlark-gateway ./apps/rlark/cmd/gateway
go build -o bin/rlark-controller-manager ./apps/rlark/cmd/controller-manager

# 数据面
go build -o bin/rlark-agent ./apps/rlark/cmd/agent
go build -o bin/rlark-network-sidecar ./apps/rlark/cmd/network-sidecar

# 工具
go build -o bin/rlarkadm ./apps/rlark/cmd/rlarkadm
go build -o bin/rlarkctl ./apps/rlark/cmd/rlarkctl
```

## 本地运行组件

### Server

```bash
rlark-server \
  --auto-sign-tls-ca-cert \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --kubeconfig=/tmp/rlark/admin.kubeconfig
```

### Gateway

```bash
rlark-gateway \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --server-address=https://localhost:8443
```

### Controller Manager

```bash
rlark-controller-manager \
  --server-address=https://localhost:8443 \
  --db-config=apps/rlark/docs/examples/db-config.yaml \
  --leader-elect=false \
  --metrics-bind-address=:0 \
  --health-probe-bind-address=:0
```

### Agent

```bash
rlark-agent \
  --mode=both \
  --server-address=https://localhost:8443 \
  --client-cert=/tmp/rlark/agent-certs/cert.pem \
  --client-key=/tmp/rlark/agent-certs/key.pem \
  --ca-cert=/tmp/rlark/agent-certs/ca-cert.pem \
  --image=rlark:latest
```

## Web UI 开发

```bash
cd apps/rlark-ui
npm install
npm run dev
```

UI 默认运行在 `http://localhost:5173`。

### 数据模式

Web UI 使用 `VITE_DATA_MODE` 选择数据源：

| 模式 | 值 | 说明 |
|------|-----|------|
| Mock | `mock`（默认） | 使用模拟数据进行 UI 开发和截图 |
| Backend | `backend` | 连接真实的 Gateway API |

```bash
# 使用模拟数据开发
npm run dev

# 使用真实后端开发
VITE_DATA_MODE=backend npm run dev
```

!!! warning "模拟数据限制"
    模拟数据仅用于 UI 开发和文档截图，不能视为真实资源状态。验证集群、节点、Job、存储或容量时务必使用 backend 模式。

## 测试

### 运行测试

```bash
# 运行所有测试
make test

# 运行特定包的测试
go test ./apps/rlark/pkg/... -v

# 运行单个测试
go test ./apps/rlark/pkg/... -run TestName -v

# 竞态检测
go test -race ./apps/rlark/pkg/...
```

### 代码检查

```bash
# Go 代码检查
make lint-go

# 前端代码检查
make lint-web

# 全部检查
make lint
```

## 调试

### 结构化日志

所有组件使用结构化日志。增加日志详细程度：

```bash
# 大多数组件使用标准 Go log 包
# 检查组件特定的日志级别配置参数
```

### Kubernetes Events

Controller Manager 和 Agent 在调和过程中会发出 Kubernetes Events。查看事件：

```bash
kubectl get events --sort-by='.lastTimestamp'
```

### CR 状态转换

监控 CR 状态转换以跟踪调和流程：

```bash
kubectl get job <name> -o yaml | grep -A20 status
kubectl get task <name> -o yaml | grep -A20 status
```

### 常见问题

| 问题 | 排查方向 |
|------|---------|
| Agent 无法连接 | 验证 TLS 证书、Server 地址和网络连通性 |
| Task 未创建 | 检查 Controller Manager 日志，验证 Job namespace 与 Node namespace 匹配 |
| 跨集群网络失败 | 验证 Domain CRD、NodeServer socket 和 SSH 隧道配置 |
| UI 无数据 | 确认 `VITE_DATA_MODE=backend` 且 Gateway 可访问 |
| 数据库连接错误 | 验证 `db-config.yaml` 凭据和 PostgreSQL 运行状态 |

### 前端调试

语言、主题和侧边栏状态存储在浏览器中。列表和详情使用稳定 URL 以支持刷新、分享和后退导航。

```bash
# 清除浏览器状态
localStorage.clear()
```

## 保持生成代码同步

修改 CRD 类型后，重新生成 API 客户端：

```bash
# 重新生成 CRD 清单
make generate

# 重新生成 typed clients、informers 和 listers
make -C api generate-clients
```

始终在 API 和 Web UI 中都测试面向用户的变更。

## IDE 推荐

推荐 VS Code 扩展：

- Go
- ESLint
- Prettier
- YAML