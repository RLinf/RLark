# API 调用样例

本文件提供 rlark gateway HTTP API 的端到端调用样例，重点围绕 **Kubernetes 接入形态**（`agentType=Kubernetes`）展开，便于前端/Agent 基于本文件与 [reference.md](../api/reference.md) 进行开发。

## 约定

- Gateway 默认监听 `http://localhost:8080`
- API 根路径：`/api/v1/rlinf.io/v1alpha1`
- 资源范围（Scope）：
  - **Namespaced**（`nodes`、`tasks`）：必须在 query 中带 `namespace=<ns>`
  - **Cluster**（`jobs`、`workflows`）：不需要 `namespace`
- 接入形态（`spec.agentType`）：`Kubernetes` / `Docker` / `Raw`，三者互斥
- 任务角色（`spec.role`，必填）：`Actor` / `Rollout` / `Env`
- K8s Workload `kind`：`Deployment` / `DaemonSet` / `StatefulSet` / `CloneSet`
- `kubernetes.workload.template` 是标准 Kubernetes `corev1.PodTemplateSpec`，包含 `metadata`（labels 等）与 `spec`（containers/volumes 等）

### 资源归属与前端可操作性

| 资源 | 创建者 | 前端可操作 |
|---|---|---|
| `Node` | agent-controller 的 node controller 从本地向控制面 push（见 [push.go](../../pkg/agent/controllers/node/push.go)） | 仅 **List / Get / 打污点（PATCH unschedulable）**，不建议前端 Create/Delete |
| `Task` | 控制面 job-controller 根据 Job 的 task 模板调谐创建（见 [job_controller.go](../../pkg/controllermanager/job/job_controller.go)、[sync.go](../../pkg/controllermanager/job/sync.go)） | **只读**（List/Get/Get Status），不建议前端 Create/Update/Delete |
| `Job` | 用户/前端创建 | **全量 CRUD**，前端主要操作对象 |
| `Workflow` | 用户/前端创建 | **全量 CRUD**，前端主要操作对象 |

> 核心用法：前端只需创建/更新 **Job**（含完整的 task 模板）或 **Workflow**（含 job 模板 DAG），控制面会自动调谐创建对应的 Task 与下层 workload。

所有样例使用 `curl`，响应体省略部分非关键字段（以 `...` 表示）。

## 1. Node 节点查询（只读 + 污点）

Node 由各节点上的 agent-controller 自动注册与上报，前端一般只做查询和调度控制。

### 1.1 列出节点（按 label 过滤）

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes?namespace=default&labelSelector=agent-type=kubernetes"
```

### 1.2 查询单个节点

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes/node-k8s-01?namespace=default"
```

### 1.3 设置节点不可调度（打污点，merge-patch）

```bash
curl -X PATCH "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes/node-k8s-01?namespace=default" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{ "spec": { "unschedulable": true } }'
```

## 2. Job 作业管理（前端主要操作对象）

Job 是 cluster-scoped，聚合多个 Task（如 RL 训练的 Actor/Rollout/Env）。每个 task 模板带 `name`、`head`（是否为主节点）及内嵌的 `TaskSpec`。

### 2.1 创建一个完整的 RL 训练 Job

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": { "name": "ppo-cartpole-v1" },
    "spec": {
      "tasks": [
        {
          "name": "actor-head",
          "head": true,
          "role": "Actor",
          "agentType": "Kubernetes",
          "kubernetes": {
            "workload": {
              "kind": "Deployment",
              "replicas": 1,
              "template": {
                "spec": {
                  "containers": [{
                    "name": "trainer",
                    "image": "pytorch/pytorch:2.3.0",
                    "resources": { "limits": { "nvidia.com/gpu": "1" } }
                  }]
                }
              }
            }
          }
        }
      ]
    }
  }'
```

### 2.2 查询 Job 状态

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1"
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1/status"
```

## 3. 更多示例

请查看英文版 [API Examples](../api/examples.md) 获取完整的 Node、Job、Task、Workflow、Docker/Raw 接入形态、认证、存储、日志等 API 调用示例。