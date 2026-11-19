# AGENTS.md

Brief for AI coding agents working on RLark. For full contribution flow, code style, and PR process see [CONTRIBUTING.md](CONTRIBUTING.md).

**Quick orientation:** RLark is a cross-cluster embodied intelligence platform built with **Go** (control plane + agents) and **TypeScript/React** (web UI). Its management API is Kubernetes-compatible and can use the deployed kcp API server or a configured Kubernetes API. The Kubernetes workload runtime is currently implemented at preview maturity; Docker and Raw workload runtimes are planned and incomplete. The API includes Domain, DomainPeer, Node, Pod, Job, Task, Workflow, and Addon resources. Data-plane agents reconcile Kubernetes workloads, report observed state, and support cross-cluster Pod networking through TUN, gVisor netstack, and SSH relay paths. All user-facing changes need tests and English and Chinese documentation.

## Required design guidance

Before designing or implementing a change, read [Project Design Principles](apps/rlark/docs/developer-guide/design-principles.md). Treat it as the target architecture even where existing code differs. Existing code contains compatibility paths, prototypes, and older patterns; repetition alone does not make a pattern preferred.

Apply this checklist to every change:

- Reconcile from current state; assume events may be duplicated, reordered, or lost.
- Make external effects idempotent, retryable, context-aware, and recoverable after restart.
- Separate desired state, observed state, one-shot operations, events, logs, and audit records.
- Use a Spec switch for a durable desired state, not an edge-triggered command. Model non-repeatable one-shot actions as identifiable operations.
- Treat `ObservedGeneration` as a controller contract for Spec-driven conditions, not an API-server guarantee; consumers must detect stale conditions according to their semantics.
- Define field and resource ownership explicitly; never adopt, mutate, or delete an object whose ownership cannot be proven.
- Use stable business IDs for long-lived identity and Kubernetes UIDs for object-instance identity; do not substitute names or labels for either.
- Preserve compatibility by reading legacy forms and writing one canonical form. Never silently adopt ambiguous legacy data or mutate cached objects while normalizing API responses.
- Prefer Kubernetes-native APIs and typed, validated contracts. Do not expose unsupported capabilities as stable APIs.
- Enforce tenant and authorization boundaries server-side at every entry point. Client-side filtering is not a security boundary.
- Keep Tenant identity independent from Namespace placement in the core model; an upper layer may still configure an explicit one-to-one mapping.
- Inject external dependencies through small interfaces and constructors; avoid hidden globals, untyped placeholders, and constructor side effects.
- Preserve error chains and domain semantics; reject invalid input rather than silently ignoring it.
- Keep logs, metrics, resource events, conditions, and audit events in their distinct roles; never expose secrets or use unbounded metric labels.
- Add failure-path and recovery tests. Update both English and Chinese documentation for user-facing behavior.

When local code conflicts with these principles, preserve required compatibility but move new code toward the documented target. Do not copy an older pattern without first establishing why it is still required.

---

## Documentation synchronization

Documentation is part of the implementation, not a follow-up task. When developing or changing a feature, update the corresponding documentation in the same change so the documented contract never knowingly lags behind the code.

Apply this workflow to every feature, fix, refactor, deprecation, configuration change, and operational behavior change:

1. **Map documentation before editing code.** Search for the affected resource, API route, flag, configuration field, UI flow, controller lifecycle, error behavior, deployment step, and example. Identify the authoritative English page and its Chinese counterpart, plus any README, architecture, API reference, example manifest, or generated reference affected by the change.
2. **Treat current behavior and supported scope precisely.** Update capability matrices, prerequisites, defaults, lifecycle semantics, failure and recovery behavior, security implications, and known limitations. Do not document planned, scaffolded, preview, or partially implemented behavior as stable support.
3. **Update docs during implementation.** Make documentation edits in the same working session and commit as the code and tests. Do not defer them to a future issue or wait until the feature is otherwise complete.
4. **Keep English and Chinese synchronized.** When paired pages exist under `apps/rlark/docs/` and `apps/rlark/docs/zh/`, update both with equivalent semantics, examples, warnings, and limitations. A translation may be phrased naturally, but it must not describe a different contract.
5. **Keep examples executable and canonical.** Update commands, YAML/JSON examples, image references, field names, status values, and migration instructions. New examples must use supported APIs and canonical fields; do not add deprecated compatibility fields to new examples.
6. **Update generated documentation at its source.** If a reference is generated from CRDs, protobufs, OpenAPI, CLI metadata, or another source, change the authoritative source and regenerate the output. Do not hand-edit generated artifacts as the only fix.
7. **Validate documentation with behavior.** Run applicable documentation builds, link checks, schema/example validation, and the feature tests whose behavior the documentation describes. Screenshots or mock data never substitute for backend validation.

Documentation is required even when a change is not a new end-user feature if it changes any externally relevant or operator/developer-observable behavior, including:

- API, CRD, protobuf, SDK, CLI, or configuration contracts;
- defaults, validation, authorization, tenancy, ownership, finalizers, or status/condition semantics;
- deployment topology, image requirements, certificates, ports, storage, networking, upgrade, rollback, or recovery procedures;
- UI workflows, labels, actions, states, errors, or capability visibility;
- supported runtime/workload/device matrices and known limitations;
- logs, metrics, events, audit behavior, health endpoints, and troubleshooting guidance;
- internal architecture boundaries that contributor or provider documentation promises.

A change is not complete when the code and tests pass but relevant documentation is stale. Before finishing, explicitly report which documentation files were updated. If a change truly has no documentation impact, state why in the final summary; never silently omit documentation because the change was described as a refactor or bug fix.

---

## Code structure

- **`api/`** -- CRD type definitions (`rlinf.io/v1alpha1`), generated Kubernetes clients (`kubeclients/`), manifests, and generation scripts. Treat `api/rlark.io/v1alpha1/groupversion_info.go` as the authoritative registered type list.
- **`apps/rlark/`** -- Main Go project (control plane + data plane agents):
  - `cmd/` -- Service, CLI, tooling, and generation entry points. Check this directory and `apps/rlark/Makefile` for the current set and default build targets.
  - `pkg/` -- Implementation packages for control-plane services, agents, networking, authentication, persistence, tenancy, distribution, remote access, logging, metrics, administration, and shared utilities. This list is intentionally non-exhaustive; the directory is authoritative.
- **`apps/embodied-runtime/`** -- Device plugins and controllers for camera, ROS, and ROS 2 integrations, plus supporting initialization, webhook, networking, protocol, and CLI packages. Check its `cmd/`, `pkg/`, and Makefile for the current component set.
- **`apps/rlark-ui/`** -- Frontend management UI: React + TypeScript + Vite. Nginx serves static files and proxies `/api/` to Gateway.
- **`sdks/embodied-runtime-go/`** -- Go SDK for embodied-runtime gRPC stubs.
- **`sdks/embodied-runtime-python/`** -- Python SDK for embodied-runtime (RobotClient / CameraClient).
- **`proto/embodied-runtime/`** -- Proto definitions for embodied-runtime gRPC services.
- **`apps/rlark/docs/`** -- RLark core documentation (EN + CN): architecture, concepts, quickstart, deployment, API reference, examples.

---

## How RLark works

### Control Plane

The control plane uses a Kubernetes-compatible management API. Deployment can include **kcp** or use a configured Kubernetes API. Its main components are:

1. **Server** -- Central hub: HTTPS API for Gateway, SSH server for Agent tunnels, reverse proxy to agents, X.509 + SSH certificate management.
2. **Gateway** -- REST API gateway: exposes CRD operations, auth, storage, cluster, and job log endpoints. Proxies requests to agents via Server.
3. **Controller Manager** -- Runs reconcilers for resources including Domain, Node, Job, Task, and Workflow, plus replication and optional persistence synchronization. Maturity varies by controller; inspect the relevant implementation and tests before relying on a lifecycle behavior.
4. **Web UI (Nginx)** -- Serves the React frontend, proxies `/api/` requests to Gateway.

### Data Plane

Each Kubernetes data plane runs an **Agent** in `cluster`, `node`, or `both` mode:

- **Cluster Agent** (typically a Deployment) -- Reconciles control-plane Tasks into local Kubernetes workloads such as Deployments, DaemonSets, and StatefulSets; synchronizes Addons and related delivery resources; and reports observed Pod and Node state.
- **Node Agent** (typically a DaemonSet) -- Runs node-scoped services including NodeServer routing, network support, image pre-pulling and event reporting, and health/metrics endpoints.

Docker and Raw types and deployment scaffolding exist, but their workload-runtime paths are not implemented. Do not infer support from a type, flag, or deployment option alone.

### Cross-Cluster Networking

The Kubernetes networking path works as follows:

1. A privileged workload sidecar uses a TUN device and gVisor netstack to intercept Pod traffic.
2. NodeServer makes routing decisions from Domain and DomainPeer information.
3. Same-cluster routes may use direct connectivity; cross-cluster routes can be relayed through SSH connections to the control-plane Server.
4. The relay avoids requiring direct inbound reachability between data-plane clusters; do not describe it as a general-purpose NAT traversal mechanism or assume every runtime supports it.

---

## Build and run

```bash
# Build all rlark binaries
make build

# Build specific components
make -C apps/rlark server
make -C apps/rlark gateway
make -C apps/rlark controller-manager
make -C apps/rlark agent
make -C apps/rlark network-sidecar
make -C apps/rlark rlarkadm

# Build embodied-runtime
make -C apps/embodied-runtime build

# Generate proto code (requires the pinned toolchain, see below)
make protoc-install
make proto

# Build Docker images
make docker-build-rlark
make docker-build-rlark-ui
make docker-build-embodied-runtime

# Lint and test repository-supported suites
make lint-go
make lint-web
make test-go
make test-web
```

### Proto toolchain is pinned — never generate with a different version

The gRPC stubs under `sdks/embodied-runtime-go/gen`, `sdks/embodied-runtime-python/embodied_runtime/gen`, and `sdks/execution-go/gen` are checked in and record the generator version in every file header. Generating with a locally installed `protoc` or plugin of a different version rewrites all of them and produces a huge, meaningless diff.

- `proto/toolchain.mk` is the single source of truth for `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`, and `buf`. `sdks/embodied-runtime-python/requirements-dev.txt` pins `grpcio-tools`.
- Do not use whatever `protoc` is on `PATH`. Run `make protoc-install` to download the pinned release into the gitignored, repository-local `proto/bin` directory; Go plugins and `buf` are installed there automatically with version-stamped names. `proto/bin/protoc` then takes precedence over `PATH`.
- `make proto` runs `protoc-check` first and fails on a version mismatch. Fix the toolchain; never relax a pin or edit generated files by hand to make the check or the diff go away.
- If `python3` is not the interpreter holding the pinned `grpcio-tools`, create a throwaway virtualenv and pass `make proto PYTHON=/path/to/venv/bin/python`. Do not install the pin globally and do not change the pin.
- Bumping a pin is a deliberate change: update `proto/toolchain.mk` (and the Python pins), regenerate every stub tree, and verify with `make verify-proto-generated` in the same commit.

Details: [Protobuf stubs and the pinned toolchain](apps/rlark/docs/developer-guide/local-development.md#protobuf-stubs-and-the-pinned-toolchain).

### Running locally

```bash
# Binaries are written under apps/rlark/bin. These examples require a usable
# management API, certificates, and any component-specific dependencies.
./apps/rlark/bin/server --kubeconfig ~/.kube/config --https-port 8443
./apps/rlark/bin/gateway --kubeconfig ~/.kube/config --addr=:8080 --server-address=https://localhost:8443
./apps/rlark/bin/controller-manager --kubeconfig ~/.kube/config --leader-election=false

# Start a Kubernetes cluster agent with credentials issued by the control plane
./apps/rlark/bin/agent \
  --mode=cluster \
  --kubeconfig ~/.kube/config \
  --server-address=https://localhost:8443 \
  --ca-cert /path/to/ca.crt \
  --client-cert /path/to/client.crt \
  --client-key /path/to/client.key
```

Use `--mode=node` or `--mode=both` when node-scoped services are required. Treat each command's `--help`, its config type, and the relevant Makefile as authoritative because flags and component prerequisites evolve.

### Deployment

```bash
# Copy and edit the examples first; replace images, addresses, credentials,
# certificates, storage, and cluster-specific values.
./apps/rlark/bin/rlarkadm install -f apps/rlark/docs/examples/deploy-control-plane.yaml

# Deploy data plane agent
./apps/rlark/bin/rlarkadm install -f apps/rlark/docs/examples/deploy-data-plane.yaml
```

---

## Key concepts

- **Domain** -- A logical grouping and addressing boundary in RLark's virtual network. It is not by itself a complete security-isolation boundary.
- **Node** -- A representation of compute capacity associated with a Domain. Its Spec includes `unschedulable`; observed state is primarily synchronized by Agents, and lifecycle behavior must be verified against the implemented reconciler.
- **Job** -- A workload composed of one or more Task templates.
- **Task** -- A single workload unit described by a runtime-specific spec. For Kubernetes, use the Pod template and Kubernetes-native volume APIs. `pvcStorageMap` and `pvcSizeGbMap` are deprecated compatibility fields and must not be used in new APIs or examples.
- **Workflow** -- A DAG of Job templates; dependencies are between Jobs in a Workflow, not between Task templates in a Job.
- **DomainPeer** -- A namespaced, data-plane-facing view of Domain routing and connection information; it is not merely a binary edge between exactly two Domains.

---

## Style and contributing

- **Go**: Follow [Effective Go](https://go.dev/doc/effective_go) and standard Go conventions. Run `make lint-go` before committing.
- **TypeScript/React**: Follow standard React conventions. Run `make lint-web` before committing.
- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/) format. Every commit must include `Signed-off-by:` (use `git commit -s`).
- **PRs**: Same title format as commits. Fill in the PR template. Link related issues.
- **Tests**: New behavior must include relevant tests, including failure and recovery paths for reconciliation and external effects. Use focused package tests while developing, then run `make test-go` and/or `make test-web` as applicable. If a test cannot reasonably be added, document the reason explicitly.
- **Docs**: All user-facing changes must include documentation updates. When corresponding pages exist, keep the English and Chinese versions synchronized.

Full details: [CONTRIBUTING.md](CONTRIBUTING.md).
