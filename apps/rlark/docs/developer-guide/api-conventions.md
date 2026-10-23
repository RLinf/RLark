# API Conventions

This page is the authoritative reference for the shared vocabulary and field
contracts used across RLark's Kubernetes-compatible management API, its REST
Gateway, and its data-plane agents.

It complements, and does not replace, the rules in
[Project Design Principles](design-principles.md). Read the principles for
*why* a rule exists; read this page for the *exact* names, owners, and
behaviours a change must respect.

!!! note "How to read this page"

    This page describes what the code on `main` guarantees today. A rule that
    is not implemented yet does not belong here: a design direction is not a
    convention, and planned behaviour must not be described as supported
    before it lands.

## 1. Terminology

Use these terms with exactly these meanings in code, API fields, commit
messages, and documentation. The registered type list in
`api/rlark.io/v1alpha1/groupversion_info.go` is authoritative; the API group is
`rlinf.io/v1alpha1` even though the directory is named `rlark.io`.

| Term | Meaning | Explicit boundary |
| --- | --- | --- |
| Domain | A logical grouping and addressing boundary in RLark's virtual network. | Not by itself a complete security-isolation boundary. |
| DomainPeer | A namespaced, data-plane-facing view of Domain routing and connection information. | Not a binary edge between exactly two Domains. |
| Node | A representation of compute capacity joined to one data-plane cluster. | Membership is determined by the owning Cluster's `spec.agentNamespace`: a Node belongs only to the Cluster whose management namespace contains its CR. `rlark.io/cluster-id` is a compatibility lookup label, not ownership proof. A Node is not a Kubernetes `Node`. |
| Pod | A control-plane view of a workload instance observed in a data plane. | Not the Kubernetes `Pod` object itself; it mirrors one. |
| Job | A workload composed of one or more Task templates. | Not a Kubernetes `Job`. |
| Task | A single workload unit described by a runtime-specific spec. | A Task is a unit of a Job, not a step in a Workflow. |
| Workflow | A DAG of Job templates. | Dependencies are between Jobs in a Workflow, never between Task templates inside one Job. |
| Addon | A delivery unit synchronized into a data plane. | — |
| Tenant | A business tenant identity. | Independent from Namespace placement in the core model. |

Two naming rules follow from the table:

- Never write "cluster" when the subject is a Domain, and never write "node" or
  "pod" for a Kubernetes object when the subject is an RLark `Node` or `Pod`.
- `Task` uses one common `PodTemplateSpec` workload model. Kubernetes applies it
  natively; future Runtime providers lower their supported subset and reject
  unsupported fields. `pvcStorageMap` and `pvcSizeGbMap` are not part of the
  canonical schema; use Kubernetes-native volume APIs.

## 2. Field ownership

Every field has exactly one writer. A writer must patch only the fields it
owns, so that two reconcilers updating the same object's status cannot clobber
each other.

### 2.1 Roles

| Surface | Role | Written by |
| --- | --- | --- |
| `spec` | Durable desired state. | The client (Gateway, CLI, UI) or an owning parent controller. |
| `status` | Observed state, reconstructible from reality. | Controllers and agents only. |
| `status.conditions` | Machine-readable detail about the current reconciliation. | The controller responsible for that condition type. |
| Operation resources | Non-repeatable one-shot actions with their own identity. | The client creates; a controller consumes. |
| Resource events | Short-lived, human-oriented, lossy narration. | Any component; never a state source. |
| Logs | Debugging detail. | Any component. |
| Audit records | Durable who-did-what. | Server-side entry points. |

Never model a one-shot action as a `spec` boolean such as `restart: true`. A
`spec` switch must be a durable desired state that is safe to re-reconcile
after a restart.

### 2.2 Control plane vs data plane

`Task` is the resource with a split status owner, and it is the pattern to
follow whenever a new split is introduced:

- The data-plane cluster agent owns `status.phase`, `status.message`, and
  `status.observedNodes`. It collects them
  from the live workload as a `StatusIntent` — the Kubernetes runtime produces
  one in `apps/rlark/pkg/agent/controllers/task/push_deployment.go` and
  `lifecycle_kubernetes.go` — and the runtime-neutral task host in
  `apps/rlark/pkg/agent/execution/host/host.go` merges the intents by priority
  and performs the single status write. A runtime provider must never patch
  Task status itself. It is also the owner of `status.observedGeneration`, which
  it advances on every accepted write — including a write whose observed values
  happen to be unchanged, because a spec-only change must not leave the reported
  status silently stale (see [4.3](#43-observedgeneration)).
- The control-plane Task controller owns `status.pullProgress` and
  `status.events`, and aggregates them in
  `apps/rlark/pkg/controllermanager/task/task_controller.go`.

Both writers use `client.MergeFrom(original)` status patches so that only their
own fields are sent. A full `Status().Update` from either side would reset the
other side's fields on every reconcile. The task host additionally fences a
write against the Task UID and generation it observed, so a status intent
computed for a stale Task incarnation is dropped rather than applied.

For the remaining resources the owner is single:

- Control plane owns `Job`, `Workflow`, and `Domain` status.
- A data-plane Agent owns only the Kubernetes-derived capacity and health fields
  of `Node` status. The control-plane Node reconciler exclusively owns Node
  connection `phase`, heartbeat time and the `Ready` condition from the Agent
  Lease; both writers patch only their declared fields.
- The control-plane Cluster reconciler owns Cluster aggregate status and its
  `Ready` condition. It derives membership by listing Node CRs in
  `Cluster.spec.agentNamespace`, never by selecting the mutable
  `rlark.io/cluster-id` label.
- The Gateway applies the same membership rule to `/nodes`, including when a
  `namespace` query is supplied. An unclaimed namespace returns an empty list
  with `items: []`. If two Clusters claim the same agent namespace, the Gateway
  lists that namespace once and logs a warning so that historical duplicate
  claims do not make the Node list unavailable. Cluster writes currently use the
  management API directly; the Gateway does not register Cluster create, update,
  or patch routes. The authoritative owner must detect and report conflicting
  claims. Because this list combines multiple
  namespaces, its opaque `metadata.continue` token is issued and consumed only
  by the Gateway rather than forwarded to the Kubernetes API. Aggregated Node
  lists do not support `labelSelector` or `fieldSelector`; both parameters are
  rejected with `400 INVALID_REQUEST`.
- Data-plane agents own `Pod` and `Addon` status, which is pure observation
  reported upward.

### 2.3 Determinism

A status writer must produce byte-identical output for identical input, because
every reconciler compares the computed status against the stored one and skips
the write when they are equal. Aggregating over a Go map without sorting breaks
this: the comparison never succeeds and the controller writes status on every
reconcile. `aggregatePullProgress` therefore sorts node names, then sorts the
result stably by image, then truncates to a fixed bound.

## 3. Identity

The name of the Kubernetes object is the stable business identity. There is no
second business ID field and none should be introduced: a parallel identifier
would give the same object two authoritative handles, and it would need its own
uniqueness and conflict rules on top of the ones the API server already
enforces for names.

Three kinds of identity exist and must not be substituted for one another:

- A **business identity** is the Namespace and name of the object for a
  namespaced resource, and the name alone for a cluster-scoped one. It is fixed
  when the object is created — supplied by the caller, or derived by a
  controller for a child — and is immutable for the lifetime of that object. It
  is the handle every layer keys on: Gateway path parameters, the `namespace`
  and `name` columns the store indexes, and the names of derived children. A
  child derives its identity from its parent rather than from whichever child
  object happens to exist: `utils.ChildName` builds a Task name as `<job
  name>-<task template name>`, so a Task's identity is reproducible from its
  Job's identity plus the template name.
- A **Kubernetes UID** (`metadata.uid`) identifies exactly one Kubernetes
  object instance. Use it to prove that a cached or child object is still the
  instance you observed. `rlark.io/task-uid` and `rlark.io/local-pod-uid` on
  mirrored Pods exist for this purpose. A missing UID stays empty; a store must
  not synthesize one, because a reader could not then tell a real instance
  identity from a fabricated one.
- A **run identity** identifies one execution of a business object. It is
  neither the business identity nor the UID. No registered type under
  `api/rlark.io/` carries one today; if one is introduced, it belongs in an
  annotation rather than in `spec`, because it is observed state about a single
  execution and not part of the desired state the user manages.

Two rules follow from the split between the first two:

- Never use a name to prove object-instance identity. A name can be deleted and
  recreated; the name is then unchanged while the UID has changed, so a
  controller matching on name alone would adopt a different instance.
- Never use a UID as the long-lived business handle. It is replaced on every
  recreation, so a record keyed by UID stops describing the business object the
  user still manages.

Because the name carries business meaning it cannot be edited; a rename is a
different business object. A mutable, user-facing label belongs in a separate
display field that no layer keys on, and such a field is never an identity.

Labels are scope metadata and selectors, not identity. Anyone who can write
metadata can relabel an object, so a label may narrow a search but may never
stand in for a name or a UID when proving ownership.

!!! note "Archive and restore — persistence-backed lifecycle"

    A finished Job is archived into the persistence backend and deleted from
    kcp after `ArchiveTTL` (5 minutes by default). The business record (spec +
    final status) is retained in PostgreSQL or a Kubernetes Secret (test mode),
    while the active CR is removed to free etcd and controller memory. Starting
    the Job later restores it from the persistence backend with the same
    Namespace and name but a new UID.

    The unified start endpoint is `POST /jobs/:name/start`: if the Job is
    active (KCP has the CR), it patches `spec.suspended=false` directly; if
    archived, it restores from the backend. The frontend calls this endpoint
    without knowing whether the Job is in kcp or archived.

    The `PersistenceBackend` interface abstracts PostgreSQL and Kubernetes
    Secret backends so Syncer and Gateway use the same contract. A new backend
    type implements `Save`/`Load`/`List`/`Delete` and is registered in
    `controller-manager.go` and `gateway.go`.

Before adopting, mutating, or deleting a child or external object, prove
ownership with the strongest evidence available: UID, owner reference, parent
name, template identity, and scope metadata. If ownership cannot be proven, do
nothing.

## 4. Phase and Condition

### 4.1 Phase

`Phase` is a single, coarse, human-oriented lifecycle summary. Every phase type
is a closed enum validated by the API server:

| Type | Values |
| --- | --- |
| `JobPhase` | `Pending`, `Running`, `Succeeded`, `Failed`, `Stopped` |
| `TaskPhase` | `Pending`, `Running`, `Succeeded`, `Failed`, `Stopped` |
| `WorkflowPhase` | `Pending`, `Running`, `Stopping`, `Stopped`, `Succeeded`, `Failed` |
| `AddonPhase` | `Pending`, `Installing`, `Ready`, `Failed`, `Upgrading` |
| `PodPhase` | `Pending`, `Running`, `Succeeded`, `Failed`, `Unknown` |
| `NodePhase` | `Online`, `Offline` |

Adding a value to one of these enums is an API change: it must ship with the
regenerated CRD manifests and with consumers that tolerate the new value.

### 4.2 Condition

Conditions carry the machine-readable detail that `Phase` deliberately omits.
All condition types are declared centrally in
`api/rlark.io/v1alpha1/conditions.go`; a new type
must be added there and documented here rather than defined ad hoc in an
implementation package.

| Type | Meaning | Writer |
| --- | --- | --- |
| `Validated` | The controller's verdict on the resource spec. `False` means the spec is invalid and will not be reconciled further. | Control-plane Job, Workflow, and Domain controllers |
| `Progressing` | The resource is still being driven toward the desired state. `True` means work is in flight. | Data-plane Task controller |
| `Ready` | Whether the resource is currently usable. It is the structured companion of `Phase`: `Phase` gives the aggregate stage, `Ready` gives the boolean verdict plus a machine-readable `Reason`. | Data-plane Addon controller |

`Progressing` is currently emitted only on the claimed-Workload migration path,
so a Task that converges without a Workload kind transition reports no
`Progressing` condition; consumers must not read its absence as "not
progressing".

`Node` declares `status.conditions` but has no writer: node readiness is
reported through `status.phase` alone, so consumers must not expect a `Ready`
condition on a Node.

Rules for every `Conditions` field:

- The field is `[]metav1.Condition` with `+listType=map` and
  `+listMapKey=type`, so a given type appears at most once.
- Write through `apimeta.SetStatusCondition` so that `LastTransitionTime` keeps
  its meaning.
- `Reason` is resource- and scenario-specific, CamelCase, stable, and
  machine-consumable. Reasons are intentionally not centralized.
- A `Validated` writer must record both verdicts. Writing only `False` leaves a
  stale rejection behind after the user fixes the spec, so the same controller
  also writes `True` once the spec passes validation.
- An invalid spec cannot be repaired by retrying. Controllers therefore record
  `Validated=False` and stop instead of returning an error, which would requeue
  forever; reconciliation resumes when a new generation appears.

### 4.3 ObservedGeneration

`ObservedGeneration` is a **controller contract, not an API server guarantee**.
It only states "the writing controller had finished reconciling this
generation at the time of the write." Consumers must treat a status whose
`observedGeneration` is behind `metadata.generation` as stale and must not
conclude anything about the current spec from it.

It therefore exists only where a Spec-to-Status reconciliation loop actually
exists: `JobStatus`, `WorkflowStatus`, `TaskStatus`, `DomainStatus`, and
`AddonStatus`. It is deliberately absent from `Node`, `Pod`, and `DomainPeer`,
whose status is pure data-plane observation with no spec to converge on; adding
it there would advertise a contract no controller honours.

For the same reason it is absent from `Ready`, which is observation-driven
rather than spec-driven. A `Ready` condition describes what the controller
last saw, not which spec generation produced it.

Writers advance it only after a reconcile pass has succeeded, never at the
start of one. `metav1.Condition` additionally carries its own per-condition
`ObservedGeneration`, which a spec-driven condition must fill in.

## 5. Tenant and owner metadata

Tenant and creator identity live in `metadata`, not in `spec`:

- Tenant: label `rlark.io/tenant-id` (`tenant.IDLabel`). An object without the
  label is logically tenant `default`, which preserves objects created before
  tenant metadata existed.
- Owner: annotation `rlark.io/created-by` (`tenant.CreatedByAnnotation`).

Both values are assigned server-side from the authenticated identity and
overwrite anything the client sent; a forged label or annotation in a create
request is discarded. Derived resources inherit tenant and owner identity from
their parent. Client-side filtering is never a tenancy boundary — every entry
point enforces it server-side.

See [Users and Tenants](tenancy.md) for the tenant-to-namespace resolution
model.

### 5.1 Tenant scope enforcement

The tenant scope of a request is resolved from the authenticated subject on
every request rather than read from the token, so that a membership change takes
effect immediately instead of when the token expires. A caller with the `admin`
role is unrestricted; a caller with no tenant assignment is refused with 403
`FORBIDDEN` instead of falling back to the `default` tenant.

Workflow, Job, Task and Pod are tenant scoped. Pod is included although the
Gateway never stamps it: the agent derives the Pod from a Task and the Pod
inherits that identity, so reading or patching one must respect the same
boundary. Node, Domain and Cluster are infrastructure objects that never carry
tenant identity, so they stay outside the scope: Node and Domain writes are
already restricted to administrators, and the Gateway does not expose Cluster
writes at all.

Enforcement applies to every entry point, including create, list, get, update,
patch, delete, the Job start endpoint, Job tag and log endpoints, Pod events,
the Pod terminal and the TensorBoard proxy. Update, patch, delete and start
decide ownership by reading the stored object, because such a request carries no
tenant identity of its own. The start endpoint reads it from the management API
object it is about to patch, or, when the Job is already archived, from the
snapshot it is about to restore; a restored Job keeps the tenant label of the
snapshot, so restoring never moves an object between tenants. Create takes the
first tenant of the scope unless the body names one the scope already allows.
The Job tag vocabulary endpoint is scoped as well, so a tag key or value that
only exists in another tenant never appears in the vocabulary.

The failure modes are deliberately different:

| Situation | Response |
| --- | --- |
| `?tenant=` names a tenant outside the scope | 403 `FORBIDDEN` |
| A create body labels an object with a tenant outside the scope | 403 `FORBIDDEN` |
| A named object belongs to another tenant | 404 `RESOURCE_NOT_FOUND` |
| The caller has no tenant assignment | 403 `FORBIDDEN` |

An out-of-scope filter is refused rather than narrowed so that a client can tell
a refused filter apart from a filter that matched nothing. A named object is
reported as missing rather than forbidden, because 403 would confirm that the
name exists in a tenant the caller cannot see.

`tenant` carries an authorization assertion, which is why it is absent from the
parameter table in [6.3](#63-filtering-and-sorting): an out-of-scope value is
refused instead of narrowing the result set. An in-scope value is accepted, and
on a database-backed list it additionally narrows the result to that tenant the
same way any other first-class filter does. An `admin` caller is unrestricted by
scope, so no value is ever refused, but an admin request that names a tenant on
a database-backed list is still narrowed to it. Management-API-backed lists do
not consume `tenant` as a filter today, so there an in-scope value leaves the
result set unchanged.

For persistence-backed lists the scope becomes a label selector, so `total` and
`hasMore` describe the rows the caller may see. The PostgreSQL backend evaluates
the selector in SQL; the Secret backend, used for test and CI deployments, has no
query language of its own and evaluates the same selector in memory before it
counts and pages the result, so both backends report the same visible set. The
missing-label default is applied by the backend as well, since a label selector
cannot express "value in (...) or label absent". For management-API-backed lists
the scope is applied after the page is fetched, so the reported paging window no
longer describes the returned set — the same caveat as the `tagSelector` filter
and any other filter applied after the server window.

The Gateway is the enforcement point. Credentials that reach the management API
directly are not covered; an admission webhook that protects tenant and owner
metadata against direct Kubernetes writes is not implemented yet.

## 6. REST contract

The Gateway REST API is a stable business view with one error shape, one
pagination shape, and strict query validation.

### 6.1 Errors

Every non-2xx response body is:

```json
{
  "code": "RESOURCE_NOT_FOUND",
  "message": "job \"train-1\" not found",
  "reason": "NotFound",
  "requestId": "7f1c...",
  "details": { "parameter": "limit", "reason": "must be a positive integer" }
}
```

`code` is a stable machine-readable identifier; `message` is human-oriented and
must not leak implementation detail; `reason` carries the upstream Kubernetes
reason when there is one; `requestId` echoes the `X-Request-Id` request header,
or the server-generated value when the client did not send one; `details` is
optional structured context.

The code set is closed and defined in
`apps/rlark/pkg/gateway/apierror.go`. A `swagger_test.go` check keeps
the OpenAPI enum and the Go constants aligned in both directions, so adding a
code requires updating both.

| Code | Typical status |
| --- | --- |
| `INVALID_QUERY_PARAMETER` | 400 |
| `INVALID_REQUEST_BODY` | 400 |
| `INVALID_REQUEST` | 400 |
| `UNAUTHORIZED` | 401 |
| `FORBIDDEN` | 403 |
| `RESOURCE_NOT_FOUND` | 404 |
| `RESOURCE_ALREADY_EXISTS` | 409 |
| `RESOURCE_CONFLICT` | 409 |
| `RESOURCE_INVALID` | 400 |
| `UPSTREAM_UNAVAILABLE` | 503 |
| `INTERNAL_ERROR` | 500 |
| `NOT_CONFIGURED` | 500 |

Kubernetes errors are mapped rather than wrapped: `NotFound` to 404,
`AlreadyExists` and `Conflict` to 409, `Invalid` and `BadRequest` to 400,
`Unauthorized` to 401, `Forbidden` to 403, `ServiceUnavailable`, `Timeout` and
`TooManyRequests` to 503, `UnsupportedMediaType` to 415, and anything else to
500.

### 6.2 Pagination

Every list endpoint is bounded. `limit` defaults to and is capped at 500
(`maxListLimit`), so a single request can never trigger an unbounded scan, and
the response always makes truncation detectable.

There are two list paths with deliberately different envelopes:

- **Database-backed lists** return `items`, `total`, `limit`, `offset`, and
  `hasMore` (`db.ListResult`). Ordering is stabilized by name so that paging is
  repeatable.
- **Kubernetes-backed lists** return `limit`, `hasMore`, and `total` alongside
  the normalized list. They do **not** return `offset`: the Kubernetes
  `continue` token stays inside `metadata` because it is valid only for a
  direct Kubernetes list and must not become the archive pagination contract.
  A request that carries `offset` on this path is rejected with 400 rather than
  ignored, because returning the first page while the caller believes it skipped
  ahead is worse than refusing the parameter.
  `total` is reported only when the server supplied enough information to
  compute it. A value of `-1` means the total is unknown; this is always used
  for aggregated Node lists because they combine independently paginated
  namespaces. Other Kubernetes-backed lists may report a **page-relative**
  total: on a continued page it is derived from the remaining-item estimate
  reported for that page, so it shrinks as the caller advances. Read it as
  "items still reachable from this page", not as a stable collection size. Only
  the database path reports a collection-wide `total`.

Normalization always works on a deep copy. Never mutate an object obtained from
an informer cache while shaping a response.

### 6.3 Filtering and sorting

Invalid query parameters are rejected with 400 and
`INVALID_QUERY_PARAMETER`, with `details.parameter` and `details.reason`
identifying the offending input. Silently ignoring an unparseable filter is
forbidden: it makes a client believe a filter was applied when it was not.

Filtering by business identity needs no parameter of its own: the identity is
the name, so `fieldSelector` on `metadata.name` already expresses it.

| Parameter | Accepted form |
| --- | --- |
| `limit` | Integer, greater than 0, at most 500. |
| `offset` | Integer, at least 0. Database path only; the Kubernetes path rejects it with 400. |
| `order` | `field [asc\|desc]`; field must match `^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`, and an unknown direction is rejected instead of silently sorting ascending. |
| `fieldSelector` | Comma-separated `key=value` or `key!=value` pairs; each key must match the same dotted-identifier pattern as `order`. |
| `labelSelector` | Comma-separated `key=value` or `key!=value` pairs on the database path, a Kubernetes label selector on the Kubernetes path. Keys must be valid qualified names. |
| `tagSelector` | Comma-separated `key=value` pairs. Applied after the server window. |
| `continue` | Opaque Kubernetes continue token. Kubernetes path only. |

Selector paths and label keys come from the query string and are therefore
bound as statement arguments on the database path, never formatted into SQL.
The pattern check above runs at the edge as defence in depth, so a nonsensical
path is refused with 400 before it can reach the database.

`!=` matches an object whose field or label is **absent**, on both paths. The
database path implements this with `IS DISTINCT FROM`; a plain `!=` comparison
would evaluate to `NULL` for a missing key and silently drop exactly the
objects the caller asked for.

## 7. Schema validation boundary

The CRD schema and the write path validate different kinds of rules, and the
split is deliberate.

The CRD schema closes **structural** semantics only — the shape a stored object
must have for every reader to interpret it the same way:

- closed enums (`+kubebuilder:validation:Enum`), such as the `Phase` types in
  [4.1](#41-phase);
- list semantics (`+listType=map`, `+listMapKey=type`), such as the
  `Conditions` rules in [4.2](#42-condition);
- required fields, types, and formats.

The CRD schema must **not** carry quantity or size quotas. Do not add
`MaxItems`, `MinItems`, `MaxLength`, `MinLength`, or `MaxProperties` markers to
types under `api/rlark.io/`: a cap such as "at most 10 tags per Job" or "a tag
key is at most 10 characters" is a product policy, not a storage invariant.
Such a limit is enforced on the write path by the Gateway for client-supplied
`spec`, and by the writing controller or agent for `status` it aggregates.

Three reasons drive the split:

- A schema bound applies to every writer, including controllers and agents
  reconstructing `status`. A controller that legitimately observes more items
  than the cap cannot write its status at all, and the reconcile loop fails
  with no way for an operator to relax the limit.
- A quota is the kind of rule that changes with product requirements. Changing
  it in the schema is a CRD change that must be rolled out to every cluster;
  changing it on the write path is a configuration or code change in one
  component. Tightening a schema bound also retroactively invalidates objects
  already stored, which the API server rejects only on the next write, so the
  breakage surfaces far from the change.
- An API-server rejection carries a Kubernetes validation message, not the
  Gateway's error envelope from [6.1](#61-errors). Enforcing on the write path
  lets the caller receive a stable `code`, a human-oriented `message`, and
  `details` naming the offending field.

The quotas in force are therefore part of the component that enforces them, not
of the CRD. A type comment may state where a field is bounded, but it must not
imply the API server rejects an over-sized object. Upstream types embedded in
our API, such as `metav1.Condition` and `corev1.PodTemplateSpec`, keep whatever
bounds upstream declares; this rule governs the markers we write ourselves.

The quotas the Gateway enforces today apply to the `JobTag` entries a client
submits, and are checked on create, update, and patch for `jobs.spec`,
`tasks.spec`, and every `workflows.spec.jobTemplates[].spec`:

| Rule | Limit |
| --- | --- |
| Tag entries per spec | 10 |
| Characters in a tag key | 10 |
| Values in a tag entry | 10 |
| Characters in a tag value | 10 |

Lengths are counted in Unicode code points, not bytes, so a limit means the same
thing for a key written in Chinese as for one written in ASCII. A violation is
refused with 400 `INVALID_REQUEST_BODY` and `details.field`, `details.limit`,
and `details.actual` naming the offending entry. A merge patch replaces an array
wholesale, so a patch either omits the tag array entirely and is not checked, or
carries it in full and is checked like a create. A body that is malformed in a
way the schema already rejects is passed through untouched rather than
reinterpreted as a quota violation.

## 8. Compatibility gates

Generated artifacts are part of the contract and are checked in CI by
`make verify-generated`, which runs `make generate` and fails when the
committed CRD manifests, deepcopy functions, typed clients, or the CRD
reference documentation differ from the regenerated output. The gate also fails
on untracked generated files.

So a change to any type under `api/rlark.io/` must be committed together with:

- `api/config/crd/bases/rlinf.io_*.yaml`
- `api/kubeclients/`
- `api/rlark.io/**/zz_generated.deepcopy.go`
- `apps/rlark/docs/reference/crd.md`

The OpenAPI specification is gated in both directions by `swagger_test.go`:

- spec -> route: every documented operation must exist in the router, no `$ref`
  may dangle, and the error code enum must match the Go constants.
- route -> spec: every route installed by `Gateway.RegisterRoutes` must be
  described by `docs/api/swagger.yaml`, otherwise it must be listed in the
  `routesUndocumentedInSwagger` baseline. The baseline may only shrink: the test
  also fails when a listed route becomes documented or disappears from the
  router, so documenting an endpoint requires deleting its entry and a new
  endpoint cannot ship undocumented by accident.

The gRPC contracts under `proto/embodied-runtime/` and `proto/execution/` are
gated by `buf`. Both trees include `proto/toolchain.mk`, which is the single
source of truth for the pins, and the repository-root targets fan out to every
tree:

- `make lint-proto` runs `buf lint` with the `STANDARD` rule set. Each tree's
  `buf.yaml` excepts a few naming rules — `PACKAGE_DIRECTORY_MATCH` and
  `SERVICE_SUFFIX` in both trees, plus `RPC_RESPONSE_STANDARD_NAME` in
  `proto/embodied-runtime/` — because the package and service names are
  embedded in the fully qualified method names already consumed by the released
  Go and Python SDKs; new files are still expected to follow `STANDARD`.
- `make proto-breaking` runs `buf breaking` against `branch=main` by default
  (CI overrides the baseline with `PROTO_BREAKING_REF=ref=<sha>`). It uses the
  `FILE` category, which also rejects source-breaking changes, because the
  generated stubs under `sdks/` are checked in and published. Add fields and
  RPCs instead of renaming, renumbering, or removing existing ones.
- `make verify-proto-generated` regenerates the stubs and fails when the result
  differs from what is committed, so a `.proto` change cannot ship without its
  Go and Python stubs.

The stubs record their generator versions in their own headers, and the Python
stubs reject an older runtime at import time, so the toolchain is pinned rather
than resolved from `latest`. Every tree must use the same pins, because a tree
generated with a different toolchain rewrites the whole stub tree on the next
regeneration:

| Tool | Version | Pinned in |
| --- | --- | --- |
| `protoc` | `36.2` (headers spell it `v7.36.2`) | `proto/toolchain.mk` |
| `protoc-gen-go` | `v1.36.12` | `proto/toolchain.mk` |
| `protoc-gen-go-grpc` | `v1.6.2` | `proto/toolchain.mk` |
| `buf` | `v1.72.0` | `proto/toolchain.mk` |
| `grpcio-tools` | `1.84.0` | `sdks/embodied-runtime-python/requirements-dev.txt` |

`make proto` refuses to run unless the pinned versions are the ones it finds:
`protoc-check` compares `protoc --version`, and `scripts/gen_proto.py` compares
the installed `grpcio-tools` distribution. Install them with
`make protoc-install` (downloads the pinned release into the gitignored
`proto/bin`, which also takes precedence over `PATH`) and
`pip install -r sdks/embodied-runtime-python/requirements-dev.txt`; the Go
plugins and `buf` are installed into the same directory on demand, under
version-stamped file names, so bumping a pin forces a reinstall. Bumping a
version means regenerating and committing every file under `sdks/*/gen/` in the
same change, and raising the runtime lower bounds in `requirements.txt` and
`setup.py` to whatever the new stubs demand.

Database migrations are gated by `pkg/db/migrations_test.go`, which fails when a
migration file does not register itself, when migration names are not unique and
increasing, or when a migration has no down function.
