# Runtime, Workload, and Environment Boundary

This document describes the internal execution boundary between RLark's management Task lifecycle, Workload semantics, Environment requirements, and runtime object operations. It is an implementation guide, not a public API or evidence that a runtime is supported.

## Compatibility scope

Kubernetes is the only supported workload runtime. Docker and Raw fields are API scaffolding and cannot execute workloads. The execution boundary is internal: it does not define CRDs, generated clients, REST endpoints, CLI flags, configuration, status schemas, finalizer or annotation keys, or generated Kubernetes object shapes.

The Kubernetes compatibility contract includes the fixed workload namespace, legacy workload adoption, claimant identity format, in-place template mutation, SSH key injection, cross-domain network sidecar injection, and cleanup order. Workload and carrier selectors are frozen when a Task is claimed. Changing either selector on a claimed Task is rejected without Runtime effects; callers must delete and recreate the Task with a new UID. Native PodTemplate storage semantics apply: `persistentVolumeClaim.claimName` references an existing external PVC, while Kubernetes owns PVCs created from `ephemeral.volumeClaimTemplate`.

The Host treats a claimed Runtime as immutable. If `spec.runtime.kind` or `spec.runtime.name` changes on an already claimed Task, the Host keeps the claimed Runtime untouched, does not touch the newly selected Runtime, reports `WaitingForChange`, and requires the selector to be restored or the Task to be deleted and recreated. Other Spec changes in the same invalid generation are not partially applied. On deletion, cleanup targets the claimed Runtime and Task identity. This prevents an accidental selector edit from becoming an implicit destructive migration.

A claimant-less legacy cleanup finalizer is adopted only while the Task is being deleted or when the current Agent can realize the Task. A non-deleting Task selected for another Agent remains untouched: the Host does not add claimant metadata, write Status, call the Executor, or clean up a Runtime. This prevents ambiguous legacy metadata from authorizing cross-Agent cleanup.

## Implementation layout

Runtime-neutral contracts and orchestration are grouped under `pkg/agent/execution`:

- `plan` owns portable compiled Plans, explicit Environment mutation targets, and independent workload-unit declarations;
- `workload` owns Workload Adapter contracts and selection;
- `environment` owns requirements, resolved bindings, trusted Handler registration, material leases, and canonical non-secret binding revisions;
- `binding` projects units, applies Environment handlers to a private Plan copy, invokes Runtime lowering, and performs final validation;
- `runtime` owns runtime object, instance, candidate inventory, profile-selection, and unit-lowering contracts;
- `coordination` and `coordination/coordinator` own Runtime-neutral selection, diff, ordering, and execution orchestration;
- `host` owns management Task claims, the cleanup finalizer, lifecycle dispatch, and the Task status fields written by the data-plane Agent;
- the `runtime/transport/grpc` and `workload/transport/grpc` subtrees adapt internal interfaces to the versioned execution protocol.

Concrete Task API compatibility and Kubernetes implementations are controller-owned under `pkg/agent/controllers/task`, not under the neutral framework:

```text
controllers/task/compatibility/kubernetes/
├── task.go             # current Task API decoder
└── kubernetesobjects/  # legacy metadata, ownership, and profile IDs/projections

controllers/task/workload/
└── ray/kubernetes/   # one Ray-oriented Adapter with three carrier kinds

controllers/task/runtime/kubernetes/              # Provider, profile executors/registry, lowering
  supervision/                                    # mandatory tools installation and process supervision
controllers/task/environment/kubernetes/          # composed Kubernetes Environment Resolver
  network/                                        # cross-domain Network Resolver/Handler
  ssh/                                            # workload SSH Secret Resolver/Handler and local material source
  tensorboard/                                    # TensorBoard Resolver/Handler
```

The production Task pull reconciler is a thin adapter over the Runtime-neutral
Host. Production Kubernetes assembly currently constructs one Coordinator with
an immutable implementation registry and one Kubernetes Runtime registration:

```text
Task Host
  -> exact (Workload Type, Workload Version, Runtime Kind) implementation
  -> Coordinator Executor
  -> Runtime Provider
```

The Workload and Runtime concepts remain orthogonal, but they are not freely
combinable. Only explicitly registered triples are selectable. The Host and
Coordinator share the same immutable implementation registry. A registration is
rejected at startup when it is duplicated, its Adapter metadata does not match,
or its Runtime kind is not declared by the Adapter. Kubernetes currently
registers one `(rlinf.io/ray, v1, Kubernetes)` implementation. The Ray Adapter
independently decodes `workload.carrier` and supports the
`rlinf.io/kubernetes-deployment`, `rlinf.io/kubernetes-daemonset`, and
`rlinf.io/kubernetes-statefulset` carriers at version `v1`. These carriers are
not general-purpose native Kubernetes workload implementations.

An unclaimed Task is decoded and checked against this exact registry before any
claim metadata is written. Invalid input or an unregistered combination remains
unclaimed and causes no Runtime write. If an already claimed, non-deleting Task
can no longer be decoded, validated, or matched to a registered implementation,
the Host preserves its claim and existing Runtime objects and waits for support
or Spec to be restored. It does not interpret implementation availability as
authorization to destroy a running workload. Task deletion and an explicit
Lifecycle cleanup select the frozen claimed Runtime and Workload implementation
and clean up by Task identity without depending on the current Workload Spec,
keeping those workflows recoverable after a downgrade or partially migrated
update. A complete per-Runtime Bundle/Factory remains a separate assembly step;
this document does not claim that it is already present.

Deployment, DaemonSet, and StatefulSet are carrier choices within the single Ray
Adapter registration, not Workload selection entries.

This compatibility tree is deliberately controller-owned rather than part of
the Runtime-neutral `execution` framework. It preserves decoding of the current
Kubernetes Task API, ownership proof for objects created by older Agents,
historical carrier ownership recovery, and Kubernetes reconcile
profile IDs and projection helpers needed during upgrades. Profile executors and
their immutable registry are Runtime-owned. The compatibility tree is temporary
migration debt, not a stable extension surface. It must be deleted when supported
upgrades no longer need to read or adopt those legacy objects and the public Task
API no longer requires the compatibility carrier selectors. New features must
not add contracts to this tree merely for convenience.

The Coordinator receives Runtime-specific object comparison through an injected
comparator on the Runtime registration. The Kubernetes comparator delegates each
versioned reconcile profile to the same immutable Runtime-owned executor registry
used by desired-object validation and Provider Apply; the Coordinator itself does
not know Kubernetes resource kinds or import this compatibility tree.

This internal correction does not alter API/CRD definitions, registration keys,
generated Kubernetes object names, manifests, or lifecycle behavior. A future
general Kubernetes Workload API requires a separate public contract and
capability design; the current carrier discriminators must not be treated as
such support.

## Internal responsibilities

The Runtime-neutral Task Host owns:

- reading and filtering management Tasks;
- claimant annotations and the cleanup finalizer;
- preserving claimed Runtime objects when a non-deleting Task cannot be decoded,
  validated, or matched to a registered implementation, and waiting for the
  original contract to be restored;
- identity-based cleanup for Task deletion and explicit Lifecycle cleanup;
- comparing the frozen claimed Workload and carrier selectors with the currently decoded selectors and rejecting in-place selector changes before Runtime-specific Lifecycle processing;
- dispatch of Runtime-specific Lifecycle decisions, including Task stop and deletion transitions;
- freezing the claimed Runtime, Workload, and carrier selectors and validating the exact
  Workload type, version, and Runtime-kind combination before claim; the
  Coordinator uses the same registry to select and execute the Workload Adapter;
- patching the Task phase, message, observed nodes, and compatibility conditions
  owned by the data-plane Agent. Both pull reconciliation and targeted Kubernetes
  observation use this status boundary.

The control-plane Task controller separately owns only the aggregated
`pullProgress` and `events` fields. It does not write the phase, message,
observed nodes, or compatibility conditions owned by the data-plane Host.

Finalizer ownership is entry-scoped, not ownership of the entire
`metadata.finalizers` list. Claim appends only `rlark.io/agent-cleanup`; release
tests and removes only that exact entry. Finalizers owned by other controllers
are preserved. The same patch tests the Task `resourceVersion`, so a reordered
or concurrently changed list is retried from a fresh read rather than removing
an entry by a stale index.

For a non-deleting legacy claim created before the frozen Workload selector was
introduced, the Host may backfill the selector annotations only when both fields
are absent and the current Task decodes to one registered implementation. The
optimistic claim patch preserves the claimant, finalizer, and all
unowned metadata, then normal reconciliation continues. A partially present
selector is ambiguous and remains blocked; it is never completed from the
current Spec. During deletion, the existing narrower fallback decodes a wholly
missing selector only for identity cleanup and does not persist it.

The claim freezes the Runtime scope. A claimed Task cannot be moved to another
Runtime by changing `runtime.kind` or `runtime.name`; the Agent leaves the existing
Runtime untouched and waits for the original values to be restored or for the Task
to be deleted and recreated. Decode, validation, or implementation-registration
failures follow the same non-destructive rule for a non-deleting claimed Task.
Deletion still uses the claimed Runtime and identity-based cleanup.
The cleanup finalizer and claim annotations are released only after a complete
Runtime snapshot confirms that every exclusively owned cleanup object is absent.
An accepted delete request is not cleanup completion. If a delete response is
lost, the next reconciliation re-reads Runtime state and safely confirms absence
instead of depending on the previous response. If the management Task is already
NotFound, the Host drops the reconcile request and never attempts name-only
cleanup because the Task UID and claimed identity can no longer be proven.

The built-in Ray-on-Kubernetes Workload Adapter owns rendering, desired-object lifecycle declarations, update compatibility, and semantic observation. The Kubernetes Lifecycle computes Task stop decisions, the Host dispatches those decisions, and `Coordinator.CleanupTask` handles identity-based Task deletion cleanup from the Adapter inventory without requiring the current Workload to compile. Job restart is outside the Agent lifecycle: the Job controller replaces Task objects and the Agent only observes their normal deletion. Its compatibility implementation currently includes:

- deciding whether the current Kubernetes Task shape can be realized;
- inventory-declared Kubernetes cleanup lifecycle and ordering;
- Workload-owned template mutations and workload identity, applied once during Workload compilation before rendering the desired Kubernetes objects; cross-domain network injection is instead performed by Environment binding;
- image pull secrets, PVCs, and Ray resources;
- declarative workload SSH requirements; the Environment layer owns Secret delivery and Pod-template mounting;
- rendering the selected Deployment, DaemonSet, or StatefulSet workload for the Coordinator and Runtime Provider to apply.

The legacy `Operations` adapter remains only as a tested compatibility port; production assembly uses the Ray-on-Kubernetes Adapter and does not call its `ApplyWorkload` method.

For a Task with a non-empty domain, the built-in Adapter emits one `CrossDomain` network requirement and authorizes one exact PodTemplate target on the primary carrier. The production Kubernetes Resolver selects an immutable trusted profile containing the shared Agent image. The network Handler mutates only the Binder-owned Plan copy, always adding `rlark-network-sidecar` as a restartable init container together with the `/var/run/rlark` NodeServer socket volume and mount and the required privileged/`NET_ADMIN` settings. Equivalent declarations are idempotent; a conflicting container or volume name blocks execution. A missing image also blocks the Task instead of silently creating a workload without requested cross-domain networking. DomainPeer routes and NodeServer SSH relay credentials remain data-plane state and never enter the Task binding or binding revision.

RLark tools and process supervision are mandatory execution-unit facilities rather than Task Environment declarations. A Workload Adapter does not request them. After Environment binding, the selected Runtime's workload assembler receives every declared `WorkloadUnit`, installs the trusted tools binary, and lowers process declarations into that Runtime's supervisor mechanism. The Kubernetes implementation owns both operations in `runtime/kubernetes/supervision`; they are one mandatory assembly facility rather than separately selectable features. It injects the existing `rlark-tools-init` copy operation, EmptyDir, and main-container mount. Runtime implementations that cannot provide this contract must fail closed rather than silently run an unsupervised workload. Kubernetes currently supports one workload unit per Plan and rejects multiple units until stable per-unit supervisor object identities are implemented.

For recognized Ray head and worker Tasks, the Adapter declares exactly one required primary `ProcessDeclaration` for `rlark-tools ray-launcher`; it does not declare tools or supervisor requirements. The SSH Handler contributes one optional auxiliary `ProcessDeclaration` for `rlark-tools sshd`, using the typed port selected by the Adapter (22 for head and 2222 for worker); the Handler does not infer Ray roles. If no contributor declares a primary for a workload unit, the workload assembler supplies the portable default primary `/rlark-tools/rlark-tools idle`, which waits for termination without relying on a shell or `sleep` implementation in the workload image. After all Environment handlers finish, the Kubernetes workload assembler validates the declarations, creates a Task-incarnation-owned supervisor ConfigMap, and replaces the main-container entrypoint with `rlark-tools supervisor`. Its content digest is recorded on the Pod template so process configuration changes trigger rollout. The Secret contents are not part of this configuration or digest, so key rotation alone does not intentionally roll the workload.

`rlark-tools supervisor` is a Unix-only runtime. It runs as PID 1 and starts the primary and auxiliary declarations as independent process groups. On Linux it attempts to enable child-subreaper behavior, but failure (for example, a seccomp denial of `PR_SET_CHILD_SUBREAPER`) is logged and does not block startup because namespace PID 1 already adopts orphaned descendants. The supervisor reaps adopted descendants and forwards termination to each process group. Windows is not supported. A required auxiliary is a resident service: any exit, including exit code zero, is a service failure. With `RestartOnFailure`, non-zero exits receive at most three consecutive recovery restarts with linear backoff. A run lasting at least five minutes resets the consecutive-failure sequence, so occasional failures over a long workload lifetime do not exhaust a lifetime budget; a zero exit remains terminal rather than restartable. Shutdown is ordered. When Kubernetes requests termination or a required auxiliary becomes terminal, the supervisor first sends `SIGTERM` only to the primary and waits for its actual exit; this wait has no supervisor deadline, so Kubernetes remains the final bound for a primary that never exits. Once the primary has exited—whether by itself or after that request—the supervisor sends `SIGTERM` to remaining auxiliaries and starts its bounded 15-second auxiliary shutdown period, followed by `SIGKILL` if necessary. After `SIGKILL`, it waits at most another five seconds for reaping and then logs each unreaped auxiliary instead of blocking PID 1 forever. Lifecycle logs use RLark's structured `logr`/zap pipeline and expose process names, roles, PIDs, exit codes, runtimes, restart counts, shutdown transitions, and escalation; they never include argv, environment variables, or secret material. The supervisor CLI preserves a primary process's non-zero exit code; required-auxiliary and supervisor-internal failures use the generic tool failure code. Primary exit determines normal workload completion; exhausting a required auxiliary's consecutive restart budget still fails the supervisor. Network and TensorBoard are not process declarations and remain under their existing Kubernetes lifecycle. `ray-launcher` does not own network configuration. When the Ray Adapter observes a canonical Network Environment, it enables a built-in, context-aware DNS readiness barrier before the user prepare script; external `WAIT_NETWORK_SCRIPT` hooks are no longer used. User prepare and run scripts remain explicit shell compatibility boundaries. The Ray lifecycle is implemented in Go; the prepare-state trampoline and Ray SDK cluster check are embedded in the binary from package files and are not delivered as ConfigMaps or mounted scripts.

Process declarations are a portable-plan contract, are deep-copied, and are included in the Adapter compiled hash. The Workload Adapter protocol transports them together with Workload units, exact mutation targets, and Environment requirements. The Runtime Provider receives only the final assembled manifests and read-only Runtime-unit projections; it does not interpret Environment requirements or process semantics.

After the optional user prepare-script boundary and before any Ray stop or start command, `ray-launcher` verifies that the configured Ray CLI can be resolved and is executable. A missing or non-executable CLI fails the primary. On the head, the embedded Ray SDK cluster check remains non-fatal for compatibility, but the following `ray status` command is fatal. On a worker, failure to start `ray start --block` and any later non-zero Ray exit both fail the primary; an ordinary process exit preserves the Ray exit code through the launcher and supervisor.

Workload semantics are selected by `spec.workload.kind` and `spec.workload.version`; arbitrary JSON `spec.workload.parameters` is strictly decoded by the selected Adapter. `spec.workload.carrier` independently contains the Runtime-specific carrier `type`, `version`, and arbitrary JSON `parameters`. Deployment and StatefulSet carriers accept only `{replicas}`; the DaemonSet carrier accepts only an empty object. Environment declarations use the same type/version/parameters envelope in an ordered list and may repeat. Each selected Environment resolver owns any uniqueness rule for its contract. Unknown types, versions, malformed parameters, and unsupported Runtime capabilities fail before Provider effects.

The Task decoder maps every Environment envelope generically without enumerating built-in types: it checks only common required fields, preserves ordering and duplicates, deep-copies parameters, and leaves type-specific uniqueness, version, and parameter validation to the selected Runtime resolver. Kubernetes Environment resolvers are installed through an immutable type-to-resolver registry; empty types, nil resolvers, and duplicate type registrations fail during construction. The composed resolver groups declarations by type in first-appearance order and passes every declaration of one type to that type's resolver in original order. Network, SSH, and TensorBoard currently reject multiple declarations themselves; future Environment contracts may register a resolver that accepts and resolves multiple declarations. Unknown types decode successfully but fail closed during resolution before Provider observation or effects. The portable execution plan preserves this same Environment envelope instead of splitting requirements into network, secret, tool, service, storage, device, or config fields. Each plan requirement contains `name`, open-string `type` and `version`, copied parameters, and its consumer selectors. SSH authorized public keys are retained only in the local execution declaration and omitted from portable requirement parameters and workload transport; the SSH handler acquires them through the material boundary.

Domain is virtual-network configuration, not Task placement or Runtime identity. `Task.spec.placement` contains scheduling constraints only. A standalone Task requests cross-domain networking with exactly one `rlinf.io/network` environment at version `v1` and parameters `{domain}`; duplicate Network declarations are rejected before Provider effects. The Job controller retains `Job.spec.domain` as an upper-layer convenience and lowers it into that Task environment. The Kubernetes Network handler owns the resulting `rlark.io/management-task-domain` PodTemplate annotation and sidecar mutation; the Job controller does not write Kubernetes-specific workload metadata directly. Host claims do not persist or compare Domain.

For a recognized Ray head or worker Task, an SSH Environment becomes a separate workload SSH Secret requirement for its exact PodTemplate target. The Environment's `authorizedKeys` array contains public authentication configuration: the material provider reads it from the deep-copied local Task snapshot, while portable Plan parameters, remote workload transport, resolved bindings, Status, logs, errors, revisions, and operation identities omit it. Tasks without the recognized Ray workload and SSH Environment do not receive the automatic workload SSH lifecycle. `HandlerRequest.Task` supplies explicit Task identity for the generated object rather than requiring the Handler or Provider to infer ownership from names or labels.

The SSH Handler declares one Task-incarnation-owned opaque Secret, makes the workload depend on it, and mounts its `authorized_key` item read-only at `/var/run/rlark/ssh/authorized_key` in the main container. The directory mount deliberately does not use `subPath`, so kubelet can update the projected file by atomically switching the volume projection. Secret creation precedes workload creation; Task cleanup deletes the workload before the Secret. The Secret name is derived from the logical Task name and a Task UID digest, and the Kubernetes profile permits in-place data updates only after canonical current-incarnation ownership has been proved. It never adopts, updates, or deletes a same-name foreign or ambiguously owned Secret.

An empty `authorized_key` file means that workload public-key authentication is disabled; the built-in Ray workload still attempts to start the SSH auxiliary when that Environment is configured. `rlark-tools sshd` reopens the configured file for every authentication attempt, so rotating or clearing referenced key material normally updates only the Secret after the initial migration rollout. SSH availability is not part of Task success: an initial optional-auxiliary start failure is logged and ignored; a clean SSH exit is terminal; and a non-zero exit receives the supervisor's bounded restart attempts. A failed restart or exhausted attempts are also logged and ignored, leaving the primary and Task running without SSH. Consumers must not copy the file into a durable location, cache its resolved `..data` path, or retain an open file descriptor. Updates are subject to kubelet Secret-projection convergence; this contract does not promise instantaneous revocation. The former `RLARK_SSH_PUBLIC_KEY` environment fallback and running-Pod `exec` append path are not used. NodeServer and DomainPeer SSH relay credentials are a separate data-plane mechanism and are unaffected.

If the Task requests `rlinf.io/tensorboard/v1` with a non-empty `parameters.logDir`, the Environment Handler emits a separate auxiliary-service requirement. This is an intentional behavior change. The legacy TensorBoard mutation ran only from Ray initialization and was therefore gated on a recognized Ray head or worker role. The Environment requirement instead depends only on the explicit, non-empty `logDir` and applies to any built-in Kubernetes carrier. The TensorBoard Handler selects its trusted image from `RLARK_TENSORBOARD_IMAGE`, falling back to the historical `rlinf/tensorboard:2.15.1-rc1` when the variable is unset or blank. This environment variable is a transitional assembly mechanism until validated typed Agent configuration owns the value; Tasks and Adapters cannot select arbitrary images.

The Handler reuses an existing main-container volume mount that covers the requested absolute directory when possible, or creates the historical EmptyDir otherwise, and emits TensorBoard as a restartable init sidecar. Equivalent declarations are idempotent. Conflicting names, mount paths, or relative TensorBoard directories fail closed before Provider observation or effects. TensorBoard and Network remain separate Environment handlers and profiles. Tools installation and supervision are instead mandatory Runtime workload assembly facilities.

The legacy `RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES=true` mutation runs during Workload compilation, before Environment binding. It therefore applies only to containers and init containers already present in the Adapter-produced Pod template; it does not blanket-upgrade containers injected later by Environment handlers. This ordering does not remove permissions required by Environment features: the trusted Network profile explicitly declares privileged/`NET_ADMIN` for `rlark-network-sidecar`, while Tools and TensorBoard use the security contexts declared by their own handlers. The unsafe switch must not be treated as authority for future Environment handlers to request privilege implicitly.

After Environment handlers finish, the Kubernetes Runtime lowerer performs the final capability adaptation over every Adapter-declared `Pod` or `PodTemplate` unit. `KUBERNETES_SUPPORT_SIDECAR_FEATURE` is read once during production assembly. Unless its value is exactly `false`, restartable init containers are preserved. When it is `false`, every init container whose `restartPolicy` is `Always` is moved, in declaration order, after the existing regular containers and its init-only restart policy is removed; ordinary init containers remain unchanged. A name collision with an existing regular container, or an unsupported unit locator, fails closed before Provider effects. This applies uniformly to sidecars produced by Workload compilation and Environment binding rather than letting either producer interpret the Runtime capability.

For a claimed, non-deleting Task, the Host treats both the Workload selector and
carrier selector as immutable. A changed Workload selector reports
`WorkloadSelectorChanged`; a changed carrier selector reports
`CarrierSelectorChanged`. Neither path deletes the existing carrier, creates a
replacement, or rewrites the frozen claim. The caller must restore the original
selectors or delete and recreate the Task with a new UID. Task deletion and
suspension continue to use the frozen selectors and identity inventory, so
cleanup remains recoverable even when the current Spec is invalid or has changed.

These shared operations are implementation reuse, not a fourth Workload type. The Kubernetes Runtime boundary owns generic object and instance operations and must not absorb RLark-specific Task orchestration semantics.

The Go interfaces are internal implementation contracts within the repository. They are narrower than the service-based Runtime Provider contract and must not be treated as a stable public SDK or wire protocol.

## Remote boundaries

The components that need external implementations have versioned remote protocol definitions:

- `WorkloadAdapter`: Workload metadata and Configuration Schema, declarative Compile, semantic Observe, and identity-derived Inventory;
- `RuntimeProvider`: runtime information, object Get/List/Apply/Delete, Adapter-directed candidate snapshots, legacy claimed-Workload recovery, and complete instance snapshots;

Workload Adapters may be built in or remote. Both implement the same internal `workload.Adapter` contract, return declarations only, and cannot access Runtime Providers or write Task status directly. This is the primary extension path for externally implemented Job types such as VolcanoJob or PyTorchJob.

Environment Resolver/Binder, Coordinator, Compatibility Mapper, and Instance Projection remain in-process so platform policy, final admission, and external API mutation stay in RLark Core. Logs, Exec, and Watch are not included in the first Runtime protocol version; they should be added as optional services only when a real remote implementation requires them.

The internal Environment path separates declaration from effect:

```text
Adapter.Compile
  -> desired objects + portable requirements + explicit MutationTargets
Resolver.Resolve
  -> validated concrete consumers + trusted non-secret profile metadata
PlanBinder.Bind
  -> Runtime-ready ExecutablePlan produced from a deep copy
Coordinator
  -> binding revision + inventory + diff
Runtime Provider
  -> observation and ordered effects for the final ExecutablePlan
```

`MutationTarget` is an allow-list entry, not a general JSON mutation API. It names one Plan object by logical identity, declares a closed target kind, and uses a static path interpreted and validated by the selected Runtime Binder. Requirements should reference exact target names. Legacy object/role/unit selectors are compatibility inputs only and must be reduced to concrete targets before a binding handler runs. Targets are transported together with Environment requirements by the remote Workload Adapter protobuf. Requirements without their exact target authorization are an incomplete and unsafe Plan, so conversion preserves both contracts atomically rather than serializing requirements while dropping targets.

Mutation targets, Workload units, and final Runtime-unit projections share one neutral `runtime.UnitKind` enum. Its current closed values are `PodTemplate` and complete `Pod`, but the type is deliberately not Pod-specific so future Runtime contracts can add other explicitly supported unit shapes without creating parallel enums. Sharing this kind describes structure only; it does not make a Workload unit an Environment mutation authorization. Either current kind may use whole-object locator `.` when the typed desired object itself has that semantic, including Runtime-specific resources that represent a PodTemplate directly, or a static field locator such as `spec.template` when embedded in another object. The neutral `execution/runtime/unit` accessor only reads and writes opaque JSON at `.` or dot-separated existing object fields; it rejects arrays and JSONPath operators and does not interpret Runtime or unit semantics. A `Pod` target never identifies Pods created later by a Deployment, DaemonSet, StatefulSet, or another controller. Declaring a path does not prove that arbitrary JSON has Pod semantics; the selected Runtime lowerer, Handler, and final validator must validate the located value's typed structure.

The Kubernetes Network Handler and Runtime lowerer use that accessor and then decode the located value as a complete Pod or PodTemplate. They do not branch on the enclosing workload GVK, so an Adapter may place an authorized unit in any Kubernetes workload object or CRD at a valid static field path. The Network Handler owns both the cross-domain sidecar/socket mutation and the Pod metadata domain annotation; the Workload Adapter retains Task name, namespace, UID, and workload identity metadata but no longer writes the network-owned domain annotation. Missing paths, malformed Pod structures, type mismatches, name conflicts, and unsupported unit kinds fail closed before Provider effects. This generality applies only to explicitly declared units and mutation targets; it does not authorize manifest scanning or mutation of controller-created running Pods.

Kubernetes reconcile profiles are executed through an immutable Runtime-owned registry shared by desired-object validation, Provider Apply, and Coordinator comparison. Each executor owns validation, comparison, and update preparation for one exact versioned profile ID; duplicate registration and unknown non-empty profiles fail closed. This removes the central profile switch without turning profiles into an untyped policy language. Built-in workload compatibility remains intentionally narrow while historical carrier adoption, recovery, discovery, and cleanup are separated from the general path.

The Kubernetes Provider resolves objects by their declared GVK. It preserves typed objects for GVKs registered in the injected Scheme and uses Kubernetes unstructured objects and lists for other discoverable GVKs, so generic exact-object CRUD and Adapter-directed inventory are not restricted to the built-in carrier kinds. A reconcile profile must still be registered and validate the object's contract before an update; dynamic decoding does not imply that an arbitrary CRD supports a built-in workload profile.

`WorkloadUnit` is separate from mutation authorization. An Adapter declares each Pod-like unit as `PodTemplate` or direct `Pod`, with its owning logical object and a typed static locator. Binding validates and projects these declarations, runs Environment handlers, then invokes the selected Runtime's effect-free `runtime.UnitLowerer`. The neutral interface accepts one `runtime.DesiredObject`, not a Workload Plan; the Kubernetes implementation is owned by `controllers/task/runtime/kubernetes`. A lowerer may adapt the final declared units for trusted Runtime version or capability differences, but performs no Runtime I/O, cannot depend on Workload or Environment packages, and remains covered by final Plan validation. Read-only object-relative `RuntimeUnit` metadata stays on the final `DesiredObject`, so a local Runtime Provider can recognize Pod semantics without scanning manifests or receiving Environment target authority. The Provider still applies the final manifest and must not treat units as permission for hidden mutation. Current remote Workload Adapter and Runtime Provider protobufs do not carry units; both transports reject non-empty units rather than silently dropping them. Wire support requires an additive, capability-negotiated protocol update using the pinned protobuf toolchain.

Resolvers and Binders must complete before the Coordinator reads a Provider snapshot or performs a Provider effect. A Binder mutates only a deep copy of the Plan, performs no management or Runtime I/O, and cannot return a partially applicable plan after a failure. Providers therefore receive only final Runtime-ready objects and do not infer network, SSH, or other Environment business semantics.

Shared portable execution data is owned by the neutral `execution/plan` package: the compiled `Plan`, its Environment requirement and mutation-target declarations, the final `ExecutablePlan`, and deep-copy behavior. Callers reference these definitions directly rather than through Workload or Environment aliases. The `execution` tree contains Runtime-neutral contracts, orchestration, validation, and transports only; it does not provide production Runtime, Workload, or Environment implementations. Task-controller-owned implementations live under `controllers/task/workload/ray/kubernetes`, `controllers/task/runtime/kubernetes`, and `controllers/task/environment/kubernetes` and may depend on the framework, never the reverse. Do not add shared data types to the root `execution` directory.

Environment handler order is not a fixed sequence of business phases. Trusted local production assembly registers handlers with stable IDs and mandatory `Requires` plus optional `Before`/`After` dependencies. A required handler must be selected by an explicit binding in the same request; the framework does not invent a binding with ambiguous input. Optional ordering references apply only when both handlers participate. The registry rejects duplicate IDs/selection keys and dependency cycles, copies ordering metadata at construction, and uses handler ID as the stable tie-breaker so registration order or later handler mutation cannot change execution. Task and Adapter payloads cannot name handlers or alter the graph. Ordering also does not authorize last-writer-wins behavior: trusted handlers declare canonical, non-secret semantic write intents before mutation; equivalent writes are idempotent and different values for one slot fail with `Conflict` before any handler applies. A concrete Binder must still perform final Runtime-schema validation after mutation; intent declarations do not prove that a resulting manifest is valid. This validation seam preserves dependency direction: `binding.DesiredPlanValidator` applies `runtime.ValidateObjects` once for common logical identity, ownership, reference, and DAG invariants, then invokes the selected Runtime's narrow, effect-free `DesiredObjectValidator` for each declaration. Binding owns `BindRequest` and Plan knowledge; Runtime packages must not import binding contracts or repeat graph validation.

This is an incremental internal contract. The dedicated `execution/binding` package owns `BindingPlanBinder`, its request/interface, mandatory Runtime workload-assembler hook, and final Plan-validation adaptation; `plan` remains a data-contract package, while `workload` owns Adapter semantics. Production Kubernetes assembly runs this Binder with the Kubernetes final Plan validator, mandatory tools/supervisor assembly, and a composed Resolver with separate trusted Network, workload SSH, and TensorBoard handlers. The built-in Ray-on-Kubernetes Adapter emits one exact PodTemplate target and unit plus its primary process declaration but no longer performs those mutations itself. Unsupported Storage, Device, non-built-in Secret, or Config requirements remain fail-closed before Provider observation or effects.

Definitions live under `proto/execution`, with generated Go bindings under `sdks/execution-go/gen`. Regenerate them from the repository root:

```bash
make proto-execution
```

Generation requires the pinned toolchain; run `make protoc-install` first and see [Protobuf stubs and the pinned toolchain](local-development.md#protobuf-stubs-and-the-pinned-toolchain).

Transport adapters live under `execution/runtime/transport/grpc` and `execution/workload/transport/grpc`. Their clients implement the internal `runtime.Provider` and `workload.Adapter` interfaces; their servers expose internal implementations through the generated SDK. A Workload Adapter receives the canonical typed `WorkloadConfiguration`: the common Pod template, opaque workload parameters, and one versioned carrier envelope containing opaque parameters. Workload identity remains in `WorkloadSelector`, while carrier identity is also frozen in `CarrierSelector`; transport conversion requires the selector and configuration envelope to agree. The Adapter is solely responsible for strictly validating workload and carrier parameters and producing one complete Plan. The existing protobuf `TaskSnapshot.configuration` remains a JSON `TypedValue`, so this JSON-shape update does not require a protobuf schema change. Environments are resolved through their separate ordered envelopes; there is no generic extension list, Runtime-native configuration escape hatch, or post-compile object-patch stage. Protobuf messages do not become the internal domain model. Protocol major version 2 transports the complete Plan contract, including Environment requirements, mutation targets, Workload and Runtime units, process declarations, and the binding revision. It also advertises capabilities for Workload inventory, Runtime candidate inventory, owned-field projection, legacy claimed-Workload recovery, proven Task-incarnation adoption, and typed reconcile profiles. The Coordinator currently enforces the adoption and reconcile-profile capabilities when those declarations are used. The older inventory, projection, and claimed-recovery flags are transported and advertised, but capability negotiation is not yet the uniform selection mechanism for those paths; the Core also checks the required Go interfaces and validates the declarations at execution time. Consumers must not treat every advertised flag as proof that all compositions have already been negotiated. Both transport adapters are covered by in-memory gRPC round-trip tests but are not yet selected by the default Task controller configuration.

Transport adapters map stable execution faults to gRPC status codes and restore them for `errors.Is`-style handling. The optional interceptors in `execution/transport/grpc` provide the same mapping for additional execution services. Request correlation and distributed tracing are not part of the current protocol; request IDs will only be added together with an end-to-end tracing implementation.

Runtime `ListCandidateObjects`, legacy `ListManaged`, and `ListInstances` are atomic full-snapshot operations. A provider must return an error if it cannot produce the complete result set; partial successful snapshots are not supported and must never drive cleanup or absence decisions.

The selected Workload Adapter derives a bounded inventory from the frozen Task and implementation identity. Candidates are declared in creation order. Identity cleanup uses `LifecycleDeleteOnTaskCleanup` as the deletion authorization and removes the last currently present exact-name cleanup candidate per reconciliation, reversing that order; it does not maintain a Kubernetes-kind allow-list. The Ray-on-Kubernetes Adapter declares the SSH Secret before its workload consumers so workload objects disappear before the Secret. It does not declare PVC candidates: referenced external PVCs are outside RLark ownership, and generic ephemeral PVCs follow their Pod lifecycle. The Kubernetes Provider executes only Adapter declarations; it does not compute Ray names or infer Ray roles. Candidate names and labels locate objects but never prove ownership: the Provider verifies canonical Task identity and object UID where applicable before returning an object. Query failure fails the complete snapshot atomically. Instance observation UID-fences the workload root, lists only Pods selected by that workload, and queries Pod Events with the `involvedObject.uid` field selector.

Each desired Kubernetes object selects a typed reconcile profile instead of asking the Provider to infer update policy from its manifest kind. The current Ray Adapter selects workload projection for Deployment, DaemonSet, and StatefulSet carriers, Service projection for the head Service, and shared-field projection for bootstrap ConfigMaps.

The immutable Runtime-owned registry dispatches the explicitly selected versioned executor and verifies that the typed manifest matches that profile. A missing, unknown, duplicate, or mismatched profile is rejected. Each executor implements validation, comparison, and update preparation for one bounded Kubernetes mechanism, such as comparing a Service-owned projection. The Adapter remains responsible for selecting the mechanism that represents its Workload contract.

Before scheduling a Service replacement, the Coordinator compares only the Service profile's owned fields through this registry. API-server materialization and allocation—including ClusterIP, ClusterIPs, IP family settings, NodePort, default protocol and target port, traffic policy, session affinity, and unrelated metadata—does not cause replacement drift. The built-in registry remains intentionally narrow; its workload executor is not a general CRD update policy.

Before comparing a desired Deployment, DaemonSet, or StatefulSet Pod template with the object returned by the API server, the Kubernetes provider conservatively preserves commonly materialized Pod and container defaults where the desired template left those fields unspecified. The current compatibility set covers Pod restart policy, DNS policy, scheduler name, termination grace period, an empty Pod security context, and container image-pull and termination-message defaults. It prevents common repeated updates without overriding explicit desired values, but it does not reproduce every version-dependent nested container, probe, volume, or ephemeral-container default. The provider currently treats the complete Pod template as Agent-managed; fields injected into that template by mutating admission webhooks or another controller are not a supported ownership boundary and may be overwritten and re-injected. Clusters should not apply such mutation to RLark-managed workload templates until an explicit Agent-owned field projection or server-side-apply ownership contract is implemented.

Ray bootstrap ConfigMaps use RLark-reserved fixed names and are retained across Task cleanup. The Adapter explicitly projects ownership of their `data`, `binaryData`, and Ray-script identity annotation into each desired object; the Provider updates only that projection and does not recognize Ray names. Equal script content is a zero-write steady state, while drift is repaired in place. Other labels and annotations are preserved. A legacy fixed-name ConfigMap without the identity annotation can be normalized, but conflicting Ray-role metadata is left untouched and reported as a conflict. A shared object with a missing or unsupported projection is rejected rather than updated as a whole. Operators must not assign these reserved names to unrelated ConfigMaps or let another manager write the script fields.

A `persistentVolumeClaim` PodTemplate volume is reference-only and must provide the name of an existing PVC in the workload namespace. The Ray Adapter and Kubernetes Provider do not create, adopt, validate, mutate, inventory, or delete that PVC. Generic ephemeral volumes remain part of the Pod template; Kubernetes creates and deletes their PVCs according to the Pod lifecycle.

The Ray head Service follows the primary workload rather than the retained shared-resource policy. It is owned by one Task incarnation, carries canonical Task UID metadata, and declares `Replace` update semantics. A conflicting managed Service is deleted with UID fencing and recreated on a later reconciliation, so API-server-assigned addresses or ports may change and a brief interruption is possible. Its stable identity excludes Task generation and adopted-generation annotations; generation changes alone therefore do not replace the Service. Task-incarnation ownership requires canonical current Task UID metadata by default. The Ray Adapter explicitly permits proven adoption for its primary workload and head Service: a compatible legacy workload uses the existing narrow workload proof. For upgrade compatibility, the Provider also returns an exact-name, non-workload auxiliary candidate with no canonical Task UID when the same complete inventory snapshot contains exactly one proved workload root. This lets `Replace` remove an old head Service that has neither canonical annotations nor an owner reference. Label-selected candidates, a foreign canonical UID, a controller owned by an unproved object, or an absent or ambiguous root block this path. Deletion remains UID-fenced. This policy does not apply to referenced external PVCs or shared ConfigMaps; external PVCs are not RLark-managed, while shared ConfigMaps retain their projection semantics.

Execution failures retain a typed fault code such as `InvalidArgument`, `NotFound`, `Conflict`, `PermissionDenied`, or `Unavailable`, while wrapped local errors keep their underlying cause for `errors.Is`/`errors.As` and diagnostics. The gRPC transports map these codes to status codes and restore the execution fault category on the client side.

The production coordinator has three separate paths:

- `Reconcile` compiles and binds the desired plan, asks the selected Adapter for identity inventory, reads the complete candidate snapshot, computes a diff, applies ordered operations, and observes the result.
- `CleanupTask` selects the claimed Runtime and the Workload implementation frozen in claim annotations, asks the Adapter for identity inventory, and uses each exact candidate's declared lifecycle after Provider ownership proof. It never recompiles the current Spec, so Task deletion remains recoverable when that Spec cannot be decoded. For claims written before the frozen Workload selector existed, a deleting Task whose selector fields are both absent may use the still-finalized Task only to decode and registration-check the Adapter selector; cleanup still uses identity inventory and Provider ownership/UID checks, and the recovered selector is not persisted. This narrow upgrade fallback lets a Job restart clean up an old Task even when deletion is the new Agent's first observation. A partially present selector, an undecodable Task, or an unavailable registration blocks deletion. Outside that deleting-Task fallback, legacy selector recovery still requires one uniquely owned workload root with complete execution revision evidence; ambiguous or conflicting evidence blocks deletion. Exact candidates declared `DeleteOnTaskCleanup` are deleted in reverse inventory order with object-UID fencing, independent of Kubernetes kind. Shared Ray script ConfigMaps remain `Retain`; external PVC references are absent from inventory.
- `ObserveRoot` gets one UID-fenced workload root, lists only its instances and relevant events, and calls the selected Adapter's `Observe`. It does not compile, resolve, bind, call `ListManaged`, compute a diff, or write Runtime objects. Runtime push reconcilers only produce hints; the pull controller uses this path for a high-frequency root change only after the fast-observation gate verifies current Task, claim, ownership, UID, Runtime, Workload, and revision evidence.

Cleanup never depends on an in-memory cache. Targeted observation re-reads the current Task and Runtime root, but its optimization gate also requires revision evidence established by a successful full reconciliation in the current Agent process. The cache is not authoritative: a restart, missing entry, desired-state trigger, periodic trigger, legacy metadata, or conflicting evidence causes a full reconciliation.

The Host classifies each reconciliation as exactly one of `Succeeded`,
`WaitingForChange`, `Retrying`, `CleanupInProgress`, `Blocked`, or `Dropped`.
Retry behavior is typed (`None`, `OnChange`, `Conflict`, `Transient`, or
`InProgress`) rather than inferred from error strings. Status intents carry the
Task UID and generation so stale observations cannot update a recreated or newer
Task incarnation.

These outcomes and retry classes are a Host-level scheduling vocabulary, not a
complete end-to-end Provider-to-Host retry protocol. The
Kubernetes production path actively uses success, waiting, retrying, cleanup in
progress, and dropped outcomes. `Blocked` is presently limited to a few stable
post-claim contract errors and defensive lifecycle branches; the canonical
ownership and policy conflicts expected to use it broadly are deferred to later
metadata and ResourcePolicy work. Provider and Coordinator fault codes are
preserved where defined, but not every fault is mapped to a distinct Host retry
class.

An absent exact Workload type, version, and Runtime-kind registration remains a
`NotFound` execution fault. A registration that exists but has no usable
Provider or Adapter is an invalid assembly and remains `FailedPrecondition`.

The Host serializes reconciliation by management Task namespace and name while
allowing different Tasks to run concurrently. Runtime-specific compatibility
lifecycle code receives the external Task through an opaque lifecycle input;
the generic Host does not inspect Kubernetes workload fields. Lifecycle code
returns decisions and status intents and must not patch the Task directly.
Status intents distinguish an omitted field from an explicitly empty value, so
condition-only updates preserve phase, message, and observed nodes. Intents
produced in the same reconciliation are fenced, merged by priority, and written
once where the lifecycle permits it.

The Kubernetes execution path coalesces desired-state, Runtime-state, and periodic hints in a bounded process-local accumulator. Saturation is reported and never silently evicts an unrelated Task; watch replay and periodic resync provide recovery from lost hints. Runtime push controllers do not observe roots or write Task Status directly. Verified fast observation intentionally bypasses the Host keyed lock and Lifecycle hooks: it may only read Runtime state and perform one UID/generation-fenced workload Status write. Any effect required on every reconciliation must not be added only to Lifecycle hooks unless the fast-path gate is also extended or disabled.

A shared cluster-Agent resync starts immediately and scans management Tasks assigned or eligible for the same Host with pagination and a bounded 10,000-Task budget. Host claim and eligibility rules are shared with reconciliation rather than reimplemented by the scanner: an effective claim requires the Agent finalizer, and a claimed Task remains assigned to its owning Host despite selector drift or deletion. Subsequent scans use a jittered interval in the range 80–120% of `--task-runtime-resync-period`; the default is `15m`, `0` disables the scan, and a negative value is rejected at startup. A transient management API list failure is logged and retried after the next interval instead of terminating the Agent. Trigger-bridge pressure is summarized after the round and does not stop later Tasks or pages from being visited. The scan emits periodic full-reconciliation hints and does not treat its in-memory progress as desired state.

For a proven-owned primary Deployment, DaemonSet, or StatefulSet, normal Apply writes canonical root annotations for Runtime identity (ID/kind), Workload identity (type/version), compiled and executable digests, and the non-secret binding revision. Existing Task name, namespace, UID, generation, and domain annotations remain the ownership and lifecycle compatibility contract. The Job restart annotation is not copied to Runtime objects and is not interpreted by the Agent. Metadata is written only on the root object's top-level annotations, not its Pod template, so metadata repair alone does not roll out Pods. Missing, malformed, or mismatched canonical revision metadata cannot authorize targeted observation and selects full reconciliation. For a canonical-owned workload, full Apply repairs this complete canonical execution revision only after Task ownership (including the Task UID annotation), the current object instance from the UID-fenced reference, and selector compatibility have been proved. A narrowly compatible legacy workload may instead be adopted from its name, namespace, kind, and selector shape when no conflicting ownership metadata exists; that adoption writes canonical Task identity and revision metadata but preserves the Pod template. Subsequent canonical-owned Apply can repair the revision at the same Task generation, making identifier normalization and digest-algorithm upgrades recoverable. Execution revision annotations are ordering and optimization evidence, not ownership proof by themselves.

The binding revision is a canonical `sha256:` digest of binding kind/name, trusted profile identity and revision, sorted concrete target consumers, ownership, lifecycle, source revision, handler algorithm, and value type. It excludes raw binding values and Secret contents. The executable hash likewise replaces Kubernetes Secret `data` and `stringData` with a fixed marker before hashing; the real desired Secret remains available to the Runtime comparator so rotation still produces an in-place Secret update. `ResolvedBinding.Value` may contain only controlled, serializable, non-sensitive options; raw keys, tokens, and credentials require a separate restricted in-process material mechanism and must not enter portable Plans, transport, Status, diagnostics, errors, hashes, or revisions. Production diagnostic sanitization cannot be disabled at runtime; observer failures or panics do not change reconciliation, Status, or Runtime effects. Before wiring new diagnostic payload shapes, extend and test sanitization for every value-bearing field: the current generic sanitizer covers environment `value` fields, sensitive-key names, and Secret `data`/`stringData`, but does not infer secrets embedded in container `command`/`args` or in non-sensitive-key `TypedOptions` values. Environment-based Network, workload SSH, and TensorBoard mutation plus Runtime-mandatory RLark tools and supervisor assembly are used in production.

The restricted material mechanism is Binder-local, generic, and non-serializable. A trusted `MaterialProvider` receives Task identity plus resolved binding identity and returns an opaque `MaterialLease`. `HandlerRequest.Materials` is always nil during semantic write-intent declaration and is non-nil during `Apply` only when a valid Provider is configured; a handler that requires material must treat nil as unavailable capability. The framework closes all leases after each handler on success, error, or panic. Implementations must return sanitized errors and must not retain, log, serialize, hash, or expose leased bytes. Environment core deliberately defines no SSH key names, injection modes, or compatibility profiles; the controller-owned Kubernetes SSH package owns those semantics. Coordinator production assembly may construct a per-Task Binder from the already decoded `TaskSnapshot`; that factory must perform no external I/O, and the resulting material provider is scoped to that evaluation rather than shared across Tasks. `HandlerRequest.Task` carries the same explicit identity used by the resolved environment so handlers can declare owned objects without reconstructing identity from manifests.

Normal pull reconciliation does not stop merely because a Task is already `Running`. A later Task generation is compiled and diffed again so supported changes such as replicas, image, and Pod template updates can converge through the same Coordinator path. Generation metadata is an ordering fence, not proof that Runtime state still matches desired state: full reconciliation repairs out-of-band drift in controller-owned replicas and Pod-template fields even at the same Task generation. Reconciliation remains zero-write when those fields and canonical metadata already match. A legacy workload's first adoption and repeated reconciliation at its adopted generation remain metadata-only so an Agent upgrade does not silently roll out its existing Pod template; desired fields begin converging after a later Task generation.

## Current capability matrix

| Capability | Kubernetes | Docker | Raw |
| --- | --- | --- | --- |
| Task claim and finalizer | Supported | Not implemented | Not implemented |
| Deployment | Supported | Not applicable | Not applicable |
| StatefulSet | Supported | Not applicable | Not applicable |
| DaemonSet | Supported | Not applicable | Not applicable |
| Container/process execution | Kubernetes Pods | Spec scaffolding only | Spec scaffolding only |
| Status observation | Supported | Not implemented | Not implemented |
| Runtime logs | Kubernetes Pod fallback | Not implemented | Not implemented |
| Exec | Kubernetes path | Not implemented | Not implemented |
| Existing PVC references and generic ephemeral volumes | Kubernetes-native Pod semantics; RLark does not own referenced PVCs | Not implemented | Not implemented |
| GPU and devices | Kubernetes resource semantics | Not implemented | Not implemented |
| Addon and Delivery | Supported | Not implemented | Not implemented |
| Cross-cluster workload networking | Kubernetes sidecar path | Not implemented | Not implemented |
| Image pull observation | Supported | Partial monitor only; no execution path | Not applicable |

This matrix documents current behavior only. These capabilities are not advertised through Node or another external resource.

## Contract extension requirements

Runtime Providers and Workload adapters may be extended with version negotiation, idempotent operations, observation, logs, exec, recovery, service identity, and conformance tests. Such extensions require separate API review and must preserve the Kubernetes compatibility and Runtime-selector contracts documented above.
