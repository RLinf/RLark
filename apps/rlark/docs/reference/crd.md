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
  - `draining`: `boolean`, optional
- `status`: `object`, optional - ClusterStatus reports the controller's current observation of a data-plane cluster.
  - `conditions`: `array`, optional
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `lastHeartbeatTime`: `string`, optional
  - `onlineNodes`: `integer`, optional
  - `phase`: `string`, optional, enum=Online,Offline,Downgraded,Cordoned,Draining - ClusterPhase summarizes the current lifecycle state of a data-plane cluster. The set is closed in the CRD schema: add...
  - `totalNodes`: `integer`, optional

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
      - `ip`: `string`, optional
      - `job`: `string`, optional
      - `pod`: `string`, optional
      - `task`: `string`, optional
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
- `spec`: `object`, optional
  - `agentType`: `string`, optional, enum=Kubernetes,Docker,Raw
  - `unschedulable`: `boolean`, optional
- `status`: `object`, optional
  - `addresses`: `array`, optional - Addresses 是节点地址列表（InternalIP / Hostname 等）。
    - `items`: `object`, optional - NodeAddress contains information for the node's address.
      - `address`: `string`, required - The node address.
      - `type`: `string`, required - Node address type, one of Hostname, ExternalIP or InternalIP.
  - `allocatable`: `object`, optional - ResourceList is a set of (resource name, quantity) pairs.
  - `capacity`: `object`, optional - ResourceList is a set of (resource name, quantity) pairs.
  - `conditions`: `array`, optional - Conditions 记录节点的结构化状态结论，当前仅包含 ConditionTypeReady。 它由观测（而非 Spec）驱动，因此不填写 ObservedGeneration。
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `diskPressure`: `boolean`, optional
  - `events`: `array`, optional - Events 由数据面 node-agent 上报节点相关 Kubernetes Event（如 DiskPressure 等 Warning 事件及镜像拉取/调度相关事件）。控制面 Task reconciler 在 Task 处于...
    - `items`: `object`, optional - NodeEvent represents a Kubernetes Event observed on a node that is relevant for surfacing to operators (e.g. DiskPres...
      - `count`: `integer`, optional
      - `lastTime`: `string`, optional
      - `message`: `string`, optional
      - `objectKind`: `string`, optional
      - `objectName`: `string`, optional
      - `reason`: `string`, required
      - `source`: `string`, optional
      - `type`: `string`, required
  - `lastHeartbeatTime`: `string`, optional - LastHeartbeatTime 是控制面 Node Reconciler 从 Agent Lease 读取到的最近一次 心跳续约时间；数据面不写入本字段。
  - `lastStatusChangeTime`: `string`, optional - LastStatusChangeTime 是 Phase 最近一次发生变化的时间，与 LastHeartbeatTime 同由控制面 Node Reconciler 写入，用于区分「长时间保持同一状态」与「刚刚翻转」。
  - `nodeInfo`: `object`, optional - NodeInfo 是节点基础信息，由数据面 Agent 上报；控制面不写入本字段。
    - `agentVersion`: `string`, optional
    - `architecture`: `string`, optional
    - `kernelVersion`: `string`, optional
    - `operatingSystem`: `string`, optional
  - `phase`: `string`, optional, enum=Online,Offline - Phase 是节点连通性聚合阶段，由数据面 node-agent 上报。
  - `pullProgress`: `array`, optional - PullProgress 是节点上正在进行的镜像拉取进度，由 node-agent 上报。
    - `items`: `object`, optional - PullProgress captures the progress of an in-flight image pull on a node.
      - `downloaded`: `integer`, required
      - `image`: `string`, required
      - `message`: `string`, optional
      - `speed`: `number`, required
      - `status`: `string`, required
      - `total`: `integer`, required
  - `reason`: `string`, optional - Reason 是 Phase 的机器可读原因（CamelCase），与 Conditions 中 ConditionTypeReady 的 Reason 同源，供列表视图直接展示。 该字段与 Conditions 均由数据面 node...
  - `storage`: `object`, optional - NodeStorageStatus reports kubelet filesystem usage in bytes.
    - `availableBytes`: `integer`, optional
    - `capacityBytes`: `integer`, optional
    - `usedBytes`: `integer`, optional
  - `used`: `object`, optional - ResourceList is a set of (resource name, quantity) pairs.

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
