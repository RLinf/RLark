# API Reference

This page lists the HTTP routes registered by the Gateway in [`pkg/gateway/router.go`](https://github.com/RLinf/RLark/tree/main/apps/rlark/pkg/gateway/router.go). For runnable requests, see [API Examples](examples.md). The machine-readable subset is available as the [OpenAPI specification](swagger.yaml).

!!! warning "Authentication limitation"
    `POST /api/v1/auth/login` validates the built-in UI credentials and returns a role, but it does not create a server-side session or issue a token. The Gateway currently does not enforce that login result on the API routes documented below. Do not expose the Gateway directly to untrusted networks; place it behind an authenticated reverse proxy or another trusted access-control layer.

Path parameters are written as `{name}` below; Gin uses the equivalent `:name` syntax in the router. Namespaced CRD routes require the `namespace` query parameter.

## CRD resources

| Resource | Scope | Routes |
|----------|-------|--------|
| `nodes` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/nodes`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}` |
| `workflows` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/workflows`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/workflows/{name}` |
| `jobs` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/jobs`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/jobs/{name}`; `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/logs`; `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/metrics` |
| `tasks` | Namespaced | `GET, POST /api/v1/rlinf.io/v1alpha1/tasks`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}`; all methods on `/api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}` |
| `pods` | Namespaced | `GET /api/v1/rlinf.io/v1alpha1/pods`; `GET, PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}`; `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/events`; `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal` |
| `domains` | Cluster | `GET, POST /api/v1/rlinf.io/v1alpha1/domains`; `GET, PUT, PATCH, DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}` |

The Gateway router does not expose CRD status subresource routes. Status is returned as part of the normal resource representation.

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
| `DELETE` | `/api/v1/ssh-user-keys/{id}` |

## Image registries and system configuration

| Method | Path |
|--------|------|
| `GET, POST` | `/api/v1/image-registries` |
| `GET, PUT, DELETE` | `/api/v1/image-registries/{name}` |
| `GET, PUT` | `/api/v1/system-config` |

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
