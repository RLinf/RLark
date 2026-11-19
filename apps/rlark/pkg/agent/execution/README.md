# Task Execution Packages

This directory contains RLark's runtime-neutral Task execution core. It separates management Task lifecycle, Workload semantics, environment binding, Runtime operations, reconciliation hints, safety evidence, and diagnostics so that no layer needs to own the whole process.

The code is internal implementation, not a stable public SDK. The authoritative design contract is documented in [`apps/rlark/docs/developer-guide/runtime-provider.md`](../../../docs/developer-guide/runtime-provider.md).

## Package map

| Package | Responsibility | Must not do |
| --- | --- | --- |
| `host` | Owns Task claim/finalizer entries, lifecycle dispatch, Agent-owned Task Status, and one-Task serialization | Directly mutate Runtime objects or infer cleanup permission from names/events |
| `coordination` | Defines immutable Task snapshots and Runtime/Workload selectors; contains registration helpers | Access any concrete Runtime implementation |
| `coordination/coordinator` | Selects an exact implementation, requests Adapter inventory, compiles and binds plans, observes current state, calculates diffs, and performs ordered Provider operations | Patch management Tasks or silently adopt/delete ownership conflicts |
| `workload` | Defines Runtime-neutral Workload Adapter, inventory, and observation contracts | Perform Runtime CRUD, provide a concrete Workload implementation, or make Runtime packages depend on Workload contracts |
| `binding` | Orchestrates unit projection, Runtime lowering, Environment handlers, and final Plan validation on a deep copy | Perform management or Runtime I/O or return a partially applicable Plan |
| `environment` | Defines requirements, Adapter-authorized mutation targets, concrete consumers, trusted profile metadata, and canonical non-secret binding revisions | Infer mutation locations, perform Runtime CRUD, or place raw credential material in portable/resolved values or diagnostics |
| `runtime` | Defines Provider, object candidates, object identity, ownership, lifecycle, owned-field projections, manifests, snapshots, and Runtime operation contracts | Interpret management API objects or infer Workload resources |
| `runtime/unit` | Reads and writes opaque JSON unit values at validated static field locators | Interpret Runtime schemas, unit semantics, arrays, or arbitrary JSONPath expressions |
| `trigger` | Coalesces typed desired-state, Runtime-state, and periodic hints in bounded process memory | Treat hints as authoritative state or persist correctness-critical facts |
| `evidence` | Defines canonical digests/revisions and the conservative fast-observation decision gate | Perform I/O or mutate metadata while evaluating a gate |
| `observability` | Records bounded, best-effort execution events through injected observers | Affect reconcile decisions, expose secrets, or become an authoritative history |
| `fault` | Classifies execution failures independently of transports | Flatten or discard underlying error chains |
| `transport/grpc` and nested transport packages | Map typed execution contracts across gRPC boundaries | Introduce new business semantics in transport conversion |

## Dependency direction

The intended dependency flow is:

```text
external Task representation
        │
        ▼
 injected decoder ──► coordination.TaskSnapshot
        │
        ▼
      host ──► coordinator ──► workload.Adapter
                         ├────► environment.Resolver / PlanBinder
                         └────► runtime.Provider

trigger ──► controller bridge ──► host or verified ObserveRoot
evidence ────────────────────────► fast/full-path decision
observability ◄── host / coordinator / provider diagnostics
```

Lower-level contracts do not import controllers or command assembly. External clients and diagnostics are injected. Constructors must not start hidden goroutines.

## Full reconciliation

A normal Task reconciliation follows current state rather than event history:

1. `host` serializes the management namespace/name and re-reads the Task.
2. It validates claim ownership, frozen Runtime/domain, deletion, and lifecycle state.
3. It decodes the Task and selects the exact `(Workload Type, Version, Runtime Kind)` implementation.
4. `coordinator` asks the Adapter to compile desired objects, portable Environment requirements, and explicit mutation targets.
5. The Environment Resolver validates the requirements against only those targets. A Binder works on a deep copy and produces a complete `ExecutablePlan`; a Resolve or Bind failure returns before any Provider snapshot or write.
6. `coordinator` computes the canonical non-secret binding revision and orders desired objects.
7. The selected Adapter derives bounded object inventory from the Task and implementation identity.
8. The selected `runtime.Provider` executes those candidates and returns complete object and instance snapshots.
9. `coordinator` calculates deterministic operations and executes them through the Provider.
10. It re-observes current state after writes instead of trusting operation responses.
11. `host` merges fenced Status intents and writes only Agent-owned Task Status fields.

### Environment target and revision contract

`plan.Plan.Targets` is an allow-list of locations that a Binder may mutate. A target has a stable name, references a desired object by `LogicalName`, declares a closed target kind, and carries a Binder-defined static locator. Arbitrary JSONPath and scans by physical object name are not authorization. Requirements should reference exact target names. Older object/role/unit selectors remain compatibility inputs and must be normalized to concrete targets before a binding handler runs.

Mutation targets, `Plan.Units`, and final `RuntimeUnit` projections share the neutral `runtime.UnitKind` enum; they do not maintain parallel kind definitions. The enum currently defines `PodTemplate` and `Pod` but is not Pod-specific and may add other explicitly supported unit shapes later. Sharing the kind describes structure only and does not give a WorkloadUnit Environment mutation authority. Either current kind may use locator `.` when the typed desired object itself has that semantic, or a Runtime-defined static locator when the value is embedded in another object. A `Pod` target never represents a controller-created running Pod. Binding projects validated units onto their object, runs Environment handlers, then invokes an effect-free Runtime lowerer for final version/capability-aware manifest adaptation before validation. It retains read-only `RuntimeUnit` metadata on the final `DesiredObject` for the local Provider. Existing remote Adapter and Provider protocols reject non-empty units until an additive capability-negotiated wire contract is introduced.

The `runtime/unit` accessor treats located values as opaque JSON. It accepts `.` or dot-separated static object fields, requires every field to exist, and rejects array indexing and JSONPath operators. Concrete Runtime and Environment implementations remain responsible for interpreting and validating the located value's schema; the accessor does not make arbitrary JSON a valid unit.

Binding revisions are canonical `sha256:` digests of non-secret execution semantics: requirement identity, trusted profile identity and revision, sorted concrete consumers, ownership/lifecycle, source revision, handler algorithm, and typed value schema. Raw binding values and secret contents are excluded. `ResolvedBinding.Value` is only for controlled, serializable, non-sensitive options; raw credentials require a separate restricted in-process material mechanism.

Sensitive material uses a generic Binder-local mechanism. A trusted `MaterialProvider` acquires an opaque `MaterialLease` by Task and resolved-binding identity. `HandlerRequest.Materials` is always nil while handlers declare write intents; during `Apply` it is non-nil only when a valid Provider is configured, so handlers that require material must treat nil as unavailable capability. The framework closes every acquired lease after each handler, including error and panic paths. Material must not influence write intents or enter Plans, binding values/revisions, transport, Status, logs, traces, metrics, or errors. Concrete feature packages—not the Environment core—own their material names, delivery options, compatibility checks, and Handlers.

The neutral `execution/plan` package owns the shared portable `Plan`, Environment requirement/target data contracts, `ExecutablePlan`, and their deep-copy behavior. Callers reference those definitions directly rather than through aliases. `workload` owns Adapter contracts, `binding` owns cross-boundary binding orchestration, and `environment` owns resolution and handler contracts. The root `execution` directory is not a catch-all common-types package. Concrete implementations and production assembly live outside this tree. They may import the execution framework; the framework must not import them.

Handler ordering is configured by trusted Bundle code rather than a fixed list of Environment phases. Each registered handler has a stable ID and may declare mandatory `Requires` plus optional `Before`/`After` edges. `Requires` must refer to another handler selected by an explicit binding in the same request; the framework does not manufacture an empty binding. Missing optional ordering targets are ignored, cycles are rejected, and otherwise-unconstrained handlers are ordered by ID so registration order cannot change output. Registration copies ordering metadata so later handler mutation cannot alter the graph. Task and Adapter payloads cannot name handlers or provide ordering edges. Ordering never grants overwrite permission: all trusted handlers declare semantic write intents before mutation, equivalent values are idempotent, and conflicting values fail the whole Bind before any handler applies. The concrete Binder must still perform final Runtime-schema validation after mutation; intent declarations are not a substitute for validating the resulting manifest. The dependency seam is deliberately split: `binding.DesiredPlanValidator` applies `runtime.ValidateObjects` once for common logical identity, ownership, reference, and DAG invariants, then invokes a narrow, effect-free `DesiredObjectValidator` implemented by the selected Runtime for each declaration. Binding owns `BindRequest`/Plan knowledge; Runtime packages must not import binding contracts or repeat graph validation.

Deletion and explicit lifecycle cleanup select the claimed Runtime and the Workload implementation frozen in claim annotations. The Adapter derives identity-only inventory without recompiling the current Task Spec, and the Provider executes only those candidates. A legacy claim without a frozen Workload selector can recover it only from one canonically owned root with complete execution revision evidence; missing, ambiguous, name-only, or foreign evidence blocks cleanup. Delete acceptance is not cleanup completion; finalizers remain until every exclusively owned cleanup object is confirmed absent. A missing frozen Adapter registration also blocks cleanup rather than allowing the Provider to guess an inventory.

## Triggers and fast observation

Watch events and periodic scans produce `trigger.Trigger` values. UID, generation, Runtime ID, and root references are hints only. `trigger.Accumulator` coalesces storms while the controller queue carries only the Task key.

The `evidence` gate permits targeted `ObserveRoot` only when current Task, claim, Runtime, Workload, root UID/ownership, and revision evidence all agree. Missing, legacy, stale, or conflicting evidence falls back to a full Host reconciliation. A fast path must not compile, diff, write Runtime objects, or scan an entire namespace.

Accumulator contents are intentionally process-local. Losing them during a restart may delay reconciliation, but watch replay and periodic resync must restore convergence.

## Canonical identity and ownership

Keep these identifiers distinct:

- management namespace/name: stable logical queue key;
- management Task UID: Task object incarnation fence;
- Task generation: desired-state version;
- Runtime ID/kind: selected Provider identity;
- Runtime object UID: concrete object incarnation fence;
- digest/revision: evidence that compiled, bound, desired, and observed projections agree.

Names and labels locate candidates but do not prove ownership. Read compatible legacy metadata, write one canonical form, and leave ambiguous or conflicting objects untouched.

Adapters declare object candidates together with explicit ownership, lifecycle, update mode, dependencies, and optional typed reconcile profiles. The Coordinator validates the declared contracts and capability agreement but does not infer Workload roles from manifest kinds. Runtime-specific comparison is supplied through the registered effect-free `ObjectComparator`; nil uses JSON semantic equality. Providers must reject unsupported profiles and prove ownership before applying or deleting objects. Compatibility adoption, default-field projection, retained resources, and shared-field policies belong to concrete implementations outside this tree.

## Observability

`observability.Observer` is diagnostic only. The default observer is a no-op. Recording and future exporter adapters consume typed, bounded events without changing execution results.

- Dynamic payload selection uses an injected `ConfigSource`.
- Production sanitization cannot be disabled by runtime configuration.
- Payloads are bounded and truncation is explicit.
- Observer errors and panics are isolated from reconciliation.
- An unsafe sanitizer is compiled only with the non-default `rlark_execution_trace_unsafe` diagnostic build tag and must not be present in standard binaries or release images.

Do not put tokens, private keys, Secret values, sensitive environment values, or raw credentials in logs, events, metrics, traces, errors, or Task Status.

## Adding an implementation

When adding a Workload or Runtime implementation outside this tree:

1. define a typed, versioned contract;
2. explicitly register each supported Workload × Runtime combination;
3. have the Adapter declare bounded inventory, ownership, lifecycle, and any shared-object owned-field projection;
4. keep Provider effects idempotent, context-aware, and restart-recoverable, without inferring Workload names or roles;
5. prove ownership before adoption, mutation, or deletion;
6. return complete candidate and instance snapshots or an error, never a successful partial snapshot;
7. advertise every optional protocol capability that the implementation requires; the Coordinator currently gates incarnation adoption and typed reconcile profiles explicitly, while inventory, owned-field projection, and claimed-recovery capabilities still rely on interface selection and execution-time validation until capability composition is unified;
8. add duplicate/lost-event, conflict, capability-missing, projection, partial-success, deletion, and restart tests;
9. update the corresponding English and Chinese project documentation.

Scaffolding, enum values, or configuration options do not by themselves make a Runtime supported.

## Focused validation

From `apps/rlark`:

```bash
GOTOOLCHAIN=go1.26.5 go test ./pkg/agent/execution/...
GOTOOLCHAIN=go1.26.5 go test -race ./pkg/agent/execution/...
```

Repository-wide lint and tests remain required before merge.
