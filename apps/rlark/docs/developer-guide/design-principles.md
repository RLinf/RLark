# Project Design Principles

This document defines the target design direction for the open-source RLark project. It is normative guidance for contributors and coding agents. Existing code is evidence of history, not automatically a precedent: the repository contains mature implementations, compatibility paths, prototypes, and patterns that predate these rules.

When existing code and this document differ:

1. preserve externally required compatibility;
2. avoid expanding the older pattern;
3. move new code toward these principles; and
4. document a deliberate exception when migration cannot be completed in the same change.

## 1. Design from state, not event history

RLark is a distributed control system. Watch notifications, HTTP requests, and connection events may be duplicated, combined, delayed, reordered, or lost.

- Treat an event as a trigger to inspect current state, not as the source of truth.
- Re-read the resource and its dependencies before deciding what to do.
- Do not depend on one controller running before another.
- Make reconciliation safe after process restart.
- Add periodic observation or repair when correctness cannot rely on event delivery alone.

A reconcile loop should normally follow this shape:

```text
read current resource
  -> handle deletion
  -> validate desired state
  -> observe dependencies and children
  -> calculate the difference
  -> perform idempotent effects
  -> patch status and conditions
  -> finish, retry, or requeue with delay
```

## 2. Make effects idempotent and recoverable

Every create, update, delete, persistence, and provider operation must be safe to repeat. A crash after an external action succeeds but before its result is recorded must not create duplicate resources or history.

- Use stable resource, run, and operation identifiers where appropriate.
- Observe before creating and observe again after uncertain results.
- Treat `NotFound` as successful cleanup when absence is the desired state.
- Do not equate acceptance of a delete request with completion of cleanup.
- Use bounded backoff and jitter for unavailable dependencies; do not create retry storms.
- Distinguish invalid input, unsupported capability, conflict, unavailable dependency, in-progress operation, external failure, and internal invariant failure.

Permanent errors should produce a stable condition and wait for relevant input to change. Conflicts should cause a fresh read and recalculation, not replay of a stale update.

## 3. Separate intent, observation, operations, and history

Each representation has one role:

| Representation | Role |
| --- | --- |
| Spec | Durable desired state owned by users or an upstream controller |
| Status | A controller's observation of the current generation |
| Condition | Machine-readable current state and reason |
| Operation | A one-shot action such as start, restart, restore, or purge |
| Resource event | A meaningful user-visible transition |
| Log | Component diagnostics |
| Audit event | Who performed an action, against what, and with what result |

- Do not store controller results in Spec.
- Do not model one-shot commands as indefinitely toggled Spec flags when an operation is the real concept.
- Use Phase for a concise lifecycle summary and Conditions for detailed, machine-readable state.
- Give conditions stable types and reasons, a useful message, and `ObservedGeneration` where generation semantics apply.
- Keep unbounded histories, logs, telemetry streams, and large payloads out of Status.

`ObservedGeneration` is a controller contract, not an automatic consistency guarantee provided by the Kubernetes API server. For a condition derived from Spec, a controller may set `ObservedGeneration` to the current `metadata.generation` only after it has observed that generation and produced the reported result. Consumers should treat such a condition as stale when its observed generation does not match the resource generation. Enforce this behavior with controller tests; schema presence alone cannot prove that the value is correct.

Apply generation freshness according to condition semantics. Conditions such as `Validated`, `Progressing`, `Scheduled`, and workload readiness are usually Spec-driven and should follow the rule above. Conditions based mainly on external facts, such as agent connectivity or infrastructure pressure, may remain meaningful across a Spec change. They should still record the generation observed when written when represented by `metav1.Condition`, but consumers must not assume that every generation mismatch invalidates every kind of external-health fact.

A boolean or switch in Spec is appropriate when it describes a durable desired state. For example, `enabled: false`, `suspended: true`, or `unschedulable: true` means the controller should continuously maintain that state and correct drift after a restart. Prefer state-oriented names such as `suspended` or `stopped` over command-oriented names such as `stop` when that is the intended meaning.

A switch is not appropriate when changing it means "perform this action once." Fields such as `restart: true`, `retry: true`, `purge: true`, or `rotateCredential: true` are ambiguous across retries and restarts: it is unclear who resets the value, whether the action already completed, and how a second identical action is requested. Model these as operations with their own stable identity, target, status, and result.

Use this test: if a controller sees the same value again after restarting, should it keep enforcing the state? If yes, it belongs in Spec. If repeating the effect would be incorrect and the value only represents an action edge, use an Operation. A word such as `stop` can represent either model, so its semantics must be explicit: "remain stopped until changed" is desired state; "stop this run once" is an operation.

Prefer status patches, merge patches, or server-side apply over whole-object updates. Each controller must modify only the fields it owns.

## 4. Use stable identity and prove ownership

Keep these concepts distinct:

- a stable business resource ID identifies a long-lived business object;
- a run or operation ID identifies one execution;
- a Kubernetes UID identifies one Kubernetes object instance;
- a display name or Kubernetes name is not a substitute for either identity.

Before adopting, changing, or deleting a child or external object, prove ownership using the strongest available evidence, such as UID, owner reference, stable parent identity, template identity, and scope metadata.

- Names and labels help locate candidates; they are not sufficient proof by themselves.
- Stop and report a conflict when authoritative metadata disagrees.
- Do not silently take over an unknown object.
- Derived resources must inherit stable tenant and owner identity from their parent.
- A namespace may be authoritative scope metadata when a trusted parent assigns it uniquely. For Cluster-to-Node membership, `Cluster.spec.agentNamespace` is the authority: aggregate only Node CRs in that namespace. Treat `rlark.io/cluster-id` as a compatibility lookup label, never as membership proof. Reject or surface a conflict if two parents claim the same authoritative scope.

## 5. Design deletion as a recoverable workflow

Deletion is state reconciliation, not a single API call.

- Give each finalizer one clear owner and responsibility.
- Do not depend on finalizer array order.
- Make finalizer work repeatable after restart or partial completion.
- Verify that children and external resources are actually gone before completing cleanup.
- Expose blocking reasons through conditions and bounded-cardinality metrics.
- Preserve an auditable manual recovery path for stuck cleanup.

Separate responsibilities such as workload cleanup, persistence, and credential or external-resource cleanup. Required persistence must complete before the active resource is removed. Forced deletion is best-effort and must not be described as lossless.

## 6. Evolve contracts conservatively

Use this migration strategy:

```text
detect legacy form
  -> prove its meaning from authoritative state
  -> normalize at the boundary or backfill safely
  -> write only the canonical form
  -> leave ambiguous data untouched and report it
```

- Preserve compatible reads while converging new writes on one representation.
- Deep-copy informer or lister objects before response normalization; never mutate cache objects.
- Avoid broad historical backfills when read-time normalization is sufficient.
- Deprecate old fields with migration guidance and tests; do not maintain two equally authoritative fields indefinitely.
- Add defaults, schema validation, conversion rules, and compatibility tests before stabilizing an API.
- Reject invalid selectors and unknown enum values rather than silently ignoring them.

An advertised enum, route, runtime, or driver is a contract. Do not publish a stable capability that is not implemented. Mark genuinely experimental surfaces clearly or keep them private until their semantics are ready.

## 7. Prefer native and typed contracts

Reuse Kubernetes conventions and types where they already express the requirement: owner references, finalizers, conditions, status subresources, leases, `PodTemplateSpec`, resource quantities, merge patches, and server-side apply.

- Prefer typed APIs and explicit discriminators over `any` or loosely structured maps.
- Put mechanically enforceable constraints in CRD, protobuf, or request schemas, not only in comments or UI validation.
- Keep quantity and size quotas out of the CRD schema. Enums, list semantics, required fields, types, and formats are structural and belong there; "at most N items" or "at most N characters" is product policy and is enforced on the write path by the Gateway or the writing controller. See [API Conventions](api-conventions.md#7-schema-validation-boundary).
- Distinguish a valid zero value from an absent value.
- Keep generated code generated; change source definitions and regeneration tooling instead.
- Keep unsupported runtime variants out of stable schemas until an implementation and conformance tests exist.

## 8. Keep API and persistence boundaries explicit

The Kubernetes API is the declaration and active coordination interface. The REST API is a stable business view and operation boundary. Persistence is an implementation of durable business data and querying, not a public contract. Provider protocols are independently versioned boundaries.

- Do not expose database rows or raw stored CR JSON as a long-term public API.
- Keep stable business identity separate from Kubernetes runtime metadata.
- Use Kubernetes resources for active reconciliation, not unbounded business history.
- Support direct Kubernetes API operations where the resource API promises them; correctness must not depend on every request passing through the Gateway.
- Give REST APIs consistent errors, filtering, sorting, and pagination.
- Use stable machine-readable error codes and request IDs without leaking sensitive implementation details.
- Represent long-running cross-component actions as observable asynchronous operations.

## 9. Enforce tenancy and security at trusted boundaries

Tenant is a stable business scope, not merely an arbitrary label and not necessarily a Kubernetes namespace. Namespace placement may be resolved from tenant and cluster context, but must not define tenant identity. RLark does not forbid an upper-layer product or deployment from defining a one-to-one Tenant-to-Namespace mapping; it only avoids making that mapping an invariant of the core model. Such a policy should be implemented through explicit configuration or a resolver so the core remains compatible with one-to-many, cluster-specific, or otherwise evolving placement rules.

- A core resource has one primary owning tenant; sharing requires an explicit binding.
- Tenant and owner metadata are set or validated by trusted server or admission paths.
- Derived resources inherit tenant ownership; downstream controllers do not change it.
- Authentication, authorization, and admission/policy are distinct responsibilities.
- Model authorization as subject, action, resource, scope, and context, with deny as the safe default.
- List filtering does not replace authorization at Get, logs, exec, proxy, terminal, or SSH time.
- Do not trust client-supplied headers, labels, annotations, names, IDs, paths, or tunnel origin as proof of authorization.
- Apply equivalent isolation to Gateway and direct Kubernetes API paths.
- Audit sensitive allow and deny decisions without recording secrets.

## 10. Model lifecycle and capabilities explicitly

Do not collapse independent dimensions into one phase. For example, a node's connection, readiness, schedulability, resource pressure, and lifecycle are distinct.

- Treat infrastructure resources as long-lived; temporary offline state is not deletion.
- Use lease-like mechanisms for high-frequency liveness instead of repeatedly rewriting a large Status.
- Use server time, configurable timeouts, and hysteresis for connection state.
- Advertise runtime, workload, network, storage, operation, and device capabilities explicitly.
- Do not infer all capabilities from a broad runtime type.
- Validate task requirements against available capabilities before dispatch when possible.

Capability declarations should include availability and version where compatibility depends on them.

## 11. Use explicit dependencies and lifecycle

Business logic belongs in testable packages; command entry points should primarily assemble and run components.

- Inject external clients through constructors and small interfaces based on real substitution points.
- Accept `context.Context` for blocking or remote work and honor cancellation and deadlines.
- Validate configuration before starting background work.
- Make background worker start and stop behavior explicit.
- Avoid package globals, hidden service locators, untyped placeholder dependencies, and constructors that start goroutines unexpectedly.
- Do not create speculative abstractions with empty implementations merely to imply future backend support.

Long-running managers should observe drift and self-heal. They should define concurrency and lock ownership clearly and be testable with fake clients or injected clocks.

## 12. Standardize configuration and errors

Long-running components should converge on:

```go
DefaultConfig()
SetupFlags(...)
Validate()
```

- Provide conservative defaults.
- Validate ranges and cross-field relationships.
- Keep configuration values immutable after startup unless the component explicitly supports reload.
- Keep environment-variable interpretation at configuration boundaries.

In Go, wrap errors with operation context and `%w`. Preserve typed error semantics with `errors.Is`, `errors.As`, and `errors.Join`. Do not ignore an error without an explanation. HTTP and RPC boundaries should map domain errors consistently rather than expose arbitrary internal strings.

## 13. Give each observability signal one job

- **Conditions** answer what the resource state is now.
- **Resource events** explain meaningful transitions.
- **Logs** provide diagnostic detail.
- **Metrics** show aggregate health, performance, and capacity.
- **Audit events** record security-relevant actions and outcomes.
- **Traces** connect selected cross-component operations.

Propagate useful request, correlation, tenant, resource, run, task-run, and operation context across components. Do not use unbounded resource IDs, usernames, or error text as routine metric labels. Rate-limit repeated logs and events. Observability failures must not block core reconciliation.

Never put tokens, private keys, secret values, or sensitive environment variables in logs, events, metrics, traces, or errors returned to users.

Keep liveness and readiness separate: liveness answers whether the process must be restarted; readiness answers whether the component can safely accept work.

## 14. Keep the UI typed and layered

- Route business requests through the shared transport and domain API layers.
- Convert wire DTOs into typed page or domain models at a boundary; do not spread `any` through components.
- Extract parsing, normalization, validation, and state-transition logic into pure utilities with focused tests.
- Separate data hooks, form models, and presentation components as pages grow.
- Centralize refresh behavior, pause when hidden where appropriate, cancel stale requests, and prevent older responses from overwriting newer state.
- Install mocks at the transport boundary rather than adding mock branches throughout business components.
- Keep UI and API semantics aligned, but never rely on the UI for authorization or validation that protects the system.

## 15. Test failure and recovery semantics

Tests for controllers and distributed components should cover more than the successful path:

- duplicate reconciliation and lost notifications;
- stale update conflicts;
- restart and reconstruction from observed state;
- partial external success;
- dependency outage and recovery;
- delayed deletion and already-absent resources;
- ownership conflicts and ambiguous legacy objects;
- cache immutability during response normalization;
- cancellation, timeout, and concurrent shutdown;
- tenant and authorization denial paths;
- configuration drift and self-healing.

Prefer fake clients and small injected interfaces for deterministic unit tests, then add integration or conformance tests where behavior crosses a real API boundary.

## 16. Contribution and compatibility gates

All user-visible changes require tests and English and Chinese documentation. Public functions and types need useful documentation. Production code uses structured logging. Follow repository linting, Conventional Commits, and sign-off requirements.

Changes to stable contracts should include the applicable checks:

- CRD schema and old-object compatibility;
- generated clients and manifests;
- protobuf compatibility;
- OpenAPI and route consistency;
- database forward migration;
- provider conformance;
- REST and UI behavior alignment.

## Agent design checklist

Before implementation, an agent should answer:

1. What is the authoritative desired state and observed state?
2. Who owns each field, child object, and finalizer?
3. Which identifier is stable business identity, run identity, and Kubernetes instance identity?
4. What happens when the event is duplicated, lost, or processed after restart?
5. What happens if an external operation succeeds but its response is lost?
6. How is legacy data read, and what makes adoption unambiguous?
7. Which tenant and authorization checks apply at every entry point?
8. Which failures are permanent, conflicting, in progress, or retryable?
9. Which condition, event, metric, log, or audit record explains the result?
10. Which failure and recovery tests demonstrate convergence?

If these questions do not have clear answers, the design is not ready to implement.
