# CRD Schema Reference

Kubernetes resource operations and schemas generated from the current CRD manifests. This is not the RLark Gateway HTTP API reference.

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
  - `conditions`: `array`, optional
    - `items`: `object`, optional - Condition contains details for one aspect of the current state of this API Resource.
      - `lastTransitionTime`: `string`, required - lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the un...
      - `message`: `string`, required - message is a human readable message indicating details about the transition. This may be an empty string.
      - `observedGeneration`: `integer`, optional - observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metad...
      - `reason`: `string`, required - reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of spe...
      - `status`: `string`, required, enum=True,False,Unknown - status of the condition, one of True, False, Unknown.
      - `type`: `string`, required - type of condition in CamelCase or in foo.example.com/CamelCase.
  - `message`: `string`, optional
  - `phase`: `string`, optional
  - `version`: `string`, optional

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
  - `ipAllocations`: `array`, optional
    - `items`: `object`, optional
      - `ip`: `string`, optional
      - `job`: `string`, optional
      - `pod`: `string`, optional
      - `task`: `string`, optional

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
  - `stopped`: `boolean`, optional
  - `tasks`: `array`, optional
    - `items`: `object`, optional
      - `agentType`: `string`, optional
      - `docker`: `object`, optional
      - `domain`: `string`, optional
      - `downstreamName`: `string`, optional
      - `head`: `boolean`, optional
      - `kubernetes`: `object`, optional
      - `name`: `string`, optional
      - `nodeSelector`: `object`, optional
      - `prepareScript`: `string`, optional
      - `raw`: `object`, optional
      - `role`: `string`, required
      - `runScript`: `string`, optional
      - `sshPublicKey`: `string`, optional
      - `tensorBoardDir`: `string`, optional
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
  - `phase`: `string`, optional
  - `startTime`: `string`, optional
  - `tasks`: `array`, optional
    - `items`: `object`, optional
      - `message`: `string`, optional
      - `name`: `string`, optional
      - `phase`: `string`, optional

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
  - `agentType`: `string`, optional
  - `unschedulable`: `boolean`, optional
- `status`: `object`, optional
  - `addresses`: `array`, optional
    - `items`: `object`, optional - NodeAddress contains information for the node's address.
      - `address`: `string`, required - The node address.
      - `type`: `string`, required - Node address type, one of Hostname, ExternalIP or InternalIP.
  - `allocatable`: `object`, optional - ResourceList is a set of (resource name, quantity) pairs.
  - `capacity`: `object`, optional - ResourceList is a set of (resource name, quantity) pairs.
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
  - `nodeInfo`: `object`, optional
    - `agentVersion`: `string`, optional
    - `architecture`: `string`, optional
    - `kernelVersion`: `string`, optional
    - `operatingSystem`: `string`, optional
  - `phase`: `string`, optional
  - `pullProgress`: `array`, optional
    - `items`: `object`, optional - PullProgress captures the progress of an in-flight image pull on a node.
      - `downloaded`: `integer`, required
      - `image`: `string`, required
      - `message`: `string`, optional
      - `speed`: `number`, required
      - `status`: `string`, required
      - `total`: `integer`, required
  - `reason`: `string`, optional
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
  - `phase`: `string`, optional

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
  - `agentType`: `string`, optional
  - `docker`: `object`, optional
    - `containers`: `array`, optional
      - `items`: `object`, optional
  - `domain`: `string`, optional
  - `downstreamName`: `string`, optional
  - `kubernetes`: `object`, optional
    - `workload`: `object`, optional
      - `kind`: `string`, optional
      - `pvcStorageMap`: `object`, optional
      - `replicas`: `integer`, optional
      - `template`: `object`, optional - PodTemplateSpec describes the data a pod should have when created from a template
  - `nodeSelector`: `object`, optional
  - `prepareScript`: `string`, optional
  - `raw`: `object`, optional
    - `artifact`: `string`, optional
    - `entrypoint`: `array`, optional
    - `environment`: `array`, optional
      - `items`: `object`, optional
  - `role`: `string`, required
  - `runScript`: `string`, optional
  - `sshPublicKey`: `string`, optional
  - `tensorBoardDir`: `string`, optional
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
  - `observedNodes`: `array`, optional
  - `phase`: `string`, optional
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

## Workflow

- Group: `rlinf.io`
- Version: `v1alpha1`
- Scope: `Cluster`
- Resource: `workflows`

### Operations

#### `GET /api/v1/rlinf.io/v1alpha1/workflows`

List workflows resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `WorkflowList`
- `401` Unauthorized

#### `POST /api/v1/rlinf.io/v1alpha1/workflows`

Create a Workflow resource.

Parameters:
- `pretty` (query, optional)
- `dryRun` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Workflow`

Responses:
- `201` Created → `Workflow`
- `202` Accepted → `Workflow`
- `401` Unauthorized

#### `DELETE /api/v1/rlinf.io/v1alpha1/workflows`

Delete a collection of workflows resources.

Parameters:
- `pretty` (query, optional)
- `continue` (query, optional)
- `limit` (query, optional)
- `fieldSelector` (query, optional)
- `labelSelector` (query, optional)

Responses:
- `200` OK → `Status`
- `401` Unauthorized

#### `GET /api/v1/rlinf.io/v1alpha1/workflows/{name}`

Get a Workflow resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Workflow`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/workflows/{name}`

Replace a Workflow resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Workflow`

Responses:
- `200` OK → `Workflow`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/workflows/{name}`

Patch a Workflow resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Workflow`

Responses:
- `200` OK → `Workflow`
- `401` Unauthorized
- `404` Not Found

#### `DELETE /api/v1/rlinf.io/v1alpha1/workflows/{name}`

Delete a Workflow resource.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Status`
- `202` Accepted → `Status`
- `401` Unauthorized
- `404` Not Found

#### `GET /api/v1/rlinf.io/v1alpha1/workflows/{name}/status`

Get the status subresource for Workflow.

Parameters:
- `name` (path)
- `pretty` (query, optional)

Responses:
- `200` OK → `Workflow`
- `401` Unauthorized
- `404` Not Found

#### `PUT /api/v1/rlinf.io/v1alpha1/workflows/{name}/status`

Replace the status subresource for Workflow.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)

Request body: `Workflow`

Responses:
- `200` OK → `Workflow`
- `202` Accepted → `Workflow`
- `401` Unauthorized
- `404` Not Found

#### `PATCH /api/v1/rlinf.io/v1alpha1/workflows/{name}/status`

Patch the status subresource for Workflow.

Parameters:
- `name` (path)
- `pretty` (query, optional)
- `fieldManager` (query, optional)
- `fieldValidation` (query, optional)
- `force` (query, optional)

Request body: `Workflow`

Responses:
- `200` OK → `Workflow`
- `202` Accepted → `Workflow`
- `401` Unauthorized
- `404` Not Found

### Request Schema

- `apiVersion`: `string`, optional - APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schema...
- `kind`: `string`, optional - Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoin...
- `metadata`: `object`, optional
- `spec`: `object`, optional
  - `jobTemplates`: `array`, optional
    - `items`: `object`, optional
      - `dependencies`: `array`, optional
      - `name`: `string`, optional
      - `spec`: `object`, optional
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
  - `jobs`: `array`, optional
    - `items`: `object`, optional
      - `message`: `string`, optional
      - `name`: `string`, optional
      - `phase`: `string`, optional
  - `phase`: `string`, optional
  - `startTime`: `string`, optional
