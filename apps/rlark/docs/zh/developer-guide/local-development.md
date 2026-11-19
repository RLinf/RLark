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

如需在项目长期运行的多集群 Kind 拓扑上执行可重复测试，请阅读[端到端测试](e2e-testing.md)。E2E
框架支持配置集群路径、镜像、超时和数据面白名单。

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
  --leader-election=false \
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

修改 CRD 类型后，需要重新生成 CRD 清单、Go Client、Schema 参考和前端
TypeScript 模型：

```bash
# 重新生成全部 CRD 派生产物
make generate

# 重新生成 typed clients、informers 和 listers
make -C api generate-clients
```

始终在 API 和 Web UI 中都测试面向用户的变更。

`make verify-generated` 使用同一套生成器重新生成产物，当签入的 CRD 清单、deepcopy
函数、类型化 Go Client、前端 TypeScript 模型或 `apps/rlark/docs/reference/crd.md`
与重新生成的结果不一致、
或生成过程产生了未跟踪文件时失败。CI 对每次变更都会执行它，因此推送 API 变更前请先
在本地运行。

前端模型以 `api/config/crd/bases` 中签入的清单为输入，生成到
`apps/rlark-ui/src/generated/rlark`。生成文件需要签入仓库且禁止手工修改。前端代码应通过
`apps/rlark-ui/src/generated/rlark-types.ts` 引用 CRD 接口，并继续将页面/领域视图模型与
wire 类型分开。生成器忠实遵循 `controller-gen` 产生的 CRD OpenAPI Schema，包括展开的
`PodTemplateSpec`、`IntOrString` 和资源 Quantity 等 Kubernetes 类型。

生成器及其运行时包在 `apps/rlark-ui/package.json` 中固定版本，并要求 Node.js 22。仓库
wrapper 直接调用生成器 API，并按文件名排序读取 CRD：

```bash
npm ci --prefix apps/rlark-ui
npm run generate:crd-models --prefix apps/rlark-ui
```

### CRD 生成器与固定版本工具链

`controller-gen` 由 `api/Makefile` 中的 `CONTROLLER_TOOLS_VERSION` 固定版本，并以带
版本戳的文件名安装到仓库内被 gitignore 的 `api/bin` 目录。`api/hack/generate-clients.sh`
调用的客户端生成器由该脚本中的 `KUBE_CODEGEN_TAG` 固定版本，并安装到同一目录。需要直接
使用该生成器的脚本，请用 `make -C api controller-gen` 安装，并用
`make -s -C api print-controller-gen` 获取其路径。

不要把 `CONTROLLER_GEN` 指向共享 `GOPATH/bin` 中的生成器。CI 中该目录跨 job、跨项目共享，
其中名为 `controller-gen` 的文件可能是陈旧的、甚至根本无法执行，而 make 仍会认为安装已
完成并跳过安装，最终在生成过程中以退出码 127 失败。安装规则在安装后会执行
`controller-gen --version`，这样不可执行的二进制会在安装阶段就失败。

升级 `CONTROLLER_TOOLS_VERSION` 是一次有意的变更：该版本会写入每个生成的 CRD 清单头部，
因此必须重新生成全部产物，并与版本钉定在同一次提交中一起提交。

### Protobuf 产物与固定版本工具链

`sdks/embodied-runtime-go/gen`、`sdks/embodied-runtime-python/embodied_runtime/gen` 和 `sdks/execution-go/gen` 下的 gRPC 产物是签入仓库的，并且每个生成文件都会在头部写入生成器版本。使用不同版本的 `protoc`、`protoc-gen-go`、`protoc-gen-go-grpc` 或 `grpcio-tools` 生成会重写全部产物，因此整个仓库统一使用一套固定版本的工具链：

- `proto/toolchain.mk` 是 `protoc`、`protoc-gen-go`、`protoc-gen-go-grpc` 和 `buf` 版本的唯一来源，两棵 proto 树（`proto/embodied-runtime` 和 `proto/execution`）都 include 它。
- `sdks/embodied-runtime-python/requirements-dev.txt` 固定 Python 产物所用的 `grpcio-tools` 版本。

不要用 `PATH` 上恰好存在的 `protoc` 生成。先把固定版本的发布包装到被 gitignore 的 `proto/bin` 目录；Go 插件和 `buf` 会自动安装到同一目录，文件名带版本戳：

```bash
# 下载固定版本的 protoc 到 proto/bin
make protoc-install

# 重新生成所有产物
make proto

# 签入的产物与 .proto 源不一致时失败
make verify-proto-generated
```

`make proto` 会先执行 `protoc-check`：当解析到的 `protoc` 自报版本与固定版本不一致时直接失败，因此本机安装的其他版本不可能悄悄重写产物。一旦 `proto/bin/protoc` 存在，它的优先级高于 `PATH`。

如果 `python3` 不是装有固定版本 `grpcio-tools` 的解释器，请把 Python 生成器指向正确的解释器，而不是把该版本装到全局环境：

```bash
python3 -m venv /tmp/rlark-protogen
/tmp/rlark-protogen/bin/pip install --requirement sdks/embodied-runtime-python/requirements-dev.txt
make proto PYTHON=/tmp/rlark-protogen/bin/python
```

!!! warning "给编码智能体的提醒"
    禁止使用未固定版本的 `protoc` 重新生成 protobuf 产物，也禁止为了迁就本机已安装的版本而放宽 `proto/toolchain.mk` 中的版本固定。请执行 `make protoc-install` 把固定版本安装到仓库内的 `proto/bin` 目录，Python 生成器在需要时用 `PYTHON=<解释器>` 覆盖。如果 diff 重写了全部生成文件，那是工具链版本不匹配，而不是真实变更。升级版本固定是一次有意的变更，必须在同一个提交中重新生成所有产物树。

## IDE 推荐

推荐 VS Code 扩展：

- Go
- ESLint
- Prettier
- YAML
