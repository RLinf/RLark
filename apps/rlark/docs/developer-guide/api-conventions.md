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

When a one-shot action does live in `spec` as an identified request list, such
as `Node.spec.operations`, the contract is validated at the API boundary rather
than interpreted by each consumer:

- The identity field is required and non-empty. An operation without an ID
  cannot be tracked, reported, or retried idempotently, so every consumer would
  have to skip it, and the request would disappear with no result.
- The action enum is closed, and every value it admits has exactly one
  consuming owner. An unsupported value is rejected by the API server instead of
  being silently ignored by the controllers that do not recognize it.

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
  it advances only on an accepted intent reporting a workload/lifecycle conclusion
  (Phase or observed payload), even when those values are unchanged. Condition-only
  and message-only writes preserve the top-level generation; each Spec-driven
  Condition records its own generation. Admission rejection must leave an older
  Running Phase recognizably stale (see [4.3](#43-observedgeneration)).
- The control-plane Task controller owns `status.pullProgress` and
  `status.events`, and aggregates them in
  `apps/rlark/pkg/controllermanager/task/task_controller.go`. It is woken by a
  Node watch that enqueues only the Pending Tasks whose observed Nodes actually
  changed their reported progress or events, plus a leader-only jittered resync
  that re-enqueues Pending Tasks so a lost watch event cannot keep an aggregated
  value stale indefinitely. Each resync round visits every Task, and it waits
  for the controller to keep up rather than skipping Tasks, so the staleness
  bound is one resync period plus however long the controller needs to drain
  the round. The controller never requeues itself on a fixed interval, so the
  aggregated fields must always be recomputed from the current Node state
  rather than from what a previous reconcile observed.

Both writers use merge status patches so that only their own fields are sent.
The task host uses `MergeFromWithOptimisticLock`: concurrent API changes must
conflict before publication, and retry must re-read and recompute the intent.
Only successful publication may refresh the local resourceVersion for ordered
claim/lifecycle writes. A full `Status().Update` from either side would reset the
other side's fields on every reconcile. The task host also checks the intent's
UID/generation against its snapshot; this local check does not replace the
API-side resourceVersion precondition.

For the remaining resources the owner is single:

- Control plane owns `Job`, `Workflow`, and `Domain` status.
- A data-plane Agent owns only the Kubernetes-derived capacity and health fields
  of `Node` status, including the raw `status.diskPressure` observation. The
  control-plane Node reconciler exclusively owns Node `phase`,
  `status.lastHeartbeatTime`, `status.lastStatusChangeTime`,
  `status.drainOperations` and the whole `status.conditions` list, which it
  derives from the Agent Lease and from the Agent's own observations; both
  writers patch only their declared fields. `lastHeartbeatTime` is the Lease
  renewal republished through the control-plane clock, so it stays comparable
  across Nodes and only ever moves forward; an Agent must not write it, because
  a timestamp each Agent stamps with its own clock would make clock skew look
  like liveness. When a controller needs to know whether an Agent has reported
  at all, it reads an Agent-owned field such as `status.kubernetesNodeUID`.
- The control-plane Cluster reconciler owns Cluster aggregate status and its
  `Ready` and `Degraded` conditions, together with `status.observedGeneration`
  and `status.lastStatusChangeTime`. `lastStatusChangeTime` carries the same
  semantics as the identically named field on `NodeStatus`: it advances only
  when `phase` actually changes, so a consumer can distinguish "has stayed in
  this phase for a long time" from "just flipped". It derives membership by
  listing Node CRs in `Cluster.spec.agentNamespace`, never by selecting the
  mutable `rlark.io/cluster-id` label.
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

The same requirement applies to the desired state a parent renders for a child.
Comparing the whole child spec structurally makes the comparison depend on
fields the parent does not own: a defaulted or data-plane-written field flips
the result and the parent rewrites a child that is already correct. A parent
therefore projects the fields it owns into a canonical form, hashes it with
`reconcile.NewDigest`, and records the digest on the child:

| Annotation | Constant | Meaning |
| --- | --- | --- |
| `rlark.io/desired-digest` | `job.DesiredDigestAnnotation` | The digest of the desired state the parent last rendered for this child. A child whose digest matches is up to date and must not be rewritten. |

A child that carries no digest predates the annotation. The parent falls back to
a structural comparison for that child only, and the next write stamps the
digest, so the fallback converges instead of persisting. The digest is written
as metadata, which does not bump `generation`, so stamping it cannot be mistaken
for a spec change by anything watching the child.

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

### 3.1 Proving ownership of a child object

Before adopting, mutating, or deleting a child or external object, prove
ownership with the strongest evidence available: UID, owner reference, parent
name, template identity, and scope metadata. If ownership cannot be proven, do
nothing.

A parent stamps every child it creates with the same canonical metadata, so a
later reconcile can prove ownership without guessing from names:

| Metadata | Constant | Meaning |
| --- | --- | --- |
| Controller `OwnerReference` | — | The child instance is controlled by this parent instance. |
| `rlark.io/parent-uid` annotation | `utils.ParentUIDAnnotation` | The UID of the parent instance that created the child, so a recreated parent of the same name cannot inherit it. |
| `rlark.io/child-template` annotation | `utils.ChildTemplateAnnotation` | Which template in the parent's spec the child was rendered from. |
| Parent label | `rlinf.io/workflow`, `rlinf.io/job`, `rlinf.io/domain` | Scope metadata used to narrow a list. Never ownership proof on its own. |

`utils.ClassifyChild` and `utils.ClassifyRemovedChild` are the only place this
evidence is interpreted. They return one of four results:

| Result | Evidence | What the caller must do |
| --- | --- | --- |
| `Owned` | Controller reference, parent UID, and template all agree. | Act on the child. |
| `Adoptable` | Only evidence that predates the canonical form, such as a controller reference without the annotations, or a label plus a name the parent itself recorded. | Act on the child and write the canonical metadata back once. |
| `Foreign` | The evidence names a different parent, or names no parent at all. | Leave the child alone. This is a correct outcome, not an error. |
| `Unproven` | The evidence is ambiguous or contradictory, for example a label-only child whose name the parent never recorded. | Return an error and reconcile nothing. Never fall back to a name prefix guess. |

`Unproven` deliberately blocks the reconcile instead of skipping the object:
silently ignoring an ambiguous child hides a leak, and guessing deletes data
that may belong to someone else. The two functions therefore differ on purpose —
`Foreign` is an error for `ClassifyChild` (a child the parent expects to manage
must be attributable) but not for `ClassifyRemovedChild` (skipping another
parent's object during pruning is the expected result).

When a controller narrows a list with a field index instead of listing every
object, the index must cover **every** evidence form the classifier accepts as
proof. Indexing by label alone hides a child that carries only a controller
reference from its own parent, which leaks it silently. `utils.ChildIndexKeys`
emits all accepted keys and must be used by the index functions.

Adoption is a one-shot convergence, not a repeated write: once the canonical
metadata is present, a resync must detect that nothing changed and issue no
update.

### 3.2 Finalizers

A finalizer is a claim about one controller's own cleanup obligation, never a
statement about the object as a whole. A controller therefore inspects and
releases only its own finalizer, and releases it as soon as its obligation is
discharged, no matter which other finalizers are still present. Waiting for the
finalizer list to be empty deadlocks two controllers that both wait for the
other.

| Finalizer | Constant | Obligation |
| --- | --- | --- |
| `domains.rlinf.io/peer-cleanup` | `domain.CleanupFinalizer` | The Domain's DomainPeers have been deleted under proven ownership. |

Cleanup must be decided before it is executed: the rule that answers "is this
Domain still responsible for peers" is a pure function over the current list, so
it is testable without an API server, and the executing pass only applies the
decision. A pass re-reads the children and re-decides, so a pass interrupted
between deleting some children and releasing the finalizer resumes correctly.

A finalizer is edited with `Update`, never with a merge patch: a merge patch
replaces the whole list and carries no optimistic concurrency, so it would
silently drop a finalizer another controller added concurrently.

Ownership still has to be proven during deletion. A child whose ownership is
`Unproven` blocks the release rather than being deleted, because a deleting
parent is exactly when a wrong guess is unrecoverable.

A pass that is still waiting for children to disappear schedules no timer, so it
needs a child event to resume on. The Domain controller therefore watches
DomainPeers through an explicit mapping rather than `Owns()`: a peer written
before ownership metadata existed carries no `OwnerReference`, so `Owns()` would
map its deletion back to no Domain at all and the finalizer would hold the
Domain until the next full resync. The mapping, the field index and the orphan
sweeper all read one helper, so they cannot disagree about which Domains a peer
might belong to: the peer's name, its Domain label, and its controller
`OwnerReference` are all candidates, and owned, legacy and renamed peers alike
wake their Domain.

Those candidates are a search key, never proof. A peer is indexed under its own
name as well as under its label, so looking up peers for a Domain that is
already gone from the API server also returns peers that belong to a Domain that
is still alive. Cleanup after a vanished Domain therefore only applies the
naming contract to peers whose label does not point at a different Domain.

### 3.3 Orphan backstop

Parent-driven cleanup is the primary path. It is not complete: a watch event can
be lost, a finalizer can be removed by force, and children written by older
controllers carry no `OwnerReference` for garbage collection to act on. The
control plane therefore runs a periodic sweeper over namespaced children whose
parent is cluster-scoped — DomainPeers under Domains, Tasks under Jobs.

The sweeper recomputes from current state on every pass and never trusts what an
earlier pass decided. It deletes in two phases, because a single missed parent
read must not be enough to delete a child:

| Annotation | Constant | Meaning |
| --- | --- | --- |
| `rlark.io/stale-since` | `sweep.StaleSinceAnnotation` | When the sweeper first observed this child as orphaned. |

A child whose parent is gone is marked with the timestamp. It is deleted only
once `--orphan-stale-ttl` has elapsed since the recorded mark, which gives an
ordinary reconcile time to recover a lost event. A child whose parent reappears
has the mark removed. The timestamp is persisted rather than kept in memory so a
leader change does not reset the clock and leave the TTL permanently unelapsed.

Matching a parent name is not proof of ownership, so a name alone never leads to
a delete. Before reclaiming a child the sweeper checks the same ownership
evidence the owning controller uses, and stops on anything short of a clear
verdict:

- A child whose controller `OwnerReference` points at another Kind is left alone.
  It belongs to a controller this sweeper knows nothing about.
- A child controlled by a live parent is left alone, even if an annotation on the
  child says otherwise. The live owner reference outranks stale metadata, and a
  contradiction is a reason to stop rather than a reason to delete.
- A child is only reclaimed when every parent name it references is either absent
  or occupied by an object that provably does not own it — for example because
  the child records the UID of an earlier object that used the same name.

A child that references no parent at all is left alone for the same reason:
ownership cannot be proven, and the sweeper never guesses. Pages are bounded by
`--orphan-sweep-page-size` and read through an uncached reader, because the
controller-runtime cache rejects a paged List and an unpaged List over every
child in the cluster is the failure this avoids. The sweeper requires leader
election and jitters its period around `--orphan-sweep-interval`.

### 3.4 Terminal-state archiving

A terminal resource is removed from the management API by the archive registry,
which keeps the terminal test and the TTL in one place per Kind rather than one
controller per Kind. Registered Kinds:

| Kind | Terminal when | TTL origin |
| --- | --- | --- |
| `Job` | `status.phase` is `Stopped` | `status.endTime` |
| `Workflow` | `status.phase` is `Stopped` | `status.endTime` |

`Succeeded` and `Failed` are not archived. They are the outcomes an operator
needs to inspect, and deleting a Workflow cascades to its Jobs. Only the
explicit `Stopped` phase, which the stop path sets deliberately, is a request to
reclaim the object. Every terminal phase records `status.endTime`, so the TTL
normally has an origin.

An object that is terminal but carries no `status.endTime` still has to be
archived, and it is not deleted on the spot either. The first pass that sees it
records the current time and keeps measuring from that instant:

| Annotation | Constant | Meaning |
| --- | --- | --- |
| `rlark.io/archive-observed-since` | `archive.ArchiveObservedSinceAnnotation` | When this controller first saw a terminal object that carries no `status.endTime`. |

The timestamp is written back onto the object rather than held in memory,
because a requeue or a process restart would otherwise restart the clock and the
object would never be archived. An unparsable value is rewritten with the
current time: an arbitrary string must not be allowed to decide when an object
is deleted.

!!! note "Task is not archived on a TTL"
    Task is deliberately absent from the registry. While its Job is alive the
    Job reconciler recreates a missing Task, so deleting a terminal Task on a
    TTL would produce a delete/recreate loop instead of an archive. A Task is
    reclaimed by its owner reference when the Job is deleted, and by the orphan
    sweeper in §3.3 if the owner reference is missing.

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
| `ClusterPhase` | `Online`, `Offline`, `Downgraded`, `Cordoned`, `Draining` |

The `Enum` marker belongs on the phase **type** declaration, not on the
individual `status.phase` field that happens to use it. The closed value set is
a property of the type, so marking the type keeps every present and future
field that references it validated by the same set.

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
| `Progressing` | The controller still has work in flight to drive the resource toward its spec. `True` names that work in `Reason`. | Control-plane Job and Workflow controllers |
| `Ready` | Whether the resource is currently usable. It is the structured companion of `Phase`: `Phase` gives the aggregate stage, `Ready` gives the boolean verdict plus a machine-readable `Reason`. | Control-plane Node and Cluster controllers, data-plane Addon controller |
| `Degraded` | The resource is usable but impaired, so a consumer should expect reduced capacity or reliability rather than an outage. It is only meaningful alongside `Ready=True`. | Control-plane Node and Cluster controllers |
| `Evicted` | Every Worker of the Job was stopped by an eviction (a Node or Cluster Drain) while the Job itself was not suspended. `True` reports that no Worker is left, with `Reason=Evicted` and a bilingual `Message`. | Control-plane Job controller |

`Evicted` describes observed Worker state rather than a verdict on a specific
spec revision, so its writer must not set `ObservedGeneration`. It is withdrawn
once the Job is suspended, leaves `Stopped`, or has at least one Worker that was
not stopped by the eviction.

`Progressing` reports work, not state. It is published only while the
controller has something left to drive, and it is **withdrawn -- not set to
`False`** -- once the resource settles. Setting it to `False` would collapse
"nothing to do" and "the work failed" into one value; failure is reported by
`Validated=False` or by a terminal `Phase` instead. Consumers must therefore
treat an absent `Progressing` as "the writer has no work in flight", never as
"the writer does not implement the condition".

`Degraded` is likewise withdrawn once the impairment clears, so its absence
means "not impaired as far as the writer can tell". A writer that reports
`Degraded=True` must keep `Ready` at its own verdict: `Degraded` narrows a
usable resource, it does not replace readiness.

Rules for every `Conditions` field:

- The field is `[]metav1.Condition` with `+listType=map` and
  `+listMapKey=type`, so a given type appears at most once.
- Write through `apimeta.SetStatusCondition` so that `LastTransitionTime` keeps
  its meaning. Controllers that publish and withdraw conditions in the same
  pass should go through `reconcile.ApplyConditionMutations`, which applies a
  batch of set-or-remove mutations and reports whether anything changed, so a
  status write is skipped when nothing did.
- A condition that is no longer applicable is removed, not inverted. Only write
  `False` when `False` is a verdict the consumer needs ("the spec is invalid",
  "the resource is not usable").
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
exists: `JobStatus`, `WorkflowStatus`, `TaskStatus`, `DomainStatus`,
`AddonStatus`, and `ClusterStatus`. `ClusterStatus` qualifies because
`ClusterSpec.cordoned` and `ClusterSpec.draining` are Spec-driven switches: a
consumer reading `phase` must be able to tell whether the phase already
reflects the latest switch position. It is deliberately absent from `Node`,
`Pod`, and `DomainPeer`, whose status is purely observational -- reported by
the data plane, or summarized by the control plane from heartbeats -- with no
spec to converge on; adding it there would advertise a contract no controller
honours.

For the same reason it is absent from the observation-driven conditions,
`Ready` and `Degraded`, which describe what the controller last saw rather than
which spec generation produced it. The spec-driven conditions, `Validated` and
`Progressing`, must carry it. `conditions.go` is the authoritative split, and
`pkg/reconcile` enforces it with a lint test that rejects a spec-driven
condition literal without an explicit `ObservedGeneration` and an
observation-driven one with it.

Writers advance it only after a reconcile pass has succeeded, never at the
start of one. `metav1.Condition` additionally carries its own per-condition
`ObservedGeneration`, which a spec-driven condition must fill in.

### 4.4 Reason cardinality

A reason is **data, not a message**. The same string is consumed as a
`metav1.Condition` reason, as a Kubernetes Event reason, and as a metric label
value. All three require the set of possible values to be bounded: a reason
built from an object name, a node name, or an error string turns one metric
series into unboundedly many and makes conditions unmatchable by automation.
The human-readable detail belongs in `Message`, which has no such constraint.

`apps/rlark/pkg/reconcile` encodes this in two complementary checks, because
neither one is sufficient alone:

- `ValidateReason` constrains the shape of a value: non-empty,
  UpperCamelCase, ASCII letters and digits only, at most
  `MaxReasonLength` (63) characters.
- `TestReasonFieldsAreConstantExpressions` constrains the shape of the
  *expression* that produces the value. A reason assembled at runtime can
  satisfy `ValidateReason` and still be unbounded, so cardinality has to be
  checked statically. A `Reason` or `ReasonCode` field must be a string
  literal, a declared constant, or a copy of the same field from another value
  (`x.Reason`, `x.GetReason()`); calls, conversions, and concatenations are
  rejected.

Field copies are allowed because they do not create a new reason. A transport
decoder that writes `Reason: wire.GetReason()` passes a value through
unchanged, and the cardinality of that value is a property of whoever produced
it. Requiring a constant at every boundary would force decoders to invent one
and would discard the reason the producer actually reported.

Reasons remain deliberately decentralized: each resource and scenario declares
its own constants near the code that writes them, as noted in
[4.2](#42-condition). The checks above bound how a reason may be *built*, not
which reasons exist.

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
| `tagSelector` | Comma-separated `key=value` pairs. Values of the same key are ORed, different keys are ANDed. Applied after the server window. |
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
committed CRD manifests, deepcopy functions, typed clients, the CRD
reference documentation, or the CRD copies bundled in the Helm chart differ
from the regenerated output. The gate also fails on untracked generated files.

So a change to any type under `api/rlark.io/` must be committed together with:

- `api/config/crd/bases/rlinf.io_*.yaml`
- `api/kubeclients/`
- `api/rlark.io/**/zz_generated.deepcopy.go`
- `apps/rlark/docs/reference/crd.md`
- `charts/rlark-control-plane/files/crds/`

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
