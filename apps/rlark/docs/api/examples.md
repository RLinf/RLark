# API Examples

This page provides end-to-end RLark Gateway HTTP API examples, focused on the **Kubernetes runtime** (`agentType=Kubernetes`). For resource operations and schemas, see [API Reference](reference.md). The machine-readable contract is available as the [OpenAPI specification](swagger.yaml).

!!! warning "Authentication limitation"
    The login endpoint only validates the built-in Web UI credentials. It does not return a Bearer token or session cookie for subsequent requests, and the other Gateway APIs do not authorize requests based on that result. Run these commands only on a trusted network or through an ingress that enforces authentication and authorization.

## Conventions

- The standalone Gateway listens on `http://localhost:8080` by default. An `rlarkadm` deployment exposes it internally on port `8090` and routes browser traffic through the UI service.
- CRD API root: `/api/v1/rlinf.io/v1alpha1`
- Namespaced resources such as `nodes` and `tasks` require `namespace=<namespace>` in the query string.
- Cluster-scoped resources such as `jobs` and `workflows` do not use a namespace query parameter.
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

Users create Jobs with complete Task templates. The Job controller creates the corresponding namespaced Task resources and the Agent creates the downstream workload.

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

The image, command, environment, resources, and volumes belong under `kubernetes.workload.template.spec.containers`; they are not top-level Task fields.

```bash
# List Jobs by label.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs?labelSelector=framework=ppo"

# Get the Job, including its status field.
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole"

# Stop the Job.
curl -X PATCH \
  "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"spec":{"stopped":true}}'

# Delete the Job.
curl -X DELETE "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole"
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

## 4. Create a Workflow

A Workflow contains Job templates linked by dependencies. Each `jobTemplates[].spec` is a complete Job spec.

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

## 5. UI credential check

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
curl "$RLARK_GATEWAY/api/v1/rlinf.io/v1alpha1/jobs/ppo-cartpole/logs"

# List SSH public keys for a user.
curl "$RLARK_GATEWAY/api/v1/ssh-user-keys?user=alice"
```

For storage upload, download, and deletion operations, see [Storage API](../storage-api.md).
