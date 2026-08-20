# API 调用示例

本页提供 RLark Gateway HTTP API 的端到端调用示例，重点围绕 **Kubernetes 运行时**（`agentType=Kubernetes`）展开。资源操作和字段定义请查看 [API 参考](reference.md)，机器可读的接口契约请查看 [OpenAPI 规范](swagger.yaml)。

!!! warning "认证限制"
    登录接口只校验内置 Web UI 凭据，不会返回可用于后续请求的 Bearer token 或会话 cookie，其他 Gateway API 也不会根据该结果执行授权。以下命令只应在可信网络中运行，或通过强制实施认证和授权的入口访问。

## 约定

- 独立运行的 Gateway 默认监听 `http://localhost:8080`。通过 `rlarkadm` 部署时，Gateway 在集群内部暴露于 `8090` 端口，浏览器流量经由 UI 服务路由。
- CRD API 根路径：`/api/v1/rlinf.io/v1alpha1`。
- `nodes`、`tasks` 等命名空间级资源必须在查询字符串中指定 `namespace=<namespace>`。
- `jobs`、`workflows` 等集群级资源不使用命名空间查询参数。
- `spec.agentType` 可取 `Kubernetes`、`Docker` 或 `Raw`。目前仅实现 Kubernetes 运行时，Docker 和 Raw 尚在规划中。
- `spec.role` 为必填字段，可取 `Actor`、`Rollout` 或 `Env`。
- `kubernetes.workload.template` 是 Kubernetes `corev1.PodTemplateSpec`。

为后续示例统一设置基础 URL：

```bash
export RLARK_GATEWAY=http://localhost:8080
```

## 1. 查询节点并设置不可调度

Node 由 Agent 注册和上报。用户通常只需列出或查看节点，以及更改其可调度状态。

```bash
# 列出命名空间中的 Node。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes?namespace=default"

# 获取单个 Node。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default"

# 将 Node 标记为不可调度。
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"unschedulable":true}}'
```

## 2. 创建和查看 Job

用户通过完整的 Task 模板创建 Job。Job 控制器创建对应的命名空间级 Task 资源，Agent 随后创建下层 workload。

```bash
curl -X POST "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": {
      "name": "ppo-cartpole",
      "labels": {"framework": "ppo"}
    },
    "spec": {
      "tasks": [
        {
          "name": "actor-head",
          "head": true,
          "role": "Actor",
          "agentType": "Kubernetes",
          "nodeSelector": {"rlark.io/cluster-id": "cluster-a"},
          "kubernetes": {
            "workload": {
              "kind": "Deployment",
              "replicas": 1,
              "template": {
                "metadata": {"labels": {"app": "ppo-cartpole"}},
                "spec": {
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "registry.example.com/rl/ppo:v1",
                      "command": ["python", "main.py"],
                      "args": ["--role=head"],
                      "resources": {
                        "limits": {"nvidia.com/gpu": "1"}
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

镜像、命令、环境变量、资源和卷应放在 `kubernetes.workload.template.spec.containers` 下，而不是作为 Task 的顶层字段。

```bash
# 按标签列出 Job。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs?labelSelector=framework=ppo"

# 获取 Job，包括其 status 字段。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole"

# 停止 Job。
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"stopped":true}}'

# 删除 Job。
curl -X DELETE "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole"
```

Merge Patch 会整体替换数组。修补 `tasks` 或 `jobTemplates` 时，应发送包含 `role` 等必填字段的完整数组元素，或者改用 JSON Patch。

## 3. 查看控制器管理的 Task

Task 由 Job 控制器创建，API 客户端应将其视为只读资源。

```bash
# 列出某个 Job 的 Task。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/tasks?namespace=default&labelSelector=rlinf.io/job=ppo-cartpole"

# 获取单个 Task，包括其 status 字段。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/tasks/ppo-cartpole-actor-head?namespace=default"
```

## 4. 创建 Workflow

Workflow 包含通过依赖关系连接的 Job 模板。每个 `jobTemplates[].spec` 都是完整的 Job spec。

```bash
curl -X POST "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Workflow",
    "metadata": {"name": "training-pipeline"},
    "spec": {
      "jobTemplates": [
        {
          "name": "prepare",
          "dependencies": [],
          "spec": {
            "tasks": [
              {
                "name": "prepare-data",
                "role": "Env",
                "agentType": "Kubernetes",
                "kubernetes": {
                  "workload": {
                    "kind": "Deployment",
                    "replicas": 1,
                    "template": {
                      "spec": {
                        "containers": [
                          {"name": "prepare", "image": "registry.example.com/rl/prepare:v1"}
                        ]
                      }
                    }
                  }
                }
              }
            ]
          }
        },
        {
          "name": "train",
          "dependencies": ["prepare"],
          "spec": {
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
                        "containers": [
                          {"name": "trainer", "image": "registry.example.com/rl/train:v1"}
                        ]
                      }
                    }
                  }
                }
              }
            ]
          }
        }
      ]
    }
  }'

curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/workflows/training-pipeline"
```

## 5. UI 凭据校验

仅接受内置用户名 `admin` 和 `user`。成功响应为 `{"ok":true,"role":"admin"}` 或 `{"ok":true,"role":"user"}`。

```bash
curl -X POST "$RLARK_GATEWAY/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

该响应仅用于当前 Web UI 的登录门禁，不会为后续 API 调用授予凭据。

## 6. 其他 Gateway 端点

```bash
# 列出已连接的集群。
curl "$RLARK_GATEWAY/api/v1/clusters"

# 跨集群列出 StorageClass，或按集群 ID 过滤。
curl "$RLARK_GATEWAY/api/v1/storage/storageclass"
curl "$RLARK_GATEWAY/api/v1/storage/storageclass?clusters=cluster-a,cluster-b"

# 列出存储提供商。
curl "$RLARK_GATEWAY/api/v1/storage/storageclass/provider"

# 读取 Job 日志。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole/logs"

# 列出用户的 SSH 公钥。
curl "$RLARK_GATEWAY/api/v1/ssh-user-keys?user=alice"
```

存储上传、下载和删除操作请参阅 [存储 API](../storage-api.md)。
