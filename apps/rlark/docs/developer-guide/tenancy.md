# Users, tenants, and workload namespaces

RLark distinguishes authentication users from business tenants. Users identify
who performs an operation; tenants own business resources. Data-plane namespace
placement is resolved from tenant and cluster context according to deployment
policy rather than defining tenant identity.

RLark's core model does not require Tenant and Namespace to be the same concept.
It also does not prevent an upper-layer product or deployment from configuring a
one-to-one Tenant-to-Namespace mapping. Keep that mapping in explicit tenant
configuration or a namespace resolver so it can evolve to cluster-specific or
one-to-many placement without changing resource ownership semantics.

## Compatibility model

The current user provider reads the existing `rlark-ui-auth` Secret and builds
the `admin` and `user` API objects in memory. The Secret data keys and password
format are unchanged. The current tenant provider similarly exposes one
in-memory tenant:

| Tenant | Data-plane namespace |
| --- | --- |
| `default` | `rlark-system` |

This table describes the current default deployment mapping, not a permanent
core-model constraint.

Resources written by older RLark versions do not have tenant metadata. RLark
treats those resources as members of the `default` tenant when reading,
authorizing, querying, or creating child resources. This default is constructed
in memory and is not written back to old records.

## Metadata

New business resources use existing Kubernetes metadata fields, so CRD specs,
database schemas, and stored Secret payloads remain compatible:

```yaml
metadata:
  labels:
    rlark.io/tenant-id: default
  annotations:
    rlark.io/created-by: user
```

Tenant metadata is inherited through Workflow → Job → Task → Pod and onto the
data-plane workloads created for a Task. StorageClass and image registry
resources also expose their tenant ownership. Existing CSI Secret placement and
image credential distribution behavior are unchanged.

## Enforcement

Tenant metadata is only a boundary because the Gateway enforces it on every
request. Workflow, Job, Task, and Pod are tenant scoped; Node and Domain are
infrastructure objects that stay outside the scope. A caller with the `admin`
role is unrestricted, and a caller with no tenant assignment is refused rather
than treated as `default` — the `default` fallback described above applies to
stored objects that predate tenant metadata, not to callers.

Lists are narrowed to the caller's tenants, a named object owned by another
tenant is reported as `404` rather than `403`, and an out-of-scope `?tenant=`
value is refused with `403`. The full entry-point list and failure semantics are
in [Tenant scope enforcement](api-conventions.md#51-tenant-scope-enforcement).

## Read-only APIs

- `GET /api/v1/users` (administrator only)
- `GET /api/v1/users/me`
- `GET /api/v1/users/{id}`
- `GET /api/v1/tenants`
- `GET /api/v1/tenants/{id}`

`GET /api/v1/users/me` always resolves to the authenticated caller and is
available to every role. `GET /api/v1/users/{id}` is available to a caller that
names itself and otherwise requires the administrator role; a non-administrator
naming another user is refused with `403`.

User and tenant mutation APIs are intentionally not exposed until persistent
providers are introduced.
