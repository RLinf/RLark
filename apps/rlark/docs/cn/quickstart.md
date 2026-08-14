# 快速开始

本文档指导你在本地环境搭建 rlark 并运行第一个训练任务。

## 前置条件

| 工具 | 版本 | 说明 |
|------|------|------|
| Go | >= 1.26.5 | 编译 Go 代码 |
| Docker | >= 24.0 | 运行 kcp 和 kind 集群 |
| kind | >= 0.20 | 运行本地 k8s 数据面集群 |
| kubectl | >= 1.28 | 与集群交互 |
| jq | >= 1.6 | 解析 JSON 响应 |

## 1. 编译

```bash
git clone https://github.com/RLinf/RLark
cd RLark
# Linux: make build
# macOS: GOOS=darwin make build
make build
```

编译完成后，`apps/rlark/bin/` 目录下会生成以下二进制：

```
apps/rlark/bin/
├── server                # 控制面 Server
├── gateway               # API Gateway
├── controller-manager    # 控制面控制器
├── agent                 # 数据面 Agent
├── network-sidecar       # Pod 网络 Sidecar
└── rlarkadm              # 部署 CLI
```

## 2. 本地开发环境搭建

rlark 支持通过 Docker Compose 快速搭建本地开发环境，包含 kcp 集群、数据库和必要的运行时组件。

### 2.1 创建运行时目录

```bash
mkdir -p ~/.rlark/certs
```

### 2.2 启动控制面

```bash
# 使用 Docker Compose 启动 kcp 和 PostgreSQL
docker compose -f apps/rlark/docs/examples/docker-compose.yml up -d

# 等待服务就绪（约 30 秒）
# 检查状态
docker compose -f apps/rlark/docs/examples/docker-compose.yml ps

# 从 kcp 容器中提取 admin kubeconfig
docker cp kcp:/.kcp/admin.kubeconfig ~/.rlark/admin.kubeconfig

# 将 Docker 内部 IP 替换为 localhost（macOS/Linux Docker Desktop）
sed -i '' 's|https://[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+:6443|https://localhost:6443|g' ~/.rlark/admin.kubeconfig 2>/dev/null || \
sed -i    's|https://[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+:6443|https://localhost:6443|g' ~/.rlark/admin.kubeconfig

# 安装 CRD 到 kcp（启动组件前必须执行）
kubectl --kubeconfig ~/.rlark/admin.kubeconfig apply -f api/config/crd/bases/
```

组件包括：
- **kcp**：API Server（控制面集群）
- **postgresql**：rlark 运行数据库

### 2.3 创建数据面 Kind 集群

```bash
# 创建 kind 集群（如果尚未创建）
kind create cluster --name rlark-data

# 导出 kubeconfig
kind get kubeconfig --name rlark-data > ~/.rlark/kind-kubeconfig
```

### 2.4 启动控制面组件

打开三个终端，分别启动 Server、Controller-Manager 和 Gateway：

```bash
# 终端 1：启动 Server
./apps/rlark/bin/server \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --https-port 8443 \
  --ssh-port 2222 \
  --auto-sign-tls-ca-cert \
  --db-config apps/rlark/docs/examples/db-config.yaml

# 终端 2：启动 Controller-Manager
./apps/rlark/bin/controller-manager \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --server-address https://localhost:8443 \
  --leader-elect=false \
  --metrics-bind-address :0

# 终端 3：启动 Gateway
./apps/rlark/bin/gateway \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --addr :8080 \
  --server-address https://localhost:8443 \
  --db-config apps/rlark/docs/examples/db-config.yaml
```

### 2.5 生成 Agent 证书

Agent 需要客户端证书才能与控制面进行认证。通过 Gateway API 申请：

```bash
# 生成 Agent 证书（请将 "my-cluster" 替换为你的集群名称）
RESP=$(curl -s -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"cluster_id": "my-cluster"}')
echo "$RESP" | jq -r .ca_cert > ~/.rlark/certs/ca-cert.pem
echo "$RESP" | jq -r .agent_cert > ~/.rlark/certs/cert.pem
echo "$RESP" | jq -r .agent_key > ~/.rlark/certs/key.pem
```

这会在 `~/.rlark/certs/` 下生成三个文件：
- `ca-cert.pem` — CA 证书，用于验证 Server
- `cert.pem` — Agent 客户端证书（X.509，由控制面 CA 签发）
- `key.pem` — Agent 私钥

### 2.6 启动数据面 Agent

```bash
./apps/rlark/bin/agent \
  --kubeconfig ~/.rlark/kind-kubeconfig \
  --server-address https://localhost:8443 \
  --client-cert ~/.rlark/certs/cert.pem \
  --client-key ~/.rlark/certs/key.pem \
  --ca-cert ~/.rlark/certs/ca-cert.pem \
  --mode both \
  --rlark-server-ssh-address localhost:2222
```

## 3. 验证环境

### 3.1 检查 Agent 注册

Agent 启动后，会在控制面自动注册节点：

```bash
# 查看注册的 Node
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes?namespace=default" | jq .
```

### 3.2 检查控制面

```bash
# 验证 API 是否正常工作（应返回节点列表）
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes"
```

## 4. 创建第一个训练任务

### 4.1 创建 Domain

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/domains" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Domain",
    "metadata": { "name": "my-first-domain" },
    "spec": { "cidr": "10.0.1.0/24" }
  }'
```

### 4.2 创建 Job

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": { "name": "hello-world" },
    "spec": {
      "domain": "my-first-domain",
      "tasks": [
        {
          "name": "trainer",
          "head": true,
          "role": "Actor",
          "agentType": "Kubernetes",
          "kubernetes": {
            "workload": {
              "kind": "Deployment",
              "replicas": 1,
              "template": {
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "busybox:latest",
                      "command": ["sh", "-c", "echo Hello from rlark! && sleep 3600"],
                      "resources": {
                        "limits": { "cpu": "100m", "memory": "128Mi" }
                      }
                    }
                  ]
                }
              }
            }
          }
        }
      ]
    }
  }'
```

### 4.3 查看任务状态

```bash
# 查看 Job 状态
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/hello-world" | jq '.status'

# 查看 Task 状态
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/tasks?namespace=default&labelSelector=rlinf.io/job=hello-world" | jq '.items[].status'

# 查看 Pod 日志
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/hello-world/logs" | jq .
```

### 4.4 在 kind 集群中验证

```bash
# 数据面应该能看到 Deployment
kubectl --kubeconfig ~/.rlark/kind-kubeconfig get deployment -A

# 查看 Pod
kubectl --kubeconfig ~/.rlark/kind-kubeconfig get pods -A
```

## 5. 使用 Web UI

```bash
# 启动前端开发服务器
cd apps/rlark-ui && npm install && npm run dev
```

浏览器访问 `http://localhost:5173`，可以看到：
- Dashboard：系统概览
- Nodes：节点列表和资源使用
- Jobs：创建和管理训练任务
- Workflows：DAG 工作流编排

## 6. 清理

```bash
# 停止所有组件
docker compose -f apps/rlark/docs/examples/docker-compose.yml down

# 删除 kind 集群
kind delete cluster --name rlark-data

# 清理 kcp 数据
docker compose -f apps/rlark/docs/examples/docker-compose.yml down -v

# 删除运行时文件
rm -rf ~/.rlark
```

## 7. 下一步

- 阅读 [核心概念](concepts.md) 了解资源模型
- 阅读 [架构设计](architecture.md) 了解实现原理
- 阅读 [API 示例](api/examples.md) 了解完整 API 用法
- 阅读 [部署指南](deployment.md) 了解生产环境部署