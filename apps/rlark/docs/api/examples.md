# API 调用样例

本文件提供 rlark gateway HTTP API 的端到端调用样例，重点围绕 **Kubernetes 接入形态**（`agentType=Kubernetes`）展开，便于前端/Agent 基于本文件与 [reference.md](./reference.md) 进行开发。

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

---

## 1. Node 节点查询（只读 + 污点）

Node 由各节点上的 agent-controller 自动注册与上报，前端一般只做查询和调度控制。

### 1.1 列出节点（按 label 过滤）

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes?namespace=default&labelSelector=agent-type=kubernetes"
```

```json
{
  "apiVersion": "rlinf.io/v1alpha1",
  "kind": "NodeList",
  "metadata": { "resourceVersion": "12345" },
  "items": [
    {
      "metadata": { "name": "node-k8s-01" },
      "spec": { "agentType": "Kubernetes", "unschedulable": false },
      "status": {
        "phase": "Online",
        "addresses": [
          { "type": "InternalIP", "address": "10.0.1.23" },
          { "type": "Hostname", "address": "node-k8s-01" }
        ],
        "capacity": { "cpu": "32", "memory": "128Gi", "nvidia.com/gpu": "8" },
        "allocatable": { "cpu": "30", "memory": "120Gi", "nvidia.com/gpu": "8" },
        "used": { "cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "2" },
        "nodeInfo": {
          "architecture": "amd64",
          "operatingSystem": "linux",
          "kernelVersion": "5.15.0-91-generic",
          "agentVersion": "0.1.0"
        }
      }
    }
  ]
}
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

---

## 2. Job 作业管理（前端主要操作对象）

Job 是 cluster-scoped，聚合多个 Task（如 RL 训练的 Actor/Rollout/Env）。每个 task 模板带 `name`、`head`（是否为主节点）及内嵌的 `TaskSpec`。**只要 task 模板完备，job-controller 会自动调谐创建对应的 Task**（Task 名称由 `<jobName>-<taskName>` 派生，namespace 当前固定为 `default`，并打上 `rlinf.io/job=<jobName>` 标签）。

### 2.1 创建一个完整的 RL 训练 Job

包含 3 个 task：1 个 head Actor（Deployment，1 副本）、4 个 worker Actor（Deployment）、1 个 Rollout（StatefulSet）。全部使用 K8s 接入。

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": {
      "name": "ppo-cartpole-v1",
      "labels": { "framework": "ppo", "env": "cartpole" }
    },
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
                "metadata": { "labels": { "job": "ppo-cartpole-v1", "task": "actor-head" } },
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "registry.example.com/rl/ppo:v0.3.0",
                      "command": ["python", "main.py"],
                      "args": ["--role=head", "--port=8000"],
                      "env": [
                        { "name": "JOB_NAME", "value": "ppo-cartpole-v1" },
                        { "name": "TASK_NAME", "value": "actor-head" }
                      ],
                      "ports": [{ "containerPort": 8000 }],
                      "resources": { "limits": { "nvidia.com/gpu": "1", "cpu": "8", "memory": "32Gi" } }
                    }
                  ]
                }
              }
            }
          }
        },
        {
          "name": "actor-worker",
          "role": "Actor",
          "agentType": "Kubernetes",
          "kubernetes": {
            "workload": {
              "kind": "Deployment",
              "replicas": 4,
              "template": {
                "metadata": { "labels": { "job": "ppo-cartpole-v1", "task": "actor-worker" } },
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "registry.example.com/rl/ppo:v0.3.0",
                      "command": ["python", "main.py"],
                      "args": ["--role=worker", "--head=actor-head:8000"],
                      "env": [
                        { "name": "JOB_NAME", "value": "ppo-cartpole-v1" },
                        { "name": "TASK_NAME", "value": "actor-worker" }
                      ],
                      "resources": { "limits": { "nvidia.com/gpu": "1", "cpu": "8", "memory": "32Gi" } }
                    }
                  ]
                }
              }
            }
          }
        },
        {
          "name": "rollout",
          "role": "Rollout",
          "agentType": "Kubernetes",
          "tensorBoardDir": "/data/tensorboard/ppo-cartpole-v1",
          "kubernetes": {
            "workload": {
              "kind": "StatefulSet",
              "replicas": 2,
              "template": {
                "metadata": { "labels": { "job": "ppo-cartpole-v1", "task": "rollout" } },
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "rollout",
                      "image": "registry.example.com/rl/rollout:v0.3.0",
                      "args": ["--env=CartPole-v1", "--head=actor-head:8000"],
                      "resources": { "limits": { "cpu": "4", "memory": "8Gi" } },
                      "volumeMounts": [
                        { "name": "tb", "mountPath": "/data/tensorboard" }
                      ]
                    }
                  ],
                  "volumes": [
                    { "name": "tb", "persistentVolumeClaim": { "claimName": "tb-pvc" } }
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

响应 `201 Created`（`status` 初始为 Pending，由 job-controller 填充）：

```json
{
  "apiVersion": "rlinf.io/v1alpha1",
  "kind": "Job",
  "metadata": { "name": "ppo-cartpole-v1", "creationTimestamp": "2026-06-25T03:20:00Z" },
  "spec": { "tasks": ["..."] },
  "status": {
    "phase": "Pending",
    "startTime": "2026-06-25T03:20:00Z",
    "tasks": [
      { "name": "actor-head", "phase": "" },
      { "name": "actor-worker", "phase": "" },
      { "name": "rollout", "phase": "" }
    ]
  }
}
```

> job-controller 进入 Pending 后会逐个 `dispatchTasks` 创建 Task，Task 名称形如 `ppo-cartpole-v1-actor-head`，namespace 为 `default`，并设置 OwnerReference 指向 Job。

### 2.2 查询 Job 状态

```bash
# 完整对象
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1"

# 仅 status 子资源
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1/status"
```

运行中的状态（job-controller 汇总各 Task phase 后驱动状态机迁移）：

```json
{
  "status": {
    "phase": "Running",
    "startTime": "2026-06-25T03:20:10Z",
    "tasks": [
      { "name": "actor-head", "phase": "Running" },
      { "name": "actor-worker", "phase": "Running" },
      { "name": "rollout", "phase": "Running" }
    ],
    "conditions": [
      { "type": "Ready", "status": "True", "lastTransitionTime": "2026-06-25T03:20:30Z", "reason": "TasksRunning" }
    ]
  }
}
```

Job 状态机迁移规则（见 [statemachine.go](../../pkg/controllermanager/job/statemachine.go)）：

| 事件 | 来源 | 迁移 |
|---|---|---|
| `init` | Job 首次调谐 | `""` → `Pending` |
| `tasks-running` | 任一 Task 进入 Running | `Pending` → `Running` |
| `all-tasks-succeeded` | 所有 Task Succeeded | `Running` → `Succeeded` |
| `any-task-failed` | 任一 Task Failed | `Running` → `Failed` |

### 2.3 列出 Job（按 label 选择）

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs?labelSelector=framework=ppo"
```

### 2.4 更新 Job（替换 spec，需带 resourceVersion）

PUT 为全量替换；建议先 GET 取得 `resourceVersion`，再回写以避免冲突。修改 task 模板后，job-controller 会在下一次调谐时同步到对应的 Task。

```bash
curl -X PUT "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": {
      "name": "ppo-cartpole-v1",
      "resourceVersion": "12345"
    },
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
                "metadata": { "labels": { "job": "ppo-cartpole-v1", "task": "actor-head" } },
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "registry.example.com/rl/ppo:v0.4.0",
                      "args": ["--role=head", "--port=8000", "--lr=0.0003"],
                      "resources": { "limits": { "nvidia.com/gpu": "1" } }
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

### 2.5 Patch Job（merge-patch 调整副本数）

```bash
curl -X PATCH "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{ "spec": { "tasks": [ { "name": "actor-worker", "role": "Actor", "agentType": "Kubernetes", "kubernetes": { "workload": { "replicas": 8 } } } ] } }'
```

> 注意：merge-patch 对数组是整体替换， patch task 数组时需保证元素包含 `role` 等必填字段，或改用 JSON-Patch 精确定位数组元素。

### 2.6 删除 Job

```bash
curl -X DELETE "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1"
```

> 删除 Job 后，由于其 OwnerReference 关系，job-controller 创建的 Task 会被垃圾回收。

---

## 3. Task 任务查询（只读）

Task 由 job-controller 根据 Job 的 task 模板调谐创建，前端**不应**直接 Create/Update/Delete Task，但可以查询以观察底层 workload 的实际运行状态。

### 3.1 列出某 Job 下的 Task

job-controller 创建 Task 时会打上 `rlinf.io/job=<jobName>` 标签，可用 labelSelector 过滤：

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/tasks?namespace=default&labelSelector=rlinf.io/job=ppo-cartpole-v1"
```

```json
{
  "apiVersion": "rlinf.io/v1alpha1",
  "kind": "TaskList",
  "items": [
    {
      "metadata": { "name": "ppo-cartpole-v1-actor-head", "namespace": "default", "labels": { "rlinf.io/job": "ppo-cartpole-v1" } },
      "spec": { "role": "Actor", "agentType": "Kubernetes", "kubernetes": { "workload": { "kind": "Deployment", "replicas": 1 } } },
      "status": { "phase": "Running", "observedNodes": ["node-k8s-01"], "retryCount": 0 }
    },
    {
      "metadata": { "name": "ppo-cartpole-v1-actor-worker", "namespace": "default" },
      "spec": { "role": "Actor", "agentType": "Kubernetes", "kubernetes": { "workload": { "kind": "Deployment", "replicas": 4 } } },
      "status": { "phase": "Running" }
    },
    {
      "metadata": { "name": "ppo-cartpole-v1-rollout", "namespace": "default" },
      "spec": { "role": "Rollout", "agentType": "Kubernetes", "kubernetes": { "workload": { "kind": "StatefulSet", "replicas": 2 } } },
      "status": { "phase": "Running" }
    }
  ]
}
```

### 3.2 查询单个 Task 状态

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/tasks/ppo-cartpole-v1-actor-head/status?namespace=default"
```

```json
{
  "status": {
    "phase": "Running",
    "observedNodes": ["node-k8s-01"],
    "conditions": [
      { "type": "Ready", "status": "True", "lastTransitionTime": "2026-06-25T03:21:00Z", "reason": "PodRunning" }
    ],
    "startTime": "2026-06-25T03:20:30Z",
    "retryCount": 0
  }
}
```

---

## 4. Workflow 工作流管理（多 Job DAG 编排）

Workflow 是 cluster-scoped，由若干 `jobTemplates` 组成，每个模板通过 `dependencies` 声明前置依赖，形成 DAG。每个 `jobTemplates[].spec` 即一个完整的 `JobSpec`，结构与独立 Job 完全一致。

### 4.1 创建一个两阶段 Workflow（数据准备 → 训练）

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Workflow",
    "metadata": { "name": "ppo-pipeline-v1" },
    "spec": {
      "jobTemplates": [
        {
          "name": "prepare-data",
          "dependencies": [],
          "spec": {
            "tasks": [
              {
                "name": "prep",
                "role": "Env",
                "agentType": "Kubernetes",
                "kubernetes": {
                  "workload": {
                    "kind": "Deployment",
                    "replicas": 1,
                    "template": {
                      "metadata": { "labels": { "step": "prepare" } },
                      "spec": {
                        "restartPolicy": "Always",
                        "containers": [
                          {
                            "name": "prep",
                            "image": "registry.example.com/rl/prep:v0.3.0",
                            "args": ["--dataset=cartpole", "--out=/data/processed", "--oneshot"],
                            "volumeMounts": [{ "name": "data", "mountPath": "/data" }]
                          }
                        ],
                        "volumes": [{ "name": "data", "persistentVolumeClaim": { "claimName": "data-pvc" } }]
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
          "dependencies": ["prepare-data"],
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
                      "metadata": { "labels": { "step": "train" } },
                      "spec": {
                        "restartPolicy": "Always",
                        "containers": [
                          {
                            "name": "trainer",
                            "image": "registry.example.com/rl/ppo:v0.3.0",
                            "args": ["--data=/data/processed"],
                            "resources": { "limits": { "nvidia.com/gpu": "1" } }
                          }
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
```

> 说明：`prepare-data` 无依赖会先调度；`train` 依赖 `prepare-data`，在前者成功后才会启动。每个 jobTemplate 的 `spec` 字段即完整 `JobSpec`，前端可复用 Job 表单组件。

### 4.2 查询 Workflow 状态

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/workflows/ppo-pipeline-v1/status"
```

```json
{
  "status": {
    "phase": "Running",
    "startTime": "2026-06-25T03:30:00Z",
    "jobs": [
      { "name": "prepare-data", "phase": "Succeeded" },
      { "name": "train", "phase": "Running" }
    ]
  }
}
```

### 4.3 列出 / 删除 Workflow

```bash
# 列表
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/workflows?limit=10"

# 删除
curl -X DELETE "http://localhost:8080/api/v1/rlinf.io/v1alpha1/workflows/ppo-pipeline-v1"
```

### 4.4 Patch Workflow（merge-patch 调整 job 模板）

```bash
curl -X PATCH "http://localhost:8080/api/v1/rlinf.io/v1alpha1/workflows/ppo-pipeline-v1" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{ "spec": { "jobTemplates": [ { "name": "train", "dependencies": ["prepare-data"], "spec": { "tasks": [ { "name": "actor-head", "head": true, "role": "Actor", "agentType": "Kubernetes", "kubernetes": { "workload": { "kind": "Deployment", "replicas": 2, "template": { "metadata": { "labels": { "step": "train" } }, "spec": { "restartPolicy": "Always", "containers": [ { "name": "trainer", "image": "registry.example.com/rl/ppo:v0.4.0" } ] } } } } } ] } } ] } }'
```

> 注意：merge-patch 对 `jobTemplates` 数组是整体替换，需带上完整的 job 模板结构。

---

## 5. 其他接入形态（Docker / Raw）在 Job task 模板中的用法

> **开发状态**：Docker 和 Raw 接入形态的代码框架已搭建（`pkg/agent/controllers/base/types.go`），但实际运行时实现尚为 TODO（`LocalDockerClient any // TODO`、`LocalRawClient any // TODO`）。以下展示的 API 结构已定稿，可供前端开发和测试使用。

Job 的 task 模板支持三种互斥的 `agentType`，前端表单应按 agentType 切换分片。以下展示在 Job 内使用 Docker / Raw task 的写法（完整字段见 [reference.md](./reference.md)）。

### 5.1 含 Docker task 的 Job

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": { "name": "docker-job-01" },
    "spec": {
      "tasks": [
        {
          "name": "docker-actor",
          "head": true,
          "role": "Actor",
          "agentType": "Docker",
          "docker": {
            "containers": [
              {
                "name": "trainer",
                "image": "pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime",
                "command": ["python"],
                "args": ["train.py"],
                "environment": [{ "name": "CUDA_VISIBLE_DEVICES", "value": "0" }],
                "volumes": [{ "source": "/host/data", "target": "/data" }]
              }
            ]
          }
        }
      ]
    }
  }'
```

### 5.2 含 Raw task 的 Job（裸进程/二进制 + artifact）

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": { "name": "raw-job-01" },
    "spec": {
      "tasks": [
        {
          "name": "raw-actor",
          "head": true,
          "role": "Actor",
          "agentType": "Raw",
          "raw": {
            "artifact": "https://repo.example.com/bin/actor-v0.3.0.tar.gz",
            "entrypoint": ["./actor", "--port=8000"],
            "environment": [{ "name": "RANK", "value": "0" }]
          }
        }
      ]
    }
  }'
```

---

## 6. 通用查询参数样例

适用于所有 `GET ...` 列表接口。

| 场景 | 示例 URL |
|---|---|
| 分页 | `tasks?namespace=default&limit=20&continue=<token>` |
| label 选择 | `jobs?labelSelector=framework=ppo,env=cartpole` |
| 字段选择 | `tasks?namespace=default&fieldSelector=status.phase=Running` |
| 按名称前缀（label） | `tasks?namespace=default&labelSelector=app%3Dmnist` |

---

## 7. 前端开发要点

1. **Scope 决定是否传 namespace**：`nodes`/`tasks` 必传 `?namespace=`；`jobs`/`workflows` 不要传。
2. **前端主要操作 Job 和 Workflow**：Task 由 job-controller 调谐创建，Node 由 agent-controller 上报，前端对这两者只读（Node 支持打污点）。
3. **PodTemplateSpec 是原生 K8s 对象**：`spec.kubernetes.workload.template` 可直接复用社区 K8s 表单/校验组件（containers、env、resources、volumes、probe 等）。
4. **三种 agentType 互斥**：`kubernetes` / `docker` / `raw` 三选一；前端表单应按 agentType 切换分片。
5. **Status 为只读**：`status` 由系统填充，前端不应在 PUT/PATCH 的 body 里带 `status`（如需更新状态用 `/status` 子资源，一般前端只读）。
6. **PUT 需 resourceVersion**：全量更新前先 GET 拿到 `metadata.resourceVersion`，避免 `409 Conflict`。
7. **PATCH 用 merge-patch**：`Content-Type: application/merge-patch+json` 适合增量更新（如改副本数、改镜像）；但对数组是整体替换，patch `tasks`/`jobTemplates` 时需带完整元素，或改用 JSON-Patch。
8. **Job/Workflow 的 task/job 模板**：内嵌的 `spec` 即 `JobSpec`，结构与独立 Job 完全一致，前端可复用同一表单组件。
9. **Task 名称派生规则**：job-controller 创建 Task 时名称为 `<jobName>-<taskName>`，namespace 为 `default`，标签 `rlinf.io/job=<jobName>`；前端可用此标签查询某 Job 下的所有 Task。

---

## 8. 认证 API

### 8.1 用户登录

```bash
curl -X POST "http://localhost:8080/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "your-password"}'
```

```json
{"ok": true, "role": "admin"}
```

---

## 9. 存储 API

### 9.1 列出 StorageClass

```bash
# 列出所有集群的 StorageClass
curl "http://localhost:8080/api/v1/storage/storageclass"

# 按集群过滤
curl "http://localhost:8080/api/v1/storage/storageclass?clusters=agent-beijing,agent-shanghai"
```

```json
{
  "data": {
    "ceph-rbd": {
      "name": "ceph-rbd",
      "clusters": ["agent-beijing"],
      "description": "Ceph RBD StorageClass",
      "bucket": ""
    }
  },
  "success": true
}
```

### 9.2 列出存储提供商

```bash
curl "http://localhost:8080/api/v1/storage/storageclass/provider"
```

---

## 10. 集群列表 API

```bash
curl "http://localhost:8080/api/v1/clusters"
```

```json
{
  "data": [
    {
      "id": "agent-beijing",
      "name": "Beijing GPU Cluster",
      "type": "cloud",
      "phase": "Online",
      "cloudNodes": 4,
      "embodiedNodes": 0,
      "robots": 0,
      "gpuModels": ["A100", "V100"],
      "cpuUsage": 0.45,
      "gpuUsage": 0.72,
      "runningJobs": 3,
      "description": "Beijing GPU training cluster"
    }
  ],
  "success": true
}
```

---

## 11. Job 日志 API

```bash
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole-v1/logs"
```

```json
{
  "pods": [
    {
      "taskName": "actor-head",
      "podName": "actor-head-0",
      "phase": "Running",
      "node": "gpu-node-01",
      "logs": "Training started...\nEpoch 1/100: loss=0.45..."
    }
  ]
}
```
