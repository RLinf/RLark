# CRD Schema Reference

> **Generated file:** This page is generated from `api/config/crd/bases` by `apps/rlark/cmd/crd-api-docgen`. Do not edit it manually; run `make generate-crd-schema-docs` instead.

Kubernetes resource operations and schemas generated from the current CRD manifests. This is not the RLark Gateway HTTP API reference. Descriptions are copied from source schemas, may retain their original language, and are shortened for readability.

## Addon

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `addons`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/addons`

List addons resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `AddonList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/addons`

Create a Addon resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Addon`

Responses:
- `201` Created → `Addon`
- `202` Accepted → `Addon`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/addons`

Delete a collection of addons resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/addons/{name}`

Get a Addon resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Addon`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/addons/{name}`

Replace a Addon resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Addon`

Responses:
- `200` OK → `Addon`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/addons/{name}`

Patch a Addon resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Addon`

Responses:
- `200` OK → `Addon`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/addons/{name}`

Delete a Addon resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/addons/{name}/status`

Get the status subresource for Addon.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Addon`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/addons/{name}/status`

Replace the status subresource for Addon.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Addon`

Responses:
- `200` OK → `Addon`
- `202` Accepted → `Addon`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/addons/{name}/status`

Patch the status subresource for Addon.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Addon`

Responses:
- `200` OK → `Addon`
- `202` Accepted → `Addon`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `addonName`: `string`, required
  - `values`: `object`, optional
  - `version`: `string`, optional
- `status`: `object`, optional
  - `conditions`: `array`, optional - Conditions 记录 Addon 的结构化状态结论，当前仅包含 ConditionTypeReady。
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `message`: `string`, optional
  - `observedGeneration`: `integer`, optional - ObservedGeneration 是 Addon 控制器最近一次完成调谐时所观测到的 metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取 phase / conditions 之前...
  - `phase`: `string`, optional, enum=Pending,Installing,Ready,Failed,Upgrading - AddonPhase 是 Addon 安装与升级流程的聚合阶段，由数据面 Agent 观测后上报。 取值集合在 CRD Schema 层封闭，新增取值必须同步更新枚举校验与文档。
  - `version`: `string`, optional

## Cluster

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Cluster`
- Resource: `clusters`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/clusters`

List clusters resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `ClusterList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/clusters`

Create a Cluster resource.

Parameters:
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Cluster`

Responses:
- `201` Created → `Cluster`
- `202` Accepted → `Cluster`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/clusters`

Delete a collection of clusters resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/clusters/{name}`

Get a Cluster resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Cluster`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/clusters/{name}`

Replace a Cluster resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Cluster`

Responses:
- `200` OK → `Cluster`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/clusters/{name}`

Patch a Cluster resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Cluster`

Responses:
- `200` OK → `Cluster`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/clusters/{name}`

Delete a Cluster resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/clusters/{name}/status`

Get the status subresource for Cluster.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Cluster`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/clusters/{name}/status`

Replace the status subresource for Cluster.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Cluster`

Responses:
- `200` OK → `Cluster`
- `202` Accepted → `Cluster`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/clusters/{name}/status`

Patch the status subresource for Cluster.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Cluster`

Responses:
- `200` OK → `Cluster`
- `202` Accepted → `Cluster`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional - ClusterSpec declares the durable operational state of a data-plane cluster.
  - `agentNamespace`: `string`, required - AgentNamespace is the management-plane namespace that contains the agent heartbeat Lease and Node CRs.
  - `cordoned`: `boolean`, optional
  - `delisted`: `boolean`, optional
  - `drainPreviousCordoned`: `boolean`, optional
  - `draining`: `boolean`, optional
- `status`: `object`, optional - ClusterStatus reports the controller's current observation of a data-plane cluster.
  - `conditions`: `array`, optional - Conditions records the structured conclusions of the Cluster controller, which is their only writer. Ready and Degrad...
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `degradedNodes`: `integer`, optional - DegradedNodes is the number of connected Nodes that are not ready or report Degraded=True.
  - `disconnectedNodes`: `integer`, optional - DisconnectedNodes is the number of Nodes whose AgentConnected condition is not True.
  - `dominantFailureReason`: `string`, optional - DominantFailureReason is the most frequent reason among unavailable candidate Nodes.
  - `lastHeartbeatTime`: `string`, optional
  - `lastStatusChangeTime`: `string`, optional - LastStatusChangeTime is the time Phase last changed. It carries the same semantics as the identically named field on ...
  - `observedGeneration`: `integer`, optional - ObservedGeneration is the metadata.generation observed by the Cluster controller the last time it finished a reconcil...
  - `onlineNodes`: `integer`, optional
  - `phase`: `string`, optional, enum=Online,Offline,Downgraded,Cordoned,Draining,Delisted - ClusterPhase summarizes the current lifecycle state of a data-plane cluster. The set is closed in the CRD schema: add...
  - `readyNodes`: `integer`, optional - ReadyNodes is the number of connected Nodes with Ready=True that are not cordoned by spec.unschedulable. A ready Node...
  - `totalNodes`: `integer`, optional
  - `unschedulableNodes`: `integer`, optional - UnschedulableNodes is the number of Nodes with spec.unschedulable=true.

## ClusterUpgradeJob

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `clusterupgradejobs`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/clusterupgradejobs`

List clusterupgradejobs resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `ClusterUpgradeJobList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/clusterupgradejobs`

Create a ClusterUpgradeJob resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `ClusterUpgradeJob`

Responses:
- `201` Created → `ClusterUpgradeJob`
- `202` Accepted → `ClusterUpgradeJob`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/clusterupgradejobs`

Delete a collection of clusterupgradejobs resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}`

Get a ClusterUpgradeJob resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}`

Replace a ClusterUpgradeJob resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `ClusterUpgradeJob`

Responses:
- `200` OK → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}`

Patch a ClusterUpgradeJob resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `ClusterUpgradeJob`

Responses:
- `200` OK → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}`

Delete a ClusterUpgradeJob resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}/status`

Get the status subresource for ClusterUpgradeJob.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}/status`

Replace the status subresource for ClusterUpgradeJob.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `ClusterUpgradeJob`

Responses:
- `200` OK → `ClusterUpgradeJob`
- `202` Accepted → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/clusterupgradejobs/{name}/status`

Patch the status subresource for ClusterUpgradeJob.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `ClusterUpgradeJob`

Responses:
- `200` OK → `ClusterUpgradeJob`
- `202` Accepted → `ClusterUpgradeJob`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional - ClusterUpgradeJobSpec defines the desired state of a cluster upgrade job. This CR is created by the control plane for...
  - `autoRollback`: `boolean`, optional - AutoRollback indicates whether to automatically rollback on failure.
  - `configPatch`: `string`, optional - ConfigPatch is the configuration patch to apply.
  - `image`: `string`, required - Image is the target rlarkadm image for upgrade.
  - `timeoutSeconds`: `integer`, optional - TimeoutSeconds is the timeout for the upgrade.
  - `upgradeJobName`: `string`, required - UpgradeJobName is the name of the parent UpgradeJob.
- `status`: `object`, optional - ClusterUpgradeJobStatus defines the observed state of a cluster upgrade job. This is updated by the data plane agent.
  - `completionTime`: `string`, optional - CompletionTime is the time when the upgrade completed.
  - `jobName`: `string`, optional - JobName is the name of the local upgrade Job.
  - `jobNamespace`: `string`, optional - JobNamespace is the namespace of the local upgrade Job.
  - `message`: `string`, optional - Message provides additional information about the current phase.
  - `phase`: `string`, optional, enum=Pending,Upgrading,Succeeded,Failed,RolledBack - Phase is the current phase of the upgrade.
  - `rollbackJobName`: `string`, optional - RollbackJobName is the name of the rollback Job if rollback was performed.
  - `startTime`: `string`, optional - StartTime is the time when the upgrade started.

## Domain

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Cluster`
- Resource: `domains`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/domains`

List domains resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `DomainList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/domains`

Create a Domain resource.

Parameters:
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Domain`

Responses:
- `201` Created → `Domain`
- `202` Accepted → `Domain`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/domains`

Delete a collection of domains resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/domains/{name}`

Get a Domain resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Domain`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/domains/{name}`

Replace a Domain resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Domain`

Responses:
- `200` OK → `Domain`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/domains/{name}`

Patch a Domain resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Domain`

Responses:
- `200` OK → `Domain`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}`

Delete a Domain resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/domains/{name}/status`

Get the status subresource for Domain.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Domain`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/domains/{name}/status`

Replace the status subresource for Domain.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Domain`

Responses:
- `200` OK → `Domain`
- `202` Accepted → `Domain`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/domains/{name}/status`

Patch the status subresource for Domain.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Domain`

Responses:
- `200` OK → `Domain`
- `202` Accepted → `Domain`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `cidr`: `string`, optional
- `status`: `object`, optional
  - `conditions`: `array`, optional - Conditions 记录 Domain 的结构化状态结论，当前仅包含 ConditionTypeValidated （Spec.CIDR 是否合法）。它由 Spec 驱动，写入方必须填写 ObservedGeneration。
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `ipAllocations`: `array`, optional - IPAllocations 由 Domain 控制器独占写入，其它组件不得修改。该列表规模随 Domain CIDR 增长（/16 可达数万条）。
    - `items`: `object`, optional
      - `ip`: `string`, optional - IP 是该条目占用的 Domain 虚拟 IP。
      - `job`: `string`, optional - Job 是拥有 Task 的 Job 名称。Job 是集群级资源，因此只有名称没有 namespace。 仅当 Task 的 controller ownerReference 可证明归属于 Job 时才填写，否则为空。
      - `pod`: `string`, optional - Pod 是上报该 IP 的数据面 Pod，形如 namespace/name。
      - `task`: `string`, optional - Task 是归属的控制面 Task，形如 namespace/name。
  - `observedGeneration`: `integer`, optional - ObservedGeneration 是 Domain 控制器最近一次完成调谐时所观测到的 metadata.generation。它是控制器契约而非 API Server 保证：消费者在依据 ipAllocations 做判断前，应...

## DomainPeer

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `domainpeers`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/domainpeers`

List domainpeers resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `DomainPeerList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/domainpeers`

Create a DomainPeer resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `DomainPeer`

Responses:
- `201` Created → `DomainPeer`
- `202` Accepted → `DomainPeer`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/domainpeers`

Delete a collection of domainpeers resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/domainpeers/{name}`

Get a DomainPeer resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/domainpeers/{name}`

Replace a DomainPeer resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `DomainPeer`

Responses:
- `200` OK → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/domainpeers/{name}`

Patch a DomainPeer resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `DomainPeer`

Responses:
- `200` OK → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/domainpeers/{name}`

Delete a DomainPeer resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/domainpeers/{name}/status`

Get the status subresource for DomainPeer.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/domainpeers/{name}/status`

Replace the status subresource for DomainPeer.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `DomainPeer`

Responses:
- `200` OK → `DomainPeer`
- `202` Accepted → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/domainpeers/{name}/status`

Patch the status subresource for DomainPeer.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `DomainPeer`

Responses:
- `200` OK → `DomainPeer`
- `202` Accepted → `DomainPeer`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `cert`: `string`, optional
  - `key`: `string`, optional
  - `pods`: `array`, optional
    - `items`: `object`, optional
      - `globalNamespace`: `string`, optional
      - `ip`: `string`, optional
      - `localIP`: `string`, optional
      - `name`: `string`, optional
      - `namespace`: `string`, optional
      - `node`: `string`, optional
      - `uid`: `string`, optional
  - `prefixLen`: `integer`, optional
- `status`: `object`, optional

## Job

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Cluster`
- Resource: `jobs`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/jobs`

List jobs resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `JobList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/jobs`

Create a Job resource.

Parameters:
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Job`

Responses:
- `201` Created → `Job`
- `202` Accepted → `Job`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/jobs`

Delete a collection of jobs resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}`

Get a Job resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Job`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/jobs/{name}`

Replace a Job resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Job`

Responses:
- `200` OK → `Job`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/jobs/{name}`

Patch a Job resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Job`

Responses:
- `200` OK → `Job`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/jobs/{name}`

Delete a Job resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/jobs/{name}/status`

Get the status subresource for Job.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Job`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/jobs/{name}/status`

Replace the status subresource for Job.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Job`

Responses:
- `200` OK → `Job`
- `202` Accepted → `Job`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/jobs/{name}/status`

Patch the status subresource for Job.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Job`

Responses:
- `200` OK → `Job`
- `202` Accepted → `Job`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `domain`: `string`, optional
  - `sshPublicKey`: `string`, optional
  - `suspended`: `boolean`, optional - Setting spec.suspended to true starts an orderly stop without deleting the Job resource. The Job becomes Stopped and ...
  - `tags`: `array`, optional
    - `items`: `object`, optional - JobTag 表示一个任务标签，由 key 和多个 value 组成。 标签的个数与 key/value 长度上限不在 CRD Schema 层封闭，由 Gateway 在写入 路径上校验，详见开发者文档的 API 约定。
      - `key`: `string`, required
      - `values`: `array`, required
  - `tasks`: `array`, optional
    - `items`: `object`, optional
      - `downstreamName`: `string`, optional
      - `drainRequests`: `array`, optional - DrainRequests are one-shot Node-agent-owned requests to stop this Task's root workload during Node Drain. The Node Ag...
        - `items`: `object`, optional - TaskDrainRequest identifies a Node Drain operation that must stop this Task before the Node can be cordoned. The oper...
      - `environments`: `array`, optional
        - `items`: `object`, optional - TaskEnvironment 表示 Task 请求的一种可扩展环境能力。 Type 是带所有者作用域的稳定类型标识，Version 表示该类型的参数版本。 Parameters 必须由对应的已注册 Handler 严格解码和校验；未...
      - `head`: `boolean`, optional
      - `name`: `string`, optional
      - `placement`: `object`, required - TaskPlacementSpec describes scheduling constraints for the workload.
      - `role`: `string`, required - TaskRole 描述 Task 在业务编排中的角色，用于前端分组与展示。 该字段当前不在 CRD 层做枚举校验：它可能承载上层编排框架的自定义角色名， 收窄取值会阻止合法的既有数据写入。请勿据此推断取值的封闭集合。
      - `runtime`: `object`, required - TaskRuntimeSpec selects the runtime implementation and instance that lowers the common Pod template. Name is a stable...
      - `suspended`: `boolean`, optional
      - `tags`: `array`, optional
        - `items`: `object`, optional - JobTag 表示一个任务标签，由 key 和多个 value 组成。 标签的个数与 key/value 长度上限不在 CRD Schema 层封闭，由 Gateway 在写入 路径上校验，详见开发者文档的 API 约定。
      - `workload`: `object`, required - TaskWorkloadSpec is the common workload declaration for every runtime. Kubernetes applies the Pod template natively. ...
- `status`: `object`, optional
  - `conditions`: `array`, optional
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `endTime`: `string`, optional
  - `observedGeneration`: `integer`, optional - ObservedGeneration 是 Job 控制器最近一次完成调谐时所观测到的 metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取 phase / conditions 之前，应...
  - `phase`: `string`, optional, enum=Pending,Running,Succeeded,Failed,Stopped - JobPhase 是 Job 的聚合生命周期阶段，由控制面 Job 控制器根据其 Task 的 观测状态推导得出。取值集合必须与 Job 状态机保持一致。
  - `startTime`: `string`, optional
  - `tasks`: `array`, optional
    - `items`: `object`, optional
      - `message`: `string`, optional
      - `name`: `string`, optional
      - `phase`: `string`, optional, enum=Pending,Running,Succeeded,Failed,Stopped - TaskPhase 是单个 Task 的生命周期阶段，由数据面 Agent 观测本地工作负载后上报。 取值集合与 JobPhase 对齐，Task 没有独立的 Stopping 中间态。

## Node

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `nodes`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/nodes`

List nodes resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `NodeList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/nodes`

Create a Node resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Node`

Responses:
- `201` Created → `Node`
- `202` Accepted → `Node`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/nodes`

Delete a collection of nodes resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/nodes/{name}`

Get a Node resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Node`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}`

Replace a Node resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Node`

Responses:
- `200` OK → `Node`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/nodes/{name}`

Patch a Node resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Node`

Responses:
- `200` OK → `Node`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/nodes/{name}`

Delete a Node resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/nodes/{name}/status`

Get the status subresource for Node.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Node`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/nodes/{name}/status`

Replace the status subresource for Node.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Node`

Responses:
- `200` OK → `Node`
- `202` Accepted → `Node`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/nodes/{name}/status`

Patch the status subresource for Node.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Node`

Responses:
- `200` OK → `Node`
- `202` Accepted → `Node`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional - NodeSpec contains only management-owned durable desired state. Agents must not write this section; administrative API...
  - `agentType`: `string`, optional, enum=Kubernetes,Docker,Raw - AgentType is a reserved compatibility field for the Node access form. Deprecated: use Runtime.Type for new integratio...
  - `identity`: `object`, optional - Identity is set by the management API or control plane when the Node is registered.
    - `domain`: `string`, optional - Domain is the owning Domain identifier.
    - `id`: `string`, optional - ID is the stable RLark business identity of the Node.
    - `owner`: `string`, optional - Owner is the management principal or controller that owns the Node.
    - `tenant`: `string`, optional - Tenant is the owning tenant identifier, independent of Kubernetes namespace placement.
  - `operations`: `array`, optional - Operations contains uniquely identified one-shot maintenance or drain requests.
    - `items`: `object`, optional - NodeOperationRequest identifies one administrative operation. Administrators create a new ID for each new action; con...
      - `id`: `string`, required - ID is a stable operation identifier supplied by the requester. It is required and must be non-empty: an operation wit...
      - `type`: `string`, required, enum=Drain,EnterMaintenance - Type selects the requested one-shot operation.
  - `runtime`: `object`, optional - Runtime is the management-selected runtime reference for this Node.
    - `name`: `string`, optional - Name identifies the runtime instance or provider configuration.
    - `type`: `string`, optional, enum=Kubernetes,Docker,Raw - Type identifies the configured runtime implementation.
  - `unschedulable`: `boolean`, optional - Unschedulable durably blocks placement of new workloads without stopping existing ones.
- `status`: `object`, optional - NodeStatus separates Agent-observed facts from Node Controller-derived lifecycle conclusions. Job and scheduler compo...
  - `activeWorkloads`: `object`, optional - ActiveWorkloads is the Agent-observed bounded summary of current workloads.
    - `pending`: `integer`, optional
    - `rlarkManaged`: `integer`, optional - RLarkManaged is the count of Pods on this Node that are managed by RLark (identified by the RLark task-management ann...
    - `running`: `integer`, optional
    - `stopping`: `integer`, optional
    - `total`: `integer`, optional
  - `addresses`: `array`, optional - Addresses are Agent-observed node addresses.
    - `items`: `object`, optional - NodeAddress contains information for the node's address.
      - `address`: `string`, required - The node address.
      - `type`: `string`, required - Node address type, one of Hostname, ExternalIP or InternalIP.
  - `allocatable`: `object`, optional - Allocatable is the Kubernetes Node's scheduler-declared capacity after system reservations. Schedulers use this value...
  - `capabilitySnapshot`: `object`, optional - CapabilitySnapshot is the Agent-observed structured capability declaration. Transient capability availability is repo...
    - `devices`: `array`, optional
      - `items`: `object`, optional - NodeDeviceCapability describes Kubernetes extended-resource inventory and scheduling availability.
    - `network`: `object`, optional - NodeNetworkCapability describes Node-local networking features supported by the runtime.
      - `crossCluster`: `boolean`, optional
      - `hostNetwork`: `boolean`, optional
      - `serviceExposure`: `boolean`, optional
    - `operations`: `object`, optional - NodeOperationsCapability is reserved for operations whose support is observed locally.
      - `exec`: `boolean`, optional
      - `logs`: `boolean`, optional
      - `terminal`: `boolean`, optional
    - `runtime`: `object`, optional - NodeRuntimeCapability describes the runtime and workload kinds the Agent can run.
      - `type`: `string`, optional, enum=Kubernetes,Docker,Raw - Type identifies the runtime implementation.
      - `version`: `string`, optional - Version is the runtime implementation version.
      - `workloadKinds`: `array`, optional - WorkloadKinds lists the workload kinds supported by this runtime.
    - `storage`: `object`, optional - NodeStorageCapability is reserved for storage support observed locally on the Node.
      - `storageClasses`: `array`, optional
      - `volumeTypes`: `array`, optional
  - `capacity`: `object`, optional - Capacity is the Kubernetes Node's physical resource inventory. It is an informational upper bound, not the schedulabl...
  - `conditions`: `array`, optional - Conditions are calculated only by the Node Controller from desired state and Agent facts. They are observation-driven...
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `diskPressure`: `boolean`, optional - DiskPressure is the raw kubelet observation used by the Node Controller to derive ResourcePressure.
  - `drainOperations`: `array`, optional - DrainOperations is the Node Controller's bounded summary of requested drain operations.
    - `items`: `object`, optional - NodeDrainOperationStatus is the bounded Node Controller summary of one drain operation.
      - `blocked`: `integer`, optional
      - `completed`: `boolean`, optional
      - `operationID`: `string`, required
      - `waiting`: `integer`, optional
  - `events`: `array`, optional - Events is a bounded node-agent-reported diagnostic event summary.
    - `items`: `object`, optional - NodeEvent represents a Kubernetes Event observed on a node that is relevant for surfacing to operators (e.g. DiskPres...
      - `count`: `integer`, optional
      - `lastTime`: `string`, optional
      - `message`: `string`, optional
      - `objectKind`: `string`, optional
      - `objectName`: `string`, optional
      - `reason`: `string`, required
      - `source`: `string`, optional
      - `type`: `string`, required
  - `kubernetesCordoned`: `boolean`, optional - KubernetesCordoned is the raw observation of the local Kubernetes Node's spec.unschedulable reported by the Agent. RL...
  - `kubernetesNodeUID`: `string`, optional - KubernetesNodeUID is the Agent-observed UID of the local Kubernetes Node object. It identifies one Node incarnation a...
  - `kubernetesReady`: `boolean`, optional - KubernetesReady is the raw Kubernetes Node Ready observation reported by the Agent.
  - `lastHeartbeatTime`: `string`, optional - LastHeartbeatTime is the Agent heartbeat renewal the Node Controller read from the coordination Lease. It is republis...
  - `lastStatusChangeTime`: `string`, optional - LastStatusChangeTime is the latest time an Agent-observed or controller-derived state changed.
  - `nodeInfo`: `object`, optional - NodeInfo is Agent-reported operating system, architecture, Agent, and runtime version information.
    - `agentVersion`: `string`, optional
    - `architecture`: `string`, optional
    - `kernelVersion`: `string`, optional
    - `operatingSystem`: `string`, optional
    - `runtimeVersion`: `string`, optional
  - `observedNodeLabels`: `object`, optional - ObservedNodeLabels is the Agent-observed label dictionary read from the local Kubernetes Node object. It is the user-...
  - `phase`: `string`, optional, enum=Registering,Online,Degraded,Offline,Cordoned,Draining - Phase is a concise lifecycle summary calculated only by the Node Controller.
  - `pullProgress`: `array`, optional - PullProgress is node-agent-reported progress for in-flight image pulls.
    - `items`: `object`, optional - PullProgress captures the progress of an in-flight image pull on a node.
      - `downloaded`: `integer`, required
      - `image`: `string`, required
      - `message`: `string`, optional
      - `speed`: `number`, required
      - `status`: `string`, required
      - `total`: `integer`, required
  - `storage`: `object`, optional - Storage is a bounded monitoring snapshot and must not be used for placement or capacity decisions.
    - `availableBytes`: `integer`, optional
    - `capacityBytes`: `integer`, optional
    - `usedBytes`: `integer`, optional
  - `used`: `object`, optional - Used is the sum of non-terminal Pod resource requests already allocated for scheduling. It is not actual runtime util...

## Pod

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `pods`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/pods`

List pods resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `PodList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/pods`

Create a Pod resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Pod`

Responses:
- `201` Created → `Pod`
- `202` Accepted → `Pod`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/pods`

Delete a collection of pods resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/pods/{name}`

Get a Pod resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Pod`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/pods/{name}`

Replace a Pod resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Pod`

Responses:
- `200` OK → `Pod`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}`

Patch a Pod resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Pod`

Responses:
- `200` OK → `Pod`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/pods/{name}`

Delete a Pod resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/pods/{name}/status`

Get the status subresource for Pod.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Pod`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/pods/{name}/status`

Replace the status subresource for Pod.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Pod`

Responses:
- `200` OK → `Pod`
- `202` Accepted → `Pod`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}/status`

Patch the status subresource for Pod.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Pod`

Responses:
- `200` OK → `Pod`
- `202` Accepted → `Pod`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional - PodSpec 包含 Pod 的标识和引用信息，由数据面上报时设置。
  - `domain`: `string`, optional
  - `podName`: `string`, optional
  - `podNamespace`: `string`, optional
  - `taskName`: `string`, optional
  - `taskNamespace`: `string`, optional
- `status`: `object`, optional - PodStatus 包含 Pod 的运行状态信息（节点、IP、阶段等）， 对应 k8s 中由调度器和 kubelet 设置的观测状态。
  - `ip`: `string`, optional
  - `message`: `string`, optional
  - `node`: `string`, optional
  - `phase`: `string`, optional, enum=Pending,Running,Succeeded,Failed,Unknown

## Task

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Namespaced`
- Resource: `tasks`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/tasks`

List tasks resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `TaskList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/tasks`

Create a Task resource.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Task`

Responses:
- `201` Created → `Task`
- `202` Accepted → `Task`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/tasks`

Delete a collection of tasks resources.

Parameters:
- `namespace` (query)
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/tasks/{name}`

Get a Task resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Task`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/tasks/{name}`

Replace a Task resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Task`

Responses:
- `200` OK → `Task`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/tasks/{name}`

Patch a Task resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Task`

Responses:
- `200` OK → `Task`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}`

Delete a Task resource.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/tasks/{name}/status`

Get the status subresource for Task.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Task`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/tasks/{name}/status`

Replace the status subresource for Task.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Task`

Responses:
- `200` OK → `Task`
- `202` Accepted → `Task`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/tasks/{name}/status`

Patch the status subresource for Task.

Parameters:
- `namespace` (query)
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Task`

Responses:
- `200` OK → `Task`
- `202` Accepted → `Task`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `downstreamName`: `string`, optional
  - `drainRequests`: `array`, optional - DrainRequests are one-shot Node-agent-owned requests to stop this Task's root workload during Node Drain. The Node Ag...
    - `items`: `object`, optional - TaskDrainRequest identifies a Node Drain operation that must stop this Task before the Node can be cordoned. The oper...
      - `nodeName`: `string`, required
      - `nodeUID`: `string`, required
      - `operationID`: `string`, required
  - `environments`: `array`, optional
    - `items`: `object`, optional - TaskEnvironment 表示 Task 请求的一种可扩展环境能力。 Type 是带所有者作用域的稳定类型标识，Version 表示该类型的参数版本。 Parameters 必须由对应的已注册 Handler 严格解码和校验；未...
      - `parameters`: `object`, optional
      - `type`: `string`, required
      - `version`: `string`, required
  - `placement`: `object`, required - TaskPlacementSpec describes scheduling constraints for the workload.
    - `nodeSelector`: `object`, optional
  - `role`: `string`, required - TaskRole 描述 Task 在业务编排中的角色，用于前端分组与展示。 该字段当前不在 CRD 层做枚举校验：它可能承载上层编排框架的自定义角色名， 收窄取值会阻止合法的既有数据写入。请勿据此推断取值的封闭集合。
  - `runtime`: `object`, required - TaskRuntimeSpec selects the runtime implementation and instance that lowers the common Pod template. Name is a stable...
    - `kind`: `string`, required - RuntimeKind 表示负责执行 Task 工作负载的 Runtime 类型。 CRD 有意不限制该字符串的枚举值，使 Docker、Raw 等 Runtime 可以在不修改 Task Schema 的情况下扩展。Agent 必须...
    - `name`: `string`, optional
  - `suspended`: `boolean`, optional
  - `tags`: `array`, optional
    - `items`: `object`, optional - JobTag 表示一个任务标签，由 key 和多个 value 组成。 标签的个数与 key/value 长度上限不在 CRD Schema 层封闭，由 Gateway 在写入 路径上校验，详见开发者文档的 API 约定。
      - `key`: `string`, required
      - `values`: `array`, required
  - `workload`: `object`, required - TaskWorkloadSpec is the common workload declaration for every runtime. Kubernetes applies the Pod template natively. ...
    - `carrier`: `object`, required - TaskWorkloadCarrier selects the Runtime-specific carrier used to realize a workload. Type and Version identify a carr...
      - `parameters`: `object`, optional
      - `type`: `string`, required
      - `version`: `string`, required
    - `kind`: `string`, optional - WorkloadKind identifies the workload semantics independently of its Runtime carrier. The CRD deliberately keeps this ...
    - `parameters`: `object`, optional
    - `template`: `object`, optional - PodTemplateSpec describes the data a pod should have when created from a template
    - `version`: `string`, required
- `status`: `object`, optional
  - `completionTime`: `string`, optional
  - `conditions`: `array`, optional
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `drainRequests`: `array`, optional - DrainRequests is the bounded current observation written by the Task Host.
    - `items`: `object`, optional - TaskDrainStatus reports the Task Host's observation of a Node Drain request.
      - `nodeUID`: `string`, required
      - `observedGeneration`: `integer`, optional
      - `operationID`: `string`, required
      - `reason`: `string`, optional
      - `state`: `string`, required, enum=WaitingForReschedule,Blocked - TaskDrainState is the current observation of a Task Drain request. WaitingForReschedule means the Task root workload ...
  - `events`: `array`, optional - Events 在 Task 处于 Pending 期间由控制面 Task reconciler 从各节点 Node.status.events 聚合而来，包含 DiskPressure 等 Warning 事件及镜像 拉取/调度相关事...
    - `items`: `object`, optional - NodeEvent represents a Kubernetes Event observed on a node that is relevant for surfacing to operators (e.g. DiskPres...
      - `count`: `integer`, optional
      - `lastTime`: `string`, optional
      - `message`: `string`, optional
      - `objectKind`: `string`, optional
      - `objectName`: `string`, optional
      - `reason`: `string`, required
      - `source`: `string`, optional
      - `type`: `string`, required
  - `message`: `string`, optional
  - `observedGeneration`: `integer`, optional - ObservedGeneration 是数据面 Agent 最近一次完成本地工作负载调谐时所观测到的 metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取 phase / message...
  - `observedNodes`: `array`, optional - ObservedNodes 是当前承载该 Task 工作负载的节点名，由数据面 Agent 排序去重后上报。
  - `phase`: `string`, optional, enum=Pending,Running,Succeeded,Failed,Stopped - TaskPhase 是单个 Task 的生命周期阶段，由数据面 Agent 观测本地工作负载后上报。 取值集合与 JobPhase 对齐，Task 没有独立的 Stopping 中间态。
  - `pullProgress`: `array`, optional - PullProgress 由数据面节点上的 image pull monitor 上报，仅在 Pod 处于 ContainerCreating（尚未 Running）期间由 cluster-agent 聚合写入。Pod 进入 Runn...
    - `items`: `object`, optional - PullProgress captures the progress of an in-flight image pull on a node.
      - `downloaded`: `integer`, required
      - `image`: `string`, required
      - `message`: `string`, optional
      - `speed`: `number`, required
      - `status`: `string`, required
      - `total`: `integer`, required
  - `retryCount`: `integer`, optional
  - `startTime`: `string`, optional
  - `tensorBoardProxy`: `string`, optional

## UpgradeJob

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Cluster`
- Resource: `upgradejobs`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/upgradejobs`

List upgradejobs resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `UpgradeJobList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/upgradejobs`

Create a UpgradeJob resource.

Parameters:
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `UpgradeJob`

Responses:
- `201` Created → `UpgradeJob`
- `202` Accepted → `UpgradeJob`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/upgradejobs`

Delete a collection of upgradejobs resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}`

Get a UpgradeJob resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}`

Replace a UpgradeJob resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `UpgradeJob`

Responses:
- `200` OK → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}`

Patch a UpgradeJob resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `UpgradeJob`

Responses:
- `200` OK → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}`

Delete a UpgradeJob resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}/status`

Get the status subresource for UpgradeJob.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}/status`

Replace the status subresource for UpgradeJob.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `UpgradeJob`

Responses:
- `200` OK → `UpgradeJob`
- `202` Accepted → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/upgradejobs/{name}/status`

Patch the status subresource for UpgradeJob.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `UpgradeJob`

Responses:
- `200` OK → `UpgradeJob`
- `202` Accepted → `UpgradeJob`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional - UpgradeJobSpec defines the desired state of an upgrade job.
  - `clusterNames`: `array`, optional - ClusterNames is the list of target cluster names for upgrade.
  - `clusterSelector`: `object`, optional - ClusterSelector selects the target clusters for upgrade.
    - `matchExpressions`: `array`, optional - matchExpressions is a list of label selector requirements. The requirements are ANDed.
      - `items`: `object`, optional - A label selector requirement is a selector that contains values, a key, and an operator that relates the key and values.
    - `matchLabels`: `object`, optional - matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels map is equivalent to an element of...
  - `configPatch`: `string`, optional - ConfigPatch is the full deployment configuration for this upgrade. When empty, the current configuration from rlark-s...
  - `image`: `string`, required - Image is the target rlarkadm image for upgrade.
  - `maintenanceWindow`: `object`, optional - MaintenanceWindow defines the maintenance window for the upgrade.
    - `endTime`: `string`, optional - EndTime is the end time of the maintenance window.
    - `startTime`: `string`, optional - StartTime is the start time of the maintenance window.
  - `strategy`: `object`, optional - Strategy defines the upgrade strategy.
    - `autoRollback`: `boolean`, optional - AutoRollback indicates whether to automatically rollback on failure.
    - `backoffLimit`: `integer`, optional - BackoffLimit is the number of retries before declaring the upgrade failed.
    - `concurrency`: `integer`, optional - Concurrency is the number of clusters to upgrade concurrently.
    - `failureTolerate`: `number`, optional - FailureTolerate is the percentage of clusters that can fail before stopping the upgrade.
    - `timeoutSeconds`: `integer`, optional - TimeoutSeconds is the timeout for each cluster upgrade.
- `status`: `object`, optional - UpgradeJobStatus defines the observed state of an upgrade job.
  - `completionTime`: `string`, optional - CompletionTime is the time when the upgrade job completed.
  - `conditions`: `array`, optional - Conditions is the list of conditions for each cluster.
    - `items`: `object`, optional - UpgradeJobCondition reports the per-cluster result of an upgrade job.
      - `cluster`: `string`, required - Cluster is the name of the cluster.
      - `reason`: `string`, optional - Reason is a human-readable message about the current status.
      - `rollbackImage`: `string`, optional - RollbackImage is the image the cluster was rolled back to.
      - `status`: `string`, required - Status is the phase of the ClusterUpgradeJob (Pending, Upgrading, Succeeded, Failed, RolledBack).
      - `targetImage`: `string`, optional - TargetImage is the image the cluster was asked to upgrade to.
      - `type`: `string`, required - Type is the type of the condition. Reserved for future use.
      - `upgradeTime`: `string`, optional - UpgradeTime is when the cluster upgrade completed.
  - `failedClusters`: `integer`, optional - FailedClusters is the number of clusters that failed to upgrade.
  - `phase`: `string`, optional, enum=Pending,Upgrading,Succeeded,Failed,RollingBack - Phase is the current phase of the upgrade job.
  - `rollbackClusters`: `integer`, optional - RollbackClusters is the number of clusters that were rolled back.
  - `startTime`: `string`, optional - StartTime is the time when the upgrade job started.
  - `totalClusters`: `integer`, optional - TotalClusters is the total number of target clusters.
  - `upgradedClusters`: `integer`, optional - UpgradedClusters is the number of clusters that have been upgraded successfully.
  - `upgradingClusters`: `integer`, optional - UpgradingClusters is the number of clusters currently being upgraded.
