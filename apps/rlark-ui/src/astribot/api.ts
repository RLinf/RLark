// The console's requests to the Gateway routes this fork adds. backend.ts keeps RLark's own.
import { request, requestJson, withQuery } from "../api.js";
import type { AdmittedMachine, IssuedNodeAdmission, MachineList, NodeAdmission, NodeClassAccess, SiteNode } from "./lib/siteNodeDisplay.js";
import type { NodeJoin } from "./lib/nodeJoin.js";
import type { ClusterProvision, NewClusterForm } from "./lib/clusterProvision.js";
import type { RobotPowerDesired } from "./lib/siteNodeDisplay.js";

interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

// A page of a Station API list: continue asks for the next.
type Page<T> = { items: T[]; continue?: string };

// A site node: the NetworkEnrollment of the node its name is, as GET /api/v1/station/stations lists it.
export interface StationItem {
  apiVersion: string;
  kind: "NetworkEnrollment";
  metadata: { name: string; namespace: string; uid?: string; creationTimestamp?: string };
  spec: {
    clusterID: string;
    nodeUID: string;
    systemUUID?: string;
    publicKey?: string;
    // The node was removed: the record stays until it is deleted or the node is admitted again.
    retired?: boolean;
  };
  status?: {
    // The node's last report; nodeUID is the Node that made it.
    observation?: { nodeUID: string; systemUUID?: string; reportedAt: string };
    node?: {
      state?: "Bound" | "Offline" | "Conflict" | "Retired";
      bootTime?: string;
      lastSeenTime?: string;
      reboots?: number;
      conditions?: Condition[];
    };
  };
}

export interface NodeOperation {
  apiVersion: string;
  kind: "NodeOperation";
  // annotations: what a removal's PowerOff stage found (station.rlark.io/power-off).
  metadata: { name: string; namespace: string; uid?: string; creationTimestamp?: string; annotations?: Record<string, string> };
  spec: {
    requestID: string;
    action: "Join" | "Remove";
    clusterID: string;
    nodeName: string;
    nodeUID?: string;
    requestedBy: string;
  };
  status?: {
    phase?: "Pending" | "Running" | "Blocked" | "Succeeded" | "Failed";
    stage?: string;
    nodeUID?: string;
    conditions?: Condition[];
  };
}

export const clusterProvisionsApi = {
  async list() {
    const response = await requestJson<{ data?: ClusterProvision[] }>("/api/v1/cluster-provisions");
    return response.data ?? [];
  },
  async get(name: string) {
    const response = await requestJson<{ data: ClusterProvision }>(
      `/api/v1/cluster-provisions/${encodeURIComponent(name)}`,
    );
    return response.data;
  },
  async create(form: NewClusterForm) {
    const response = await requestJson<{ data: ClusterProvision }>("/api/v1/cluster-provisions", {
      method: "POST",
      body: {
        site: form.site.trim(),
        cluster: form.cluster.trim(),
        address: form.address.trim(),
        port: Number(form.port),
        user: form.user.trim(),
        password: form.password,
        ...(form.tunnelEndpoint.trim() ? { tunnel_endpoint: form.tunnelEndpoint.trim() } : {}),
      },
    });
    return response.data;
  },
  async retry(name: string, password: string, user: string) {
    const response = await requestJson<{ data: ClusterProvision }>(
      `/api/v1/cluster-provisions/${encodeURIComponent(name)}/retry`,
      { method: "POST", body: { password, user: user.trim() } },
    );
    return response.data;
  },
};

export const stationsApi = {
  list(query: Record<string, string | undefined> = {}, signal?: AbortSignal) {
    return requestJson<Page<StationItem>>(withQuery("/api/v1/station/stations", query), { signal });
  },
  get(name: string, clusterID: string, signal?: AbortSignal) {
    return requestJson<StationItem>(withQuery(`/api/v1/station/stations/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { signal });
  },
  listNodeOperations(clusterID: string, signal?: AbortSignal) {
    return requestJson<Page<NodeOperation>>(withQuery("/api/v1/station/node-operations", { cluster_id: clusterID, limit: 100 }), { signal });
  },
  getNodeOperation(name: string, clusterID: string, signal?: AbortSignal) {
    return requestJson<NodeOperation>(withQuery(`/api/v1/station/node-operations/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { signal });
  },
  // The desired power of a robot's Node, which its Addon converges on; the Gateway records who set it.
  setRobotPower(node: string, namespace: string, desired: RobotPowerDesired) {
    return request(withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/robot-power`, { namespace }), { method: "PUT", body: { desired } });
  },
};

export const siteNodesApi = {
  list(clusterID: string, signal?: AbortSignal) {
    return requestJson<{ items: SiteNode[] }>(
      withQuery("/api/v1/site-nodes", { cluster_id: clusterID }),
      { signal },
    );
  },
};

// Joins over SSH: RLark logs in to the machine with the account given and joins it in stages, which the NodeJoin
// records; the password is sent with the request that starts or retries a join only.
export const nodeJoinsApi = {
  async list(clusterID: string, signal?: AbortSignal) {
    const response = await requestJson<{ items?: NodeJoin[] }>(withQuery("/api/v1/station/node-joins", { cluster_id: clusterID }), { signal });
    return response.items ?? [];
  },
  async get(name: string) {
    const response = await requestJson<{ data: NodeJoin }>(`/api/v1/station/node-joins/${encodeURIComponent(name)}`);
    return response.data;
  },
  async create(input: {
    cluster_id: string;
    address: string;
    port?: number;
    user: string;
    password: string;
    node_name?: string;
    tunnel_endpoint?: string;
  }) {
    const response = await requestJson<{ data: NodeJoin }>("/api/v1/station/node-joins", { method: "POST", body: input });
    return response.data;
  },
  async retry(name: string, password: string, user: string) {
    const response = await requestJson<{ data: NodeJoin }>(`/api/v1/station/node-joins/${encodeURIComponent(name)}/retry`, {
      method: "POST",
      body: { password, user: user.trim() },
    });
    return response.data;
  },
  async clear(name: string, password: string, user: string, machineGone: boolean) {
    const response = await requestJson<{ steps?: string[] }>(`/api/v1/station/node-joins/${encodeURIComponent(name)}/clear`, {
      method: "POST",
      body: { password, user: user.trim(), machine_gone: machineGone },
    });
    return response.steps ?? [];
  },
};

// Join codes. The code is in the answer of issue and reissue only; the
// management plane keeps its hash.
export const nodeAdmissionsApi = {
  list(clusterID: string, signal?: AbortSignal) {
    return requestJson<{ items: NodeAdmission[] }>(
      withQuery("/api/v1/station/node-admissions", { cluster_id: clusterID }),
      { signal },
    );
  },
  issue(input: {
    cluster_id: string;
    node_name: string;
    site?: string;
    address?: string;
    uplink?: string;
    node_class?: string;
    labels?: Record<string, string>;
    taints?: string[];
    ttl_seconds?: number;
  }) {
    return requestJson<IssuedNodeAdmission>("/api/v1/station/node-admissions", {
      method: "POST",
      body: input,
    });
  },
  reissue(name: string, clusterID: string, ttlSeconds?: number) {
    return requestJson<IssuedNodeAdmission>(
      withQuery(`/api/v1/station/node-admissions/${encodeURIComponent(name)}/reissue`, { cluster_id: clusterID }),
      { method: "POST", body: ttlSeconds ? { ttl_seconds: ttlSeconds } : {} },
    );
  },
  revoke(name: string, clusterID: string) {
    return requestJson<{ admission: NodeAdmission }>(
      withQuery(`/api/v1/station/node-admissions/${encodeURIComponent(name)}/revoke`, { cluster_id: clusterID }),
      { method: "POST" },
    );
  },
};

export const machinesApi = {
  list(signal?: AbortSignal) {
    return requestJson<MachineList>("/api/v1/station/machines", { signal });
  },
  admit(input: { cluster_id: string; machines: string[]; labels?: Record<string, string> }) {
    return requestJson<{ machines: AdmittedMachine[] }>("/api/v1/station/machines/admit", {
      method: "POST",
      body: input,
    });
  },
  // The node's passwords of a NodeClass: written, never read back.
  setClassAccess(nodeClass: string, input: { node_password: string; sudo_password?: string }) {
    return requestJson<NodeClassAccess>(`/api/v1/station/node-classes/${encodeURIComponent(nodeClass)}/access`, {
      method: "PUT",
      body: input,
    });
  },
};

// Joining again and starting over (astribot/docs/install-join.md): a node removed by its name, a cluster's
// making cleared, a site node's record deleted.
export const selfHealApi = {
  // Removes whatever is under the name: the Node its cluster has (nodeUID names the instance RLark verified, when
  // it has one), or, when its cluster has none, what RLark keeps of the name.
  removeNode(clusterID: string, nodeName: string, requestID: string, nodeUID?: string) {
    return requestJson<NodeOperation>("/api/v1/station/node-operations", {
      method: "POST",
      body: { cluster_id: clusterID, request_id: requestID, action: "Remove", node_name: nodeName, ...(nodeUID ? { node_uid: nodeUID } : {}) },
    });
  },
  // Takes away a making that stopped, or what the making of a deleted cluster left; answers what it did.
  async clearClusterProvision(name: string, password: string, user: string, machineGone: boolean) {
    const response = await requestJson<{ steps?: string[] }>(
      `/api/v1/cluster-provisions/${encodeURIComponent(name)}/clear`,
      { method: "POST", body: { password, user: user.trim(), machine_gone: machineGone } },
    );
    return response.steps ?? [];
  },
  // Deletes the record of a removed node, of a node in Conflict, or of a name no node reports under and RLark has
  // no Node of.
  deleteStation(name: string, clusterID: string) {
    return request(withQuery(`/api/v1/station/stations/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { method: "DELETE" });
  },
};

/** One tag with image metadata read by the Gateway; `error` marks unreadable metadata. */
export interface RegistryTagDetail {
  tag: string;
  digest?: string;
  created?: string;
  size?: number;
  platforms?: string[];
  error?: "unavailable";
}

export interface RegistryTagDetailPage {
  items: RegistryTagDetail[];
  next: string;
}

// Read-only browsing of a configured image registry through the Gateway; pages continue from `next` as `last`.
export const registryBrowseApi = {
  repositories(id: string, last = "", signal?: AbortSignal) {
    return requestJson<{ items: string[]; next: string }>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/repositories`, { last }), { signal });
  },
  tags(id: string, repository: string, last = "", signal?: AbortSignal) {
    return requestJson<{ items: string[]; next: string }>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/tags`, { repository, last }), { signal });
  },
  tagDetails(id: string, repository: string, last = "", signal?: AbortSignal) {
    return requestJson<RegistryTagDetailPage>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/tag-details`, { repository, last }), { signal });
  },
};
