# Core Concepts

## 1. Resource Hierarchy

RLark uses a multi-layer resource abstraction, from underlying infrastructure to top-level embodied AI workloads:

```
  │
  └── Job  ──── Training Job (a complete embodied AI task)
        │
        └── Task  ──── Task Unit (Actor/Rollout/Env roles)
              │
              └── K8s Workload / Docker Container  ──── Underlying runtime
                    │
                    └── Pod / Container  ──── Actual running workload
```

**Embodied AI Scenario**: A typical pipeline involves GPU clusters training a policy model, edge devices (robot arms, sensors) executing the policy in physical environments, and data flowing back to training — all orchestrated through the same platform.

## 2. Domain

Domain is the fundamental unit of **virtual network grouping and addressing** in RLark.

### Concept

A Domain represents a logical training network. Pods within the same Domain can communicate via virtual IPs, and cross-cluster forwarding checks Domain membership. A Domain does not isolate the underlying clusters, nodes, runtimes, storage, or ordinary Kubernetes networking; use the infrastructure's network policies and security controls when isolation is required.

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Domain
metadata:
  name: ppo-experiment-1
spec:
  cidr: "10.0.1.0/24"    # IP subnet for this Domain
status:
  ipAllocations:          # List of allocated IPs
    - ip: "10.0.1.1"
      task: "actor-head"
      job: "ppo-cartpole-v1"
```

### Design Intent

- **Virtual Network Grouping**: Workloads assigned to a Domain share its virtual address space and cross-cluster forwarding scope; this is not infrastructure-level isolation
- **IP Management**: The Domain Controller in Controller-Manager allocates IP subnets and assigns unique IPs to each workload
- **Certificate Granularity**: Each Domain has an independent X.509 certificate for cross-cluster communication authentication

## 3. DomainPeer

DomainPeer is the **view of a Domain within a specific data plane cluster**, automatically created by the Domain Controller.

### Concept

Each Domain has one corresponding DomainPeer object in each data plane cluster, containing the set of all Pods in that Domain within that cluster.

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: DomainPeer
metadata:
  name: ppo-experiment-1
  namespace: agent-beijing    # This Agent's workspace
spec:
  cert: "-----BEGIN CERTIFICATE-----..."  # Domain certificate
  key: "-----BEGIN PRIVATE KEY-----..."   # Domain private key
  prefixLen: 24
  pods:
    - name: "actor-head-0"
      namespace: "default"
      uid: "abc-123"
      ip: "10.0.1.1"           # Domain virtual IP
      localIP: "172.17.0.5"    # Actual Pod IP (within k8s cluster)
      node: "node-01"
      globalNamespace: "agent-beijing"
```

### Usage

- **Routing Table**: NodeServer uses DomainPeer to locate target Pod's cluster and actual IP
- **Certificate Distribution**: DomainPeer carries the Domain's certificate and private key; Agent uses it to establish cross-cluster SSH tunnels
- **Auto-Update**: Domain Controller automatically updates DomainPeer's pods list on Pod create/delete

## 4. Node

Node represents a **physical/virtual compute node** in a data plane cluster.

### Concept

Node is automatically reported by the Agent's Push controller, reflecting the node's actual state (addresses, resource capacity, GPU count, etc.).

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Node
metadata:
  name: gpu-node-01
  namespace: agent-beijing
  labels:
    nvidia.com/gpu: "true"
spec:
  agentType: Kubernetes
  unschedulable: false
status:
  phase: Online
  addresses:
    - type: InternalIP
      address: "10.0.1.23"
    - type: Hostname
      address: "gpu-node-01"
  capacity:
    cpu: "32"
    memory: "128Gi"
    nvidia.com/gpu: "8"
  allocatable:
    cpu: "30"
    memory: "120Gi"
    nvidia.com/gpu: "8"
  nodeInfo:
    architecture: amd64
    operatingSystem: linux
    kernelVersion: "5.15.0-91-generic"
    agentVersion: "0.1.0"
```

### Usage

- **Resource Awareness**: Control plane makes scheduling decisions based on Node resource state (via NodeSelector)
- **Label Management**: Control plane can push labels to Nodes; Agent's Pull controller syncs them to local k8s Nodes
- **Taint Management**: Control node schedulability via the `unschedulable` field

### Administrator Metadata

Administrators can select multiple Nodes in the Admin UI and apply metadata used by both the admin and workspace resource views:

- `rlark.io/city` annotation: administrator-managed physical location
- `rlark.io/node-category-{cloud,edge,robot}=true` labels: one or more Node categories
- `rlark.io/gpu-model` annotation: GPU model for cloud compute Nodes
- `rlark.io/device-model` annotation: embodied device model for edge compute and robot Nodes

These business fields are stored on the KCP Node CR and are preserved when the Agent refreshes Kubernetes-discovered state. The batch editor shows common current values, marks mixed selections, and changes only properties explicitly enabled by the administrator; clearing an enabled property removes it. The Admin UI also supports selecting all filtered Nodes, clearing the selection, and batch cordon/uncordon. A Node may belong to multiple categories and carry both GPU and embodied-device models.

Workspace node totals and cluster-detail node lists include usable Workers carrying an RLark category label. Legacy `rlark.io/node-category` values and unlabeled Nodes that explicitly advertise GPU or embodied-device resources remain supported. Nodes carrying a Kubernetes `master` or `control-plane` role label remain visible only in the administration workspace and are not counted or shown as workload Workers in the business workspace.

CPU, memory, and GPU usage in Node details is aggregated from the Kubernetes `resources.requests` of running Workers. It represents scheduler-reserved resources, not real-time hardware utilization from metrics-server. Disk is reported separately from kubelet Stats Summary: the Agent combines nodefs with a dedicated imagefs, while avoiding double-counting when both refer to the same filesystem. The UI shows real used, total, and available storage and warns at 90% usage or when kubelet reports `DiskPressure=True`. The detail page also lists every Worker on the Node with its Job, role, IP, resource requests, and runtime phase.

## 5. Node Cordon/Uncordon

Node scheduling can be controlled via the `unschedulable` field.

### Concept

Setting `spec.unschedulable: true` marks a node as unschedulable, preventing new workloads from being assigned to it. This is equivalent to Kubernetes `kubectl cordon`.

### Usage

```bash
# PATCH to toggle unschedulable
curl -X PATCH "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes/gpu-node-01?namespace=default" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{ "spec": { "unschedulable": true } }'
```

The Web UI provides a Cordon/Uncordon button for each node in the node list.

## 6. Job

Job is the **core resource directly operated by users**, representing a complete RL training task.

### Concept

A Job contains multiple Task templates, each representing a role in training (e.g., Actor, Rollout, Env). The Controller-Manager's Job Controller creates and manages Tasks based on templates.

Before dispatching any Task, the Job Controller validates every template as one desired unit. Template names must be non-empty and unique, and `runtime.kind`, workload kind/version, and carrier type/version must be non-empty open identifiers. For the current Job-to-Ray-on-Kubernetes path, the controller additionally requires a Pod template containing exactly one regular container named `main`. This is an implementation capability check, not a universal Task CRD constraint: the CRD permits omitting `workload.template`, and other Workload Adapter/Runtime combinations define their own requirements. If any template is invalid, the controller creates none of them and reports `Validated=False` on the Job with the failing field path. It does not silently skip individual templates. The Agent independently validates the selected implementation before Runtime effects. The Job Controller does not enforce Ray head count or head replica topology.

The Web UI keeps its existing Job and Workflow creation, editing, cloning, lifecycle, storage, and Worker interactions, but serializes them directly with the generated CRD contracts. New requests use `runtime`, `workload`, `carrier`, `placement`, and `environments`; lifecycle actions write `spec.suspended`; and storage selections become Kubernetes-native Pod template volumes. The UI does not emit the removed `agentType`, nested `kubernetes.workload`, `stopped`, `pvcStorageMap`, or `pvcSizeGbMap` forms.

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Job
metadata:
  name: ppo-cartpole-v1
  labels:
    framework: ppo
    env: cartpole
spec:
  domain: ppo-experiment-1    # Virtual network joined by generated Tasks
  tasks:
    - name: actor-head
      head: true               # Whether this is the head node
      role: Actor              # Role: Actor/Rollout/Env
      placement:
        nodeSelector:
          nvidia.com/gpu: "true"
      runtime:
        kind: Kubernetes
      workload:
        kind: rlinf.io/ray
        version: v1
        parameters:
          role: head
          prepare:
            inline: pip install -r requirements.txt
          run:
            inline: python main.py --role=head
        carrier:
          type: rlinf.io/kubernetes-deployment
          version: v1
          parameters:
            replicas: 1
        template:
          spec:
            containers:
              - name: main
                image: pytorch/pytorch:2.3.0
                resources:
                  limits:
                    nvidia.com/gpu: "1"
    - name: rollout
      role: Rollout
      runtime:
        kind: Kubernetes
      workload:
        kind: rlinf.io/ray
        version: v1
        parameters:
          role: worker
          headTaskName: ppo-cartpole-v1-actor-head
        carrier:
          type: rlinf.io/kubernetes-statefulset
          version: v1
          parameters:
            replicas: 2
        template: ...
status:
  phase: Running               # Pending/Running/Succeeded/Failed
  startTime: "2026-06-25T03:20:00Z"
  tasks:
    - name: actor-head
      phase: Running
    - name: rollout
      phase: Running
```

### State Machine

```
(empty) ──init──▶ Pending ──tasks-running──▶ Running
                     │             │              │
                     │ stop        │ stop         │ all-tasks-succeeded
                     ▼             ▼              ▼
                  Stopping ──cleanup-complete──▶ Stopped    Succeeded
                     ▲
                     │ failed-run stop/restart cleanup
                   Failed
```

`Stopping` is transitional: RLark deletes the child Tasks and waits for their Workers to be cleaned up. Referenced external PVCs are not managed by RLark. Pod-scoped PVCs created from `ephemeral.volumeClaimTemplate` remain governed by Kubernetes and are deleted with their Pods. Stopping and then starting a terminal Job creates new Tasks and Workers, but it does not by itself guarantee empty persistent storage.

### Relationship with Task

After Job Controller reconciliation:
- Each Task template generates a Task CR named `<job-name>-<task-name>`
- Tasks have the label `rlinf.io/job=<job-name>` for querying
- Tasks set OwnerReference to Job; deleting a Job cascades to delete Tasks

## 7. Job Stop/Start

Jobs can be suspended and restarted via the `suspended` field in the Job spec, providing manual lifecycle control over running workloads.

### Concept

Setting `spec.suspended: true` starts an orderly stop without deleting the Job resource. The Job is shown as `Stopping` while RLark waits for the Job and all child Tasks and Workers to stop, and then becomes `Stopped`. After `ArchiveTTL` (default 5 minutes), the Archive Controller deletes the Job CR from kcp (archive), while the persistence backend (PostgreSQL or Kubernetes Secret, test mode) retains the complete Job configuration and final status. Starting the Job later restores it from the persistence backend with the same name and spec.

### How it works

1. **Stop**: RLark deletes the child Task CRs, waits for their underlying workloads to be cleaned up, and retains the Job CR and configuration. Referenced external PVCs remain outside RLark management. Pod-scoped ephemeral PVCs are deleted with their Pods.
2. **Archive**: After `ArchiveTTL`, the Archive Controller deletes the Job CR from kcp. The persistence backend retains the complete Job record (spec + status). Deleting the Job CR frees etcd and controller memory; business records remain queryable and restorable.
3. **Start**: `POST /jobs/:name/start` is the unified start endpoint. If the Job is active (KCP has the CR), it patches `spec.suspended=false` directly. If archived (KCP CR was removed after stop), it restores the Job from the persistence backend with the same name and spec, and sets `spec.suspended=false`. The new CR has a new Kubernetes UID but the same business identity (name).
4. **Restart**: The Job controller treats the restart annotation as a Job-level operation identifier. It deletes the current Task objects, waits for each Agent-owned cleanup finalizer to complete Runtime cleanup, and then creates new Task objects with new Kubernetes UIDs from the templates. The Agent does not interpret the restart annotation itself. Restart leaves referenced external PVCs untouched but replaces Pod-scoped ephemeral PVCs. `Succeeded` and `Failed` Jobs both start a new run from their templates.
5. **Delete**: Deleting a Job first performs the stop-and-wait sequence, then removes the Job and child Tasks. Referenced external PVCs and hostPath data are not deleted by RLark; Pod-scoped ephemeral PVCs are deleted with their Pods. The persistence backend record is also deleted (PostgreSQL row or Secret).

### Key Features

- **External persistent storage**: Stop, restart, and delete do not manage PVCs referenced through `persistentVolumeClaim.claimName`; administrators own their lifecycle.
- **Existing claim references**: A new Task can explicitly reference the same existing PVC and data. Pod-scoped `ephemeral.volumeClaimTemplate` claims are different: Kubernetes creates and deletes them with each Pod.
- **Unified start endpoint**: `POST /jobs/:name/start` works for both active and archived Jobs. The frontend calls this endpoint without knowing whether the Job is in kcp or archived.
- **Web UI integration**: The Web UI provides Stop, Start, Restart, and Delete actions with lifecycle-aware availability

When a Workflow is stopped, RLark deletes all Jobs from the current run, including completed Jobs. Resuming the Workflow creates a new run from the beginning of the DAG; completion state from the previous run is not reused.

## 8. Task

Task is the **execution unit** of a Job, automatically created by the Job Controller. **Users should not create Tasks directly**.

### Concept

Each Task represents a concrete "training role instance". Agent's Pull controller watches for Tasks and creates corresponding K8s workloads.

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Task
metadata:
  name: ppo-cartpole-v1-actor-head
  namespace: agent-beijing
  labels:
    rlinf.io/job: ppo-cartpole-v1
spec:
  role: Actor
  placement:
    nodeSelector:
      nvidia.com/gpu: "true"
  runtime:
    kind: Kubernetes
  workload:
    kind: rlinf.io/ray
    version: v1
    parameters:
      role: head
    carrier:
      type: rlinf.io/kubernetes-deployment
      version: v1
      parameters:
        replicas: 1
    template:
      metadata:
        labels:
          app: ppo-trainer
      spec:
        containers:
          - name: trainer
            image: pytorch/pytorch:2.3.0
  environments:
    - type: rlinf.io/network
      version: v1
      parameters:
        domain: ppo-experiment-1
status:
  phase: Running
  observedNodes: ["gpu-node-01"]
  startTime: "2026-06-25T03:20:30Z"
  conditions:
    - type: Ready
      status: "True"
      reason: PodRunning
```

### Task Roles

| Role | Description | Typical Use |
|------|-------------|-------------|
| `Actor` | Execute policy inference, generate training data | PPO Actor process |
| `Rollout` | Environment interaction, collect trajectories | Game environment simulator |
| `Env` | Environment/auxiliary service | Data preprocessing, parameter server |

### Runtime Types

| Type | Description | Use Case |
|------|-------------|----------|
| `Kubernetes` | Run RLark/Ray Tasks using Deployment, DaemonSet, or StatefulSet as the Kubernetes carrier | GPU clusters for large-scale training |
| `Docker` | Planned provider that lowers the common Pod template to Docker containers; not implemented or advertised | Edge devices (robot arm, sensor, camera) |
| `Raw` | Planned provider that lowers the common Pod template to host processes; not implemented or advertised | Bare metal servers, embedded devices |

`runtime.kind`, `workload.kind`, and `workload.carrier.type` are open identifiers rather than CRD enums. Openness permits provider extension without replacing the Task schema; it does not imply support. The Agent must resolve a registered Provider and Adapter and validate capabilities before any external effect. Workload semantics use `kind`, `version`, and `parameters`; the carrier independently uses `type`, `version`, and `parameters`. `environments` is an ordered list and may contain the same environment type more than once with different parameters.

**Embodied AI Mapping**:

```mermaid
graph LR
    subgraph Cloud["Cloud (k8s)"]
        Training["Training (GPU)"]
        Rollout["Rollout (Env)"]
    end
    subgraph Edge["Edge (k8s / Docker / Raw)"]
        Robot["Robot Arm (Inference)"]
        Camera["Camera (Sensor)"]
    end
    Training -->|"grad"| Robot
    Training <-->|"data"| Robot
    Rollout -->|"ctrl"| Camera
    Rollout <-->|"obs"| Camera
```

## 9. Pod

Pod CR is the **control plane mirror** of data plane Pods, reported by the Agent's Push controller.

### Concept

When a Pod is created in the data plane cluster, the Agent's Pod Push controller reports the Pod info to the control plane, creating a corresponding Pod CR. Pod CR contains Pod identification info (name, owning Task) and runtime status (IP, node, phase).

### Usage

- **Status Tracking**: Control plane understands underlying Pod real-time status via Pod CR
- **SSH Lookup**: Server's PodCache quickly locates Pod's Agent based on Pod CR
- **Log Queries**: Gateway finds Pod's Agent via Pod CR and forwards log requests

## 9. Resource Relationship Summary

```mermaid
graph TD
    job -->|"1:N"| task["Task<br/>(Namespaced: agent-{id})<br/>Exec Unit"]
    task -->|"1:1 (K8s workload)"| workload["Deployment /<br/>DaemonSet /<br/>StatefulSet<br/>(Local k8s cluster)"]
    workload -->|"1:N"| pod["Pod + Sidecar<br/>Agent Push reports → Pod CR"]
    domain["Domain<br/>(Cluster scoped)<br/>Network Isolation"] -->|"1:N (one per cluster)"| dp["DomainPeer<br/>(Namespaced: agent-{id})<br/>Pod Routing Table"]
    node["Node<br/>(Namespaced)<br/>Compute Node"]
```

## 9. Naming Conventions

| Namespace Prefix | Meaning | Example |
|-----------------|---------|---------|
| `agent-` | Agent workspace | `agent-beijing-01` |
| `rlark-system` | Default namespace created by Agent | `rlark-system` |
| Label `rlinf.io/job` | Pod/Task's owning Job | `rlinf.io/job=ppo-cartpole-v1` |
| Annotation `rlinf.io/ray-role` | Ray cluster role | `head` / `worker` |

## 9. Ray Cluster Integration

Ray configuration is carried by `spec.workload.parameters` for the `rlinf.io/ray` workload at version `v1`. A worker uses `parameters.headTaskName` to reference the concrete head Task in the same namespace. The Job controller also writes the derived `parameters.totalNodes` and `parameters.nodeRankStart` values used by the launcher. The implementation must verify the trusted Job relationship rather than trusting a matching name alone. Existing Ray annotations remain implementation compatibility metadata and must not be used as the canonical new Task declaration:

```yaml
annotations:
  rlinf.io/ray-role: "head"          # head | worker
  rlinf.io/ray-total-nodes: "5"     # head only
  rlinf.io/ray-head-task-name: "actor-head"  # worker only
```

**Process assembly and startup**:

1. Agent's Pull controller resolves `rlinf.io/ray/v1` and strictly decodes `role`, `headTaskName`, `prepare.inline`, and `run.inline`
2. The Workload Adapter declares only `ray-launcher` as the required primary process, while the SSH Environment Handler declares `sshd` as an optional auxiliary process. Tools installation and supervision are mandatory Runtime facilities and require no Task or Adapter declaration. If a workload unit has no declared primary, the assembler supplies `/rlark-tools/rlark-tools idle`. An SSH failure does not stop the Task: an initial start failure leaves SSH unavailable, while a non-zero exit receives bounded restart attempts; a failed restart or exhausted attempts also leave SSH unavailable. Network and TensorBoard remain independently managed Environment features.
3. The Binder's Runtime workload assembler installs `rlark-tools`, writes a Task-incarnation-owned supervisor ConfigMap, and changes the main container entrypoint to `rlark-tools supervisor`. The supervisor runs `ray-launcher` and `sshd` as separate OS child processes; RLark-owned Ray orchestration no longer uses mounted shell scripts.
4. The Adapter consumes the canonical workload parameters and injects the launcher environment, including total-node and rank metadata plus the Head Pod IP. In particular, a worker's `parameters.headTaskName` becomes the same-namespace Head Service DNS name without networking, or `<headTaskName>-0.rlark-domain` when a Network Environment is declared. In the latter case the Adapter also enables the launcher's built-in network barrier: before the prepare script, Head waits for its own `.rlark-domain` name and Worker waits for the Head virtual-domain name to resolve. Cancellation stops the wait. This Go implementation replaces the inactive external `WAIT_NETWORK_SCRIPT` compatibility hook. Compatibility annotations are not the source of these values.
5. Head node creates a Task-incarnation-owned Service (exposing 6379/8265/8080). Drift in RLark-owned Service fields uses delete-and-recreate replacement, which can briefly interrupt access and change API-server-assigned addresses or ports; Kubernetes defaults and allocations alone do not trigger replacement.
6. `ray-launcher` verifies that the configured Ray CLI is executable before issuing Ray commands. Worker nodes then resolve and wait for Head before joining the cluster. The head waits for the declared node count and runs `ray status`; the cluster readiness check remains non-fatal for compatibility, while a failed `ray status` is fatal.

Supervisor configuration changes update a Pod-template revision annotation and therefore roll out the workload. SSH public-key rotation updates only the separately projected Secret after the initial migration rollout; raw key bytes are not stored in the supervisor ConfigMap.

Shutdown is primary-first. On Pod termination, the supervisor asks `ray-launcher` to stop and waits for its actual exit without starting its own timeout; Kubernetes' Pod termination grace remains the final bound if the primary hangs. Only after the primary exits does the supervisor terminate `sshd` and other declared auxiliaries, allowing them up to 15 seconds before force-killing any remainder.

Ray's platform ports remain fixed compatibility contracts: cluster 6379, dashboard 8265, metrics 8080, head SSH 22, and worker SSH 2222. Task/Job configuration controls roles, replicas (from which total nodes and rank ranges are derived), the head Task, domain, `prepare.inline`, and `run.inline`. A workload image may continue to set `RAY_TEMP_DIR` and the existing `RAY_health_check_*` / `RAY_num_heartbeats_timeout` environment variables. Retry counts and ports are not currently Task-level settings. The standalone `ray-launcher` command also accepts Ray, Bash, and Python binary flags, but the built-in Adapter uses their defaults.

## 14. Object Storage & PVCs

RLark mounts remote storage to training tasks with Kubernetes generic ephemeral volumes.

### Concept

Each Pod gets a PVC created from its volume's `ephemeral.volumeClaimTemplate`. Kubernetes owns this PVC and removes it with the Pod.

### Configuration

```yaml
workload:
  template:
    spec:
      volumes:
        - name: data
          ephemeral:
            volumeClaimTemplate:
              spec:
                accessModes: [ReadWriteOnce]
                storageClassName: ceph-rbd
                resources:
                  requests:
                    storage: 10Gi
```

### How it works

1. The frontend retrieves RLark-managed StorageClass declarations through `GET /api/v1/storage/storageclass?clusters=<agent-id>`; this filters configured target clusters and does not query the Agent.
2. The selected class and requested size are written into `volumeClaimTemplate`.
3. After the declaration is delivered, Kubernetes creates one PVC for each Pod and binds it through the selected StorageClass.
4. Kubernetes removes the PVC when its Pod is deleted.

A declaration in the list represents configured distribution intent. PVC provisioning also requires successful asynchronous delivery and a working rclone CSI driver in the target cluster.

`pvcStorageMap` and `pvcSizeGbMap` are not part of the canonical Task schema. Use `ephemeral.volumeClaimTemplate`; upgrading deployments may require old Tasks or Jobs to be deleted and recreated.

## 15. User Authentication

RLark authenticates Web UI and API requests with short-lived JWT access tokens. The verified `admin` and `user` roles enforce coarse-grained API authorization: platform operations are available to both roles, while control-plane configuration and credential management require `admin`.

### Authentication Flow

1. During deployment, `rlarkadm` generates random passwords and a JWT signing key and stores them in a KCP Secret (`rlark-ui-auth`)
2. Web UI sends `POST /api/v1/auth/login` with username and password
3. Gateway validates against the Secret and returns an HS256 JWT containing the subject, role, issue time, and expiration time
4. The frontend stores the token in `sessionStorage` and sends it as `Authorization: Bearer <token>` on API requests; the Gateway verifies it before dispatching protected routes

`user` can manage Jobs, Workflows, Tasks, Pods, terminal sessions, SSH keys, and storage objects, and read cluster, node, Domain, image, StorageClass, and system configuration data. Node and Domain mutations, certificates, image registries, system configuration updates, StorageClass management, and Addon management require `admin`. This is role-level authorization; per-user resource ownership, including SSH key ownership, is not yet enforced.

## 16. Addon (Component Management)

Addon is RLark's component management system, allowing users to install, configure, and manage third-party components (device plugins, monitoring agents, etc.) across multiple data plane clusters.

### Concept

Addons are managed through three layers:

1. **Addon Catalog** (`/api/v1/addons`) — A curated list of available addons (e.g., `embodied-runtime-device-plugin`). Each addon includes Kubernetes manifests (DaemonSet, ConfigMap, RBAC) and configurable values.
2. **Addon CRD** (`rlinf.io/v1alpha1/Addon`) — A Kubernetes-style CR representing an installed addon instance in a specific data plane cluster. The Spec specifies the addon name, version, and configuration values.
3. **Addon Controller** (`../pkg/agent/controllers/addon/pull.go`) — The Agent's Pull controller watches Addon CRs, renders the addon manifests with the configured values, and applies them to the local Kubernetes cluster.

### Addon Lifecycle

```
Catalog Item ──install──▶ Addon CR ──pull──▶ Agent applies manifests
                            │
                            ├── UPDATE values ──▶ Agent re-applies
                            └── DELETE ──▶ Agent removes manifests
```

### Addon Phase

```
Pending ──▶ Installing ──▶ Ready
                │
                └──▶ Failed
                     │
                     └──▶ Upgrading ──▶ Ready
```

### Example: Embodied Runtime Device Plugin

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Addon
metadata:
  name: embodied-device-plugin
  namespace: agent-beijing
spec:
  addonName: embodied-runtime-device-plugin
  version: "0.1.0"
  values:
    image: "rlark/embodied-device-plugin:0.1.0"
    nodeSelector: "nvidia.com/gpu=true"
status:
  phase: Ready
  version: "0.1.0"
```

### Key Properties

- **Multi-cluster**: Each data plane cluster can have different addons installed
- **Configurable**: Addon values can be customized per cluster via `spec.values`
- **Versioned**: Addons support version upgrades via `spec.version`
- **Auto-apply**: Agent's Pull controller automatically applies manifests when Addon CR is created or updated
- **Mutating Webhook**: The Device Plugin includes a mutating admission webhook that auto-injects a `devinit` init container into Pods requesting `rlinf.io/device`, creating macvlans in the Pod's network namespace without manual configuration

## 17. Web Terminal

Web Terminal provides interactive Pod terminal access directly from the Web UI.

### Concept

The Web Terminal allows users to open a terminal session into any Pod managed by RLark, without needing to SSH into the underlying node or install kubectl locally.

### Architecture

```
Browser ──WebSocket──▶ Gateway ──proxy via Server──▶ Agent ──exec──▶ Pod
```

### How it works

1. User clicks "Terminal" on a Pod in the Web UI, opening the session in a new browser tab. Multiple Pods can therefore have active terminal tabs at the same time.
2. The new tab establishes a WebSocket connection to Gateway at `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal`
3. Gateway proxies the WebSocket connection through Server to the Pod's Agent
4. Agent opens an exec session into the Pod's container and streams I/O
5. The terminal session persists until the WebSocket is closed

The Web Terminal supports interactive full-screen programs such as Vim on macOS Safari. Printable keys and terminal control keys are forwarded directly while browser shortcuts and IME composition remain handled by the browser.
When the shell exits (for example, after `exit`), the proxy chain forwards a normal WebSocket close frame. The UI reports the process exit code when it is non-zero, while transport failures remain distinguishable from process exit.

## 18. Pod HTTP Proxy

Pod HTTP Proxy allows direct HTTP access to Pods managed by RLark through the Server → Agent proxy chain.

### Concept

The Pod HTTP Proxy enables users to send HTTP requests to specific Pods without needing to know the Pod's real IP address or establish SSH tunnels. The Server resolves the Pod's location via the pod cache and proxies the request through the Agent's local HTTP server.

### Architecture

```
Client ──HTTP──▶ Gateway ──proxy via Server──▶ Agent ──reverse proxy──▶ Pod (http://<podIP>:<port>/<path>)
```

### How it works

1. Client sends `ANY /api/podproxy/{podName}:{port}/*path` or `ANY /api/taskproxy/{taskName}:{port}/*path` to Gateway
2. Gateway forwards the request to Server
3. Server looks up the Pod in the pod cache (by Pod name or Task name) to get the Pod's real IP and Agent ID
4. Server constructs a reverse proxy to the target Agent, targeting `0.0.0.0:1` (Agent's local server)
5. Agent receives the request at `/api/proxy/http://<podIP>:<port>/<path>` and reverse-proxies to the Pod
6. The response flows back through the same chain

### Key Features

- **No direct Pod IP needed**: Users only need the Pod name or Task name and port
- **Dual resolution**: Proxy by Pod name (`/api/podproxy/`) or Task name (`/api/taskproxy/`)
- **Certificate-based access control**: Access is gated by the client certificate's permissions
- **Transparent proxy**: All HTTP methods (GET, POST, PUT, DELETE, etc.) are supported

## 19. TensorBoard Proxy

TensorBoard Proxy provides a web-based visualization dashboard for training metrics (loss curves, scalar summaries, histograms, etc.) directly from the RLark Web UI, without needing to expose TensorBoard ports externally.

### Concept

When a training Task runs TensorBoard (listening on port 6006), RLark automatically proxies the TensorBoard UI through the Gateway. Users can open TensorBoard from the Web UI by clicking a link, and the browser accesses it via the Gateway proxy.

### Architecture

```
Browser ──HTTP──▶ Gateway ──proxy to Server──▶ Server ──podproxy──▶ Agent ──▶ Pod:6006 (TensorBoard)
```

### How it works

1. The `rlinf.io/tensorboard/v1` Environment Handler declares TensorBoard as a restartable Kubernetes sidecar when `parameters.logDir` is non-empty; it is independent of the Ray supervisor processes
2. Gateway injects a `tensorBoardProxy` field into Task status responses, pointing to the proxy URL
3. User clicks the TensorBoard link in the Web UI
4. Browser sends a request to `GET /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/`
5. Gateway resolves the Task's Pod via the KCP API and proxies the request to the Server's Pod Proxy
6. Gateway rewrites HTML/CSS responses to ensure all asset paths (fonts, JS, API calls) work through the proxy prefix
7. TensorBoard UI renders in the browser as if it were accessed directly

### Key Features

- **No port exposure**: TensorBoard port 6006 stays internal to the Pod
- **Automatic proxy injection**: Task listing/GET responses include the `tensorBoardProxy` URL
- **HTML rewriting**: Proxied TensorBoard pages are rewritten so all relative and absolute paths work correctly
- **Explicit integration**: TensorBoard starts when the Task requests the Environment below; it is not managed by `rlark-tools supervisor`

```yaml
environments:
  - type: rlinf.io/tensorboard
    version: v1
    parameters:
      logDir: /data/tensorboard
```

## 20. SSH Key Management

SSH Key Management allows users to upload SSH public keys through the API or Web UI. A registered key authenticates its named user to the RLark SSH bastion. Workload SSH is declared by `rlinf.io/ssh/v1`; its `parameters.authorizedKeys` array is Task-scoped desired-state material.

### Concept

Keys are stored by username in a Kubernetes Secret (`rlark-ssh-user-keys`) in the control plane namespace. The SSH server verifies that the presented key is registered for the SSH username. The current implementation does **not** authorize that user for particular Pods; after bastion authentication, there is no per-user or per-Pod policy check. Selecting a key while creating a Job is separate from bastion authorization and places that public key in the generated workload configuration.

### API

- `GET /api/v1/ssh-user-keys` — list all SSH keys (optionally filtered by user)
- `POST /api/v1/ssh-user-keys` — add a new SSH public key for a user
- `DELETE /api/v1/ssh-user-keys/:id` — delete a key by index

### Key Features

- **Web UI management**: A dedicated SSH Keys page in the Web UI for viewing and managing keys
- **Explicit workload injection**: a workload requests `rlinf.io/ssh/v1` with an `authorizedKeys` array. Authorized keys are public authentication configuration and may be retained in the Agent's local execution snapshot. They are omitted from portable plans and remote workload transport and must not be copied into status, logs, errors, revisions, or operation keys.
- **Conflict detection**: Duplicate keys are detected and rejected with a 409 response
- **Retry on conflict**: The API automatically retries on write conflicts (up to 5 attempts)
- **Key validation**: Public keys are validated using `golang.org/x/crypto/ssh` before storage
