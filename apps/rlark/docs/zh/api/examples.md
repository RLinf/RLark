# API 调用示例

本页提供 RLark Gateway HTTP API 的端到端调用示例，重点围绕 **Kubernetes 运行时**（`agentType=Kubernetes`）展开。资源操作和字段定义请查看 [API 参考](reference.md)，机器可读的接口契约请查看 [OpenAPI 规范](../../api/swagger.yaml)。

请先登录，并在后续 Gateway API 请求中以 `Authorization: Bearer <token>` 携带响应中的 `token`。token 默认 8 小时过期。

## 约定

- 独立运行的 Gateway 默认监听 `http://localhost:8080`。通过 `rlarkadm` 部署时，Gateway 在集群内部暴露于 `8090` 端口，浏览器流量经由 UI 服务路由。
- CRD API 根路径：`/api/v1/rlinf.io/v1alpha1`。
- `nodes`、`tasks` 等命名空间级资源必须在查询字符串中指定 `namespace=<namespace>`。
- `jobs` 等集群级资源不使用命名空间查询参数。
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

用户通过完整的 Task 模板创建 Job。Gateway 会将请求中的 `metadata.name` 保存为展示名，并在响应的 `metadata.name` 中返回系统生成的资源 ID；API 客户端必须保存该 ID，后续通过它访问任务。Job 控制器创建对应的命名空间级 Task 资源，Agent 随后创建下层 workload。

```bash
JOB_ID="$(curl -fsS -X POST "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs" \
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
  }' | jq -r '.metadata.name')"

echo "$JOB_ID" # jo-<16 位十六进制字符>
```

镜像、命令、环境变量、资源和卷应放在 `kubernetes.workload.template.spec.containers` 下，而不是作为 Task 的顶层字段。

### 通过 HostNetwork 访问未适配设备

如果 embodied-runtime 尚未适配某个网络设备，但设备能从数据面节点直接访问，可以在 Task 的 PodTemplate 中显式启用宿主机网络：

```json
{
  "nodeSelector": {"kubernetes.io/hostname": "worker-1"},
  "kubernetes": {
    "workload": {
      "kind": "Deployment",
      "replicas": 1,
      "template": {
        "metadata": {"labels": {"app": "vendor-device-client"}},
        "spec": {
          "hostNetwork": true,
          "dnsPolicy": "ClusterFirstWithHostNet",
          "containers": [
            {
              "name": "app",
              "image": "registry.example.com/vendor/device-sdk:latest",
              "command": ["sh", "-c"],
              "args": ["./device-client --address 192.168.10.20"]
            }
          ]
        }
      }
    }
  }
}
```

这种方式不会提供 embodied-runtime 的设备发现、资源隔离、controller、CLI 或 SDK 注入，设备驱动和生命周期管理由业务镜像负责。`hostNetwork` 会降低网络隔离并可能造成端口冲突，只应在可信数据面和专用设备节点使用。不要为此开启 `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES`；该变量会对多个任务全局启用旧版 privileged/hostNetwork 模式。

完整的原生接入与兼容方案，请参阅 embodied-runtime 文档 `apps/embodied-runtime/docs/examples.zh-CN.md` 中的“未适配设备”章节。

```bash
# 按标签列出 Job。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs?labelSelector=framework=ppo"

# 获取 Job，包括其 status 字段。
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID"

# 停止 Job。
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"stopped":true}}'

# 删除 Job。
curl -X DELETE "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID"
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

## 4. UI 凭据校验

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
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID/logs"

# 列出用户的 SSH 公钥。
curl "$RLARK_GATEWAY/api/v1/ssh-user-keys?user=alice"
```

存储上传、下载和删除操作请参阅 [存储 API](../storage-api.md)。
