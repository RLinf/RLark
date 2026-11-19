# RLark Roadmap

[English](ROADMAP.md) | [简体中文](ROADMAP.zh-CN.md)

This roadmap describes how the open-source RLark project will evolve from a Kubernetes-based workload platform into open, extensible infrastructure for cross-cluster embodied intelligence. RLark will provide stable resource models, APIs, and extension mechanisms for integrating external systems and adapting deployments to different environments. Priorities and implementation details may change as the project evolves.

## Design Principles

- Keep tenant isolation, identity, authorization, resource ownership, and lifecycle state in the RLark core rather than delegating security boundaries to extensions.
- Integrate extensions as independently deployed services through versioned gRPC or HTTP APIs instead of loading code into RLark processes.
- Define focused extension contracts for each domain instead of one unrestricted plugin API.
- Support capability discovery so the control plane and API consumers do not assume every data plane is Kubernetes or supports the same operations.
- Preserve declarative, idempotent reconciliation so operations can recover from retries, service restarts, and network failures.

## Phase 1: Runtime and Workload Extensibility

### Data Plane Runtimes

The data plane currently supports Kubernetes only. Introduce a versioned Runtime Provider contract and add:

- **Docker**: run and manage tasks directly with Docker for lightweight environments where Kubernetes is not suitable.
- **Raw**: run and manage tasks directly on hosts or edge devices without a container runtime or orchestrator.
- **Capability discovery**: advertise supported workload types, logs, exec, networking, storage, devices, and other runtime operations so unsupported features can be rejected before scheduling.

The existing Kubernetes implementation should become the first implementation of the same runtime contract used by Docker and Raw providers.

### Custom Workload Resources

The Kubernetes data plane currently supports three built-in workload resource types: Deployment, StatefulSet, and DaemonSet.

Introduce a service-based Workload Adapter contract for additional Kubernetes resources and external workload systems. The contract should cover the complete lifecycle, including validation, rendering or creation, status observation, updates, and deletion. It should support both:

- **Declarative translation**: convert an RLark Task into resources such as RayJob, PyTorchJob, or a custom Kubernetes resource, while the Agent retains responsibility for applying and observing them.
- **Delegated execution**: submit and reconcile work through an external service such as Slurm, a cloud training service, or a domain-specific robotics platform.

The exact API remains under design. Requests must be versioned and idempotent, and generated resources must pass RLark authorization and policy checks before execution.

### Device Capability Foundation

RLark should elevate embodied devices from runtime implementation details into reusable platform primitives. This provides a common foundation for discovering, observing, allocating, and controlling cameras, robot arms, sensors, and accelerators without hard-coding every device type into RLark core.

- **Device model and inventory**: stable identities, ownership and scope, location and topology, device class, vendor metadata, structured capabilities, endpoints, and lifecycle state.
- **Discovery and synchronization**: service-based Device Providers discover physical or virtual devices and continuously reconcile their health, availability, capabilities, and connection state.
- **Observation**: uniform APIs and events for status, telemetry, logs, alarms, and capability changes.
- **Controlled operations**: capability-described commands such as opening a camera, capturing a frame, switching a robot mode, or resetting a device, with validation, timeout, idempotency, and result reporting.
- **Data channels**: authorized access to real-time streams and large device data without forcing video frames or telemetry through the control-plane resource store.
- **Allocation and leases**: exclusive or shared reservations, expiration, renewal, release, and conflict handling so interactive control and scheduled Tasks cannot use a device unsafely at the same time.
- **Safety and authorization**: operation-level permissions, policy checks, emergency-stop and other safety boundaries where supported, complete auditing, and explicit distinction between read-only observation and physical control.

The existing embodied-runtime robot and camera APIs can serve as initial use cases, while the platform contract should remain extensible to new device classes and vendor-specific operations. Generic metadata and capability schemas should be combined with typed, versioned operation interfaces where safety or interoperability requires stronger semantics.

## Phase 2: Identity, Authorization, and Resource Scopes

Improve account management and provide reusable security primitives without prescribing a single tenancy model:

- Users, service accounts, machine identities, roles, and permissions.
- Explicit ownership and scope for Domains, Nodes, Jobs, Workflows, Tasks, runtimes, credentials, policies, and quotas.
- Delegated, short-lived, renewable, and revocable credentials for Agents, extension services, and external controllers.
- End-to-end propagation of verified identity and scope context across the Gateway, controllers, database, Server, Agents, and extension services.
- Scope-aware authorization for API access, list and watch operations, logs, exec, proxying, events, and secrets.
- Audited privileged access and integration with external identity providers such as OIDC where appropriate.

These primitives should allow operators to model users, projects, workspaces, organizations, or other security boundaries without requiring RLark to prescribe a single tenancy workflow. Core authorization and resource isolation must still be enforced by RLark rather than delegated entirely to schedulers or policy extensions.

## Phase 3: Service-Based Extension Framework

Provide a common operational framework for independently deployed extension services while keeping separate contracts for different extension domains.

Initial extension types:

- **Runtime Provider**: execute Tasks on Kubernetes, Docker, Raw hosts, or other runtimes.
- **Workload Adapter**: translate or delegate custom workload lifecycles.
- **Device Provider**: discover devices, publish structured capabilities and status, and expose authorized observation and control operations.
- **Admission and Policy Service**: validate or mutate requests according to deployment-specific policy.
- **Event Consumer**: integrate approvals, notifications, metering, auditing, and automation through webhooks or event streams.

The framework should provide:

- Declarative extension registration, health checks, API version negotiation, and capability discovery.
- mTLS service identity, least-privilege authorization, request auditing, and secret-safe configuration.
- Deadlines, retries, idempotency keys, circuit breaking, and reconciliation after lost events or partial failures.
- `ExtensionDefinition` and scoped `ExtensionBinding` resources, separating installation and trust decisions from scope-specific configuration.
- Protocol specifications, SDKs as convenience libraries, conformance tests, and reference implementations.

Initially, platform administrators should approve extension endpoints. Allowing regular users to register arbitrary endpoints requires additional controls against data exfiltration, credential misuse, and server-side request forgery.

## Phase 4: Open Integration and Customization

Provide stable, composable interfaces so operators and developers can add domain capabilities without modifying RLark core:

- **Extension resources and external controllers**: define resources such as `TrainingPipeline` or `RobotMission`, then reconcile them into RLark Jobs and Tasks through stable APIs.
- **Stable public APIs and events**: prevent external systems and extensions from depending on internal database schemas or implementation details.
- **Device APIs and data-plane access**: support integrations such as device inventories, status views, remote operations, and live camera access through stable discovery, control, event, and streaming contracts.
- **Schema-driven UI contributions**: generate forms and resource views from schemas before considering arbitrary frontend code or micro-frontends.
- **Quotas, policies, and metering events**: provide foundations for approvals, resource governance, usage accounting, and external system integration.

RLark should expose composable primitives rather than hard-code a particular application workflow. Deployment-specific onboarding, user interfaces, approvals, and domain resources can be implemented outside RLark core.

## Future Extension Areas

After the foundational contracts are proven, evaluate focused service interfaces for:

- Scheduler filtering and scoring based on topology, locality, latency, cost, quota, or domain-specific constraints.
- Dataset, model, artifact, object storage, and secret providers.
- Logs, metrics, traces, and alert integrations based on open observability standards.

These interfaces should be introduced from concrete use cases rather than through a single generic plugin abstraction.
