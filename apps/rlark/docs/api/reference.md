# API Reference

This page lists the HTTP routes registered by the Gateway in [`pkg/gateway/router.go`](https://github.com/RLinf/RLark/tree/main/apps/rlark/pkg/gateway/router.go). For runnable requests, see [API Examples](examples.md). The machine-readable subset is available as the [OpenAPI specification](swagger.yaml).

Except for `POST /api/v1/auth/login` and `/metrics`, Gateway routes require the JWT returned by login in the `Authorization: Bearer <token>` header. Tokens expire after 8 hours by default; configure the lifetime with `--jwt-token-ttl`. A missing or invalid token returns `401`; an authenticated `user` calling an admin-only route returns `403`.

Admin-only operations are Node and Domain mutations, all certificate, image registry, and Addon routes, system configuration updates, plus StorageClass provider and mutation routes. System configuration reads and all SSH key operations accept either authenticated role. Authorization is role-based and does not yet isolate resources, including SSH keys, by individual user.

Path parameters are written as `{name}` below; Gin uses the equivalent `:name` syntax in the router. Namespaced CRD routes require the `namespace` query parameter.

## CRD resources

| Resource | Scope | Routes |
|----------|-------|--------|
| `nodes` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/nodes`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}` |
| `jobs` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/jobs`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/jobs/{name}`; `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs`; `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs/label-values`; `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/metrics` |
| `tasks` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/tasks`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}`; all methods on `/api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}` |
| `pods` | Namespaced | `GET /api/v1/rlinf.io/v1alpha1/pods`; `GET, PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}`; `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/events`; `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal` |
| `domains` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/domains`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}` |

When creating a Job, the Gateway stores the submitted `metadata.name` as its display name and returns a generated `jo-<16 hexadecimal characters>` resource ID in `metadata.name`. Use the returned ID for subsequent Job API requests.

The Gateway router does not expose CRD status subresource routes. Status is returned as part of the normal resource representation.

The Job `logs` route is implemented and accepts `from`, `to`, `task`, `pod`, `query`, `cursor`, and `order`. `from` and `to` are RFC 3339 timestamps; `order=asc` requests ascending backend results, while any other value uses descending order. When `from` is supplied and a log backend is configured, a successful response contains `source: "backend"`, `entries`, `hasMore`, and `nextCursor`. Otherwise—including backend errors—it falls back to Pod logs and returns `source: "pod"` plus a `pods` array containing `taskName`, `podName`, `phase`, `node`, and `logs`.

`logs/label-values` accepts `label` (default `pod`), `from`, `to`, `task`, and `pod`, and returns `{ "values": [...] }`; without a configured backend the array is empty. Job `metrics` is registered but currently only returns HTTP `501 Not Implemented`.

## Clusters and certificates

| Method | Path |
|--------|------|
| `GET` | `/api/v1/clusters` |
| `GET` | `/api/v1/clusters/{cluster_id}` |
| `GET` | `/api/v1/certificates/agent` |
| `GET` | `/api/v1/certificates/agent/{cluster_id}` |
| `POST` | `/api/v1/certificates/agent` |

`POST /api/v1/certificates/revoke` is registered by the Gateway but is not implemented and must not be treated as an available API.

## Authentication and SSH keys

| Method | Path |
|--------|------|
| `POST` | `/api/v1/auth/login` |
| `GET` | `/api/v1/ssh-user-keys` |
| `POST` | `/api/v1/ssh-user-keys` |
| `DELETE` | `/api/v1/ssh-user-keys/{index}?user={user}` |

## API reference metadata

| Method | Path |
|--------|------|
| `GET` | `/api/v1/api-reference` |

The authenticated API reference metadata endpoint returns localized section titles, endpoint methods and paths, descriptions, and response examples. The Web UI uses this endpoint as its API reference data source instead of maintaining a separate endpoint list.

## Gateway observability

| Method | Path |
|--------|------|
| `GET` | `/metrics` |

`GET /metrics` exposes Gateway metrics in Prometheus text format.

## Images, image registries, and system configuration

| Method | Path |
|--------|------|
| `GET` | `/api/v1/images` |
| `GET, POST` | `/api/v1/image-registries` |
| `GET, PUT, DELETE` | `/api/v1/image-registries/{id}` |
| `GET, PUT` | `/api/v1/system-config` |

System configuration is split into `ssh` and `log` categories. `ssh.jumpHost` must be a hostname or IP address without a scheme, user, path, or port; `ssh.jumpPort`, when set, must be an integer from 1 to 65535. Set `log.backend` to `none` to disable historical log queries, or to `sls` with the required SLS connection fields. Sensitive SLS credentials are returned as `****`; sending that placeholder back preserves the stored value. A successful `PUT` returns the effective masked configuration.

The optional `deployment` category follows the relevant subset of the `rlarkadm` `DeployConfig` structure and controls defaults in the deployment YAML shown after signing a data-plane Agent cluster. It accepts only `controlPlaneAddress`, `sshAddress`, `insecureSkipTlsVerify`, and the Kubernetes Agent fields `kubeconfig`, `agentImage`, `image`, `imagePullPolicy`, `imagePullSecrets`, and `containerdSocket`. Control-plane components, database, certificate, Docker, and Raw deployment fields are rejected. If `controlPlaneAddress` is empty, the signing API's Server address is used. Image pull policy may be `Always`, `IfNotPresent`, or `Never`.

Image registry credentials use an immutable `ir-<16 lowercase hexadecimal characters>` ID; the display `name` may be duplicated or changed. Create requests require `name`, `registry`, `username`, `password`, and `clusterSelection`:

```json
{"name":"Production Harbor","registry":"harbor.example.com","username":"robot","password":"secret","clusterSelection":{"mode":"Selected","clusters":["cluster-a"]}}
```

`clusterSelection.mode` is `None` (store only), `Selected` (one or more logical cluster names), or `All` (current and future clusters). `clusters` must be empty for `None` and `All`. Responses omit the password and include `id`. `POST` returns `201 Created`. On `PUT`, omitting `password` preserves it; an explicitly empty password is invalid. `DELETE` returns `202 Accepted` because Replication and Delivery remove distributed Secrets asynchronously.

## Storage

| Method | Path |
|--------|------|
| `GET, POST` | `/api/v1/storage/storageclass` |
| `PUT, DELETE` | `/api/v1/storage/storageclass/{name}` |
| `GET` | `/api/v1/storage/storageclass/provider` |
| `GET` | `/api/v1/storage/storageclass/{name}/{cluster}/list` |
| `POST` | `/api/v1/storage/storageclass/{name}/{cluster}/upload` |
| `GET, DELETE` | `/api/v1/storage/storageclass/{name}/{cluster}/object/{key}` |

## Addons

| Method | Path |
|--------|------|
| `GET` | `/api/v1/addons` |
| `GET` | `/api/v1/addons/{name}` |
| `GET` | `/api/v1/installed-addons` |
| `GET, POST` | `/api/v1/clusters/{cluster_id}/addons` |
| `GET, PUT, DELETE` | `/api/v1/clusters/{cluster_id}/addons/{name}` |
