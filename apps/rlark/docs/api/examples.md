# API Examples

This page provides end-to-end RLark Gateway HTTP API examples, focused on the **Kubernetes runtime** (`agentType=Kubernetes`). For resource operations and schemas, see [API Reference](reference.md). The machine-readable contract is available as the [OpenAPI specification](swagger.yaml).

Authenticate first and use the returned `token` as `Authorization: Bearer <token>` for subsequent Gateway API requests. Tokens expire after 8 hours by default.

## Conventions

- The standalone Gateway listens on `http://localhost:8080` by default. An `rlarkadm` deployment exposes it internally on port `8090` and routes browser traffic through the UI service.
- CRD API root: `/api/v1/rlinf.io/v1alpha1`
- Namespaced resources such as `nodes` and `tasks` require `namespace=<namespace>` in the query string.
- Cluster-scoped resources such as `jobs` do not use a namespace query parameter.
- `spec.agentType` accepts `Kubernetes`, `Docker`, or `Raw`. Only the Kubernetes runtime is currently implemented; Docker and Raw are planned.
- `spec.role` is required and accepts `Actor`, `Rollout`, or `Env`.
- `kubernetes.workload.template` is a Kubernetes `corev1.PodTemplateSpec`.

Set a base URL once for the examples:

```bash
export RLARK_GATEWAY=http://localhost:8080
```

## 1. Query and cordon Nodes

Nodes are registered and reported by Agents. Users normally list or inspect them and change only their schedulability.

```bash
# List Nodes in a namespace.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes?namespace=default"

# Get one Node.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default"

# Mark the Node unschedulable.
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"unschedulable":true}}'
```

## 2. Create and inspect a Job

Users create Jobs with complete Task templates. The Gateway stores the submitted `metadata.name` as the display name and returns a generated resource ID in `metadata.name`. API clients must retain that returned ID for later requests. The Job controller creates the corresponding namespaced Task resources and the Agent creates the downstream workload.

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

echo "$JOB_ID" # jo-<16 hexadecimal characters>
```

The image, command, environment, resources, and volumes belong under `kubernetes.workload.template.spec.containers`; they are not top-level Task fields.

### Access an unsupported device through HostNetwork

If embodied-runtime does not yet support a network device but the data-plane node can reach it directly, enable host networking explicitly in the Task PodTemplate:

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

This path does not provide embodied-runtime device discovery, resource isolation, controllers, CLIs, or SDK injection. The workload image is responsible for device drivers and lifecycle management. `hostNetwork` reduces network isolation and can cause port conflicts, so use it only on trusted data planes and dedicated device nodes. Do not enable `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES` for this purpose; that variable globally applies the legacy privileged/hostNetwork mode to multiple tasks.

See the [embodied-runtime deployment and usage examples](../../../embodied-runtime/docs/examples.md#unsupported-devices) for the native and compatibility paths.

```bash
# List Jobs by label.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs?labelSelector=framework=ppo"

# Get the Job, including its status field.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID"

# Stop the Job.
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"stopped":true}}'

# Delete the Job.
curl -X DELETE "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID"
```

A merge patch replaces arrays as a whole. When patching `tasks` or `jobTemplates`, send complete array elements, including required fields such as `role`, or use JSON Patch.

## 3. Inspect controller-managed Tasks

Tasks are created by the Job controller and should be treated as read-only by API clients.

```bash
# List Tasks for a Job.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/tasks?namespace=default&labelSelector=rlinf.io/job=ppo-cartpole"

# Get one Task, including its status field.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/tasks/ppo-cartpole-actor-head?namespace=default"
```

## 4. UI credential check

Only the built-in usernames `admin` and `user` are accepted. A successful response is `{"ok":true,"role":"admin"}` or `{"ok":true,"role":"user"}`.

```bash
curl -X POST "$RLARK_GATEWAY/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

This response is for the current Web UI login gate only. It does not grant credentials for later API calls.

## 6. Other Gateway endpoints

```bash
# List connected clusters.
curl "$RLARK_GATEWAY/api/v1/clusters"

# List StorageClasses across clusters or filter by cluster IDs.
curl "$RLARK_GATEWAY/api/v1/storage/storageclass"
curl "$RLARK_GATEWAY/api/v1/storage/storageclass?clusters=cluster-a,cluster-b"

# List storage providers.
curl "$RLARK_GATEWAY/api/v1/storage/storageclass/provider"

# Read Job logs.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/$JOB_ID/logs"

# List SSH public keys for a user.
curl "$RLARK_GATEWAY/api/v1/ssh-user-keys?user=alice"
```

For storage upload, download, and deletion operations, see [Storage API](../storage-api.md).
