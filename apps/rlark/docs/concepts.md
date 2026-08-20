# Core Concepts

## 1. Resource Hierarchy

RLark uses a multi-layer resource abstraction, from underlying infrastructure to top-level embodied AI workloads:

```
Workflow  ──── Workflow (DAG orchestration of multiple Jobs)
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

CPU, memory, and GPU usage in Node details is aggregated from the Kubernetes `resources.requests` of running Workers. It represents scheduler-reserved resources, not real-time hardware utilization from metrics-server. The detail page also lists every Worker on the Node with its Job, role, IP, resource requests, and runtime phase.

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
  domain: ppo-experiment-1    # Owning Domain
  tasks:
    - name: actor-head
      head: true               # Whether this is the head node
      role: Actor              # Role: Actor/Rollout/Env
      agentType: Kubernetes    # Agent type: Kubernetes/Docker/Raw
      nodeSelector:            # Node selector
        nvidia.com/gpu: "true"
      kubernetes:
        workload:
          kind: Deployment     # Workload type
          replicas: 1
          template:
            spec:
              containers:
                - name: trainer
                  image: pytorch/pytorch:2.3.0
                  resources:
                    limits:
                      nvidia.com/gpu: "1"
    - name: rollout
      role: Rollout
      agentType: Kubernetes
      kubernetes:
        workload:
          kind: StatefulSet
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
                              │                  │
                              │ any-task-failed  │ all-tasks-succeeded
                              ▼                  ▼
                          Failed            Succeeded
```

### Relationship with Task

After Job Controller reconciliation:
- Each Task template generates a Task CR named `<job-name>-<task-name>`
- Tasks have the label `rlinf.io/job=<job-name>` for querying
- Tasks set OwnerReference to Job; deleting a Job cascades to delete Tasks

## 7. Job Stop/Start

Jobs can be stopped and restarted via the `stopped` field in the Job spec, providing manual lifecycle control over running workloads.

### Concept

Setting `spec.stopped: true` on a Job causes the Job controller to stop all associated workloads (Pods, Deployments, StatefulSets) without deleting the Job resource. Setting it back to `false` (or removing the field) restarts the workloads.

### How it works

1. **Stop**: When `spec.stopped` is set to `true`, the Job controller detects the change and deletes the underlying Kubernetes workloads (Deployments/StatefulSets) while keeping the Job CR.
2. **Restart**: When `spec.stopped` is removed or set to `false`, the Job controller recreates the workloads from the Task templates.
3. **State preservation**: The Job's phase and status fields are preserved during stop/restart cycles.

### Key Features

- **Non-destructive**: Stopping a Job does not delete the Job CR or its Tasks
- **Persistent state**: PVCs and other persistent resources are not affected by stopping
- **Web UI integration**: The Web UI provides one-click Stop/Start buttons in the Job list

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
  agentType: Kubernetes
  domain: ppo-experiment-1
  nodeSelector:
    nvidia.com/gpu: "true"
  kubernetes:
    workload:
      kind: Deployment
      replicas: 1
      template:
        namespace: default
        spec:
          containers:
            - name: trainer
              image: pytorch/pytorch:2.3.0
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

### Agent Types

| Type | Description | Use Case |
|------|-------------|----------|
| `Kubernetes` | Create K8s native workloads (Deployment/DaemonSet/StatefulSet) | GPU clusters for large-scale training |
| `Docker` | Manage containers via Docker API (TODO) | Edge devices (robot arm, sensor, camera) |
| `Raw` | Download artifact and execute binary directly (TODO) | Bare metal servers, embedded devices |

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

## 9. Workflow

Workflow is **DAG orchestration of multiple Jobs**, supporting training pipelines with dependencies.

### Concept

A Workflow contains multiple Job templates, each declaring upstream dependencies via `dependencies`. The Workflow Controller schedules Jobs in topological order: upstream Jobs must succeed before dependent Jobs can start.

### Key Properties

```yaml
apiVersion: rlinf.io/v1alpha1
kind: Workflow
metadata:
  name: training-pipeline-v1
spec:
  jobTemplates:
    - name: prepare-data
      dependencies: []            # No dependencies, start immediately
      spec:
        tasks:
          - name: prep
            role: Env
            agentType: Kubernetes
            kubernetes: ...
    - name: train
      dependencies: ["prepare-data"]  # Start after prepare-data succeeds
      spec:
        tasks:
          - name: actor-head
            head: true
            role: Actor
            agentType: Kubernetes
            kubernetes: ...
    - name: evaluate
      dependencies: ["train"]
      spec:
        tasks: ...
```

### Typical Pipeline

```
Data Preparation ──▶ Model Training ──▶ Model Evaluation
    prepare             train             evaluate
```

### State Machine

Similar to Job, Workflow state is determined by the aggregate of its Jobs' states:
- All Jobs succeed → Workflow Succeeded
- Any Job fails → Workflow Failed

## 10. Pod

Pod CR is the **control plane mirror** of data plane Pods, reported by the Agent's Push controller.

### Concept

When a Pod is created in the data plane cluster, the Agent's Pod Push controller reports the Pod info to the control plane, creating a corresponding Pod CR. Pod CR contains Pod identification info (name, owning Task) and runtime status (IP, node, phase).

### Usage

- **Status Tracking**: Control plane understands underlying Pod real-time status via Pod CR
- **SSH Lookup**: Server's PodCache quickly locates Pod's Agent based on Pod CR
- **Log Queries**: Gateway finds Pod's Agent via Pod CR and forwards log requests

## 11. Resource Relationship Summary

```mermaid
graph TD
    wf["Workflow<br/>(Cluster scoped)<br/>DAG Pipeline"] -->|"1:N"| job["Job<br/>(Cluster scoped)<br/>Training Job"]
    job -->|"1:N"| task["Task<br/>(Namespaced: agent-{id})<br/>Exec Unit"]
    task -->|"1:1 (K8s workload)"| workload["Deployment /<br/>DaemonSet /<br/>StatefulSet<br/>(Local k8s cluster)"]
    workload -->|"1:N"| pod["Pod + Sidecar<br/>Agent Push reports → Pod CR"]
    domain["Domain<br/>(Cluster scoped)<br/>Network Isolation"] -->|"1:N (one per cluster)"| dp["DomainPeer<br/>(Namespaced: agent-{id})<br/>Pod Routing Table"]
    node["Node<br/>(Namespaced)<br/>Compute Node"]
```

## 12. Naming Conventions

| Namespace Prefix | Meaning | Example |
|-----------------|---------|---------|
| `agent-` | Agent workspace | `agent-beijing-01` |
| `rlark-system` | Default namespace created by Agent | `rlark-system` |
| Label `rlinf.io/job` | Pod/Task's owning Job | `rlinf.io/job=ppo-cartpole-v1` |
| Annotation `rlinf.io/ray-role` | Ray cluster role | `head` / `worker` |

## 13. Ray Cluster Integration

RLark supports declarative Ray cluster creation via Task annotations:

```yaml
annotations:
  rlinf.io/ray-role: "head"          # head | worker
  rlinf.io/ray-total-nodes: "5"     # head only
  rlinf.io/ray-head-task-name: "actor-head"  # worker only
```

**Auto-initialization process**:

1. Agent's Pull controller detects Ray annotations
2. Creates ConfigMap mounting init scripts (`ray_head.sh` / `ray_worker.sh` / `ray_check.py`)
3. Modifies container startup command to `bash ray_head.sh` (or `ray_worker.sh`)
4. Injects environment variables (`RLARK_RAY_PORT`, `RLARK_HEAD_ADDRESS`, etc.)
5. Head node creates a Service (exposing 6379/8265/8080)
6. Worker nodes wait for Head to be ready, then join the cluster

## 14. Object Storage & PVCs

RLark supports mounting persistent volumes to training tasks via `pvcStorageMap` in the Task specification.

### Concept

When a Task specifies `pvcStorageMap`, the Agent's Pull controller automatically creates PVCs with the specified StorageClass before creating the workload, and cleans them up when the task is deleted.

### Configuration

```yaml
kubernetes:
  workload:
    pvcStorageMap:
      my-data-pvc: "ceph-rbd"    # PVC name → StorageClass name
```

### How it works

1. Agent queries StorageClasses via `GET /api/v1/storage/storageclass?clusters=<agent-id>`
2. When creating a workload, Agent calls `ensurePVCs` to create PVCs with the specified StorageClass
3. PVCs are created in the target namespace, scoped to the task
4. On task deletion, PVCs are cleaned up automatically

## 15. User Authentication

RLark provides login and role-based navigation for the Web UI. The current `admin` and `user` distinction is a **frontend gate only**: it selects the admin or platform console, but the Gateway does not enforce these roles as API authorization. Do not treat the UI role as a security boundary or expose the Gateway to untrusted clients on that basis.

### Authentication Flow

1. During deployment, `rlarkadm` generates random passwords and stores them in a KCP Secret (`rlark-ui-auth`)
2. Web UI sends `POST /api/v1/auth/login` with username and password
3. Gateway validates against the KCP Secret and returns the role
4. Frontend stores the login result in `sessionStorage` and uses the selected console route as the role gate

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

1. Agent's Ray init script starts TensorBoard alongside the Ray head process
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
- **Ray integration**: TensorBoard auto-starts for Ray tasks with the appropriate configuration

## 20. SSH Key Management

SSH Key Management allows users to upload SSH public keys through the API or Web UI. A registered key authenticates its named user to the RLark SSH bastion. A key explicitly selected in a Job is also copied to that Job's `sshPublicKey` field for workload injection.

### Concept

Keys are stored by username in a Kubernetes Secret (`rlark-ssh-user-keys`) in the control plane namespace. The SSH server verifies that the presented key is registered for the SSH username. The current implementation does **not** authorize that user for particular Pods; after bastion authentication, there is no per-user or per-Pod policy check. Selecting a key while creating a Job is separate from bastion authorization and places that public key in the generated workload configuration.

### API

- `GET /api/v1/ssh-user-keys` — list all SSH keys (optionally filtered by user)
- `POST /api/v1/ssh-user-keys` — add a new SSH public key for a user
- `DELETE /api/v1/ssh-user-keys/:id` — delete a key by index

### Key Features

- **Web UI management**: A dedicated SSH Keys page in the Web UI for viewing and managing keys
- **Explicit workload injection**: Individual Jobs and Tasks can specify an `sshPublicKey` field; the UI copies the selected registered key into a new Job
- **Conflict detection**: Duplicate keys are detected and rejected with a 409 response
- **Retry on conflict**: The API automatically retries on write conflicts (up to 5 attempts)
- **Key validation**: Public keys are validated using `golang.org/x/crypto/ssh` before storage
